package webui

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func wsURL(ts *httptest.Server, path string) string {
	return "ws" + strings.TrimPrefix(ts.URL, "http") + path
}

func TestLogsLiveWebsocket(t *testing.T) {
	ts, token := newScanServer(t)

	// 1. 无效 token → 4001 关闭
	conn, _, err := websocket.DefaultDialer.Dial(wsURL(ts, "/api/logs/live?token=bad"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = conn.ReadMessage() // gorilla 返回 close frame 错误
	if ce, ok := err.(*websocket.CloseError); !ok || ce.Code != 4001 {
		t.Errorf("无效 token 关闭码 = %v, want 4001", err)
	}

	// 2. 有效 token → 连接成功,收到广播
	conn2, _, err := websocket.DefaultDialer.Dial(wsURL(ts, "/api/logs/live?token=")+token, nil)
	if err != nil {
		t.Fatalf("dial valid: %v", err)
	}
	defer conn2.Close()

	BroadcastLog("SUCCESS", "测试消息 ABC123", "run000001")

	_ = conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn2.ReadMessage()
	if err != nil {
		t.Fatalf("read broadcast: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(msg, &payload); err != nil {
		t.Fatalf("payload 不是 JSON: %s", msg)
	}
	if payload["type"] != "log" || payload["level"] != "SUCCESS" || payload["message"] != "测试消息 ABC123" {
		t.Errorf("payload = %v", payload)
	}
	if payload["event"] != "pipeline_output" || payload["category"] != "pipeline" {
		t.Errorf("payload 应携带完整字段（category/event）：%v", payload)
	}
	details, _ := payload["details"].(map[string]any)
	if details == nil || details["runId"] != "run000001" {
		t.Errorf("details.runId = %v", payload["details"])
	}

	// 3. 广播同时写入了日志库(前端实时页与历史页数据一致)
	ls := RT().GetLogStore()
	entries := ls.Read(10, "pipeline", "", 0)
	found := false
	for _, e := range entries {
		if e.Message == "测试消息 ABC123" {
			found = true
		}
	}
	if !found {
		t.Errorf("广播消息未写入日志库, entries=%d", len(entries))
	}
}

// 4. 中间件豁免: WS 升级请求不带 Authorization 头也能到达处理器
//    (上面 Dial 均未带 Bearer 头,已隐式验证)
