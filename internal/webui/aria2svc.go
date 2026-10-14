// aria2svc.go —— Aria2 JSON-RPC 客户端（订阅推送使用的 addUri 薄封装）。
// 完整实现见 aria2full.go。
package webui

// aria2AddURI 添加下载链接（订阅推送用）。返回 gid。
func aria2AddURI(uris []string) (string, error) {
	gid, _, err := aria2AddURICore(uris, nil, "")
	return gid, err
}
