package pipeline

import (
	"os"
	"testing"

	"meta2cloud/internal/config"
)

// TestNotifyPreviewManual 手动预览富通知样式：
//   TG_PREVIEW=1 go test ./internal/pipeline/ -run TestNotifyPreviewManual -v
func TestNotifyPreviewManual(t *testing.T) {
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
	provider := newFakeProvider()
	p := New(provider, cfg, Options{
		OnVideoIngested: func(ProcessResult) error { return nil },
	})
	e1, e2 := 1, 2
	base := notifyItem{
		Title: "学生会也有洞！", Year: "2026", IsTV: true,
		Season: 1, Rating: 7.7, Country: mediaCountryLabel("ja", true, true),
		Genres:      []string{"动画", "喜剧"},
		SeasonTotal: 12,
		Overview:    "高一生水之江梅有着极其高超的文学造诣，文科成绩无人能及，但与之相反的是数理化等理科却差强人意，有留级甚至影响保送的风险，为此班主任平冢推荐他加入学生会，以提高其综合能力。",
		BackdropPath: "/kuYMjcQ6BeLTBYUR1l1T8W8ZMcf.jpg",
		StrmOK:      true,
	}
	items := []notifyItem{
		func() notifyItem { it := base; it.Episode = &e1; return it }(),
		func() notifyItem { it := base; it.Episode = &e2; return it }(),
	}
	caption := p.buildNotifyCaption(base, items)
	t.Logf("caption:\n%s", caption)
	p.sendTgPhoto(cfg.Telegram.BotToken, cfg.Telegram.ChatID, base.BackdropPath, caption)
}
