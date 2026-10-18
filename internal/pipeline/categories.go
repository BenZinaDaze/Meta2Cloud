// categories.go —— 管线侧媒体分类目录解析（媒体库根/一级/二级 目录链）。
package pipeline

import (
	"fmt"

	"meta2cloud/internal/cloudfile"
	"meta2cloud/internal/jsonx"
)

// TmdbGenreIDs 提取 TMDB 详情的流派 ID 列表（16 = 动画，与语言设置无关）。
func TmdbGenreIDs(info map[string]any) []int {
	genres, _ := info["genres"].([]any)
	ids := make([]int, 0, len(genres))
	for _, g := range genres {
		gm, ok := g.(map[string]any)
		if !ok {
			continue
		}
		if id := jsonx.Int(gm, "id", 0); id != 0 {
			ids = append(ids, int(id))
		}
	}
	return ids
}

// ensureCategoryChain 创建并解析分类目录链（媒体库根/一级/二级）。
// 返回链条节点（含 ID 与名字，供 STRM 钩子免网络计算相对路径）与二级目录 ID。
// 配置值为媒体库根目录的相对路径（兼容旧名字/ID），在此解析为目录 ID。
func (p *Pipeline) ensureCategoryChain(level1, level2 string) ([]cloudfile.CloudFile, string, error) {
	rootName := p.cfg.ActiveRootFolderID()
	rootID, ok := cloudfile.ResolveFolderRefOrPath(p.provider, rootName)
	if !ok {
		return nil, "", fmt.Errorf("媒体库根目录 %q 在网盘根目录下不存在", rootName)
	}
	nodes := []cloudfile.CloudFile{{ID: rootID, Name: rootName, FileType: cloudfile.TypeFolder}}
	l1ID, err := p.categoryDir(rootID, level1)
	if err != nil {
		return nil, "", err
	}
	nodes = append(nodes, cloudfile.CloudFile{ID: l1ID, Name: level1, FileType: cloudfile.TypeFolder})
	leafID := l1ID
	if level2 != "" && level2 != level1 {
		l2ID, err := p.categoryDir(l1ID, level2)
		if err != nil {
			return nil, "", err
		}
		nodes = append(nodes, cloudfile.CloudFile{ID: l2ID, Name: level2, FileType: cloudfile.TypeFolder})
		leafID = l2ID
	}
	return nodes, leafID, nil
}

// categoryDir 按名取父目录下的分类目录（幂等创建，带 per-run 缓存）。
func (p *Pipeline) categoryDir(parentID, name string) (string, error) {
	key := parentID + "/" + name
	if id, ok := p.categoryDirCache[key]; ok {
		return id, nil
	}
	dir, err := cloudfile.GetOrCreateFolder(p.provider, name, parentID)
	if err != nil {
		return "", err
	}
	p.categoryDirCache[key] = dir.ID
	return dir.ID, nil
}
