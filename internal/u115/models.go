package u115

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Token 115 access_token / refresh_token 持久化模型。
type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshTime  int64  `json:"refresh_time"`
}

// ExpiresAt 过期时间戳（秒）。
func (t Token) ExpiresAt() int64 { return t.RefreshTime + t.ExpiresIn }

// IsExpired 是否已过期（预留 60 秒时钟偏移）。
func (t Token) IsExpired() bool {
	return time.Now().Unix()+60 >= t.ExpiresAt()
}

// DeviceCodeSession 扫码登录会话。
type DeviceCodeSession struct {
	Qrcode       string
	UID          string
	TimeValue    string
	Sign         string
	CodeVerifier string
}

// QrcodeStatus 扫码轮询状态。
type QrcodeStatus struct {
	Status  int
	Message string
	Raw     map[string]any
}

// Confirmed 是否已确认（扫码成功）。
func (s QrcodeStatus) Confirmed() bool { return s.Status == 2 }

// File 115 文件或目录的通用描述。
type File struct {
	ID           string
	Name         string
	Category     string
	Size         *int64
	ModifiedTime *int64
	PickCode     *string
	SHA1         *string
	ParentID     *string
	Ico          *string
	Raw          map[string]any
}

// IsFolder 115 约定 category == "0" 为目录。
func (f File) IsFolder() bool { return f.Category == "0" }

// IsFile 是否为文件。
func (f File) IsFile() bool { return !f.IsFolder() }

// Extension 返回小写扩展名（含点）。
func (f File) Extension() string {
	return strings.ToLower(path.Ext(f.Name))
}

// FileFromListItem 从 /open/ufile/files 列表项构造。
func FileFromListItem(raw map[string]any) File {
	return File{
		ID:           jString(raw, "fid"),
		Name:         jString(raw, "fn"),
		Category:     jString(raw, "fc"),
		Size:         jIntPtr(raw, "fs"),
		ModifiedTime: jIntPtr(raw, "upt"),
		PickCode:     jStringPtr(raw, "pc"),
		ParentID:     firstStringPtr(raw, "parent_id", "pid"),
		Ico:          jStringPtr(raw, "ico"),
		Raw:          raw,
	}
}

// firstStringPtr 依次取首个非空字符串字段。
func firstStringPtr(raw map[string]any, keys ...string) *string {
	for _, k := range keys {
		if v := jStringPtr(raw, k); v != nil && *v != "" {
			return v
		}
	}
	return nil
}

// FileFromPathInfo 从 /open/folder/get_info 返回构造。
func FileFromPathInfo(raw map[string]any) File {
	return File{
		ID:           jString(raw, "file_id"),
		Name:         jString(raw, "file_name"),
		Category:     jString(raw, "file_category"),
		Size:         jIntPtr(raw, "size_byte"),
		ModifiedTime: jIntPtr(raw, "utime"),
		PickCode:     jStringPtr(raw, "pick_code"),
		SHA1:         jStringPtr(raw, "sha1"),
		ParentID:     jStringPtr(raw, "parent_id"),
		Ico:          jStringPtr(raw, "ico"),
		Raw:          raw,
	}
}

// FileFromSearchItem 从 /open/ufile/search 结果项构造。
func FileFromSearchItem(raw map[string]any) File {
	return File{
		ID:           jString(raw, "file_id"),
		Name:         jString(raw, "file_name"),
		Category:     jString(raw, "file_category"),
		Size:         jIntPtr(raw, "file_size"),
		ModifiedTime: jIntPtr(raw, "user_utime"),
		PickCode:     jStringPtr(raw, "pick_code"),
		SHA1:         jStringPtr(raw, "sha1"),
		ParentID:     jStringPtr(raw, "parent_id"),
		Ico:          jStringPtr(raw, "ico"),
		Raw:          raw,
	}
}

// SpaceInfo 空间使用信息。
type SpaceInfo struct {
	TotalSize  int64
	RemainSize int64
	Raw        map[string]any
}

// UploadHashes 上传前特征值。
type UploadHashes struct {
	FileSize int64
	FileID   string // 全文件 SHA1
	PreID    string // 前 128MB SHA1
}

// UploadInitInfo 初始化上传返回。
type UploadInitInfo struct {
	Bucket    *string
	Object    *string
	Callback  map[string]any
	SignCheck *string
	PickCode  *string
	SignKey   *string
	Code      *int64
	Status    *int64
	FileID    *string
	Raw       map[string]any
}

// IsFastUpload 是否秒传成功。
func (i UploadInitInfo) IsFastUpload() bool { return i.Status != nil && *i.Status == 2 }

// RequiresSecondCheck 是否需要二次认证。
func (i UploadInitInfo) RequiresSecondCheck() bool {
	return i.Code != nil && (*i.Code == 700 || *i.Code == 701) && i.SignCheck != nil
}

// UploadToken 对象存储临时凭证。
type UploadToken struct {
	Endpoint        string
	AccessKeyID     string
	AccessKeySecret string
	SecurityToken   string
	Raw             map[string]any
}

// ResumeInfo 断点续传信息。
type ResumeInfo struct {
	Callback map[string]any
	Raw      map[string]any
}

// RecycleBinItem 回收站条目。
type RecycleBinItem struct {
	ID       string
	Name     string
	Category *string
	Size     *int64
	Raw      map[string]any
}

// ShareInfo 分享链接信息。
type ShareInfo struct {
	ShareCode    string
	ReceiveCode  string
	Title        string
	FileSize     int64
	ReceiveCount int64
	CreateTime   *string
	ExpireTime   *string
	UserID       string
	UserName     string
	SnapID       string
	Raw          map[string]any
}

// ShareItem 分享链接中的文件/文件夹项。
type ShareItem struct {
	ID           string
	Name         string
	Size         int64
	IsFolder     bool
	ModifiedTime *string
	Fid          *string
	Raw          map[string]any
}

// FileID 返回用于转存的 ID（文件夹用 cid，文件用 fid）。
func (s ShareItem) FileID() string {
	if s.IsFolder {
		return s.ID
	}
	if s.Fid != nil {
		return *s.Fid
	}
	return s.ID
}

// ReceiveResult 转存结果。
type ReceiveResult struct {
	Success     bool
	FolderCount int64
	FileCount   int64
	TotalSize   int64
	Title       string
	Error       string
	Raw         map[string]any
}

// ── PKCE 与 token 持久化 ─────────────────────────────────────

// GenerateCodeVerifier 生成 PKCE code_verifier（115 要求长度 43~128）。
func GenerateCodeVerifier(length int) (string, error) {
	if length < 43 || length > 128 {
		return "", NewAPIError("code_verifier 长度必须在 43~128 之间")
	}
	buf := make([]byte, 96)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf)[:length], nil
}

// GenerateCodeChallenge 根据 code_verifier 计算 SHA256 challenge。
func GenerateCodeChallenge(codeVerifier string) string {
	digest := sha256.Sum256([]byte(codeVerifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

// SaveToken 将 token 写入本地 JSON 文件。
func SaveToken(token Token, tokenPath string) error {
	if dir := filepath.Dir(tokenPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tokenPath, data, 0o600); err != nil {
		return err
	}
	// WriteFile 的 perm 仅对新建文件生效，已存在文件（历史版本可能 0644）显式收紧
	return os.Chmod(tokenPath, 0o600)
}

// LoadToken 从本地 JSON 文件读取 token，不存在返回 (zero, false), nil。
func LoadToken(tokenPath string) (Token, bool, error) {
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Token{}, false, nil
		}
		return Token{}, false, err
	}
	// 自愈历史遗留的宽松权限（凭据文件应仅属主可读）
	_ = os.Chmod(tokenPath, 0o600)
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return Token{}, false, err
	}
	token := Token{
		AccessToken:  jString(raw, "access_token"),
		RefreshToken: jString(raw, "refresh_token"),
		ExpiresIn:    jInt(raw, "expires_in", 0),
		RefreshTime:  jInt(raw, "refresh_time", time.Now().Unix()),
	}
	return token, true, nil
}
