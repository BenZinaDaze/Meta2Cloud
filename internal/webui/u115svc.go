// u115svc.go —— 115 授权与云下载服务（对应 webui/services/u115.py + routes/u115.py）。
package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"meta2cloud/internal/u115"
)

// resolveConfigPath 相对路径基于项目根目录解析。
func resolveConfigPath(pathStr string) string {
	if pathStr == "" {
		return ""
	}
	if filepath.IsAbs(pathStr) {
		return pathStr
	}
	return filepath.Join(RT().RootDir, pathStr)
}

// ── 扫码会话持久化 ──────────────────────────────────────────

type deviceSessionJSON struct {
	Qrcode       string `json:"qrcode"`
	UID          string `json:"uid"`
	TimeValue    string `json:"time_value"`
	Sign         string `json:"sign"`
	CodeVerifier string `json:"code_verifier"`
}

func saveU115DeviceSession(session u115.DeviceCodeSession, sessionPath string) error {
	if dir := filepath.Dir(sessionPath); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	data, err := json.MarshalIndent(deviceSessionJSON{
		Qrcode: session.Qrcode, UID: session.UID, TimeValue: session.TimeValue,
		Sign: session.Sign, CodeVerifier: session.CodeVerifier,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sessionPath, data, 0o600)
}

// loadU115DeviceSession 读取扫码会话；不存在返回错误。
func loadU115DeviceSession(sessionPath string) (*u115.DeviceCodeSession, error) {
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		return nil, fmt.Errorf("115 扫码会话文件不存在：%s", sessionPath)
	}
	var s deviceSessionJSON
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &u115.DeviceCodeSession{
		Qrcode: s.Qrcode, UID: s.UID, TimeValue: s.TimeValue,
		Sign: s.Sign, CodeVerifier: s.CodeVerifier,
	}, nil
}

// ── OAuth 状态 ──────────────────────────────────────────────

func u115OauthStatusPayload() map[string]any {
	cfg := RT().GetConfig().U115
	tokenPath := resolveConfigPath(cfg.TokenJSON)
	sessionPath := resolveConfigPath(cfg.SessionJSON)
	_, tokenStatErr := os.Stat(tokenPath)
	tokenExists := tokenStatErr == nil
	_, sessionStatErr := os.Stat(sessionPath)
	sessionExists := sessionStatErr == nil

	tokenValid, tokenExpired, authorized, refreshed := false, false, false, false
	var expiresAt, refreshTime any
	if tokenExists {
		if token, ok, _ := u115.LoadToken(tokenPath); ok {
			refreshTime = token.RefreshTime
			expiresAt = token.ExpiresAt()
			tokenExpired = time.Now().Unix() >= token.ExpiresAt()
			if tokenExpired {
				// 尝试自动刷新
				client := RT().GetU115Client()
				if client != nil {
					if refreshedToken, err := client.RefreshToken(); err == nil {
						refreshed = true
						refreshTime = refreshedToken.RefreshTime
						expiresAt = refreshedToken.ExpiresAt()
						tokenExpired = false
						tokenValid = true
						authorized = true
					}
				}
			} else {
				tokenValid = true
				authorized = true
			}
		}
	}
	return map[string]any{
		"client_id":  cfg.ClientID,
		"token_path": cfg.TokenJSON, "session_path": cfg.SessionJSON,
		"cookie_configured": strings.TrimSpace(cfg.Cookie) != "",
		"token_exists":      tokenExists, "session_exists": sessionExists,
		"authorized": authorized, "token_valid": tokenValid,
		"token_expired": tokenExpired, "refresh_time": refreshTime,
		"expires_at": expiresAt, "refreshed": refreshed,
	}
}

// ── HTTP 处理器 ─────────────────────────────────────────────

func u115AuthError(w http.ResponseWriter, err error) {
	if u115.IsAuth(err) {
		writeJSONError(w, http.StatusBadRequest, "115 Token 已失效，请重新扫码授权")
		return
	}
	writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("115 接口调用失败：%v", err))
}

func handleU115OauthStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, u115OauthStatusPayload())
}

func handleU115OauthCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ClientID  string `json:"client_id"`
		TokenJSON string `json:"token_json"`
	}
	_ = jsonDecodeBody(r, &body)
	cfg := RT().GetConfig().U115
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
		client = RT().GetU115Client()
	}
	if client == nil {
		client = u115.NewClient(clientID, u115.WithTokenPath(resolveConfigPath(tokenJSON)))
	}
	session, err := client.CreateDeviceCode("")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("115 创建扫码会话失败：%v", err))
		return
	}
	if err := saveU115DeviceSession(session, resolveConfigPath(sessionJSON)); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("会话保存失败：%v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "qrcode": session.Qrcode, "uid": session.UID,
		"session_path": sessionJSON, "status": u115OauthStatusPayload(),
	})
}

func handleU115OauthQrcode(w http.ResponseWriter, _ *http.Request) {
	cfg := RT().GetConfig().U115
	session, err := loadU115DeviceSession(resolveConfigPath(cfg.SessionJSON))
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

func handleU115OauthPoll(w http.ResponseWriter, _ *http.Request) {
	cfg := RT().GetConfig().U115
	client := RT().GetU115Client()
	if client == nil {
		writeJSONError(w, http.StatusBadRequest, "115 客户端不可用")
		return
	}
	sessionPath := resolveConfigPath(cfg.SessionJSON)
	session, err := loadU115DeviceSession(sessionPath)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	status, err := client.GetQrcodeStatus(*session)
	if err != nil {
		u115AuthError(w, err)
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
		invalidateU115RuntimeCache()
		// 会话已消费，删除会话文件
		if onDisk, err := loadU115DeviceSession(sessionPath); err == nil && onDisk.UID == session.UID {
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
func invalidateU115RuntimeCache() {
	rt := RT()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.u115Client = nil
	rt.provider = nil
}

func handleU115OauthExchange(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ClientID  string `json:"client_id"`
		TokenJSON string `json:"token_json"`
	}
	_ = jsonDecodeBody(r, &body)
	cfg := RT().GetConfig().U115
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
	client := u115.NewClient(clientID, u115.WithTokenPath(resolveConfigPath(tokenJSON)))
	sessionPath := resolveConfigPath(strings.TrimSpace(cfg.SessionJSON))
	session, err := loadU115DeviceSession(sessionPath)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	token, err := client.ExchangeDeviceToken(*session)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("115 换取 token 失败：%v", err))
		return
	}
	invalidateU115RuntimeCache()
	if onDisk, err := loadU115DeviceSession(sessionPath); err == nil && onDisk.UID == session.UID {
		_ = os.Remove(sessionPath)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "expires_in": token.ExpiresIn, "refresh_time": token.RefreshTime,
		"token_path": tokenJSON, "status": u115OauthStatusPayload(),
	})
}

func handleU115TestConnection(w http.ResponseWriter, _ *http.Request) {
	client := RT().GetU115Client()
	if client == nil {
		writeJSONError(w, http.StatusBadRequest, "115 客户端不可用（请先完成授权）")
		return
	}
	space, err := client.GetSpaceInfo()
	if err != nil {
		u115AuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "total_space": space.TotalSize, "remain_space": space.RemainSize,
	})
}

func handleU115TestCookie(w http.ResponseWriter, _ *http.Request) {
	cfg := RT().GetConfig().U115
	if strings.TrimSpace(cfg.Cookie) == "" {
		writeJSONError(w, http.StatusBadRequest, "u115.cookie 未配置")
		return
	}
	client := RT().GetU115Client()
	if client == nil {
		writeJSONError(w, http.StatusBadRequest, "115 客户端不可用")
		return
	}
	userInfo, err := client.GetUserInfo()
	if err != nil {
		u115AuthError(w, err)
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

func offlineClient() *u115.OfflineClient {
	client := RT().GetU115Client()
	if client == nil {
		return nil
	}
	return u115.NewOfflineClient(client)
}

func serializeOfflineTask(task u115.OfflineTask) map[string]any {
	return map[string]any{
		"info_hash": task.InfoHash, "name": task.Name, "status": task.Status,
		"percent_done": task.PercentDone, "size": task.Size,
		"add_time": task.AddTime, "last_update": task.LastUpdate,
		"file_id": task.FileID, "delete_file_id": task.DeleteFileID,
		"url": task.URL, "wp_path_id": task.WpPathID,
		"is_finished": task.IsFinished(), "is_downloading": task.IsDownloading(),
		"is_failed": task.IsFailed(),
	}
}

func handleU115OfflineOverview(w http.ResponseWriter, r *http.Request) {
	offline := offlineClient()
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
		u115AuthError(w, err)
		return
	}
	tasks := make([]map[string]any, 0, len(taskPage.Tasks))
	for _, t := range taskPage.Tasks {
		tasks = append(tasks, serializeOfflineTask(t))
	}
	totalPages := taskPage.PageCount
	if totalPages < 1 {
		totalPages = 1
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "tasks": tasks,
		"pagination": map[string]any{
			"page": taskPage.Page, "page_size": len(taskPage.Tasks),
			"total": taskPage.Count, "total_pages": totalPages,
			"has_prev": taskPage.Page > 1, "has_next": taskPage.Page < totalPages,
		},
	})
}

func handleU115OfflineQuota(w http.ResponseWriter, _ *http.Request) {
	offline := offlineClient()
	if offline == nil {
		writeJSONError(w, http.StatusBadRequest, "115 客户端不可用")
		return
	}
	quota, err := offline.GetQuotaInfo()
	if err != nil {
		u115AuthError(w, err)
		return
	}
	packages := make([]map[string]any, 0, len(quota.Packages))
	for _, item := range quota.Packages {
		expireInfo := make([]map[string]any, 0, len(item.ExpireInfo))
		for _, exp := range item.ExpireInfo {
			expireInfo = append(expireInfo, map[string]any{
				"surplus": exp.Surplus, "expire_time": exp.ExpireTime,
			})
		}
		packages = append(packages, map[string]any{
			"name": item.Name, "count": item.Count, "used": item.Used,
			"surplus": item.Surplus, "expire_info": expireInfo,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"quota": map[string]any{
			"count": quota.Count, "used": quota.Used, "surplus": quota.Surplus,
			"packages": packages,
		},
	})
}

func handleU115OfflineAddURLs(w http.ResponseWriter, r *http.Request) {
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
	offline := offlineClient()
	if offline == nil {
		writeJSONError(w, http.StatusBadRequest, "115 客户端不可用")
		return
	}
	cfg := RT().GetConfig().U115
	targetPathID := strings.TrimSpace(body.WpPathID)
	if targetPathID == "" {
		targetPathID = strings.TrimSpace(cfg.DownloadFolderID)
	}
	results, err := offline.AddTaskURLs(urlLines, targetPathID)
	if err != nil {
		u115AuthError(w, err)
		return
	}
	serialized := make([]map[string]any, 0, len(results))
	for _, item := range results {
		serialized = append(serialized, map[string]any{
			"state": item.State, "code": item.Code, "message": item.Message,
			"url": item.URL, "info_hash": item.InfoHash,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "count": len(results), "results": serialized, "wp_path_id": targetPathID,
	})
}

func handleU115OfflineDeleteTasks(w http.ResponseWriter, r *http.Request) {
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
	offline := offlineClient()
	if offline == nil {
		writeJSONError(w, http.StatusBadRequest, "115 客户端不可用")
		return
	}
	for _, infoHash := range infoHashes {
		if err := offline.DelTask(infoHash, body.DelSourceFile); err != nil {
			u115AuthError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": len(infoHashes)})
}

func handleU115OfflineClearTasks(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Flag int `json:"flag"`
	}
	if err := jsonDecodeBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	offline := offlineClient()
	if offline == nil {
		writeJSONError(w, http.StatusBadRequest, "115 客户端不可用")
		return
	}
	if err := offline.ClearTasks(body.Flag); err != nil {
		u115AuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "flag": body.Flag})
}

// handleU115AutoOrganizeStatus 115 自动整理监听状态。
func handleU115AutoOrganizeStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, U115AutoOrganizeStatusPayload())
}
