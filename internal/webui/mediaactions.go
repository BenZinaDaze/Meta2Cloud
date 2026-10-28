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
func (s *Server) fetchTmdbInfoForRefresh(tmdbID int64, mediaType string) (map[string]any, error) {
	client := s.rt.GetTmdbClient()
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
				if season, e, ok := ParseEpisodeFromFilename(file.Name); ok && season == seasonNum {
					episodes[epKey{season, e}] = true
				}
			}
		}
	}
	return episodes
}

// buildItemUpdates 根据新 TMDB 信息构造库条目更新。
func (s *Server) buildItemUpdates(info map[string]any, mediaType, driveFolderID string, client cloudfile.Provider) map[string]any {
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
		updates["poster_url"] = s.rt.tmdbImageURL(p, "w500")
	}
	if b := jsonx.Str(info, "backdrop_path"); b != "" {
		updates["backdrop_url"] = s.rt.tmdbImageURL(b, "")
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
			seasonsStatus, totalEps, inLibEps := s.rt.BuildSeasonsStatus(tmdbID, info, driveEpisodes, false)
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
func (s *Server) doRefreshItem(tmdbID int64, mediaType, driveFolderID, title, year string) (map[string]any, error) {
	cfg := s.rt.GetConfig()
	if tmdbID <= 0 {
		if title == "" {
			return nil, fmt.Errorf("该媒体项没有 TMDB ID 且未提供标题，无法搜索")
		}
		if !cfg.IsTmdbReady() {
			return nil, fmt.Errorf("TMDB API Key 未配置，无法搜索")
		}
		client := s.rt.GetTmdbClient()
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

	info, err := s.fetchTmdbInfoForRefresh(tmdbID, mediaType)
	if err != nil {
		return nil, err
	}

	updates := s.buildItemUpdates(info, mediaType, driveFolderID, s.rt.GetStorageProvider())
	return map[string]any{
		"ok": true, "tmdb_id": tmdbID, "updates": updates,
	}, nil
}

// persistLibraryItem 更新库条目并返回最新数据。
func (s *Server) persistLibraryItem(driveFolderID string, result map[string]any) map[string]any {
	updates, _ := result["updates"].(map[string]any)
	if len(updates) == 0 {
		if id := jsonx.Int(result, "tmdb_id", 0); id > 0 {
			updates = map[string]any{"tmdb_id": id}
		}
	}
	if len(updates) == 0 || driveFolderID == "" {
		return nil
	}
	libStore := s.rt.GetLibraryStore()
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

var unsafeFilenameRe = regexp.MustCompile(`[\\/:*?"<>|]`)

func safeFilename(name string) string {
	return strings.TrimSpace(unsafeFilenameRe.ReplaceAllString(name, ""))
}

// renameLibraryFiles 修正识别后把目录内的媒体文件一并改成标准命名（与整理管线
// buildCleanNameFromMeta 同一套命名）：电影目录唯一视频 → 「标题 (年份).ext」；
// 剧集 Season 目录内视频 → 「标题 - SxxEyy.ext」。字幕文件随同名视频一起改名
// （保留语言标签后缀）。已符合标准命名的文件跳过（零网盘请求）。
func renameLibraryFiles(provider cloudfile.Provider, driveFolderID, mediaType string, updates map[string]any) (int, []string) {
	title, _ := updates["title"].(string)
	if strings.TrimSpace(title) == "" {
		return 0, nil
	}
	files, err := provider.ListFiles(driveFolderID, 500)
	if err != nil {
		return 0, []string{fmt.Sprintf("读取目录文件失败：%v", err)}
	}
	if mediaType == "tv" {
		return renameTvFiles(provider, files, title)
	}
	return renameMovieFiles(provider, files, title, updates)
}

// renameMovieFiles 电影目录：唯一视频改为「标题 (年份).ext」，字幕随视频基名同步。
func renameMovieFiles(provider cloudfile.Provider, files []cloudfile.CloudFile, title string, updates map[string]any) (int, []string) {
	year, _ := updates["year"].(string)
	var videos, siblings []cloudfile.CloudFile
	for _, f := range files {
		if f.IsVideo() {
			videos = append(videos, f)
		} else {
			siblings = append(siblings, f)
		}
	}
	if len(videos) == 0 {
		return 0, nil
	}
	if len(videos) > 1 {
		return 0, []string{fmt.Sprintf("目录内有 %d 个视频文件，无法确定命名，跳过文件重命名", len(videos))}
	}
	video := videos[0]
	newStem := movieFolderName(safeFilename(title), year)
	renamed, err := renameMediaFile(provider, video, newStem+video.Extension())
	if err != nil {
		return 0, []string{fmt.Sprintf("改名失败 %s：%v", video.Name, err)}
	}
	n := 0
	if renamed {
		n++
	}
	subN, subErrs := renameSiblingSubtitles(provider, siblings, fileStemOf(video.Name), newStem)
	return n + subN, subErrs
}

// renameTvFiles 剧集目录：各 Season 子目录内视频改为「标题 - SxxEyy.ext」，
// 季号须与所在 Season 目录一致（同 collectDriveEpisodes 的判定）。
func renameTvFiles(provider cloudfile.Provider, files []cloudfile.CloudFile, title string) (int, []string) {
	renamed := 0
	var errs []string
	for _, folder := range files {
		if !folder.IsFolder() || !seasonNameRe.MatchString(folder.Name) {
			continue
		}
		m := seasonNumRe.FindString(folder.Name)
		if m == "" {
			continue
		}
		seasonNum, _ := strconv.Atoi(m)
		seasonFiles, err := provider.ListFiles(folder.ID, 500)
		if err != nil {
			errs = append(errs, fmt.Sprintf("读取 %s 失败：%v", folder.Name, err))
			continue
		}
		var videos, siblings []cloudfile.CloudFile
		for _, file := range seasonFiles {
			if file.IsVideo() {
				videos = append(videos, file)
			} else {
				siblings = append(siblings, file)
			}
		}
		for _, file := range videos {
			season, e, ok := ParseEpisodeFromFilename(file.Name)
			if !ok || season != seasonNum {
				errs = append(errs, fmt.Sprintf("无法识别季集，跳过改名：%s", file.Name))
				continue
			}
			newStem := fmt.Sprintf("%s - S%02dE%02d", safeFilename(title), season, e)
			done, err := renameMediaFile(provider, file, newStem+file.Extension())
			if err != nil {
				errs = append(errs, fmt.Sprintf("改名失败 %s：%v", file.Name, err))
				continue
			}
			if done {
				renamed++
			}
			// 视频已就位（本轮改名或原本就是标准名）后字幕跟随；改名失败则字幕不动
			subN, subErrs := renameSiblingSubtitles(provider, siblings, fileStemOf(file.Name), newStem)
			renamed += subN
			errs = append(errs, subErrs...)
		}
	}
	return renamed, errs
}

// renameSiblingSubtitles 把与视频旧基名同前缀的字幕改成新基名（保留语言标签等后缀）。
func renameSiblingSubtitles(provider cloudfile.Provider, siblings []cloudfile.CloudFile, oldStem, newStem string) (int, []string) {
	if oldStem == "" || oldStem == newStem {
		return 0, nil
	}
	renamed := 0
	var errs []string
	for _, sub := range siblings {
		if !sub.IsSubtitle() || !strings.HasPrefix(sub.Name, oldStem+".") {
			continue
		}
		done, err := renameMediaFile(provider, sub, newStem+sub.Name[len(oldStem):])
		if err != nil {
			errs = append(errs, fmt.Sprintf("字幕改名失败 %s：%v", sub.Name, err))
			continue
		}
		if done {
			renamed++
		}
	}
	return renamed, errs
}

// renameMediaFile 目标名与当前名一致时跳过（零网盘请求），否则改名。
func renameMediaFile(provider cloudfile.Provider, file cloudfile.CloudFile, newName string) (bool, error) {
	if file.Name == newName {
		return false, nil
	}
	if _, err := provider.RenameFile(file.ID, newName); err != nil {
		return false, err
	}
	return true, nil
}

func fileStemOf(name string) string {
	if i := strings.LastIndex(name, "."); i > 0 {
		return name[:i]
	}
	return name
}

// rewriteIngestHistory 修正识别后把该目录的历史入库记录改写到新 TMDB 身份：
// 标题/原始标题/年份/海报/ID 全量覆盖；剧集行按新条目单集详情补正单集标题
// （详情走 TMDB 客户端缓存）。入库时间与原始文件名是历史事实，保留不动。
// 记录页与「入库历史 → TMDB」识别回退（resolveMediaIdentity）都随修正同步，
// 否则旧记录会把同名再次下载重新识别到错误条目上。
func (s *Server) rewriteIngestHistory(mediaType, driveFolderID string, tmdbID int64) (int, error) {
	ingest := s.rt.GetIngestStore()
	rows := ingest.ListByFolder(mediaType, driveFolderID)
	if len(rows) == 0 {
		return 0, nil
	}
	// 复用修正识别刚拉取的详情缓存（零额外请求）
	info, err := s.fetchTmdbInfoForRefresh(tmdbID, mediaType)
	if err != nil {
		return 0, err
	}
	titleKey, origKey, dateKey := "title", "original_title", "release_date"
	if mediaType == "tv" {
		titleKey, origKey, dateKey = "name", "original_name", "first_air_date"
	}
	title := jsonx.Str(info, titleKey)
	originalTitle := jsonx.Str(info, origKey)
	year := firstN4Str(jsonx.Str(info, dateKey))
	posterPath := jsonx.Str(info, "poster_path")
	client := s.rt.GetTmdbClient()
	for _, row := range rows {
		episodeTitle, _ := row["episode_title"].(string)
		if mediaType == "tv" && client != nil {
			if s, ok1 := row["season"].(int64); ok1 {
				if e, ok2 := row["episode"].(int64); ok2 && e > 0 {
					// 拉取失败保持原值（下轮修正识别可再补）
					if detail := client.GetEpisodeDetail(int(tmdbID), int(s), int(e)); detail != nil {
						episodeTitle = jsonx.Str(detail, "name")
					}
				}
			}
		}
		rowID, _ := row["id"].(int64)
		if err := ingest.UpdateIdentity(rowID, tmdbID, title, originalTitle, year, posterPath, episodeTitle); err != nil {
			return 0, err
		}
	}
	return len(rows), nil
}

// reclassifyLibraryFolder 修正识别后按新 TMDB 信息重新归类：
// 解析分类目录链（媒体库根/一级/二级），目录不在新分类下时移动过去（目录 ID 不变）。
// 分类判定取 TMDB 最新详情（与修正识别弹窗展示的应属分类同标准）。
func (s *Server) reclassifyLibraryFolder(provider cloudfile.Provider, driveFolderID, mediaType string, tmdbID int64) (map[string]any, error) {
	categories := s.rt.GetCategories()
	rootRef := strings.TrimSpace(s.rt.GetConfig().ActiveRootFolderID())
	if categories == nil || rootRef == "" {
		return map[string]any{"moved": false}, nil
	}
	info, err := s.fetchTmdbInfoForClassification(tmdbID, mediaType)
	if err != nil {
		return nil, err
	}
	rootID, ok := cloudfile.ResolveFolderRefOrPath(provider, rootRef)
	if !ok {
		return nil, fmt.Errorf("媒体库根目录 %q 不存在", rootRef)
	}
	targetParent, category, err := s.classifyTargetParent(provider, categories, rootID, mediaType == "tv", info)
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
	if ls := s.rt.GetLibraryStore(); ls != nil {
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

func (s *Server) handleRefreshItem(w http.ResponseWriter, r *http.Request) {
	var body refreshItemBody
	if err := jsonDecodeBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if body.DriveFolderID == "" {
		writeJSONError(w, http.StatusBadRequest, "drive_folder_id 不能为空")
		return
	}
	result, err := s.doRefreshItem(body.TmdbID, body.MediaType, body.DriveFolderID, body.Title, body.Year)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	s.rt.appLog("library", "refresh_item", "SUCCESS",
		fmt.Sprintf("已刷新元数据：%s（TMDB 编号 %d）", body.MediaType, body.TmdbID), map[string]any{
			"tmdb_id": body.TmdbID, "media_type": body.MediaType,
			"drive_folder_id": body.DriveFolderID,
		})
	if item := s.persistLibraryItem(body.DriveFolderID, result); item != nil {
		result["item"] = item
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleReidentifyItem(w http.ResponseWriter, r *http.Request) {
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
	result, err := s.doRefreshItem(body.TmdbID, body.MediaType, body.DriveFolderID, body.Title, body.Year)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	if item := s.persistLibraryItem(body.DriveFolderID, result); item != nil {
		result["item"] = item
	}
	// 入库历史改写到新身份（与库条目同级的一致性修正，不受重命名开关约束）
	historyErrors := []string{}
	if n, err := s.rewriteIngestHistory(body.MediaType, body.DriveFolderID, body.TmdbID); err != nil {
		historyErrors = append(historyErrors, "入库记录改写失败："+err.Error())
	} else if n > 0 {
		result["history_rewritten"] = n
	}
	if len(historyErrors) > 0 {
		result["errors"] = historyErrors
	}
	renameErrors := []string{}
	renamed := false
	folderName := ""
	filesRenamed := 0
	if body.RenameFolder {
		updates, _ := result["updates"].(map[string]any)
		if renameResult, err := renameLibraryFolder(
			s.rt.GetStorageProvider(), body.DriveFolderID, body.MediaType, updates); err != nil {
			renameErrors = append(renameErrors, err.Error())
		} else {
			renamed, _ = renameResult["renamed"].(bool)
			folderName, _ = renameResult["folder_name"].(string)
		}
		// 目录内媒体文件同步改成标准命名（跟随新识别的标题/季集）
		n, fileErrs := renameLibraryFiles(s.rt.GetStorageProvider(), body.DriveFolderID, body.MediaType, updates)
		filesRenamed = n
		renameErrors = append(renameErrors, fileErrs...)
	}
	result["renamed"] = renamed
	result["files_renamed"] = filesRenamed
	if folderName != "" {
		result["folder_name"] = folderName
	}
	if len(renameErrors) > 0 {
		result["rename_errors"] = renameErrors
	}
	// 重新归类：按新 TMDB 流派解析分类目录链，目录不在新分类下时移动过去
	reclassified := false
	category := ""
	reclassifyError := ""
	if body.TmdbID > 0 {
		if r, err := s.reclassifyLibraryFolder(s.rt.GetStorageProvider(), body.DriveFolderID, body.MediaType, body.TmdbID); err != nil {
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
	// 目录被移动、重命名或文件改名都会改变本地 STRM 树的镜像路径，触发全量同步立即跟进
	if reclassified || renamed || filesRenamed > 0 {
		s.rt.TriggerStrmSync("reidentify")
	}
	partial := len(renameErrors) > 0 || reclassifyError != "" || len(historyErrors) > 0
	result["partial"] = partial
	result["ok"] = !partial
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleCacheStats(w http.ResponseWriter, _ *http.Request) {
	cache := s.rt.GetTmdbCache()
	if cache == nil {
		writeJSON(w, http.StatusOK, map[string]int64{})
		return
	}
	writeJSON(w, http.StatusOK, cache.Stats())
}

func (s *Server) handleCacheEvict(w http.ResponseWriter, _ *http.Request) {
	cache := s.rt.GetTmdbCache()
	if cache == nil {
		writeJSON(w, http.StatusOK, map[string]any{"evicted": 0})
		return
	}
	n := cache.EvictExpired()
	writeJSON(w, http.StatusOK, map[string]any{
		"evicted": n, "message": fmt.Sprintf("已清理 %d 条过期缓存", n),
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleTmdbSearch(w http.ResponseWriter, r *http.Request) {
	keyword := r.URL.Query().Get("keyword")
	cfg := s.rt.GetConfig()
	if cfg.Tmdb.APIKey == "" {
		writeJSONError(w, http.StatusBadRequest, "TMDB API Key 未配置")
		return
	}
	client := s.rt.GetTmdbClient()
	results := client.SearchRawMulti(keyword)
	serialized := []map[string]any{}
	for _, item := range results {
		if mt := jsonx.Str(item, "media_type"); mt != "movie" && mt != "tv" {
			continue
		}
		if serializedItem := s.rt.serializeTmdbResult(item); serializedItem != nil {
			serialized = append(serialized, serializedItem)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "results": serialized})
}

func (s *Server) handleTmdbDetail(w http.ResponseWriter, r *http.Request) {
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
	libStore := s.rt.GetLibraryStore()
	joined := libStore.GetJoinedMediaItem(mediaType, tmdbID)
	if joined != nil && (mediaType != "tv" || joined["seasons"] != nil) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "detail": joined})
		return
	}
	client := s.rt.GetTmdbClient()
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
	serialized := s.rt.serializeTmdbResult(raw)
	if mediaType == "tv" && raw["seasons"] != nil {
		seasonsStatus, _, _ := s.rt.BuildSeasonsStatus(tmdbID, raw, map[epKey]bool{}, true)
		serialized["seasons"] = seasonsToAny(seasonsStatus)
	}
	serialized["in_library"] = false
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "detail": serialized})
}

func (s *Server) handleTmdbAlternativeNames(w http.ResponseWriter, r *http.Request) {
	tmdbID, err := strconv.ParseInt(r.PathValue("tmdbID"), 10, 64)
	if err != nil || tmdbID <= 0 {
		writeJSONError(w, http.StatusBadRequest, "tmdb_id 无效")
		return
	}
	mediaType := r.URL.Query().Get("media_type")
	client := s.rt.GetTmdbClient()
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

func (s *Server) handleScraperSearch(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) handleScraperSites(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sites": scraper.DefaultFactory().ListSites()})
}

func (s *Server) handleScraperEpisodes(w http.ResponseWriter, r *http.Request) {
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
func (s *Server) handleBangumiCalendar(w http.ResponseWriter, _ *http.Request) {
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
