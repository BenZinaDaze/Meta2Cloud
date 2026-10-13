// Package storage 网盘存储 Provider 层（对应 Python 版 storage/ 包）。
//
// Google Drive 支持已在 Go 重构中移除，当前仅提供 115 网盘。
package storage

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"meta2cloud/internal/cloudfile"
	"meta2cloud/internal/u115"
)

// Pan115Provider 115 网盘存储 Provider。
//
// 包装 u115.Client，将 u115.File 映射为统一的 cloudfile.CloudFile。
type Pan115Provider struct {
	clientGetter func() *u115.Client

	// LookupRoots 返回 GetFile 按 ID 回退扫描的受管根目录。
	// 由应用层注入（配置中的媒体库根目录等），为空时回退到 "0"。
	LookupRoots func() []string
}

// 静态检查：确保实现 Provider 接口。
var _ cloudfile.Provider = (*Pan115Provider)(nil)

// NewPan115ProviderWithClient 使用固定 client 构造。
func NewPan115ProviderWithClient(client *u115.Client) *Pan115Provider {
	return &Pan115Provider{clientGetter: func() *u115.Client { return client }}
}

// NewPan115ProviderWithGetter 使用共享 client getter 构造，适合 WebUI 等长生命周期场景。
func NewPan115ProviderWithGetter(getter func() *u115.Client) *Pan115Provider {
	return &Pan115Provider{clientGetter: getter}
}

// ProviderName 实现 cloudfile.Provider。
func (p *Pan115Provider) ProviderName() string { return "pan115" }

// RawClient 暴露底层 u115.Client，供需要平台特有功能的场景使用。
func (p *Pan115Provider) RawClient() *u115.Client { return p.clientGetter() }

func (p *Pan115Provider) client() *u115.Client { return p.clientGetter() }

// normalizeFolderID 兼容 "root" → "0" 映射与空串默认根目录。
func normalizeFolderID(folderID string) string {
	if folderID == "" || folderID == "root" {
		return "0"
	}
	return folderID
}

// ── 类型转换 ────────────────────────────────────────────────

// ToCloudFile 将 u115.File 转换为统一的 CloudFile。
func ToCloudFile(pf u115.File) cloudfile.CloudFile {
	// Unix 时间戳转 ISO8601（与 Python 版一致使用 UTC）
	modifiedTime := ""
	if pf.ModifiedTime != nil {
		modifiedTime = time.Unix(*pf.ModifiedTime, 0).UTC().Format("2006-01-02T15:04:05-07:00")
	}
	extra := map[string]any{}
	if pf.PickCode != nil {
		extra["pick_code"] = *pf.PickCode
	}
	if pf.SHA1 != nil {
		extra["sha1"] = *pf.SHA1
	}
	if pf.Ico != nil {
		extra["ico"] = *pf.Ico
	}
	fileType := cloudfile.TypeFile
	if pf.IsFolder() {
		fileType = cloudfile.TypeFolder
	}
	parents := []string{}
	if pf.ParentID != nil && *pf.ParentID != "" {
		parents = append(parents, *pf.ParentID)
	}
	return cloudfile.CloudFile{
		ID:           pf.ID,
		Name:         pf.Name,
		FileType:     fileType,
		Size:         pf.Size,
		ModifiedTime: modifiedTime,
		ParentID:     derefString(pf.ParentID),
		Parents:      parents,
		Extra:        extra,
	}
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ── 列举 ────────────────────────────────────────────────────

// ListFiles 列举文件夹直接子项（不递归）。
func (p *Pan115Provider) ListFiles(folderID string, pageSize int) ([]cloudfile.CloudFile, error) {
	if pageSize <= 0 {
		pageSize = 1000
	}
	raw, err := p.client().ListFiles(normalizeFolderID(folderID), pageSize, 0)
	if err != nil {
		return nil, err
	}
	out := make([]cloudfile.CloudFile, 0, len(raw))
	for _, f := range raw {
		out = append(out, ToCloudFile(f))
	}
	return out, nil
}

// ListAllFiles 自动翻页列举文件夹全部子项（不递归），
// 递归遍历（cloudfile.WalkAll）依赖此方法避免单页 1000 条截断。
func (p *Pan115Provider) ListAllFiles(folderID string) ([]cloudfile.CloudFile, error) {
	raw, err := p.client().ListAllFiles(normalizeFolderID(folderID), 1000)
	if err != nil {
		return nil, err
	}
	out := make([]cloudfile.CloudFile, 0, len(raw))
	for _, f := range raw {
		out = append(out, ToCloudFile(f))
	}
	return out, nil
}

// GetFile 获取单个文件的元数据。
// 115 开放接口没有稳定的 get_by_id API，底层 client 在受管根目录下
// 做按 ID 查询与回退扫描。
func (p *Pan115Provider) GetFile(fileID string) (cloudfile.CloudFile, error) {
	found, ok, err := p.client().GetFileByID(fileID, p.managedLookupRoots(), 12)
	if err != nil {
		return cloudfile.CloudFile{}, err
	}
	if !ok {
		return cloudfile.CloudFile{}, u115.NewAPIError("未找到文件：" + fileID)
	}
	return ToCloudFile(found), nil
}

func (p *Pan115Provider) managedLookupRoots() []string {
	roots := []string{}
	if p.LookupRoots != nil {
		for _, candidate := range p.LookupRoots() {
			v := strings.TrimSpace(candidate)
			if v != "" && !containsStr(roots, v) {
				roots = append(roots, v)
			}
		}
	}
	if !containsStr(roots, "0") {
		roots = append(roots, "0")
	}
	return roots
}

func containsStr(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

// ReadText 读取文本文件内容。
func (p *Pan115Provider) ReadText(file cloudfile.CloudFile) (string, error) {
	pickCode, _ := file.Extra["pick_code"].(string)
	if pickCode == "" {
		return "", errors.New("文件缺少 pick_code，无法读取")
	}
	client := p.client()
	downloadURL, err := client.GetDownloadURL(pickCode)
	if err != nil {
		return "", err
	}
	resp, err := client.GetHTTPClient().Get(downloadURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("读取文件失败：HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// FindFile 按精确文件名查找。folderID 为空表示根目录。
func (p *Pan115Provider) FindFile(name string, folderID string) (*cloudfile.CloudFile, error) {
	cid := normalizeFolderID(folderID)
	results, err := p.client().Search(name, 10, 0, cid, "", "")
	if err != nil {
		return nil, err
	}
	for _, r := range results {
		if r.Name == name {
			cf := ToCloudFile(r)
			return &cf, nil
		}
	}
	// 115 搜索结果存在延迟，回退到直接列目录做精确匹配
	if items, err := p.ListFiles(cid, 1000); err == nil {
		for _, item := range items {
			if item.Name == name {
				cf := item
				return &cf, nil
			}
		}
	}
	return nil, nil
}

// ── 修改 ────────────────────────────────────────────────────

// RenameFile 重命名文件或文件夹。
func (p *Pan115Provider) RenameFile(fileID string, newName string) (cloudfile.CloudFile, error) {
	client := p.client()
	if err := client.Rename(fileID, newName); err != nil {
		return cloudfile.CloudFile{}, err
	}
	results, err := client.Search(newName, 20, 0, "", "", "")
	if err == nil {
		for _, r := range results {
			if r.ID == fileID {
				return ToCloudFile(r), nil
			}
		}
	}
	return cloudfile.CloudFile{ID: fileID, Name: newName, FileType: cloudfile.TypeFile}, nil
}

// MoveFile 移动文件到另一个文件夹，newName 非空时同时重命名。
func (p *Pan115Provider) MoveFile(fileID string, newFolderID string, newName string) (cloudfile.CloudFile, error) {
	client := p.client()
	if err := client.Move(fileID, normalizeFolderID(newFolderID)); err != nil {
		return cloudfile.CloudFile{}, err
	}
	if newName != "" {
		if err := client.Rename(fileID, newName); err != nil {
			return cloudfile.CloudFile{}, err
		}
		if moved, err := p.FindFile(newName, newFolderID); err == nil && moved != nil {
			return *moved, nil
		}
	}
	if items, err := p.ListFiles(newFolderID, 1000); err == nil {
		for _, item := range items {
			if item.ID == fileID {
				return item, nil
			}
		}
	}
	name := newName
	if name == "" {
		name = fileID
	}
	cid := normalizeFolderID(newFolderID)
	return cloudfile.CloudFile{
		ID:       fileID,
		Name:     name,
		FileType: cloudfile.TypeFile,
		ParentID: cid,
		Parents:  []string{cid},
	}, nil
}

// ── 上传 ────────────────────────────────────────────────────

// UploadText 上传文本内容为文件（适合 NFO 等小文件）。
func (p *Pan115Provider) UploadText(content string, name string, parentID string, _ string, overwrite bool) (cloudfile.CloudFile, error) {
	cid := normalizeFolderID(parentID)
	if overwrite {
		if err := p.deleteExisting(name, cid); err != nil {
			return cloudfile.CloudFile{}, err
		}
	}
	return p.uploadData([]byte(content), name, cid)
}

// UploadBytes 上传二进制内容为文件（适合图片等非文本文件）。
func (p *Pan115Provider) UploadBytes(content []byte, name string, parentID string, _ string, overwrite bool) (cloudfile.CloudFile, error) {
	cid := normalizeFolderID(parentID)
	if overwrite {
		if err := p.deleteExisting(name, cid); err != nil {
			return cloudfile.CloudFile{}, err
		}
	}
	return p.uploadData(content, name, cid)
}

func (p *Pan115Provider) deleteExisting(name, cid string) error {
	existing, err := p.FindFile(name, cid)
	if err != nil {
		return err
	}
	if existing != nil {
		if err := p.client().Delete(existing.ID); err != nil {
			// 删除已有文件失败不阻断上传（与 Python 版一致）
			_ = err
		}
	}
	return nil
}

// uploadData 秒传 → 二次认证 → OSS 分片上传。
func (p *Pan115Provider) uploadData(data []byte, name, cid string) (cloudfile.CloudFile, error) {
	client := p.client()
	hashes := u115.ComputeUploadHashesBytes(data)

	initInfo, err := client.InitUpload(u115.InitUploadArgs{
		FileName: name,
		FileSize: hashes.FileSize,
		Cid:      cid,
		FileID:   hashes.FileID,
		PreID:    hashes.PreID,
	})
	if err != nil {
		return cloudfile.CloudFile{}, err
	}

	if initInfo.IsFastUpload() && initInfo.FileID != nil {
		return fastUploadResult(name, cid, hashes.FileSize, *initInfo.FileID), nil
	}

	if initInfo.RequiresSecondCheck() {
		signVal, err := u115.ComputeSignValBytes(data, *initInfo.SignCheck)
		if err != nil {
			return cloudfile.CloudFile{}, err
		}
		initInfo, err = client.InitUpload(u115.InitUploadArgs{
			FileName: name,
			FileSize: hashes.FileSize,
			Cid:      cid,
			FileID:   hashes.FileID,
			PreID:    hashes.PreID,
			PickCode: derefString(initInfo.PickCode),
			SignKey:  derefString(initInfo.SignKey),
			SignVal:  signVal,
		})
		if err != nil {
			return cloudfile.CloudFile{}, err
		}
		if initInfo.IsFastUpload() && initInfo.FileID != nil {
			return fastUploadResult(name, cid, hashes.FileSize, *initInfo.FileID), nil
		}
	}

	if initInfo.Bucket == nil || initInfo.Object == nil || initInfo.PickCode == nil {
		return cloudfile.CloudFile{}, u115.NewAPIError("115 上传初始化信息不完整：" + name)
	}

	callback := initInfo.Callback
	if resume, err := client.ResumeUpload(hashes.FileSize, cid, hashes.FileID, *initInfo.PickCode); err == nil && resume.Callback != nil {
		callback = resume.Callback
	}

	if err := p.multipartUpload(data, cid, hashes.FileID, *initInfo.Bucket, *initInfo.Object, *initInfo.PickCode, callback); err != nil {
		return cloudfile.CloudFile{}, err
	}

	if uploaded, err := p.FindFile(name, cid); err == nil && uploaded != nil {
		return *uploaded, nil
	}

	return cloudfile.CloudFile{
		ID:       firstNonEmptyStr(derefString(initInfo.FileID), *initInfo.PickCode),
		Name:     name,
		FileType: cloudfile.TypeFile,
		Size:     &hashes.FileSize,
		ParentID: cid,
		Parents:  []string{cid},
		Extra:    map[string]any{"pick_code": *initInfo.PickCode},
	}, nil
}

func fastUploadResult(name, cid string, size int64, fileID string) cloudfile.CloudFile {
	return cloudfile.CloudFile{
		ID:       fileID,
		Name:     name,
		FileType: cloudfile.TypeFile,
		Size:     &size,
		ParentID: cid,
		Parents:  []string{cid},
	}
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// multipartUpload OSS 分片上传（数据已在内存，直接分片发送）。
func (p *Pan115Provider) multipartUpload(data []byte, cid, fileID, bucketName, objectName, pickCode string, callback map[string]any) error {
	client := p.client()
	token, err := client.GetUploadToken()
	if err != nil {
		return err
	}

	oss := &ossClient{
		endpoint: token.Endpoint,
		bucket:   bucketName,
		ak:       token.AccessKeyID,
		sk:       token.AccessKeySecret,
		token:    token.SecurityToken,
		hc:       client.GetHTTPClient(),
	}

	uploadID, err := oss.initiateMultipartUpload(objectName)
	if err != nil {
		return fmt.Errorf("115 OSS 分片上传失败：%w", err)
	}

	partSize := determinePartSize(int64(len(data)))
	var parts []ossPart
	partNumber := 1
	for offset := 0; offset < len(data); offset += partSize {
		end := offset + partSize
		if end > len(data) {
			end = len(data)
		}
		etag, err := oss.uploadPart(objectName, uploadID, partNumber, data[offset:end])
		if err != nil {
			return fmt.Errorf("115 OSS 分片上传失败：%w", err)
		}
		parts = append(parts, ossPart{Number: partNumber, ETag: etag})
		partNumber++
	}

	callbackBody, _ := callback["callback"].(string)
	callbackVar, _ := callback["callback_var"].(string)
	if err := oss.completeMultipartUpload(objectName, uploadID, parts, callbackBody, callbackVar); err != nil {
		return fmt.Errorf("115 OSS 分片上传失败：%w", err)
	}
	return nil
}

// determinePartSize 与 Python oss2 determine_part_size(preferred=10MB) 对齐：
// 保证不超过 10000 片，向上取整到 1KB 对齐。
func determinePartSize(fileSize int64) int {
	const maxParts = 10000
	partSize := int64(ossPartPreferredSize)
	if fileSize > partSize*maxParts {
		partSize = (fileSize + maxParts - 1) / maxParts
		partSize = (partSize + 1023) / 1024 * 1024
	}
	return int(partSize)
}

// ── 删除 ────────────────────────────────────────────────────

// TrashFile 115 没有回收站移入 API（只有列出和清空），此处用删除代替。
func (p *Pan115Provider) TrashFile(fileID string) (cloudfile.CloudFile, error) {
	if err := p.client().Delete(fileID); err != nil {
		return cloudfile.CloudFile{}, err
	}
	return cloudfile.CloudFile{ID: fileID, Name: fileID, FileType: cloudfile.TypeFile, Trashed: true}, nil
}

// DeleteFile 彻底删除文件。
func (p *Pan115Provider) DeleteFile(fileID string) error {
	return p.client().Delete(fileID)
}

// ── 文件夹 ──────────────────────────────────────────────────

// CreateFolder 创建文件夹。
func (p *Pan115Provider) CreateFolder(name string, parentID string) (cloudfile.CloudFile, error) {
	pid := normalizeFolderID(parentID)
	newID, err := p.client().CreateFolder(pid, name)
	if err != nil {
		return cloudfile.CloudFile{}, err
	}
	return cloudfile.CloudFile{
		ID:       newID,
		Name:     name,
		FileType: cloudfile.TypeFolder,
		ParentID: pid,
		Parents:  []string{pid},
	}, nil
}

// GetOrCreateFolder 先查找，不存在则创建（115 的 create_folder 本身支持幂等）。
func (p *Pan115Provider) GetOrCreateFolder(name string, parentID string) (cloudfile.CloudFile, error) {
	pid := normalizeFolderID(parentID)
	// 115 的 create_folder 已经是幂等的（已存在返回已有 ID）
	newID, err := p.client().CreateFolder(pid, name)
	if err != nil {
		return cloudfile.CloudFile{}, err
	}
	if newID != "" {
		return cloudfile.CloudFile{
			ID:       newID,
			Name:     name,
			FileType: cloudfile.TypeFolder,
			ParentID: pid,
			Parents:  []string{pid},
		}, nil
	}
	// 回退：手动查找
	existing, err := p.FindFile(name, pid)
	if err != nil {
		return cloudfile.CloudFile{}, err
	}
	if existing != nil && existing.IsFolder() {
		return *existing, nil
	}
	return cloudfile.CloudFile{}, u115.NewAPIError("创建文件夹失败：" + name)
}

// ── 分享转存（平台特有，透传底层 client） ──────────────────

// SetCookie 设置 Cookie 用于 Web API 认证。
func (p *Pan115Provider) SetCookie(cookie string) { p.client().SetCookie(cookie) }

// ParseShareURL 解析分享链接，返回 (shareCode, password)。
func ParseShareURL(shareURL string) (string, string) { return u115.ParseShareURL(shareURL) }

// GetShareInfo 获取分享链接信息。
func (p *Pan115Provider) GetShareInfo(shareCode, receiveCode, cid string) (u115.ShareInfo, []u115.ShareItem, error) {
	return p.client().GetShareInfo(shareCode, receiveCode, cid, 0, 100)
}

// ReceiveShare 转存分享文件到网盘。
func (p *Pan115Provider) ReceiveShare(shareCode, receiveCode string, fileIDs []string, targetFolderID string) (u115.ReceiveResult, error) {
	return p.client().ReceiveShare(shareCode, receiveCode, fileIDs, normalizeFolderID(targetFolderID))
}

// ReceiveShareAll 转存分享链接中的全部文件。
func (p *Pan115Provider) ReceiveShareAll(shareCode, receiveCode, targetFolderID, cid string) (u115.ReceiveResult, error) {
	return p.client().ReceiveShareAll(shareCode, receiveCode, normalizeFolderID(targetFolderID), cid)
}

// ReceiveShareByURL 通过分享链接 URL 转存全部文件。
func (p *Pan115Provider) ReceiveShareByURL(shareURL string, targetFolderID string) (u115.ReceiveResult, error) {
	shareCode, password := u115.ParseShareURL(shareURL)
	if shareCode == "" {
		return u115.ReceiveResult{Error: "无法解析分享链接"}, nil
	}
	return p.ReceiveShareAll(shareCode, password, targetFolderID, "")
}
