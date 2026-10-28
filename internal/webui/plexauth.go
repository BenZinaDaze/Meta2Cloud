// Package webui —— Plex 配对态持久化（plex-auth.json）与 OAuth 后的运行时缓存失效（自 traktsvc.go 拆出）。
package webui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"time"
)

// ── Plex 配对态持久化（config/plex-auth.json，0o600）────────

type plexAuthJSON struct {
	AuthToken        string `json:"auth_token"`
	ClientIdentifier string `json:"client_identifier"`
	CreatedAt        int64  `json:"created_at"`
}

func plexAuthPath() string {
	return resolveConfigPath(RT().GetConfig().Plex.TokenJSON)
}

func loadPlexAuth(path string) (*plexAuthJSON, error) {
	return loadJSONSession[plexAuthJSON](path)
}

func savePlexAuth(path string, auth *plexAuthJSON) error {
	return saveJSONSession(path, auth)
}

// ensurePlexAuthLocked 确保 client_identifier 存在（需已持 rt.mu；只做文件 I/O）。
func ensurePlexAuthLocked(rt *Runtime) *plexAuthJSON {
	cfg := rt.getConfigLocked()
	tokenPath := cfg.Plex.TokenJSON
	if !filepath.IsAbs(tokenPath) {
		tokenPath = filepath.Join(rt.RootDir, tokenPath)
	}
	if auth, err := loadPlexAuth(tokenPath); err == nil && auth.ClientIdentifier != "" {
		return auth
	}
	identifier := generateClientIdentifier()
	auth := &plexAuthJSON{ClientIdentifier: identifier, CreatedAt: time.Now().Unix()}
	if old, err := loadPlexAuth(tokenPath); err == nil {
		auth.AuthToken = old.AuthToken
		auth.CreatedAt = old.CreatedAt
	}
	if err := savePlexAuth(tokenPath, auth); err != nil {
		return nil
	}
	return auth
}

func generateClientIdentifier() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(buf[0:4]), hex.EncodeToString(buf[4:6]),
		hex.EncodeToString(buf[6:8]), hex.EncodeToString(buf[8:10]),
		hex.EncodeToString(buf[10:16]))
}

// ── 运行时缓存失效 ─────────────────────────────────────────

func invalidateTraktRuntimeCache() {
	rt := RT()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.traktClient = nil
}

func invalidatePlexRuntimeCache() {
	rt := RT()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.plexClient = nil
}
