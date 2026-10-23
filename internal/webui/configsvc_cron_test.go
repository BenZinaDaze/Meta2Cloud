package webui

import "testing"

// 保存校验：四个 cron 新键非空值须能被 robfig 标准解析，非法拒绝、空值放行；
// 旧数值间隔键不再校验（残留静默放行）。
func TestValidateMainConfigPayloadCron(t *testing.T) {
	// 非法 cron 被拒绝，错误信息明确
	payload := map[string]any{"rss": map[string]any{"poll_cron": "not a cron"}}
	if msg := validateMainConfigPayload(payload); msg == "" {
		t.Fatal("非法 cron 应被拒绝")
	}

	payload = map[string]any{"u115": map[string]any{"auto_organize_cron": "99 * * * *"}}
	if msg := validateMainConfigPayload(payload); msg == "" {
		t.Fatal("非法 auto_organize_cron 应被拒绝")
	}

	payload = map[string]any{"strm": map[string]any{"sync_cron": 123}}
	if msg := validateMainConfigPayload(payload); msg == "" {
		t.Fatal("非字符串 sync_cron 应被拒绝")
	}

	payload = map[string]any{"trakt": map[string]any{"sync_cron": "not a cron"}}
	if msg := validateMainConfigPayload(payload); msg == "" {
		t.Fatal("非法 trakt.sync_cron 应被拒绝")
	}

	// 合法但永不触发的表达式（robfig 可解析、Next 为零时刻）同样拒绝
	payload = map[string]any{"trakt": map[string]any{"sync_cron": "0 0 30 2 *"}}
	if msg := validateMainConfigPayload(payload); msg == "" {
		t.Fatal("永不触发的 cron 应被拒绝")
	}

	// @every 间隔下限 1 分钟（延续旧 sync_interval_seconds 的 60s 下限）
	payload = map[string]any{"trakt": map[string]any{"sync_cron": "@every 1s"}}
	if msg := validateMainConfigPayload(payload); msg == "" {
		t.Fatal("@every 低于 1 分钟应被拒绝")
	}

	// 合法表达式（含 @every 简写）通过
	payload = map[string]any{
		"u115":  map[string]any{"auto_organize_cron": "*/5 * * * *"},
		"rss":   map[string]any{"poll_cron": "0 8 * * *"},
		"strm":  map[string]any{"sync_cron": "@every 1h"},
		"trakt": map[string]any{"sync_cron": "@every 15m"},
	}
	if msg := validateMainConfigPayload(payload); msg != "" {
		t.Fatalf("合法 cron 不应报错: %s", msg)
	}

	// 空值放行（不定时）
	payload = map[string]any{
		"u115":  map[string]any{"auto_organize_cron": ""},
		"rss":   map[string]any{"poll_cron": ""},
		"strm":  map[string]any{"sync_cron": ""},
		"trakt": map[string]any{"sync_cron": ""},
	}
	if msg := validateMainConfigPayload(payload); msg != "" {
		t.Fatalf("空 cron 应放行: %s", msg)
	}

	// 旧数值间隔键残留：不再校验，静默放行
	payload = map[string]any{
		"u115":  map[string]any{"auto_organize_poll_seconds": "abc"},
		"rss":   map[string]any{"poll_seconds": "not-a-number"},
		"strm":  map[string]any{"sync_interval_seconds": "abc"},
		"trakt": map[string]any{"sync_interval_seconds": "not-a-number"},
	}
	if msg := validateMainConfigPayload(payload); msg != "" {
		t.Fatalf("旧键残留应静默放行: %s", msg)
	}
}
