// Package watched 定义观看状态源的统一抽象（仿 cloudfile.Provider 模式）。
// 同步引擎只依赖本包的接口与模型；Trakt / Plex（及未来的 Emby、Jellyfin）
// 各自提供 HTTP 客户端 + Provider 实现，接入新播放端不影响引擎与同步状态存储。
package watched

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// MediaKey 规范化媒体身份（两侧对齐的唯一键）。
// 锚定规则：TMDBID>0 为 tmdb 锚；剧集缺 tmdb 时以 TVDBID>0 回退为 tvdb 锚
// （电影必须 tmdb）。电影 Season/Episode 为 0；剧集只以「集」为粒度参与同步，
// S00 特辑是普通键。
type MediaKey struct {
	Type    string // "movie" | "tv"
	TMDBID  int64  // tmdb 锚（TVDBID>0 时为 0）
	TVDBID  int64  // >0 表示 tvdb 锚定的剧集键（仅剧集）
	Season  int
	Episode int
}

// String 可读标识（日志用）。
func (k MediaKey) String() string {
	if k.Type == "movie" {
		return fmt.Sprintf("movie:%d", k.TMDBID)
	}
	if k.TVDBID > 0 {
		return fmt.Sprintf("tv:tvdb:%d:S%02dE%02d", k.TVDBID, k.Season, k.Episode)
	}
	return fmt.Sprintf("tv:%d:S%02dE%02d", k.TMDBID, k.Season, k.Episode)
}

// MediaIDs 同一媒体的已知外部 ID 集合。匹配与推送载荷都应携带全部已知 ID
// （Trakt 任一 ID 命中即匹配，缓解「Trakt 词条缺某系映射」导致的 not_found）。
type MediaIDs struct {
	TMDB int64
	TVDB int64
	IMDB string
}

// IDMapper 媒体身份归一化：把任意来源的外部 ID 集合归并为两侧一致的规范键，
// 并把新见 ID 并入映射，使「一侧只知 tvdb、另一侧知 tmdb+tvdb」最终归并到
// 同一个键（映射表由宿主存储实现持久化）。
type IDMapper interface {
	// ResolveKey 归并 IDs 并返回规范键。无法成键（电影缺 tmdb；剧集缺 tmdb+tvdb）
	// 返回 false。
	ResolveKey(mediaType string, ids MediaIDs, season, episode int) (MediaKey, bool)
	// IDsOf 返回键对应的全部已知 ID（组多 ID 推送载荷用）；无记录返回零值。
	IDsOf(key MediaKey) MediaIDs
}

// MemIDMapper IDMapper 的内存实现（测试与无持久化场景用）。
type MemIDMapper struct {
	mu   sync.Mutex
	rows map[string][]MediaIDs // mediaType → 已知身份集合
}

// NewMemIDMapper 构造内存 IDMapper。
func NewMemIDMapper() *MemIDMapper { return &MemIDMapper{rows: map[string][]MediaIDs{}} }

// ResolveKey 实现 IDMapper。
func (m *MemIDMapper) ResolveKey(mediaType string, ids MediaIDs, season, episode int) (MediaKey, bool) {
	if ids == (MediaIDs{}) {
		return MediaKey{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var merged *MediaIDs
	for i := range m.rows[mediaType] {
		row := &m.rows[mediaType][i]
		if idsMatch(*row, ids) {
			if row.TMDB == 0 {
				row.TMDB = ids.TMDB
			}
			if row.TVDB == 0 {
				row.TVDB = ids.TVDB
			}
			if row.IMDB == "" {
				row.IMDB = ids.IMDB
			}
			merged = row
			break
		}
	}
	if merged == nil {
		m.rows[mediaType] = append(m.rows[mediaType], ids)
		merged = &m.rows[mediaType][len(m.rows[mediaType])-1]
	}
	return CanonicalKey(mediaType, *merged, season, episode)
}

// IDsOf 实现 IDMapper。
func (m *MemIDMapper) IDsOf(key MediaKey) MediaIDs {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, row := range m.rows[key.Type] {
		if idsMatch(row, MediaIDs{TMDB: key.TMDBID, TVDB: key.TVDBID}) {
			return row
		}
	}
	return MediaIDs{}
}

func idsMatch(a, b MediaIDs) bool {
	if a.TMDB > 0 && b.TMDB > 0 {
		return a.TMDB == b.TMDB
	}
	if a.TVDB > 0 && b.TVDB > 0 {
		return a.TVDB == b.TVDB
	}
	if a.IMDB != "" && b.IMDB != "" {
		return strings.EqualFold(a.IMDB, b.IMDB)
	}
	return false
}

// CanonicalKey 按 ID 集规范成键：tmdb 锚优先；剧集缺 tmdb 回退 tvdb 锚；
// 无法成键（电影缺 tmdb、剧集两者皆缺）返回 false。
func CanonicalKey(mediaType string, ids MediaIDs, season, episode int) (MediaKey, bool) {
	key := MediaKey{Type: mediaType, Season: season, Episode: episode}
	switch {
	case ids.TMDB > 0:
		key.TMDBID = ids.TMDB
	case mediaType == "tv" && ids.TVDB > 0:
		key.TVDBID = ids.TVDB
	default:
		return MediaKey{}, false // 电影缺 tmdb；剧集缺 tmdb+tvdb
	}
	return key, true
}

// ItemState 单侧观测到的观看状态。
type ItemState struct {
	Present bool   // 本轮枚举中出现（缺席≠未看，由引擎结合同步状态判断）
	Watched bool   // 已观看
	LastAt  int64  // 最近观看时间（unix 秒；0=未知）
	Plays   int    // 观看次数（仅用于冲突/重看判断，不做两侧对齐）
	Ref     string // 播放端原生键（如 Plex ratingKey）；Trakt 侧为空
}

// WatchOp 一次「标记已看」操作。
type WatchOp struct {
	Key MediaKey
	At  int64 // 期望记录的观看时间（unix 秒；0=由目标端自行决定）
}

// OpResult 单键操作结果。
type OpResult struct {
	Key MediaKey
	OK  bool
	Err string
}

// Provider 观看状态源：全量枚举 + 双向标记。实现须满足：
//   - ListAll 内部完成身份解析与多版本/多文件聚合（同一 MediaKey 只出现一次：
//     Watched=OR、LastAt=max、Plays=max，求和会因多版本重复计数）
//   - Mark* 内部分批与节流，逐键返回结果；批量中途失败时已成功键仍返回 OK
type Provider interface {
	Name() string
	ListAll(ctx context.Context) (map[MediaKey]ItemState, error)
	MarkWatched(ctx context.Context, ops []WatchOp) []OpResult
	MarkUnwatched(ctx context.Context, keys []MediaKey) []OpResult
}

// Session 单条进行中播放。
type Session struct {
	SessionKey string
	Ref        string // 播放端原生键（ratingKey）
	Key        MediaKey
	OffsetMS   int64
	DurationMS int64
	Title      string
}

// SessionProvider 「正在播放」能力（可选实现，目前仅 Plex 提供）。
type SessionProvider interface {
	ListSessions(ctx context.Context) ([]Session, error)
}

// UnresolvedItem 枚举时无法解析出 MediaKey 的条目（用于 run 明细与排障）。
type UnresolvedItem struct {
	Ref    string // 播放端原生键
	Title  string
	Reason string
}

// UnresolvedReporter 播放端可选实现：上报本轮 ListAll 中解析失败的条目。
// 引擎在每轮开始时 Drain，计入 run 统计。
type UnresolvedReporter interface {
	DrainUnresolved() []UnresolvedItem
}

// WatchedStampProvider 可选能力：返回该侧观看数据（已看列表）的最后活动时间
// （unix 秒）。用于最终一致性防护——远端写入刚完成时，已看列表可能尚未反映；
// 引擎把「数据时间戳早于本端最近一次写入」的读取视为过期，本轮跳过该侧 diff，
// 避免把「还没读到」误判成「被重置」。
type WatchedStampProvider interface {
	// WatchedStamp 返回电影与剧集（含集）两类观看数据的最后活动时间。
	WatchedStamp(ctx context.Context) (movies, shows int64, err error)
}

// TitleIndex 标题 → TMDB 身份反查（播放端路径兜底用；由宿主注入实现，
// 数据来自 panoply 自己维护的 media_title_pins / library_media，只读）。
type TitleIndex interface {
	// Lookup 精确/归一化匹配 (媒体类型, 标题, 年份)；未命中返回 false。
	Lookup(mediaType, title string, year int64) (MediaKey, bool)
}
