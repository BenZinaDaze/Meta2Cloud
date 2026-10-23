// source.go —— TraktSource：把 Trakt 客户端适配成 watched.Provider。
// 身份经 IDMapper 归一（tmdb 锚优先，剧集缺 tmdb 时 tvdb 锚）；批量写入按
// ≤100 条/请求分块，载荷携带全部已知 ID。
package trakt

import (
	"context"
	"fmt"
	"time"

	"meta2cloud/internal/watched"
)

// historyChunkSize 单次 /sync/history 请求携带的最大条数（保守安全值）。
const historyChunkSize = 100

// TraktSource watched.Provider 的 Trakt 实现。
type TraktSource struct {
	Client *Client
	Mapper watched.IDMapper
}

// NewTraktSource 构造。mapper 为 nil 时使用内存映射。
func NewTraktSource(client *Client, mapper watched.IDMapper) *TraktSource {
	if mapper == nil {
		mapper = watched.NewMemIDMapper()
	}
	return &TraktSource{Client: client, Mapper: mapper}
}

// Name 实现 watched.Provider。
func (s *TraktSource) Name() string { return "trakt" }

// ListAll 全量枚举 Trakt 已看状态。
//
// 电影：/sync/watched/movies 直接给出逐部状态。
// 剧集：/sync/watched/shows 只能列出「有记录的剧」，但记录可能是剧级聚合
// （如 Trakt 官方 Plex 集成写入的条目，无 seasons 明细）；逐集的权威状态
// 以 /shows/{id}/progress/watched 为准——每剧一调，枚举 completed 集。
// 身份经 IDMapper 归一：tmdb 缺失的剧以 tvdb 锚成键（不再直接跳过）。
func (s *TraktSource) ListAll(ctx context.Context) (map[watched.MediaKey]watched.ItemState, error) {
	out := map[watched.MediaKey]watched.ItemState{}

	movies, err := s.Client.SyncWatchedMovies()
	if err != nil {
		return nil, fmt.Errorf("拉取 Trakt 电影已看失败：%w", err)
	}
	for _, m := range movies {
		key, ok := s.Mapper.ResolveKey("movie", watched.MediaIDs{
			TMDB: m.Movie.IDs.TMDB, TVDB: m.Movie.IDs.TVDB, IMDB: m.Movie.IDs.IMDB,
		}, 0, 0)
		if !ok {
			continue
		}
		out[key] = watched.ItemState{
			Present: true, Watched: true,
			LastAt: m.LastWatchedAt.Unix(), Plays: m.Plays,
		}
	}

	shows, err := s.Client.SyncWatchedShows()
	if err != nil {
		return nil, fmt.Errorf("拉取 Trakt 剧集已看失败：%w", err)
	}
	for _, show := range shows {
		if show.Show.IDs.Trakt <= 0 {
			continue
		}
		showKey, ok := s.Mapper.ResolveKey("tv", watched.MediaIDs{
			TMDB: show.Show.IDs.TMDB, TVDB: show.Show.IDs.TVDB, IMDB: show.Show.IDs.IMDB,
		}, 0, 0)
		if !ok {
			continue
		}
		progress, err := s.Client.ShowWatchedProgress(show.Show.IDs.Trakt)
		if err != nil {
			// 单剧进度失败不拖垮整轮：跳过该剧（同步状态保留旧值，下轮重试）
			continue
		}
		var resetAt int64
		if progress.ResetAt != "" {
			resetAt = progress.ResetAt.Unix()
		}
		for _, season := range progress.Seasons {
			for _, ep := range season.Episodes {
				if !ep.Completed {
					continue
				}
				// 官方语义：reset 后应忽略早于 reset_at 的观看
				lastAt := ep.LastWatchedAt.Unix()
				if resetAt > 0 && lastAt > 0 && lastAt < resetAt {
					continue
				}
				key := showKey
				key.Season, key.Episode = season.Number, ep.Number
				out[key] = watched.ItemState{
					Present: true, Watched: true,
					LastAt: lastAt, Plays: 1,
				}
			}
		}
	}
	return out, nil
}

// MarkWatched 批量记录已看；分块提交，块内未匹配或 added=0 均视为失败（同步状态不动，下轮重试）。
func (s *TraktSource) MarkWatched(ctx context.Context, ops []watched.WatchOp) []watched.OpResult {
	return s.apply(s.Client.AddHistory, ops, true)
}

// MarkUnwatched 批量移除已看记录。
func (s *TraktSource) MarkUnwatched(ctx context.Context, keys []watched.MediaKey) []watched.OpResult {
	ops := make([]watched.WatchOp, 0, len(keys))
	for _, key := range keys {
		ops = append(ops, watched.WatchOp{Key: key})
	}
	return s.apply(s.Client.RemoveHistory, ops, false)
}

// entryMatchesKey not_found 回带 ID 与本地键是否同指一媒体（任一系 ID 命中即算；
// imdb 不参与——本地键不含 imdb 锚，能构造出的载荷必带 tmdb 或 tvdb）。
func entryMatchesKey(e NotFoundEntry, k watched.MediaKey) bool {
	if k.TMDBID > 0 && e.TmdbID > 0 && k.TMDBID == e.TmdbID {
		return true
	}
	if k.TVDBID > 0 && e.TvdbID > 0 && k.TVDBID == e.TvdbID {
		return true
	}
	return false
}

// notFoundFailedKeys 把 not_found 清单归因到块内的具体键，返回 键→错误说明。
// entries 为 nil、存在无法归因的条目（未回带 id 且块内不止一部剧、或对不上
// 任何键）时返回 ok=false，调用方回退整批失败的保守策略。
func notFoundFailedKeys(chunk []watched.WatchOp, entries []NotFoundEntry) (map[watched.MediaKey]string, bool) {
	if entries == nil {
		return nil, false
	}
	inChunk := map[watched.MediaKey]bool{}
	anchorIDs := map[int64]bool{} // 块内剧集的锚 ID（tmdb 或 tvdb）
	for _, op := range chunk {
		inChunk[op.Key] = true
		if op.Key.Type == "tv" {
			if op.Key.TVDBID > 0 {
				anchorIDs[op.Key.TVDBID] = true
			} else {
				anchorIDs[op.Key.TMDBID] = true
			}
		}
	}
	var loneShow int64
	if len(anchorIDs) == 1 {
		for id := range anchorIDs {
			loneShow = id
		}
	}
	failed := map[watched.MediaKey]string{}
	for _, e := range entries {
		if e.TmdbID == 0 && e.TvdbID == 0 && e.ImdbID == "" {
			// 响应未回带任何 ID：仅块内单一媒体时可归因
			switch {
			case e.MediaType == "movie" && len(inChunk) == 1:
				for k := range inChunk {
					e.TmdbID = k.TMDBID
				}
			case e.MediaType != "movie" && loneShow != 0:
				if loneShowIsTVDB(chunk, loneShow) {
					e.TvdbID = loneShow
				} else {
					e.TmdbID = loneShow
				}
			default:
				return nil, false
			}
		}
		hit := false
		for k := range inChunk {
			if k.Type != e.MediaType || !entryMatchesKey(e, k) {
				continue
			}
			switch {
			case e.Season < 0:
				if e.MediaType == "movie" {
					failed[k] = "Trakt 未收录该电影（外部 ID 无映射）"
				} else {
					failed[k] = "Trakt 未收录该剧（外部 ID 无映射）"
				}
				hit = true
			case e.Episode < 0:
				if k.Season == e.Season {
					failed[k] = fmt.Sprintf("Trakt 未收录该剧第 %d 季", e.Season)
					hit = true
				}
			default:
				if k.Season == e.Season && k.Episode == e.Episode {
					failed[k] = fmt.Sprintf("Trakt 未收录该集 S%02dE%02d", e.Season, e.Episode)
					hit = true
				}
			}
		}
		if !hit {
			return nil, false
		}
	}
	return failed, true
}

// loneShowIsTVDB 判断块内唯一的剧集锚是否为 tvdb 锚。
func loneShowIsTVDB(chunk []watched.WatchOp, anchorID int64) bool {
	for _, op := range chunk {
		if op.Key.Type == "tv" && (op.Key.TVDBID == anchorID || op.Key.TMDBID == anchorID) {
			return op.Key.TVDBID > 0
		}
	}
	return false
}

type historyFunc func(movies []HistoryMovie, episodes []HistoryEpisode) (*HistoryResult, error)

func (s *TraktSource) apply(fn historyFunc, ops []watched.WatchOp, requireAdded bool) []watched.OpResult {
	results := make([]watched.OpResult, 0, len(ops))
	for start := 0; start < len(ops); start += historyChunkSize {
		end := start + historyChunkSize
		if end > len(ops) {
			end = len(ops)
		}
		chunk := ops[start:end]

		var movies []HistoryMovie
		episodes := make([]HistoryEpisode, 0, len(chunk))
		for _, op := range chunk {
			ids := s.Mapper.IDsOf(op.Key)
			switch op.Key.Type {
			case "movie":
				m := HistoryMovie{TMDBID: ids.TMDB, TVDBID: ids.TVDB, IMDBID: ids.IMDB}
				if m.TMDBID == 0 && m.TVDBID == 0 && m.IMDBID == "" {
					m.TMDBID = op.Key.TMDBID
				}
				if op.At > 0 {
					m.WatchedAt = time.Unix(op.At, 0)
				}
				movies = append(movies, m)
			case "tv":
				ep := HistoryEpisode{
					ShowTMDBID: ids.TMDB, ShowTVDBID: ids.TVDB, ShowIMDBID: ids.IMDB,
					Season: op.Key.Season, Episode: op.Key.Episode,
				}
				if ep.ShowTMDBID == 0 && ep.ShowTVDBID == 0 && ep.ShowIMDBID == "" {
					if op.Key.TVDBID > 0 {
						ep.ShowTVDBID = op.Key.TVDBID
					} else {
						ep.ShowTMDBID = op.Key.TMDBID
					}
				}
				if op.At > 0 {
					ep.WatchedAt = time.Unix(op.At, 0)
				}
				episodes = append(episodes, ep)
			}
		}
		// 同一块内电影+剧集可共存于一次 POST；电影超块由循环切分。
		result, err := fn(movies, episodes)
		if err != nil {
			for _, op := range chunk {
				results = append(results, watched.OpResult{Key: op.Key, OK: false, Err: err.Error()})
			}
			continue
		}
		if result.NotFoundCount > 0 {
			// 精确归因：not_found 只连坐真正未匹配的键，同批已匹配的按成功
			// 推进（Trakt 逐条独立处理，未匹配之外的条目已实际写入）。
			// 无法归因（形态未知/对不上键）时回退整批失败的保守策略。
			failedMsgs, ok := notFoundFailedKeys(chunk, result.NotFound)
			if !ok {
				msg := fmt.Sprintf("Trakt 返回 %d 条未匹配（tmdb id 无法识别），本批将下轮重试", result.NotFoundCount)
				for _, op := range chunk {
					results = append(results, watched.OpResult{Key: op.Key, OK: false, Err: msg})
				}
				continue
			}
			for _, op := range chunk {
				if msg, bad := failedMsgs[op.Key]; bad {
					results = append(results, watched.OpResult{Key: op.Key, OK: false, Err: msg})
					continue
				}
				if requireAdded && result.Count == 0 {
					results = append(results, watched.OpResult{Key: op.Key, OK: false,
						Err: "Trakt 返回 added=0（写入未被接受），本批将下轮重试"})
					continue
				}
				results = append(results, watched.OpResult{Key: op.Key, OK: true})
			}
			continue
		}
		// 记录已看必须拿到服务端确认（added>0）：added=0 说明载荷未被实际
		// 接受（如条目嵌套未匹配），按失败处理（同步状态不动，下轮重试），
		// 严禁在未确认写入的情况下把同步状态标成已看。取消已看不要求 deleted>0：
		// 目标本就未看时 deleted=0 即幂等达成。
		if requireAdded && result.Count == 0 {
			msg := fmt.Sprintf("Trakt 返回 added=0（写入未被接受），本批将下轮重试")
			for _, op := range chunk {
				results = append(results, watched.OpResult{Key: op.Key, OK: false, Err: msg})
			}
			continue
		}
		for _, op := range chunk {
			results = append(results, watched.OpResult{Key: op.Key, OK: true})
		}
	}
	return results
}
