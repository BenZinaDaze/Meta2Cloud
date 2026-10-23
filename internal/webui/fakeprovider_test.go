package webui

import (
	"fmt"
	"strconv"
	"sync"

	"meta2cloud/internal/cloudfile"
)

// fakeWebProvider 内存版网盘（webui 测试用）。
type fakeWebProvider struct {
	mu            sync.Mutex
	children      map[string][]cloudfile.CloudFile
	byID          map[string]cloudfile.CloudFile
	nextID        int
	failDeleteIDs map[string]bool // 置位的条目 DeleteFile 报错（模拟网盘删除失败）
}

func newFakeWebProvider() *fakeWebProvider {
	return &fakeWebProvider{
		children: map[string][]cloudfile.CloudFile{},
		byID:     map[string]cloudfile.CloudFile{},
	}
}

func (f *fakeWebProvider) id() string {
	f.nextID++
	return fmt.Sprintf("w%d", f.nextID)
}

func (f *fakeWebProvider) addFolder(name, parentID string) cloudfile.CloudFile {
	f.mu.Lock()
	defer f.mu.Unlock()
	cf := cloudfile.CloudFile{ID: f.id(), Name: name, FileType: cloudfile.TypeFolder,
		ParentID: parentID, Parents: []string{parentID}}
	f.children[parentID] = append(f.children[parentID], cf)
	f.byID[cf.ID] = cf
	return cf
}

func (f *fakeWebProvider) addFile(name, parentID string) cloudfile.CloudFile {
	f.mu.Lock()
	defer f.mu.Unlock()
	cf := cloudfile.CloudFile{ID: f.id(), Name: name, FileType: cloudfile.TypeFile,
		ParentID: parentID, Parents: []string{parentID}}
	f.children[parentID] = append(f.children[parentID], cf)
	f.byID[cf.ID] = cf
	return cf
}

func (f *fakeWebProvider) provider() cloudfile.Provider { return f }

// setMtime 更新条目的修改时间（byID 与 children 各存一份值拷贝，需同步）。
func (f *fakeWebProvider) setMtime(itemID string, unix int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cf := f.byID[itemID]
	cf.ModifiedTime = strconv.FormatInt(unix, 10)
	f.byID[itemID] = cf
	for i := range f.children[cf.ParentID] {
		if f.children[cf.ParentID][i].ID == itemID {
			f.children[cf.ParentID][i] = cf
		}
	}
}

func (f *fakeWebProvider) ProviderName() string { return "fake-web" }

func (f *fakeWebProvider) ListFiles(folderID string, pageSize int) ([]cloudfile.CloudFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// 已删除/不存在的目录报「未找到」（对齐真实网盘语义），不再返回孤儿子项
	if folderID != "0" {
		if _, ok := f.byID[folderID]; !ok {
			return nil, fmt.Errorf("未找到目录：%s: %w", folderID, cloudfile.ErrNotFound)
		}
	}
	out := []cloudfile.CloudFile{}
	return append(out, f.children[folderID]...), nil
}

func (f *fakeWebProvider) ListAllFiles(folderID string) ([]cloudfile.CloudFile, error) {
	return f.ListFiles(folderID, 1000)
}

func (f *fakeWebProvider) GetFile(fileID string) (cloudfile.CloudFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if cf, ok := f.byID[fileID]; ok {
		return cf, nil
	}
	return cloudfile.CloudFile{}, fmt.Errorf("未找到：%s: %w", fileID, cloudfile.ErrNotFound)
}

func (f *fakeWebProvider) ReadText(cloudfile.CloudFile) (string, error) { return "", nil }

func (f *fakeWebProvider) FindFile(name, folderID string) (*cloudfile.CloudFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, cf := range f.children[folderID] {
		if cf.Name == name {
			out := cf
			return &out, nil
		}
	}
	return nil, nil
}

func (f *fakeWebProvider) RenameFile(fileID, newName string) (cloudfile.CloudFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cf, ok := f.byID[fileID]
	if !ok {
		return cloudfile.CloudFile{}, fmt.Errorf("未找到：%s", fileID)
	}
	cf.Name = newName
	f.byID[fileID] = cf
	for i := range f.children[cf.ParentID] {
		if f.children[cf.ParentID][i].ID == fileID {
			f.children[cf.ParentID][i] = cf
		}
	}
	return cf, nil
}

func (f *fakeWebProvider) MoveFile(fileID, newFolderID, newName string) (cloudfile.CloudFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cf, ok := f.byID[fileID]
	if !ok {
		return cloudfile.CloudFile{}, fmt.Errorf("未找到：%s", fileID)
	}
	old := f.children[cf.ParentID]
	for i, c := range old {
		if c.ID == fileID {
			f.children[cf.ParentID] = append(old[:i], old[i+1:]...)
			break
		}
	}
	cf.ParentID = newFolderID
	cf.Parents = []string{newFolderID}
	if newName != "" {
		cf.Name = newName
	}
	f.byID[fileID] = cf
	f.children[newFolderID] = append(f.children[newFolderID], cf)
	return cf, nil
}

func (f *fakeWebProvider) UploadText(content, name, parentID, _ string, _ bool) (cloudfile.CloudFile, error) {
	return cloudfile.CloudFile{ID: f.id(), Name: name}, nil
}

func (f *fakeWebProvider) UploadBytes(content []byte, name, parentID, mime string, overwrite bool) (cloudfile.CloudFile, error) {
	return f.UploadText(string(content), name, parentID, mime, overwrite)
}

func (f *fakeWebProvider) TrashFile(fileID string) (cloudfile.CloudFile, error) {
	return f.GetFile(fileID)
}

// failDelete 让指定条目的 DeleteFile 报错（模拟网盘删除失败）。
func (f *fakeWebProvider) failDelete(fileID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failDeleteIDs == nil {
		f.failDeleteIDs = map[string]bool{}
	}
	f.failDeleteIDs[fileID] = true
}

// DeleteFile 删除条目：从父目录 children 与 byID 移除。
// 已删条目再列举/反查即报「未找到」（ErrNotFound），对齐真实网盘语义；
// 子项 map 残留不可达（其所在目录已无法列举），仅供直接 map 操作的测试使用。
func (f *fakeWebProvider) DeleteFile(fileID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cf, ok := f.byID[fileID]
	if !ok {
		return fmt.Errorf("未找到：%s: %w", fileID, cloudfile.ErrNotFound)
	}
	if f.failDeleteIDs[fileID] {
		return fmt.Errorf("模拟删除失败：%s", fileID)
	}
	old := f.children[cf.ParentID]
	for i, c := range old {
		if c.ID == fileID {
			f.children[cf.ParentID] = append(old[:i], old[i+1:]...)
			break
		}
	}
	delete(f.byID, fileID)
	return nil
}

func (f *fakeWebProvider) CreateFolder(name, parentID string) (cloudfile.CloudFile, error) {
	return f.addFolder(name, parentID), nil
}
