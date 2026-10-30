// u115svc.go —— 115 授权与云下载服务（对应 webui/services/u115.py + routes/u115.py）。
package webui

import (
	"crypto/sha1"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"panoply/internal/u115"
)

// resolveConfigPath 相对路径基于项目根目录解析。
func (rt *Runtime) resolveConfigPath(pathStr string) string {
	if pathStr == "" {
		return ""
	}
	if filepath.IsAbs(pathStr) {
		return pathStr
	}
	return filepath.Join(rt.RootDir, pathStr)
}

// ── 扫码会话持久化 ──────────────────────────────────────────

type deviceSessionJSON struct {
	Qrcode       string `json:"qrcode"`
	UID          string `json:"uid"`
	TimeValue    string `json:"time_value"`
	Sign         string `json:"sign"`
	CodeVerifier string `json:"code_verifier"`
}

func (s *Server) saveU115DeviceSession(session u115.DeviceCodeSession, sessionPath string) error {
	return saveJSONSession(sessionPath, &deviceSessionJSON{
		Qrcode: session.Qrcode, UID: session.UID, TimeValue: session.TimeValue,
		Sign: session.Sign, CodeVerifier: session.CodeVerifier,
	})
}

// loadU115DeviceSession 读取扫码会话；不存在返回错误。
func (s *Server) loadU115DeviceSession(sessionPath string) (*u115.DeviceCodeSession, error) {
	raw, err := loadJSONSession[deviceSessionJSON](sessionPath)
	if err != nil {
		return nil, missingSessionErr(err, fmt.Sprintf("115 扫码会话文件不存在：%s", sessionPath))
	}
	return &u115.DeviceCodeSession{
		Qrcode: raw.Qrcode, UID: raw.UID, TimeValue: raw.TimeValue,
		Sign: raw.Sign, CodeVerifier: raw.CodeVerifier,
	}, nil
}

// ── OAuth 状态 ──────────────────────────────────────────────

// U115OauthStatusPayload GET /api/u115/oauth/status 响应（也内嵌于扫码创建/交换响应的 status 字段）。
type U115OauthStatusPayload struct {
	ClientID         string `json:"client_id"`
	TokenPath        string `json:"token_path"`
	SessionPath      string `json:"session_path"`
	CookieConfigured bool   `json:"cookie_configured"`
	TokenExists      bool   `json:"token_exists"`
	SessionExists    bool   `json:"session_exists"`
	Authorized       bool   `json:"authorized"`
	TokenValid       bool   `json:"token_valid"`
	TokenExpired     bool   `json:"token_expired"`
	RefreshTime      *int64 `json:"refresh_time"`
	ExpiresAt        *int64 `json:"expires_at"`
	Refreshed        bool   `json:"refreshed"`
	RefreshError     string `json:"refresh_error"`
}

func (rt *Runtime) u115OauthStatusPayload() U115OauthStatusPayload {
	cfg := rt.GetConfig().U115
	tokenPath := rt.resolveConfigPath(cfg.TokenJSON)
	sessionPath := rt.resolveConfigPath(cfg.SessionJSON)
	_, tokenStatErr := os.Stat(tokenPath)
	tokenExists := tokenStatErr == nil
	_, sessionStatErr := os.Stat(sessionPath)
	sessionExists := sessionStatErr == nil

	payload := U115OauthStatusPayload{
		ClientID:         cfg.ClientID,
		TokenPath:        cfg.TokenJSON,
		SessionPath:      cfg.SessionJSON,
		CookieConfigured: strings.TrimSpace(cfg.Cookie) != "",
		TokenExists:      tokenExists,
		SessionExists:    sessionExists,
	}
	if !tokenExists {
		return payload
	}
	token, ok, _ := u115.LoadToken(tokenPath)
	if !ok {
		return payload
	}
	refreshTime := token.RefreshTime
	expiresAt := token.ExpiresAt()
	payload.RefreshTime = &refreshTime
	payload.ExpiresAt = &expiresAt
	payload.TokenExpired = time.Now().Unix() >= expiresAt
	if payload.TokenExpired {
		// 尝试自动刷新
		if client := rt.GetU115Client(); client != nil {
			if refreshedToken, err := client.RefreshToken(); err == nil {
				payload.Refreshed = true
				rt := refreshedToken.RefreshTime
				exp := refreshedToken.ExpiresAt()
				payload.RefreshTime = &rt
				payload.ExpiresAt = &exp
				payload.TokenExpired = false
				payload.TokenValid = true
				payload.Authorized = true
			} else {
				payload.RefreshError = err.Error() // 刷新被拒：后台轮询凭此区分「刷新被拒」与「未配置」
			}
		}
	} else {
		payload.TokenValid = true
		payload.Authorized = true
	}
	return payload
}

// ── HTTP 处理器 ─────────────────────────────────────────────

func (rt *Runtime) u115AuthError(w http.ResponseWriter, err error) {
	if u115.IsAuth(err) {
		writeJSONError(w, http.StatusBadRequest, "115 Token 已失效，请重新扫码授权")
		return
	}
	writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("115 接口调用失败：%v", err))
}

func (s *Server) handleU115OauthStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.rt.u115OauthStatusPayload())
}

func (s *Server) handleU115OauthCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ClientID  string `json:"client_id"`
		TokenJSON string `json:"token_json"`
	}
	_ = jsonDecodeBody(r, &body)
	cfg := s.rt.GetConfig().U115
	clientID := strings.TrimSpace(body.ClientID)
	if clientID == "" {
		clientID = cfg.ClientID
	}
	tokenJSON := strings.TrimSpace(body.TokenJSON)
	if tokenJSON == "" {
		tokenJSON = cfg.TokenJSON
	}
	sessionJSON := strings.TrimSpace(cfg.SessionJSON)
	if clientID == "" {
		writeJSONError(w, http.StatusBadRequest, "u115.client_id 不能为空")
		return
	}
	// 默认参数时复用运行时客户端（保持 token/限流状态与测试注入）；
	// 显式指定 client_id/token 路径时构造独立客户端
	var client *u115.Client
	if clientID == cfg.ClientID && tokenJSON == cfg.TokenJSON {
		client = s.rt.GetU115Client()
	}
	if client == nil {
		client = u115.NewClient(clientID, u115.WithTokenPath(s.rt.resolveConfigPath(tokenJSON)))
	}
	session, err := client.CreateDeviceCode("")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("115 创建扫码会话失败：%v", err))
		return
	}
	if err := s.saveU115DeviceSession(session, s.rt.resolveConfigPath(sessionJSON)); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("会话保存失败：%v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "qrcode": session.Qrcode, "uid": session.UID,
		"session_path": sessionJSON, "status": s.rt.u115OauthStatusPayload(),
	})
}

func (s *Server) handleU115OauthQrcode(w http.ResponseWriter, _ *http.Request) {
	cfg := s.rt.GetConfig().U115
	session, err := s.loadU115DeviceSession(s.rt.resolveConfigPath(cfg.SessionJSON))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("115 二维码生成失败：%v", err))
		return
	}
	png, err := qrcode.Encode(session.Qrcode, qrcode.Medium, 256)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("115 二维码生成失败：%v", err))
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(png)
}

func (s *Server) handleU115OauthPoll(w http.ResponseWriter, _ *http.Request) {
	cfg := s.rt.GetConfig().U115
	client := s.rt.GetU115Client()
	if client == nil {
		writeJSONError(w, http.StatusBadRequest, "115 客户端不可用")
		return
	}
	sessionPath := s.rt.resolveConfigPath(cfg.SessionJSON)
	session, err := s.loadU115DeviceSession(sessionPath)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	status, err := client.GetQrcodeStatus(*session)
	if err != nil {
		s.rt.u115AuthError(w, err)
		return
	}
	if status.Confirmed() {
		if _, err := client.ExchangeDeviceToken(*session); err != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"ok": false, "status": status.Status, "confirmed": true,
				"message": fmt.Sprintf("换取 token 失败：%v", err), "raw": status.Raw,
			})
			return
		}
		s.rt.invalidateU115RuntimeCache()
		// 会话已消费，删除会话文件
		if onDisk, err := s.loadU115DeviceSession(sessionPath); err == nil && onDisk.UID == session.UID {
			_ = os.Remove(sessionPath)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "status": status.Status, "confirmed": true, "authorized": true,
			"message": "已确认并完成授权", "raw": status.Raw,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "status": status.Status, "message": status.Message,
		"confirmed": status.Confirmed(), "raw": status.Raw,
	})
}

// invalidateU115RuntimeCache 失效 115 客户端与 Provider 缓存。
func (rt *Runtime) invalidateU115RuntimeCache() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.u115Client = nil
	rt.provider = nil
}

func (s *Server) handleU115OauthExchange(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ClientID  string `json:"client_id"`
		TokenJSON string `json:"token_json"`
	}
	_ = jsonDecodeBody(r, &body)
	cfg := s.rt.GetConfig().U115
	clientID := strings.TrimSpace(body.ClientID)
	if clientID == "" {
		clientID = cfg.ClientID
	}
	tokenJSON := strings.TrimSpace(body.TokenJSON)
	if tokenJSON == "" {
		tokenJSON = cfg.TokenJSON
	}
	if clientID == "" {
		writeJSONError(w, http.StatusBadRequest, "u115.client_id 不能为空")
		return
	}
	client := u115.NewClient(clientID, u115.WithTokenPath(s.rt.resolveConfigPath(tokenJSON)))
	sessionPath := s.rt.resolveConfigPath(strings.TrimSpace(cfg.SessionJSON))
	session, err := s.loadU115DeviceSession(sessionPath)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	token, err := client.ExchangeDeviceToken(*session)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("115 换取 token 失败：%v", err))
		return
	}
	s.rt.invalidateU115RuntimeCache()
	if onDisk, err := s.loadU115DeviceSession(sessionPath); err == nil && onDisk.UID == session.UID {
		_ = os.Remove(sessionPath)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "expires_in": token.ExpiresIn, "refresh_time": token.RefreshTime,
		"token_path": tokenJSON, "status": s.rt.u115OauthStatusPayload(),
	})
}

// U115SpacePayload GET /api/u115/space 响应（测试连接响应内嵌同形状 + ok）。
type U115SpacePayload struct {
	TotalSpace  int64 `json:"total_space"`
	RemainSpace int64 `json:"remain_space"`
}

// u115SpacePayload 获取 115 容量信息；不可用时写出错误响应并返回 false。
// 测试连接与仪表盘的 /u115/space 共用。
func (rt *Runtime) u115SpacePayload(w http.ResponseWriter) (U115SpacePayload, bool) {
	client := rt.GetU115Client()
	if client == nil {
		writeJSONError(w, http.StatusBadRequest, "115 客户端不可用（请先完成授权）")
		return U115SpacePayload{}, false
	}
	space, err := client.GetSpaceInfo()
	if err != nil {
		rt.u115AuthError(w, err)
		return U115SpacePayload{}, false
	}
	return U115SpacePayload{TotalSpace: space.TotalSize, RemainSpace: space.RemainSize}, true
}

// u115TestConnectionResponse 测试连接响应：容量信息 + ok。
type u115TestConnectionResponse struct {
	U115SpacePayload
	OK bool `json:"ok"`
}

func (s *Server) handleU115TestConnection(w http.ResponseWriter, _ *http.Request) {
	payload, ok := s.rt.u115SpacePayload(w)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, u115TestConnectionResponse{U115SpacePayload: payload, OK: true})
}

func (s *Server) handleU115TestCookie(w http.ResponseWriter, _ *http.Request) {
	cfg := s.rt.GetConfig().U115
	if strings.TrimSpace(cfg.Cookie) == "" {
		writeJSONError(w, http.StatusBadRequest, "u115.cookie 未配置")
		return
	}
	client := s.rt.GetU115Client()
	if client == nil {
		writeJSONError(w, http.StatusBadRequest, "115 客户端不可用")
		return
	}
	userInfo, err := client.GetUserInfo()
	if err != nil {
		s.rt.u115AuthError(w, err)
		return
	}
	if len(userInfo) == 0 {
		writeJSONError(w, http.StatusBadRequest, "Cookie 不可用")
		return
	}
	payload := map[string]any{"ok": true}
	for k, v := range userInfo {
		payload[k] = v
	}
	writeJSON(w, http.StatusOK, payload)
}

// ── 云下载 ──────────────────────────────────────────────────

func (rt *Runtime) offlineClient() *u115.OfflineClient {
	client := rt.GetU115Client()
	if client == nil {
		return nil
	}
	return u115.NewOfflineClient(client)
}

// U115OfflineTaskPayload 云下载任务（overview.tasks 元素）。
type U115OfflineTaskPayload struct {
	InfoHash      string  `json:"info_hash"`
	Name          string  `json:"name"`
	Status        int     `json:"status"`
	PercentDone   float64 `json:"percent_done"`
	Size          int64   `json:"size"`
	AddTime       *int64  `json:"add_time"`
	LastUpdate    *int64  `json:"last_update"`
	FileID        *string `json:"file_id"`
	DeleteFileID  *string `json:"delete_file_id"`
	URL           *string `json:"url"`
	WpPathID      *string `json:"wp_path_id"`
	IsFinished    bool    `json:"is_finished"`
	IsDownloading bool    `json:"is_downloading"`
	IsFailed      bool    `json:"is_failed"`
}

func (s *Server) serializeOfflineTask(task u115.OfflineTask) U115OfflineTaskPayload {
	return U115OfflineTaskPayload{
		InfoHash: task.InfoHash, Name: task.Name, Status: task.Status,
		PercentDone: task.PercentDone, Size: task.Size,
		AddTime: task.AddTime, LastUpdate: task.LastUpdate,
		FileID: task.FileID, DeleteFileID: task.DeleteFileID,
		URL: task.URL, WpPathID: task.WpPathID,
		IsFinished: task.IsFinished(), IsDownloading: task.IsDownloading(),
		IsFailed: task.IsFailed(),
	}
}

// U115OfflinePagination 云下载任务分页。
type U115OfflinePagination struct {
	Page       int  `json:"page"`
	PageSize   int  `json:"page_size"`
	Total      int  `json:"total"`
	TotalPages int  `json:"total_pages"`
	HasPrev    bool `json:"has_prev"`
	HasNext    bool `json:"has_next"`
}

// U115OfflineOverviewPayload GET /api/u115/offline/overview 响应。
type U115OfflineOverviewPayload struct {
	OK         bool                     `json:"ok"`
	Tasks      []U115OfflineTaskPayload `json:"tasks"`
	Pagination U115OfflinePagination    `json:"pagination"`
}

func (s *Server) handleU115OfflineOverview(w http.ResponseWriter, r *http.Request) {
	offline := s.rt.offlineClient()
	if offline == nil {
		writeJSONError(w, http.StatusBadRequest, "115 客户端不可用")
		return
	}
	page := intQuery(r, "page", 1)
	if page < 1 {
		page = 1
	}
	taskPage, err := offline.GetTaskList(page)
	if err != nil {
		s.rt.u115AuthError(w, err)
		return
	}
	tasks := make([]U115OfflineTaskPayload, 0, len(taskPage.Tasks))
	for _, t := range taskPage.Tasks {
		tasks = append(tasks, s.serializeOfflineTask(t))
	}
	totalPages := taskPage.PageCount
	if totalPages < 1 {
		totalPages = 1
	}
	writeJSON(w, http.StatusOK, U115OfflineOverviewPayload{
		OK: true, Tasks: tasks,
		Pagination: U115OfflinePagination{
			Page: taskPage.Page, PageSize: len(taskPage.Tasks),
			Total: taskPage.Count, TotalPages: totalPages,
			HasPrev: taskPage.Page > 1, HasNext: taskPage.Page < totalPages,
		},
	})
}

func (s *Server) handleU115Space(w http.ResponseWriter, _ *http.Request) {
	payload, ok := s.rt.u115SpacePayload(w)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// U115QuotaExpirePayload 配额过期明细。
type U115QuotaExpirePayload struct {
	Surplus    int64 `json:"surplus"`
	ExpireTime int64 `json:"expire_time"`
}

// U115QuotaPackagePayload 单个配额包。
type U115QuotaPackagePayload struct {
	Name       string                   `json:"name"`
	Count      int64                    `json:"count"`
	Used       int64                    `json:"used"`
	Surplus    int64                    `json:"surplus"`
	ExpireInfo []U115QuotaExpirePayload `json:"expire_info"`
}

// U115QuotaInfoPayload 离线下载配额。
type U115QuotaInfoPayload struct {
	Count    int64                     `json:"count"`
	Used     int64                     `json:"used"`
	Surplus  int64                     `json:"surplus"`
	Packages []U115QuotaPackagePayload `json:"packages"`
}

// U115QuotaResponse GET /api/u115/offline/quota 响应。
type U115QuotaResponse struct {
	OK    bool                 `json:"ok"`
	Quota U115QuotaInfoPayload `json:"quota"`
}

func (s *Server) handleU115OfflineQuota(w http.ResponseWriter, _ *http.Request) {
	offline := s.rt.offlineClient()
	if offline == nil {
		writeJSONError(w, http.StatusBadRequest, "115 客户端不可用")
		return
	}
	quota, err := offline.GetQuotaInfo()
	if err != nil {
		s.rt.u115AuthError(w, err)
		return
	}
	packages := make([]U115QuotaPackagePayload, 0, len(quota.Packages))
	for _, item := range quota.Packages {
		expireInfo := make([]U115QuotaExpirePayload, 0, len(item.ExpireInfo))
		for _, exp := range item.ExpireInfo {
			expireInfo = append(expireInfo, U115QuotaExpirePayload{
				Surplus: exp.Surplus, ExpireTime: exp.ExpireTime,
			})
		}
		packages = append(packages, U115QuotaPackagePayload{
			Name: item.Name, Count: item.Count, Used: item.Used,
			Surplus: item.Surplus, ExpireInfo: expireInfo,
		})
	}
	writeJSON(w, http.StatusOK, U115QuotaResponse{
		OK: true,
		Quota: U115QuotaInfoPayload{
			Count: quota.Count, Used: quota.Used, Surplus: quota.Surplus,
			Packages: packages,
		},
	})
}

func (s *Server) handleU115OfflineAddURLs(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URLs     string `json:"urls"`
		WpPathID string `json:"wp_path_id"`
	}
	if err := jsonDecodeBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	var urlLines []string
	for _, line := range strings.Split(body.URLs, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			urlLines = append(urlLines, line)
		}
	}
	if len(urlLines) == 0 {
		writeJSONError(w, http.StatusBadRequest, "请至少输入一个下载链接")
		return
	}
	offline := s.rt.offlineClient()
	if offline == nil {
		writeJSONError(w, http.StatusBadRequest, "115 客户端不可用")
		return
	}
	cfg := s.rt.GetConfig().U115
	targetPathID := strings.TrimSpace(body.WpPathID)
	if targetPathID == "" {
		// 配置值为目录名，按名解析为目录 ID（兼容旧 ID 值）
		targetPathID = s.rt.resolveFolderID(cfg.DownloadFolderID)
	}
	// .torrent 链接需走 BT 任务流程（链接任务只会保存种子文件本身）
	var urlTasks []string
	for _, line := range urlLines {
		if s.rt.isTorrentLink(line) {
			continue
		}
		urlTasks = append(urlTasks, line)
	}
	serialized := make([]map[string]any, 0, len(urlLines))
	if len(urlTasks) > 0 {
		results, err := offline.AddTaskURLs(urlTasks, targetPathID)
		if err != nil {
			s.rt.u115AuthError(w, err)
			return
		}
		for _, item := range results {
			serialized = append(serialized, map[string]any{
				"state": item.State, "code": item.Code, "message": item.Message,
				"url": item.URL, "info_hash": item.InfoHash,
			})
		}
	}
	for _, line := range urlLines {
		if !s.rt.isTorrentLink(line) {
			continue
		}
		if err := s.rt.addU115TorrentTask(line, targetPathID); err != nil {
			serialized = append(serialized, map[string]any{
				"state": false, "code": -1, "message": err.Error(), "url": line,
			})
			continue
		}
		serialized = append(serialized, map[string]any{
			"state": true, "code": 0, "message": "已创建 BT 任务", "url": line,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "count": len(serialized), "results": serialized, "wp_path_id": targetPathID,
	})
}

func (s *Server) handleU115OfflineDeleteTasks(w http.ResponseWriter, r *http.Request) {
	var body struct {
		InfoHashes    []string `json:"info_hashes"`
		DelSourceFile int      `json:"del_source_file"`
	}
	if err := jsonDecodeBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	var infoHashes []string
	for _, item := range body.InfoHashes {
		if item = strings.TrimSpace(item); item != "" {
			infoHashes = append(infoHashes, item)
		}
	}
	if len(infoHashes) == 0 {
		writeJSONError(w, http.StatusBadRequest, "请至少选择一个云下载任务")
		return
	}
	offline := s.rt.offlineClient()
	if offline == nil {
		writeJSONError(w, http.StatusBadRequest, "115 客户端不可用")
		return
	}
	for _, infoHash := range infoHashes {
		if err := offline.DelTask(infoHash, body.DelSourceFile); err != nil {
			s.rt.u115AuthError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": len(infoHashes)})
}

func (s *Server) handleU115OfflineClearTasks(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Flag int `json:"flag"`
	}
	if err := jsonDecodeBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	offline := s.rt.offlineClient()
	if offline == nil {
		writeJSONError(w, http.StatusBadRequest, "115 客户端不可用")
		return
	}
	if err := offline.ClearTasks(body.Flag); err != nil {
		s.rt.u115AuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "flag": body.Flag})
}

// handleU115AutoOrganizeStatus 115 自动整理监听状态。
func (s *Server) handleU115AutoOrganizeStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.rt.U115AutoOrganizeStatusPayload())
}

// ── 种子链接转 BT 云下载任务 ────────────────────────────────

// u115TorrentUploadPath 115 官方约定：提交 BT 任务前种子文件上传到「云下载/种子文件」目录。
const u115TorrentUploadPath = "/云下载/种子文件"

// maxTorrentSize 种子文件大小上限（10MB，实际种子远小于此）。
const maxTorrentSize = 10 << 20

// torrentFetchUA 下载种子文件用的浏览器 UA（mikan 等站点校验 UA）。
const torrentFetchUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36"

// isTorrentLink 判断链接是否指向 .torrent 种子文件。
// 115 云下载的链接任务会把 .torrent 链接当作普通 HTTP 下载，
// 最终保存的是种子文件本身（如 xxx.mkv.torrent），必须转 BT 任务。
func (rt *Runtime) isTorrentLink(rawURL string) bool {
	if rawURL == "" || strings.HasPrefix(rawURL, "magnet:") {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return strings.HasSuffix(strings.ToLower(parsed.Path), ".torrent")
}

// rt.torrentURLValidator 种子链接外呼校验（包级变量，测试可替换以使用本地 httptest 服务器）。

// fetchTorrentContent 下载种子文件内容（bencode 字典以 'd' 开头）。
// URL 可能源自 RSS 条目等外部内容：仅允许 http/https 公网地址（防 SSRF 探测内网）。
func (rt *Runtime) fetchTorrentContent(torrentURL string) ([]byte, error) {
	if err := rt.torrentURLValidator(torrentURL); err != nil {
		return nil, fmt.Errorf("种子链接被拒绝：%v", err)
	}
	req, err := http.NewRequest(http.MethodGet, torrentURL, nil)
	if err != nil {
		return nil, fmt.Errorf("种子链接无效：%v", err)
	}
	req.Header.Set("User-Agent", torrentFetchUA)
	if parsed, perr := url.Parse(torrentURL); perr == nil && parsed.Host != "" {
		req.Header.Set("Referer", parsed.Scheme+"://"+parsed.Host+"/")
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载种子文件失败：%v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("下载种子文件失败：HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxTorrentSize+1))
	if err != nil {
		return nil, fmt.Errorf("下载种子文件失败：%v", err)
	}
	if len(data) > maxTorrentSize {
		return nil, fmt.Errorf("种子文件超过大小限制（%dMB）", maxTorrentSize>>20)
	}
	if len(data) == 0 || data[0] != 'd' {
		return nil, fmt.Errorf("下载内容不是有效的种子文件：%s", torrentURL)
	}
	return data, nil
}

// torrentFileName 从种子链接推断文件名。
func (rt *Runtime) torrentFileName(torrentURL string) string {
	parsed, err := url.Parse(torrentURL)
	if err == nil {
		if base := path.Base(parsed.Path); base != "" && base != "/" && base != "." {
			if decoded, derr := url.PathUnescape(base); derr == nil && decoded != "" {
				return decoded
			}
			return base
		}
	}
	return "panoply.torrent"
}

// wantedFileIndexes 依据种子解析结果生成选中文件序号；无标记时全选。
func (rt *Runtime) wantedFileIndexes(files []u115.TorrentFileItem) []int {
	var wanted []int
	for i, f := range files {
		if f.Wanted != 0 {
			wanted = append(wanted, i)
		}
	}
	if len(wanted) > 0 || len(files) == 0 {
		return wanted
	}
	all := make([]int, len(files))
	for i := range all {
		all[i] = i
	}
	return all
}

// pushU115Download 按链接类型推送 115 云下载：
// 磁力/普通链接走链接任务（保存到 wp_path_id），.torrent 链接转 BT 任务。
// 未配置下载目录时任务落到根目录（与 Python 版一致）。
func (rt *Runtime) pushU115Download(magnetURL, torrentURL string) error {
	offline := rt.offlineClient()
	if offline == nil {
		return fmt.Errorf("115 客户端不可用")
	}
	// 配置值为目录名，按名解析为目录 ID（兼容旧 ID 值）
	wpPathID := rt.resolveFolderID(rt.GetConfig().U115.DownloadFolderID)
	if magnetURL != "" {
		_, err := offline.AddTaskURLs([]string{magnetURL}, wpPathID)
		return err
	}
	if rt.isTorrentLink(torrentURL) {
		return rt.addU115TorrentTask(torrentURL, wpPathID)
	}
	_, err := offline.AddTaskURLs([]string{torrentURL}, wpPathID)
	return err
}

// addU115TorrentTask .torrent 链接 → 下载种子 → 上传至「云下载/种子文件」
// → 解析 info_hash → 提交 BT 任务（保存到 wp_path_id）。
func (rt *Runtime) addU115TorrentTask(torrentURL, wpPathID string) error {
	data, err := rt.fetchTorrentContent(torrentURL)
	if err != nil {
		return err
	}
	client := rt.GetU115Client()
	if client == nil {
		return fmt.Errorf("115 客户端不可用")
	}
	provider := rt.GetStorageProvider()
	if provider == nil {
		return fmt.Errorf("115 存储 Provider 不可用")
	}
	folder, err := client.EnsureFolderPath(u115TorrentUploadPath)
	if err != nil {
		return fmt.Errorf("创建种子上传目录失败：%v", err)
	}
	name := rt.torrentFileName(torrentURL)
	uploaded, err := provider.UploadBytes(data, name, folder.ID, "application/x-bittorrent", true)
	if err != nil {
		return fmt.Errorf("上传种子文件失败：%v", err)
	}
	pickCode, _ := uploaded.Extra["pick_code"].(string)
	if pickCode == "" {
		// 秒传路径不返回 pick_code，回查一次文件
		if found, ferr := provider.FindFile(name, folder.ID); ferr == nil && found != nil {
			pickCode, _ = found.Extra["pick_code"].(string)
		}
	}
	if pickCode == "" {
		return fmt.Errorf("上传种子缺少 pick_code，无法创建 BT 任务")
	}
	torrentSHA1 := fmt.Sprintf("%x", sha1.Sum(data))
	offline := u115.NewOfflineClient(client)
	infos, err := offline.ParseTorrent(torrentSHA1, pickCode)
	if err != nil {
		return fmt.Errorf("解析种子失败：%v", err)
	}
	if len(infos) == 0 {
		return fmt.Errorf("解析种子失败：未返回任务信息")
	}
	return offline.AddTaskBT(u115.AddTaskBTArgs{
		InfoHash:    infos[0].InfoHash,
		Wanted:      rt.wantedFileIndexes(infos[0].Files),
		SavePath:    "",
		TorrentSHA1: torrentSHA1,
		PickCode:    pickCode,
		WpPathID:    wpPathID,
	})
}
