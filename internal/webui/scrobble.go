// Package webui —— Plex 播放会话 → Trakt scrobble 三段式推送（自 traktsvc.go 拆出）。
package webui

import (
	"errors"
	"fmt"
	"time"

	"meta2cloud/internal/plexserver"
	"meta2cloud/internal/syncengine"
	"meta2cloud/internal/trakt"
	"meta2cloud/internal/watched"
	"meta2cloud/internal/webui/store"
)

// ── 会话推送（正在看 → Trakt scrobble 三段式）──────────────

// sessionMaxMisses 连续缺席轮询若干次后收尾（stop）并移除会话观察行。
const sessionMaxMisses = 2

// displayOrKey 日志展示名：优先媒体标题，缺失时回退机器键。
func (s *traktService) displayOrKey(key watched.MediaKey, title string) string {
	if title != "" {
		return title
	}
	return key.String()
}

// sessionDisplayTitle 会话条目的可读标题。
func (s *traktService) sessionDisplayTitle(item *plexserver.Metadata) string {
	if item.Type == "episode" && item.GrandparentTitle != "" {
		return fmt.Sprintf("%s S%02dE%02d「%s」", item.GrandparentTitle, item.ParentIndex, item.Index, item.Title)
	}
	return item.Title
}

// sessionScrobbleGate 会话 scrobble 的公共门控：→Trakt 方向开关 + 媒体忽略。
func (s *traktService) sessionScrobbleGate(syncStore *store.TraktSyncStore, key watched.MediaKey) bool {
	if !s.rt.GetConfig().Trakt.SyncToTrakt {
		return false
	}
	return !syncStore.IsIgnoredKey(key)
}

// scrobbleIDs 组 scrobble 载荷 ID（映射表补全多 ID；无记录时回带键自身锚）。
func (s *traktService) scrobbleIDs(mapper watched.IDMapper, key watched.MediaKey) watched.MediaIDs {
	ids := mapper.IDsOf(key)
	if ids.TMDB == 0 && ids.TVDB == 0 && ids.IMDB == "" {
		if key.TVDBID > 0 {
			ids.TVDB = key.TVDBID
		} else {
			ids.TMDB = key.TMDBID
		}
	}
	return ids
}

// scrobbleProgress 播放进度百分比（0~100）；时长未知返回 false（无法上报）。
func (s *traktService) scrobbleProgress(offsetMS, durationMS int64) (float64, bool) {
	if durationMS <= 0 {
		return 0, false
	}
	p := float64(offsetMS) / float64(durationMS) * 100
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	return p, true
}

// sessionScrobble 每会话的上报状态（内存态；进程重启即重建，start 幂等无碍）。
// lastSent: "" | "playing" | "paused" | "finished" —— 仅在状态跃迁时调 API，
// 播放中的周期性 progress 事件不重复上报（Trakt watching 状态按剩余时长自动过期）；
// finished 表示已按收尾上报 stop（≥80% 暂停视同看完），同会话后续事件不再上报。
// awaitPlay: 播放队列切集后置位——抓包证实（2026-10-25，
// config/data/sse-capture/20261025-172916）切换后新条目的 paused/buffering
// 事件仍带旧条目的遗留 offset、playing 才复位，期间进度按 0 记。
type sessionScrobble struct {
	lastSent  string
	awaitPlay bool
	// lastObserved 本会话最近一次观察到的非 stopped 状态：暂停中的播放器
	// 位置不可能前进，其后的 stopped 大幅超出观察轨迹即客户端垃圾值；
	// playing 态则与 stopped 之间的自然推进属正常，不封顶
	lastObserved string
	// unresolvedLogged 身份解析失败告警只记一次（事件每 ~10s 重发，
	// 无闸会刷屏；2026-10-26 事故曾因该路径完全静默排查数小时）
	unresolvedLogged bool
}

// scrobbleTransition 按状态跃迁上报 start/pause（均不计次）。仅 stop 收尾
// 由 finishSession 负责（计次与否由 Trakt 服务端裁定）。Trakt 的 pause 仅
// 接受 1%~80% 进度：<1% 尚无进度可存（跳过上报），≥80% 官方语义为「应改用
// stop」，按已看完收尾处理。
func (s *traktService) scrobbleTransition(source string, syncStore *store.TraktSyncStore, traktClient *trakt.Client, mapper watched.IDMapper, key watched.MediaKey, title, state string, progress float64, sessionKey string) {
	st := s.scrobbleStateFor(sessionKey)
	switch state {
	case "playing":
		if st.lastSent == "playing" {
			return
		}
		// 已收尾且仍在片尾段（≥80%）：不再上报 start（亦超出其有效区间），
		// 会话结束时的 stop 由服务端去重（409），不会重复计次
		if st.lastSent == "finished" && progress >= 80 {
			return
		}
		if s.scrobbleCall(mapper, traktClient, trakt.ScrobbleStart, key, title, progress) {
			st.lastSent = "playing"
		}
	case "paused":
		if st.lastSent != "playing" {
			return
		}
		switch {
		case progress >= 80:
			// Trakt 不接受 >80% 的 pause（官方提示改用 stop）：进度已过计次线，
			// 视同看完直接按收尾上报；置终态避免暂停期间后续 paused 事件反复收尾
			st.lastSent = "finished"
			s.finishSession(syncStore, traktClient, mapper, key, title, source, progress/100, sessionKey)
		case progress < 1:
			// Trakt 至少要求 1% 才接受 pause：刚开播尚无进度可存，本地记为已暂停
			// 即可（否则暂停期间每个事件都会重试同一失败），恢复播放仍会正常上报 start
			st.lastSent = "paused"
		default:
			if s.scrobbleCall(mapper, traktClient, trakt.ScrobblePause, key, title, progress) {
				st.lastSent = "paused"
			}
		}
	}
}

// scrobbleCall 单次 start/pause 上报（仅在状态跃迁时调用，量级为每次观看
// 个位数条）：成功记 INFO 便于观察上报轨迹，失败记 WARNING（下个事件自然重试）。
func (s *traktService) scrobbleCall(mapper watched.IDMapper, traktClient *trakt.Client, action trakt.ScrobbleAction, key watched.MediaKey, title string, progress float64) bool {
	display := s.displayOrKey(key, title)
	actionName := "开始记录"
	if action == trakt.ScrobblePause {
		actionName = "暂停记录"
	}
	if _, err := s.scrobbleDoWith(mapper, traktClient, action, key, progress); err != nil {
		s.rt.appLog("trakt", "session_scrobble_failed", "WARNING",
			fmt.Sprintf("播放记录推送 Trakt 失败（%s %s）：%v", actionName, display, err), nil)
		return false
	}
	switch action {
	case trakt.ScrobbleStart:
		s.rt.appLog("trakt", "session_scrobble_started", "INFO",
			fmt.Sprintf("已通知 Trakt 开始记录播放：%s（进度 %.0f%%）", display, progress), nil)
	case trakt.ScrobblePause:
		s.rt.appLog("trakt", "session_scrobble_paused", "INFO",
			fmt.Sprintf("已通知 Trakt 暂停记录播放：%s（进度 %.0f%%）", display, progress), nil)
	}
	return true
}

// scrobbleDoWith 组多 ID 载荷并执行 scrobble（mapper 为 nil 时用键自身锚）。
func (s *traktService) scrobbleDoWith(mapper watched.IDMapper, traktClient *trakt.Client, action trakt.ScrobbleAction, key watched.MediaKey, progress float64) (trakt.ScrobbleResult, error) {
	var ids watched.MediaIDs
	if mapper != nil {
		ids = s.scrobbleIDs(mapper, key)
	} else if key.TVDBID > 0 {
		ids = watched.MediaIDs{TVDB: key.TVDBID}
	} else {
		ids = watched.MediaIDs{TMDB: key.TMDBID}
	}
	if key.Type == "movie" {
		return traktClient.Scrobble(action,
			&trakt.HistoryMovie{TMDBID: ids.TMDB, TVDBID: ids.TVDB, IMDBID: ids.IMDB}, nil, progress)
	}
	return traktClient.Scrobble(action, nil,
		&trakt.HistoryEpisode{
			ShowTMDBID: ids.TMDB, ShowTVDBID: ids.TVDB, ShowIMDBID: ids.IMDB,
			Season: key.Season, Episode: key.Episode,
		}, progress)
}

// finishSession 会话收尾：以最后已知进度上报 stop，计次与否由 Trakt 服务端裁定
// （action=scrobble 计一次；pause 未达阈值；409 处于去重窗口即媒体已在历史中，
// 仅对齐本地状态不落记录；422 进度不足被忽略）。失败落退避标记与失败记录，
// 观看状态最终由定时全量同步兜底。
// 不清理 scrobble 上报态（调用方在移除会话时 dropScrobbleState），以便 ≥80% 暂停
// 经由此收尾后终态标记仍生效。
func (s *traktService) finishSession(syncStore *store.TraktSyncStore, traktClient *trakt.Client, mapper watched.IDMapper, key watched.MediaKey, title, source string, progress float64, sessionKey string) {
	if (key.TMDBID == 0 && key.TVDBID == 0) || !s.sessionScrobbleGate(syncStore, key) {
		return
	}
	item := syncengine.RunItem{Key: key, Title: title, Direction: "to_trakt", Action: "watched"}
	record := func(success bool, errMsg string) {
		_ = syncStore.RecordSessionPushRun(source, item, progress, success, errMsg)
	}
	now := time.Now().Unix()
	// 记录层沿用 0~1 进度分数；scrobble API 要求 0~100 百分比
	res, err := s.scrobbleDoWith(mapper, traktClient, trakt.ScrobbleStop, key, progress*100)
	switch {
	case err == nil && res.Action == "scrobble":
		_ = syncStore.UpdateTraktWatched(key, now, now)
		_ = syncStore.SetTraktPushedAt(now) // 供下轮全量同步的过期读取防护
		record(true, "")
		s.rt.appLog("trakt", "session_pushed", "SUCCESS",
			fmt.Sprintf("播放已记录到 Trakt：%s（进度 %.0f%%）", s.displayOrKey(key, title), progress*100), nil)
		// 推送成功即 Trakt 授权有效：会话推送是独立于全量同步的恢复上报点
		s.rt.NotifyServiceHealth("Trakt", true, "")
	case err == nil:
		// action=pause：进度未达阈值，服务端未计次——不落记录
	case errors.As(err, new(*trakt.DuplicateScrobbleError)):
		// 服务端去重（409）：媒体已在 Trakt 历史中，目的态达成——仅对齐本地
		// 观看状态并记日志，不落 run 记录（页面只呈现真实计次推送）
		watchedAt := now
		var dup *trakt.DuplicateScrobbleError
		if errors.As(err, &dup) && !dup.WatchedAt.IsZero() {
			watchedAt = dup.WatchedAt.Unix()
		}
		_ = syncStore.UpdateTraktWatched(key, watchedAt, now)
		_ = syncStore.SetTraktPushedAt(now)
		s.rt.appLog("trakt", "session_pushed", "INFO",
			fmt.Sprintf("Trakt 近期已记录过该内容，本次不重复计入：%s", s.displayOrKey(key, title)), nil)
	case errors.As(err, new(*trakt.ScrobbleIgnoredError)):
		// 422：进度不足等被服务端忽略（如刚开播即停）——静默
	default:
		_ = syncStore.MarkSessionPushFailure(key, now)
		record(false, err.Error())
		s.rt.appLog("trakt", "session_push_failed", "WARNING",
			fmt.Sprintf("播放记录推送 Trakt 失败（%s）：%v", s.displayOrKey(key, title), err), nil)
		if errors.Is(err, trakt.ErrUnauthorized) {
			s.rt.NotifyServiceHealth("Trakt", false, "播放上报认证失败："+err.Error())
		}
	}
}

// finishSwitchedSession 会话内条目切换（播放队列切集）时收尾旧条目并重置
// 跟踪：以观察行的最大进度上报 stop（已收尾过则不重复），随后清空观察行
// 与上报状态，新条目重新解析身份、进度从零跟踪——上一集的进度不会带到
// 下一集。切换后到 playing 前的 offset 守卫（awaitPlay）由调用方置位。
func (s *traktService) finishSwitchedSession(syncStore *store.TraktSyncStore, traktClient *trakt.Client, prev *store.TraktSyncSession, source, sessionKey string) {
	if prev.TmdbID > 0 && s.scrobbleStateFor(sessionKey).lastSent != "finished" {
		s.finishSession(syncStore, traktClient, syncStore, prev.Key(), s.derefString(prev.Title), source, prev.MaxProgress, sessionKey)
	}
	s.dropScrobbleState(sessionKey)
	syncStore.DeleteSyncSession(sessionKey)
}

// pollPlexSessionsOnce 单次会话轮询（SSE 离线时的兜底）：
// 首见会话上报 start；连续缺席的会话以最后已知进度 stop 收尾。
func (s *traktService) pollPlexSessionsOnce() {
	cfg := s.rt.GetConfig()
	nowStr := utcNowISO()
	s.sessionWatcher.mu.Lock()
	s.sessionWatcher.lastPolledAt = &nowStr
	s.sessionWatcher.lastPollError = nil
	s.sessionWatcher.mu.Unlock()

	// 纯兜底策略：SSE 在线时不并行轮询——实时事件已覆盖会话观察与上报，
	// 仅在 SSE 未连接（开关关闭/断线窗口/旧版 PMS 无端点）时才主动轮询。
	// 心跳簿记已在上方更新，watcher 存活状态不受影响。
	if s.sseConnected() {
		return
	}
	if !cfg.Trakt.SyncToTrakt || !cfg.IsPlexReady() || !cfg.IsTraktReady() {
		return
	}
	if !s.traktOauthStatusPayload().Authorized {
		return
	}
	if s.TraktSyncRunning() {
		return // 避免与全量同步交叉写 Trakt
	}
	plexClient := s.rt.GetPlexClient()
	if plexClient == nil {
		return
	}
	traktClient := s.rt.GetTraktClient()
	if traktClient == nil {
		return
	}
	syncStore := s.rt.GetTraktSyncStore()
	sessions, err := plexClient.Sessions()
	if err != nil {
		msg := fmt.Sprintf("%v", err)
		s.sessionWatcher.mu.Lock()
		s.sessionWatcher.lastPollError = &msg
		s.sessionWatcher.mu.Unlock()
		if errors.Is(err, plexserver.ErrUnauthorized) {
			s.rt.NotifyServiceHealth("Plex", false, err.Error())
		}
		return
	}
	// 会话查询成功即 Plex 可达且授权有效：SSE 关闭时这里是唯一的 Plex 恢复上报点
	s.rt.NotifyServiceHealth("Plex", true, "")

	seen := map[string]bool{}
	source := plexserver.NewSource(plexClient, s.rt.buildTitleIndex(), syncStore)
	for i := range sessions {
		sess := &sessions[i]
		seen[sess.SessionKey] = true

		// 切集检测（同 SSE 路径，事件形态见 handlePlexPlaybackNotification）：
		// 检测到切换的这一轮快照 offset 是旧条目遗留值，进度按 0 记且不派发
		// 跃迁；轮询间隔 ≥ 切换过渡窗，下一轮快照 offset 已复位
		switched := false
		if prev := syncStore.GetSyncSession(sess.SessionKey); prev != nil && prev.PlexRef != "" && prev.PlexRef != sess.RatingKey {
			s.finishSwitchedSession(syncStore, traktClient, prev, "poll", sess.SessionKey)
			switched = true
		}

		key, keyOK := s.resolveSessionKey(syncStore, source, sess)
		pct, progressOK := s.scrobbleProgress(int64(sess.ViewOffset), int64(sess.Duration))
		title := s.sessionDisplayTitle(sess)
		maxProgress := func() float64 {
			f, _ := s.scrobbleProgress(int64(sess.ViewOffset), int64(sess.Duration))
			return f / 100
		}()
		if switched {
			maxProgress = 0
		}
		row := store.TraktSyncSession{
			SessionKey: sess.SessionKey, PlexRef: sess.RatingKey,
			ViewOffsetMS: int64(sess.ViewOffset), DurationMS: int64(sess.Duration),
			MaxProgress: maxProgress, Title: &title,
		}
		if keyOK {
			row.MediaType = key.Type
			if key.TVDBID > 0 {
				row.Anchor = "tvdb"
				row.TmdbID = key.TVDBID
			} else {
				row.TmdbID = key.TMDBID
			}
			row.Season = key.Season
			row.Episode = key.Episode
		}
		syncStore.UpsertSyncSession(row)
		if keyOK && progressOK && !switched && s.sessionScrobbleGate(syncStore, key) {
			s.scrobbleTransition("poll", syncStore, traktClient, syncStore, key, title, "playing", pct, sess.SessionKey)
		}
	}

	// 缺席检测：连续 2 次未见 → 以最后已知进度 stop 收尾并移除
	s.reapAbsentSessions(syncStore, traktClient, seen)
}

// sessionStaleAge 会话观察行超过该时长未刷新即视为僵尸（上一段进程生命
// 周期遗留）。活跃会话随每条 SSE 事件/每轮轮询刷新 updated_at；正常收尾
// 路径（连续缺席 2 轮 × 30s）远早于该阈值，只有重启前遗留的旧行会触到。
const sessionStaleAge = 10 * time.Minute

// sessionRowStale 会话观察行是否已过期。时间不可解析时保守按未过期处理
// （走原缺席计数路径，行为与修复前一致）。
func (s *traktService) sessionRowStale(row store.TraktSyncSession) bool {
	t, err := time.Parse(time.RFC3339, row.UpdatedAt)
	if err != nil {
		return false
	}
	return time.Since(t) > sessionStaleAge
}

// reapAbsentSessions 清理本轮未见到的会话观察行：僵尸行（updated_at 距今
// 过久，重启前遗留）直接丢弃——其进度来自上一段进程生命周期，不能当作
// 「刚停止」推送，否则重启会把昨天看过的集再记一次 Trakt（2026-10-26
// 实测事故：docker 重启即推送 tv:312266:S01E09 进度 99%）；确有未推送的
// 观看由定时全量同步兜底。新鲜行连续缺席 2 轮后以最后已知进度收尾。
func (s *traktService) reapAbsentSessions(syncStore *store.TraktSyncStore, traktClient *trakt.Client, seen map[string]bool) {
	for _, row := range syncStore.ListSyncSessions() {
		if seen[row.SessionKey] {
			continue
		}
		if s.sessionRowStale(row) {
			s.dropScrobbleState(row.SessionKey)
			syncStore.DeleteSyncSession(row.SessionKey)
			s.rt.appLog("trakt", "session_stale_dropped", "INFO",
				fmt.Sprintf("已忽略一段重启前遗留的播放会话（%s，最后更新于 %s），不推送播放记录",
					s.displayOrKey(row.Key(), s.derefString(row.Title)), row.UpdatedAt), nil)
			continue
		}
		if misses := syncStore.IncrementSessionMiss(row.SessionKey); misses >= sessionMaxMisses {
			s.finishSession(syncStore, traktClient, syncStore, row.Key(), s.derefString(row.Title), "poll", row.MaxProgress, row.SessionKey)
			s.dropScrobbleState(row.SessionKey)
			syncStore.DeleteSyncSession(row.SessionKey)
		}
	}
}

func (s *traktService) derefString(ps *string) string {
	if ps == nil {
		return ""
	}
	return *ps
}

// （实时推送的实现见下方 handlePlexPlaybackNotification：scrobble 三段式，
// start/pause 不计次，stop 由 Trakt 服务端裁定是否计次与去重。）
