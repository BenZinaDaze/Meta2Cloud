// cron.go —— cron 表达式解析与启动校验。从 webui/watcher.go 下沉到本包：
// 配置加载校验、WebUI 保存校验、后台调度三处共用同一实现，保证行为一致。
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// CronNextDelay 计算距下一次 cron 触发的等待时长（服务器本地时区，5 段式，支持 @every 简写）。
// spec 为空串返回 ok=false（未配置定时）；表达式非法返回 err。
// 另两类畸形同样按非法处理（否则 time.After(负值/1s) 会让后台循环热空转）：
//   - 合法但永不触发的表达式（如 0 0 30 2 *、0 0 31 4 *）：robfig Next 返回零时刻，
//     Sub 出负数等待，循环会被立即反复触发；
//   - @every 间隔小于 1 分钟（robfig 向上取整后可打出秒级节奏，如 @every 500ms ≙ 1s），
//     延续旧 *_interval_seconds 的 60s 下限，防止全量同步/轮询被打成秒级。
func CronNextDelay(spec string, now time.Time) (time.Duration, bool, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0, false, nil
	}
	sched, err := cron.ParseStandard(spec)
	if err != nil {
		return 0, false, err
	}
	if delay, isDelay := sched.(cron.ConstantDelaySchedule); isDelay && delay.Delay < time.Minute {
		return 0, false, fmt.Errorf("@every 间隔不得小于 1 分钟（当前 %v）", delay.Delay)
	}
	next := sched.Next(now)
	if next.IsZero() || next.Sub(now) <= 0 {
		return 0, false, fmt.Errorf("表达式合法但永远不触发（如 2 月 30 日这类不存在的日期）")
	}
	return next.Sub(now), true, nil
}

// Validate 启动期配置校验：各定时开关的 cron 表达式必须合法（空串＝不定时，放行）。
// Load 只做 YAML 语法检查，语义校验（表达式能否被调度器接受）集中在这里，
// 由 InitRuntime 启动时调用，非法即拒绝启动而非静默不调度。
func (c *Config) Validate() error {
	for _, f := range []struct{ key, spec string }{
		{"u115.auto_organize_cron", c.U115.AutoOrganizeCron},
		{"strm.sync_cron", c.Strm.SyncCron},
		{"trakt.sync_cron", c.Trakt.SyncCron},
		{"rss.poll_cron", c.Rss.PollCron},
	} {
		if _, _, err := CronNextDelay(f.spec, time.Now()); err != nil {
			return fmt.Errorf("%s 无效：%w", f.key, err)
		}
	}
	return nil
}
