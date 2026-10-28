// Package webui —— Trakt ↔ Plex 双向同步：单飞触发、执行、状态与历史查询、忽略清单（自 traktsvc.go 拆出）。
package webui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"meta2cloud/internal/plexserver"
	"meta2cloud/internal/syncengine"
	"meta2cloud/internal/trakt"
	"meta2cloud/internal/watched"
	"meta2cloud/internal/webui/store"
)

// ── 双向同步：单飞触发 ─────────────────────────────────────

var traktSyncState struct {
	mu      sync.Mutex
	running bool
}

// TraktSyncRunning 同步是否进行中。
func TraktSyncRunning() bool {
	traktSyncState.mu.Lock()
	defer traktSyncState.mu.Unlock()
	return traktSyncState.running
}

// TriggerTraktSync 异步触发一轮同步；已在进行中返回 false。
func (rt *Runtime) TriggerTraktSync(trigger string) bool {
	traktSyncState.mu.Lock()
	if traktSyncState.running {
		traktSyncState.mu.Unlock()
		return false
	}
	traktSyncState.running = true
	traktSyncState.mu.Unlock()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				rt.appLog("trakt", "sync_panic", "ERROR", fmt.Sprintf("观看同步异常退出：%v", r), nil)
			}
			traktSyncState.mu.Lock()
			traktSyncState.running = false
			traktSyncState.mu.Unlock()
		}()
		rt.runTraktSync(trigger)
	}()
	return true
}

// traktSyncLogger 引擎日志回调 → appLog。
func (rt *Runtime) traktSyncLogger(level, message string, details map[string]any) {
	rt.appLog("trakt", "sync", level, message, details)
}

// runTraktSync 构建引擎并执行一轮同步。
func (rt *Runtime) runTraktSync(trigger string) {
	cfg := rt.GetConfig()
	traktClient := rt.GetTraktClient()
	if traktClient == nil {
		rt.appLog("trakt", "sync_skipped", "WARNING", "Trakt 客户端不可用（未配置或未授权），跳过同步", nil)
		return
	}
	plexClient := rt.GetPlexClient()
	if plexClient == nil {
		rt.appLog("trakt", "sync_skipped", "WARNING", "Plex 未配置服务器地址或未授权，跳过同步", nil)
		return
	}
	syncStore := rt.GetTraktSyncStore()
	engine := &syncengine.Engine{
		Playback: plexserver.NewSource(plexClient, rt.buildTitleIndex(), syncStore),
		Trakt:    trakt.NewTraktSource(traktClient, syncStore, syncStore),
		Store:    syncStore,
		ToPlex:   cfg.Trakt.SyncToPlex,
		ToTrakt:  cfg.Trakt.SyncToTrakt,
		Log:      rt.traktSyncLogger,
	}
	started := time.Now()
	summary, err := engine.Run(context.Background(), trigger)
	took := time.Since(started).Round(time.Millisecond)
	if err != nil {
		rt.appLog("trakt", "sync_failed", "ERROR", fmt.Sprintf("观看同步失败（耗时 %s）：%v", took, err), nil)
		// 仅授权失效上报健康状态；瞬时网络错误不告警
		switch {
		case errors.Is(err, trakt.ErrUnauthorized):
			NotifyServiceHealth("Trakt", false, "观看同步："+err.Error())
		case errors.Is(err, plexserver.ErrUnauthorized):
			NotifyServiceHealth("Plex", false, "观看同步："+err.Error())
		}
		return
	}
	// 全量成功即双向接口可达且授权有效：同时上报 Trakt 与 Plex 恢复
	NotifyServiceHealth("Trakt", true, "")
	NotifyServiceHealth("Plex", true, "")
	rt.appLog("trakt", "sync_done", "SUCCESS",
		fmt.Sprintf("观看同步完成（耗时 %s）：记录到 Trakt %d 条，回写 Plex %d 条，冲突 %d，无法识别 %d，跳过 %d，失败 %d",
			took, summary.ToTrakt, summary.ToPlex, summary.Conflicts, summary.Unresolved, summary.Skipped, summary.Failed),
		map[string]any{
			"to_trakt": summary.ToTrakt, "to_plex": summary.ToPlex,
			"conflicts": summary.Conflicts, "unresolved": summary.Unresolved,
			"skipped": summary.Skipped, "failed": summary.Failed,
			"took_ms": took.Milliseconds(),
		})
	// 有未处理项时提醒（长尾故障不再沉默数天）：写入失败 / 两侧冲突 / 无法识别
	if summary.Failed > 0 || summary.Conflicts > 0 || summary.Unresolved > 0 {
		notifyTraktSyncAnomaly(summary)
	}
}

// traktSyncAnomaly 异常提醒节流：持续异常每小时至多一条，避免 15 分钟一轮刷屏。
var (
	traktAnomalyMu   sync.Mutex
	traktAnomalyLast time.Time
)

func notifyTraktSyncAnomaly(summary syncengine.RunSummary) {
	traktAnomalyMu.Lock()
	if time.Since(traktAnomalyLast) < time.Hour {
		traktAnomalyMu.Unlock()
		return
	}
	traktAnomalyLast = time.Now()
	traktAnomalyMu.Unlock()
	sendHealthTelegram("trakt", "Trakt", fmt.Sprintf(
		"⚠️ 观看同步有未处理项：写入失败 %d 条，两侧状态冲突 %d 条，无法识别 %d 条。详情见同步记录",
		summary.Failed, summary.Conflicts, summary.Unresolved))
}

// ── HTTP 处理器：同步状态 / 触发 / 历史 ─────────────────────

func (rt *Runtime) traktSyncStatusPayload() map[string]any {
	cfg := rt.GetConfig()
	traktStatus := rt.traktOauthStatusPayload()
	plex := rt.plexStatusPayload()
	payload := map[string]any{
		"enabled":                cfg.Trakt.Enabled,
		"trakt_ready":            traktStatus["ready"],
		"trakt_authorized":       traktStatus["authorized"],
		"plex_ready":             plex["ready"],
		"plex_connected":         plex["token_configured"],
		"plex_reachable":         plex["reachable"],
		"sync_to_plex":           cfg.Trakt.SyncToPlex,
		"sync_to_trakt":          cfg.Trakt.SyncToTrakt,
		"sync_cron":              cfg.Trakt.SyncCron,
		"session_poll_seconds":   cfg.Trakt.SessionPollSeconds,
		"running":                TraktSyncRunning(),
		"sync_watcher_alive":     watcherAlive(traktWatcher),
		"session_watcher_alive":  watcherAlive(traktSessionWatcher),
		"sse_enabled":            cfg.Trakt.SSEEnabled,
		"sse_connected":          rt.sseConnected(),
		"last_polled_at":         traktWatcher.lastPolledAt,
		"last_poll_error":        traktWatcher.lastPollError,
		"last_session_polled_at": traktSessionWatcher.lastPolledAt,
	}
	if store := rt.GetTraktSyncStore(); store != nil {
		if last := store.GetLastFinishedSyncRun(); last != nil {
			payload["last_run"] = map[string]any{
				"id": last.ID, "started_at": last.StartedAt, "finished_at": last.FinishedAt,
				"trigger": last.Trigger, "to_trakt": last.ToTrakt, "to_plex": last.ToPlex,
				"conflicts": last.Conflicts, "unresolved": last.Unresolved,
				"skipped": last.Skipped, "error": last.Error,
			}
		}
	}
	return payload
}

func (s *Server) handleTraktSyncStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.rt.traktSyncStatusPayload())
}

func (s *Server) handleTraktSyncTrigger(w http.ResponseWriter, _ *http.Request) {
	status := s.rt.traktOauthStatusPayload()
	if authorized, _ := status["authorized"].(bool); !authorized {
		writeJSONError(w, http.StatusBadRequest, "Trakt 未授权，请先完成设备码配对")
		return
	}
	if connected, _ := s.rt.plexStatusPayload()["token_configured"].(bool); !connected {
		writeJSONError(w, http.StatusBadRequest, "Plex 未授权，请先完成 PIN 配对")
		return
	}
	if !s.rt.TriggerTraktSync(triggerManual) {
		writeJSONError(w, http.StatusConflict, "观看同步已在进行中")
		return
	}
	s.rt.appLog("trakt", "sync_triggered", "INFO", "手动触发观看同步", nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "观看同步已触发"})
}

func (s *Server) handleTraktSyncRuns(w http.ResponseWriter, r *http.Request) {
	store := s.rt.GetTraktSyncStore()
	limit := intQuery(r, "limit", 20)
	offset := intQuery(r, "offset", 0)
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	runs, total, err := store.ListSyncRuns(limit, offset)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("读取同步历史失败：%v", err))
		return
	}
	list := make([]map[string]any, 0, len(runs))
	for _, run := range runs {
		list = append(list, map[string]any{
			"id": run.ID, "started_at": run.StartedAt, "finished_at": run.FinishedAt,
			"trigger": run.Trigger, "to_trakt": run.ToTrakt, "to_plex": run.ToPlex,
			"conflicts": run.Conflicts, "unresolved": run.Unresolved,
			"skipped": run.Skipped, "error": run.Error,
			"title": run.Title, "progress": run.Progress,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"total": total, "runs": list})
}

func (s *Server) handleTraktSyncRunItems(w http.ResponseWriter, r *http.Request) {
	syncStore := s.rt.GetTraktSyncStore()
	runID, err := strconv.ParseInt(r.PathValue("runID"), 10, 64)
	if err != nil || runID <= 0 {
		writeJSONError(w, http.StatusBadRequest, "无效的 run id")
		return
	}
	limit := intQuery(r, "limit", 200)
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	items, err := syncStore.ListRunItems(runID, limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("读取同步明细失败：%v", err))
		return
	}
	enrichRunItemTitles(items)
	list := make([]map[string]any, 0, len(items))
	for _, it := range items {
		list = append(list, map[string]any{
			"id": it.ID, "media_type": it.MediaType, "tmdb_id": it.TmdbID,
			"season": it.Season, "episode": it.Episode, "title": it.Title,
			"direction": it.Direction, "action": it.Action,
			"success": it.Success, "error": it.Error,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

// ── 忽略清单（Trakt 无法识别等持续失败媒体的手动止损）────────

// traktMediaTitle 查媒体展示标题（tmdb_media 缓存优先，回退 library_media）。
func (rt *Runtime) traktMediaTitle(mediaType string, tmdbID int64) string {
	libStore := rt.GetLibraryStore()
	if row := libStore.GetTmdbEntry(mediaType, tmdbID); row != nil && row.Title != "" {
		return row.Title
	}
	return libStore.GetMediaTitle(mediaType, tmdbID)
}

// handleTraktSyncIgnores GET /api/trakt/sync/ignores：忽略清单 + 反复失败候选。
func (s *Server) handleTraktSyncIgnores(w http.ResponseWriter, _ *http.Request) {
	syncStore := s.rt.GetTraktSyncStore()
	ignores := make([]map[string]any, 0)
	for _, it := range syncStore.ListSyncIgnores() {
		ignores = append(ignores, map[string]any{
			"media_type": it.MediaType, "anchor": it.Anchor, "tmdb_id": it.TmdbID,
			"title": it.Title, "reason": it.Reason, "created_at": it.CreatedAt,
		})
	}
	candidates := make([]map[string]any, 0)
	for _, c := range syncStore.FailingMediaCandidates(7) {
		title := ""
		if c.Title != nil {
			title = *c.Title
		}
		if title == "" {
			title = s.rt.traktMediaTitle(c.MediaType, c.TmdbID)
		}
		candidates = append(candidates, map[string]any{
			"media_type": c.MediaType, "anchor": c.Anchor, "tmdb_id": c.TmdbID, "title": title,
			"fails": c.Fails, "last_error": c.LastError, "last_at": c.LastAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ignores": ignores, "candidates": candidates})
}

// handleTraktSyncIgnoreSet POST /api/trakt/sync/ignore
// {action: "add"|"remove", media_type, tmdb_id, reason?, anchor?}。
// anchor 可选："tmdb"（默认）| "tvdb"（tvdb 锚定的剧集键）。
func (s *Server) handleTraktSyncIgnoreSet(w http.ResponseWriter, r *http.Request) {
	syncStore := s.rt.GetTraktSyncStore()
	var req struct {
		Action    string `json:"action"`
		MediaType string `json:"media_type"`
		Anchor    string `json:"anchor"`
		TmdbID    int64  `json:"tmdb_id"`
		Reason    string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if req.Anchor != "tvdb" {
		req.Anchor = "tmdb"
	}
	if (req.MediaType != "movie" && req.MediaType != "tv") || req.TmdbID <= 0 {
		writeJSONError(w, http.StatusBadRequest, "media_type 须为 movie/tv 且 tmdb_id 有效")
		return
	}
	ignoreDisplayName := s.rt.traktMediaTitle(req.MediaType, req.TmdbID)
	if ignoreDisplayName == "" {
		ignoreDisplayName = fmt.Sprintf("%s:%d", req.MediaType, req.TmdbID)
	}
	switch req.Action {
	case "add":
		title := s.rt.traktMediaTitle(req.MediaType, req.TmdbID)
		if err := syncStore.AddSyncIgnore(req.MediaType, req.Anchor, req.TmdbID, title, req.Reason); err != nil {
			writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("保存忽略失败：%v", err))
			return
		}
		s.rt.appLog("trakt", "ignore_added", "INFO",
			fmt.Sprintf("已忽略「%s」，双方都不再同步", ignoreDisplayName), nil)
		writeJSON(w, http.StatusOK, map[string]any{"message": "已忽略，该媒体将不再参与同步"})
	case "remove":
		if err := syncStore.RemoveSyncIgnore(req.MediaType, req.Anchor, req.TmdbID); err != nil {
			writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("解除忽略失败：%v", err))
			return
		}
		s.rt.appLog("trakt", "ignore_removed", "INFO",
			fmt.Sprintf("已解除忽略「%s」，下轮同步自动纳入", ignoreDisplayName), nil)
		writeJSON(w, http.StatusOK, map[string]any{"message": "已恢复，下轮同步自动纳入"})
	default:
		writeJSONError(w, http.StatusBadRequest, "action 须为 add 或 remove")
	}
}

// enrichRunItemTitles 就地为明细补充媒体标题（原位覆盖 Title）。
// 优先 tmdb_media 缓存（含剧名/译名），回退 library_media；两者都只读。
// unresolved 明细在记录时已带播放端标题，不覆盖。
func enrichRunItemTitles(items []store.TraktSyncRunItem) {
	libStore := RT().GetLibraryStore()
	for i := range items {
		it := &items[i]
		if it.MediaType == nil || it.TmdbID == nil || *it.TmdbID <= 0 {
			continue
		}
		if it.Title != nil && *it.Title != "" {
			continue
		}
		mediaType := *it.MediaType
		tmdbID := *it.TmdbID
		title := ""
		if row := libStore.GetTmdbEntry(mediaType, tmdbID); row != nil {
			title = row.Title
		}
		if title == "" {
			title = libStore.GetMediaTitle(mediaType, tmdbID)
		}
		if title != "" {
			it.Title = &title
		}
	}
}

var (
	_ syncengine.ShadowStore = (*store.TraktSyncStore)(nil)
	_ watched.IDMapper       = (*store.TraktSyncStore)(nil)
	_ watched.Provider       = (*plexserver.Source)(nil)
	_ watched.Provider       = (*trakt.TraktSource)(nil)
	_ watched.TitleIndex     = (*titleIndex)(nil)
)
