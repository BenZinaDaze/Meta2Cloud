// miscroutes.go —— MediaVault/Telegram 测试 + TMDB 季详情 + WebSocket 日志流。
package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"meta2cloud/internal/config"
	"meta2cloud/internal/pipeline"
)

// handleMediavaultTest 测试 MediaVault 连接。
func handleMediavaultTest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Scheme  string `json:"scheme"`
		Host    string `json:"host"`
		APIKey  string `json:"api_key"`
		Timeout int    `json:"timeout"`
	}
	_ = jsonDecodeBody(r, &body)
	cfg := RT().GetConfig().MediaVault
	scheme := body.Scheme
	if scheme == "" {
		scheme = cfg.Scheme
	}
	host := body.Host
	if host == "" {
		host = cfg.Host
	}
	apiKey := body.APIKey
	if apiKey == "" {
		apiKey = cfg.APIKey
	}
	timeout := body.Timeout
	if timeout == 0 {
		timeout = cfg.Timeout
	}

	scheme = strings.ToLower(strings.TrimSpace(scheme))
	if scheme != "http" && scheme != "https" {
		scheme = "http"
	}
	host = strings.TrimRight(hostPrefixRe.ReplaceAllString(strings.TrimSpace(host), ""), "/")
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		writeJSONError(w, http.StatusBadRequest, "MediaVault API Key 不能为空")
		return
	}
	if timeout < 1 || timeout > 120 {
		writeJSONError(w, http.StatusBadRequest, "MediaVault 请求超时必须在 1 到 120 之间")
		return
	}
	if host == "" {
		writeJSONError(w, http.StatusBadRequest, "MediaVault Host 不能为空")
		return
	}
	endpoint := fmt.Sprintf("%s://%s/api/v1", scheme, host)
	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	resp, err := client.Post(endpoint+"?api_key="+url.QueryEscape(apiKey), "", nil)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "无法连接 MediaVault："+err.Error())
		return
	}
	defer resp.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadGateway, "MediaVault 返回了无效响应")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "payload": payload, "endpoint": endpoint})
}

// handleTelegramTest 发送一条 Telegram 测试通知（表单值优先，缺省回退已保存配置）。
func handleTelegramTest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BotToken string `json:"bot_token"`
		ChatID   string `json:"chat_id"`
	}
	_ = jsonDecodeBody(r, &body)
	cfg := RT().GetConfig().Telegram
	botToken := strings.TrimSpace(body.BotToken)
	if botToken == "" {
		botToken = cfg.BotToken
	}
	chatID := strings.TrimSpace(body.ChatID)
	if chatID == "" {
		chatID = cfg.ChatID
	}
	if botToken == "" || chatID == "" {
		writeJSONError(w, http.StatusBadRequest, "Bot Token 与 Chat ID 均不能为空")
		return
	}
	tg := config.Telegram{BotToken: botToken, ChatID: chatID}
	if err := pipeline.SendTelegramMessage(tg, "✅ <b>Meta2Cloud</b>\nTelegram 通知配置正常，入库完成后将推送到此会话。"); err != nil {
		writeJSONError(w, http.StatusBadGateway, "测试通知发送失败："+err.Error())
		return
	}
	appLog("telegram", "test_notify", "SUCCESS", "Telegram 测试通知已发送", nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleTmdbSeasonDetail TMDB 某季详情（含每集）。
func handleTmdbSeasonDetail(w http.ResponseWriter, r *http.Request) {
	tmdbID, err := strconvAtoi64(r.PathValue("tmdbID"))
	if err != nil || tmdbID <= 0 {
		writeJSONError(w, http.StatusBadRequest, "tmdb_id 无效")
		return
	}
	season, err := strconvAtoi64(r.PathValue("season"))
	if err != nil || season < 0 {
		writeJSONError(w, http.StatusBadRequest, "season 无效")
		return
	}
	client := RT().GetTmdbClient()
	if client == nil {
		writeJSONError(w, http.StatusInternalServerError, "TMDB API Key 未配置")
		return
	}
	detail := client.GetSeasonDetail(int(tmdbID), int(season))
	if detail == nil {
		writeJSONError(w, http.StatusNotFound, "未找到季详情")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "season": detail})
}

func strconvAtoi64(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// ── WebSocket 日志广播（对应 webui/websocket.py 的最小子集） ──

type wsBroadcaster struct {
	mu    sync.Mutex
	conns map[chan map[string]any]bool
}

var broadcaster = &wsBroadcaster{conns: map[chan map[string]any]bool{}}

// BroadcastLog 广播一条日志（websocket 客户端 + app log）。
func BroadcastLog(level, message string, runID string) {
	appLog("pipeline", "pipeline_output", level, message, map[string]any{"runId": runID})
	broadcaster.mu.Lock()
	defer broadcaster.mu.Unlock()
	if len(broadcaster.conns) == 0 {
		return
	}
	payload := map[string]any{
		"type": "log", "ts": nil, "level": level, "message": message, "runId": runID,
	}
	for ch := range broadcaster.conns {
		select {
		case ch <- payload:
		default: // 慢消费者跳过
		}
	}
}

// SubscribeLog 订阅日志流（返回通道与取消函数）。
func SubscribeLog() (<-chan map[string]any, func()) {
	ch := make(chan map[string]any, 64)
	broadcaster.mu.Lock()
	broadcaster.conns[ch] = true
	broadcaster.mu.Unlock()
	cancel := func() {
		broadcaster.mu.Lock()
		delete(broadcaster.conns, ch)
		broadcaster.mu.Unlock()
	}
	return ch, cancel
}
