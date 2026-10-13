// auth.go —— JWT 认证（对应 webui/core/auth.py）。
package webui

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	jwtSecretOnce  sync.Once
	jwtSecretValue string
)

// GetJWTSecret JWT 签名密钥：配置优先，其次持久化文件，否则生成并落盘。
func GetJWTSecret() string {
	jwtSecretOnce.Do(func() {
		cfg := RT().GetConfig()
		if cfg.WebUI.SecretKey != "" {
			jwtSecretValue = cfg.WebUI.SecretKey
			return
		}
		secretFile := filepath.Join(RT().DataDir(), ".jwt_secret")
		if data, err := os.ReadFile(secretFile); err == nil {
			if v := strings.TrimSpace(string(data)); v != "" {
				jwtSecretValue = v
				return
			}
		}
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			// 加密源异常时仍生成一个（安全性下降但不崩溃）
			jwtSecretValue = hex.EncodeToString([]byte(time.Now().String()))
			return
		}
		jwtSecretValue = hex.EncodeToString(buf)
		_ = os.MkdirAll(filepath.Dir(secretFile), 0o755)
		if err := os.WriteFile(secretFile, []byte(jwtSecretValue), 0o600); err != nil {
			log.Printf("无法持久化 JWT 密钥：%v", err)
		}
	})
	return jwtSecretValue
}

// ResetJWTSecretCache 重置密钥缓存（测试用）。
func ResetJWTSecretCache() {
	jwtSecretOnce = sync.Once{}
	jwtSecretValue = ""
}

// CreateToken 签发 HS256 JWT。
func CreateToken(username string, expireHours int) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub": username,
		"iat": now.Unix(),
		"exp": now.Add(time.Duration(expireHours) * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(GetJWTSecret()))
}

// VerifyToken 校验 token，返回用户名；失败返回空串。
func VerifyToken(tokenStr string) string {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(GetJWTSecret()), nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !token.Valid {
		return ""
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return ""
	}
	sub, _ := claims["sub"].(string)
	return sub
}

// authMiddleware 认证中间件：
// 豁免 /api/auth:login、/trigger*、以及非 /api/ 路径（静态资源等）。
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/api/auth:login" || strings.HasPrefix(path, "/trigger") || !strings.HasPrefix(path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeJSONError(w, http.StatusUnauthorized, "未授权，请先登录")
			return
		}
		username := VerifyToken(auth[7:])
		if username == "" {
			writeJSONError(w, http.StatusUnauthorized, "Token 已过期或无效，请重新登录")
			return
		}
		next.ServeHTTP(w, r)
	})
}
