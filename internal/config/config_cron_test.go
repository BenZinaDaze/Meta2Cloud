package config

import "testing"

// 轮询定时 cron 化：新键读取、旧键静默忽略、显式空串＝不定时。
func TestParseCronKeys(t *testing.T) {
	data := []byte(`
u115:
  auto_organize_enabled: true
  auto_organize_cron: "*/10 * * * *"
rss:
  poll_cron: "0 8 * * *"
strm:
  sync_cron: "@every 30m"
trakt:
  sync_cron: "0 4 * * *"
`)
	cfg, err := parseConfig(data)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.U115.AutoOrganizeCron != "*/10 * * * *" {
		t.Errorf("AutoOrganizeCron = %q", cfg.U115.AutoOrganizeCron)
	}
	if cfg.Rss.PollCron != "0 8 * * *" {
		t.Errorf("PollCron = %q", cfg.Rss.PollCron)
	}
	if cfg.Strm.SyncCron != "@every 30m" {
		t.Errorf("SyncCron = %q", cfg.Strm.SyncCron)
	}
	if cfg.Trakt.SyncCron != "0 4 * * *" {
		t.Errorf("Trakt SyncCron = %q", cfg.Trakt.SyncCron)
	}
}

func TestParseCronLegacyKeysSilentlyIgnored(t *testing.T) {
	data := []byte(`
u115:
  auto_organize_poll_seconds: 45
rss:
  poll_seconds: 300
strm:
  sync_interval_seconds: 600
trakt:
  sync_interval_seconds: 900
`)
	cfg, err := parseConfig(data)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	// 旧键不报错、不读取：u115 回落每分钟默认（等价旧默认 45s 轮询，存量部署不停摆）；
	// strm 保持默认空（等价旧默认 0＝关闭定时）
	if cfg.U115.AutoOrganizeCron != "* * * * *" {
		t.Errorf("AutoOrganizeCron 默认应为 * * * * *，实际 %q", cfg.U115.AutoOrganizeCron)
	}
	if cfg.Strm.SyncCron != "" {
		t.Errorf("SyncCron 默认应为空，实际 %q", cfg.Strm.SyncCron)
	}
	// rss.poll_cron 键缺失时取默认值（等价旧默认 300s）
	if cfg.Rss.PollCron != "*/5 * * * *" {
		t.Errorf("PollCron 默认应为 */5 * * * *，实际 %q", cfg.Rss.PollCron)
	}
	// trakt.sync_cron 键缺失时取默认值（等价旧默认 900s，存量部署不停摆）
	if cfg.Trakt.SyncCron != "*/15 * * * *" {
		t.Errorf("Trakt SyncCron 默认应为 */15 * * * *，实际 %q", cfg.Trakt.SyncCron)
	}
}

func TestParseCronExplicitEmptyMeansDisabled(t *testing.T) {
	data := []byte(`
rss:
  poll_cron: ""
strm:
  sync_cron: "  "
trakt:
  sync_cron: ""
`)
	cfg, err := parseConfig(data)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	// 显式空串（含纯空白）保留为空＝不定时，不回退默认值
	if cfg.Rss.PollCron != "" {
		t.Errorf("显式空串 PollCron 应为空，实际 %q", cfg.Rss.PollCron)
	}
	if cfg.Strm.SyncCron != "" {
		t.Errorf("显式空白 SyncCron 应去空白后为空，实际 %q", cfg.Strm.SyncCron)
	}
	if cfg.Trakt.SyncCron != "" {
		t.Errorf("显式空串 Trakt SyncCron 应为空，实际 %q", cfg.Trakt.SyncCron)
	}
}

// 非字符串标量（手改 YAML 如 sync_cron: 123）转成字符串：
// 交由运行时 cron 解析报错，而非静默回退默认（与 WebUI 保存校验行为对齐）。
func TestParseCronNonStringCoerced(t *testing.T) {
	data := []byte(`
u115:
  auto_organize_cron: 0
strm:
  sync_cron: 123
`)
	cfg, err := parseConfig(data)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.U115.AutoOrganizeCron != "0" {
		t.Errorf("AutoOrganizeCron = %q，期望 \"0\"", cfg.U115.AutoOrganizeCron)
	}
	if cfg.Strm.SyncCron != "123" {
		t.Errorf("SyncCron = %q，期望 \"123\"", cfg.Strm.SyncCron)
	}
}

func TestDefaultCronValues(t *testing.T) {
	def := Default()
	if def.U115.AutoOrganizeCron != "* * * * *" {
		t.Errorf("AutoOrganizeCron 默认应为 * * * * *，实际 %q", def.U115.AutoOrganizeCron)
	}
	if def.Strm.SyncCron != "" {
		t.Errorf("SyncCron 默认应为空，实际 %q", def.Strm.SyncCron)
	}
	if def.Rss.PollCron != "*/5 * * * *" {
		t.Errorf("PollCron 默认应为 */5 * * * *，实际 %q", def.Rss.PollCron)
	}
	if def.Trakt.SyncCron != "*/15 * * * *" {
		t.Errorf("Trakt SyncCron 默认应为 */15 * * * *，实际 %q", def.Trakt.SyncCron)
	}
}

func TestParseRssDefaults(t *testing.T) {
	got := parseRss(nil)
	if got.PollCron != "*/5 * * * *" {
		t.Errorf("PollCron = %q", got.PollCron)
	}
}
