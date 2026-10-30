package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeStrmURLBase(t *testing.T) {
	cases := map[string]string{
		"panoply:38765":             "http://panoply:38765",
		" http://panoply:38765/":    "http://panoply:38765",
		"https://strm.example.com":  "https://strm.example.com",
		"https://strm.example.com/": "https://strm.example.com",
		"  192.168.1.10:38765  ":    "http://192.168.1.10:38765",
		"":                          "",
		"   ":                       "",
	}
	for in, want := range cases {
		if got := normalizeStrmURLBase(in); got != want {
			t.Errorf("normalizeStrmURLBase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseStrmURLBaseNormalization(t *testing.T) {
	s := parseStrm(map[string]any{"enabled": true, "url_base": "panoply:38765"})
	if s.URLBase != "http://panoply:38765" {
		t.Errorf("URLBase = %q", s.URLBase)
	}
	s = parseStrm(map[string]any{"url_base": "http://a.b/"})
	if s.URLBase != "http://a.b" {
		t.Errorf("URLBase = %q", s.URLBase)
	}
}

func TestParseStrmRefreshPlex(t *testing.T) {
	// yaml 显式开启 → 生效（通知 Plex 扫库的闸门在 strmsvc.notifyPlexRefresh）
	if s := parseStrm(map[string]any{"refresh_plex": true}); !s.RefreshPlex {
		t.Error("refresh_plex: true 应解析为 RefreshPlex = true")
	}
	// 缺省 → 默认 false
	if s := parseStrm(map[string]any{}); s.RefreshPlex {
		t.Error("refresh_plex 缺省应解析为 RefreshPlex = false")
	}
}

func TestEnsureDefaultFiles(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	secretsPath := filepath.Join(dir, SecretsFilename)
	rulesPath := filepath.Join(dir, "parser-rules.yaml")
	catsPath := filepath.Join(dir, "categories.yaml")

	// 四件套缺失 → 全部生成
	if err := EnsureDefaultFiles(cfgPath, secretsPath, rulesPath, catsPath); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{cfgPath, secretsPath, rulesPath, catsPath} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s 未生成: %v", p, err)
		}
	}

	// 生成的主配置可被 Load 正常解析
	if _, err := Load(cfgPath); err != nil {
		t.Errorf("Load 生成的主配置失败: %v", err)
	}
	if _, err := LoadCategories(catsPath); err != nil {
		t.Errorf("LoadCategories 生成的分类配置失败: %v", err)
	}

	if os.Getenv("VERBOSE") != "" {
		if data, err := os.ReadFile(cfgPath); err == nil {
			t.Logf("生成的 config.yaml:\n%s", data)
		}
	}

	// 已存在的不覆盖：改写标记后重新生成，标记应保留
	os.WriteFile(cfgPath, []byte("标记\n"), 0o644)
	if err := EnsureDefaultFiles(cfgPath, secretsPath, rulesPath, catsPath); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(cfgPath)
	if string(data) != "标记\n" {
		t.Fatal("已存在的配置不应被覆盖")
	}
}

// TestEnsureDefaultFilesPasswordHash 首次生成的密码只落哈希：主配置无明文，
// secrets.yaml 为 0600 且含可校验的 password_hash。
func TestEnsureDefaultFilesPasswordHash(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	secretsPath := filepath.Join(dir, SecretsFilename)
	if err := EnsureDefaultFiles(cfgPath, secretsPath,
		filepath.Join(dir, "parser-rules.yaml"), filepath.Join(dir, "categories.yaml")); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WebUI.Password != "" {
		t.Errorf("首次生成的主配置不应含明文密码，got %q", cfg.WebUI.Password)
	}
	if cfg.WebUI.PasswordHash == "" {
		t.Fatal("password_hash 未生成")
	}
	if CheckPasswordHash(cfg.WebUI.PasswordHash, "错误密码") {
		t.Error("错误密码不应通过校验")
	}
	if st, err := os.Stat(secretsPath); err == nil {
		if perm := st.Mode().Perm(); perm != 0o600 {
			t.Errorf("secrets.yaml 权限 = %o, want 600", perm)
		}
	} else {
		t.Fatalf("secrets.yaml 未生成: %v", err)
	}
	// 主配置文件内容不含哈希（哈希只在 secrets.yaml）
	mainData, _ := os.ReadFile(cfgPath)
	if strings.Contains(string(mainData), "password_hash") {
		t.Error("password_hash 不应出现在主配置文件")
	}
}

// TestLoadWithSecrets secrets.yaml 中的敏感键优先于主配置，旧版主配置明文密码仍可读。
func TestLoadWithSecrets(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	main := "u115:\n  cookie: \"旧cookie\"\nwebui:\n  username: admin\n  password: \"旧明文\"\n"
	if err := os.WriteFile(cfgPath, []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	// 无 secrets.yaml → 兼容读取主配置明文
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.U115.Cookie != "旧cookie" || cfg.WebUI.Password != "旧明文" {
		t.Fatalf("兼容读取失败: cookie=%q password=%q", cfg.U115.Cookie, cfg.WebUI.Password)
	}

	// secrets.yaml 存在 → 敏感键值优先
	secrets := "u115:\n  cookie: \"新cookie\"\nwebui:\n  password_hash: \"$2a$10$hash\"\n"
	if err := os.WriteFile(filepath.Join(dir, SecretsFilename), []byte(secrets), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.U115.Cookie != "新cookie" {
		t.Errorf("Cookie = %q, want 新cookie（secrets 优先）", cfg.U115.Cookie)
	}
	if cfg.WebUI.PasswordHash != "$2a$10$hash" {
		t.Errorf("PasswordHash = %q, want 来自 secrets.yaml", cfg.WebUI.PasswordHash)
	}
	if cfg.WebUI.Password != "旧明文" {
		t.Errorf("主配置明文密码应保留可读（待迁移），got %q", cfg.WebUI.Password)
	}
}

// TestSplitMergeSecrets 拆分与合并互逆：SplitSecrets 摘出敏感键并从原 map 删除。
func TestSplitMergeSecrets(t *testing.T) {
	payload := map[string]any{
		"u115":     map[string]any{"cookie": "C", "qps": 3},
		"telegram": map[string]any{"bot_token": "T"},
		"webui":    map[string]any{"username": "admin", "password_hash": "H"},
	}
	secrets := SplitSecrets(payload)
	if len(payload["u115"].(map[string]any)) != 1 || payload["u115"].(map[string]any)["qps"] != 3 {
		t.Error("非敏感键不应被摘除")
	}
	if _, ok := payload["u115"].(map[string]any)["cookie"]; ok {
		t.Error("敏感键应从原 map 删除")
	}
	if len(secrets) != 3 {
		t.Errorf("secrets 段数 = %d, want 3", len(secrets))
	}

	// 合并回新 map → 敏感键还原
	dst := map[string]any{"u115": map[string]any{"cookie": "旧"}}
	MergeSecrets(dst, secrets)
	if dst["u115"].(map[string]any)["cookie"] != "C" {
		t.Error("MergeSecrets 未覆盖敏感键")
	}
	if dst["webui"].(map[string]any)["password_hash"] != "H" {
		t.Error("MergeSecrets 未写入 password_hash")
	}
}
