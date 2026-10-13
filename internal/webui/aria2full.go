// aria2full.go —— Aria2 下载器完整服务（对应 webui/services/aria2_core.py + aria2.py）。
package webui

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
)

var aria2TaskKeys = []string{
	"gid", "status", "totalLength", "completedLength", "uploadLength",
	"downloadSpeed", "uploadSpeed", "connections", "numSeeders", "seeder",
	"errorCode", "errorMessage", "dir", "files", "bittorrent",
}

var aria2GlobalOptionKeys = []string{
	"dir", "max-concurrent-downloads", "max-overall-download-limit",
	"max-overall-upload-limit", "split", "max-connection-per-server",
	"min-split-size", "continue", "max-tries", "retry-wait",
	"user-agent", "all-proxy", "seed-ratio", "seed-time", "bt-max-peers",
}

var aria2TorrentNameRe = regexp.MustCompile(`^[0-9a-fA-F]{40}\.torrent$`)

// aria2ListString 将 task keys 转为 RPC 参数。
func aria2Keys() []any {
	out := make([]any, 0, len(aria2TaskKeys))
	for _, k := range aria2TaskKeys {
		out = append(out, k)
	}
	return out
}

func mapFromAny(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func sliceFromAny(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

func aria2Int(m map[string]any, key string) int64 {
	switch n := m[key].(type) {
	case float64:
		return int64(n)
	case string:
		var out int64
		fmt.Sscanf(n, "%d", &out)
		return out
	}
	return 0
}

func aria2Str(m map[string]any, key string) string {
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

// aria2GuessName 猜测任务名（BT info → 文件路径 → URI）。
func aria2GuessName(task map[string]any) string {
	info := mapFromAny(mapFromAny(task["bittorrent"])["info"])
	if name := aria2Str(info, "name"); name != "" {
		return name
	}
	files := sliceFromAny(task["files"])
	var firstFile map[string]any
	if len(files) > 0 {
		firstFile = mapFromAny(files[0])
	}
	if p := aria2Str(firstFile, "path"); p != "" {
		taskDir := aria2Str(task, "dir")
		if taskDir != "" && strings.HasPrefix(p, taskDir) {
			rel := strings.TrimPrefix(p, taskDir)
			rel = strings.TrimPrefix(rel, "/")
			if seg := strings.SplitN(rel, "/", 2)[0]; seg != "" && seg != "." && seg != ".." {
				return seg
			}
		}
		return path.Base(p)
	}
	for _, u := range sliceFromAny(firstFile["uris"]) {
		uri := aria2Str(mapFromAny(u), "uri")
		if uri == "" {
			continue
		}
		if parsed, err := url.Parse(uri); err == nil {
			if guess := path.Base(parsed.Path); guess != "" && guess != "/" {
				if decoded, err := url.QueryUnescape(guess); err == nil {
					return decoded
				}
				return guess
			}
			return parsed.Host
		}
		return uri
	}
	if name := aria2Str(info, "name"); name != "" {
		return name
	}
	if gid := aria2Str(task, "gid"); gid != "" {
		return gid
	}
	return "Unnamed Task"
}

// aria2NormalizeTask 任务标准化（前端字段）。
func aria2NormalizeTask(task map[string]any) map[string]any {
	firstFile := map[string]any{}
	if files := sliceFromAny(task["files"]); len(files) > 0 {
		firstFile = mapFromAny(files[0])
	}
	bittorrent := mapFromAny(task["bittorrent"])
	info := mapFromAny(bittorrent["info"])

	var uriList []string
	for _, u := range sliceFromAny(firstFile["uris"]) {
		if uri := aria2Str(mapFromAny(u), "uri"); uri != "" {
			uriList = append(uriList, uri)
		}
	}
	var filesOut []map[string]any
	for _, f := range sliceFromAny(task["files"]) {
		fm := mapFromAny(f)
		var fUris []string
		for _, u := range sliceFromAny(fm["uris"]) {
			if uri := aria2Str(mapFromAny(u), "uri"); uri != "" {
				fUris = append(fUris, uri)
			}
		}
		filesOut = append(filesOut, map[string]any{
			"path": aria2Str(fm, "path"), "length": aria2Int(fm, "length"),
			"completedLength": aria2Int(fm, "completedLength"),
			"selected":        aria2Str(fm, "selected") != "false", "uris": fUris,
		})
	}
	total := aria2Int(task, "totalLength")
	completed := aria2Int(task, "completedLength")
	progress := 0.0
	if total > 0 {
		progress = float64(int64(float64(completed)*100/float64(total)*10+0.5)) / 10
	}
	return map[string]any{
		"gid": task["gid"], "status": task["status"],
		"name": aria2GuessName(task), "dir": aria2Str(task, "dir"),
		"progress":    progress,
		"totalLength": total, "completedLength": completed,
		"uploadLength":  aria2Int(task, "uploadLength"),
		"downloadSpeed": aria2Int(task, "downloadSpeed"),
		"uploadSpeed":   aria2Int(task, "uploadSpeed"),
		"connections":   aria2Int(task, "connections"),
		"numSeeders":    aria2Int(task, "numSeeders"),
		"seeder":        aria2Str(task, "seeder") == "true",
		"errorCode":     firstNonEmptyStr(aria2Str(task, "errorCode"), ""),
		"errorMessage":  firstNonEmptyStr(aria2Str(task, "errorMessage"), ""),
		"fileCount":     len(filesOut),
		"uris":          uriList,
		"bittorrent": map[string]any{
			"name": aria2Str(info, "name"), "comment": aria2Str(bittorrent, "comment"),
			"mode": aria2Str(bittorrent, "mode"),
		},
		"files": filesOut,
	}
}

// aria2PaginationPayload 分页信息。
func aria2PaginationPayload(page, pageSize, total int) map[string]any {
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}
	return map[string]any{
		"page": page, "page_size": pageSize, "total": total,
		"total_pages": totalPages, "has_prev": page > 1, "has_next": page < totalPages,
	}
}

// aria2NormalizeStopped 清理完成的 METADATA/torrent 任务。
func aria2NormalizeStopped(tasks []any) []map[string]any {
	var normalized []map[string]any
	var metadataGIDs []string
	for _, t := range tasks {
		task := aria2NormalizeTask(mapFromAny(t))
		name, _ := task["name"].(string)
		status, _ := task["status"].(string)
		gid, _ := task["gid"].(string)
		if status == "complete" && (strings.Contains(name, "[METADATA]") || aria2TorrentNameRe.MatchString(name)) {
			metadataGIDs = append(metadataGIDs, gid)
			continue
		}
		normalized = append(normalized, task)
	}
	for _, gid := range metadataGIDs {
		_, _ = aria2RPCCall("removeDownloadResult", []any{gid})
	}
	if len(metadataGIDs) > 0 {
		log.Printf("自动清理了 %d 个已完成的 [METADATA] 或 .torrent 任务", len(metadataGIDs))
	}
	return normalized
}

// aria2FetchWaitingSlice tellWaiting 分页（aria2 倒序 → 转正序）。
func aria2FetchWaitingSlice(start, count, total int) []map[string]any {
	if count <= 0 || start >= total {
		return nil
	}
	rawOffset := total - (start + count)
	if rawOffset < 0 {
		rawOffset = 0
	}
	rawLimit := count
	if total-start < rawLimit {
		rawLimit = total - start
	}
	result, err := aria2RPCCall("tellWaiting", []any{rawOffset, rawLimit, aria2Keys()})
	if err != nil {
		return nil
	}
	tasks := sliceFromAny(result)
	out := make([]map[string]any, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, aria2NormalizeTask(mapFromAny(t)))
	}
	// 反转为正序
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func aria2FetchStoppedSlice(start, count int) []map[string]any {
	if count <= 0 {
		return nil
	}
	result, err := aria2RPCCall("tellStopped", []any{start, count, aria2Keys()})
	if err != nil {
		return nil
	}
	return aria2NormalizeStopped(sliceFromAny(result))
}

// aria2TaskMatches 任务搜索过滤。
func aria2TaskMatches(task map[string]any, search string) bool {
	if search == "" {
		return true
	}
	var haystacks []string
	for _, key := range []string{"gid", "name", "dir", "errorMessage"} {
		if v, ok := task[key].(string); ok && v != "" {
			haystacks = append(haystacks, v)
		}
	}
	for _, f := range sliceFromAny(task["files"]) {
		fm := mapFromAny(f)
		haystacks = append(haystacks, aria2Str(fm, "path"))
		for _, uri := range sliceFromAny(fm["uris"]) {
			haystacks = append(haystacks, aria2Str(mapFromAny(uri), "uri"))
		}
	}
	for _, h := range haystacks {
		if strings.Contains(strings.ToLower(h), search) {
			return true
		}
	}
	return false
}

// aria2Summary 全局统计 + 版本。
func aria2Summary(globalStat, version map[string]any, activeCount, waitingTotal, stoppedTotal int) map[string]any {
	return map[string]any{
		"activeCount": activeCount, "waitingCount": waitingTotal, "stoppedCount": stoppedTotal,
		"downloadSpeed": aria2Int(globalStat, "downloadSpeed"),
		"uploadSpeed":   aria2Int(globalStat, "uploadSpeed"),
		"numActive":     aria2Int(globalStat, "numActive"),
		"numWaiting":    waitingTotal, "numStopped": stoppedTotal,
	}
}

func aria2VersionPayload(version map[string]any) map[string]any {
	return map[string]any{
		"version":         firstNonEmptyStr(aria2Str(version, "version"), ""),
		"enabledFeatures": orEmptyList(version["enabledFeatures"]),
	}
}

func orEmptyList(v any) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	return []any{}
}

// fetchAria2QueueItems 队列分页列表（支持 active/waiting/stopped/all + 搜索）。
func fetchAria2QueueItems(queue string, page, pageSize int, search string) (map[string]any, int) {
	globalStatResult, err := aria2RPCCall("getGlobalStat", nil)
	if err != nil {
		return nil, http.StatusBadGateway
	}
	globalStat := mapFromAny(globalStatResult)
	versionResult, _ := aria2RPCCall("getVersion", nil)
	version := mapFromAny(versionResult)

	activeResult, _ := aria2RPCCall("tellActive", []any{aria2Keys()})
	activeTasks := make([]map[string]any, 0)
	for _, t := range sliceFromAny(activeResult) {
		activeTasks = append(activeTasks, aria2NormalizeTask(mapFromAny(t)))
	}
	// 反转为正序
	for i, j := 0, len(activeTasks)-1; i < j; i, j = i+1, j-1 {
		activeTasks[i], activeTasks[j] = activeTasks[j], activeTasks[i]
	}
	activeCount := len(activeTasks)
	waitingCount := int(aria2Int(globalStat, "numWaiting"))
	stoppedCount := int(aria2Int(globalStat, "numStopped"))

	switch queue {
	case "active", "waiting", "stopped":
	default:
		queue = "all"
	}
	search = strings.TrimSpace(strings.ToLower(search))

	if search != "" {
		waiting := aria2FetchWaitingSlice(0, waitingCount, waitingCount)
		stopped := aria2FetchStoppedSlice(0, stoppedCount)
		filter := func(list []map[string]any) []map[string]any {
			var out []map[string]any
			for _, t := range list {
				if aria2TaskMatches(t, search) {
					out = append(out, t)
				}
			}
			return out
		}
		filteredActive := filter(activeTasks)
		filteredWaiting := filter(waiting)
		filteredStopped := filter(stopped)
		var items []map[string]any
		switch queue {
		case "active":
			items = filteredActive
		case "waiting":
			items = filteredWaiting
		case "stopped":
			items = filteredStopped
		default:
			items = append(append(filteredActive, filteredWaiting...), filteredStopped...)
		}
		pagination := aria2PaginationPayload(page, pageSize, len(items))
		offset := (pagination["page"].(int) - 1) * pageSize
		end := offset + pageSize
		if end > len(items) {
			end = len(items)
		}
		var pageItems []map[string]any
		if offset < len(items) {
			pageItems = items[offset:end]
		}
		pagination["queue"] = queue
		pagination["search"] = search
		return map[string]any{
			"summary": aria2Summary(globalStat, version, len(filteredActive), waitingCount, stoppedCount),
			"items":   pageItems, "pagination": pagination,
			"version": aria2VersionPayload(version),
		}, 0
	}

	total := activeCount + waitingCount + stoppedCount
	switch queue {
	case "active":
		total = activeCount
	case "waiting":
		total = waitingCount
	case "stopped":
		total = stoppedCount
	}
	pagination := aria2PaginationPayload(page, pageSize, total)
	pageNum := pagination["page"].(int)
	offset := (pageNum - 1) * pageSize

	var items []map[string]any
	switch queue {
	case "active":
		end := offset + pageSize
		if end > len(activeTasks) {
			end = len(activeTasks)
		}
		if offset < len(activeTasks) {
			items = activeTasks[offset:end]
		}
	case "waiting":
		items = aria2FetchWaitingSlice(offset, pageSize, waitingCount)
	case "stopped":
		items = aria2FetchStoppedSlice(offset, pageSize)
	default:
		remaining := pageSize
		currentOffset := offset
		if currentOffset < activeCount && remaining > 0 {
			take := remaining
			if activeCount-currentOffset < take {
				take = activeCount - currentOffset
			}
			items = append(items, activeTasks[currentOffset:currentOffset+take]...)
			remaining -= take
			currentOffset = 0
		} else {
			currentOffset -= activeCount
			if currentOffset < 0 {
				currentOffset = 0
			}
		}
		if remaining > 0 {
			if currentOffset < waitingCount {
				take := remaining
				if waitingCount-currentOffset < take {
					take = waitingCount - currentOffset
				}
				items = append(items, aria2FetchWaitingSlice(currentOffset, take, waitingCount)...)
				remaining -= take
				currentOffset = 0
			} else {
				currentOffset -= waitingCount
				if currentOffset < 0 {
					currentOffset = 0
				}
			}
		}
		if remaining > 0 {
			items = append(items, aria2FetchStoppedSlice(currentOffset, remaining)...)
		}
	}
	pagination["queue"] = queue
	return map[string]any{
		"summary": aria2Summary(globalStat, version, activeCount, waitingCount, stoppedCount),
		"items":   items, "pagination": pagination,
		"version": aria2VersionPayload(version),
	}, 0
}

// aria2GetTask tellStatus + normalize。
func aria2GetTask(gid string) (map[string]any, error) {
	result, err := aria2RPCCall("tellStatus", []any{gid, aria2Keys()})
	if err != nil {
		return nil, err
	}
	return aria2NormalizeTask(mapFromAny(result)), nil
}

// ── HTTP 处理器 ─────────────────────────────────────────────

func handleAria2Overview(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	queue := q.Get("queue")
	if queue == "" {
		queue = "all"
	}
	payload, errStatus := fetchAria2QueueItems(queue, intQuery(r, "page", 1), intQuery(r, "page_size", 20), q.Get("search"))
	if payload == nil {
		writeJSONError(w, errStatus, "无法连接 aria2 RPC")
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func handleAria2OptionsGet(w http.ResponseWriter, _ *http.Request) {
	result, err := aria2RPCCall("getGlobalOption", nil)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	options := mapFromAny(result)
	payload := map[string]any{}
	for _, key := range aria2GlobalOptionKeys {
		if v, ok := options[key]; ok {
			payload[key] = v
		} else {
			payload[key] = ""
		}
	}
	writeJSON(w, http.StatusOK, payload)
}

func handleAria2OptionsPut(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := jsonDecodeBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	options := map[string]string{}
	keys := make([]string, 0, len(body))
	for k, v := range body {
		if v == nil {
			continue
		}
		keys = append(keys, k)
		options[k] = fmt.Sprintf("%v", v)
	}
	sort.Strings(keys)
	rpcOptions := map[string]any{}
	for k, v := range options {
		rpcOptions[k] = v
	}
	if _, err := aria2RPCCall("changeGlobalOption", []any{rpcOptions}); err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	appLog("download", "options_updated", "SUCCESS", "下载器全局配置已更新",
		map[string]any{"keys": keys})
	// 返回更新后的选项
	handleAria2OptionsGet(w, nil)
}

func handleAria2AddTorrent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Torrent string            `json:"torrent"`
		Uris    []string          `json:"uris"`
		Options map[string]string `json:"options"`
		Title   string            `json:"title"`
	}
	if err := jsonDecodeBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if body.Torrent == "" {
		writeJSONError(w, http.StatusBadRequest, "Torrent 内容不能为空")
		return
	}
	uris := body.Uris
	if uris == nil {
		uris = []string{}
	}
	options := body.Options
	if options == nil {
		options = map[string]string{}
	}
	result, err := aria2RPCCall("addTorrent", []any{body.Torrent, uris, options})
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	gid, _ := result.(string)
	task, _ := aria2GetTask(gid)
	message := "已添加种子下载任务"
	if body.Title != "" {
		message = "已推送下载：" + body.Title
	}
	appLog("download", "task_added_torrent", "SUCCESS", message,
		map[string]any{"gid": gid, "name": body.Title})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "task": task})
}

func aria2BatchAction(w http.ResponseWriter, r *http.Request, action string) {
	var body struct {
		Gids []string `json:"gids"`
	}
	if err := jsonDecodeBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	for _, gid := range body.Gids {
		var method string
		switch action {
		case "pause":
			method = "pause"
		case "unpause":
			method = "unpause"
		case "remove":
			method = "remove"
		default:
			method = action
		}
		if _, err := aria2RPCCall(method, []any{gid}); err != nil && action == "remove" {
			_ = err
		}
	}
	appLog("download", "tasks_"+action, "SUCCESS",
		map[string]string{"pause": "下载任务已暂停", "unpause": "下载任务已恢复",
			"remove": "下载任务已移除"}[action],
		map[string]any{"gids": body.Gids, "count": len(body.Gids)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func handleAria2TasksPause(w http.ResponseWriter, r *http.Request) {
	aria2BatchAction(w, r, "pause")
}

func handleAria2TasksUnpause(w http.ResponseWriter, r *http.Request) {
	aria2BatchAction(w, r, "unpause")
}

func handleAria2TasksRemove(w http.ResponseWriter, r *http.Request) {
	aria2BatchAction(w, r, "remove")
}

func handleAria2TasksRetry(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Gids []string `json:"gids"`
	}
	if err := jsonDecodeBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	var tasks []map[string]any
	var newGids []string
	for _, gid := range body.Gids {
		statusResult, err := aria2RPCCall("tellStatus", []any{gid, aria2Keys()})
		if err != nil {
			continue
		}
		raw := mapFromAny(statusResult)
		var uris []string
		for _, f := range sliceFromAny(raw["files"]) {
			for _, u := range sliceFromAny(mapFromAny(f)["uris"]) {
				uri := aria2Str(mapFromAny(u), "uri")
				if uri != "" && !containsStr(uris, uri) {
					uris = append(uris, uri)
				}
			}
		}
		if len(uris) == 0 {
			writeJSONError(w, http.StatusBadRequest, "该任务没有可重试的原始 URI")
			return
		}
		options := map[string]string{}
		if dir := aria2Str(raw, "dir"); dir != "" {
			options["dir"] = dir
		}
		newGid, err := aria2RPCCall("addUri", []any{uris, options})
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		gidStr, _ := newGid.(string)
		newGids = append(newGids, gidStr)
		if task, err := aria2GetTask(gidStr); err == nil {
			tasks = append(tasks, task)
		}
	}
	appLog("download", "tasks_retried", "SUCCESS", "下载任务已重试",
		map[string]any{"sourceGids": body.Gids, "newGids": newGids})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tasks": tasks})
}

func containsStr(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func handleAria2TasksPurge(w http.ResponseWriter, _ *http.Request) {
	if _, err := aria2RPCCall("purgeDownloadResult", nil); err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	appLog("download", "tasks_purged", "SUCCESS", "已清空已完成/已停止下载记录", nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func handleAria2AddURI(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Uris    []string          `json:"uris"`
		Options map[string]string `json:"options"`
		Title   string            `json:"title"`
	}
	if err := jsonDecodeBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	var cleaned []string
	for _, uri := range body.Uris {
		if uri = strings.TrimSpace(uri); uri != "" {
			cleaned = append(cleaned, uri)
		}
	}
	if len(cleaned) == 0 {
		writeJSONError(w, http.StatusBadRequest, "至少需要一个下载链接")
		return
	}
	options := body.Options
	if options == nil {
		options = map[string]string{}
	}
	result, err := aria2RPCCall("addUri", []any{cleaned, options})
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	gid, _ := result.(string)
	task, _ := aria2GetTask(gid)
	message := "已添加链接下载任务"
	if body.Title != "" {
		message = "已推送下载：" + body.Title
	}
	appLog("download", "task_added_uri", "SUCCESS", message,
		map[string]any{"gid": gid, "uriCount": len(cleaned), "name": body.Title})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "task": task})
}
