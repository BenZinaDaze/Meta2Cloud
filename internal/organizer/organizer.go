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
// provider 为任意网盘 Provider；movieRootID / tvRootID 为空时使用 rootFolderID。
type MediaOrganizer struct {
	provider cloudfile.Provider
	rootID   string
}

// New 构造整理器（rootFolderID 为媒体库根目录，分类布局下的所有目录链挂在其下）。
func New(provider cloudfile.Provider, rootFolderID string) *MediaOrganizer {
	return &MediaOrganizer{
		provider: provider,
		rootID:   rootFolderID,
	}
}

// SetRootID 更新媒体库根目录 ID（配置以名字存储时，运行前解析后回填）。
func (o *MediaOrganizer) SetRootID(id string) { o.rootID = id }

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
// 解析失败时返回 nil。
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

// EnsureFolderUnder 在指定基准目录下按需创建目录链（电影一层；剧集含 Season），返回叶子目录。
// 供分类布局使用：baseID 为分类目录链（媒体库根/一级/二级）解析出的基准目录。
func (o *MediaOrganizer) EnsureFolderUnder(baseID string, meta *mediaparser.MetaBase, label string) *cloudfile.CloudFile {
	if baseID == "" {
		return nil
	}
	folder, _, err := o.ensureFoldersUnder(baseID, meta, label)
	if err != nil {
		log.Printf("[organizer] 创建文件夹失败 %q: %v", label, err)
		return nil
	}
	return folder
}

// EnsureFolderWithChainUnder 幂等创建目录链，返回「叶子目录 + 自基准之下到叶子的链条」。
// 链条节点（剧名/Season）含 ID 与实际目录名，供 STRM 钩子免网络计算相对路径。
func (o *MediaOrganizer) EnsureFolderWithChainUnder(baseID string, meta *mediaparser.MetaBase, label string) ([]cloudfile.CloudFile, *cloudfile.CloudFile) {
	if baseID == "" {
		return nil, nil
	}
	folder, chain, err := o.ensureFoldersUnder(baseID, meta, label)
	if err != nil {
		log.Printf("[organizer] 创建文件夹失败 %q: %v", label, err)
		return nil, nil
	}
	return chain, folder
}

// EnsureFolderWithChainForMeta 同 EnsureFolderForMeta，附带自根之下到叶子的链条。
func (o *MediaOrganizer) EnsureFolderWithChainForMeta(meta *mediaparser.MetaBase, label string) ([]cloudfile.CloudFile, *cloudfile.CloudFile) {
	folder, chain, err := o.ensureFoldersUnder(o.rootID, meta, label)
	if err != nil {
		log.Printf("[organizer] 创建文件夹失败 %q: %v", label, err)
		return nil, nil
	}
	return chain, folder
}

// ensureFolders 在云存储中幂等创建目录链，返回叶子文件夹。
func (o *MediaOrganizer) ensureFolders(meta *mediaparser.MetaBase, label string) (*cloudfile.CloudFile, error) {
	if meta == nil || meta.Name() == "" {
		return nil, fmt.Errorf("解析失败，跳过：%q", label)
	}

	// 基准目录：媒体库根（分类布局下由管线传入分类目录链）
	folder, _, err := o.ensureFoldersUnder(o.rootID, meta, label)
	return folder, err
}

// ensureFoldersUnder 在指定基准目录下幂等创建目录链。
// 返回叶子文件夹与「自基准之下到叶子」的链条（剧名/Season，含 ID 与实际目录名）。
func (o *MediaOrganizer) ensureFoldersUnder(baseID string, meta *mediaparser.MetaBase, label string) (*cloudfile.CloudFile, []cloudfile.CloudFile, error) {
	if meta == nil || meta.Name() == "" {
		return nil, nil, fmt.Errorf("解析失败，跳过：%q", label)
	}
	if baseID == "" {
		return nil, nil, fmt.Errorf("基准目录未设置")
	}

	isTV := meta.Type == mediaparser.MediaTV
	kind := "Movie"
	if isTV {
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

	// 第一层：片名/剧名 (年份)
	topFolder, err := cloudfile.GetOrCreateFolder(o.provider, topName, baseID)
	if err != nil {
		return nil, nil, err
	}
	chain := []cloudfile.CloudFile{{ID: topFolder.ID, Name: topFolder.Name, FileType: cloudfile.TypeFolder}}
	if !isTV {
		return &topFolder, chain, nil
	}

	// 第二层（仅剧集）：Season NN（两位零填充）
	seasonName := fmt.Sprintf("Season %02d", seasonInt)
	seasonFolder, err := cloudfile.GetOrCreateFolder(o.provider, seasonName, topFolder.ID)
	if err != nil {
		return nil, nil, err
	}
	chain = append(chain, cloudfile.CloudFile{ID: seasonFolder.ID, Name: seasonFolder.Name, FileType: cloudfile.TypeFolder})
	return &seasonFolder, chain, nil
}
