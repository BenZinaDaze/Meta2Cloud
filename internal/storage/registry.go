// registry.go —— Provider 注册与构造（对应 Python 版 storage/__init__.py）。
package storage

import (
	"fmt"
	"path/filepath"

	"panoply/internal/cloudfile"
	"panoply/internal/config"
	"panoply/internal/u115"
)

// ListProviders 列出内置 Provider 名称。
func ListProviders() []string {
	return []string{"pan115"}
}

// NewPan115Provider 从配置构造 115 Provider（读取 u115 配置段）。
func NewPan115Provider(cfg *config.Config) (*Pan115Provider, error) {
	u115cfg := cfg.U115
	tokenPath := u115cfg.TokenJSON
	if !filepath.IsAbs(tokenPath) {
		if abs, err := filepath.Abs(tokenPath); err == nil {
			tokenPath = abs
		}
	}
	client, err := u115.NewClientFromTokenFile(u115cfg.ClientID, tokenPath)
	if err != nil {
		return nil, err
	}
	if u115cfg.Cookie != "" {
		client.SetCookie(u115cfg.Cookie)
	}
	return NewPan115ProviderWithClient(client), nil
}

// GetProvider 按名称构造 Provider。未知名称返回错误。
func GetProvider(name string, cfg *config.Config) (cloudfile.Provider, error) {
	switch name {
	case "pan115":
		return NewPan115Provider(cfg)
	default:
		return nil, fmt.Errorf("未知的存储 Provider：%s", name)
	}
}
