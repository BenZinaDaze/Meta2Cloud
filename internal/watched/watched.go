// Package watched 定义观看状态源的统一抽象（仿 cloudfile.Provider 模式）。
// 同步引擎只依赖本包的接口与模型；Trakt / Plex（及未来的 Emby、Jellyfin）
// 各自提供 HTTP 客户端 + Provider 实现，接入新播放端不影响引擎与影子存储。
package watched

import (
	"context"
	"fmt"
)

// MediaKey 规范化媒体身份（以 TMDB 为锚，两侧对齐的唯一键）。
// 电影 Season/Episode 为 0；剧集只以「集」为粒度参与同步，S00 特辑是普通键。
type MediaKey struct {
	Type    string // "movie" | "tv"
	TMDBID  int64
	Season  int
	Episode int
}

// String 可读标识（日志用）。
func (k MediaKey) String() string {
	if k.Type == "movie" {
		return fmt.Sprintf("movie:%d", k.TMDBID)
	}
	return fmt.Sprintf("tv:%d:S%02dE%02d", k.TMDBID, k.Season, k.Episode)
}

// ItemState 单侧观测到的观看状态。
type ItemState struct {
	Present bool   // 本轮枚举中出现（缺席≠未看，由引擎结合影子判断）
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
// 数据来自 meta2cloud 自己维护的 media_title_pins / library_media，只读）。
type TitleIndex interface {
	// Lookup 精确/归一化匹配 (媒体类型, 标题, 年份)；未命中返回 false。
	Lookup(mediaType, title string, year int64) (MediaKey, bool)
}
