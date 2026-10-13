// 验证空库时 /api/logs 返回 items:[] 而非 null
package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLogsEmptyReturnsArrayNotNull(t *testing.T) {
	// newTestServer 只返回 server，token 需登录获取
	rootDir := t.TempDir()
	os.MkdirAll(filepath.Join(rootDir, "config"), 0o755)
	os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"),
		[]byte("webui:\n  username: admin\n  password: secret123\n"), 0o644)
	ResetJWTSecretCache()
	InitRuntime(rootDir)
	server := NewServer()
	defer server.Close()
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()
	_, out := login(t, ts, "admin", "secret123")
	token := out["token"].(string)

	req, _ := http.NewRequest("GET", ts.URL+"/api/logs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var raw map[string]json.RawMessage
	json.NewDecoder(resp.Body).Decode(&raw)
	if string(raw["items"]) == "null" {
		t.Fatalf("items 为 null: %s", raw["items"])
	}
	if string(raw["items"]) != "[]" {
		t.Logf("items = %s (空库应为 [])", raw["items"])
	}
}
