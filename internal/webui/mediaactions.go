// mediaactions.go —— 媒体元数据操作服务（对应 webui/services/media_actions.py）。
package webui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"meta2cloud/internal/cloudfile"
	"meta2cloud/internal/jsonx"
	"meta2cloud/internal/mediaparser"
	"meta2cloud/internal/scraper"
	"meta2cloud/internal/tmdb"
)

var seasonNameRe = regexp.MustCompile(`(?i)Season\s*\d+`)
var seasonNumRe = regexp.MustCompile(`\d+`)

func movieFolderName(title, year string) string {
	t := strings.TrimSpace(title)
	y := strings.TrimSpace(year)
	if t != "" && y != "" {
		return fmt.Sprintf("%s (%s)", t, y)
	}
	return t
}

// fetchTmdbInfoWithCredits 拉取带 credits/ids 的详情（回退基础请求 + 单独 credits）。
func fetchTmdbInfoWithCredits(client *tmdb.Client, path, extraKeys string) map[string]any {
	info := client.Get(path + "?append_to_response=" + extraKeys)
	if info == nil {
		info = client.Get(path)
	}
	if info == nil {
		return nil
	}
	if info["credits"] == nil {
		if credits := client.Get(path + "/credits"); credits != nil {
			info["credits"] = credits
		}
	}
	return info
}

// fetchTmdbInfoForRefresh 拉取刷新用的详情（含导演/演员提取）。
func fetchTmdbInfoForRefresh(tmdbID int64, mediaType string) (map[string]any, error) {
	client := RT().GetTmdbClient()
	if client == nil {
		return nil, fmt.Errorf("TMDB 客户端不可用")
	}
	switch mediaType {
	case "movie":
		info := fetchTmdbInfoWithCredits(client, fmt.Sprintf("/movie/%d", tmdbID),
			"credits,external_ids,release_dates")
		if info == nil {
			return nil, fmt.Errorf("TMDB 未找到电影 tmdb_id=%d，请稍候重试", tmdbID)
		}
		info["tmdb_id"] = float64(tmdbID)
		info["directors"] = extractCrew(info, "Director", 0)
		info["actors"] = extractCast(info, 20)
		return info, nil
	case "tv":
		info := fetchTmdbInfoWithCredits(client, fmt.Sprintf("/tv/%d", tmdbID),
			"credits,external_ids,content_ratings")
		if info == nil {
			return nil, fmt.Errorf("TMDB 未找到剧集 tmdb_id=%d，请稍候重试", tmdbID)
		}
		info["tmdb_id"] = float64(tmdbID)
		info["directors"] = extractCrew(info, "Director", 5)
		// TV 导演含 Executive Producer（Python 版 job in ("Director", "Executive Producer") 取前 5）
		execs := extractCrew(info, "Executive Producer", 5)
		info["directors"] = append(info["directors"].([]map[string]any), execs...)
		if len(info["directors"].([]map[string]any)) > 5 {
			info["directors"] = info["directors"].([]map[string]any)[:5]
		}
		info["actors"] = extractCast(info, 20)
		return info, nil
	}
	return nil, fmt.Errorf("不支持的 media_type: %s", mediaType)
}

func extractCrew(info map[string]any, job string, limit int) []map[string]any {
	out := []map[string]any{}
	for _, p := range jsonx.MapSlice(jsonx.Map(info, "credits"), "crew") {
		if jsonx.Str(p, "job") == job {
			out = append(out, map[string]any{
				"id": p["id"], "name": p["name"], "profile_path": p["profile_path"],
			})
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out
}

func extractCast(info map[string]any, limit int) []map[string]any {
	out := []map[string]any{}
	for i, p := range jsonx.MapSlice(jsonx.Map(info, "credits"), "cast") {
		if i >= limit {
			break
		}
		out = append(out, map[string]any{
			"id": p["id"], "name": p["name"], "character": p["character"], "profile_path": p["profile_path"],
		})
	}
	return out
}

// collectDriveEpisodes 扫描目录下的 Season 文件夹收集已入库集。
func collectDriveEpisodes(client cloudfile.Provider, driveFolderID string) map[epKey]bool {
	episodes := map[epKey]bool{}
	files, err := client.ListFiles(driveFolderID, 200)
	if err != nil {
		return episodes
	}
	for _, seasonFolder := range files {
		if !seasonFolder.IsFolder() || !seasonNameRe.MatchString(seasonFolder.Name) {
			continue
		}
		m := seasonNumRe.FindString(seasonFolder.Name)
		if m == "" {
			continue
		}
		seasonNum, _ := strconv.Atoi(m)
		seasonFiles, err := client.ListFiles(seasonFolder.ID, 500)
		if err != nil {
			continue
		}
		for _, file := range seasonFiles {
			if file.IsVideo() {
				if s, e, ok := ParseEpisodeFromFilename(file.Name); ok && s == seasonNum {
					episodes[epKey{s, e}] = true
				}
			}
		}
	}
	return episodes
}

// buildItemUpdates 根据新 TMDB 信息构造库条目更新。
func buildItemUpdates(info map[string]any, mediaType, driveFolderID string, client cloudfile.Provider) map[string]any {
	tmdbID := jsonx.Int(info, "tmdb_id", jsonx.Int(info, "id", 0))
	titleKey, origKey := "title", "original_title"
	if mediaType == "tv" {
		titleKey, origKey = "name", "original_name"
	}
	updates := map[string]any{
		"tmdb_id": tmdbID, "media_type": mediaType,
		"title": jsonx.Str(info, titleKey), "original_title": jsonx.Str(info, origKey),
		"overview": jsonx.Str(info, "overview"),
		"rating":   roundTo1F(jsonx.Float(info, "vote_average", 0)),
	}
	if p := jsonx.Str(info, "poster_path"); p != "" {
		updates["poster_url"] = tmdbImageURL(p, "w500")
	}
	if b := jsonx.Str(info, "backdrop_path"); b != "" {
		updates["backdrop_url"] = tmdbImageURL(b, "")
	}
	if mediaType == "tv" {
		date := jsonx.Str(info, "first_air_date")
		if len(date) >= 4 {
			updates["year"] = date[:4]
		}
		updates["status"] = jsonx.Str(info, "status")
		if n := info["number_of_episodes"]; n != nil {
			updates["total_episodes"] = n
		}
		if seasons := jsonx.MapSlice(info, "seasons"); len(seasons) > 0 {
			driveEpisodes := map[epKey]bool{}
			if client != nil {
				driveEpisodes = collectDriveEpisodes(client, driveFolderID)
			}
			seasonsStatus, totalEps, inLibEps := BuildSeasonsStatus(tmdbID, info, driveEpisodes, false)
			if len(seasonsStatus) > 0 {
				updates["seasons"] = seasonsToAny(seasonsStatus)
			}
			if totalEps > 0 {
				updates["total_episodes"] = totalEps
			}
			updates["in_library_episodes"] = inLibEps
		}
	} else {
		date := jsonx.Str(info, "release_date")
		if len(date) >= 4 {
			updates["year"] = date[:4]
		}
	}
	return updates
}

func seasonsToAny(seasons []SeasonStatus) []any {
	data, _ := json.Marshal(seasons)
	var out []any
	_ = json.Unmarshal(data, &out)
	return out
}

// doRefreshItem 刷新单条媒体元数据（拉取 TMDB 最新信息并构造库记录更新）。
func doRefreshItem(tmdbID int64, mediaType, driveFolderID, title, year string) (map[string]any, error) {
	cfg := RT().GetConfig()
	if tmdbID <= 0 {
		if title == "" {
			return nil, fmt.Errorf("该媒体项没有 TMDB ID 且未提供标题，无法搜索")
		}
		if !cfg.IsTmdbReady() {
			return nil, fmt.Errorf("TMDB API Key 未配置，无法搜索")
		}
		client := RT().GetTmdbClient()
		mtype := mediaparser.MediaMovie
		if mediaType == "tv" {
			mtype = mediaparser.MediaTV
		}
		found := client.SearchByName(title, year, mtype)
		if found == nil {
			return nil, fmt.Errorf("TMDB 搜索无结果：%q（%s），请先手动确认 TMDB ID", title, mediaType)
		}
		tmdbID = jsonx.Int(found, "tmdb_id", jsonx.Int(found, "id", 0))
	}

	info, err := fetchTmdbInfoForRefresh(tmdbID, mediaType)
	if err != nil {
		return nil, err
	}

	updates := buildItemUpdates(info, mediaType, driveFolderID, RT().GetStorageProvider())
	return map[string]any{
		"ok": true, "tmdb_id": tmdbID, "updates": updates,
	}, nil
}

// persistLibraryItem 更新库条目并返回最新数据。
func persistLibraryItem(driveFolderID string, result map[string]any) map[string]any {
	updates, _ := result["updates"].(map[string]any)
	if len(updates) == 0 {
		if id := jsonx.Int(result, "tmdb_id", 0); id > 0 {
			updates = map[string]any{"tmdb_id": id}
		}
	}
	if len(updates) == 0 || driveFolderID == "" {
		return nil
	}
	libStore := RT().GetLibraryStore()
	if !libStore.PatchItem(driveFolderID, updates) {
		return nil
	}
	return libStore.GetLibraryItemByFolderID(driveFolderID)
}

// renameLibraryFolder 按 TMDB 新标题重命名媒体目录。
func renameLibraryFolder(client cloudfile.Provider, driveFolderID, mediaType string, updates map[string]any) (map[string]any, error) {
	title, _ := updates["title"].(string)
	year, _ := updates["year"].(string)
	targetName := movieFolderName(title, year)
	if targetName == "" {
		return nil, fmt.Errorf("纠错后的媒体标题为空，无法重命名目录")
	}
	currentName := ""
	if current, err := client.GetFile(driveFolderID); err == nil {
		currentName = current.Name
		if currentName == targetName {
			return map[string]any{"renamed": false, "folder_name": currentName}, nil
		}
	}
	renamed, err := client.RenameFile(driveFolderID, targetName)
	if err != nil {
		return nil, err
	}
	renamedName := renamed.Name
	if renamedName == "" {
		renamedName = targetName
	}
	return map[string]any{"renamed": renamedName != currentName, "folder_name": renamedName}, nil
}

// reclassifyLibraryFolder 修正识别后按新 TMDB 信息重新归类：
// 解析分类目录链（媒体库根/一级/二级），目录不在新分类下时移动过去（目录 ID 不变）。
// 分类判定取 TMDB 最新详情（与修正识别弹窗展示的应属分类同标准）。
func reclassifyLibraryFolder(provider cloudfile.Provider, driveFolderID, mediaType string, tmdbID int64) (map[string]any, error) {
	categories := RT().GetCategories()
	rootRef := strings.TrimSpace(RT().GetConfig().ActiveRootFolderID())
	if categories == nil || rootRef == "" {
		return map[string]any{"moved": false}, nil
	}
	info, err := fetchTmdbInfoForClassification(tmdbID, mediaType)
	if err != nil {
		return nil, err
	}
	rootID, ok := cloudfile.ResolveFolderRefOrPath(provider, rootRef)
	if !ok {
		return nil, fmt.Errorf("媒体库根目录 %q 不存在", rootRef)
	}
	targetParent, category, err := classifyTargetParent(provider, categories, rootID, mediaType == "tv", info)
	if err != nil {
		return nil, err
	}
	current, err := provider.GetFile(driveFolderID)
	if err != nil {
		return nil, err
	}
	if current.ParentID == targetParent {
		return map[string]any{"moved": false, "category": category}, nil
	}
	if _, err := provider.MoveFile(driveFolderID, targetParent, ""); err != nil {
		return nil, err
	}
	if ls := RT().GetLibraryStore(); ls != nil {
		ls.UpdateMediaParentID(driveFolderID, targetParent)
	}
	return map[string]any{"moved": true, "category": category}, nil
}

// ── HTTP 处理器 ─────────────────────────────────────────────

type refreshItemBody struct {
	TmdbID        int64  `json:"tmdb_id"`
	MediaType     string `json:"media_type"`
	DriveFolderID string `json:"drive_folder_id"`
	Title         string `json:"title"`
	Year          string `json:"year"`
	RenameFolder  bool   `json:"rename_folder"`
}

func handleRefreshItem(w http.ResponseWriter, r *http.Request) {
	var body refreshItemBody
	if err := jsonDecodeBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if body.DriveFolderID == "" {
		writeJSONError(w, http.StatusBadRequest, "drive_folder_id 不能为空")
		return
	}
	result, err := doRefreshItem(body.TmdbID, body.MediaType, body.DriveFolderID, body.Title, body.Year)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	appLog("library", "refresh_item", "SUCCESS",
		fmt.Sprintf("已刷新元数据：%s（TMDB 编号 %d）", body.MediaType, body.TmdbID), map[string]any{
			"tmdb_id": body.TmdbID, "media_type": body.MediaType,
			"drive_folder_id": body.DriveFolderID,
		})
	if item := persistLibraryItem(body.DriveFolderID, result); item != nil {
		result["item"] = item
	}
	writeJSON(w, http.StatusOK, result)
}

func handleReidentifyItem(w http.ResponseWriter, r *http.Request) {
	var body refreshItemBody
	if err := jsonDecodeBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if body.DriveFolderID == "" {
		writeJSONError(w, http.StatusBadRequest, "drive_folder_id 不能为空")
		return
	}
	if body.TmdbID <= 0 {
		writeJSONError(w, http.StatusBadRequest, "tmdb_id 无效")
		return
	}
	result, err := doRefreshItem(body.TmdbID, body.MediaType, body.DriveFolderID, body.Title, body.Year)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	if item := persistLibraryItem(body.DriveFolderID, result); item != nil {
		result["item"] = item
	}
	renameError := ""
	renamed := false
	folderName := ""
	if body.RenameFolder {
		updates, _ := result["updates"].(map[string]any)
		if renameResult, err := renameLibraryFolder(
			RT().GetStorageProvider(), body.DriveFolderID, body.MediaType, updates); err != nil {
			renameError = err.Error()
		} else {
			renamed, _ = renameResult["renamed"].(bool)
			folderName, _ = renameResult["folder_name"].(string)
		}
	}
	result["renamed"] = renamed
	if folderName != "" {
		result["folder_name"] = folderName
	}
	if renameError != "" {
		result["rename_errors"] = []string{renameError}
	}
	// 重新归类：按新 TMDB 流派解析分类目录链，目录不在新分类下时移动过去
	reclassified := false
	category := ""
	reclassifyError := ""
	if body.TmdbID > 0 {
		if r, err := reclassifyLibraryFolder(RT().GetStorageProvider(), body.DriveFolderID, body.MediaType, body.TmdbID); err != nil {
			reclassifyError = err.Error()
		} else {
			reclassified, _ = r["moved"].(bool)
			category, _ = r["category"].(string)
		}
	}
	result["reclassified"] = reclassified
	if category != "" {
		result["category"] = category
	}
	if reclassifyError != "" {
		result["reclassify_errors"] = []string{reclassifyError}
	}
	// 目录被移动或重命名都会改变本地 STRM 树的镜像路径，触发全量同步立即跟进
	if reclassified || renamed {
		TriggerStrmSync("reidentify")
	}
	partial := renameError != "" || reclassifyError != ""
	result["partial"] = partial
	result["ok"] = !partial
	writeJSON(w, http.StatusOK, result)
}

func handleCacheStats(w http.ResponseWriter, _ *http.Request) {
	cache := RT().GetTmdbCache()
	if cache == nil {
		writeJSON(w, http.StatusOK, map[string]int64{})
		return
	}
	writeJSON(w, http.StatusOK, cache.Stats())
}

func handleCacheEvict(w http.ResponseWriter, _ *http.Request) {
	cache := RT().GetTmdbCache()
	if cache == nil {
		writeJSON(w, http.StatusOK, map[string]any{"evicted": 0})
		return
	}
	n := cache.EvictExpired()
	writeJSON(w, http.StatusOK, map[string]any{
		"evicted": n, "message": fmt.Sprintf("已清理 %d 条过期缓存", n),
	})
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleTmdbSearch(w http.ResponseWriter, r *http.Request) {
	keyword := r.URL.Query().Get("keyword")
	cfg := RT().GetConfig()
	if cfg.Tmdb.APIKey == "" {
		writeJSONError(w, http.StatusBadRequest, "TMDB API Key 未配置")
		return
	}
	client := RT().GetTmdbClient()
	results := client.SearchRawMulti(keyword)
	serialized := []map[string]any{}
	for _, item := range results {
		if mt := jsonx.Str(item, "media_type"); mt != "movie" && mt != "tv" {
			continue
		}
		if s := serializeTmdbResult(item); s != nil {
			serialized = append(serialized, s)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "results": serialized})
}

func handleTmdbDetail(w http.ResponseWriter, r *http.Request) {
	tmdbID, err := strconv.ParseInt(r.PathValue("tmdbID"), 10, 64)
	if err != nil || tmdbID <= 0 {
		writeJSONError(w, http.StatusBadRequest, "tmdb_id 无效")
		return
	}
	mediaType := r.URL.Query().Get("media_type")
	if mediaType != "movie" && mediaType != "tv" {
		writeJSONError(w, http.StatusBadRequest, "media_type 必须是 movie 或 tv")
		return
	}
	libStore := RT().GetLibraryStore()
	joined := libStore.GetJoinedMediaItem(mediaType, tmdbID)
	if joined != nil && (mediaType != "tv" || joined["seasons"] != nil) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "detail": joined})
		return
	}
	client := RT().GetTmdbClient()
	if client == nil {
		writeJSONError(w, http.StatusInternalServerError, "TMDB API Key 未配置")
		return
	}
	var raw map[string]any
	if mediaType == "movie" {
		raw = client.Get(fmt.Sprintf("/movie/%d?append_to_response=%s", tmdbID,
			"credits,alternative_titles,translations,external_ids"))
	} else {
		raw = client.Get(fmt.Sprintf("/tv/%d?append_to_response=%s", tmdbID,
			"credits,alternative_titles,translations,external_ids"))
	}
	if raw == nil {
		writeJSONError(w, http.StatusInternalServerError, "TMDB 详情获取失败")
		return
	}
	serialized := serializeTmdbResult(raw)
	if mediaType == "tv" && raw["seasons"] != nil {
		seasonsStatus, _, _ := BuildSeasonsStatus(tmdbID, raw, map[epKey]bool{}, true)
		serialized["seasons"] = seasonsToAny(seasonsStatus)
	}
	serialized["in_library"] = false
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "detail": serialized})
}

func handleTmdbAlternativeNames(w http.ResponseWriter, r *http.Request) {
	tmdbID, err := strconv.ParseInt(r.PathValue("tmdbID"), 10, 64)
	if err != nil || tmdbID <= 0 {
		writeJSONError(w, http.StatusBadRequest, "tmdb_id 无效")
		return
	}
	mediaType := r.URL.Query().Get("media_type")
	client := RT().GetTmdbClient()
	if client == nil {
		writeJSONError(w, http.StatusInternalServerError, "TMDB API Key 未配置")
		return
	}
	mediaPath := "movie"
	if mediaType == "tv" {
		mediaPath = "tv"
	}
	data := client.Get(fmt.Sprintf("/%s/%d/alternative_titles", mediaPath, tmdbID))
	if data == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "alternative_names": []any{}})
		return
	}
	rawTitles := jsonx.MapSlice(data, "titles")
	if rawTitles == nil {
		rawTitles = jsonx.MapSlice(data, "results")
	}
	langMap := map[string]string{"CN": "zh", "TW": "zh", "HK": "zh", "SG": "zh", "JP": "ja", "US": "en", "GB": "en"}
	seen := map[string]bool{}
	result := []map[string]string{}
	for _, item := range rawTitles {
		name := jsonx.Str(item, "title")
		iso3166 := strings.ToUpper(jsonx.Str(item, "iso_3166_1"))
		iso639 := strings.ToLower(iso3166)
		if len(iso639) > 2 {
			iso639 = iso639[:2]
		}
		if v, ok := langMap[iso3166]; ok {
			iso639 = v
		}
		if name != "" && !seen[name] {
			seen[name] = true
			result = append(result, map[string]string{"name": name, "iso_639_1": iso639})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "alternative_names": result})
}

func handleScraperSearch(w http.ResponseWriter, r *http.Request) {
	factory := scraper.DefaultFactory()
	results := factory.SearchAll(r.URL.Query().Get("keyword"))
	grouped := map[string][]map[string]any{}
	var order []string
	for _, result := range results {
		if _, ok := grouped[result.Name]; !ok {
			order = append(order, result.Name)
		}
		grouped[result.Name] = append(grouped[result.Name], map[string]any{
			"site": result.Site, "media_id": result.MediaID, "url": result.URL,
			"cover_image": result.CoverImage, "subgroup_id": result.SubgroupID,
			"subgroup_name": result.SubgroupName, "rss_url": result.RssURL,
		})
	}
	aggregate := []map[string]any{}
	for _, name := range order {
		sources := grouped[name]
		var cover any
		for _, source := range sources {
			if source["cover_image"] != nil && source["cover_image"] != "" {
				cover = source["cover_image"]
				break
			}
		}
		aggregate = append(aggregate, map[string]any{"name": name, "cover_image": cover, "sources": sources})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "results": aggregate})
}

func handleScraperSites(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sites": scraper.DefaultFactory().ListSites()})
}

func handleScraperEpisodes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	site := q.Get("site")
	mediaID := q.Get("media_id")
	subgroupID := q.Get("subgroup_id")
	spider, err := scraper.DefaultFactory().GetSpider(site)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	episodes, err := spider.GetEpisodes(mediaID, subgroupID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "episodes": []any{}, "error": err.Error()})
		return
	}
	data, _ := json.Marshal(episodes)
	out := []any{}
	_ = json.Unmarshal(data, &out)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "episodes": out})
}

// handleBangumiCalendar 代理 Bangumi 日历 API（解决移动端 CORS）。
func handleBangumiCalendar(w http.ResponseWriter, _ *http.Request) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, "https://api.bgm.tv/calendar", nil)
	req.Header.Set("User-Agent", "Meta2Cloud/1.0 (https://github.com/BenZinaDaze/Meta2Cloud)")
	resp, err := client.Do(req)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "Bangumi 日历服务暂时不可用，请稍后重试")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		writeJSONError(w, http.StatusBadGateway,
			fmt.Sprintf("Bangumi 日历服务暂时不可用（上游返回 %d），请稍后重试", resp.StatusCode))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = io.Copy(w, resp.Body)
}
