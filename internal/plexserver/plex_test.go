package plexserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRefreshSection(t *testing.T) {
	var gotPath, gotMethod, gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod, gotToken = r.URL.Path, r.Method, r.Header.Get("X-Plex-Token")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok-1", "cid", WithHTTPClient(srv.Client()))
	if err := client.RefreshSection("4"); err != nil {
		t.Fatalf("RefreshSection 失败：%v", err)
	}
	if gotPath != "/library/sections/4/refresh" || gotMethod != http.MethodPost {
		t.Fatalf("应 POST /library/sections/{key}/refresh，实际 %s %s", gotMethod, gotPath)
	}
	if gotToken != "tok-1" {
		t.Fatalf("应携带 X-Plex-Token")
	}
}

// TestUnauthorizedSentinel 401 应返回可 errors.Is 识别的 ErrUnauthorized（健康监测依赖此分类）。
func TestUnauthorizedSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "bad-token", "cid", WithHTTPClient(srv.Client()))
	if _, err := client.Identity(); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("Identity 401 应命中 ErrUnauthorized: %v", err)
	}
	if _, err := client.Sections(); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("Sections 401 应命中 ErrUnauthorized: %v", err)
	}
	if err := client.RefreshSection("4"); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("RefreshSection 401 应命中 ErrUnauthorized: %v", err)
	}

	// 通知流（SSE）401 同样归类为授权失效
	if err := client.StreamNotifications(context.Background(), nil, nil); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("通知流 401 应命中 ErrUnauthorized: %v", err)
	}
	// plex.tv 分支（PIN 轮询）401 同样归类为授权失效
	pinClient := NewClient(srv.URL, "", "cid", WithHTTPClient(srv.Client()))
	pinClient.PlexTVBase = srv.URL
	if _, err := pinClient.PollPIN(42); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("plex.tv 401 应命中 ErrUnauthorized: %v", err)
	}
	// 普通错误不误报
	if errors.Is(ErrNotificationsUnsupported, ErrUnauthorized) {
		t.Error("SSE 不支持不应误判为授权失效")
	}
}
