// Package config 从 YAML 文件加载全局配置，对外提供结构化的配置对象。
// 对应 Python 版 mediaparser/config.py（Google Drive 配置段已移除）。
package config

import (
	"crypto/rand"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

// DefaultTMDBImageBaseURL TMDB 图片默认根地址。
const DefaultTMDBImageBaseURL = "https://image.tmdb.org"

// DefaultWebUIUsername WebUI 默认用户名（配置缺失时的兜底；运行时登录比对与此同源）。
const DefaultWebUIUsername = "admin"

// DefaultPlexTimeoutSecs Plex 请求超时默认秒数（配置缺失/非法时的兜底，运行时回退与此同源）。
const DefaultPlexTimeoutSecs = 10

// parserRulesFilename 解析规则文件名（与主配置同目录）。
const parserRulesFilename = "parser-rules.yaml"

// SecretsFilename 敏感配置文件名（与主配置同目录，权限 0600）。
// 敏感键（见 SecretKeys）的值持久化在此文件而非主配置，主配置可整体外发（贴日志/求助）而不泄漏凭证。
const SecretsFilename = "secrets.yaml"

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
	Cookie                    string `yaml:"cookie,omitempty"`
	DownloadFolderID          string `yaml:"download_folder_id"`
	RootFolderID              string `yaml:"root_folder_id"`
	AutoOrganizeEnabled       bool   `yaml:"auto_organize_enabled"`
	AutoOrganizeCron          string `yaml:"auto_organize_cron"`
	AutoOrganizeStableSeconds int    `yaml:"auto_organize_stable_seconds"`
	QPS                       int    `yaml:"qps"`
}

// Rss RSS 订阅轮询配置。
type Rss struct {
	PollCron string `yaml:"poll_cron"`
}

// Pipeline 整理流水线开关。
type Pipeline struct {
	SkipTMDB             bool `yaml:"skip_tmdb"`
	MoveOnTMDBMiss       bool `yaml:"move_on_tmdb_miss"`
	ReplaceExistingVideo bool `yaml:"replace_existing_video"`
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
	BotToken        string `yaml:"bot_token,omitempty"`
	ChatID          string `yaml:"chat_id"`
	DebounceSeconds int    `yaml:"debounce_seconds"`
}

// MediaVault MediaVault 配置。
type MediaVault struct {
	Enabled   bool   `yaml:"enabled"`
	Scheme    string `yaml:"scheme"`
	Host      string `yaml:"host"`
	APIKey    string `yaml:"api_key,omitempty"`
	SourceDir string `yaml:"source_dir"`
	Timeout   int    `yaml:"timeout"`
}

// WebUI Web 界面配置。
// Password 为旧版明文密码（读取兼容，登录成功后自动迁移为 PasswordHash）；两者并存时哈希优先。
type WebUI struct {
	Username         string `yaml:"username"`
	Password         string `yaml:"password,omitempty"`
	PasswordHash     string `yaml:"password_hash,omitempty"`
	SecretKey        string `yaml:"secret_key,omitempty"`
	TokenExpireHours int    `yaml:"token_expire_hours"`
	WebhookSecret    string `yaml:"webhook_secret,omitempty"`
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
	Enabled      bool          `yaml:"enabled"`
	URLBase      string        `yaml:"url_base"`
	URLSecret    string        `yaml:"url_secret,omitempty"`
	SyncCron     string        `yaml:"sync_cron"`
	RefreshPlex  bool          `yaml:"refresh_plex"`
	MediaExt     []string      `yaml:"media_ext"`
	FetchSidecar bool          `yaml:"fetch_sidecar"`
	SidecarExt   []string      `yaml:"sidecar_ext"`
	Mappings     []StrmMapping `yaml:"mappings"`
}

// Trakt Trakt 观看同步配置（token 持久化在 TokenJSON 文件，设备码配对获取）。
type Trakt struct {
	Enabled            bool   `yaml:"enabled"`
	ClientID           string `yaml:"client_id"`
	TokenJSON          string `yaml:"token_json"`
	SyncCron           string `yaml:"sync_cron"`
	SessionPollSeconds int    `yaml:"session_poll_seconds"`
	SSEEnabled         bool   `yaml:"sse_enabled"`
	SyncToPlex         bool   `yaml:"sync_to_plex"`
	SyncToTrakt        bool   `yaml:"sync_to_trakt"`
}

// Plex Plex Media Server 连接配置（authToken 与 client_identifier 等配对态
// 持久化在 TokenJSON 文件，不进 YAML）。
type Plex struct {
	BaseURL   string `yaml:"base_url"`
	TokenJSON string `yaml:"token_json"`
	Timeout   int    `yaml:"timeout"`
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
	Trakt      Trakt      `yaml:"trakt"`
	Plex       Plex       `yaml:"plex"`
	Rss        Rss        `yaml:"rss"`
}

// Default 返回带默认值的配置。
func Default() *Config {
	return &Config{
		Tmdb:   Tmdb{Language: "zh-CN", Timeout: 10},
		Parser: Parser{CustomWords: []string{}, CustomReleaseGroups: []string{}},
		U115: U115{
			ClientID:    "100197847",
			TokenJSON:   "config/115-token.json",
			SessionJSON: "config/115-device-session.json",
			// 默认每分钟：cron 最小粒度 1 分钟，等价旧版默认 45s 轮询；
			// 空串仍表示不定时。旧版 enabled 判定在轮询内部，此处保持一致。
			AutoOrganizeCron:          "* * * * *",
			AutoOrganizeStableSeconds: 30,
			QPS:                       3,
		},
		Subtitle:   Subtitle{Enabled: true},
		MediaVault: MediaVault{Scheme: "http", Timeout: 10},
		WebUI:      WebUI{Username: DefaultWebUIUsername, TokenExpireHours: 24, LogRetentionDays: 7},
		Storage:    Storage{Primary: "pan115"},
		Strm: Strm{
			MediaExt:     append([]string{}, DefaultStrmMediaExt...),
			FetchSidecar: true,
			SidecarExt:   append([]string{}, DefaultStrmSidecarExt...),
		},
		Trakt: Trakt{
			TokenJSON: "config/trakt-token.json",
			// 默认每 15 分钟：等价旧默认 900s；空串表示不定时（仅 SSE 实时/手动触发）
			SyncCron:           "*/15 * * * *",
			SessionPollSeconds: 45,
			SSEEnabled:         true,
			SyncToPlex:         true,
			SyncToTrakt:        true,
		},
		Plex: Plex{
			TokenJSON: "config/plex-auth.json",
			Timeout:   DefaultPlexTimeoutSecs,
		},
		Rss: Rss{
			PollCron: "*/5 * * * *",
		},
	}
}

// Load 从 YAML 文件加载配置；path 为空时按默认搜索路径查找。
// 文件不存在时返回默认配置（首次启动，缺失文件由 EnsureDefaultFiles 生成）；
// 文件存在但读取/解析失败时返回错误——手改 YAML 的缩进错误若静默回退默认值，
// 应用会带着清零的 root folder 等关键配置继续运行，比拒绝启动难排查得多。
func Load(path string) (*Config, error) {
	configPath, found := findConfig(path)
	if !found {
		return Default(), nil
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件 %s 失败：%w", configPath, err)
	}
	main, err := unmarshalYAMLMapStrict(data)
	if err != nil {
		return nil, fmt.Errorf("解析配置文件 %s 失败：%w", configPath, err)
	}
	dir := filepath.Dir(configPath)
	// 敏感配置文件（与主配置同目录）：敏感键的值优先于主配置
	secretsPath := filepath.Join(dir, SecretsFilename)
	if secretsData, err := os.ReadFile(secretsPath); err == nil {
		secrets, err := unmarshalYAMLMapStrict(secretsData)
		if err != nil {
			return nil, fmt.Errorf("解析敏感配置文件 %s 失败：%w", secretsPath, err)
		}
		MergeSecrets(main, secrets)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("读取敏感配置文件 %s 失败：%w", secretsPath, err)
	}
	cfg := parseConfigMap(main)

	// 解析规则文件（与主配置同目录）
	rulesPath := filepath.Join(dir, parserRulesFilename)
	if rulesData, err := os.ReadFile(rulesPath); err == nil {
		parser, err := parseParserRules(rulesData)
		if err != nil {
			return nil, fmt.Errorf("解析规则文件 %s 失败：%w", rulesPath, err)
		}
		cfg.Parser = parser
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("读取规则文件 %s 失败：%w", rulesPath, err)
	}
	return cfg, nil
}

func parseConfig(data []byte) (*Config, error) {
	return parseConfigMap(unmarshalYAMLMap(data)), nil
}

// parseConfigMap 从配置 map 解析（各段默认值兜底，与 Python from_dict 语义一致）。
func parseConfigMap(m map[string]any) *Config {
	cfg := Default()
	cfg.Tmdb = parseTmdb(sectionMap(m, "tmdb"))
	cfg.Parser = parseParser(sectionMap(m, "parser"))
	cfg.U115 = parseU115(sectionMap(m, "u115"))
	cfg.Pipeline = parsePipeline(sectionMap(m, "pipeline"))
	cfg.Subtitle = parseSubtitle(sectionMap(m, "subtitle"))
	cfg.Telegram = parseTelegram(sectionMap(m, "telegram"))
	cfg.MediaVault = parseMediaVault(sectionMap(m, "mediavault"))
	cfg.WebUI = parseWebUI(sectionMap(m, "webui"))
	cfg.Storage = parseStorage(sectionMap(m, "storage"))
	cfg.Strm = parseStrm(sectionMap(m, "strm"))
	cfg.Trakt = parseTrakt(sectionMap(m, "trakt"))
	cfg.Plex = parsePlex(sectionMap(m, "plex"))
	cfg.Rss = parseRss(sectionMap(m, "rss"))
	return cfg
}

// sectionMap 取配置段（缺失返回 nil，各段解析函数按默认值兜底）。
func sectionMap(m map[string]any, name string) map[string]any {
	sec, _ := m[name].(map[string]any)
	return sec
}

// unmarshalYAMLMap 解析为配置 map；失败时打印日志并返回空 map（默认值兜底）。
// 仅限辅助场景（webhook 密钥探测等，配置坏时不该被其阻塞）；主加载路径用
// unmarshalYAMLMapStrict 严格报错。
func unmarshalYAMLMap(data []byte) map[string]any {
	out := map[string]any{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		fmt.Fprintf(os.Stderr, "读取配置文件失败：%v，将使用默认配置。\n", err)
		return map[string]any{}
	}
	return out
}

// unmarshalYAMLMapStrict 解析为配置 map，语法/类型错误原样返回（Load 快速失败用）。
func unmarshalYAMLMapStrict(data []byte) (map[string]any, error) {
	out := map[string]any{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
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

// cronString 读取 cron 表达式配置（TrimSpace）。
// 显式空串保留（空＝不定时）；键缺失时用默认值。
// 非字符串标量（手改 YAML 如 sync_cron: 123）转成字符串：
// 交由运行时 cron 解析报错，而非静默回退默认（与 WebUI 保存校验拒绝非字符串的行为对齐）。
func cronString(m map[string]any, key, def string) string {
	if v, ok := m[key].(string); ok {
		return strings.TrimSpace(v)
	}
	if v := m[key]; v != nil {
		return strings.TrimSpace(fmt.Sprint(v))
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
		AutoOrganizeEnabled:       boolean(m, "auto_organize_enabled", false),
		AutoOrganizeCron:          cronString(m, "auto_organize_cron", "* * * * *"),
		AutoOrganizeStableSeconds: maxInt(integer(m, "auto_organize_stable_seconds", 30), 0),
		QPS:                       maxInt(integer(m, "qps", 3), 1),
	}
	return cfg
}

func parsePipeline(m map[string]any) Pipeline {
	return Pipeline{
		SkipTMDB:             boolean(m, "skip_tmdb", false),
		MoveOnTMDBMiss:       boolean(m, "move_on_tmdb_miss", false),
		ReplaceExistingVideo: boolean(m, "replace_existing_video", false),
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
		Username:         str(m, "username", DefaultWebUIUsername),
		Password:         str(m, "password", ""),
		PasswordHash:     str(m, "password_hash", ""),
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
// 图片等元数据不下载：媒体服务器对 strm 用文件名+刮削即可，海报可由刮削器自行拉取。
var DefaultStrmSidecarExt = []string{"srt", "ass", "ssa"}

// normalizeStrmURLBase 归一化 STRM 外部访问地址：去掉首尾空白与末尾斜杠；
// 省略协议时默认补 http://（容器互访场景常见写法，如 "meta2cloud:38765"）。
func normalizeStrmURLBase(u string) string {
	u = strings.TrimRight(strings.TrimSpace(u), "/")
	if u == "" {
		return ""
	}
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		u = "http://" + u
	}
	return u
}

// EnsureDefaultFiles 首次启动友好：缺失的配置文件写出默认内容（已存在的不覆盖）。
// 覆盖 config.yaml / secrets.yaml / parser-rules.yaml / categories.yaml。
// 首次生成的主配置不含密码：随机初始密码（打印在启动日志）以 bcrypt 哈希存入 secrets.yaml。
func EnsureDefaultFiles(configPath, secretsPath, rulesPath, categoriesPath string) error {
	if dir := filepath.Dir(configPath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		def := Default()
		// 首次生成：随机初始密码打印在启动日志，密码只写入 secrets.yaml（哈希优先，避免空密码锁死）
		plain := randomPassword(10)
		secretsWebUI := map[string]any{
			// /trigger 豁免 JWT 认证，首启即生成密钥，避免默认裸奔
			"webhook_secret": randomPassword(32),
		}
		if hash, err := HashPassword(plain); err == nil {
			secretsWebUI["password_hash"] = hash
		} else {
			log.Printf("初始密码哈希失败（降级为 secrets.yaml 明文存储）：%v", err)
			secretsWebUI["password"] = plain
		}
		data, err := yaml.Marshal(def)
		if err != nil {
			return err
		}
		header := "# Meta2Cloud 主配置（首次启动自动生成；完整可配置项参考 config.example.yaml）\n" +
			"# 登录密码等敏感项保存在同目录 secrets.yaml（权限 0600）；初始密码见启动日志\n"
		if err := os.WriteFile(configPath, append([]byte(header), data...), 0o644); err != nil {
			return err
		}
		if err := SaveSecrets(secretsPath, map[string]any{"webui": secretsWebUI}); err != nil {
			return err
		}
		log.Printf("已生成默认配置 %s，WebUI 初始密码：%s（用户名 %s，登录后请修改）", configPath, plain, def.WebUI.Username)
	}
	if _, err := os.Stat(rulesPath); os.IsNotExist(err) {
		data, err := yaml.Marshal(Default().Parser)
		if err != nil {
			return err
		}
		header := "# 解析规则（首次启动自动生成；custom_words / custom_release_groups 可自行补充）\n"
		if err := os.WriteFile(rulesPath, append([]byte(header), data...), 0o644); err != nil {
			return err
		}
	}
	if _, err := os.Stat(categoriesPath); os.IsNotExist(err) {
		if err := SaveCategories(categoriesPath, DefaultCategories()); err != nil {
			return err
		}
	}
	// 既有安装：webhook 密钥缺失时补生成（首启分支已写入，这里直接跳过）
	EnsureWebhookSecret(configPath, secretsPath)
	return nil
}

// EnsureWebhookSecret 确保存在 /trigger webhook 密钥：主配置与 secrets.yaml 均无
// webui.webhook_secret 键时自动生成写入 secrets.yaml。该接口豁免 JWT 认证，
// 无密钥时任何可达请求都能触发整理（搬动网盘文件、消耗 115 配额）。
// 键存在但为空视为用户主动停用外部触发（服务端仍限制仅本机可触发），不补写。
func EnsureWebhookSecret(configPath, secretsPath string) {
	if hasWebhookSecretKey(configPath) || hasWebhookSecretKey(secretsPath) {
		return
	}
	secrets := map[string]any{}
	if data, err := os.ReadFile(secretsPath); err == nil {
		secrets = unmarshalYAMLMap(data)
	}
	sec, _ := secrets["webui"].(map[string]any)
	if sec == nil {
		sec = map[string]any{}
		secrets["webui"] = sec
	}
	sec["webhook_secret"] = randomPassword(32)
	if err := SaveSecrets(secretsPath, secrets); err != nil {
		log.Printf("webhook 密钥自动生成失败：%v", err)
		return
	}
	log.Printf("已自动生成 /trigger webhook 密钥（webui.webhook_secret → %s）；外部触发需携带请求头 X-Webhook-Secret 或查询参数 secret", secretsPath)
}

// hasWebhookSecretKey 判断 YAML 文件中是否显式存在 webui.webhook_secret 键（空值也算存在）。
func hasWebhookSecretKey(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	sec, _ := unmarshalYAMLMap(data)["webui"].(map[string]any)
	_, ok := sec["webhook_secret"]
	return ok
}

// randomPassword 生成易读随机密码（去除易混字符 0/O/1/l/I）。
func randomPassword(n int) string {
	const charset = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789"
	b := make([]byte, n)
	for i := range b {
		idx, _ := cryptoRandInt(len(charset))
		b[i] = charset[idx]
	}
	return string(b)
}

func cryptoRandInt(max int) (int, error) {
	if max <= 0 {
		return 0, fmt.Errorf("max 必须为正数")
	}
	var buf [1]byte
	for {
		if _, err := rand.Read(buf[:]); err != nil {
			return 0, err
		}
		v := int(buf[0])
		if v < 256-(256%max) {
			return v % max, nil
		}
	}
}

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
		Enabled:      boolean(m, "enabled", false),
		URLBase:      normalizeStrmURLBase(str(m, "url_base", "")),
		URLSecret:    strings.TrimSpace(str(m, "url_secret", "")),
		SyncCron:     cronString(m, "sync_cron", ""),
		RefreshPlex:  boolean(m, "refresh_plex", false),
		MediaExt:     mediaExt,
		FetchSidecar: boolean(m, "fetch_sidecar", true),
		SidecarExt:   sidecarExt,
		Mappings:     mappings,
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// parseTrakt Trakt 同步段解析（方向开关默认开）。
// sync_cron 缺省 */15 * * * *（等价旧默认 900s）；旧 sync_interval_seconds 键静默忽略。
// 注：Trakt 已改为 PKCE 模式、不再签发 Client Secret，故无此配置项；
// 旧配置文件里残留的 client_secret 键会被静默忽略。
func parseTrakt(m map[string]any) Trakt {
	return Trakt{
		Enabled:            boolean(m, "enabled", false),
		ClientID:           strings.TrimSpace(str(m, "client_id", "")),
		TokenJSON:          str(m, "token_json", "config/trakt-token.json"),
		SyncCron:           cronString(m, "sync_cron", "*/15 * * * *"),
		SessionPollSeconds: maxInt(integer(m, "session_poll_seconds", 45), 15),
		SSEEnabled:         boolean(m, "sse_enabled", true),
		SyncToPlex:         boolean(m, "sync_to_plex", true),
		SyncToTrakt:        boolean(m, "sync_to_trakt", true),
	}
}

// parsePlex Plex 连接段解析。
func parsePlex(m map[string]any) Plex {
	return Plex{
		BaseURL:   normalizeStrmURLBase(str(m, "base_url", "")),
		TokenJSON: str(m, "token_json", "config/plex-auth.json"),
		Timeout:   maxInt(integer(m, "timeout", DefaultPlexTimeoutSecs), 1),
	}
}

// parseRss RSS 订阅段解析。
// poll_cron 缺省 */5 * * * *（等价旧默认 300s）；旧 poll_seconds 键静默忽略。
func parseRss(m map[string]any) Rss {
	return Rss{PollCron: cronString(m, "poll_cron", "*/5 * * * *")}
}

// ── 敏感配置（secrets.yaml）────────────────────────────────

// SecretKeys 主配置中的敏感键集合（"section.key" 形式）。
// 这些键的值持久化在 secrets.yaml（0600）而非主配置；读取时 secrets.yaml 优先，
// 主配置中的同名键仅作旧配置兼容（保存时自动搬迁）。
var SecretKeys = map[string]bool{
	"tmdb.api_key":         true,
	"u115.cookie":          true,
	"telegram.bot_token":   true,
	"mediavault.api_key":   true,
	"webui.password":       true, // 旧版明文密码：读取兼容，登录成功后自动迁移为 password_hash
	"webui.password_hash":  true,
	"webui.secret_key":     true,
	"webui.webhook_secret": true,
	"strm.url_secret":      true,
}

// splitKeyPath 拆开 "section.key"。
func splitKeyPath(path string) (section, key string) {
	section, key, _ = strings.Cut(path, ".")
	return
}

// MergeSecrets 将 secrets 中的敏感键覆盖进 dst（secrets.yaml 值优先于主配置）。
func MergeSecrets(dst, secrets map[string]any) {
	for path := range SecretKeys {
		section, key := splitKeyPath(path)
		secMap, ok := secrets[section].(map[string]any)
		if !ok {
			continue
		}
		v, ok := secMap[key]
		if !ok {
			continue
		}
		if dstSec, ok := dst[section].(map[string]any); ok {
			dstSec[key] = v
		} else {
			dst[section] = map[string]any{key: v}
		}
	}
}

// SplitSecrets 从 payload 中摘出敏感键（并从 payload 删除），返回按段组织的敏感 map。
// 调用后 payload 不含敏感键，可直接写主配置文件。
func SplitSecrets(payload map[string]any) map[string]any {
	secrets := map[string]any{}
	for path := range SecretKeys {
		section, key := splitKeyPath(path)
		secMap, ok := payload[section].(map[string]any)
		if !ok {
			continue
		}
		v, ok := secMap[key]
		if !ok {
			continue
		}
		delete(secMap, key)
		dst, _ := secrets[section].(map[string]any)
		if dst == nil {
			dst = map[string]any{}
			secrets[section] = dst
		}
		dst[key] = v
	}
	return secrets
}

// MaskedReadKeys 配置读接口仍掩码的敏感键：仅 webui.secret_key（JWT 签名密钥）。
// 它在前端配置页没有输入项（掩码不影响界面），泄露代价不可逆（可自签任意
// token，修改密码也无法吊销）。其余敏感键（cookie/bot_token/api_key/
// webhook_secret 等）明文回显：管理员需要查看与复制（如 webhook_secret
// 要填进 *arr），属自部署单用户工具的预期可见性。
var MaskedReadKeys = map[string]bool{
	"webui.secret_key": true,
}

// MaskSecretValues 将 m 中已配置（非空字符串）的掩码键值替换为 placeholder，
// 用于配置读接口。
func MaskSecretValues(m map[string]any, placeholder string) {
	for path := range MaskedReadKeys {
		section, key := splitKeyPath(path)
		sec, ok := m[section].(map[string]any)
		if !ok {
			continue
		}
		if v, ok := sec[key].(string); ok && v != "" {
			sec[key] = placeholder
		}
	}
}

// RestoreMaskedSecrets 写接口配套还原：payload 中等于占位符的掩码键（读接口
// 掩码原样回传的场景）替换为 existing 中的现有值，现有值也为空时删除该键。
// 其余值原样保留：空串 = 清除该键，非占位符新值 = 更新。
func RestoreMaskedSecrets(payload, existing map[string]any, placeholder string) {
	for path := range MaskedReadKeys {
		section, key := splitKeyPath(path)
		sec, ok := payload[section].(map[string]any)
		if !ok {
			continue
		}
		v, ok := sec[key].(string)
		if !ok || v != placeholder {
			continue
		}
		existingSec, _ := existing[section].(map[string]any)
		if cur, _ := existingSec[key].(string); cur != "" {
			sec[key] = cur
		} else {
			delete(sec, key)
		}
	}
}

// SaveSecrets 写出敏感配置文件（权限 0600，仅属主可读写）。
func SaveSecrets(path string, m map[string]any) error {
	out, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	header := "# Meta2Cloud 敏感配置（权限 0600；请勿提交到代码仓库或随日志外发）\n" +
		"# 键与主配置一致，由 WebUI 保存时自动维护；此文件中的值优先于主配置\n"
	return os.WriteFile(path, append([]byte(header), out...), 0o600)
}

// HashPassword 生成 bcrypt 哈希（WebUI 登录密码的存储格式）。
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPasswordHash 校验密码与 bcrypt 哈希是否匹配。
func CheckPasswordHash(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
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

// IsTraktReady 是否具备 Trakt 设备码配对条件（Trakt 为 PKCE 模式，仅需 Client ID）。
func (c *Config) IsTraktReady() bool { return c.Trakt.ClientID != "" }

// IsPlexReady 是否具备 Plex 连接条件（服务器地址已填写）。
func (c *Config) IsPlexReady() bool { return c.Plex.BaseURL != "" }

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
