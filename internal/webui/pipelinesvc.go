// pipelinesvc.go —— Web 触发的整理流程调度（对应 webui/services/pipeline.py）。
package webui

import (
	"crypto/subtle"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"panoply/internal/pipeline"
	"panoply/internal/webui/store"
)

// appLog 写应用日志（库不可用时静默）。
func (rt *Runtime) appLog(category, event, level, message string, details map[string]any) {
	if ls := rt.GetLogStore(); ls != nil {
		entry := ls.Write(category, event, level, message, details)
		// 统一日志出口：落库（日志页 + WS 实时）并镜像到进程 stdout，
		// 终端/docker logs 与「日志」页观察同一份事件
		log.Printf("[%s/%s] %s %s", category, event, level, message)
		rt.broadcastEntry(entry)
	}
}

// ingestStoreAdapter 将 *store.IngestStore 适配为 pipeline.IngestStore 接口。
type ingestStoreAdapter struct{ s *store.IngestStore }

func (a ingestStoreAdapter) RecordIngest(r pipeline.IngestRecord) error {
	_, err := a.s.RecordIngest(store.IngestRecord{
		MediaType:     r.MediaType,
		TmdbID:        int64(r.TmdbID),
		Title:         r.Title,
		OriginalTitle: r.OriginalTitle,
		Year:          r.Year,
		Season:        r.Season,
		Episode:       r.Episode,
		EpisodeTitle:  r.EpisodeTitle,
		PosterPath:    r.PosterPath,
		DriveFolderID: r.DriveFolderID,
		OriginalName:  r.OriginalName,
		Status:        r.Status,
		ErrorMessage:  r.ErrorMessage,
	})
	return err
}

// titlePinAdapter 将 *store.LibraryStore 适配为 pipeline.TitlePinStore 接口。
type titlePinAdapter struct{ s *store.LibraryStore }

func (a titlePinAdapter) ResolveTitlePin(mediaType string, tmdbID int64, fallbackTitle, fallbackYear string) (string, string, error) {
	return a.s.ResolveTitlePin(mediaType, tmdbID, fallbackTitle, fallbackYear)
}

// duplicateFolderAdapter 将 *store.LibraryStore 适配为 pipeline.DuplicateFolderSource 接口。
type duplicateFolderAdapter struct{ s *store.LibraryStore }

func (a duplicateFolderAdapter) DuplicateFolderGroups() []pipeline.DuplicateFolderGroup {
	groups, err := a.s.ListDuplicateFolderGroups()
	if err != nil {
		log.Printf("查询重复目录分组失败：%v", err)
		return nil
	}
	out := make([]pipeline.DuplicateFolderGroup, 0, len(groups))
	for _, g := range groups {
		folders := make([]pipeline.DuplicateFolderInfo, 0, len(g.Folders))
		for _, f := range g.Folders {
			folders = append(folders, pipeline.DuplicateFolderInfo{ID: f.ID, ModifiedTime: f.ModifiedTime})
		}
		out = append(out, pipeline.DuplicateFolderGroup{
			MediaType: g.MediaType,
			TmdbID:    g.TmdbID,
			Folders:   folders,
		})
	}
	return out
}

// pipelineService 整理管线域服务：单飞调度 + 防抖 + 执行与状态负载。
type pipelineService struct {
	rt *Runtime

	mu         sync.Mutex
	debounce   *time.Timer
	running    bool
	runCounter int
}

// runID 生成短运行 ID（DI-2 迁移时曾丢失递增，所有运行都变成 run000000，已恢复）。
func (s *pipelineService) newRunID() string {
	s.mu.Lock()
	s.runCounter++
	n := s.runCounter
	s.mu.Unlock()
	return fmt.Sprintf("run%06d", n)
}

// doRunPipeline 执行整理流程并刷新媒体库。
func (s *pipelineService) doRunPipeline() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.debounce = nil
	s.running = true
	s.mu.Unlock()

	runID := s.newRunID()
	s.rt.appLog("pipeline", "pipeline_start", "INFO", "整理流程启动", map[string]any{"runId": runID})

	cfg := s.rt.GetConfig()
	provider := s.rt.GetStorageProvider()
	if provider == nil {
		s.rt.appLog("pipeline", "pipeline_exception", "ERROR", "整理流程执行异常", map[string]any{
			"runId": runID, "error": "存储 Provider 不可用",
		})
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		return
	}

	opts := pipeline.Options{
		LogFn: func(level, message string) {
			s.rt.BroadcastLog(level, message, runID)
		},
		TmdbCache: s.rt.GetTmdbCache(),
	}
	if is := s.rt.GetIngestStore(); is != nil {
		opts.IngestStore = ingestStoreAdapter{is}
	}
	if ls := s.rt.GetLibraryStore(); ls != nil {
		opts.TitlePinStore = titlePinAdapter{ls}
		opts.DuplicateFolders = duplicateFolderAdapter{ls}
	}
	opts.Categories = s.rt.GetCategories()
	if cfg.Strm.Enabled {
		opts.OnVideoIngested = s.rt.strmSvc.HandleVideoIngested
	}
	pipe := pipeline.New(provider, cfg, opts)
	summary, runErr := pipe.Run()

	finishLevel, finishMsg := "SUCCESS", "整理流程完成"
	if runErr != nil {
		finishLevel, finishMsg = "ERROR", "整理流程中止："+runErr.Error()
	}
	s.rt.appLog("pipeline", "pipeline_finish", finishLevel, finishMsg, map[string]any{
		"runId": runID, "okCount": summary.OKCount,
		"skippedCount": summary.SkippedCount, "failedCount": summary.FailedCount,
	})

	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("媒体库刷新异常：%v", r)
			}
		}()
		s.rt.librarySvc.RefreshLibrary(true)
	}()
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}

// SchedulePipeline 调度整理流程（debounce>0 时防抖合并）。
// 返回是否受理（已在运行且无防抖时拒绝）。
func (s *pipelineService) SchedulePipeline(debounce int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if debounce > 0 {
		if s.debounce != nil {
			s.debounce.Stop()
			log.Printf("整理流程防抖计时器已重置 | 防抖：%d 秒", debounce)
		} else {
			s.rt.appLog("pipeline", "pipeline_schedule", "INFO", "检测到媒体变化，等待稳定后开始整理",
				map[string]any{"debounceSeconds": debounce})
		}
		s.debounce = time.AfterFunc(time.Duration(debounce)*time.Second, s.doRunPipeline)
		return true
	}
	if s.running {
		s.rt.appLog("pipeline", "pipeline_skip_running", "WARNING", "整理流程已在运行，跳过本次触发", nil)
		return false
	}
	s.rt.appLog("pipeline", "pipeline_schedule", "INFO", "整理流程立即执行", map[string]any{"debounceSeconds": 0})
	go s.doRunPipeline()
	return true
}

// PipelineStatusPayload 当前调度状态。
// PipelineStatusPayload GET /api/pipeline/status 响应。
type PipelineStatusPayload struct {
	Running  bool `json:"running"`
	Debounce bool `json:"debounce"`
}

func (s *pipelineService) PipelineStatusPayload() PipelineStatusPayload {
	s.mu.Lock()
	defer s.mu.Unlock()
	return PipelineStatusPayload{Running: s.running, Debounce: s.debounce != nil}
}

// ── HTTP 处理器 ─────────────────────────────────────────────

// isLoopbackRequest 判断请求是否来自本机（webhook 未配置密钥时的兜底限制，
// 防止整理流水线被局域网内任意请求触发）。
func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return host == "127.0.0.1" || host == "::1" || strings.EqualFold(host, "localhost")
}

func (s *Server) handleTriggerWebhook(w http.ResponseWriter, r *http.Request) {
	cfg := s.rt.GetConfig()
	if ws := cfg.WebUI.WebhookSecret; ws != "" {
		provided := r.Header.Get("X-Webhook-Secret")
		if provided == "" {
			provided = r.URL.Query().Get("secret")
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(ws)) != 1 {
			writeJSONError(w, http.StatusForbidden, "Invalid webhook secret")
			return
		}
	} else if !isLoopbackRequest(r) {
		// 未配置密钥（旧配置升级前）时仅允许本机触发；密钥由启动时自动生成（config.EnsureWebhookSecret）
		writeJSONError(w, http.StatusForbidden, "未配置 webhook 密钥，仅允许本机触发；请在 WebUI 配置页设置 webui.webhook_secret")
		return
	}
	debounce := cfg.Telegram.DebounceSeconds
	log.Printf("收到整理触发请求 | 来源：webhook，防抖：%d 秒", debounce)
	status := "triggered"
	if s.rt.pipelineSvc.SchedulePipeline(debounce) && debounce > 0 {
		status = "scheduled"
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}

func (s *Server) handlePipelineStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.rt.pipelineSvc.PipelineStatusPayload())
}

func (s *Server) handlePipelineTrigger(w http.ResponseWriter, _ *http.Request) {
	s.rt.pipelineSvc.SchedulePipeline(0)
	writeJSON(w, http.StatusOK, map[string]string{"status": "triggered"})
}

func (s *Server) handleRefreshLibrary(w http.ResponseWriter, r *http.Request) {
	full := r.URL.Query().Get("full") == "true"
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("刷新媒体库失败: %v", rec)
			s.rt.appLog("library", "refresh_failed", "ERROR", "媒体库刷新失败",
				map[string]any{"error": fmt.Sprintf("%v", rec)})
			writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("%v", rec))
		}
	}()
	diff := s.rt.librarySvc.RefreshLibrary(!full)
	s.rt.appLog("library", "refresh_scan_finish", "SUCCESS", "媒体库刷新完成", diff)
	writeJSON(w, http.StatusOK, diff)
}

func (s *Server) handleGetMovies(w http.ResponseWriter, _ *http.Request) {
	client := s.rt.GetStorageProvider()
	if client == nil {
		writeJSONError(w, http.StatusInternalServerError, "存储 Provider 不可用")
		return
	}
	writeJSON(w, http.StatusOK, s.rt.librarySvc.ScanMovies(client, s.rt.GetConfig()))
}

func (s *Server) handleGetTVShows(w http.ResponseWriter, _ *http.Request) {
	client := s.rt.GetStorageProvider()
	if client == nil {
		writeJSONError(w, http.StatusInternalServerError, "存储 Provider 不可用")
		return
	}
	writeJSON(w, http.StatusOK, s.rt.librarySvc.ScanTVShows(client, s.rt.GetConfig(), true))
}

func (s *Server) handleGetTVDetail(w http.ResponseWriter, r *http.Request) {
	tmdbID, err := strconvParseInt(r.PathValue("tmdbID"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "tmdb_id 无效")
		return
	}
	if item := s.rt.GetLibraryStore().GetLibraryItemByTmdb("tv", tmdbID); item != nil {
		writeJSON(w, http.StatusOK, item)
		return
	}
	client := s.rt.GetStorageProvider()
	if client == nil {
		writeJSONError(w, http.StatusInternalServerError, "存储 Provider 不可用")
		return
	}
	for _, show := range s.rt.librarySvc.ScanTVShows(client, s.rt.GetConfig(), true) {
		if show.TmdbID == tmdbID {
			writeJSON(w, http.StatusOK, show)
			return
		}
	}
	writeJSONError(w, http.StatusNotFound, fmt.Sprintf("未找到 tmdb_id=%d 的剧集", tmdbID))
}

func (s *Server) handleGetMovieDetail(w http.ResponseWriter, r *http.Request) {
	tmdbID, err := strconvParseInt(r.PathValue("tmdbID"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "tmdb_id 无效")
		return
	}
	if item := s.rt.GetLibraryStore().GetLibraryItemByTmdb("movie", tmdbID); item != nil {
		writeJSON(w, http.StatusOK, item)
		return
	}
	client := s.rt.GetStorageProvider()
	if client == nil {
		writeJSONError(w, http.StatusInternalServerError, "存储 Provider 不可用")
		return
	}
	for _, movie := range s.rt.librarySvc.ScanMovies(client, s.rt.GetConfig()) {
		if movie.TmdbID == tmdbID {
			writeJSON(w, http.StatusOK, movie)
			return
		}
	}
	writeJSONError(w, http.StatusNotFound, fmt.Sprintf("未找到 tmdb_id=%d 的电影", tmdbID))
}

func strconvParseInt(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func (s *Server) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	// 与 Python logs_payload 一致：{items, summary, maxId}，并同步保留天数配置
	ls := s.rt.GetLogStore()
	ls.SetRetentionDays(s.rt.GetConfig().WebUI.LogRetentionDays)
	limit := intQuery(r, "limit", 200)
	items := ls.Read(limit, r.URL.Query().Get("category"), r.URL.Query().Get("level"), 0)
	if items == nil {
		items = []store.LogEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":   items,
		"summary": ls.Summary(500),
		"maxId":   ls.MaxID(),
	})
}

func (s *Server) handleIngestHistory(w http.ResponseWriter, r *http.Request) {
	is := s.rt.GetIngestStore()
	q := r.URL.Query()
	result := is.Query(ingestQueryFromParams(q))
	for i := range result.Items {
		if result.Items[i].PosterPath != "" {
			result.Items[i].PosterURL = s.rt.librarySvc.tmdbImageURL(result.Items[i].PosterPath, "w500")
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleIngestStats(w http.ResponseWriter, r *http.Request) {
	is := s.rt.GetIngestStore()
	days := intQuery(r, "days", 7)
	writeJSON(w, http.StatusOK, is.Stats(days))
}

func (s *Server) handleIngestDailyStats(w http.ResponseWriter, r *http.Request) {
	is := s.rt.GetIngestStore()
	writeJSON(w, http.StatusOK, is.DailyStats(intQuery(r, "days", 30)))
}

func (s *Server) handleIngestRecent(w http.ResponseWriter, r *http.Request) {
	is := s.rt.GetIngestStore()
	items := is.GetRecent(intQuery(r, "limit", 10))
	if items == nil {
		items = []store.IngestRecordRow{}
	}
	for i := range items {
		if items[i].PosterPath != "" {
			items[i].PosterURL = s.rt.librarySvc.tmdbImageURL(items[i].PosterPath, "w500")
		}
	}
	writeJSON(w, http.StatusOK, items)
}
