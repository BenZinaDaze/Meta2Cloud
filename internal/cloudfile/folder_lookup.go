// folder_lookup.go —— 在云盘根目录下按目录名查找目录 ID。
//
// 背景：配置里目录以“名字”存储（用户在网盘上看到的即所见即所得），
// 后端使用时在此处解析为目录 ID。调用频率低（管线启动/轮询/离线推送各一次），
// 不做缓存，避免改名/重建目录后的脏读。
package cloudfile

// folderLookupRootID 云盘根目录（115 固定为 "0"）。
const folderLookupRootID = "0"

// FindFolderByName 在云盘根目录下按名字查找目录，返回其 ID；未找到返回 ok=false。
func FindFolderByName(p Provider, name string) (string, bool) {
	if name == "" {
		return "", false
	}
	folders, err := p.ListFiles(folderLookupRootID, 500)
	if err != nil {
		return "", false
	}
	for _, f := range folders {
		if f.IsFolder() && f.Name == name {
			return f.ID, true
		}
	}
	return "", false
}

// ResolveFolderRef 解析配置中的目录引用：优先按名字在网盘根下查找（新配置存名字），
// 找不到时回退按 ID 直查（兼容旧配置存 ID）。两者皆失败返回 ok=false。
func ResolveFolderRef(p Provider, ref string) (string, bool) {
	if ref == "" {
		return "", false
	}
	if id, ok := FindFolderByName(p, ref); ok {
		return id, true
	}
	if cf, err := p.GetFile(ref); err == nil && cf.IsFolder() {
		return cf.ID, true
	}
	return "", false
}
