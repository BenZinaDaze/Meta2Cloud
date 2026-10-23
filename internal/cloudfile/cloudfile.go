// Package cloudfile 定义网盘存储抽象层：
// CloudFile 统一数据模型与 Provider 接口。
// 所有网盘 Provider 实现此接口，Pipeline / Organizer / WebUI 只依赖此接口。
package cloudfile

import (
	"errors"
	"path"
	"strconv"
	"strings"
)

// ErrNotFound 目标条目在网盘不存在。
// Provider 的 GetFile / DeleteFile 在「确认未找到」时返回包装此哨兵的错误，
// 与网络失败、限流等瞬时错误相区分——调用方（如联动删除的幂等探测）只有
// 在 errors.Is(err, ErrNotFound) 成立时才能把「探测失败」当作「条目已不存在」。
var ErrNotFound = errors.New("cloudfile: 条目不存在")

// FileType 通用文件类型。
type FileType int

const (
	TypeFile FileType = iota
	TypeFolder
)

// 常见视频扩展名（跨网盘通用判断）。
var videoExtensions = map[string]struct{}{
	".mp4": {}, ".mkv": {}, ".avi": {}, ".mov": {}, ".wmv": {}, ".flv": {},
	".ts": {}, ".m2ts": {}, ".webm": {}, ".rmvb": {}, ".rm": {}, ".mpg": {},
	".mpeg": {}, ".vob": {}, ".iso": {}, ".3gp": {},
}

// 字幕文件扩展名。
var subtitleExtensions = map[string]struct{}{
	".srt": {}, ".ass": {}, ".ssa": {},
}

// CloudFile 统一的云文件/文件夹描述。
// 所有网盘 Provider 返回此类型，屏蔽各平台字段差异。
//
// ID / Name 等字段含义与 Python 版一致；
// Size 为指针类型，nil 表示未知（如文件夹）。
type CloudFile struct {
	ID           string
	Name         string
	FileType     FileType
	Size         *int64
	ModifiedTime string
	ParentID     string
	Parents      []string
	MimeType     string
	Trashed      bool
	Extra        map[string]any
}

// IsFolder 是否为文件夹。
func (f CloudFile) IsFolder() bool { return f.FileType == TypeFolder }

// IsVideo 按 MIME 或扩展名判断是否为视频（跨平台通用）。
func (f CloudFile) IsVideo() bool {
	if f.IsFolder() {
		return false
	}
	if f.MimeType != "" && strings.HasPrefix(f.MimeType, "video/") {
		return true
	}
	_, ok := videoExtensions[f.Extension()]
	return ok
}

// IsSubtitle 判断是否为字幕文件。
func (f CloudFile) IsSubtitle() bool {
	if f.IsFolder() {
		return false
	}
	_, ok := subtitleExtensions[f.Extension()]
	return ok
}

// Extension 返回小写扩展名（含点），无扩展名返回空串。
func (f CloudFile) Extension() string {
	ext := path.Ext(f.Name)
	return strings.ToLower(ext)
}

// String 实现 fmt.Stringer，与 Python 版 __repr__ 风格一致。
func (f CloudFile) String() string {
	var tag string
	switch {
	case f.IsFolder():
		tag = "📁"
	case f.IsVideo():
		tag = "🎬"
	default:
		tag = "📄"
	}
	id := f.ID
	if len(id) > 12 {
		id = id[:12]
	}
	sizeStr := ""
	if f.Size != nil && *f.Size > 0 {
		sizeStr = " " + strconv.FormatInt(*f.Size/1024/1024, 10) + "MB"
	}
	return tag + " '" + f.Name + "' [" + id + "...]" + sizeStr
}

// Provider 网盘存储 Provider 抽象接口。
//
// 命名约定（与 Python 版一致）：
//   - 参数统一使用 folderID（而非 115 的 cid/pid）
//   - 返回值统一使用 CloudFile
//   - FindFile 未找到时返回 (nil, nil)
type Provider interface {
	// ProviderName 返回 Provider 名称，如 "pan115"。
	ProviderName() string

	// ListFiles 列举文件夹直接子项（不递归，仅一页）。
	ListFiles(folderID string, pageSize int) ([]CloudFile, error)

	// ListAllFiles 自动翻页列举文件夹全部子项（不递归）。
	// 递归遍历（WalkAll）依赖此方法避免单页截断。
	ListAllFiles(folderID string) ([]CloudFile, error)

	// GetFile 获取单个文件的元数据。
	GetFile(fileID string) (CloudFile, error)

	// ReadText 读取文本文件内容。
	ReadText(file CloudFile) (string, error)

	// FindFile 按精确文件名查找（返回第一个匹配；nil, nil 表示未找到）。
	// folderID 传空串表示各平台默认根目录。
	FindFile(name string, folderID string) (*CloudFile, error)

	// RenameFile 重命名文件或文件夹。
	RenameFile(fileID string, newName string) (CloudFile, error)

	// MoveFile 移动文件到另一个文件夹，可同时重命名（newName 传空串表示不改名）。
	MoveFile(fileID string, newFolderID string, newName string) (CloudFile, error)

	// UploadText 上传文本内容为文件（适合字幕等小文件）。
	UploadText(content string, name string, parentID string, mimeType string, overwrite bool) (CloudFile, error)

	// UploadBytes 上传二进制内容为文件（适合图片等非文本文件）。
	UploadBytes(content []byte, name string, parentID string, mimeType string, overwrite bool) (CloudFile, error)

	// TrashFile 移动到回收站（可恢复）。
	TrashFile(fileID string) (CloudFile, error)

	// DeleteFile 彻底删除文件（不可恢复）。
	DeleteFile(fileID string) error

	// CreateFolder 创建文件夹。
	CreateFolder(name string, parentID string) (CloudFile, error)
}

// GetOrCreateFolder 如果文件夹不存在则创建，否则返回已有的。
func GetOrCreateFolder(p Provider, name string, parentID string) (CloudFile, error) {
	existing, err := p.FindFile(name, parentID)
	if err != nil {
		return CloudFile{}, err
	}
	if existing != nil && existing.IsFolder() {
		return *existing, nil
	}
	return p.CreateFolder(name, parentID)
}

// ListMediaFiles 列举文件夹内所有视频文件（默认实现语义：列举后过滤）。
func ListMediaFiles(p Provider, folderID string) ([]CloudFile, error) {
	files, err := p.ListFiles(folderID, 1000)
	if err != nil {
		return nil, err
	}
	var out []CloudFile
	for _, f := range files {
		if f.IsVideo() {
			out = append(out, f)
		}
	}
	return out, nil
}

// Exists 检查指定名称的文件是否存在。
func Exists(p Provider, name string, folderID string) (bool, error) {
	found, err := p.FindFile(name, folderID)
	if err != nil {
		return false, err
	}
	return found != nil, nil
}

// WalkAll 递归遍历文件夹，对每个条目调用 fn（含目录本身）。
// 目录项先回调自身，再递归进入子目录（与 Python 版 yield 顺序一致）。
// 返回 fn 的第一个错误即停止。maxDepth 限制递归深度。
//
// 注意：对于超大目录可能产生大量请求，请谨慎使用。
func WalkAll(p Provider, folderID string, maxDepth int, fn func(CloudFile) error) error {
	return walkAll(p, folderID, maxDepth, 0, fn)
}

func walkAll(p Provider, folderID string, maxDepth, depth int, fn func(CloudFile) error) error {
	if depth > maxDepth {
		return nil
	}
	items, err := p.ListAllFiles(folderID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := fn(item); err != nil {
			return err
		}
		if item.IsFolder() {
			if err := walkAll(p, item.ID, maxDepth, depth+1, fn); err != nil {
				return err
			}
		}
	}
	return nil
}
