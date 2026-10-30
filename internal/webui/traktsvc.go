// traktsvc.go —— Trakt/Plex OAuth：授权状态载荷、设备码配对与 PIN 配对处理器。
// （会话持久化见 oauth_session.go，双向同步见 traktsyncsvc.go，播放推送见 scrobble.go / plexsse.go）
package webui

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"panoply/internal/plexserver"
	"panoply/internal/trakt"
)

// ── 状态载荷 ───────────────────────────────────────────────

// TraktPairingPayload 进行中的设备码配对会话（前端据此继续轮询）。
type TraktPairingPayload struct {
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresAt       int64  `json:"expires_at"`
}

// TraktOAuthStatusPayload GET /api/trakt/oauth/status 响应
// （也内嵌于设备码创建/断开授权的响应 status 字段）。
type TraktOAuthStatusPayload struct {
	Ready        bool                 `json:"ready"`
	ClientID     string               `json:"client_id"`
	TokenPath    string               `json:"token_path"`
	TokenExists  bool                 `json:"token_exists"`
	Authorized   bool                 `json:"authorized"`
	TokenValid   bool                 `json:"token_valid"`
	TokenExpired bool                 `json:"token_expired"`
	RefreshTime  *int64               `json:"refresh_time"`
	ExpiresAt    *int64               `json:"expires_at"`
	Refreshed    bool                 `json:"refreshed"`
	Pairing      *TraktPairingPayload `json:"pairing"`
}

func (s *traktService) traktOauthStatusPayload() TraktOAuthStatusPayload {
	cfg := s.rt.GetConfig().Trakt
	ready := s.rt.GetConfig().IsTraktReady()
	tokenPath := s.rt.resolveConfigPath(cfg.TokenJSON)
	token, tokenExists := func() (trakt.Token, bool) {
		t, ok, err := trakt.LoadToken(tokenPath)
		if err != nil {
			return trakt.Token{}, false
		}
		return t, ok
	}()

	authorized, tokenValid, tokenExpired, refreshed := false, false, false, false
	var expiresAt, refreshTime *int64
	if tokenExists {
		refreshTime = &token.RefreshTime
		exp := token.ExpiresAt()
		expiresAt = &exp
		tokenExpired = time.Now().Unix() >= exp
		if tokenExpired {
			// 尝试自动刷新（与 115 状态载荷同思路）
			if client := s.rt.GetTraktClient(); client != nil {
				if err := client.RefreshToken(); err == nil {
					refreshed = true
					if t, ok, _ := trakt.LoadToken(tokenPath); ok {
						refreshTime = &t.RefreshTime
						exp = t.ExpiresAt()
						expiresAt = &exp
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
	session, _ := s.rt.loadTraktDeviceSession()
	var pairing *TraktPairingPayload
	if session != nil && time.Now().Unix() < session.ExpiresAt {
		pairing = &TraktPairingPayload{
			UserCode: session.UserCode, VerificationURL: session.VerificationURL,
			ExpiresAt: session.ExpiresAt,
		}
	}
	return TraktOAuthStatusPayload{
		Ready:    ready,
		ClientID: cfg.ClientID, TokenPath: cfg.TokenJSON,
		TokenExists: tokenExists, Authorized: authorized,
		TokenValid: tokenValid, TokenExpired: tokenExpired,
		RefreshTime: refreshTime, ExpiresAt: expiresAt, Refreshed: refreshed,
		Pairing: pairing,
	}
}

// PlexPairingPayload 进行中的 PIN 配对会话。
type PlexPairingPayload struct {
	Code string `json:"code"`
}

// PlexStatusPayload Plex 连接状态（观看同步状态接口的内嵌来源，非独立 API）。
type PlexStatusPayload struct {
	Ready            bool                `json:"ready"`
	BaseURL          string              `json:"base_url"`
	AuthPath         string              `json:"auth_path"`
	TokenConfigured  bool                `json:"token_configured"`
	ClientIdentifier string              `json:"client_identifier"`
	Reachable        bool                `json:"reachable"`
	Pairing          *PlexPairingPayload `json:"pairing,omitempty"`
	ServerName       string              `json:"server_name,omitempty"`
	ServerVersion    string              `json:"server_version,omitempty"`
	Error            string              `json:"error,omitempty"`
}

func (s *traktService) plexStatusPayload() PlexStatusPayload {
	cfg := s.rt.GetConfig()
	authPath := s.rt.plexAuthPath()
	auth, authErr := s.rt.loadPlexAuth(authPath)
	hasToken := authErr == nil && auth.AuthToken != ""
	identifier := ""
	if authErr == nil {
		identifier = auth.ClientIdentifier
	}
	payload := PlexStatusPayload{
		Ready:            cfg.IsPlexReady(),
		BaseURL:          cfg.Plex.BaseURL,
		AuthPath:         cfg.Plex.TokenJSON,
		TokenConfigured:  hasToken,
		ClientIdentifier: identifier,
	}
	if pinSession, err := s.rt.loadPlexPinSession(); err == nil {
		payload.Pairing = &PlexPairingPayload{Code: pinSession.Code}
	}
	if !cfg.IsPlexReady() || !hasToken {
		return payload
	}
	client := s.rt.GetPlexClient()
	if client == nil {
		return payload
	}
	if identity, err := client.Identity(); err == nil {
		payload.Reachable = true
		payload.ServerName = identity.MediaContainer.MachineIdentifier
		payload.ServerVersion = identity.MediaContainer.Version
	} else {
		payload.Error = err.Error()
	}
	return payload
}

// ── HTTP 处理器：Trakt 设备码 ───────────────────────────────

func (s *Server) handleTraktOauthStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.rt.traktSvc.traktOauthStatusPayload())
}

func (s *Server) handleTraktOauthDevice(w http.ResponseWriter, _ *http.Request) {
	if !s.rt.GetConfig().IsTraktReady() {
		writeJSONError(w, http.StatusBadRequest, "请先在配置中填写 trakt.client_id（trakt.tv/oauth/applications 创建应用；Development 应用无需 Client Secret）")
		return
	}
	client := s.rt.GetTraktClient()
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
	if err := s.rt.saveTraktDeviceSession(&traktDeviceSessionJSON{
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
		"status": s.rt.traktSvc.traktOauthStatusPayload(),
	})
}

func (s *Server) handleTraktOauthPoll(w http.ResponseWriter, _ *http.Request) {
	client := s.rt.GetTraktClient()
	if client == nil {
		writeJSONError(w, http.StatusBadRequest, "Trakt 客户端不可用（请先填写应用凭证并保存）")
		return
	}
	session, err := s.rt.loadTraktDeviceSession()
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	if time.Now().Unix() >= session.ExpiresAt {
		_ = os.Remove(s.rt.traktSessionPath())
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
		s.rt.invalidateTraktRuntimeCache()
		// 会话已消费，删除会话文件
		if onDisk, err := s.rt.loadTraktDeviceSession(); err == nil && onDisk.DeviceCode == session.DeviceCode {
			_ = os.Remove(s.rt.traktSessionPath())
		}
		user, _ := client.Me()
		s.rt.appLog("trakt", "oauth_paired", "SUCCESS", "Trakt 授权成功（用户 "+user+"）", nil)
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

func (s *Server) handleTraktOauthDisconnect(w http.ResponseWriter, _ *http.Request) {
	if client := s.rt.GetTraktClient(); client != nil {
		client.Disconnect()
	} else {
		// 客户端不可用（凭证缺失）时直接删 token 文件
		_ = os.Remove(s.rt.resolveConfigPath(s.rt.GetConfig().Trakt.TokenJSON))
	}
	s.rt.invalidateTraktRuntimeCache()
	s.rt.appLog("trakt", "oauth_disconnected", "INFO", "已断开 Trakt 授权", nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "status": s.rt.traktSvc.traktOauthStatusPayload(),
	})
}

// ── HTTP 处理器：Plex PIN ───────────────────────────────────

func (s *Server) handlePlexPin(w http.ResponseWriter, _ *http.Request) {
	cfg := s.rt.GetConfig()
	if !cfg.IsPlexReady() {
		writeJSONError(w, http.StatusBadRequest, "请先在配置中填写 plex.base_url")
		return
	}
	rt := s.rt
	rt.mu.Lock()
	auth := s.rt.ensurePlexAuthLocked()
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
	if err := s.rt.savePlexPinSession(&plexPinSessionJSON{
		PinID: pin.ID, Code: pin.Code, ClientIdentifier: auth.ClientIdentifier,
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("配对会话保存失败：%v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "pin_id": pin.ID, "code": pin.Code, "auth_url": authURL,
	})
}

func (s *Server) handlePlexPinPoll(w http.ResponseWriter, _ *http.Request) {
	cfg := s.rt.GetConfig()
	session, err := s.rt.loadPlexPinSession()
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
	authPath := s.rt.plexAuthPath()
	auth := &plexAuthJSON{
		AuthToken: pin.AuthToken, ClientIdentifier: session.ClientIdentifier,
		CreatedAt: time.Now().Unix(),
	}
	if old, err := s.rt.loadPlexAuth(authPath); err == nil && old.CreatedAt > 0 {
		auth.CreatedAt = old.CreatedAt
	}
	if err := s.rt.savePlexAuth(authPath, auth); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Plex 凭证保存失败：%v", err))
		return
	}
	_ = os.Remove(s.rt.plexPinSessionPath())
	s.rt.invalidatePlexRuntimeCache()

	serverVersion := ""
	reachable := false
	if plexClient := s.rt.GetPlexClient(); plexClient != nil {
		if identity, ierr := plexClient.Identity(); ierr == nil {
			reachable = true
			serverVersion = identity.MediaContainer.Version
		}
	}
	s.rt.appLog("trakt", "plex_paired", "SUCCESS",
		fmt.Sprintf("Plex 授权成功（服务器可达=%v）", reachable), nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "confirmed": true, "authorized": true,
		"reachable": reachable, "server_version": serverVersion,
		"message": "Plex 授权完成",
	})
}

func (s *Server) handlePlexTest(w http.ResponseWriter, _ *http.Request) {
	client := s.rt.GetPlexClient()
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
