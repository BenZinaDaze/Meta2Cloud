package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeStrmURLBase(t *testing.T) {
	cases := map[string]string{
		"meta2cloud:38765":          "http://meta2cloud:38765",
		" http://meta2cloud:38765/": "http://meta2cloud:38765",
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
	s := parseStrm(map[string]any{"enabled": true, "url_base": "meta2cloud:38765"})
	if s.URLBase != "http://meta2cloud:38765" {
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
	rulesPath := filepath.Join(dir, "parser-rules.yaml")
	catsPath := filepath.Join(dir, "categories.yaml")

	// 三件套缺失 → 全部生成
	if err := EnsureDefaultFiles(cfgPath, rulesPath, catsPath); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{cfgPath, rulesPath, catsPath} {
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
	if err := EnsureDefaultFiles(cfgPath, rulesPath, catsPath); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(cfgPath)
	if string(data) != "标记\n" {
		t.Fatal("已存在的配置不应被覆盖")
	}
}
