// source.go —— PlexSource：把 Plex 客户端适配成 watched.Provider，
// 并完成 Plex 条目 → TMDB 身份的解析（身份链见 resolveMetadata 注释）。
package plexserver

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"panoply/internal/watched"
)

// Source watched.Provider 的 Plex 实现。
//
// 注意：Mark* 依赖同一进程内最近一次 ListAll 建立的 MediaKey → ratingKey 索引
// （同步引擎每轮先枚举后写入，天然满足）；跨轮直接调用 Mark* 会因索引缺失失败。
type Source struct {
	Client *Client
	// Titles 标题反查（panoply 自有 media_title_pins/library_media 的只读视图），可为 nil。
	Titles watched.TitleIndex
	// Mapper 外部 ID 归一化（tmdb/tvdb/imdb → 规范键），由宿主注入持久化实现。
	Mapper watched.IDMapper

	mu          sync.Mutex
	refIndex    map[watched.MediaKey][]string // MediaKey → ratingKeys（多版本全量保留）
	unresolved  []watched.UnresolvedItem
	showCache   map[string]*Metadata // grandparentRatingKey → 剧集条目（单轮缓存）
	showCacheMu sync.Mutex
}

// NewSource 构造。mapper 为 nil 时使用内存映射（仅身份不可持久化，功能不受影响）。
func NewSource(client *Client, titles watched.TitleIndex, mapper watched.IDMapper) *Source {
	if mapper == nil {
		mapper = watched.NewMemIDMapper()
	}
	return &Source{
		Client: client, Titles: titles, Mapper: mapper,
		refIndex:  map[watched.MediaKey][]string{},
		showCache: map[string]*Metadata{}, // 会话路径不经 ListAll，必须随构造初始化
	}
}

// Name 实现 watched.Provider。
func (s *Source) Name() string { return "plex" }

// RefsOf 返回 MediaKey 对应的全部 ratingKey。
func (s *Source) RefsOf(key watched.MediaKey) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.refIndex[key]...)
}

// DrainUnresolved 实现 watched.UnresolvedReporter。
func (s *Source) DrainUnresolved() []watched.UnresolvedItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.unresolved
	s.unresolved = nil
	return out
}

// ListAll 枚举全部电影分区（type=1）与剧集分区（type=4 集叶子）。
func (s *Source) ListAll(ctx context.Context) (map[watched.MediaKey]watched.ItemState, error) {
	s.mu.Lock()
	s.refIndex = map[watched.MediaKey][]string{}
	s.unresolved = nil
	s.mu.Unlock()
	s.showCacheMu.Lock()
	s.showCache = map[string]*Metadata{}
	s.showCacheMu.Unlock()

	sections, err := s.Client.Sections()
	if err != nil {
		return nil, fmt.Errorf("枚举 Plex 分区失败：%w", err)
	}
	out := map[watched.MediaKey]watched.ItemState{}
	for _, section := range sections {
		plexType := 1
		if section.Type == "show" {
			plexType = 4
		}
		items, err := s.Client.LibraryItems(section.Key, plexType)
		if err != nil {
			return nil, fmt.Errorf("枚举 Plex 分区 %q 失败：%w", section.Title, err)
		}
		for i := range items {
			s.consumeItem(&items[i], section.Type, out)
		}
	}
	return out, nil
}

// consumeItem 处理单条 Plex 条目：解析身份 → 聚合进结果与 ratingKey 索引。
func (s *Source) consumeItem(item *Metadata, sectionType string, out map[watched.MediaKey]watched.ItemState) {
	watchedState := item.ViewCount > 0
	lastAt := int64(item.LastViewedAt)
	key, ok := s.resolveMetadata(item, sectionType)
	if !ok {
		s.mu.Lock()
		s.unresolved = append(s.unresolved, watched.UnresolvedItem{
			Ref: item.RatingKey, Title: displayTitle(item), Reason: "无法解析 TMDB 身份",
		})
		s.mu.Unlock()
		return
	}

	s.mu.Lock()
	s.refIndex[key] = append(s.refIndex[key], item.RatingKey)
	s.mu.Unlock()

	// 多版本/多文件聚合：Watched=OR、LastAt=max、Plays=max（求和会重复计数）
	current, exists := out[key]
	if !exists {
		out[key] = watched.ItemState{
			Present: true, Watched: watchedState, LastAt: lastAt, Plays: item.ViewCount, Ref: item.RatingKey,
		}
		return
	}
	current.Present = true
	current.Watched = current.Watched || watchedState
	if lastAt > current.LastAt {
		current.LastAt = lastAt
	}
	if item.ViewCount > current.Plays {
		current.Plays = item.ViewCount
	}
	out[key] = current
}

func displayTitle(item *Metadata) string {
	if item.GrandparentTitle != "" && item.Type == "episode" {
		return fmt.Sprintf("%s S%02dE%02d「%s」", item.GrandparentTitle, item.ParentIndex, item.Index, item.Title)
	}
	return item.Title
}

// ── 身份解析 ────────────────────────────────────────────────

var (
	seasonSegmentRe = regexp.MustCompile(`(?i)^season[ _\-.]?(\d{1,2})$`)
	titleYearRe     = regexp.MustCompile(`^(.+?)\s*\((\d{4})\)$`)
	// maxWalkSegments 从文件名向上寻找 Title (Year) 目录段的最大步数。
	maxWalkSegments = 4
)

// resolveMetadata 身份解析链（媒体类型取自 Plex 分区/条目类型，不猜）：
//  1. 电影：条目自身 guid 集合（tmdb/tvdb/imdb）归一化成键
//  2. 文件路径右向行走：跳过 Season NN 段后，取顶层 `Title (Year)` 目录名
//     → TitleIndex 反查（对剧集即剧身份；季集号直接用 Plex 的 parentIndex/index）
//  3. 剧集兜底：拉取上级剧集条目（grandparentRatingKey，单轮缓存）走 1/2
//  4. 电影兜底：条目自身的 (title, year) 反查
func (s *Source) resolveMetadata(item *Metadata, sectionType string) (watched.MediaKey, bool) {
	mediaType := "movie"
	if sectionType == "show" || item.Type == "episode" {
		mediaType = "tv"
	}

	// 集条目 Guid 里的 tmdb:// / tvdb:// 是「这一集」在 TMDB/TVDB 的 id
	// （与剧集 id 不同号段，真实案例：剧集本身 tmdb://335790，其 S01E02
	// 集级却是 tmdb://7905973），不能当剧集身份用；电影条目的 guids 即自身
	// 身份。故电影直接归并自身 ID 集；剧集一律从上级剧集条目取权威身份。
	if mediaType == "movie" {
		if key, ok := s.Mapper.ResolveKey(mediaType, s.collectIDs(item), 0, 0); ok {
			return key, true
		}
	}

	// 剧集：上级剧集的权威 Guid（Plex 自身匹配）先于本地路径/标题反查——
	// 自建索引若有脏 id，路径反查会把它当命中并短路后面的正确链路。
	if mediaType == "tv" && item.GrandparentTitle != "" {
		if key, ok := s.resolveShowByGrandparent(item); ok {
			return key, true
		}
	}

	if path := item.FilePath(); path != "" {
		if key, ok := s.resolveByPath(mediaType, path, item); ok {
			return key, true
		}
	}

	if mediaType == "movie" && item.Title != "" && item.Year > 0 {
		if key, ok := s.lookupTitle(mediaType, item.Title, int64(item.Year)); ok {
			return s.buildKey(mediaType, key.TMDBID, item), true
		}
	}
	return watched.MediaKey{}, false
}

// collectIDs 采集条目 guid 列表中的全部已知外部 ID（tmdb/tvdb/imdb）。
func (s *Source) collectIDs(item *Metadata) watched.MediaIDs {
	return watched.MediaIDs{
		TMDB: item.TMDBGuid(),
		TVDB: item.TVDBGuid(),
		IMDB: item.IMDBGuid(),
	}
}

// buildKey 组装 MediaKey；剧集季集号直接取 Plex 字段（S00 特辑是普通键）。
func (s *Source) buildKey(mediaType string, tmdbID int64, item *Metadata) watched.MediaKey {
	key := watched.MediaKey{Type: mediaType, TMDBID: tmdbID}
	if mediaType == "tv" {
		key.Season = item.ParentIndex
		key.Episode = item.Index
	}
	return key
}

// resolveByPath 路径右向行走解析。示例：
//
//	/mnt/115/媒体库/剧集/某剧 (2020)/Season 01/某剧 - S01E02.mkv
func (s *Source) resolveByPath(mediaType, path string, item *Metadata) (watched.MediaKey, bool) {
	normalized := strings.ReplaceAll(path, "\\", "/")
	parts := strings.Split(normalized, "/")
	// 从右向左：跳过文件名（及紧邻的 Season NN 段），在有限步内找 `Title (Year)`
	for i := len(parts) - 2; i >= 0 && i >= len(parts)-2-maxWalkSegments; i-- {
		segment := strings.TrimSpace(parts[i])
		if seasonSegmentRe.MatchString(segment) {
			continue
		}
		title, year, ok := parseTitleYear(segment)
		if !ok {
			continue
		}
		if key, ok := s.lookupTitle(mediaType, title, year); ok {
			return s.buildKey(mediaType, key.TMDBID, item), true
		}
		// 目录段符合 Title (Year) 形态但反查未中：不再继续向上（更上层是分类目录）
		break
	}
	return watched.MediaKey{}, false
}

func parseTitleYear(segment string) (title string, year int64, ok bool) {
	m := titleYearRe.FindStringSubmatch(strings.TrimSpace(segment))
	if m == nil {
		return "", 0, false
	}
	y, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil || y < 1900 || y > 2100 {
		return "", 0, false
	}
	return strings.TrimSpace(m[1]), y, true
}

func (s *Source) lookupTitle(mediaType, title string, year int64) (watched.MediaKey, bool) {
	if s.Titles == nil {
		return watched.MediaKey{}, false
	}
	return s.Titles.Lookup(mediaType, title, year)
}

// resolveShowByGrandparent 拉取上级剧集条目做身份解析（单轮缓存避免重复请求）。
func (s *Source) resolveShowByGrandparent(item *Metadata) (watched.MediaKey, bool) {
	// 集条目没有直接的 grandparentRatingKey 字段暴露在这里时，从元数据接口补查
	show, err := s.cachedShow(item)
	if err != nil || show == nil {
		return watched.MediaKey{}, false
	}
	// 剧集条目 guids 为权威身份集（tmdb 优先，缺失时 tvdb 锚）
	if key, ok := s.Mapper.ResolveKey("tv", s.collectIDs(show), item.ParentIndex, item.Index); ok {
		return key, true
	}
	if path := show.FilePath(); path != "" {
		return s.resolveByPath("tv", path, item)
	}
	return watched.MediaKey{}, false
}

func (s *Source) cachedShow(item *Metadata) (*Metadata, error) {
	// 上级剧集定位：详情接口（会话轮询/SSE 路径）的集条目自带 grandparentRatingKey，
	// 直接使用；枚举视图缺失该字段时从 Key 路径推导兜底——
	// Key 形如 /library/metadata/{showRatingKey}/children/{seasonRatingKey}/{episodeRatingKey}。
	// 注意详情接口的 Key 是集自身（/library/metadata/{ep}），不能参与推导。
	showRef := item.GrandparentRatingKey
	if showRef == "" {
		showRef = grandparentRef(item.Key)
	}
	if showRef == "" {
		return nil, fmt.Errorf("无法确定上级剧集 ratingKey")
	}
	s.showCacheMu.Lock()
	defer s.showCacheMu.Unlock()
	if s.showCache == nil {
		s.showCache = map[string]*Metadata{}
	}
	if show, ok := s.showCache[showRef]; ok {
		return show, nil
	}
	show, err := s.Client.Metadata(showRef)
	if err != nil {
		return nil, err
	}
	s.showCache[showRef] = show
	return show, nil
}

// grandparentRef 从条目 Key 路径提取剧集 ratingKey。
func grandparentRef(key string) string {
	// /library/metadata/{show}/children/...
	parts := strings.Split(strings.Trim(key, "/"), "/")
	if len(parts) >= 3 && parts[0] == "library" && parts[1] == "metadata" {
		return parts[2]
	}
	return ""
}

// ── watched.Provider 写入 ───────────────────────────────────

// MarkWatched 批量 scrobble（按最近一次 ListAll 的索引定位 ratingKey；多版本全部标记）。
func (s *Source) MarkWatched(ctx context.Context, ops []watched.WatchOp) []watched.OpResult {
	results := make([]watched.OpResult, 0, len(ops))
	for _, op := range ops {
		results = append(results, s.mark(op.Key, true)...)
	}
	return results
}

// MarkUnwatched 批量 unscrobble。
func (s *Source) MarkUnwatched(ctx context.Context, keys []watched.MediaKey) []watched.OpResult {
	results := make([]watched.OpResult, 0, len(keys))
	for _, key := range keys {
		results = append(results, s.mark(key, false)...)
	}
	return results
}

func (s *Source) mark(key watched.MediaKey, watchedState bool) []watched.OpResult {
	refs := s.RefsOf(key)
	if len(refs) == 0 {
		return []watched.OpResult{{Key: key, OK: false, Err: "Plex ratingKey 未知（条目不在本轮枚举中）"}}
	}
	var lastErr string
	okAll := true
	for _, ref := range refs {
		var err error
		if watchedState {
			err = s.Client.Scrobble(ref)
		} else {
			err = s.Client.Unscrobble(ref)
		}
		if err != nil {
			okAll = false
			lastErr = err.Error()
		}
	}
	return []watched.OpResult{{Key: key, OK: okAll, Err: lastErr}}
}

// ResolveMetadataRef 供会话轮询补解析：按 ratingKey 拉详情并解析身份。
func (s *Source) ResolveMetadataRef(ratingKey string) (watched.MediaKey, bool) {
	item, err := s.Client.Metadata(ratingKey)
	if err != nil {
		return watched.MediaKey{}, false
	}
	// 会话详情可能是集：需要分区类型推断媒体类型；episode 类型即 tv
	sectionType := "movie"
	if item.Type == "episode" {
		sectionType = "show"
	}
	return s.resolveMetadata(item, sectionType)
}
