// categories.go —— 管线侧媒体分类目录解析（媒体库根/一级/二级 目录链）。
package pipeline

import (
	"fmt"

	"meta2cloud/internal/cloudfile"
	"meta2cloud/internal/jsonx"
)

// tmdbGenreIDs 提取 TMDB 详情的流派 ID 列表（16 = 动画，与语言设置无关）。
func tmdbGenreIDs(info map[string]any) []int {
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

// ensureCategoryChain 创建并解析分类目录链（媒体库根/一级/二级），返回二级目录 ID。
func (p *Pipeline) ensureCategoryChain(level1, level2 string) (string, error) {
	rootID := p.cfg.ActiveRootFolderID()
	if rootID == "" {
		return "", fmt.Errorf("媒体库根目录未设置")
	}
	l1ID, err := p.categoryDir(rootID, level1)
	if err != nil {
		return "", err
	}
	if level2 == "" || level2 == level1 {
		return l1ID, nil
	}
	return p.categoryDir(l1ID, level2)
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
