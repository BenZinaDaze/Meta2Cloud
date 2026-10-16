// runtime.go —— WebUI 运行时：配置/Provider/存储单例与缓存失效（对应 webui/core/runtime.py）。
package webui

import (
	"crypto/subtle"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"meta2cloud/internal/cloudfile"
	"meta2cloud/internal/config"
	"meta2cloud/internal/storage"
	"meta2cloud/internal/tmdb"
	"meta2cloud/internal/u115"
	"meta2cloud/internal/webui/store"
)

// Runtime 全局运行时状态。
type Runtime struct {
	RootDir string

	mu         sync.Mutex
	cfg        *config.Config
	configPath string
	rulesPath  string
	configSig  [2]int64 // (config mtime, rules mtime)
	library    *store.LibraryStore
	tmdbCache  *store.TmdbCache
	provider   cloudfile.Provider
	u115Client *u115.Client
	tmdbClient *tmdb.Client
	ingest     *store.IngestStore
	logStore   *store.LogStore
	rss        *store.RSSStore
	u115Auto   *store.U115AutoStore
	strm       *store.StrmStore
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
	defaultRuntime.configSig = [2]int64{}
	return defaultRuntime
}

func fileMtime(path string) int64 {
	if st, err := os.Stat(path); err == nil {
		return st.ModTime().UnixNano()
	}
	return 0
}

// invalidateIfChanged 配置文件变更时重载缓存（需已持锁）。
func (rt *Runtime) invalidateIfChanged() {
	sig := [2]int64{fileMtime(rt.configPath), fileMtime(rt.rulesPath)}
	if rt.configSig != sig {
		log.Printf("检测到配置文件已变更，重载配置与存储 Provider 缓存")
		rt.cfg = nil
		rt.provider = nil
		rt.u115Client = nil
		rt.tmdbClient = nil
		rt.configSig = sig
	}
}

// getConfigLocked 当前配置（需已持锁；sync.Mutex 不可重入，内部方法禁止再取锁）。
func (rt *Runtime) getConfigLocked() *config.Config {
	rt.invalidateIfChanged()
	if rt.cfg == nil {
		cfg, err := config.Load(rt.configPath)
		if err != nil {
			log.Printf("配置加载失败：%v，使用默认配置", err)
			cfg = config.Default()
		}
		rt.cfg = cfg
		rt.configSig = [2]int64{fileMtime(rt.configPath), fileMtime(rt.rulesPath)}
	}
	return rt.cfg
}

// GetConfig 当前配置（带 mtime 缓存失效）。
func (rt *Runtime) GetConfig() *config.Config {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.getConfigLocked()
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
			p.LookupRoots = func() []string {
				c := RT().GetConfig()
				return []string{
					c.ActiveRootFolderID(), c.ActiveMovieRootID(),
					c.ActiveTvRootID(), c.ActiveScanFolderID(),
					c.U115.RootFolderID, c.U115.MovieRootID, c.U115.TvRootID, c.U115.DownloadFolderID,
				}
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

// CredentialsMatch 校验用户名密码（恒定时间比较）。
func (rt *Runtime) CredentialsMatch(username, password string) bool {
	cfg := rt.GetConfig()
	cfgUser := cfg.WebUI.Username
	if cfgUser == "" {
		cfgUser = "admin"
	}
	cfgPass := cfg.WebUI.Password
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(cfgUser)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(password), []byte(cfgPass)) == 1
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
