package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Load 快速失败：文件存在但 YAML 畸形必须返回错误而非静默回退默认值——
// 否则手改配置的缩进错误会让应用带着清零的 root folder 继续运行。
func TestLoadMalformedYAMLError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("u115:\n  root_folder_id: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if cfg, err := Load(cfgPath); err == nil {
		t.Fatalf("畸形 YAML 应返回错误，实际返回配置：%+v", cfg)
	}
}

func TestLoadMalformedSecretsError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("webui:\n  username: admin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, SecretsFilename), []byte("webui: [broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(cfgPath); err == nil {
		t.Fatal("畸形 secrets.yaml 应返回错误")
	}
}

func TestLoadMalformedRulesError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("webui:\n  username: admin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, parserRulesFilename), []byte("parser: {custom_words:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("畸形 parser-rules.yaml 应返回错误")
	}
	if !strings.Contains(err.Error(), parserRulesFilename) {
		t.Errorf("错误信息应包含文件路径，实际：%v", err)
	}
}

// 文件不存在仍是默认配置（首次启动路径，缺失文件由 EnsureDefaultFiles 生成）。
func TestLoadMissingFilesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if err != nil {
		t.Fatalf("缺失文件不应报错：%v", err)
	}
	if cfg.WebUI.Username != "admin" || cfg.U115.AutoOrganizeCron != "* * * * *" {
		t.Errorf("应返回默认配置，实际 username=%q cron=%q", cfg.WebUI.Username, cfg.U115.AutoOrganizeCron)
	}
}

// Validate 启动校验：默认配置全部通过；非法/永不触发/秒级 @every 拒绝；空串＝不定时放行。
func TestConfigValidateCron(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("默认配置应通过校验：%v", err)
	}
	cases := []struct{ key, spec string }{
		{"u115.auto_organize_cron", "not a cron"},
		{"strm.sync_cron", "0 0 30 2 *"}, // 合法但永不触发
		{"trakt.sync_cron", "@every 1s"}, // 秒级节奏
		{"rss.poll_cron", "99 * * * *"},
	}
	for _, tc := range cases {
		cfg := Default()
		switch tc.key {
		case "u115.auto_organize_cron":
			cfg.U115.AutoOrganizeCron = tc.spec
		case "strm.sync_cron":
			cfg.Strm.SyncCron = tc.spec
		case "trakt.sync_cron":
			cfg.Trakt.SyncCron = tc.spec
		case "rss.poll_cron":
			cfg.Rss.PollCron = tc.spec
		}
		err := cfg.Validate()
		if err == nil {
			t.Errorf("%s = %q 应校验失败", tc.key, tc.spec)
			continue
		}
		if !strings.Contains(err.Error(), tc.key) {
			t.Errorf("错误信息应包含键名 %s，实际：%v", tc.key, err)
		}
	}
	empty := Default()
	empty.Rss.PollCron = ""
	empty.Strm.SyncCron = "   "
	if err := empty.Validate(); err != nil {
		t.Fatalf("空串＝不定时应放行：%v", err)
	}
}
