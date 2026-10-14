package webui

import (
	"fmt"
	"strconv"
	"sync"

	"meta2cloud/internal/cloudfile"
)

// fakeWebProvider 内存版网盘（webui 测试用）。
type fakeWebProvider struct {
	mu       sync.Mutex
	children map[string][]cloudfile.CloudFile
	byID     map[string]cloudfile.CloudFile
	nextID   int
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
	return cloudfile.CloudFile{}, fmt.Errorf("未找到：%s", fileID)
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
	return f.GetFile(fileID)
}

func (f *fakeWebProvider) MoveFile(fileID, newFolderID, newName string) (cloudfile.CloudFile, error) {
	return f.GetFile(fileID)
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

func (f *fakeWebProvider) DeleteFile(fileID string) error { return nil }

func (f *fakeWebProvider) CreateFolder(name, parentID string) (cloudfile.CloudFile, error) {
	return f.addFolder(name, parentID), nil
}
