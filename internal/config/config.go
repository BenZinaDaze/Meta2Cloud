// Package config 从 YAML 文件加载全局配置，对外提供结构化的配置对象。
// 对应 Python 版 mediaparser/config.py（Google Drive 配置段已移除）。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultTMDBImageBaseURL TMDB 图片默认根地址。
const DefaultTMDBImageBaseURL = "https://image.tmdb.org"

// parserRulesFilename 解析规则文件名（与主配置同目录）。
const parserRulesFilename = "parser-rules.yaml"

// ParseError 配置解析错误。
type ParseError struct{ msg string }

func (e *ParseError) Error() string { return e.msg }

func parseErr(format string, args ...any) error {
	return &ParseError{msg: fmt.Sprintf(format, args...)}
}

// normalizeImageBaseURL 规范化 TMDB 图片根地址。
func normalizeImageBaseURL(baseURL string) string {
	value := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if value == "" {
		return DefaultTMDBImageBaseURL
	}
	return value
}

// ── 各配置段 ────────────────────────────────────────────────

// Tmdb TMDB API 配置。
type Tmdb struct {
	APIKey       string `yaml:"api_key"`
	Language     string `yaml:"language"`
	Proxy        string `yaml:"proxy"`
	Timeout      int    `yaml:"timeout"`
	ImageBaseURL string `yaml:"image_base_url"`
}

// Parser 文件名识别自定义规则。
type Parser struct {
	CustomWords         []string `yaml:"custom_words"`
	CustomReleaseGroups []string `yaml:"custom_release_groups"`
}

// U115 115 网盘配置。
type U115 struct {
	ClientID                  string `yaml:"client_id"`
	TokenJSON                 string `yaml:"token_json"`
	SessionJSON               string `yaml:"session_json"`
	Cookie                    string `yaml:"cookie"`
	DownloadFolderID          string `yaml:"download_folder_id"`
	RootFolderID              string `yaml:"root_folder_id"`
	MovieRootID               string `yaml:"movie_root_id"`
	TvRootID                  string `yaml:"tv_root_id"`
	AutoOrganizeEnabled       bool   `yaml:"auto_organize_enabled"`
	AutoOrganizePollSeconds   int    `yaml:"auto_organize_poll_seconds"`
	AutoOrganizeStableSeconds int    `yaml:"auto_organize_stable_seconds"`
	QPS                       int    `yaml:"qps"`
}

// Pipeline 整理流水线开关。
type Pipeline struct {
	SkipTMDB             bool `yaml:"skip_tmdb"`
	MoveOnTMDBMiss       bool `yaml:"move_on_tmdb_miss"`
	DryRun               bool `yaml:"dry_run"`
	ReplaceExistingVideo bool `yaml:"replace_existing_video"`
	SkipMetadataUpload   bool `yaml:"skip_metadata_upload"`
	// MergeDuplicateFolders 同一媒体存在多个目录（历史译名差异）时自动合并进钉扎目录；
	// nil/未配置时默认开启。
	MergeDuplicateFolders *bool `yaml:"merge_duplicate_folders"`
}

// MergeDuplicatesEnabled 是否开启重复目录自动合并（默认 true）。
func (p Pipeline) MergeDuplicatesEnabled() bool {
	return p.MergeDuplicateFolders == nil || *p.MergeDuplicateFolders
}

// Subtitle 字幕整理配置。
type Subtitle struct {
	Enabled bool `yaml:"enabled"`
}

// Telegram 通知配置。
type Telegram struct {
	BotToken        string `yaml:"bot_token"`
	ChatID          string `yaml:"chat_id"`
	DebounceSeconds int    `yaml:"debounce_seconds"`
}

// MediaVault MediaVault 配置。
type MediaVault struct {
	Enabled   bool   `yaml:"enabled"`
	Scheme    string `yaml:"scheme"`
	Host      string `yaml:"host"`
	APIKey    string `yaml:"api_key"`
	SourceDir string `yaml:"source_dir"`
	Timeout   int    `yaml:"timeout"`
}

// WebUI Web 界面配置。
type WebUI struct {
	Username         string `yaml:"username"`
	Password         string `yaml:"password"`
	SecretKey        string `yaml:"secret_key"`
	TokenExpireHours int    `yaml:"token_expire_hours"`
	WebhookSecret    string `yaml:"webhook_secret"`
	LogRetentionDays int    `yaml:"log_retention_days"`
}

// Storage 存储后端选择配置。
// 主存储 Provider：pipeline / organizer 使用此 provider。当前仅支持 pan115。
type Storage struct {
	Primary string `yaml:"primary"`
}

// StrmMapping STRM 路径映射：网盘目录 → 本地 STRM 输出目录。
type StrmMapping struct {
	PanFolderID string `yaml:"pan_folder_id"`
	LocalPath   string `yaml:"local_path"`
}

// Strm STRM 生成与 302 直链播放配置。
type Strm struct {
	Enabled             bool          `yaml:"enabled"`
	URLBase             string        `yaml:"url_base"`
	URLSecret           string        `yaml:"url_secret"`
	SyncIntervalSeconds int           `yaml:"sync_interval_seconds"`
	MediaExt            []string      `yaml:"media_ext"`
	FetchSidecar        bool          `yaml:"fetch_sidecar"`
	SidecarExt          []string      `yaml:"sidecar_ext"`
	Mappings            []StrmMapping `yaml:"mappings"`
}

// Config 全局配置根。
type Config struct {
	Tmdb       Tmdb       `yaml:"tmdb"`
	Parser     Parser     `yaml:"parser"`
	U115       U115       `yaml:"u115"`
	Pipeline   Pipeline   `yaml:"pipeline"`
	Subtitle   Subtitle   `yaml:"subtitle"`
	Telegram   Telegram   `yaml:"telegram"`
	MediaVault MediaVault `yaml:"mediavault"`
	WebUI      WebUI      `yaml:"webui"`
	Storage    Storage    `yaml:"storage"`
	Strm       Strm       `yaml:"strm"`
}

// Default 返回带默认值的配置。
func Default() *Config {
	return &Config{
		Tmdb:   Tmdb{Language: "zh-CN", Timeout: 10},
		Parser: Parser{CustomWords: []string{}, CustomReleaseGroups: []string{}},
		U115: U115{
			ClientID:                  "100197847",
			TokenJSON:                 "config/115-token.json",
			SessionJSON:               "config/115-device-session.json",
			AutoOrganizePollSeconds:   45,
			AutoOrganizeStableSeconds: 30,
			QPS:                       3,
		},
		Subtitle:   Subtitle{Enabled: true},
		MediaVault: MediaVault{Scheme: "http", Timeout: 10},
		WebUI:      WebUI{Username: "admin", TokenExpireHours: 24, LogRetentionDays: 7},
		Storage:    Storage{Primary: "pan115"},
		Strm: Strm{
			MediaExt:     append([]string{}, DefaultStrmMediaExt...),
			FetchSidecar: true,
			SidecarExt:   append([]string{}, DefaultStrmSidecarExt...),
		},
	}
}

// rawConfig 与 YAML 结构对应的原始映射（先取默认值再覆盖）。
type rawConfig struct {
	Tmdb       map[string]any `yaml:"tmdb"`
	Parser     map[string]any `yaml:"parser"`
	U115       map[string]any `yaml:"u115"`
	Pipeline   map[string]any `yaml:"pipeline"`
	Subtitle   map[string]any `yaml:"subtitle"`
	Telegram   map[string]any `yaml:"telegram"`
	MediaVault map[string]any `yaml:"mediavault"`
	WebUI      map[string]any `yaml:"webui"`
	Storage    map[string]any `yaml:"storage"`
	Strm       map[string]any `yaml:"strm"`
}

// Load 从 YAML 文件加载配置；path 为空时按默认搜索路径查找。
// 文件不存在时返回默认配置（不报错，与 Python 版一致）。
func Load(path string) (*Config, error) {
	configPath, found := findConfig(path)
	if !found {
		return Default(), nil
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取配置文件失败：%v，将使用默认配置。\n", err)
		return Default(), nil
	}
	cfg, err := parseConfig(data)
	if err != nil {
		return nil, err
	}

	// 解析规则文件（与主配置同目录）
	rulesPath := filepath.Join(filepath.Dir(configPath), parserRulesFilename)
	if rulesData, err := os.ReadFile(rulesPath); err == nil {
		if parser, err := parseParserRules(rulesData); err == nil {
			cfg.Parser = parser
		}
	}
	return cfg, nil
}

func parseConfig(data []byte) (*Config, error) {
	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		fmt.Fprintf(os.Stderr, "读取配置文件失败：%v，将使用默认配置。\n", err)
		return Default(), nil
	}
	cfg := Default()
	cfg.Tmdb = parseTmdb(raw.Tmdb)
	cfg.Parser = parseParser(raw.Parser)
	cfg.U115 = parseU115(raw.U115)
	cfg.Pipeline = parsePipeline(raw.Pipeline)
	cfg.Subtitle = parseSubtitle(raw.Subtitle)
	cfg.Telegram = parseTelegram(raw.Telegram)
	cfg.MediaVault = parseMediaVault(raw.MediaVault)
	cfg.WebUI = parseWebUI(raw.WebUI)
	cfg.Storage = parseStorage(raw.Storage)
	cfg.Strm = parseStrm(raw.Strm)
	return cfg, nil
}

// parseParserRules 解析 parser-rules.yaml（支持顶层或 parser: 包裹）。
func parseParserRules(data []byte) (Parser, error) {
	var top map[string]any
	if err := yaml.Unmarshal(data, &top); err != nil {
		return Parser{}, err
	}
	section := top
	if inner, ok := top["parser"].(map[string]any); ok {
		section = inner
	}
	return parseParser(section), nil
}

func findConfig(path string) (string, bool) {
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
		return "", false
	}
	// 搜索路径：工作目录 config/ → 可执行文件旁 config/ → 兼容根目录
	candidates := []string{
		filepath.Join("config", "config.yaml"),
		"config.yaml",
	}
	if exe, err := os.Executable(); err == nil {
		base := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(base, "config", "config.yaml"),
			filepath.Join(base, "config.yaml"),
		)
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			abs, _ := filepath.Abs(candidate)
			return abs, true
		}
	}
	return "", false
}

// ── 各段解析（默认值兜底，与 Python from_dict 语义一致） ──

func str(m map[string]any, key, def string) string {
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return def
}

func integer(m map[string]any, key string, def int) int {
	switch v := m[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return def
}

func boolean(m map[string]any, key string, def bool) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return def
}

func strList(m map[string]any, key string) []string {
	out := []string{}
	if items, ok := m[key].([]any); ok {
		for _, item := range items {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func parseTmdb(m map[string]any) Tmdb {
	return Tmdb{
		APIKey:       str(m, "api_key", ""),
		Language:     str(m, "language", "zh-CN"),
		Proxy:        str(m, "proxy", ""),
		Timeout:      integer(m, "timeout", 10),
		ImageBaseURL: str(m, "image_base_url", ""),
	}
}

func parseParser(m map[string]any) Parser {
	return Parser{
		CustomWords:         strList(m, "custom_words"),
		CustomReleaseGroups: strList(m, "custom_release_groups"),
	}
}

func parseU115(m map[string]any) U115 {
	cfg := U115{
		ClientID:                  str(m, "client_id", "100197847"),
		TokenJSON:                 str(m, "token_json", "config/115-token.json"),
		SessionJSON:               str(m, "session_json", "config/115-device-session.json"),
		Cookie:                    str(m, "cookie", ""),
		DownloadFolderID:          str(m, "download_folder_id", ""),
		RootFolderID:              str(m, "root_folder_id", ""),
		MovieRootID:               str(m, "movie_root_id", ""),
		TvRootID:                  str(m, "tv_root_id", ""),
		AutoOrganizeEnabled:       boolean(m, "auto_organize_enabled", false),
		AutoOrganizePollSeconds:   maxInt(integer(m, "auto_organize_poll_seconds", 45), 10),
		AutoOrganizeStableSeconds: maxInt(integer(m, "auto_organize_stable_seconds", 30), 0),
		QPS:                       maxInt(integer(m, "qps", 3), 1),
	}
	return cfg
}

func parsePipeline(m map[string]any) Pipeline {
	return Pipeline{
		SkipTMDB:             boolean(m, "skip_tmdb", false),
		MoveOnTMDBMiss:       boolean(m, "move_on_tmdb_miss", false),
		DryRun:               boolean(m, "dry_run", false),
		ReplaceExistingVideo: boolean(m, "replace_existing_video", false),
		SkipMetadataUpload:   boolean(m, "skip_metadata_upload", false),
	}
}

func parseSubtitle(m map[string]any) Subtitle {
	return Subtitle{Enabled: boolean(m, "enabled", true)}
}

func parseTelegram(m map[string]any) Telegram {
	return Telegram{
		BotToken:        str(m, "bot_token", ""),
		ChatID:          str(m, "chat_id", ""),
		DebounceSeconds: integer(m, "debounce_seconds", 0),
	}
}

func parseMediaVault(m map[string]any) MediaVault {
	return MediaVault{
		Enabled:   boolean(m, "enabled", false),
		Scheme:    strings.ToLower(strings.TrimSpace(str(m, "scheme", "http"))),
		Host:      str(m, "host", ""),
		APIKey:    str(m, "api_key", ""),
		SourceDir: str(m, "source_dir", ""),
		Timeout:   maxInt(integer(m, "timeout", 10), 1),
	}
}

func parseWebUI(m map[string]any) WebUI {
	return WebUI{
		Username:         str(m, "username", "admin"),
		Password:         str(m, "password", ""),
		SecretKey:        str(m, "secret_key", ""),
		TokenExpireHours: integer(m, "token_expire_hours", 24),
		WebhookSecret:    str(m, "webhook_secret", ""),
		LogRetentionDays: maxInt(integer(m, "log_retention_days", 7), 1),
	}
}

func parseStorage(m map[string]any) Storage {
	return Storage{Primary: str(m, "primary", "pan115")}
}

// DefaultStrmMediaExt STRM 默认收集的视频扩展名（与 cloudfile.videoExtensions 对齐）。
var DefaultStrmMediaExt = []string{
	"mp4", "mkv", "avi", "mov", "wmv", "flv", "ts", "m2ts", "webm",
	"rmvb", "rm", "mpg", "mpeg", "vob", "iso", "3gp",
}

// DefaultStrmSidecarExt STRM 同步默认下载的伴随文件扩展名（默认仅外挂字幕）。
// NFO/图片不下载：媒体服务器对 strm 用文件名+刮削即可，海报图片可由刮削器自行拉取。
var DefaultStrmSidecarExt = []string{"srt", "ass", "ssa"}

func parseStrm(m map[string]any) Strm {
	mediaExt := strList(m, "media_ext")
	if len(mediaExt) == 0 {
		mediaExt = append([]string{}, DefaultStrmMediaExt...)
	}
	mappings := []StrmMapping{}
	if items, ok := m["mappings"].([]any); ok {
		for _, item := range items {
			mm, ok := item.(map[string]any)
			if !ok {
				continue
			}
			mapping := StrmMapping{
				PanFolderID: str(mm, "pan_folder_id", ""),
				LocalPath:   str(mm, "local_path", ""),
			}
			if mapping.PanFolderID != "" && mapping.LocalPath != "" {
				mappings = append(mappings, mapping)
			}
		}
	}
	sidecarExt := strList(m, "sidecar_ext")
	if len(sidecarExt) == 0 {
		sidecarExt = append([]string{}, DefaultStrmSidecarExt...)
	}
	return Strm{
		Enabled:             boolean(m, "enabled", false),
		URLBase:             strings.TrimRight(strings.TrimSpace(str(m, "url_base", "")), "/"),
		URLSecret:           strings.TrimSpace(str(m, "url_secret", "")),
		SyncIntervalSeconds: integer(m, "sync_interval_seconds", 0),
		MediaExt:            mediaExt,
		FetchSidecar:        boolean(m, "fetch_sidecar", true),
		SidecarExt:          sidecarExt,
		Mappings:            mappings,
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ── 便利方法 ────────────────────────────────────────────────

// TmdbProxy TMDB 代理（空串返回 nil 语义由调用方处理）。
func (c *Config) TmdbProxy() string { return c.Tmdb.Proxy }

// TmdbImageBaseURL 规范化后的图片根地址。
func (c *Config) TmdbImageBaseURL() string {
	return normalizeImageBaseURL(c.Tmdb.ImageBaseURL)
}

// MediaVaultBaseURL MediaVault 基础 URL。
func (c *Config) MediaVaultBaseURL() string {
	scheme := strings.ToLower(strings.TrimSpace(c.MediaVault.Scheme))
	if scheme == "" {
		scheme = "http"
	}
	host := strings.TrimSpace(c.MediaVault.Host)
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}

// IsTmdbReady 是否具备 TMDB 查询条件（有 api_key）。
func (c *Config) IsTmdbReady() bool { return c.Tmdb.APIKey != "" }

// ActiveStorageName 返回当前启用的存储后端名称。
func (c *Config) ActiveStorageName() string {
	if c.Storage.Primary != "" {
		return c.Storage.Primary
	}
	return "pan115"
}

// ActiveScanFolderID 返回当前存储后端对应的扫描目录 ID（115: 下载目录）。
func (c *Config) ActiveScanFolderID() string {
	return c.U115.DownloadFolderID
}

// ActiveRootFolderID 返回当前存储后端的媒体库根目录 ID。
func (c *Config) ActiveRootFolderID() string {
	return c.U115.RootFolderID
}

// ActiveMovieRootID 返回电影根目录 ID，未单独配置时回退到媒体库根目录。
func (c *Config) ActiveMovieRootID() string {
	if c.U115.MovieRootID != "" {
		return c.U115.MovieRootID
	}
	return c.U115.RootFolderID
}

// ActiveTvRootID 返回剧集根目录 ID，未单独配置时回退到媒体库根目录。
func (c *Config) ActiveTvRootID() string {
	if c.U115.TvRootID != "" {
		return c.U115.TvRootID
	}
	return c.U115.RootFolderID
}
