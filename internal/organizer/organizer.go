// Package organizer 媒体文件夹整理器（对应 Python 版 core/organizer.py）。
//
// 解析媒体文件名 → 生成标准目录路径 → 在云存储中按需创建（幂等）文件夹。
//
// 目录结构规范：
//
//	电影：{片名} ({年份})/
//	剧集：{剧名} ({年份})/Season {季号}/
package organizer

import (
	"fmt"
	"log"
	"strconv"

	"meta2cloud/internal/cloudfile"
	"meta2cloud/internal/mediaparser"
)

// MediaOrganizer 媒体文件夹整理器。
//
// provider 为任意网盘 Provider；movieRootID / tvRootID 为空时使用 rootFolderID；
// dryRun 为 true 时只计算路径，不实际创建文件夹。
type MediaOrganizer struct {
	provider    cloudfile.Provider
	rootID      string
	movieRootID string
	tvRootID    string
	dryRun      bool
}

// New 构造整理器。
func New(provider cloudfile.Provider, rootFolderID, movieRootID, tvRootID string, dryRun bool) *MediaOrganizer {
	if movieRootID == "" {
		movieRootID = rootFolderID
	}
	if tvRootID == "" {
		tvRootID = rootFolderID
	}
	return &MediaOrganizer{
		provider:    provider,
		rootID:      rootFolderID,
		movieRootID: movieRootID,
		tvRootID:    tvRootID,
		dryRun:      dryRun,
	}
}

// ── 公共接口 ────────────────────────────────────────────────

// FolderPathFor 根据文件名（或目录名）计算目标文件夹路径字符串，不操作网盘。
// 解析失败或名称为空时返回 ""。
func (o *MediaOrganizer) FolderPathFor(name string, isfile bool) string {
	meta := mediaparser.MetaInfo(name, "", isfile, nil, nil)
	return o.buildPath(meta)
}

// FolderPathForMeta 直接通过 MetaInfo 计算目标文件夹路径字符串。
func (o *MediaOrganizer) FolderPathForMeta(meta *mediaparser.MetaBase) string {
	return o.buildPath(meta)
}

// FolderPathForPath 根据文件路径（合并父目录信息）计算目标文件夹路径字符串。
func (o *MediaOrganizer) FolderPathForPath(path string) string {
	meta := mediaparser.MetaInfoPath(path, nil, nil)
	return o.buildPath(meta)
}

// EnsureFolderFor 根据文件名在云存储中按需创建文件夹（幂等），返回叶子文件夹。
// 解析失败或 dryRun 时返回 nil。
func (o *MediaOrganizer) EnsureFolderFor(name string, isfile bool) *cloudfile.CloudFile {
	meta := mediaparser.MetaInfo(name, "", isfile, nil, nil)
	folder, err := o.ensureFolders(meta, name)
	if err != nil {
		log.Printf("[organizer] 创建文件夹失败 %q: %v", name, err)
		return nil
	}
	return folder
}

// EnsureFolderForMeta 直接通过 MetaInfo 在云存储中按需创建文件夹（幂等）。
func (o *MediaOrganizer) EnsureFolderForMeta(meta *mediaparser.MetaBase, label string) *cloudfile.CloudFile {
	folder, err := o.ensureFolders(meta, label)
	if err != nil {
		log.Printf("[organizer] 创建文件夹失败 %q: %v", label, err)
		return nil
	}
	return folder
}

// EnsureFolderForPath 根据文件路径（合并父目录信息）在云存储中按需创建文件夹（幂等）。
func (o *MediaOrganizer) EnsureFolderForPath(path string) *cloudfile.CloudFile {
	meta := mediaparser.MetaInfoPath(path, nil, nil)
	folder, err := o.ensureFolders(meta, path)
	if err != nil {
		log.Printf("[organizer] 创建文件夹失败 %q: %v", path, err)
		return nil
	}
	return folder
}

// ── 内部实现 ────────────────────────────────────────────────

// buildPath 根据 MetaBase 拼装路径字符串（不含根目录）。
func (o *MediaOrganizer) buildPath(meta *mediaparser.MetaBase) string {
	if meta == nil || meta.Name() == "" {
		return ""
	}
	// 顶层目录：片名 (年份)
	top := meta.Name()
	if meta.Year != "" {
		top = fmt.Sprintf("%s (%s)", top, meta.Year)
	}
	if meta.Type == mediaparser.MediaTV {
		// Season NN（两位零填充，Emby/Plex/Infuse 三者标准）
		seasonNum := 1
		if s := meta.SeasonSeq(); s != "" {
			seasonNum, _ = strconv.Atoi(s)
		}
		return fmt.Sprintf("%s/Season %02d", top, seasonNum)
	}
	// 电影或未知类型，只有一层
	return top
}

// ensureFolders 在云存储中幂等创建目录链，返回叶子文件夹。
func (o *MediaOrganizer) ensureFolders(meta *mediaparser.MetaBase, label string) (*cloudfile.CloudFile, error) {
	if meta == nil || meta.Name() == "" {
		return nil, fmt.Errorf("解析失败，跳过：%q", label)
	}

	isTV := meta.Type == mediaparser.MediaTV

	// 选择基准目录
	baseID := o.movieRootID
	kind := "Movie"
	if isTV {
		baseID = o.tvRootID
		kind = "TV"
	}

	// 顶层目录名
	topName := meta.Name()
	if meta.Year != "" {
		topName = fmt.Sprintf("%s (%s)", topName, meta.Year)
	}

	seasonInt := 1
	if s := meta.SeasonSeq(); s != "" {
		seasonInt, _ = strconv.Atoi(s)
	}
	if isTV {
		log.Printf("[%s] %q → /%s/Season %02d", kind, label, topName, seasonInt)
	} else {
		log.Printf("[%s] %q → /%s", kind, label, topName)
	}

	if o.dryRun {
		log.Printf("dryRun=true，跳过云存储操作")
		return nil, nil
	}

	// 第一层：片名/剧名 (年份)
	topFolder, err := cloudfile.GetOrCreateFolder(o.provider, topName, baseID)
	if err != nil {
		return nil, err
	}
	if !isTV {
		return &topFolder, nil
	}

	// 第二层（仅剧集）：Season NN（两位零填充）
	seasonName := fmt.Sprintf("Season %02d", seasonInt)
	seasonFolder, err := cloudfile.GetOrCreateFolder(o.provider, seasonName, topFolder.ID)
	if err != nil {
		return nil, err
	}
	return &seasonFolder, nil
}
