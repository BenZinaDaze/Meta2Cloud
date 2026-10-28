// health.go —— 服务配置失效/恢复的健康监测（u115 / Plex / Trakt / 订阅源）：
// 各后台循环在既有错误路径上上报观测，本模块按服务独立维护健康态，
// 失效边沿、每日提醒与恢复边沿发送 Telegram 通知，并同步写应用日志。
package webui

import (
	"fmt"
	"html"
	"strings"
	"time"
)

// healthNotifyCooldown 同一服务持续失效时的重复提醒冷却（1 天）。
const healthNotifyCooldown = 24 * time.Hour

// serviceHealth 单服务健康状态。
type serviceHealth struct {
	failing    bool
	reason     string
	notifiedAt time.Time // 最近一次失效通知时刻（冷却用）
}

// healthLogCategory 服务名 → 应用日志 category（保证「日志」页可按服务过滤）。
var healthLogCategory = map[string]string{
	"115": "u115", "Plex": "plex", "Trakt": "trakt", "订阅源": "rss",
}

// NotifyServiceHealth 记录一次服务健康观测，并按状态边沿发送 Telegram 通知：
//   - 正常→失效：发失效通知（应用日志 ERROR）；
//   - 持续失效：距上次通知 ≥ 24h 再提醒一次，否则仅更新失效原因；
//   - 失效→正常：发恢复通知（应用日志 INFO）并清除失效态；
//   - 持续正常：无操作。
//
// service 取「115」「Plex」「Trakt」「订阅源」；TG 未配置时静默降级为仅应用日志，
// TG 发送失败记应用日志 WARN，不影响状态机。并发安全。
func (rt *Runtime) NotifyServiceHealth(service string, ok bool, reason string) {
	category := healthLogCategory[service]
	if category == "" {
		category = "u115"
	}
	rt.healthMu.Lock()
	st := rt.healthServices[service]
	if st == nil {
		st = &serviceHealth{}
		rt.healthServices[service] = st
	}
	if ok {
		if !st.failing { // 持续正常：无操作
			rt.healthMu.Unlock()
			return
		}
		st.failing = false
		st.reason = ""
		rt.healthMu.Unlock()
		rt.appLog(category, "health_recovered", "INFO",
			fmt.Sprintf("%s 配置已恢复正常", service), nil)
		rt.sendHealthTelegram(category, service, fmt.Sprintf("✅ %s 已恢复", service))
		return
	}
	// 失效观测：失败边沿立即通知，持续失效按冷却每日提醒一次（原因始终更新）
	wasFailing := st.failing
	st.reason = reason
	now := rt.healthNow()
	notify := !wasFailing || now.Sub(st.notifiedAt) >= healthNotifyCooldown
	if notify {
		st.failing = true
		st.notifiedAt = now
	}
	rt.healthMu.Unlock()
	if !notify {
		return
	}
	text := fmt.Sprintf("%s 配置失效：%s", service, healthReasonText(reason))
	rt.appLog(category, "health_failed", "ERROR", text, map[string]any{"reason": reason})
	rt.sendHealthTelegram(category, service, "⚠️ "+html.EscapeString(text))
}

// healthReasonText 失效原因展示文案（空原因兜底，避免通知残缺）。
func healthReasonText(reason string) string {
	if reason = strings.TrimSpace(reason); reason != "" {
		return reason
	}
	return "原因未明，请查看应用日志"
}

// sendHealthTelegram 发送健康通知（未配置 Telegram 静默跳过；发送失败仅记应用日志）。
func (rt *Runtime) sendHealthTelegram(category, service, text string) {
	tg := rt.healthTGConfig()
	if tg.BotToken == "" || tg.ChatID == "" {
		return
	}
	if err := rt.healthSendTG(tg, text); err != nil {
		rt.appLog(category, "health_notify_failed", "WARNING",
			fmt.Sprintf("发送 %s 健康通知失败：%v", service, err), nil)
	}
}
