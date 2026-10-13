// aria2svc.go —— Aria2 JSON-RPC 最小实现（订阅推送 addUri 所需子集，
// 对应 webui/services/aria2.py 的 aria2_rpc_call / aria2_add_uri_payload）。
package webui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// aria2RPCCall 调用 aria2 JSON-RPC。method 如 "aria2.addUri"。
func aria2RPCCall(method string, params []any) (any, error) {
	cfg := RT().GetConfig().Aria2
	if !cfg.Enabled {
		return nil, fmt.Errorf("Aria2 未启用")
	}
	scheme := "http"
	if cfg.Secure {
		scheme = "https"
	}
	rpcURL := fmt.Sprintf("%s://%s:%d%s", scheme, cfg.Host, cfg.Port, cfg.Path)

	// secret 前缀参数（aria2 约定：token:xxx 作为第一个参数）
	rpcParams := make([]any, 0, len(params)+1)
	if cfg.Secret != "" {
		rpcParams = append(rpcParams, "token:"+cfg.Secret)
	}
	rpcParams = append(rpcParams, params...)

	requestID := time.Now().UnixNano()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": requestID, "method": method, "params": rpcParams,
	})
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(rpcURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("无法连接 Aria2 RPC：%v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Aria2 RPC HTTP %d", resp.StatusCode)
	}
	var result struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("Aria2 RPC 响应解析失败：%v", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("Aria2 RPC 错误 %d: %s", result.Error.Code, result.Error.Message)
	}
	var out any
	if json.Unmarshal(result.Result, &out) == nil {
		return out, nil
	}
	return string(result.Result), nil
}

// aria2AddURI 添加下载链接（订阅推送用）。返回 gid。
func aria2AddURI(uris []string) (string, error) {
	var cleaned []string
	for _, uri := range uris {
		if uri = strings.TrimSpace(uri); uri != "" {
			cleaned = append(cleaned, uri)
		}
	}
	if len(cleaned) == 0 {
		return "", fmt.Errorf("至少需要一个下载链接")
	}
	result, err := aria2RPCCall("aria2.addUri", []any{cleaned, map[string]string{}})
	if err != nil {
		return "", err
	}
	gid, _ := result.(string)
	appLog("download", "task_added_uri", "SUCCESS", "已添加链接下载任务",
		map[string]any{"gid": gid, "uriCount": len(cleaned)})
	return gid, nil
}
