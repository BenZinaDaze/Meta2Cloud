// tmdbcache.go —— 基于 tmdb_media 实体表的 TMDB 缓存适配层（对应 webui/tmdb_cache.py）。
//
// 对外实现 tmdb.Cache 接口（get/is_failed/set/set_failed），
// 底层不再以请求 path 作为主键，而是回写到 tmdb_media 实体表。
package store

import (
	"log"
	"strconv"
	"strings"
	"time"
)

// 按 path 前缀选择 TTL（秒）。
var defaultTTL = map[string]float64{
	"movie":   7 * 86400,
	"tv":      6 * 86400,
	"season":  6 * 86400,
	"episode": 6 * 86400,
	"default": 1 * 86400,
}

func pickTTL(path string) float64 {
	for key, ttl := range defaultTTL {
		if key == "default" {
			continue
		}
		if strings.Contains(path, "/"+key) {
			return ttl
		}
	}
	return defaultTTL["default"]
}

// parsedPath 解析后的 TMDB 资源路径。
type parsedPath struct {
	mediaType     string
	tmdbID        int64
	seasonNumber  *int
	episodeNumber *int
	resourceType  string // detail | season | episode
}

func parseTmdbPath(path string) *parsedPath {
	normalized := strings.SplitN(path, "?", 2)[0]
	var parts []string
	for _, part := range strings.Split(strings.Trim(normalized, "/"), "/") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) < 2 {
		return nil
	}
	if parts[0] != "movie" && parts[0] != "tv" {
		return nil
	}
	tmdbID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil
	}
	parsed := &parsedPath{mediaType: parts[0], tmdbID: tmdbID, resourceType: "detail"}
	if len(parts) >= 4 && parts[2] == "season" {
		n, err := strconv.Atoi(parts[3])
		if err != nil {
			return nil
		}
		parsed.seasonNumber = &n
		parsed.resourceType = "season"
	}
	if len(parts) >= 6 && parts[4] == "episode" {
		n, err := strconv.Atoi(parts[5])
		if err != nil {
			return nil
		}
		parsed.episodeNumber = &n
		parsed.resourceType = "episode"
	}
	return parsed
}

// TmdbCache tmdb.Cache 的实体缓存实现。
type TmdbCache struct {
	store *LibraryStore
}

// NewTmdbCache 构造。
func NewTmdbCache(store *LibraryStore) *TmdbCache { return &TmdbCache{store: store} }

// Get 读取缓存；路径不合法/语言不符/已过期返回未命中。
func (c *TmdbCache) Get(path, language string) (map[string]any, bool) {
	parsed := parseTmdbPath(path)
	if parsed == nil {
		return nil, false
	}
	entry := c.store.GetTmdbEntry(parsed.mediaType, parsed.tmdbID)
	if entry == nil {
		return nil, false
	}
	if language != "" && entry.Language != "" && entry.Language != language {
		return nil, false
	}
	if float64(time.Now().Unix()) > entry.ExpiresAt {
		return nil, false
	}
	switch parsed.resourceType {
	case "detail":
		if len(entry.RawJSON) == 0 {
			return nil, false
		}
		return entry.RawJSON, true
	case "season":
		if parsed.seasonNumber != nil {
			if v, ok := entry.SeasonDetailsJSON[strconv.Itoa(*parsed.seasonNumber)]; ok {
				if d, ok := v.(map[string]any); ok {
					return d, true
				}
			}
		}
		return nil, false
	default: // episode
		if parsed.seasonNumber != nil && parsed.episodeNumber != nil {
			key := strconv.Itoa(*parsed.seasonNumber) + ":" + strconv.Itoa(*parsed.episodeNumber)
			if v, ok := entry.EpisodeDetails[key]; ok {
				if d, ok := v.(map[string]any); ok {
					return d, true
				}
			}
		}
		return nil, false
	}
}

// IsFailed 负缓存（实体缓存模式恒为 false）。
func (c *TmdbCache) IsFailed(path, language string) bool { return false }

// Set 写入缓存。
func (c *TmdbCache) Set(path string, data map[string]any, language string) {
	parsed := parseTmdbPath(path)
	if parsed == nil || data == nil {
		return
	}
	expiresAt := float64(time.Now().Unix()) + pickTTL(path)
	var err error
	switch parsed.resourceType {
	case "detail":
		err = c.store.UpsertTmdbDetail(parsed.mediaType, parsed.tmdbID, language, data, expiresAt)
	case "season":
		err = c.store.UpsertTmdbSeasonDetail(parsed.tmdbID, language, *parsed.seasonNumber, data, expiresAt)
	default:
		err = c.store.UpsertTmdbEpisodeDetail(parsed.tmdbID, language, *parsed.seasonNumber, *parsed.episodeNumber, data, expiresAt)
	}
	if err != nil {
		log.Printf("写入 TMDB 实体缓存失败 [%s]: %v", path, err)
	}
}

// SetFailed 失败冷却（实体缓存模式忽略）。
func (c *TmdbCache) SetFailed(path, language string) {
	log.Printf("TMDB 失败冷却已忽略（实体缓存模式）：%s", path)
}

// EvictExpired 清理过期条目。
func (c *TmdbCache) EvictExpired() int64 {
	n, err := c.store.EvictTmdbExpired()
	if err != nil {
		return 0
	}
	return n
}

// Stats 缓存统计。
func (c *TmdbCache) Stats() map[string]int64 { return c.store.TmdbStats() }

// 编译期确认实现 tmdb.Cache。
