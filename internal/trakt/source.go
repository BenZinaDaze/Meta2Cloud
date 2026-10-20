// source.go —— TraktSource：把 Trakt 客户端适配成 watched.Provider。
// 身份天然以 tmdb id 为锚，无需额外解析；批量写入按 ≤100 条/请求分块。
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
}

// NewTraktSource 构造。
func NewTraktSource(client *Client) *TraktSource { return &TraktSource{Client: client} }

// Name 实现 watched.Provider。
func (s *TraktSource) Name() string { return "trakt" }

// ListAll 全量枚举 Trakt 已看状态。
//
// 电影：/sync/watched/movies 直接给出逐部状态。
// 剧集：/sync/watched/shows 只能列出「有记录的剧」，但记录可能是剧级聚合
// （如 Trakt 官方 Plex 集成写入的条目，无 seasons 明细）；逐集的权威状态
// 以 /shows/{id}/progress/watched 为准——每剧一调，枚举 completed 集。
func (s *TraktSource) ListAll(ctx context.Context) (map[watched.MediaKey]watched.ItemState, error) {
	out := map[watched.MediaKey]watched.ItemState{}

	movies, err := s.Client.SyncWatchedMovies()
	if err != nil {
		return nil, fmt.Errorf("拉取 Trakt 电影已看失败：%w", err)
	}
	for _, m := range movies {
		if m.Movie.IDs.TMDB <= 0 {
			continue
		}
		key := watched.MediaKey{Type: "movie", TMDBID: m.Movie.IDs.TMDB}
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
		if show.Show.IDs.TMDB <= 0 || show.Show.IDs.Trakt <= 0 {
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
				key := watched.MediaKey{Type: "tv", TMDBID: show.Show.IDs.TMDB, Season: season.Number, Episode: ep.Number}
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

// notFoundFailedKeys 把 not_found 清单归因到块内的具体键，返回 键→错误说明。
// entries 为 nil、存在无法归因的条目（未回带 id 且块内不止一部剧、或对不上
// 任何键）时返回 ok=false，调用方回退整批失败的保守策略。
func notFoundFailedKeys(chunk []watched.WatchOp, entries []NotFoundEntry) (map[watched.MediaKey]string, bool) {
	if entries == nil {
		return nil, false
	}
	inChunk := map[watched.MediaKey]bool{}
	showIDs := map[int64]bool{}
	for _, op := range chunk {
		inChunk[op.Key] = true
		if op.Key.Type == "tv" {
			showIDs[op.Key.TMDBID] = true
		}
	}
	var loneShow int64
	if len(showIDs) == 1 {
		for id := range showIDs {
			loneShow = id
		}
	}
	failed := map[watched.MediaKey]string{}
	for _, e := range entries {
		if e.TmdbID == 0 {
			if loneShow == 0 {
				return nil, false
			}
			e.TmdbID = loneShow
		}
		hit := false
		for k := range inChunk {
			if k.Type != e.MediaType || k.TMDBID != e.TmdbID {
				continue
			}
			switch {
			case e.Season < 0:
				if e.MediaType == "movie" {
					failed[k] = fmt.Sprintf("Trakt 未收录该电影（tmdb %d 无映射）", e.TmdbID)
				} else {
					failed[k] = fmt.Sprintf("Trakt 未收录该剧（tmdb %d 无映射）", e.TmdbID)
				}
				hit = true
			case e.Episode < 0:
				if k.Season == e.Season {
					failed[k] = fmt.Sprintf("Trakt 未收录该剧第 %d 季（tmdb %d）", e.Season, e.TmdbID)
					hit = true
				}
			default:
				if k.Season == e.Season && k.Episode == e.Episode {
					failed[k] = fmt.Sprintf("Trakt 未收录该集 S%02dE%02d（tmdb %d）", e.Season, e.Episode, e.TmdbID)
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
			switch op.Key.Type {
			case "movie":
				m := HistoryMovie{TMDBID: op.Key.TMDBID}
				if op.At > 0 {
					m.WatchedAt = time.Unix(op.At, 0)
				}
				movies = append(movies, m)
			case "tv":
				ep := HistoryEpisode{ShowTMDBID: op.Key.TMDBID, Season: op.Key.Season, Episode: op.Key.Episode}
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
