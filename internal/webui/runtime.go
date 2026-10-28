// runtime.go —— WebUI 运行时：配置/Provider/存储单例与缓存失效（对应 webui/core/runtime.py）。
package webui

import (
	"crypto/subtle"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"meta2cloud/internal/cloudfile"
	"meta2cloud/internal/config"
	"meta2cloud/internal/plexserver"
	"meta2cloud/internal/storage"
	"meta2cloud/internal/tmdb"
	"meta2cloud/internal/trakt"
	"meta2cloud/internal/u115"
	"meta2cloud/internal/webui/store"
)

// appliedScheduleGen 已应用过调度启停的配置代数（ApplyWatchSchedules 幂等去重）。
var appliedScheduleGen atomic.Int64

// Runtime 全局运行时状态。
type Runtime struct {
	RootDir string

	mu          sync.Mutex
	cfg         *config.Config
	configPath  string
	rulesPath   string
	secretsPath string
	configSig   [3]int64 // (config mtime, rules mtime, secrets mtime)
	scheduleGen int64    // 配置变更代数（每次重载递增，驱动 cron 调度启停）
	categories  *config.CategoriesConfig
	categoriesM int64 // categories.yaml mtime
	library     *store.LibraryStore
	tmdbCache   *store.TmdbCache
	provider    cloudfile.Provider
	u115Client  *u115.Client
	tmdbClient  *tmdb.Client
	ingest      *store.IngestStore
	logStore    *store.LogStore
	rss         *store.RSSStore
	u115Auto    *store.U115AutoStore
	strm        *store.StrmStore
	traktClient *trakt.Client
	plexClient  *plexserver.Client
	traktSync   *store.TraktSyncStore
}

var defaultRuntime = &Runtime{}

// RT 全局运行时。
func RT() *Runtime { return defaultRuntime }

// InitRuntime 初始化运行时（rootDir 为空时取当前目录）。
// 重置全部缓存状态（配置/Provider/存储单例）。
func InitRuntime(rootDir string) *Runtime {
	if rootDir == "" {
		if exe, err := os.Executable(); err == nil {
			rootDir = filepath.Dir(exe)
		} else {
			rootDir, _ = os.Getwd()
		}
	}
	defaultRuntime.mu.Lock()
	defer defaultRuntime.mu.Unlock()
	defaultRuntime.RootDir = rootDir
	defaultRuntime.configPath = filepath.Join(rootDir, "config", "config.yaml")
	defaultRuntime.rulesPath = filepath.Join(rootDir, "config", "parser-rules.yaml")
	defaultRuntime.secretsPath = filepath.Join(rootDir, "config", config.SecretsFilename)
	// 首次启动友好：缺失的配置文件自动生成默认内容（已存在的不覆盖）
	if err := config.EnsureDefaultFiles(
		defaultRuntime.configPath,
		defaultRuntime.secretsPath,
		defaultRuntime.rulesPath,
		filepath.Join(rootDir, "config", config.CategoriesFilename),
	); err != nil {
		log.Printf("默认配置文件生成失败：%v", err)
	}
	defaultRuntime.cfg = nil
	defaultRuntime.provider = nil
	defaultRuntime.u115Client = nil
	defaultRuntime.tmdbClient = nil
	defaultRuntime.library = nil
	defaultRuntime.tmdbCache = nil
	defaultRuntime.ingest = nil
	defaultRuntime.logStore = nil
	defaultRuntime.rss = nil
	defaultRuntime.u115Auto = nil
	defaultRuntime.strm = nil
	defaultRuntime.traktClient = nil
	defaultRuntime.plexClient = nil
	defaultRuntime.traktSync = nil
	defaultRuntime.categories = nil
	defaultRuntime.categoriesM = 0
	defaultRuntime.configSig = [3]int64{}
	return defaultRuntime
}

func fileMtime(path string) int64 {
	if st, err := os.Stat(path); err == nil {
		return st.ModTime().UnixNano()
	}
	return 0
}

// configSignature 配置三件套（主配置/解析规则/敏感配置）的 mtime 签名。
func (rt *Runtime) configSignature() [3]int64 {
	return [3]int64{fileMtime(rt.configPath), fileMtime(rt.rulesPath), fileMtime(rt.secretsPath)}
}

// invalidateIfChanged 配置文件变更时重载缓存（需已持锁）。
// 返回是否发生了重载（含首次加载）。
func (rt *Runtime) invalidateIfChanged() bool {
	sig := rt.configSignature()
	if rt.configSig == sig {
		return false
	}
	log.Printf("检测到配置文件已变更，重载配置与存储 Provider 缓存")
	rt.cfg = nil
	rt.provider = nil
	rt.u115Client = nil
	rt.tmdbClient = nil
	rt.traktClient = nil
	rt.plexClient = nil
	rt.configSig = sig
	rt.scheduleGen++
	return true
}

// getConfigLocked 当前配置（需已持锁；sync.Mutex 不可重入，内部方法禁止再取锁）。
func (rt *Runtime) getConfigLocked() *config.Config {
	if rt.invalidateIfChanged() {
		rt.scheduleGen++
	}
	if rt.cfg == nil {
		cfg, err := config.Load(rt.configPath)
		if err != nil {
			log.Printf("配置加载失败：%v，使用默认配置", err)
			cfg = config.Default()
		}
		rt.cfg = cfg
		rt.configSig = rt.configSignature()
	}
	return rt.cfg
}

// GetConfig 当前配置（带 mtime 缓存失效）。检测到配置变更时同步启停
// 各 cron 调度（开关开启才存在轮询循环，见 ApplyWatchSchedules）。
func (rt *Runtime) GetConfig() *config.Config {
	rt.mu.Lock()
	rt.getConfigLocked()
	gen := rt.scheduleGen
	cfg := rt.cfg
	rt.mu.Unlock()
	if gen != appliedScheduleGen.Load() {
		// 先记代数再应用：Apply 内部会读配置（如订阅 cron），重入时据此返回，
		// 避免无限递归；并发双跑无害（调度启停幂等且有互斥）
		appliedScheduleGen.Store(gen)
		ApplyWatchSchedules(cfg)
	}
	return cfg
}

func (rt *Runtime) categoriesPath() string {
	return filepath.Join(filepath.Dir(rt.configPath), config.CategoriesFilename)
}

// GetCategories 分类配置单例（categories.yaml 变更自动重载；
// 文件不存在时使用默认分类——分类布局始终启用）。
func (rt *Runtime) GetCategories() *config.CategoriesConfig {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if m := fileMtime(rt.categoriesPath()); m != rt.categoriesM {
		rt.categories = nil
		rt.categoriesM = m
	}
	if rt.categories == nil {
		if c, err := config.LoadCategories(rt.categoriesPath()); err == nil {
			rt.categories = c
		} else {
			if !os.IsNotExist(err) {
				log.Printf("分类配置加载失败（使用默认分类）：%v", err)
			}
			rt.categories = config.DefaultCategories()
		}
	}
	return rt.categories
}

// SetCategories 保存分类配置并刷新单例。
func (rt *Runtime) SetCategories(c *config.CategoriesConfig) error {
	if err := config.SaveCategories(rt.categoriesPath(), c); err != nil {
		return err
	}
	rt.mu.Lock()
	rt.categories = c
	rt.categoriesM = fileMtime(rt.categoriesPath())
	rt.mu.Unlock()
	return nil
}

// getLibraryStoreLocked 媒体实体库单例（需已持锁）。
func (rt *Runtime) getLibraryStoreLocked() *store.LibraryStore {
	if rt.library == nil {
		dbPath := filepath.Join(rt.RootDir, "config", "data", "library.db")
		s, err := store.OpenLibraryStore(dbPath)
		if err != nil {
			log.Printf("媒体实体库初始化失败：%v", err)
			return nil
		}
		rt.library = s
	}
	rt.library.ImageBaseURL = rt.getConfigLocked().TmdbImageBaseURL
	return rt.library
}

// GetLibraryStore 媒体实体库单例。
func (rt *Runtime) GetLibraryStore() *store.LibraryStore {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.getLibraryStoreLocked()
}

// getTmdbCacheLocked TMDB 缓存单例（需已持锁）。
func (rt *Runtime) getTmdbCacheLocked() *store.TmdbCache {
	if rt.tmdbCache == nil {
		rt.tmdbCache = store.NewTmdbCache(rt.getLibraryStoreLocked())
	}
	return rt.tmdbCache
}

// GetTmdbCache TMDB 缓存单例。
func (rt *Runtime) GetTmdbCache() *store.TmdbCache {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.getTmdbCacheLocked()
}

// getU115ClientLocked 115 客户端（需已持锁）。
func (rt *Runtime) getU115ClientLocked() *u115.Client {
	rt.invalidateIfChanged()
	if rt.u115Client == nil {
		cfg := rt.getConfigLocked()
		tokenPath := cfg.U115.TokenJSON
		if !filepath.IsAbs(tokenPath) {
			tokenPath = filepath.Join(rt.RootDir, tokenPath)
		}
		client, err := u115.NewClientFromTokenFile(cfg.U115.ClientID, tokenPath, u115.WithQPS(float64(cfg.U115.QPS)))
		if err != nil {
			log.Printf("115 客户端初始化失败：%v", err)
			return nil
		}
		if cfg.U115.Cookie != "" {
			client.SetCookie(cfg.U115.Cookie)
		}
		rt.u115Client = client
	}
	return rt.u115Client
}

// GetU115Client 115 客户端单例（配置变更时重建）。
func (rt *Runtime) GetU115Client() *u115.Client {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.getU115ClientLocked()
}

// GetStorageProvider 存储 Provider 单例。
func (rt *Runtime) GetStorageProvider() cloudfile.Provider {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.invalidateIfChanged()
	if rt.provider == nil {
		cfg := rt.getConfigLocked()
		if cfg.ActiveStorageName() == "pan115" {
			client := rt.getU115ClientLocked()
			if client == nil {
				return nil
			}
			p := storage.NewPan115ProviderWithGetter(func() *u115.Client {
				rt.mu.Lock()
				defer rt.mu.Unlock()
				return rt.getU115ClientLocked()
			})
			// 根目录解析缓存（随 Provider 重建而失效）：GetFile 每次都要取根目录，
			// 短 TTL 缓存避免重复解析消耗 QPS。
			type rootResolveEntry struct {
				id string
				at time.Time
			}
			var (
				rootCacheMu sync.Mutex
				rootCache   = map[string]rootResolveEntry{}
			)
			p.LookupRoots = func() []string {
				// 配置值为目录名/相对路径（兼容旧 ID），查询时解析为目录 ID；
				// 延迟到 GetFile 时执行，此时 Provider 已初始化完毕。
				// 只能走纯路径解析：回退 GetFile 会在 GetFile → managedLookupRoots
				// → 本闭包间成环，名字解析失败（如 115 冷却）时无限递归栈溢出。
				c := RT().GetConfig()
				refs := []string{
					c.ActiveRootFolderID(), c.ActiveScanFolderID(),
					c.U115.RootFolderID, c.U115.DownloadFolderID,
				}
				rootCacheMu.Lock()
				defer rootCacheMu.Unlock()
				now := time.Now()
				ids := make([]string, 0, len(refs))
				for _, ref := range refs {
					if e, ok := rootCache[ref]; ok && now.Sub(e.at) < time.Minute {
						ids = append(ids, e.id)
						continue
					}
					id := resolveFolderRefPathOnly(p, ref)
					rootCache[ref] = rootResolveEntry{id: id, at: now}
					ids = append(ids, id)
				}
				return ids
			}
			rt.provider = p
		} else {
			p, err := storage.GetProvider(cfg.ActiveStorageName(), cfg)
			if err != nil {
				log.Printf("初始化存储 Provider 失败：%v", err)
				return nil
			}
			rt.provider = p
		}
	}
	return rt.provider
}

// GetTmdbClient TMDB 客户端单例。
func (rt *Runtime) GetTmdbClient() *tmdb.Client {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.invalidateIfChanged()
	if rt.tmdbClient == nil {
		cfg := rt.getConfigLocked()
		if !cfg.IsTmdbReady() {
			return nil
		}
		rt.tmdbClient = tmdb.NewClient(cfg.Tmdb.APIKey, cfg.Tmdb.Language, cfg.Tmdb.Proxy, cfg.Tmdb.Timeout, rt.getTmdbCacheLocked())
	}
	return rt.tmdbClient
}

// DataDir 数据目录（config/data）。
func (rt *Runtime) DataDir() string {
	return filepath.Join(rt.RootDir, "config", "data")
}

// CredentialsMatch 校验用户名密码：优先 bcrypt 哈希比对；
// 旧版明文密码恒定时间比对，成功后自动迁移为哈希（迁移失败不影响本次登录）。
func (rt *Runtime) CredentialsMatch(username, password string) bool {
	cfg := rt.GetConfig()
	cfgUser := cfg.WebUI.Username
	if cfgUser == "" {
		cfgUser = "admin"
	}
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(cfgUser)) == 1
	if cfg.WebUI.PasswordHash != "" {
		return userOK && config.CheckPasswordHash(cfg.WebUI.PasswordHash, password)
	}
	passOK := cfg.WebUI.Password != "" && subtle.ConstantTimeCompare([]byte(password), []byte(cfg.WebUI.Password)) == 1
	if userOK && passOK {
		migratePlaintextPassword(password)
	}
	return userOK && passOK
}

// 定期清理 TMDB 过期缓存（后台任务，由 Server 启动）。
func (rt *Runtime) startTmdbEvictLoop(stop <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if cache := rt.GetTmdbCache(); cache != nil {
					if n := cache.EvictExpired(); n > 0 {
						log.Printf("已清理 %d 条过期 TMDB 缓存", n)
					}
				}
			case <-stop:
				return
			}
		}
	}()
}

// GetIngestStore 入库历史存储单例（与 library.db 同库）。
func (rt *Runtime) GetIngestStore() *store.IngestStore {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.ingest == nil {
		dbPath := filepath.Join(rt.RootDir, "config", "data", "library.db")
		s, err := store.OpenIngestStore(dbPath)
		if err != nil {
			log.Printf("入库历史存储初始化失败：%v", err)
			return nil
		}
		rt.ingest = s
	}
	return rt.ingest
}

// GetLogStore 应用日志存储单例。
func (rt *Runtime) GetLogStore() *store.LogStore {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.logStore == nil {
		legacyJSONL := filepath.Join(rt.DataDir(), "app_logs.jsonl") // Python 时代旧日志一次性迁移
		s, err := store.OpenLogStore(rt.DataDir(), rt.getConfigLocked().WebUI.LogRetentionDays, legacyJSONL)
		if err != nil {
			return nil
		}
		rt.logStore = s
	}
	return rt.logStore
}

// GetRSSStore RSS 订阅存储单例（与 library.db 同库）。
func (rt *Runtime) GetRSSStore() *store.RSSStore {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.rss == nil {
		dbPath := filepath.Join(rt.RootDir, "config", "data", "library.db")
		s, err := store.OpenRSSStore(dbPath, dbPath)
		if err != nil {
			log.Printf("RSS 订阅存储初始化失败：%v", err)
			return nil
		}
		cfg := rt.getConfigLocked()
		s.ImageBaseURL = cfg.TmdbImageBaseURL
		rt.rss = s
	}
	return rt.rss
}

// GetU115AutoStore 115 自动整理状态存储单例。
func (rt *Runtime) GetU115AutoStore() *store.U115AutoStore {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.u115Auto == nil {
		dbPath := filepath.Join(rt.RootDir, "config", "data", "u115_auto_organize.db")
		s, err := store.OpenU115AutoStore(dbPath)
		if err != nil {
			return nil
		}
		rt.u115Auto = s
	}
	return rt.u115Auto
}

// GetStrmStore STRM 存储单例（独立 strm.db，不影响 library.db）。
func (rt *Runtime) GetStrmStore() *store.StrmStore {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.strm == nil {
		dbPath := filepath.Join(rt.RootDir, "config", "data", "strm.db")
		s, err := store.OpenStrmStore(dbPath)
		if err != nil {
			log.Printf("STRM 存储初始化失败：%v", err)
			return nil
		}
		rt.strm = s
	}
	return rt.strm
}

// getTraktClientLocked Trakt 客户端单例（需已持锁；token 持久化在 config/trakt-token.json）。
func (rt *Runtime) getTraktClientLocked() *trakt.Client {
	rt.invalidateIfChanged()
	if rt.traktClient == nil {
		cfg := rt.getConfigLocked()
		if !cfg.IsTraktReady() {
			return nil
		}
		tokenPath := cfg.Trakt.TokenJSON
		if !filepath.IsAbs(tokenPath) {
			tokenPath = filepath.Join(rt.RootDir, tokenPath)
		}
		client, err := trakt.NewClientFromTokenFile(cfg.Trakt.ClientID, tokenPath)
		if err != nil {
			log.Printf("Trakt 客户端初始化失败：%v", err)
			return nil
		}
		rt.traktClient = client
	}
	return rt.traktClient
}

// GetTraktClient Trakt 客户端单例（配置变更时重建）。
func (rt *Runtime) GetTraktClient() *trakt.Client {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.getTraktClientLocked()
}

// getPlexClientLocked Plex 客户端单例（需已持锁）。
// token 与 client_identifier 持久化在 config/plex-auth.json（配对态）。
func (rt *Runtime) getPlexClientLocked() *plexserver.Client {
	rt.invalidateIfChanged()
	if rt.plexClient == nil {
		cfg := rt.getConfigLocked()
		if !cfg.IsPlexReady() {
			return nil
		}
		auth := ensurePlexAuthLocked(rt)
		if auth == nil {
			return nil
		}
		timeout := cfg.Plex.Timeout
		if timeout <= 0 {
			timeout = 10
		}
		rt.plexClient = plexserver.NewClient(cfg.Plex.BaseURL, auth.AuthToken, auth.ClientIdentifier,
			plexserver.WithTimeout(timeout))
	}
	return rt.plexClient
}

// GetPlexClient Plex 客户端单例。
func (rt *Runtime) GetPlexClient() *plexserver.Client {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.getPlexClientLocked()
}

// GetTraktSyncStore 观看同步同步状态存储单例（独立 traktsync.db）。
func (rt *Runtime) GetTraktSyncStore() *store.TraktSyncStore {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.traktSync == nil {
		dbPath := filepath.Join(rt.RootDir, "config", "data", "traktsync.db")
		s, err := store.OpenTraktSyncStore(dbPath)
		if err != nil {
			log.Printf("观看同步存储初始化失败：%v", err)
			return nil
		}
		rt.traktSync = s
	}
	return rt.traktSync
}

// SetProviderForTest 注入测试 Provider（绕过配置构造）。
// 先预热配置，避免后续首次 GetConfig 触发失效逻辑清掉注入的 Provider。
func (rt *Runtime) SetProviderForTest(p cloudfile.Provider) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	_ = rt.getConfigLocked()
	rt.provider = p
}

// SetU115ClientForTest 注入测试 115 客户端。
func (rt *Runtime) SetU115ClientForTest(c *u115.Client) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	_ = rt.getConfigLocked()
	rt.u115Client = c
}

// SetTmdbClientForTest 注入测试 TMDB 客户端。
func (rt *Runtime) SetTmdbClientForTest(c *tmdb.Client) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	_ = rt.getConfigLocked()
	rt.tmdbClient = c
}
