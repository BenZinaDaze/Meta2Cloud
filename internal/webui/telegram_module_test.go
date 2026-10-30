package webui

import (
	"os"
	"testing"

	"panoply/internal/config"
	"panoply/internal/telegram"
	"panoply/internal/tmdb"
)

// TestTelegramModuleFromExternal 从外部包（webui）调用 pipeline 通知模块的联调测试，
// 验证导出的 SendTelegramMessage / SendTelegramPhoto 可被任意模块复用。
// 手动运行：TG_PREVIEW=1 go test ./internal/webui/ -run TestTelegramModuleFromExternal -v
// （会真实发送两条 Telegram 消息；默认跳过，不影响 CI）
func TestTelegramModuleFromExternal(t *testing.T) {
	if os.Getenv("TG_PREVIEW") == "" {
		t.Skip("设置 TG_PREVIEW=1 手动运行（会真实发送 Telegram 消息）")
	}
	cfg, err := config.Load("../../config/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Telegram.BotToken == "" || cfg.Telegram.ChatID == "" {
		t.Skip("config.yaml 未配置 Telegram")
	}

	// ① 文本通知
	if err := telegram.SendMessage(cfg.Telegram, "🧩 <b>模块联调</b> · SendTelegramMessage（webui → pipeline）"); err != nil {
		t.Fatalf("SendTelegramMessage: %v", err)
	}

	// ② 图片通知（TMDB 横版背景图）
	imageURL := tmdb.ImageURL("/kuYMjcQ6BeLTBYUR1l1T8W8ZMcf.jpg", "w780", cfg.TmdbImageBaseURL())
	if err := telegram.SendPhoto(cfg.Telegram, imageURL, "🧩 <b>模块联调</b> · SendTelegramPhoto"); err != nil {
		t.Fatalf("SendTelegramPhoto: %v", err)
	}
}
