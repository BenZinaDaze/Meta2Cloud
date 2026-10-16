// configsvc.go —— 配置读写与解析测试服务（对应 webui/services/config.py + tmdb_service 序列化）。
// Google Drive 相关 payload 不再提供（支持已移除）。
package webui

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"meta2cloud/internal/jsonx"
	"meta2cloud/internal/mediaparser"
	"meta2cloud/internal/tmdb"
)

// ── 读取 ────────────────────────────────────────────────────

func readYAMLMap(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{}
	}
	out := map[string]any{}
	_ = yaml.Unmarshal(data, &out)
	return out
}

func extractParserRules(data map[string]any) map[string]any {
	parser := jsonx.Map(data, "parser")
	if parser == nil {
		return map[string]any{"custom_words": []any{}, "custom_release_groups": []any{}}
	}
	words, _ := parser["custom_words"].([]any)
	if words == nil {
		words = []any{}
	}
	groups, _ := parser["custom_release_groups"].([]any)
	if groups == nil {
		groups = []any{}
	}
	return map[string]any{"custom_words": words, "custom_release_groups": groups}
}

// readMergedConfigDict 主配置 + 解析规则合并。
func readMergedConfigDict() map[string]any {
	parsed := readYAMLMap(RT().configPath)
	rules := readYAMLMap(RT().rulesPath)
	if section, ok := rules["parser"].(map[string]any); ok {
		rules = section
	}
	parsed["parser"] = extractParserRules(map[string]any{"parser": rules})
	return parsed
}

// ── 解析测试 ────────────────────────────────────────────────

var (
	pathPrefixRe = regexp.MustCompile(`^([a-zA-Z]:[\\/]|\\\\|/|\./|\.\./)`)
	seasonPathRe = regexp.MustCompile(`(?i)[/\\](Season\s*\d+|S\d{2}|Specials|Extras|Featurettes|OVA)[/\\]?`)
)

// looksLikeFilePath 判断输入是文件路径还是纯文件名。
func looksLikeFilePath(text string) bool {
	if strings.Contains(text, " / ") || strings.Contains(text, " \\") || strings.Contains(text, "\\ ") {
		return false
	}
	if pathPrefixRe.MatchString(text) {
		return true
	}
	if strings.HasPrefix(text, "[") || strings.HasPrefix(text, "【") {
		return false
	}
	if seasonPathRe.MatchString(text) {
		return true
	}
	return strings.Contains(text, "/") || strings.Contains(text, "\\")
}

// serializeMeta MetaBase → 前端载荷（对应 tmdb_service.serialize_meta）。
func serializeMeta(meta *mediaparser.MetaBase) map[string]any {
	return map[string]any{
		"isfile":          meta.IsFile,
		"title":           meta.Title,
		"org_string":      meta.OrgString,
		"subtitle":        meta.Subtitle,
		"type":            meta.Type.ToAgent(),
		"type_label":      meta.Type.String(),
		"cn_name":         meta.CnName,
		"en_name":         meta.EnName,
		"name":            meta.Name(),
		"year":            meta.Year,
		"total_season":    meta.TotalSeason,
		"begin_season":    meta.BeginSeason,
		"end_season":      meta.EndSeason,
		"season":          meta.Season(),
		"sea":             meta.Sea(),
		"season_seq":      meta.SeasonSeq(),
		"season_list":     meta.SeasonList(),
		"total_episode":   meta.TotalEpisode,
		"begin_episode":   meta.BeginEpisode,
		"end_episode":     meta.EndEpisode,
		"episode":         meta.Episode(),
		"episode_list":    meta.EpisodeList(),
		"episodes":        meta.Episodes(),
		"episode_seqs":    meta.EpisodeSeqs(),
		"episode_seq":     meta.EpisodeSeq(),
		"season_episode":  meta.SeasonEpisode(),
		"part":            meta.Part,
		"resource_type":   meta.ResourceType,
		"resource_effect": meta.ResourceEffect,
		"resource_pix":    meta.ResourcePix,
		"resource_term":   meta.ResourceTerm(),
		"edition":         meta.Edition(),
		"resource_team":   meta.ResourceTeam,
		"release_group":   meta.ReleaseGroup(),
		"video_encode":    meta.VideoEncode,
		"video_term":      meta.VideoTerm(),
		"audio_encode":    meta.AudioEncode,
		"audio_term":      meta.AudioTerm(),
		"web_source":      meta.WebSource,
		"customization":   meta.Customization,
		"tmdbid":          meta.TmdbID,
		"doubanid":        meta.DoubanID,
		"frame_rate":      meta.FrameRate(),
	}
}

// extractAlternativeNamesWithLang 提取带语言的别名列表。
func extractAlternativeNamesWithLang(info map[string]any) []map[string]string {
	seen := map[string]bool{}
	result := []map[string]string{}
	altRoot := jsonx.Map(info, "alternative_titles")
	altTitles := jsonx.MapSlice(altRoot, "titles")
	if altTitles == nil {
		altTitles = jsonx.MapSlice(altRoot, "results")
	}
	langMap := map[string]string{"CN": "zh", "TW": "zh", "HK": "zh", "SG": "zh", "JP": "ja", "US": "en", "GB": "en"}
	for _, title := range altTitles {
		name := jsonx.Str(title, "title")
		iso3166 := strings.ToUpper(jsonx.Str(title, "iso_3166_1"))
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
	for _, tr := range jsonx.MapSlice(jsonx.Map(info, "translations"), "translations") {
		iso639 := jsonx.Str(tr, "iso_639_1")
		name := jsonx.Str(jsonx.Map(tr, "data"), "title")
		if name == "" {
			name = jsonx.Str(jsonx.Map(tr, "data"), "name")
		}
		if name != "" && !seen[name] {
			seen[name] = true
			result = append(result, map[string]string{"name": name, "iso_639_1": iso639})
		}
	}
	return result
}

// serializeTmdbResult TMDB 详情 → 前端载荷。
func serializeTmdbResult(info map[string]any) map[string]any {
	if info == nil {
		return nil
	}
	mediaTypeStr := jsonx.Str(info, "media_type")
	agent := mediaTypeStr
	switch mediaTypeStr {
	case "电影":
		agent = "movie"
	case "电视剧":
		agent = "tv"
	}
	releaseDate := firstNonEmptyStr(jsonx.Str(info, "release_date"), jsonx.Str(info, "first_air_date"))
	year := ""
	if len(releaseDate) >= 4 {
		year = releaseDate[:4]
	}
	cfg := RT().GetConfig()
	baseURL := cfg.TmdbImageBaseURL()

	genres := []string{}
	for _, g := range jsonx.MapSlice(info, "genres") {
		if n := jsonx.Str(g, "name"); n != "" {
			genres = append(genres, n)
		}
	}
	directors := []string{}
	for _, p := range peopleListFrom(info["directors"]) {
		if n := jsonx.Str(p, "name"); n != "" {
			directors = append(directors, n)
		}
	}
	actors := []string{}
	for i, p := range peopleListFrom(info["actors"]) {
		if i >= 8 {
			break
		}
		if n := jsonx.Str(p, "name"); n != "" {
			actors = append(actors, n)
		}
	}
	var names any = []string{}
	if n, ok := info["names"].([]any); ok {
		names = n
	}
	return map[string]any{
		"tmdb_id":           jsonx.Int(info, "tmdb_id", jsonx.Int(info, "id", 0)),
		"media_type":        agent,
		"media_type_label":  mediaTypeStr,
		"title":             firstNonEmptyStr(jsonx.Str(info, "title"), jsonx.Str(info, "name")),
		"original_title":    firstNonEmptyStr(jsonx.Str(info, "original_title"), jsonx.Str(info, "original_name")),
		"year":              year,
		"release_date":      releaseDate,
		"overview":          jsonx.Str(info, "overview"),
		"rating":            roundTo1F(jsonx.Float(info, "vote_average", 0)),
		"poster_url":        tmdb.ImageURL(jsonx.Str(info, "poster_path"), "w500", baseURL),
		"backdrop_url":      tmdb.ImageURL(jsonx.Str(info, "backdrop_path"), "", baseURL),
		"status":            jsonx.Str(info, "status"),
		"genres":            genres,
		"names":             names,
		"alternative_names": extractAlternativeNamesWithLang(info),
		"directors":         directors,
		"actors":            actors,
		"season_count":      info["number_of_seasons"],
		"episode_count":     info["number_of_episodes"],
	}
}

func peopleListFrom(v any) []map[string]any {
	if items, ok := v.([]map[string]any); ok {
		return items
	}
	if items, ok := v.([]any); ok {
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if d, ok := item.(map[string]any); ok {
				out = append(out, d)
			}
		}
		return out
	}
	return nil
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func roundTo1F(v float64) float64 {
	return float64(int64(v*10+0.5)) / 10
}

// parserTestPayload 解析测试。
func parserTestPayload(filename string, skipTmdb bool) (map[string]any, string, int) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return nil, "文件名不能为空", http.StatusBadRequest
	}
	cfg := RT().GetConfig()
	matcher := mediaparser.NewReleaseGroupsMatcher(cfg.Parser.CustomReleaseGroups)

	var meta *mediaparser.MetaBase
	if looksLikeFilePath(filename) {
		meta = mediaparser.MetaInfoPath(filename, cfg.Parser.CustomWords, matcher)
	} else {
		meta = mediaparser.MetaInfo(filename, "", true, cfg.Parser.CustomWords, matcher)
	}
	payload := serializeMeta(meta)

	if skipTmdb {
		payload["tmdb"] = nil
		return payload, "", 0
	}
	var tmdbPayload map[string]any
	if client := RT().GetTmdbClient(); client != nil {
		if found := client.Recognize(meta); found != nil {
			tmdbPayload = serializeTmdbResult(found)
			if id := jsonx.Int(found, "tmdb_id", jsonx.Int(found, "id", 0)); id != 0 && meta.TmdbID == 0 {
				payload["tmdbid"] = id
			}
			if y := jsonx.Str(tmdbPayload, "year"); y != "" && meta.Year == "" {
				payload["year"] = y
			}
		}
	}
	payload["tmdb"] = tmdbPayload
	appLog("parser", "parse_test", "INFO", "执行了解析测试", map[string]any{
		"filename": filename, "name": payload["name"],
		"type": payload["type"], "tmdbMatched": tmdbPayload != nil,
	})
	return payload, "", 0
}

// ── 校验与写入（对应 runtime._validate_main_config_payload） ──

func validateIntField(section map[string]any, key, label string, minimum, maximum int64) string {
	v, ok := section[key]
	if !ok || v == nil || v == "" {
		return ""
	}
	var n int64
	switch t := v.(type) {
	case int:
		n = int64(t)
	case int64:
		n = t
	case float64:
		n = int64(t)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		if err != nil {
			return label + " 必须是整数"
		}
		n = parsed
	default:
		return label + " 必须是整数"
	}
	if n < minimum || n > maximum {
		return fmt.Sprintf("%s 必须在 %d 到 %d 之间", label, minimum, maximum)
	}
	section[key] = n
	return ""
}

func validateBoolField(payload map[string]any, key string) string {
	pipeline, _ := payload["pipeline"].(map[string]any)
	if pipeline == nil {
		return ""
	}
	v, ok := pipeline[key]
	if !ok {
		return ""
	}
	switch t := v.(type) {
	case bool:
	case float64:
		if t != 0 && t != 1 {
			return "pipeline." + key + " 无法解析为布尔值"
		}
		pipeline[key] = t == 1
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		switch s {
		case "true", "yes", "1":
			pipeline[key] = true
		case "false", "no", "0":
			pipeline[key] = false
		default:
			return "pipeline." + key + " 无法解析为布尔值"
		}
	default:
		return "pipeline." + key + " 必须是布尔值"
	}
	payload["pipeline"] = pipeline
	return ""
}

// validateMainConfigPayload 主配置规范化校验，返回错误信息（空串为通过）。
func validateMainConfigPayload(payload map[string]any) string {
	getSection := func(name string) map[string]any {
		section, _ := payload[name].(map[string]any)
		if section == nil {
			section = map[string]any{}
			payload[name] = section
		}
		return section
	}
	webuiSec := getSection("webui")
	tmdbSec := getSection("tmdb")
	u115Sec := getSection("u115")
	rssSec := getSection("rss")
	telegramSec := getSection("telegram")
	mediavaultSec := getSection("mediavault")

	for _, check := range []struct {
		section map[string]any
		key     string
		label   string
		min     int64
		max     int64
	}{
		{webuiSec, "token_expire_hours", "Token 有效期", 1, 8760},
		{webuiSec, "log_retention_days", "日志保留天数", 1, 365},
		{tmdbSec, "timeout", "TMDB 请求超时", 1, 120},
		{u115Sec, "auto_organize_poll_seconds", "115 自动整理轮询间隔", 10, 600},
		{u115Sec, "auto_organize_stable_seconds", "115 完成稳定等待", 0, 600},
		{rssSec, "poll_seconds", "RSS 订阅轮询间隔", 10, 3600},
		{telegramSec, "debounce_seconds", "Telegram 防抖延时", 0, 600},
		{mediavaultSec, "timeout", "MediaVault 请求超时", 1, 120},
	} {
		if msg := validateIntField(check.section, check.key, check.label, check.min, check.max); msg != "" {
			return msg
		}
	}
	// mediavault host/scheme 规范化
	if host, ok := mediavaultSec["host"].(string); ok {
		mediavaultSec["host"] = strings.TrimRight(hostPrefixRe.ReplaceAllString(host, ""), "/")
	}
	for _, key := range []string{"skip_tmdb", "move_on_tmdb_miss", "dry_run", "replace_existing_video", "skip_metadata_upload"} {
		if msg := validateBoolField(payload, key); msg != "" {
			return msg
		}
	}
	return ""
}

var hostPrefixRe = regexp.MustCompile(`^https?://`)

func writeYAMLMap(path string, data map[string]any) error {
	out, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

// invalidateRuntimeCache 保存配置后清空运行时缓存。
func invalidateRuntimeCache() {
	rt := RT()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.cfg = nil
	rt.provider = nil
	rt.u115Client = nil
	rt.tmdbClient = nil
	rt.configSig = [2]int64{0, 0}
}

// ── HTTP 处理器 ─────────────────────────────────────────────

func requireConfigFile(w http.ResponseWriter) bool {
	if _, err := os.Stat(RT().configPath); err != nil {
		writeJSONError(w, http.StatusNotFound, "config.yaml 不存在")
		return false
	}
	return true
}

func handleReadConfig(w http.ResponseWriter, _ *http.Request) {
	if !requireConfigFile(w) {
		return
	}
	writeJSON(w, http.StatusOK, readMergedConfigDict())
}

func handleReadMainConfig(w http.ResponseWriter, _ *http.Request) {
	if !requireConfigFile(w) {
		return
	}
	writeJSON(w, http.StatusOK, readYAMLMap(RT().configPath))
}

func handleReadParserRules(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, extractParserRules(map[string]any{"parser": readYAMLMap(RT().rulesPath)}))
}

func handleParserTest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Filename string `json:"filename"`
	}
	_ = jsonDecodeBody(r, &body)
	skipTmdb := r.URL.Query().Get("skip_tmdb") == "true"
	payload, errMsg, status := parserTestPayload(body.Filename, skipTmdb)
	if errMsg != "" {
		writeJSONError(w, status, errMsg)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func handleWriteConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := jsonDecodeBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	payload := body.Data
	if payload == nil {
		payload = map[string]any{}
	}
	if msg := validateMainConfigPayload(payload); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	parserRules := extractParserRules(payload)
	delete(payload, "parser")
	if err := writeYAMLMap(RT().configPath, payload); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := writeYAMLMap(RT().rulesPath, parserRules); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	invalidateRuntimeCache()
	appLog("system", "config_updated", "SUCCESS", "配置文件已更新", nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "配置已保存"})
}

func handleWriteMainConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := jsonDecodeBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	payload := body.Data
	if payload == nil {
		payload = map[string]any{}
	}
	if msg := validateMainConfigPayload(payload); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	delete(payload, "parser")
	if err := writeYAMLMap(RT().configPath, payload); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	invalidateRuntimeCache()
	appLog("system", "main_config_updated", "SUCCESS", "主配置文件已更新", nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "主配置已保存"})
}

func handleWriteParserRules(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := jsonDecodeBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	rules := extractParserRules(map[string]any{"parser": body.Data})
	if err := writeYAMLMap(RT().rulesPath, rules); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	invalidateRuntimeCache()
	appLog("system", "parser_rules_updated", "SUCCESS", "文件名识别规则已更新", nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "识别规则已保存"})
}
