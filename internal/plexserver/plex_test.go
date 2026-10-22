package plexserver

import (
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
