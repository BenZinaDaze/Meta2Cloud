package cloudfile

import (
	"fmt"
	"sync"
	"testing"
)

// lookupFake 内存版 Provider：children 按父 ID 键控，byID 支持直查。
type lookupFake struct {
	mu       sync.Mutex
	children map[string][]CloudFile
	byID     map[string]CloudFile
}

func newLookupFake() *lookupFake {
	return &lookupFake{children: map[string][]CloudFile{}, byID: map[string]CloudFile{}}
}

func (f *lookupFake) add(name, id, parentID string, folder bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ft := TypeFile
	if folder {
		ft = TypeFolder
	}
	cf := CloudFile{ID: id, Name: name, FileType: ft, ParentID: parentID}
	f.children[parentID] = append(f.children[parentID], cf)
	f.byID[id] = cf
}

func (f *lookupFake) ProviderName() string { return "lookup-fake" }

func (f *lookupFake) ListFiles(folderID string, _ int) ([]CloudFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]CloudFile{}, f.children[folderID]...), nil
}

func (f *lookupFake) ListAllFiles(folderID string) ([]CloudFile, error) {
	return f.ListFiles(folderID, 1000)
}

func (f *lookupFake) GetFile(fileID string) (CloudFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if cf, ok := f.byID[fileID]; ok {
		return cf, nil
	}
	return CloudFile{}, fmt.Errorf("未找到：%s", fileID)
}

// 以下方法与查找无关，仅为满足 Provider 接口。
func (f *lookupFake) ReadText(CloudFile) (string, error) { return "", nil }

func (f *lookupFake) FindFile(name, folderID string) (*CloudFile, error) {
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

func (f *lookupFake) RenameFile(fileID, _ string) (CloudFile, error) { return f.GetFile(fileID) }
func (f *lookupFake) MoveFile(fileID, _, _ string) (CloudFile, error) {
	return f.GetFile(fileID)
}
func (f *lookupFake) UploadText(_ string, name, parentID, _ string, _ bool) (CloudFile, error) {
	f.add(name, "up-"+name, parentID, false)
	return f.GetFile("up-" + name)
}
func (f *lookupFake) UploadBytes(b []byte, name, parentID, mime string, overwrite bool) (CloudFile, error) {
	return f.UploadText(string(b), name, parentID, mime, overwrite)
}
func (f *lookupFake) TrashFile(fileID string) (CloudFile, error) { return f.GetFile(fileID) }
func (f *lookupFake) DeleteFile(string) error                    { return nil }
func (f *lookupFake) CreateFolder(name, parentID string) (CloudFile, error) {
	id := fmt.Sprintf("d%d", len(f.byID)+1)
	f.add(name, id, parentID, true)
	return f.GetFile(id)
}

func TestFindFolderByName(t *testing.T) {
	fp := newLookupFake()
	fp.add("媒体库", "w1", "0", true)
	fp.add("媒体库.txt", "w2", "0", false) // 同名文件不应命中
	fp.add("其他", "w3", "w1", true)      // 非根下不参与查找

	if id, ok := FindFolderByName(fp, "媒体库"); !ok || id != "w1" {
		t.Fatalf("FindFolderByName = %q,%v want w1,true", id, ok)
	}
	if _, ok := FindFolderByName(fp, "不存在"); ok {
		t.Fatal("不存在的目录不应命中")
	}
	if _, ok := FindFolderByName(fp, ""); ok {
		t.Fatal("空名不应命中")
	}
}

func TestResolveFolderRef(t *testing.T) {
	fp := newLookupFake()
	fp.add("媒体库", "w1", "0", true)
	fp.add("旧目录", "legacy-id", "0", true)

	// 新配置：按名解析
	if id, ok := ResolveFolderRef(fp, "媒体库"); !ok || id != "w1" {
		t.Fatalf("按名解析 = %q,%v want w1,true", id, ok)
	}
	// 旧配置：值本身就是 ID，按名找不到时回退直查
	if id, ok := ResolveFolderRef(fp, "legacy-id"); !ok || id != "legacy-id" {
		t.Fatalf("ID 回退 = %q,%v want legacy-id,true", id, ok)
	}
	// 两者皆失败
	if _, ok := ResolveFolderRef(fp, "不存在"); ok {
		t.Fatal("未知的名字/ID 不应命中")
	}
	if _, ok := ResolveFolderRef(fp, ""); ok {
		t.Fatal("空值不应命中")
	}
}

func TestResolveFolderPath(t *testing.T) {
	fp := newLookupFake()
	fp.add("Media", "w1", "0", true)
	fp.add("剧集", "w2", "w1", true)
	fp.add("电影", "w3", "w1", true)

	if id, ok := ResolveFolderPath(fp, "Media/剧集"); !ok || id != "w2" {
		t.Fatalf("两级路径解析 = %q,%v want w2,true", id, ok)
	}
	if id, ok := ResolveFolderPath(fp, "/Media/"); !ok || id != "w1" {
		t.Fatalf("斜杠容错解析 = %q,%v want w1,true", id, ok)
	}
	if _, ok := ResolveFolderPath(fp, "Media/不存在"); ok {
		t.Fatal("末级不存在不应命中")
	}
	if _, ok := ResolveFolderPath(fp, ""); ok {
		t.Fatal("空路径不应命中")
	}
	if NormalizeFolderPath("/Media//剧集/") != "Media/剧集" {
		t.Fatal("路径规范化错误")
	}
}
