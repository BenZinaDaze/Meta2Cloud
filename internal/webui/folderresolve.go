// folderresolve.go —— 配置目录值（名字）→ 网盘目录 ID 的统一解析。
package webui

import (
	"strings"

	"meta2cloud/internal/cloudfile"
)

// resolveFolderID 将配置中的目录值（网盘根起算的相对路径，兼容旧名字/ID）解析为网盘目录 ID。
// 解析失败时原样返回（兼容直接填 ID 的旧配置）。
func resolveFolderID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if p := RT().GetStorageProvider(); p != nil {
		if id, ok := cloudfile.ResolveFolderRefOrPath(p, value); ok {
			return id
		}
	}
	return value
}

// resolveFolderRefPathOnly 仅按「网盘根起算的相对路径/名字」解析目录引用，
// 实现为纯 ListFiles，不含 GetFile 回退。供存储 Provider 的根目录回调使用：
// 回退 GetFile 会在 Provider.GetFile → 根目录解析 → Provider.GetFile 之间成环，
// 名字解析失败（如接口冷却）时无限递归直至栈溢出。失败时原样返回（兼容旧 ID 配置）。
func resolveFolderRefPathOnly(p cloudfile.Provider, value string) string {
	value = strings.TrimSpace(value)
	if value == "" || p == nil {
		return value
	}
	if id, ok := cloudfile.ResolveFolderPath(p, value); ok {
		return id
	}
	return value
}
