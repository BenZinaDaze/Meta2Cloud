// Package webui —— 设备码/PIN 会话文件的通用持久化（0600）。
// 各 OAuth 会话（Trakt 设备码、Plex PIN、Plex 配对态、115 扫码）共用同一套
// 「MkdirAll + MarshalIndent + WriteFile 0600 + chmod 收紧」与「ReadFile + Unmarshal」模式。
package webui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// saveJSONSession 将会话对象写入 path（0600，已存在文件显式收紧权限）。
func saveJSONSession[T any](path string, v *T) error {
	if dir := dirOf(path); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	// WriteFile 的 perm 仅对新建文件生效，已存在文件显式收紧
	return os.Chmod(path, 0o600)
}

// loadJSONSession 从 path 读取会话对象；文件不存在返回 os.ErrNotExist 包装错误。
func loadJSONSession[T any](path string) (*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// missingSessionErr 读取失败且文件不存在时返回面向用户的提示，其他错误原样透传。
func missingSessionErr(err error, missing string) error {
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s", missing)
	}
	return err
}

// dirOf 返回目录部分；当前目录返回空串（无需 MkdirAll）。
func dirOf(path string) string {
	dir := filepath.Dir(path)
	if dir == "." {
		return ""
	}
	return dir
}

// ── Trakt 设备码会话 ────────────────────────────────────────

type traktDeviceSessionJSON struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresAt       int64  `json:"expires_at"`
}

func traktSessionPath() string {
	return resolveConfigPath("config/trakt-device-session.json")
}

func saveTraktDeviceSession(sess *traktDeviceSessionJSON) error {
	return saveJSONSession(traktSessionPath(), sess)
}

func loadTraktDeviceSession() (*traktDeviceSessionJSON, error) {
	sess, err := loadJSONSession[traktDeviceSessionJSON](traktSessionPath())
	if err != nil {
		return nil, missingSessionErr(err, "Trakt 配对会话不存在，请先发起配对")
	}
	return sess, nil
}

// ── Plex PIN 会话 ───────────────────────────────────────────

type plexPinSessionJSON struct {
	PinID            int    `json:"pin_id"`
	Code             string `json:"code"`
	ClientIdentifier string `json:"client_identifier"`
}

func plexPinSessionPath() string {
	return resolveConfigPath("config/plex-pin-session.json")
}

func savePlexPinSession(sess *plexPinSessionJSON) error {
	return saveJSONSession(plexPinSessionPath(), sess)
}

func loadPlexPinSession() (*plexPinSessionJSON, error) {
	sess, err := loadJSONSession[plexPinSessionJSON](plexPinSessionPath())
	if err != nil {
		return nil, missingSessionErr(err, "Plex 配对会话不存在，请先发起配对")
	}
	return sess, nil
}
