// folderresolve.go —— 配置目录值（名字）→ 网盘目录 ID 的统一解析。
package webui

import (
	"strings"

	"panoply/internal/cloudfile"
)

// resolveFolderID 将配置中的目录值（网盘根起算的相对路径，兼容旧名字/ID）解析为网盘目录 ID。
// 解析失败时原样返回（兼容直接填 ID 的旧配置）。
func (rt *Runtime) resolveFolderID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if p := rt.GetStorageProvider(); p != nil {
		if id, ok := cloudfile.ResolveFolderRefOrPath(p, value); ok {
			return id
		}
	}
	return value
}
