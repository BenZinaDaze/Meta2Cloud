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
func (s *traktService) TraktSyncRunning() bool {
	s.run.mu.Lock()
	defer s.run.mu.Unlock()
	return s.run.running
}

// TriggerTraktSync 异步触发一轮同步；已在进行中返回 false。
func (s *traktService) TriggerTraktSync(trigger string) bool {
	s.run.mu.Lock()
	if s.run.running {
		s.run.mu.Unlock()
		return false
	}
	s.run.running = true
	s.run.mu.Unlock()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.rt.appLog("trakt", "sync_panic", "ERROR", fmt.Sprintf("观看同步异常退出：%v", r), nil)
			}
			s.run.mu.Lock()
			s.run.running = false
			s.run.mu.Unlock()
		}()
		s.runTraktSync(trigger)
	}()
	return true
}

// traktSyncLogger 引擎日志回调 → appLog。
func (s *traktService) traktSyncLogger(level, message string, details map[string]any) {
	s.rt.appLog("trakt", "sync", level, message, details)
}

// runTraktSync 构建引擎并执行一轮同步。
func (s *traktService) runTraktSync(trigger string) {
	cfg := s.rt.GetConfig()
	traktClient := s.rt.GetTraktClient()
	if traktClient == nil {
		s.rt.appLog("trakt", "sync_skipped", "WARNING", "Trakt 客户端不可用（未配置或未授权），跳过同步", nil)
		return
	}
	plexClient := s.rt.GetPlexClient()
	if plexClient == nil {
		s.rt.appLog("trakt", "sync_skipped", "WARNING", "Plex 未配置服务器地址或未授权，跳过同步", nil)
		return
	}
	syncStore := s.rt.GetTraktSyncStore()
	engine := &syncengine.Engine{
		Playback: plexserver.NewSource(plexClient, s.rt.buildTitleIndex(), syncStore),
		Trakt:    trakt.NewTraktSource(traktClient, syncStore, syncStore),
		Store:    syncStore,
		ToPlex:   cfg.Trakt.SyncToPlex,
		ToTrakt:  cfg.Trakt.SyncToTrakt,
		Log:      s.traktSyncLogger,
	}
	started := time.Now()
	summary, err := engine.Run(context.Background(), trigger)
	took := time.Since(started).Round(time.Millisecond)
	if err != nil {
		s.rt.appLog("trakt", "sync_failed", "ERROR", fmt.Sprintf("观看同步失败（耗时 %s）：%v", took, err), nil)
		// 仅授权失效上报健康状态；瞬时网络错误不告警
		switch {
		case errors.Is(err, trakt.ErrUnauthorized):
			s.rt.NotifyServiceHealth("Trakt", false, "观看同步："+err.Error())
		case errors.Is(err, plexserver.ErrUnauthorized):
			s.rt.NotifyServiceHealth("Plex", false, "观看同步："+err.Error())
		}
		return
	}
	// 全量成功即双向接口可达且授权有效：同时上报 Trakt 与 Plex 恢复
	s.rt.NotifyServiceHealth("Trakt", true, "")
	s.rt.NotifyServiceHealth("Plex", true, "")
	s.rt.appLog("trakt", "sync_done", "SUCCESS",
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
		s.notifyTraktSyncAnomaly(summary)
	}
}

// traktSyncAnomaly 异常提醒节流：持续异常每小时至多一条，避免 15 分钟一轮刷屏。
var (
	traktAnomalyMu   sync.Mutex
	traktAnomalyLast time.Time
)

func (s *traktService) notifyTraktSyncAnomaly(summary syncengine.RunSummary) {
	traktAnomalyMu.Lock()
	if time.Since(traktAnomalyLast) < time.Hour {
		traktAnomalyMu.Unlock()
		return
	}
	traktAnomalyLast = time.Now()
	traktAnomalyMu.Unlock()
	s.rt.sendHealthTelegram("trakt", "Trakt", fmt.Sprintf(
		"⚠️ 观看同步有未处理项：写入失败 %d 条，两侧状态冲突 %d 条，无法识别 %d 条。详情见同步记录",
		summary.Failed, summary.Conflicts, summary.Unresolved))
}

// ── HTTP 处理器：同步状态 / 触发 / 历史 ─────────────────────

// TraktSyncRunPayload 同步运行记录（status.last_run 与 runs 列表共用同一形状）。
type TraktSyncRunPayload struct {
	ID         int64    `json:"id"`
	StartedAt  string   `json:"started_at"`
	FinishedAt *string  `json:"finished_at"`
	Trigger    string   `json:"trigger"`
	ToTrakt    int      `json:"to_trakt"`
	ToPlex     int      `json:"to_plex"`
	Conflicts  int      `json:"conflicts"`
	Unresolved int      `json:"unresolved"`
	Skipped    int      `json:"skipped"`
	Error      *string  `json:"error"`
	Title      *string  `json:"title"`    // 实时推送（sse/poll）记录的媒体标题；全量同步为 null
	Progress   *float64 `json:"progress"` // 实时推送记录的进度；全量同步为 null
}

// TraktSyncStatusPayload GET /api/trakt/sync/status 响应。
type TraktSyncStatusPayload struct {
	Enabled             bool                 `json:"enabled"`
	TraktReady          bool                 `json:"trakt_ready"`
	TraktAuthorized     bool                 `json:"trakt_authorized"`
	PlexReady           bool                 `json:"plex_ready"`
	PlexConnected       bool                 `json:"plex_connected"`
	PlexReachable       bool                 `json:"plex_reachable"`
	SyncToPlex          bool                 `json:"sync_to_plex"`
	SyncToTrakt         bool                 `json:"sync_to_trakt"`
	SyncCron            string               `json:"sync_cron"`
	SessionPollSeconds  int                  `json:"session_poll_seconds"`
	Running             bool                 `json:"running"`
	SyncWatcherAlive    bool                 `json:"sync_watcher_alive"`
	SessionWatcherAlive bool                 `json:"session_watcher_alive"`
	SSEEnabled          bool                 `json:"sse_enabled"`
	SSEConnected        bool                 `json:"sse_connected"`
	LastPolledAt        *string              `json:"last_polled_at"`
	LastPollError       *string              `json:"last_poll_error"`
	LastSessionPolledAt *string              `json:"last_session_polled_at"`
	LastRun             *TraktSyncRunPayload `json:"last_run"`
}

func (s *traktService) traktSyncStatusPayload() TraktSyncStatusPayload {
	cfg := s.rt.GetConfig()
	traktStatus := s.traktOauthStatusPayload()
	plex := s.plexStatusPayload()
	payload := TraktSyncStatusPayload{
		Enabled:             cfg.Trakt.Enabled,
		TraktReady:          traktStatus.Ready,
		TraktAuthorized:     traktStatus.Authorized,
		PlexReady:           plex.Ready,
		PlexConnected:       plex.TokenConfigured,
		PlexReachable:       plex.Reachable,
		SyncToPlex:          cfg.Trakt.SyncToPlex,
		SyncToTrakt:         cfg.Trakt.SyncToTrakt,
		SyncCron:            cfg.Trakt.SyncCron,
		SessionPollSeconds:  cfg.Trakt.SessionPollSeconds,
		Running:             s.TraktSyncRunning(),
		SyncWatcherAlive:    watcherAlive(s.syncWatcher),
		SessionWatcherAlive: watcherAlive(s.sessionWatcher),
		SSEEnabled:          cfg.Trakt.SSEEnabled,
		SSEConnected:        s.sseConnected(),
		LastPolledAt:        s.syncWatcher.lastPolledAt,
		LastPollError:       s.syncWatcher.lastPollError,
		LastSessionPolledAt: s.sessionWatcher.lastPolledAt,
	}
	if store := s.rt.GetTraktSyncStore(); store != nil {
		if last := store.GetLastFinishedSyncRun(); last != nil {
			run := traktSyncRunPayload(*last)
			payload.LastRun = &run
		}
	}
	return payload
}

// traktSyncRunPayload store 运行记录 → API 负载（含实时推送的标题/进度）。
func traktSyncRunPayload(run store.TraktSyncRun) TraktSyncRunPayload {
	return TraktSyncRunPayload{
		ID: run.ID, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
		Trigger: run.Trigger, ToTrakt: run.ToTrakt, ToPlex: run.ToPlex,
		Conflicts: run.Conflicts, Unresolved: run.Unresolved,
		Skipped: run.Skipped, Error: run.Error,
		Title: run.Title, Progress: run.Progress,
	}
}

func (s *Server) handleTraktSyncStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.rt.traktSvc.traktSyncStatusPayload())
}

func (s *Server) handleTraktSyncTrigger(w http.ResponseWriter, _ *http.Request) {
	if !s.rt.traktSvc.traktOauthStatusPayload().Authorized {
		writeJSONError(w, http.StatusBadRequest, "Trakt 未授权，请先完成设备码配对")
		return
	}
	if !s.rt.traktSvc.plexStatusPayload().TokenConfigured {
		writeJSONError(w, http.StatusBadRequest, "Plex 未授权，请先完成 PIN 配对")
		return
	}
	if !s.rt.traktSvc.TriggerTraktSync(triggerManual) {
		writeJSONError(w, http.StatusConflict, "观看同步已在进行中")
		return
	}
	s.rt.appLog("trakt", "sync_triggered", "INFO", "手动触发观看同步", nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "观看同步已触发"})
}

// TraktSyncRunsPayload GET /api/trakt/sync/runs 响应。
type TraktSyncRunsPayload struct {
	Total int                   `json:"total"`
	Runs  []TraktSyncRunPayload `json:"runs"`
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
	list := make([]TraktSyncRunPayload, 0, len(runs))
	for _, run := range runs {
		list = append(list, traktSyncRunPayload(run))
	}
	writeJSON(w, http.StatusOK, TraktSyncRunsPayload{Total: total, Runs: list})
}

// TraktSyncRunItemPayload GET /api/trakt/sync/runs/{id}/items 单条明细。
type TraktSyncRunItemPayload struct {
	ID        int64   `json:"id"`
	MediaType *string `json:"media_type"`
	TmdbID    *int64  `json:"tmdb_id"`
	Season    *int    `json:"season"`
	Episode   *int    `json:"episode"`
	Title     *string `json:"title"`
	Direction string  `json:"direction"`
	Action    string  `json:"action"`
	Success   bool    `json:"success"`
	Error     *string `json:"error"`
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
	s.rt.traktSvc.enrichRunItemTitles(items)
	list := make([]TraktSyncRunItemPayload, 0, len(items))
	for _, it := range items {
		list = append(list, TraktSyncRunItemPayload{
			ID: it.ID, MediaType: it.MediaType, TmdbID: it.TmdbID,
			Season: it.Season, Episode: it.Episode, Title: it.Title,
			Direction: it.Direction, Action: it.Action,
			Success: it.Success, Error: it.Error,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

// ── 忽略清单（Trakt 无法识别等持续失败媒体的手动止损）────────

// TraktSyncIgnorePayload 忽略清单条目。
type TraktSyncIgnorePayload struct {
	MediaType string  `json:"media_type"`
	Anchor    string  `json:"anchor"`
	TmdbID    int64   `json:"tmdb_id"`
	Title     *string `json:"title"`
	Reason    *string `json:"reason"`
	CreatedAt string  `json:"created_at"`
}

// TraktFailingMediaPayload 反复失败候选（建议忽略）。
type TraktFailingMediaPayload struct {
	MediaType string  `json:"media_type"`
	Anchor    string  `json:"anchor"`
	TmdbID    int64   `json:"tmdb_id"`
	Title     string  `json:"title"`
	Fails     int     `json:"fails"`
	LastError *string `json:"last_error"`
	LastAt    string  `json:"last_at"`
}

// TraktSyncIgnoresPayload GET /api/trakt/sync/ignores 响应。
type TraktSyncIgnoresPayload struct {
	Ignores    []TraktSyncIgnorePayload   `json:"ignores"`
	Candidates []TraktFailingMediaPayload `json:"candidates"`
}

// handleTraktSyncIgnores GET /api/trakt/sync/ignores：忽略清单 + 反复失败候选。
func (s *Server) handleTraktSyncIgnores(w http.ResponseWriter, _ *http.Request) {
	syncStore := s.rt.GetTraktSyncStore()
	ignores := make([]TraktSyncIgnorePayload, 0)
	for _, it := range syncStore.ListSyncIgnores() {
		ignores = append(ignores, TraktSyncIgnorePayload{
			MediaType: it.MediaType, Anchor: it.Anchor, TmdbID: it.TmdbID,
			Title: it.Title, Reason: it.Reason, CreatedAt: it.CreatedAt,
		})
	}
	candidates := make([]TraktFailingMediaPayload, 0)
	for _, c := range syncStore.FailingMediaCandidates(7) {
		title := ""
		if c.Title != nil {
			title = *c.Title
		}
		if title == "" {
			title = s.rt.traktSvc.traktMediaTitle(c.MediaType, c.TmdbID)
		}
		candidates = append(candidates, TraktFailingMediaPayload{
			MediaType: c.MediaType, Anchor: c.Anchor, TmdbID: c.TmdbID, Title: title,
			Fails: c.Fails, LastError: c.LastError, LastAt: c.LastAt,
		})
	}
	writeJSON(w, http.StatusOK, TraktSyncIgnoresPayload{Ignores: ignores, Candidates: candidates})
}

// traktMediaTitle 查媒体展示标题（tmdb_media 缓存优先，回退 library_media）。
func (s *traktService) traktMediaTitle(mediaType string, tmdbID int64) string {
	libStore := s.rt.GetLibraryStore()
	if row := libStore.GetTmdbEntry(mediaType, tmdbID); row != nil && row.Title != "" {
		return row.Title
	}
	return libStore.GetMediaTitle(mediaType, tmdbID)
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
	ignoreDisplayName := s.rt.traktSvc.traktMediaTitle(req.MediaType, req.TmdbID)
	if ignoreDisplayName == "" {
		ignoreDisplayName = fmt.Sprintf("%s:%d", req.MediaType, req.TmdbID)
	}
	switch req.Action {
	case "add":
		title := s.rt.traktSvc.traktMediaTitle(req.MediaType, req.TmdbID)
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
func (s *traktService) enrichRunItemTitles(items []store.TraktSyncRunItem) {
	libStore := s.rt.GetLibraryStore()
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
