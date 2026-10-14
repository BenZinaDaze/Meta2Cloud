// wslogs.go —— 实时日志 WebSocket 端点（对应 webui/websocket.py）。
//
// 协议：客户端带 ?token=JWT 连接 /api/logs/live；无效 token 以 4001 关闭。
// 服务端只读丢弃客户端消息，向前端广播 {type:"log", ts, level, message, runId}。
package webui

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// wsUpgrader 与 Python 版一致，不校验 Origin（token 已做认证）。
var wsUpgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
}

// handleLogsLiveWS /api/logs/live WebSocket 处理器。
func handleLogsLiveWS(w http.ResponseWriter, r *http.Request) {
	// token 无效：升级后以 4001 关闭（对齐 Python close(code=4001)）
	if VerifyToken(r.URL.Query().Get("token")) == "" {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(4001, "invalid token"),
			time.Now().Add(time.Second))
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ch, cancel := SubscribeLog()
	defer cancel()

	// 读取循环：丢弃客户端消息，连接断开时通知写循环退出
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn.SetReadLimit(64 * 1024)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case msg := <-ch:
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}
