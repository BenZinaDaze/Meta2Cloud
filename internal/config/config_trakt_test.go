package config

import "testing"

func TestParseTraktDefaults(t *testing.T) {
	got := parseTrakt(nil)
	if got.Enabled {
		t.Error("Enabled 默认应为 false")
	}
	if got.TokenJSON != "config/trakt-token.json" {
		t.Errorf("TokenJSON = %q", got.TokenJSON)
	}
	if got.SyncIntervalSeconds != 900 {
		t.Errorf("SyncIntervalSeconds = %d", got.SyncIntervalSeconds)
	}
	if got.SessionPollSeconds != 45 {
		t.Errorf("SessionPollSeconds = %d", got.SessionPollSeconds)
	}
	if !got.SSEEnabled {
		t.Error("SSEEnabled 默认应为 true")
	}
	if !got.SyncToPlex || !got.SyncToTrakt {
		t.Error("方向开关默认均应开启")
	}
}

func TestParseTraktFloors(t *testing.T) {
	got := parseTrakt(map[string]any{
		"sync_interval_seconds": 10,  // 低于下限 60
		"session_poll_seconds":  5,   // 低于下限 15
		"sync_to_plex":          false,
		"client_id":             "  abc  ",
	})
	if got.SyncIntervalSeconds < 60 {
		t.Errorf("SyncIntervalSeconds 应钳到 ≥60，实际 %d", got.SyncIntervalSeconds)
	}
	if got.SessionPollSeconds < 15 {
		t.Errorf("SessionPollSeconds 应钳到 ≥15，实际 %d", got.SessionPollSeconds)
	}
	if got.SyncToPlex {
		t.Error("显式 false 不应被覆盖")
	}
	if got.ClientID != "abc" {
		t.Errorf("ClientID 应去空白，实际 %q", got.ClientID)
	}
}

func TestParsePlexDefaults(t *testing.T) {
	got := parsePlex(nil)
	if got.TokenJSON != "config/plex-auth.json" {
		t.Errorf("TokenJSON = %q", got.TokenJSON)
	}
	if got.Timeout != 10 {
		t.Errorf("Timeout = %d", got.Timeout)
	}
	if got.BaseURL != "" {
		t.Errorf("BaseURL 默认应为空，实际 %q", got.BaseURL)
	}
}

func TestParsePlexBaseURLNormalized(t *testing.T) {
	got := parsePlex(map[string]any{"base_url": " http://192.168.1.10:32400/ ", "timeout": 0})
	if got.BaseURL != "http://192.168.1.10:32400" {
		t.Errorf("BaseURL = %q", got.BaseURL)
	}
	if got.Timeout < 1 {
		t.Errorf("Timeout 应钳到 ≥1，实际 %d", got.Timeout)
	}
}

func TestLoadTraktPlexFromYAML(t *testing.T) {
	cfg, err := parseConfig([]byte(`
trakt:
  enabled: true
  client_id: cid
  client_secret: secret
plex:
  base_url: http://plex.local:32400
`))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Trakt.Enabled || cfg.Trakt.ClientID != "cid" {
		t.Errorf("trakt 段解析不符：%+v", cfg.Trakt)
	}
	if cfg.Plex.BaseURL != "http://plex.local:32400" {
		t.Errorf("plex 段解析不符：%+v", cfg.Plex)
	}
	if !cfg.IsTraktReady() {
		t.Error("IsTraktReady 应为 true")
	}
	if !cfg.IsPlexReady() {
		t.Error("IsPlexReady 应为 true")
	}
}
