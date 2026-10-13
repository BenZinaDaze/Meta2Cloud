// server.go —— WebUI HTTP 服务（对应 webui/app.py）。
//
// 路由使用 Go 1.22+ 标准库 ServeMux（方法 + 路径模式），
// 认证中间件与 CORS 与 Python 版语义一致。
package webui

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"meta2cloud/internal/webui/store"
)

// Server WebUI 服务。
type Server struct {
	mux    *http.ServeMux
	rootFS http.Handler // 前端静态资源（可为 nil）
	stop   chan struct{}
}

// NewServer 构造服务（注册全部路由）。
func NewServer() *Server {
	s := &Server{mux: http.NewServeMux(), stop: make(chan struct{})}
	s.registerRoutes()
	s.rootFS = loadStaticFiles()
	RT().startTmdbEvictLoop(s.stop)
	StartupBackgroundWatchers()
	return s
}

// Close 释放后台任务。
func (s *Server) Close() { close(s.stop) }

// loadStaticFiles 加载 frontend/dist（存在时）。
func loadStaticFiles() http.Handler {
	staticDir := filepath.Join(RT().RootDir, "frontend", "dist")
	if st, err := os.Stat(staticDir); err == nil && st.IsDir() {
		return http.FileServer(http.Dir(staticDir))
	}
	return nil
}

func (s *Server) registerRoutes() {
	m := s.mux

	// auth
	m.HandleFunc("POST /api/auth:login", handleAuthLogin)
	m.HandleFunc("POST /api/auth:logout", handleAuthLogout)
	m.HandleFunc("GET /api/auth/me", handleAuthMe)

	// library / stats
	m.HandleFunc("GET /api/library", handleGetLibrary)
	m.HandleFunc("POST /api/library:refresh", handleRefreshLibrary)
	m.HandleFunc("GET /api/stats", handleGetStats)
	m.HandleFunc("DELETE /api/library/{driveFolderID}", handleDeleteLibraryItem)
	m.HandleFunc("GET /api/movies", handleGetMovies)
	m.HandleFunc("GET /api/tv-shows", handleGetTVShows)
	m.HandleFunc("GET /api/tv-shows/{tmdbID}", handleGetTVDetail)

	// pipeline / webhook
	m.HandleFunc("POST /trigger", handleTriggerWebhook)
	m.HandleFunc("GET /api/pipeline/status", handlePipelineStatus)
	m.HandleFunc("POST /api/pipeline/trigger", handlePipelineTrigger)

	// config
	m.HandleFunc("GET /api/config", handleReadConfig)
	m.HandleFunc("GET /api/config/main", handleReadMainConfig)
	m.HandleFunc("GET /api/config/parser-rules", handleReadParserRules)
	m.HandleFunc("POST /api/parser/test", handleParserTest)
	m.HandleFunc("PUT /api/config", handleWriteConfig)
	m.HandleFunc("PUT /api/config/main", handleWriteMainConfig)
	m.HandleFunc("PUT /api/config/parser-rules", handleWriteParserRules)

	// aria2
	m.HandleFunc("GET /api/aria2/overview", handleAria2Overview)
	m.HandleFunc("GET /api/aria2/options", handleAria2OptionsGet)
	m.HandleFunc("PUT /api/aria2/options", handleAria2OptionsPut)
	m.HandleFunc("POST /api/aria2/add-uri", handleAria2AddURI)
	m.HandleFunc("POST /api/aria2/add-torrent", handleAria2AddTorrent)
	m.HandleFunc("POST /api/aria2/tasks/pause", handleAria2TasksPause)
	m.HandleFunc("POST /api/aria2/tasks/unpause", handleAria2TasksUnpause)
	m.HandleFunc("POST /api/aria2/tasks/remove", handleAria2TasksRemove)
	m.HandleFunc("POST /api/aria2/tasks/retry", handleAria2TasksRetry)
	m.HandleFunc("POST /api/aria2/tasks/purge", handleAria2TasksPurge)

	// mediavault / tmdb season
	m.HandleFunc("POST /api/mediavault/test", handleMediavaultTest)
	m.HandleFunc("GET /api/tmdb/tv/{tmdbID}/season/{season}", handleTmdbSeasonDetail)

	// file manager
	m.HandleFunc("GET /api/files", handleListFiles)
	m.HandleFunc("GET /api/files/{fileID}", handleGetFile)
	m.HandleFunc("POST /api/files", handleCreateFile)
	m.HandleFunc("PATCH /api/files/{fileID}", handleUpdateFile)
	m.HandleFunc("DELETE /api/files/{fileID}", handleDeleteFile)
	m.HandleFunc("POST /api/files/batch", handleFileBatch)

	// media actions / tmdb / scraper / bangumi
	m.HandleFunc("POST /api/library:refresh-item", handleRefreshItem)
	m.HandleFunc("POST /api/library:reidentify", handleReidentifyItem)
	m.HandleFunc("GET /api/cache", handleCacheStats)
	m.HandleFunc("POST /api/cache:evict", handleCacheEvict)
	m.HandleFunc("GET /api/health", handleHealth)
	m.HandleFunc("GET /api/tmdb/search", handleTmdbSearch)
	m.HandleFunc("GET /api/tmdb/{tmdbID}", handleTmdbDetail)
	m.HandleFunc("GET /api/tmdb/{tmdbID}/alternative-names", handleTmdbAlternativeNames)
	m.HandleFunc("GET /api/scraper/search", handleScraperSearch)
	m.HandleFunc("GET /api/scraper/sites", handleScraperSites)
	m.HandleFunc("GET /api/scraper/episodes", handleScraperEpisodes)
	m.HandleFunc("GET /api/bangumi/calendar", handleBangumiCalendar)

	// subscriptions
	m.HandleFunc("GET /api/subscriptions", handleListSubscriptions)
	m.HandleFunc("POST /api/subscriptions", handleCreateSubscription)
	m.HandleFunc("POST /api/subscriptions:test", handleTestSubscription)
	m.HandleFunc("POST /api/subscriptions:parse-rss", handleParseSubscriptionRss)
	m.HandleFunc("GET /api/subscriptions/{subscriptionID}", handleGetSubscription)
	m.HandleFunc("PUT /api/subscriptions/{subscriptionID}", handleUpdateSubscription)
	m.HandleFunc("DELETE /api/subscriptions/{subscriptionID}", handleDeleteSubscription)
	m.HandleFunc("POST /api/subscriptions/{subscriptionID}", handleSubscriptionPostOp)

	// u115 (OAuth + 云下载)
	m.HandleFunc("GET /api/u115/oauth/status", handleU115OauthStatus)
	m.HandleFunc("POST /api/u115/oauth/create", handleU115OauthCreate)
	m.HandleFunc("GET /api/u115/oauth/qrcode", handleU115OauthQrcode)
	m.HandleFunc("GET /api/u115/oauth/poll", handleU115OauthPoll)
	m.HandleFunc("POST /api/u115/oauth/exchange", handleU115OauthExchange)
	m.HandleFunc("POST /api/u115/test", handleU115TestConnection)
	m.HandleFunc("POST /api/u115/test-cookie", handleU115TestCookie)
	m.HandleFunc("GET /api/u115/offline/overview", handleU115OfflineOverview)
	m.HandleFunc("GET /api/u115/offline/auto-organize-status", handleU115AutoOrganizeStatus)
	m.HandleFunc("GET /api/u115/offline/quota", handleU115OfflineQuota)
	m.HandleFunc("POST /api/u115/offline/add-urls", handleU115OfflineAddURLs)
	m.HandleFunc("POST /api/u115/offline/tasks/delete", handleU115OfflineDeleteTasks)
	m.HandleFunc("POST /api/u115/offline/tasks/clear", handleU115OfflineClearTasks)

	// logs / ingest
	m.HandleFunc("GET /api/logs", handleGetLogs)
	m.HandleFunc("GET /api/ingest/history", handleIngestHistory)
	m.HandleFunc("GET /api/ingest/stats", handleIngestStats)
	m.HandleFunc("GET /api/ingest/recent", handleIngestRecent)

	// 兜底：静态资源 / SPA
	m.HandleFunc("/", s.handleRoot)
}

// Handler 最终 HTTP 处理器（认证 + CORS）。
func (s *Server) Handler() http.Handler {
	var handler http.Handler = s.mux
	handler = authMiddleware(handler)
	handler = corsMiddleware(handler)
	return handler
}

// ListenAndServe 在 addr 上启动 HTTP 服务。
func ListenAndServe(addr string, s *Server) error {
	return http.ListenAndServe(addr, s.Handler())
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleRoot 静态资源 / SPA 兜底。
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if s.rootFS == nil {
		http.NotFound(w, r)
		return
	}
	// 已存在的文件直接返回，其余回退 index.html（SPA 路由）
	path := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if path == "." {
		path = "index.html"
	}
	if _, err := os.Stat(filepath.Join(RT().RootDir, "frontend", "dist", path)); err != nil {
		// SPA 回退
		data, err := os.ReadFile(filepath.Join(RT().RootDir, "frontend", "dist", "index.html"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
		return
	}
	s.rootFS.ServeHTTP(w, r)
}

// ── 响应工具 ────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}

// ── auth 处理器 ─────────────────────────────────────────────

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	var body loginRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	cfg := RT().GetConfig()
	if cfg.WebUI.Password == "" {
		writeJSONError(w, http.StatusForbidden, "密码未设置，请先在配置文件中设置 webui.password")
		return
	}
	if !RT().CredentialsMatch(body.Username, body.Password) {
		writeJSONError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	token, err := CreateToken(body.Username, cfg.WebUI.TokenExpireHours)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Token 生成失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":        token,
		"username":     body.Username,
		"expire_hours": cfg.WebUI.TokenExpireHours,
	})
}

func handleAuthLogout(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func handleAuthMe(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	token := strings.TrimPrefix(auth, "Bearer ")
	username := VerifyToken(token)
	if username == "" {
		writeJSONError(w, http.StatusUnauthorized, "Token 无效")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"username": username})
}

// ── library / stats 处理器 ──────────────────────────────────

func handleGetLibrary(w http.ResponseWriter, r *http.Request) {
	sortBy := r.URL.Query().Get("sort_by")
	if sortBy == "" {
		sortBy = "folder_modified_time"
	}
	sortOrder := r.URL.Query().Get("sort_order")
	if sortOrder == "" {
		sortOrder = "desc"
	}
	libStore := RT().GetLibraryStore()
	if libStore == nil {
		writeJSONError(w, http.StatusInternalServerError, "媒体库存储不可用")
		return
	}
	snapshot := libStore.GetSnapshot(sortBy, sortOrder)
	if snapshot == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"movies": []any{}, "tv_shows": []any{},
			"total_movies": 0, "total_tv": 0, "scanned_at": nil,
			"hint": "媒体库尚未扫描，请点击「刷新媒体库」按钮",
		})
		return
	}
	writeJSON(w, http.StatusOK, snapshotResponse(snapshot, ""))
}

func snapshotResponse(snapshot *store.Snapshot, hint string) map[string]any {
	resp := map[string]any{
		"movies":       snapshot.Movies,
		"tv_shows":     snapshot.TvShows,
		"total_movies": snapshot.TotalMovies,
		"total_tv":     snapshot.TotalTV,
		"scanned_at":   snapshot.ScannedAt,
	}
	if hint != "" {
		resp["hint"] = hint
	}
	return resp
}

func handleGetStats(w http.ResponseWriter, _ *http.Request) {
	libStore := RT().GetLibraryStore()
	if libStore == nil {
		writeJSONError(w, http.StatusInternalServerError, "媒体库存储不可用")
		return
	}
	writeJSON(w, http.StatusOK, libStore.GetStats())
}

func handleDeleteLibraryItem(w http.ResponseWriter, r *http.Request) {
	driveFolderID := r.PathValue("driveFolderID")
	if driveFolderID == "" {
		writeJSONError(w, http.StatusBadRequest, "缺少 drive_folder_id")
		return
	}
	libStore := RT().GetLibraryStore()
	if libStore == nil {
		writeJSONError(w, http.StatusInternalServerError, "媒体库存储不可用")
		return
	}
	if !libStore.DeleteLibraryItem(driveFolderID) {
		writeJSONError(w, http.StatusNotFound, "媒体库条目不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "drive_folder_id": driveFolderID})
}

// 编译期引用检查（保持 import 干净）
