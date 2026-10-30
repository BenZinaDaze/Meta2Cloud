# 云存储接入指南

本文说明如何为 Panoply 接入新的云存储后端。项目通过 `internal/cloudfile.Provider` 接口抽象网盘能力，整理流程、媒体库扫描和 WebUI 文件管理都只依赖这个统一接口。

## 接入位置

新增 Provider 建议放在：

```text
internal/storage/<provider>.go
```

实现后在 `internal/storage/registry.go` 注册：

```go
func GetProvider(name string, cfg *config.Config) (cloudfile.Provider, error) {
	switch name {
	case "pan115":
		return NewPan115Provider(cfg)
	case "example": // 新增
		return NewExampleProvider(cfg)
	default:
		return nil, fmt.Errorf("未知的存储 Provider：%s", name)
	}
}
```

`GetProvider` 的名称必须和配置里的 `storage.primary` 一致。当前内置 Provider 列表在 `ListProviders()` 中维护。

## 必须实现的接口

Provider 需要实现 `cloudfile.Provider`（`internal/cloudfile/cloudfile.go`）：

```go
type Provider interface {
	ProviderName() string
	ListFiles(folderID string, pageSize int) ([]CloudFile, error)
	// 递归遍历依赖此方法避免单页截断
	ListAllFiles(folderID string) ([]CloudFile, error)
	GetFile(fileID string) (CloudFile, error)
	ReadText(file CloudFile) (string, error)
	// FindFile 未找到时返回 (nil, nil)
	FindFile(name string, folderID string) (*CloudFile, error)
	RenameFile(fileID string, newName string) (CloudFile, error)
	MoveFile(fileID string, newFolderID string, newName string) (CloudFile, error)
	UploadText(content, name, parentID, mimeType string, overwrite bool) (CloudFile, error)
	UploadBytes(content []byte, name, parentID, mimeType string, overwrite bool) (CloudFile, error)
	TrashFile(fileID string) (CloudFile, error)
	DeleteFile(fileID string) error
	CreateFolder(name, parentID string) (CloudFile, error)
}
```

### 约定

- **统一数据模型**：所有方法出入参使用 `cloudfile.CloudFile`。`IsVideo()`/`IsSubtitle()` 按扩展名判定，无需各平台自行实现。
- **folderID 约定**：参数统一使用 folderID；115 的根目录是 `"0"`（Python 版遗留的 `"root"` 在 Pan115Provider 内部做了归一）。
- **FindFile 语义**：未找到返回 `(nil, nil)`，错误仅用于真正的调用失败。
- **通用辅助**：`GetOrCreateFolder` / `ListMediaFiles` / `Exists` / `WalkAll` 以包级函数形式提供（Go 无默认方法实现），直接复用：

```go
folder, err := cloudfile.GetOrCreateFolder(provider, "剧名 (2024)", parentID)
```

## 测试要求

参照 `internal/storage/storage_test.go`：用 `httptest` 模拟平台 API 服务端，穿透完整请求管线（限流/重试/分页），至少覆盖：

- 平台原始模型 → `CloudFile` 的类型转换
- 递归遍历顺序（目录自身 → 子目录内容 → 后续同级）
- 根目录 ID 归一
- 注册表对未知名称报错
