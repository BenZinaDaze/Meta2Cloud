// traktsvc.go —— Plex ↔ Trakt 观看同步服务：
// Trakt 设备码配对、Plex PIN 配对、双向同步触发与历史、会话轮询（正在看 → Trakt）、
// TitleIndex（media_title_pins/library_media 只读反查）。
package webui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"meta2cloud/internal/plexserver"
	"meta2cloud/internal/syncengine"
	"meta2cloud/internal/trakt"
	"meta2cloud/internal/watched"
	"meta2cloud/internal/webui/store"
)

// ── Plex 配对态持久化（config/plex-auth.json，0o600）────────

type plexAuthJSON struct {
	AuthToken        string `json:"auth_token"`
	ClientIdentifier string `json:"client_identifier"`
	CreatedAt        int64  `json:"created_at"`
}

func plexAuthPath() string {
	return resolveConfigPath(RT().GetConfig().Plex.TokenJSON)
}

func loadPlexAuth(path string) (*plexAuthJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var auth plexAuthJSON
	if err := json.Unmarshal(data, &auth); err != nil {
		return nil, err
	}
	return &auth, nil
}

func savePlexAuth(path string, auth *plexAuthJSON) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	data, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// ensurePlexAuthLocked 确保 client_identifier 存在（需已持 rt.mu；只做文件 I/O）。
func ensurePlexAuthLocked(rt *Runtime) *plexAuthJSON {
	cfg := rt.getConfigLocked()
	tokenPath := cfg.Plex.TokenJSON
	if !filepath.IsAbs(tokenPath) {
		tokenPath = filepath.Join(rt.RootDir, tokenPath)
	}
	if auth, err := loadPlexAuth(tokenPath); err == nil && auth.ClientIdentifier != "" {
		return auth
	}
	identifier := generateClientIdentifier()
	auth := &plexAuthJSON{ClientIdentifier: identifier, CreatedAt: time.Now().Unix()}
	if old, err := loadPlexAuth(tokenPath); err == nil {
		auth.AuthToken = old.AuthToken
		auth.CreatedAt = old.CreatedAt
	}
	if err := savePlexAuth(tokenPath, auth); err != nil {
		return nil
	}
	return auth
}

func generateClientIdentifier() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(buf[0:4]), hex.EncodeToString(buf[4:6]),
		hex.EncodeToString(buf[6:8]), hex.EncodeToString(buf[8:10]),
		hex.EncodeToString(buf[10:16]))
}

// ── 设备码 / PIN 会话文件 ───────────────────────────────────

type traktDeviceSessionJSON struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresAt       int64  `json:"expires_at"`
}

func traktSessionPath() string {
	return resolveConfigPath("config/trakt-device-session.json")
}

func saveTraktDeviceSession(sess *traktDeviceSessionJSON) error {
	path := traktSessionPath()
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func loadTraktDeviceSession() (*traktDeviceSessionJSON, error) {
	data, err := os.ReadFile(traktSessionPath())
	if err != nil {
		return nil, fmt.Errorf("Trakt 配对会话不存在，请先发起配对")
	}
	var sess traktDeviceSessionJSON
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

type plexPinSessionJSON struct {
	PinID            int    `json:"pin_id"`
	Code             string `json:"code"`
	ClientIdentifier string `json:"client_identifier"`
}

func plexPinSessionPath() string {
	return resolveConfigPath("config/plex-pin-session.json")
}

func savePlexPinSession(sess *plexPinSessionJSON) error {
	path := plexPinSessionPath()
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func loadPlexPinSession() (*plexPinSessionJSON, error) {
	data, err := os.ReadFile(plexPinSessionPath())
	if err != nil {
		return nil, fmt.Errorf("Plex 配对会话不存在，请先发起配对")
	}
	var sess plexPinSessionJSON
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

// ── 运行时缓存失效 ─────────────────────────────────────────

func invalidateTraktRuntimeCache() {
	rt := RT()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.traktClient = nil
}

func invalidatePlexRuntimeCache() {
	rt := RT()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.plexClient = nil
}

// ── 状态载荷 ───────────────────────────────────────────────

func traktOauthStatusPayload() map[string]any {
	cfg := RT().GetConfig().Trakt
	ready := RT().GetConfig().IsTraktReady()
	tokenPath := resolveConfigPath(cfg.TokenJSON)
	token, tokenExists := func() (trakt.Token, bool) {
		t, ok, err := trakt.LoadToken(tokenPath)
		if err != nil {
			return trakt.Token{}, false
		}
		return t, ok
	}()

	authorized, tokenValid, tokenExpired, refreshed := false, false, false, false
	var expiresAt, refreshTime any
	if tokenExists {
		refreshTime = token.RefreshTime
		expiresAt = token.ExpiresAt()
		tokenExpired = time.Now().Unix() >= token.ExpiresAt()
		if tokenExpired {
			// 尝试自动刷新（与 115 状态载荷同思路）
			if client := RT().GetTraktClient(); client != nil {
				if err := client.RefreshToken(); err == nil {
					refreshed = true
					if t, ok, _ := trakt.LoadToken(tokenPath); ok {
						refreshTime = t.RefreshTime
						expiresAt = t.ExpiresAt()
					}
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
	// 配对会话存在时前端可继续轮询
	session, _ := loadTraktDeviceSession()
	var pairing any
	if session != nil && time.Now().Unix() < session.ExpiresAt {
		pairing = map[string]any{
			"user_code": session.UserCode, "verification_url": session.VerificationURL,
			"expires_at": session.ExpiresAt,
		}
	}
	return map[string]any{
		"ready":     ready,
		"client_id": cfg.ClientID, "token_path": cfg.TokenJSON,
		"token_exists": tokenExists, "authorized": authorized,
		"token_valid": tokenValid, "token_expired": tokenExpired,
		"refresh_time": refreshTime, "expires_at": expiresAt, "refreshed": refreshed,
		"pairing": pairing,
	}
}

func plexStatusPayload() map[string]any {
	cfg := RT().GetConfig()
	authPath := plexAuthPath()
	auth, authErr := loadPlexAuth(authPath)
	hasToken := authErr == nil && auth.AuthToken != ""
	identifier := ""
	if authErr == nil {
		identifier = auth.ClientIdentifier
	}
	payload := map[string]any{
		"ready":             cfg.IsPlexReady(),
		"base_url":          cfg.Plex.BaseURL,
		"auth_path":         cfg.Plex.TokenJSON,
		"token_configured":  hasToken,
		"client_identifier": identifier,
		"reachable":         false,
	}
	if pinSession, err := loadPlexPinSession(); err == nil {
		payload["pairing"] = map[string]any{"code": pinSession.Code}
	}
	if !cfg.IsPlexReady() || !hasToken {
		return payload
	}
	client := RT().GetPlexClient()
	if client == nil {
		return payload
	}
	if identity, err := client.Identity(); err == nil {
		payload["reachable"] = true
		payload["server_name"] = identity.MediaContainer.MachineIdentifier
		payload["server_version"] = identity.MediaContainer.Version
	} else {
		payload["error"] = err.Error()
	}
	return payload
}

// ── HTTP 处理器：Trakt 设备码 ───────────────────────────────

func handleTraktOauthStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, traktOauthStatusPayload())
}

func handleTraktOauthDevice(w http.ResponseWriter, _ *http.Request) {
	if !RT().GetConfig().IsTraktReady() {
		writeJSONError(w, http.StatusBadRequest, "请先在配置中填写 trakt.client_id（trakt.tv/oauth/applications 创建应用；Development 应用无需 Client Secret）")
		return
	}
	client := RT().GetTraktClient()
	if client == nil {
		writeJSONError(w, http.StatusBadRequest, "Trakt 客户端不可用")
		return
	}
	code, err := client.CreateDeviceCode()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Trakt 创建设备码失败：%v", err))
		return
	}
	expiresAt := time.Now().Unix() + code.ExpiresIn
	if err := saveTraktDeviceSession(&traktDeviceSessionJSON{
		DeviceCode: code.DeviceCode, UserCode: code.UserCode,
		VerificationURL: code.VerificationURL, ExpiresAt: expiresAt,
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("配对会话保存失败：%v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "user_code": code.UserCode,
		"verification_url": code.VerificationURL,
		"expires_in":       code.ExpiresIn, "interval": code.Interval,
		"status": traktOauthStatusPayload(),
	})
}

func handleTraktOauthPoll(w http.ResponseWriter, _ *http.Request) {
	client := RT().GetTraktClient()
	if client == nil {
		writeJSONError(w, http.StatusBadRequest, "Trakt 客户端不可用（请先填写应用凭证并保存）")
		return
	}
	session, err := loadTraktDeviceSession()
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	if time.Now().Unix() >= session.ExpiresAt {
		_ = os.Remove(traktSessionPath())
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": false, "confirmed": false, "expired": true,
			"message": "设备码已过期，请重新发起配对",
		})
		return
	}
	result, err := client.PollDeviceToken(session.DeviceCode)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Trakt 轮询失败：%v", err))
		return
	}
	if result.Confirmed() {
		invalidateTraktRuntimeCache()
		// 会话已消费，删除会话文件
		if onDisk, err := loadTraktDeviceSession(); err == nil && onDisk.DeviceCode == session.DeviceCode {
			_ = os.Remove(traktSessionPath())
		}
		user, _ := client.Me()
		appLog("trakt", "oauth_paired", "SUCCESS", "Trakt 授权成功（用户 "+user+"）", nil)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "confirmed": true, "authorized": true, "user": user,
			"message": "已确认并完成授权",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "confirmed": false, "expired": result.State == trakt.PollExpired,
		"message": result.Message,
	})
}

func handleTraktOauthDisconnect(w http.ResponseWriter, _ *http.Request) {
	if client := RT().GetTraktClient(); client != nil {
		client.Disconnect()
	} else {
		// 客户端不可用（凭证缺失）时直接删 token 文件
		_ = os.Remove(resolveConfigPath(RT().GetConfig().Trakt.TokenJSON))
	}
	invalidateTraktRuntimeCache()
	appLog("trakt", "oauth_disconnected", "INFO", "已断开 Trakt 授权", nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "status": traktOauthStatusPayload(),
	})
}

// ── HTTP 处理器：Plex PIN ───────────────────────────────────

func handlePlexPin(w http.ResponseWriter, _ *http.Request) {
	cfg := RT().GetConfig()
	if !cfg.IsPlexReady() {
		writeJSONError(w, http.StatusBadRequest, "请先在配置中填写 plex.base_url")
		return
	}
	rt := RT()
	rt.mu.Lock()
	auth := ensurePlexAuthLocked(rt)
	rt.mu.Unlock()
	if auth == nil {
		writeJSONError(w, http.StatusInternalServerError, "Plex 配对态初始化失败")
		return
	}
	client := plexserver.NewClient(cfg.Plex.BaseURL, "", auth.ClientIdentifier)
	pin, authURL, err := client.CreatePIN()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("plex.tv 创建 PIN 失败：%v", err))
		return
	}
	if err := savePlexPinSession(&plexPinSessionJSON{
		PinID: pin.ID, Code: pin.Code, ClientIdentifier: auth.ClientIdentifier,
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("配对会话保存失败：%v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "pin_id": pin.ID, "code": pin.Code, "auth_url": authURL,
	})
}

func handlePlexPinPoll(w http.ResponseWriter, _ *http.Request) {
	cfg := RT().GetConfig()
	session, err := loadPlexPinSession()
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	client := plexserver.NewClient(cfg.Plex.BaseURL, "", session.ClientIdentifier)
	pin, err := client.PollPIN(session.PinID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("plex.tv 轮询失败：%v", err))
		return
	}
	if pin.AuthToken == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "confirmed": false, "message": "等待用户在 Plex 授权页确认"})
		return
	}
	// 授权完成：保存 token，验证服务器连通性
	authPath := plexAuthPath()
	auth := &plexAuthJSON{
		AuthToken: pin.AuthToken, ClientIdentifier: session.ClientIdentifier,
		CreatedAt: time.Now().Unix(),
	}
	if old, err := loadPlexAuth(authPath); err == nil && old.CreatedAt > 0 {
		auth.CreatedAt = old.CreatedAt
	}
	if err := savePlexAuth(authPath, auth); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Plex 凭证保存失败：%v", err))
		return
	}
	_ = os.Remove(plexPinSessionPath())
	invalidatePlexRuntimeCache()

	serverVersion := ""
	reachable := false
	if plexClient := RT().GetPlexClient(); plexClient != nil {
		if identity, ierr := plexClient.Identity(); ierr == nil {
			reachable = true
			serverVersion = identity.MediaContainer.Version
		}
	}
	appLog("trakt", "plex_paired", "SUCCESS",
		fmt.Sprintf("Plex 授权成功（服务器可达=%v）", reachable), nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "confirmed": true, "authorized": true,
		"reachable": reachable, "server_version": serverVersion,
		"message": "Plex 授权完成",
	})
}

func handlePlexTest(w http.ResponseWriter, _ *http.Request) {
	client := RT().GetPlexClient()
	if client == nil {
		writeJSONError(w, http.StatusBadRequest, "Plex 客户端不可用（请先填写 base_url 并完成授权）")
		return
	}
	identity, err := client.Identity()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Plex 连接失败：%v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "server_name": identity.MediaContainer.MachineIdentifier,
		"server_version": identity.MediaContainer.Version,
	})
}

// ── TitleIndex（media_title_pins / library_media 只读反查）──

var titleNormalizeRe = regexp.MustCompile(`\s+`)

// normalizeTitle 标题归一化：小写、收空白、全角括号转半角。
func normalizeTitle(s string) string {
	s = strings.ReplaceAll(s, "（", "(")
	s = strings.ReplaceAll(s, "）", ")")
	s = strings.TrimSpace(strings.ToLower(s))
	return titleNormalizeRe.ReplaceAllString(s, " ")
}

// titleIndex watched.TitleIndex 的实现（每轮同步构建一次，只读库）。
type titleIndex struct {
	byExact map[string]watched.MediaKey // mediaType|normTitle|year
	byTitle map[string][]identityEntry  // mediaType|normTitle → 候选（年份容错用）
}

type identityEntry struct {
	key  watched.MediaKey
	year int64
}

func titleIndexKey(mediaType, normTitle string, year int64) string {
	return fmt.Sprintf("%s|%s|%d", mediaType, normTitle, year)
}

// buildTitleIndex 从 LibraryStore 构建反查索引（只读，不写入 library.db）。
func buildTitleIndex() watched.TitleIndex {
	idx := &titleIndex{
		byExact: map[string]watched.MediaKey{},
		byTitle: map[string][]identityEntry{},
	}
	libStore := RT().GetLibraryStore()
	if libStore == nil {
		return idx
	}
	add := func(mediaType, title, yearStr string, tmdbID int64) {
		if mediaType == "" || title == "" || tmdbID <= 0 {
			return
		}
		keyType := mediaType
		if keyType == "show" {
			keyType = "tv"
		}
		var year int64
		if y, err := strconv.ParseInt(strings.TrimSpace(yearStr), 10, 64); err == nil {
			year = y
		}
		norm := normalizeTitle(title)
		entry := identityEntry{key: watched.MediaKey{Type: keyType, TMDBID: tmdbID}, year: year}
		if year > 0 {
			exact := titleIndexKey(keyType, norm, year)
			if _, exists := idx.byExact[exact]; !exists {
				idx.byExact[exact] = entry.key
			}
		}
		listKey := keyType + "|" + norm
		idx.byTitle[listKey] = append(idx.byTitle[listKey], entry)
	}
	if pins, err := libStore.ListTitlePins(); err == nil {
		for _, pin := range pins {
			add(pin.MediaType, pin.PinnedTitle, pin.PinnedYear, pin.TmdbID)
		}
	}
	if items, err := libStore.ListMediaIdentities(); err == nil {
		for _, m := range items {
			add(m.MediaType, m.Title, m.Year, m.TmdbID)
		}
	}
	return idx
}

// Lookup 实现 watched.TitleIndex：精确年份优先，未中时同名单内年份差 ≤1 容错。
func (idx *titleIndex) Lookup(mediaType, title string, year int64) (watched.MediaKey, bool) {
	norm := normalizeTitle(title)
	if key, ok := idx.byExact[titleIndexKey(mediaType, norm, year)]; ok {
		return key, true
	}
	candidates := idx.byTitle[mediaType+"|"+norm]
	if len(candidates) == 0 {
		return watched.MediaKey{}, false
	}
	for _, c := range candidates {
		if c.year > 0 && abs64i(c.year-year) <= 1 {
			return c.key, true
		}
	}
	// 年份全对不上但同名唯一时仍接受（钉扎年份缺失的场景）
	if len(candidates) == 1 {
		return candidates[0].key, true
	}
	return watched.MediaKey{}, false
}

func abs64i(a int64) int64 {
	if a < 0 {
		return -a
	}
	return a
}

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
func TriggerTraktSync(trigger string) bool {
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
				appLog("trakt", "sync_panic", "ERROR", fmt.Sprintf("观看同步异常退出：%v", r), nil)
			}
			traktSyncState.mu.Lock()
			traktSyncState.running = false
			traktSyncState.mu.Unlock()
		}()
		runTraktSync(trigger)
	}()
	return true
}

// traktSyncLogger 引擎日志回调 → appLog。
func traktSyncLogger(level, message string, details map[string]any) {
	appLog("trakt", "sync", level, message, details)
}

// runTraktSync 构建引擎并执行一轮同步。
func runTraktSync(trigger string) {
	cfg := RT().GetConfig()
	traktClient := RT().GetTraktClient()
	if traktClient == nil {
		appLog("trakt", "sync_skipped", "WARNING", "Trakt 客户端不可用（未配置或未授权），跳过同步", nil)
		return
	}
	plexClient := RT().GetPlexClient()
	if plexClient == nil {
		appLog("trakt", "sync_skipped", "WARNING", "Plex 客户端不可用（未配置 base_url 或未授权），跳过同步", nil)
		return
	}
	syncStore := RT().GetTraktSyncStore()
	if syncStore == nil {
		appLog("trakt", "sync_skipped", "WARNING", "观看同步存储不可用，跳过同步", nil)
		return
	}
	engine := &syncengine.Engine{
		Playback: plexserver.NewSource(plexClient, buildTitleIndex()),
		Trakt:    trakt.NewTraktSource(traktClient),
		Store:    syncStore,
		ToPlex:   cfg.Trakt.SyncToPlex,
		ToTrakt:  cfg.Trakt.SyncToTrakt,
		Log:      traktSyncLogger,
	}
	started := time.Now()
	summary, err := engine.Run(context.Background(), trigger)
	took := time.Since(started).Round(time.Millisecond)
	if err != nil {
		appLog("trakt", "sync_failed", "ERROR", fmt.Sprintf("观看同步失败（耗时 %s）：%v", took, err), nil)
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
	appLog("trakt", "sync_done", "SUCCESS",
		fmt.Sprintf("观看同步完成（耗时 %s）：→Trakt %d 条，→Plex %d 条，冲突 %d，未识别 %d，跳过 %d",
			took, summary.ToTrakt, summary.ToPlex, summary.Conflicts, summary.Unresolved, summary.Skipped),
		map[string]any{
			"to_trakt": summary.ToTrakt, "to_plex": summary.ToPlex,
			"conflicts": summary.Conflicts, "unresolved": summary.Unresolved,
			"skipped": summary.Skipped, "took_ms": took.Milliseconds(),
		})
}

// ── HTTP 处理器：同步状态 / 触发 / 历史 ─────────────────────

func traktSyncStatusPayload() map[string]any {
	cfg := RT().GetConfig()
	traktStatus := traktOauthStatusPayload()
	plex := plexStatusPayload()
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
		"sse_connected":          sseConnected(),
		"last_polled_at":         traktWatcher.lastPolledAt,
		"last_poll_error":        traktWatcher.lastPollError,
		"last_session_polled_at": traktSessionWatcher.lastPolledAt,
	}
	if store := RT().GetTraktSyncStore(); store != nil {
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

func handleTraktSyncStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, traktSyncStatusPayload())
}

func handleTraktSyncTrigger(w http.ResponseWriter, _ *http.Request) {
	status := traktOauthStatusPayload()
	if authorized, _ := status["authorized"].(bool); !authorized {
		writeJSONError(w, http.StatusBadRequest, "Trakt 未授权，请先完成设备码配对")
		return
	}
	if connected, _ := plexStatusPayload()["token_configured"].(bool); !connected {
		writeJSONError(w, http.StatusBadRequest, "Plex 未授权，请先完成 PIN 配对")
		return
	}
	if !TriggerTraktSync(triggerManual) {
		writeJSONError(w, http.StatusConflict, "观看同步已在进行中")
		return
	}
	appLog("trakt", "sync_triggered", "INFO", "手动触发观看同步", nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "观看同步已触发"})
}

func handleTraktSyncRuns(w http.ResponseWriter, r *http.Request) {
	store := RT().GetTraktSyncStore()
	if store == nil {
		writeJSONError(w, http.StatusInternalServerError, "观看同步存储不可用")
		return
	}
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

func handleTraktSyncRunItems(w http.ResponseWriter, r *http.Request) {
	syncStore := RT().GetTraktSyncStore()
	if syncStore == nil {
		writeJSONError(w, http.StatusInternalServerError, "观看同步存储不可用")
		return
	}
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
func traktMediaTitle(mediaType string, tmdbID int64) string {
	libStore := RT().GetLibraryStore()
	if libStore == nil {
		return ""
	}
	if row := libStore.GetTmdbEntry(mediaType, tmdbID); row != nil && row.Title != "" {
		return row.Title
	}
	return libStore.GetMediaTitle(mediaType, tmdbID)
}

// handleTraktSyncIgnores GET /api/trakt/sync/ignores：忽略清单 + 反复失败候选。
func handleTraktSyncIgnores(w http.ResponseWriter, _ *http.Request) {
	syncStore := RT().GetTraktSyncStore()
	if syncStore == nil {
		writeJSONError(w, http.StatusInternalServerError, "观看同步存储不可用")
		return
	}
	ignores := make([]map[string]any, 0)
	for _, it := range syncStore.ListSyncIgnores() {
		ignores = append(ignores, map[string]any{
			"media_type": it.MediaType, "tmdb_id": it.TmdbID,
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
			title = traktMediaTitle(c.MediaType, c.TmdbID)
		}
		candidates = append(candidates, map[string]any{
			"media_type": c.MediaType, "tmdb_id": c.TmdbID, "title": title,
			"fails": c.Fails, "last_error": c.LastError, "last_at": c.LastAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ignores": ignores, "candidates": candidates})
}

// handleTraktSyncIgnoreSet POST /api/trakt/sync/ignore
// {action: "add"|"remove", media_type, tmdb_id, reason?}。
func handleTraktSyncIgnoreSet(w http.ResponseWriter, r *http.Request) {
	syncStore := RT().GetTraktSyncStore()
	if syncStore == nil {
		writeJSONError(w, http.StatusInternalServerError, "观看同步存储不可用")
		return
	}
	var req struct {
		Action    string `json:"action"`
		MediaType string `json:"media_type"`
		TmdbID    int64  `json:"tmdb_id"`
		Reason    string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if (req.MediaType != "movie" && req.MediaType != "tv") || req.TmdbID <= 0 {
		writeJSONError(w, http.StatusBadRequest, "media_type 须为 movie/tv 且 tmdb_id 有效")
		return
	}
	switch req.Action {
	case "add":
		title := traktMediaTitle(req.MediaType, req.TmdbID)
		if err := syncStore.AddSyncIgnore(req.MediaType, req.TmdbID, title, req.Reason); err != nil {
			writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("保存忽略失败：%v", err))
			return
		}
		appLog("trakt", "ignore_added", "INFO",
			fmt.Sprintf("已忽略媒体 %s:%d（两侧均不再同步）", req.MediaType, req.TmdbID), nil)
		writeJSON(w, http.StatusOK, map[string]any{"message": "已忽略，该媒体将不再参与同步"})
	case "remove":
		if err := syncStore.RemoveSyncIgnore(req.MediaType, req.TmdbID); err != nil {
			writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("解除忽略失败：%v", err))
			return
		}
		appLog("trakt", "ignore_removed", "INFO",
			fmt.Sprintf("已解除忽略 %s:%d，下轮同步自动纳入", req.MediaType, req.TmdbID), nil)
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
	if libStore == nil {
		return
	}
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

// ── 会话轮询（正在看 → Trakt）───────────────────────────────

// sessionPushThreshold 视为「已看」的进度阈值（与 Trakt scrobble 规则一致）。
const sessionPushThreshold = 0.8

// sessionPushCooldown 同一观看推送后的最小间隔（防重复推送）。
const sessionPushCooldown = int64(3600)

// sessionEchoSkew trakt 同步状态已是新看时视为回声的时间窗口（与引擎 echoSkew 对齐）。
const sessionEchoSkew = int64(600)

// sessionMaxMisses 连续缺席轮询若干次后移除会话观察行。
const sessionMaxMisses = 2

// sessionDisplayTitle 会话条目的可读标题。
func sessionDisplayTitle(item *plexserver.Metadata) string {
	if item.Type == "episode" && item.GrandparentTitle != "" {
		return fmt.Sprintf("%s S%02dE%02d「%s」", item.GrandparentTitle, item.ParentIndex, item.Index, item.Title)
	}
	return item.Title
}

// pollPlexSessionsOnce 单次会话轮询：≥80% 进度的会话推送到 Trakt。
func pollPlexSessionsOnce() {
	cfg := RT().GetConfig()
	nowStr := utcNowISO()
	traktSessionWatcher.mu.Lock()
	traktSessionWatcher.lastPolledAt = &nowStr
	traktSessionWatcher.lastPollError = nil
	traktSessionWatcher.mu.Unlock()

	// 纯兜底策略：SSE 在线时不并行轮询——实时事件已覆盖会话观察与推送，
	// 仅在 SSE 未连接（开关关闭/断线窗口/旧版 PMS 无端点）时才主动轮询。
	// 心跳簿记已在上方更新，watcher 存活状态不受影响。
	if sseConnected() {
		return
	}
	if !cfg.Trakt.SyncToTrakt || !cfg.IsPlexReady() || !cfg.IsTraktReady() {
		return
	}
	if authorized, _ := traktOauthStatusPayload()["authorized"].(bool); !authorized {
		return
	}
	if TraktSyncRunning() {
		return // 避免与全量同步交叉写 Trakt
	}
	plexClient := RT().GetPlexClient()
	if plexClient == nil {
		return
	}
	traktClient := RT().GetTraktClient()
	if traktClient == nil {
		return
	}
	syncStore := RT().GetTraktSyncStore()
	if syncStore == nil {
		return
	}
	sessions, err := plexClient.Sessions()
	if err != nil {
		msg := fmt.Sprintf("%v", err)
		traktSessionWatcher.mu.Lock()
		traktSessionWatcher.lastPollError = &msg
		traktSessionWatcher.mu.Unlock()
		if errors.Is(err, plexserver.ErrUnauthorized) {
			NotifyServiceHealth("Plex", false, err.Error())
		}
		return
	}
	// 会话查询成功即 Plex 可达且授权有效：SSE 关闭（默认）时这里是唯一的 Plex 恢复上报点
	NotifyServiceHealth("Plex", true, "")

	seen := map[string]bool{}
	source := plexserver.NewSource(plexClient, buildTitleIndex())
	for i := range sessions {
		sess := &sessions[i]
		seen[sess.SessionKey] = true

		key, keyOK := resolveSessionKey(syncStore, source, sess)
		progress := 0.0
		if sess.Duration > 0 {
			progress = float64(sess.ViewOffset) / float64(sess.Duration)
		}
		title := sessionDisplayTitle(sess)
		row := store.TraktSyncSession{
			SessionKey: sess.SessionKey, PlexRef: sess.RatingKey,
			ViewOffsetMS: int64(sess.ViewOffset), DurationMS: int64(sess.Duration),
			MaxProgress: progress, Title: &title,
		}
		if keyOK {
			row.MediaType = key.Type
			row.TmdbID = key.TMDBID
			row.Season = key.Season
			row.Episode = key.Episode
		}
		syncStore.UpsertSyncSession(row)
		if keyOK {
			tryPushWatchedSession(syncStore, traktClient, key, title, progress, "poll")
		}
	}

	// 缺席检测：连续 2 次未见移除
	for _, row := range syncStore.ListSyncSessions() {
		if seen[row.SessionKey] {
			continue
		}
		if misses := syncStore.IncrementSessionMiss(row.SessionKey); misses >= sessionMaxMisses {
			syncStore.DeleteSyncSession(row.SessionKey)
		}
	}
}

// tryPushWatchedSession 会话进度达标（≥80%）时把该集推送到 Trakt。
// 内置防重门控（近期推送过 / trakt 同步状态已是新看则跳过），
// 轮询与 SSE 实时推送两条路径共用，保证不会重复计 plays。
// source 标记来源（"sse" / "poll"），title 为展示用媒体标题；
// 每次实际尝试（无论成败）都经 RecordSessionPushRun 落一条 run 记录，
// 防重闸静默跳过的不记录。
// traktSessionPushMu 串行化单媒体推送：SSE worker 与会话轮询并发调用本核心，
// 防重闸（session_pushed_at/echo）的检查与写入须原子完成，避免同瞬双推重复 play。
var traktSessionPushMu sync.Mutex

func tryPushWatchedSession(syncStore *store.TraktSyncStore, traktClient *trakt.Client, key watched.MediaKey, title string, progress float64, source string) bool {
	traktSessionPushMu.Lock()
	defer traktSessionPushMu.Unlock()
	if progress < sessionPushThreshold {
		return false
	}
	if !RT().GetConfig().Trakt.SyncToTrakt {
		return false
	}
	// 已忽略的媒体不参与同步（含实时推送），静默跳过不落记录
	if syncStore.IsIgnoredMedia(key.Type, key.TMDBID) {
		return false
	}
	shadow, hasShadow, _ := syncStore.GetShadow(key)
	now := time.Now().Unix()
	if hasShadow {
		if shadow.SessionPushedAt > 0 && now-shadow.SessionPushedAt < sessionPushCooldown {
			return false
		}
		if shadow.TraktWatched != nil && *shadow.TraktWatched && now-shadow.TraktLastAt < sessionEchoSkew {
			return false
		}
	}
	item := syncengine.RunItem{
		Key: key, Title: title, Direction: "to_trakt", Action: "watched",
	}
	record := func(success bool, errMsg string) {
		_ = syncStore.RecordSessionPushRun(source, item, progress, success, errMsg)
	}
	movies := []trakt.HistoryMovie{}
	episodes := []trakt.HistoryEpisode{}
	if key.Type == "movie" {
		movies = append(movies, trakt.HistoryMovie{TMDBID: key.TMDBID, WatchedAt: time.Unix(now, 0)})
	} else {
		episodes = append(episodes, trakt.HistoryEpisode{
			ShowTMDBID: key.TMDBID, Season: key.Season, Episode: key.Episode,
			WatchedAt: time.Unix(now, 0),
		})
	}
	result, err := traktClient.AddHistory(movies, episodes)
	if err != nil {
		record(false, fmt.Sprintf("Trakt 写入失败：%v", err))
		appLog("trakt", "session_push_failed", "WARNING",
			fmt.Sprintf("播放进度推送 Trakt 失败（%s）：%v", key, err), nil)
		if errors.Is(err, trakt.ErrUnauthorized) {
			NotifyServiceHealth("Trakt", false, "播放进度推送认证失败："+err.Error())
		}
		return false
	}
	if result.NotFoundCount > 0 {
		record(false, "Trakt 未匹配到该媒体（not_found），稍后自动重试")
		appLog("trakt", "session_push_failed", "WARNING",
			fmt.Sprintf("播放进度推送 Trakt 未匹配（%s）：not_found，稍后自动重试", key), nil)
		return false
	}
	_ = syncStore.UpdateTraktWatched(key, now, now)
	_ = syncStore.SetTraktPushedAt(now) // 供下轮全量同步的过期读取防护
	record(true, "")
	appLog("trakt", "session_pushed", "SUCCESS",
		fmt.Sprintf("播放进度已记录到 Trakt：%s（进度 %.0f%%）", key, progress*100), nil)
	// 推送成功即 Trakt 授权有效：会话推送是独立于全量同步的恢复上报点
	NotifyServiceHealth("Trakt", true, "")
	return true
}

// ── 实时推送（Plex SSE）─────────────────────────────────────

// traktSSEState SSE 连接状态（状态接口展示用）。
var traktSSEState struct {
	mu        sync.Mutex
	connected bool
}

func setSSEConnected(connected bool) {
	traktSSEState.mu.Lock()
	traktSSEState.connected = connected
	traktSSEState.mu.Unlock()
}

func sseConnected() bool {
	traktSSEState.mu.Lock()
	defer traktSSEState.mu.Unlock()
	return traktSSEState.connected
}

// sseEventQueueSize SSE 事件异步分发的有界队列长度：
// 处理慢（如 Trakt 冷却）时丢弃最旧事件——进度事件几秒后自然再来，
// 且 45s 轮询兜底，丢事件不影响正确性，只保证连接读循环不被卡死。
const sseEventQueueSize = 64

// handlePlexPlaybackNotification 处理一条实时播放通知：
// 更新会话观察行，进度达标即推送 Trakt（与轮询共用门控）。
func handlePlexPlaybackNotification(plexClient *plexserver.Client, traktClient *trakt.Client, syncStore *store.TraktSyncStore, notif plexserver.PlaybackNotification) {
	defer func() {
		if r := recover(); r != nil {
			appLog("trakt", "sse_event_panic", "ERROR", fmt.Sprintf("实时通知处理异常：%v", r), nil)
		}
	}()
	if notif.RatingKey == "" {
		return
	}
	// duration 缺失时先取会话观察行，再退回详情接口
	duration := notif.DurationMS
	if row := syncStore.GetSyncSession(notif.SessionKey); row != nil {
		if duration == 0 {
			duration = row.DurationMS
		}
	}
	if duration == 0 {
		if md, err := plexClient.Metadata(notif.RatingKey); err == nil {
			duration = int64(md.Duration)
		}
	}
	source := plexserver.NewSource(plexClient, buildTitleIndex())
	sess := &plexserver.Metadata{
		SessionKey: notif.SessionKey, RatingKey: notif.RatingKey,
		ViewOffset: int(notif.ViewOffsetMS), Duration: int(duration),
	}
	key, keyOK := resolveSessionKey(syncStore, source, sess)

	progress := 0.0
	if duration > 0 {
		progress = float64(notif.ViewOffsetMS) / float64(duration)
	}
	title := sessionDisplayTitle(sess)
	row := store.TraktSyncSession{
		SessionKey: notif.SessionKey, PlexRef: notif.RatingKey,
		ViewOffsetMS: notif.ViewOffsetMS, DurationMS: duration,
		MaxProgress: progress, Title: &title,
	}
	if keyOK {
		row.MediaType = key.Type
		row.TmdbID = key.TMDBID
		row.Season = key.Season
		row.Episode = key.Episode
	}
	syncStore.UpsertSyncSession(row)
	if keyOK {
		tryPushWatchedSession(syncStore, traktClient, key, title, progress, "sse")
	}
	// 播放结束即清理会话观察行：轮询不与 SSE 并行运行后，
	// 缺席检测不再触发，收尾改由 stopped 事件承担
	if notif.State == "stopped" {
		syncStore.DeleteSyncSession(notif.SessionKey)
	}
}

// ensureTraktSSELoop 启动 Plex 实时通知订阅（幂等）：
// 常驻一条 SSE 连接，断线指数退避重连；事件经有界队列异步分发。
func ensureTraktSSELoop() {
	traktSSEOnce.Do(func() {
		go func() {
			log.Printf("Plex 实时通知线程已启动")
			backoff := 5 * time.Second
			lastGateOK := true
			for {
				// 门控只看 sse 开关、配对就绪与 →Trakt 方向，不受「启用定时同步」影响：
				// 实时推送是事件驱动的观看同步，与定时全量是两个独立开关
				gate := func() (plexClient *plexserver.Client, traktClient *trakt.Client, syncStore *store.TraktSyncStore, ok bool) {
					cfg := RT().GetConfig()
					if !cfg.Trakt.SSEEnabled || !cfg.Trakt.SyncToTrakt || !cfg.IsPlexReady() || !cfg.IsTraktReady() {
						return nil, nil, nil, false
					}
					plexClient = RT().GetPlexClient()
					traktClient = RT().GetTraktClient()
					syncStore = RT().GetTraktSyncStore()
					if plexClient == nil || traktClient == nil || syncStore == nil {
						return nil, nil, nil, false
					}
					return plexClient, traktClient, syncStore, true
				}
				plexClient, traktClient, syncStore, ok := gate()
				if !ok {
					setSSEConnected(false)
					if lastGateOK {
						log.Printf("Plex 实时通知未启用或未就绪（需开启 sse_enabled、完成 Trakt 与 Plex 配对并启用 →Trakt 方向），30 秒后重新检查")
						lastGateOK = false
					}
					if !sleepWatcher(traktSSEWatcher, 30*time.Second) {
						return
					}
					continue
				}
				lastGateOK = true

				// 有界异步分发：读循环永不被 Trakt 写入阻塞
				events := make(chan plexserver.PlaybackNotification, sseEventQueueSize)
				go func() {
					defer func() {
						if r := recover(); r != nil {
							appLog("trakt", "sse_worker_panic", "ERROR", fmt.Sprintf("实时事件分发异常：%v", r), nil)
						}
					}()
					for notif := range events {
						handlePlexPlaybackNotification(plexClient, traktClient, syncStore, notif)
					}
				}()

				// 「已连接」只在真正订阅成功（HTTP 200）后报告，
				// 订阅被拒/断线不算连接成功，退避继续累加
				var openedAt time.Time
				ctx, cancel := context.WithCancel(context.Background())
				go func() {
					select {
					case <-traktSSEWatcher.stopCh:
						cancel()
					case <-ctx.Done():
					}
				}()
				err := plexClient.StreamNotifications(ctx, func() {
					openedAt = time.Now()
					log.Printf("Plex 实时通知已连接：%s", plexClient.BaseURL)
					setSSEConnected(true)
					NotifyServiceHealth("Plex", true, "")
					appLog("trakt", "sse_connected", "INFO",
						fmt.Sprintf("Plex 实时通知已连接（%s）", plexClient.BaseURL), nil)
				}, func(n plexserver.PlaybackNotification) {
					select {
					case events <- n:
					default:
						// 队列满：丢最旧，保最新
						select {
						case <-events:
						default:
						}
						select {
						case events <- n:
						default:
						}
					}
				})
				cancel()
				close(events)
				setSSEConnected(false)
				if errors.Is(err, plexserver.ErrUnauthorized) {
					NotifyServiceHealth("Plex", false, err.Error())
				}
				msg := "连接结束"
				if err != nil {
					msg = err.Error()
				}
				appLog("trakt", "sse_disconnected", "WARNING",
					fmt.Sprintf("Plex 实时通知已断开（%s），将于 %s 后重新连接，期间由会话轮询同步", msg, backoff), nil)
				// 退避策略：稳定连接（存活 ≥60s）后重置；秒断（订阅被拒/服务不可达）
				// 则逐次加倍直至 60s，避免日志刷屏；端点不存在（旧版 PMS）降到 10 分钟一探
				switch {
				case errors.Is(err, plexserver.ErrNotificationsUnsupported):
					backoff = 10 * time.Minute
				case !openedAt.IsZero() && time.Since(openedAt) >= 60*time.Second:
					backoff = 5 * time.Second
				case backoff < 60*time.Second:
					backoff *= 2
				}
				if !sleepWatcher(traktSSEWatcher, backoff) {
					return
				}
			}
		}()
	})
}

// sleepWatcher 可中断的睡眠：stopCh 关闭时返回 false（线程退出）。
func sleepWatcher(w *watcherState, d time.Duration) bool {
	select {
	case <-w.stopCh:
		return false
	case <-time.After(d):
		return true
	}
}

// resolveSessionKey 会话身份解析：状态记录快路径 → 会话行缓存 → Plex 详情补解析。
func resolveSessionKey(store *store.TraktSyncStore, source *plexserver.Source, sess *plexserver.Metadata) (watched.MediaKey, bool) {
	if shadow, ok, _ := store.GetShadowByPlexRef(sess.RatingKey); ok && shadow.Key.TMDBID > 0 {
		return shadow.Key, true
	}
	if row := store.GetSyncSession(sess.SessionKey); row != nil && row.TmdbID > 0 {
		return watched.MediaKey{
			Type: row.MediaType, TMDBID: row.TmdbID, Season: row.Season, Episode: row.Episode,
		}, true
	}
	return source.ResolveMetadataRef(sess.RatingKey)
}

// ── 后台轮询循环 ───────────────────────────────────────────

var (
	traktWatcher        = &watcherState{stopCh: make(chan struct{})}
	traktSessionWatcher = &watcherState{stopCh: make(chan struct{})}
	traktSSEWatcher     = &watcherState{stopCh: make(chan struct{})}
	traktOnce           sync.Once
	traktSessionOnce    sync.Once
	traktSSEOnce        sync.Once
)

// plexReachable 轻探测 Plex 可达性（/identity，失败含 DNS/连接错误）。
func plexReachable() bool {
	plexClient := RT().GetPlexClient()
	if plexClient == nil {
		return true // 未完成 Plex 配对时不等待，由同步自身报出明确的配置错误
	}
	_, err := plexClient.Identity()
	return err == nil
}

// waitPlexReachable 等待 Plex 恢复可达：30 秒一探，上限 10 分钟。
// Plex 常与本服务同 compose 部署、随之同时重启——启动首轮同步若在
// 对端未就绪时触发，会在 DNS/连接层立刻失败并留下错误记录。探测
// 不可达时短周期等待，恢复后立即开始本轮；超时放弃，交由下一轮定时。
func waitPlexReachable() bool {
	if plexReachable() {
		return true
	}
	appLog("trakt", "plex_unreachable", "WARNING", "Plex 暂不可达，等待就绪后开始本轮同步", nil)
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		if !sleepWatcher(traktWatcher, 30*time.Second) {
			return false
		}
		if plexReachable() {
			return true
		}
	}
	appLog("trakt", "plex_unreachable_timeout", "WARNING", "等待 Plex 就绪超时，本轮定时同步跳过", nil)
	return false
}

// ensureTraktSyncLoop 启动观看同步定时循环（幂等；按 trakt.sync_cron 计划执行，
// 未配置 cron 时仅 SSE 实时推送/会话轮询/手动触发）。
func ensureTraktSyncLoop() {
	traktOnce.Do(func() {
		go func() {
			log.Printf("观看同步定时线程已启动（按 trakt.sync_cron 计划执行；未配置 cron 时仅 SSE 实时/会话轮询/手动触发）")
			cronWarned := "" // 已告警过的非法 cron：仅状态变化时告警，避免每 60s 重复刷日志
			lastGateOK := true
			for {
				spec := RT().GetConfig().Trakt.SyncCron
				d, ok, err := cronNextDelay(spec, time.Now())
				if err != nil {
					if cronWarned != spec {
						appLog("trakt", "sync_cron_invalid", "ERROR",
							"观看同步 cron 表达式无效，60s 后重试："+err.Error(), map[string]any{"cron": spec})
						log.Printf("观看同步 cron 表达式无效（%q），60s 后重试", spec)
						cronWarned = spec
					}
					ok = false
				} else {
					cronWarned = "" // 恢复正常后重置，再次非法时重新告警
				}
				if !ok {
					d = 60 * time.Second // 未配置 cron（仅实时/手动触发）或表达式非法：空转等待，保持配置热响应
				}
				select {
				case <-traktWatcher.stopCh:
					log.Printf("观看同步定时线程已停止")
					return
				case <-time.After(d):
				}
				if !ok {
					continue
				}
				func() {
					defer func() {
						if r := recover(); r != nil {
							msg := fmt.Sprintf("%v", r)
							traktWatcher.mu.Lock()
							traktWatcher.lastPollError = &msg
							traktWatcher.mu.Unlock()
							appLog("trakt", "sync_loop_panic", "ERROR", "观看同步轮询异常: "+msg, nil)
						}
					}()
					nowStr := utcNowISO()
					traktWatcher.mu.Lock()
					traktWatcher.lastPolledAt = &nowStr
					traktWatcher.mu.Unlock()
					// 触发点现读配置：睡眠前的快照可陈旧整个 cron 周期（如每天一次可达 24h），
					// 睡眠窗口内的开关/配对变更必须以执行时刻的配置为准
					cfg := RT().GetConfig()
					// 定时同步需开关开启且两侧均已配置；不满足时跳过本轮（手动触发会报出明确原因）
					enabled := cfg.Trakt.Enabled && cfg.IsPlexReady() && cfg.IsTraktReady()
					if !enabled {
						if lastGateOK {
							appLog("trakt", "sync_schedule_skipped", "INFO",
								"观看同步定时触发跳过（需开启 trakt.enabled 并完成 Trakt 与 Plex 配对），下个 cron 点重试", nil)
							log.Printf("观看同步定时触发条件未满足，本 cron 点跳过")
							lastGateOK = false
						}
						return
					}
					lastGateOK = true
					if waitPlexReachable() {
						TriggerTraktSync(triggerSchedule)
					}
				}()
			}
		}()
	})
}

// ensureTraktSessionLoop 启动 Plex 会话轮询（正在看 → Trakt）。
// 纯兜底：每轮先查 SSE 是否在线，在线则跳过实际轮询（见 pollPlexSessionsOnce）。
func ensureTraktSessionLoop() {
	traktSessionOnce.Do(func() {
		go func() {
			log.Printf("Plex 会话轮询线程已启动")
			for {
				interval := RT().GetConfig().Trakt.SessionPollSeconds
				if interval < 15 {
					interval = 15
				}
				func() {
					defer func() {
						if r := recover(); r != nil {
							msg := fmt.Sprintf("%v", r)
							traktSessionWatcher.mu.Lock()
							traktSessionWatcher.lastPollError = &msg
							traktSessionWatcher.mu.Unlock()
						}
					}()
					pollPlexSessionsOnce()
				}()
				select {
				case <-traktSessionWatcher.stopCh:
					log.Printf("Plex 会话轮询线程已停止")
					return
				case <-time.After(time.Duration(interval) * time.Second):
				}
			}
		}()
	})
}

// TraktSyncStatusPayload 供外部（如状态聚合）调用。
func TraktSyncStatusPayload() map[string]any { return traktSyncStatusPayload() }

// 编译期接口检查
var (
	_ syncengine.ShadowStore = (*store.TraktSyncStore)(nil)
	_ watched.Provider       = (*plexserver.Source)(nil)
	_ watched.Provider       = (*trakt.TraktSource)(nil)
	_ watched.TitleIndex     = (*titleIndex)(nil)
)
