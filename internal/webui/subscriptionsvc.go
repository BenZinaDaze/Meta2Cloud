// subscriptionsvc.go —— RSS 订阅服务（对应 webui/services/subscriptions.py）。
package webui

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"meta2cloud/internal/jsonx"
	"meta2cloud/internal/mediaparser"
	"meta2cloud/internal/scraper"
	"meta2cloud/internal/tmdb"
	"meta2cloud/internal/webui/store"
)

const defaultSubscriptionPollSeconds = 300

// getSubscriptionPollSeconds 轮询间隔（配置 rss.poll_seconds，下限 10）。
func getSubscriptionPollSeconds() int {
	raw := readMergedConfigDict()
	rss := jsonx.Map(raw, "rss")
	value := jsonx.Int(rss, "poll_seconds", 0)
	if value == 0 {
		return defaultSubscriptionPollSeconds
	}
	if value < 10 {
		return 10
	}
	return int(value)
}

func normalizeKeywords(keywordAll []string) []string {
	seen := map[string]bool{}
	var normalized []string
	for _, item := range keywordAll {
		value := strings.ToLower(strings.TrimSpace(item))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	return normalized
}

func hitToPayload(h *store.HitRecord) map[string]any {
	return map[string]any{
		"id": h.ID, "subscription_id": h.SubscriptionID,
		"episode_title": h.EpisodeTitle, "season_number": h.SeasonNumber,
		"episode_number": h.EpisodeNumber, "torrent_url": h.TorrentURL,
		"magnet_url": h.MagnetURL, "published_at": h.PublishedAt,
		"dedupe_key": h.DedupeKey, "pushed_to": h.PushedTo,
		"push_status": h.PushStatus, "push_error": h.PushError,
		"created_at": h.CreatedAt,
	}
}

// serializeSubscription 订阅记录 → 前端载荷（含 TMDB/库状态增强）。
func serializeSubscription(rssStore *store.RSSStore, record *store.SubscriptionRecord) map[string]any {
	payload := map[string]any{
		"id": record.ID, "name": record.Name, "media_title": record.MediaTitle,
		"media_type": record.MediaType, "tmdb_id": record.TmdbID,
		"poster_url": tmdbImageURL(derefStr(record.PosterURL), "w500"),
		"site":       record.Site, "rss_url": record.RssURL, "subgroup_name": record.SubgroupName,
		"season_number": record.SeasonNumber, "start_episode": record.StartEpisode,
		"keyword_all": decodeKeywordAll(record.KeywordAll),
		"push_target": record.PushTarget, "enabled": record.Enabled,
		"last_checked_at": record.LastCheckedAt, "last_matched_at": record.LastMatchedAt,
		"last_error": record.LastError, "created_at": record.CreatedAt,
		"updated_at": record.UpdatedAt,
		"hit_count":  rssStore.CountHits(record.ID),
		"tmdb":       nil,
		"library":    nil,
	}
	if record.TmdbID != nil && *record.TmdbID > 0 {
		libStore := RT().GetLibraryStore()
		if entry := libStore.GetTmdbEntry(record.MediaType, *record.TmdbID); entry != nil {
			raw := entry.RawJSON
			posterPath := firstNonEmptyStr(jsonx.Str(raw, "poster_path"), entry.PosterPath)
			backdropPath := firstNonEmptyStr(jsonx.Str(raw, "backdrop_path"), entry.BackdropPath)
			payload["tmdb"] = map[string]any{
				"tmdb_id": *record.TmdbID, "media_type": record.MediaType,
				"title":          firstNonEmptyStr(jsonx.Str(raw, "title"), jsonx.Str(raw, "name"), entry.Title),
				"original_title": firstNonEmptyStr(jsonx.Str(raw, "original_title"), jsonx.Str(raw, "original_name"), entry.OriginalTitle),
				"overview":       firstNonEmptyStr(jsonx.Str(raw, "overview"), entry.Overview),
				"poster_path":    posterPath,
				"poster_url":     tmdbImageURL(posterPath, "w500"),
				"backdrop_path":  backdropPath,
				"backdrop_url":   tmdbImageURL(backdropPath, ""),
				"release_date":   firstNonEmptyStr(jsonx.Str(raw, "release_date"), jsonx.Str(raw, "first_air_date")),
				"status":         firstNonEmptyStr(jsonx.Str(raw, "status"), entry.Status),
				"rating":         roundTo1F(firstNonZeroF(jsonx.Float(raw, "vote_average", 0), entry.VoteAverage)),
			}
		}
		if libraryEntry := libStore.GetLibraryItemByTmdb(record.MediaType, *record.TmdbID); libraryEntry != nil {
			payload["library"] = map[string]any{
				"in_library":          true,
				"drive_folder_id":     libraryEntry["drive_folder_id"],
				"title":               libraryEntry["title"],
				"year":                libraryEntry["year"],
				"poster_url":          libraryEntry["poster_url"],
				"total_episodes":      libraryEntry["total_episodes"],
				"in_library_episodes": libraryEntry["in_library_episodes"],
			}
		} else {
			payload["library"] = map[string]any{"in_library": false}
		}
	}
	recentHits := []map[string]any{}
	for _, h := range rssStore.ListHits(record.ID, 5) {
		recentHits = append(recentHits, hitToPayload(h))
	}
	payload["recent_hits"] = recentHits
	return payload
}

func firstNonZeroF(vals ...float64) float64 {
	for _, v := range vals {
		if v != 0 {
			return v
		}
	}
	return 0
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func decodeKeywordAll(s string) []string {
	out := []string{}
	if s != "" {
		var parsed []string
		if json.Unmarshal([]byte(s), &parsed) == nil && parsed != nil {
			return parsed
		}
	}
	return out
}

func encodeKeywordAll(items []string) string {
	normalized := normalizeKeywords(items)
	if normalized == nil {
		normalized = []string{} // 避免 nil 切片被存成字面量 "null"
	}
	data, _ := json.Marshal(normalized)
	return string(data)
}

// ── RSS URL 解析与抓取 ──────────────────────────────────────

func rssQueryFirst(u *url.URL, keys ...string) string {
	q := u.Query()
	for _, key := range keys {
		if vals, ok := q[key]; ok && len(vals) > 0 && vals[0] != "" {
			return vals[0]
		}
	}
	return ""
}

func parseRssURL(site, rssURL string) (string, string, error) {
	parsed, err := url.Parse(rssURL)
	if err != nil {
		return "", "", fmt.Errorf("RSS 链接无效")
	}
	switch site {
	case "mikan":
		mediaID := rssQueryFirst(parsed, "bangumiId", "bangumiid")
		if mediaID == "" {
			return "", "", fmt.Errorf("RSS 链接缺少 bangumiId，暂不支持该格式")
		}
		return mediaID, rssQueryFirst(parsed, "subgroupid", "subgroupId"), nil
	case "anibt":
		mediaID := rssQueryFirst(parsed, "bgmId", "bangumiId", "bangumiid")
		if mediaID == "" {
			return "", "", fmt.Errorf("AniBT RSS 链接缺少 bgmId，暂不支持该格式")
		}
		return mediaID, rssQueryFirst(parsed, "groupSlug", "subgroupSlug", "subgroupid", "subgroupId"), nil
	}
	return "", "", fmt.Errorf("暂不支持 RSS 站点：%s", site)
}

func fetchFeedItems(site, rssURL string) ([]map[string]any, error) {
	mediaID, subgroupID, err := parseRssURL(site, rssURL)
	if err != nil {
		return nil, err
	}
	factory := scraper.DefaultFactory()
	spider, err := factory.GetSpider(site)
	if err != nil {
		return nil, err
	}
	episodes, err := spider.GetEpisodes(mediaID, subgroupID)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(episodes))
	for _, ep := range episodes {
		items = append(items, map[string]any{
			"title": ep.Title, "torrent_url": ep.TorrentURL, "magnet_url": ep.MagnetURL,
			"publish_time": ep.PublishTime, "file_size_mb": ep.FileSizeMB, "site": ep.Site,
		})
	}
	return items, nil
}

// ── 标题分析与匹配 ──────────────────────────────────────────

func analyzeTitle(title string) map[string]any {
	cfg := RT().GetConfig()
	meta := mediaparser.MetaInfo(title, "", false, cfg.Parser.CustomWords, nil)
	seasonNumber := 1
	if meta.BeginSeason != nil {
		seasonNumber = *meta.BeginSeason
	}
	var episodeNumber any
	if meta.BeginEpisode != nil {
		episodeNumber = *meta.BeginEpisode
	}
	var parts []string
	for _, part := range []string{title, meta.Name(), meta.CnName, meta.EnName, meta.ResourceTerm(), meta.ReleaseGroup()} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	keywordSource := strings.ToLower(strings.Join(parts, " "))
	return map[string]any{
		"parse_success":  meta.BeginEpisode != nil,
		"name":           meta.Name(),
		"season_number":  seasonNumber,
		"episode_number": episodeNumber,
		"keyword_source": keywordSource,
		"resource_team":  meta.ReleaseGroup(),
		"resource_term":  meta.ResourceTerm(),
	}
}

func dedupeKey(item map[string]any) string {
	base := firstNonEmptyStr(
		anyStrValue(item["magnet_url"]),
		anyStrValue(item["torrent_url"]),
		anyStrValue(item["title"])+"|"+anyStrValue(item["publish_time"]),
	)
	sum := sha1.Sum([]byte(base))
	return hex.EncodeToString(sum[:])
}

func anyStrValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// filterMatchItems 过滤匹配条目（对应 _filter_match_items）。
func filterMatchItems(items []map[string]any, seasonNumber, startEpisode int, keywordAll []string) map[string]any {
	normalizedKeywords := normalizeKeywords(keywordAll)
	matches := []map[string]any{}
	parsedItems := 0

	for _, item := range items {
		analysis := analyzeTitle(anyStrValue(item["title"]))
		parseSuccess, _ := analysis["parse_success"].(bool)
		if parseSuccess {
			parsedItems++
		}
		episodeNumber := analysis["episode_number"]
		keywordSource, _ := analysis["keyword_source"].(string)
		var keywordHits []string
		for _, kw := range normalizedKeywords {
			if strings.Contains(keywordSource, kw) {
				keywordHits = append(keywordHits, kw)
			}
		}
		allKeywordsHit := len(keywordHits) == len(normalizedKeywords)
		seasonMatch := analysis["season_number"].(int) == seasonNumber
		episodeMatch := false
		if epNum, ok := episodeNumber.(int); ok {
			episodeMatch = epNum >= startEpisode
		}
		wouldPush := parseSuccess && seasonMatch && episodeMatch && allKeywordsHit
		matches = append(matches, map[string]any{
			"title":            anyStrValue(item["title"]),
			"season_number":    analysis["season_number"],
			"episode_number":   episodeNumber,
			"publish_time":     item["publish_time"],
			"torrent_url":      item["torrent_url"],
			"magnet_url":       item["magnet_url"],
			"parse_success":    parseSuccess,
			"keyword_hits":     keywordHits,
			"all_keywords_hit": allKeywordsHit,
			"season_match":     seasonMatch,
			"episode_match":    episodeMatch,
			"would_push":       wouldPush,
			"dedupe_key":       dedupeKey(item),
		})
	}

	effectiveMatches := []map[string]any{}
	for _, item := range matches {
		if item["would_push"] == true {
			effectiveMatches = append(effectiveMatches, item)
		}
	}
	return map[string]any{
		"summary": map[string]any{
			"total_items": len(items), "parsed_items": parsedItems,
			"matched_items": len(effectiveMatches),
		},
		"matches":           matches,
		"effective_matches": effectiveMatches,
	}
}

// withRecentHits 给 joined 载荷补最近命中。
func withRecentHits(rssStore *store.RSSStore, payload map[string]any) map[string]any {
	id, _ := payload["id"].(int64)
	recentHits := []map[string]any{}
	for _, h := range rssStore.ListHits(id, 5) {
		recentHits = append(recentHits, hitToPayload(h))
	}
	payload["recent_hits"] = recentHits
	return payload
}

// ── 推送 ────────────────────────────────────────────────────

func pushItem(pushTarget string, item map[string]any) error {
	magnetURL := anyStrValue(item["magnet_url"])
	torrentURL := anyStrValue(item["torrent_url"])
	switch pushTarget {
	case "u115":
		auth := u115OauthStatusPayload()
		if authorized, _ := auth["authorized"].(bool); !authorized {
			return fmt.Errorf("115 未授权，无法推送云下载")
		}
		if magnetURL == "" && torrentURL == "" {
			return fmt.Errorf("RSS 条目缺少下载地址")
		}
		// 磁力/普通链接走链接任务；.torrent 链接在 pushU115Download 内转 BT 任务
		return pushU115Download(magnetURL, torrentURL)
	}
	return fmt.Errorf("未知推送目标：%s", pushTarget)
}

// checkSubscriptionRecord 检查一条订阅（抓取→匹配→推送→记录）。
func checkSubscriptionRecord(rssStore *store.RSSStore, record *store.SubscriptionRecord, manual bool) map[string]any {
	items, err := fetchFeedItems(record.Site, record.RssURL)
	if err != nil {
		rssStore.MarkChecked(record.ID, "", err.Error())
		return map[string]any{"matched": 0, "pushed": 0, "skipped": 0,
			"errors": []string{err.Error()}, "last_matched_at": nil}
	}
	result := filterMatchItems(items, record.SeasonNumber, record.StartEpisode, decodeKeywordAll(record.KeywordAll))

	pushed, skipped := 0, 0
	errors := []string{}
	effective, _ := result["effective_matches"].([]map[string]any)
	for _, item := range effective {
		dedupe := item["dedupe_key"].(string)
		if rssStore.HitExists(record.ID, dedupe) {
			skipped++
			continue
		}
		seasonNumber, _ := item["season_number"].(int)
		hitPayload := map[string]any{
			"subscription_id": record.ID, "episode_title": item["title"],
			"season_number": seasonNumber, "episode_number": item["episode_number"],
			"torrent_url": item["torrent_url"], "magnet_url": item["magnet_url"],
			"published_at": item["publish_time"], "dedupe_key": dedupe,
			"pushed_to": record.PushTarget,
		}
		if err := pushItem(record.PushTarget, item); err != nil {
			hitPayload["push_status"] = "failed"
			hitPayload["push_error"] = err.Error()
			_, _ = rssStore.RecordHit(hitPayload)
			errors = append(errors, err.Error())
			continue
		}
		hitPayload["push_status"] = "success"
		if _, err := rssStore.RecordHit(hitPayload); err == nil {
			pushed++
		}
	}

	var lastMatchedAt any
	if pushed > 0 {
		rssStore.MarkMatched(record.ID, "")
		if refreshed, _ := rssStore.GetSubscription(record.ID); refreshed != nil {
			lastMatchedAt = refreshed.LastMatchedAt
		}
	}
	errorStr := ""
	if len(errors) > 0 {
		errorStr = strings.Join(errors, "; ")
	}
	rssStore.MarkChecked(record.ID, "", errorStr)

	if pushed > 0 || manual {
		level := "SUCCESS"
		if len(errors) > 0 {
			level = "WARNING"
		}
		summary := result["summary"].(map[string]any)
		appLog("subscription", "checked", level, "RSS 订阅检查完成："+record.Name, map[string]any{
			"subscription_id": record.ID,
			"matched":         summary["matched_items"],
			"pushed":          pushed, "skipped": skipped, "errors": errors,
		})
	}
	return map[string]any{
		"summary": result["summary"], "matched": result["summary"].(map[string]any)["matched_items"],
		"pushed": pushed, "skipped": skipped, "errors": errors,
		"last_matched_at": lastMatchedAt,
	}
}

// PollSubscriptionsOnce 轮询全部启用订阅（watcher 调用）。
func PollSubscriptionsOnce() map[string]any {
	rssStore := RT().GetRSSStore()
	if rssStore == nil {
		return map[string]any{"checked": 0, "pushed": 0, "errors": 1}
	}
	records := rssStore.ListEnabledSubscriptions()
	checked, pushed, errCount := 0, 0, 0
	for i, record := range records {
		if i > 0 {
			time.Sleep(3 * time.Second) // 防止触发 mikan 阈值
		}
		checked++
		result := checkSubscriptionRecord(rssStore, record, false)
		if p, ok := result["pushed"].(int); ok {
			pushed += p
		}
		if errs, ok := result["errors"].([]string); ok && len(errs) > 0 {
			errCount++
		}
	}
	return map[string]any{"checked": checked, "pushed": pushed, "errors": errCount}
}

// SubscriptionsStatusPayload 订阅状态。
func SubscriptionsStatusPayload() map[string]any {
	rssStore := RT().GetRSSStore()
	if rssStore == nil {
		return map[string]any{"enabled_count": 0, "total_count": 0, "poll_seconds": defaultSubscriptionPollSeconds}
	}
	return map[string]any{
		"enabled_count": len(rssStore.ListEnabledSubscriptions()),
		"total_count":   len(rssStore.ListSubscriptions()),
		"poll_seconds":  getSubscriptionPollSeconds(),
	}
}

// detectSiteFromURL 从 RSS URL 识别站点。
func detectSiteFromURL(rssURL string) (string, error) {
	parsed, err := url.Parse(rssURL)
	if err != nil {
		return "", fmt.Errorf("不支持的 RSS 站点：")
	}
	host := strings.ToLower(parsed.Host)
	if strings.Contains(host, "mikan") {
		return "mikan", nil
	}
	if strings.Contains(host, "anibt.net") {
		return "anibt", nil
	}
	return "", fmt.Errorf("不支持的 RSS 站点：%s", host)
}

// ── HTTP 处理器 ─────────────────────────────────────────────

type subscriptionBody struct {
	Name         string   `json:"name"`
	MediaTitle   string   `json:"media_title"`
	MediaType    string   `json:"media_type"`
	TmdbID       *int64   `json:"tmdb_id"`
	PosterURL    *string  `json:"poster_url"`
	Site         string   `json:"site"`
	RssURL       string   `json:"rss_url"`
	SubgroupName string   `json:"subgroup_name"`
	SeasonNumber int      `json:"season_number"`
	StartEpisode int      `json:"start_episode"`
	KeywordAll   []string `json:"keyword_all"`
	PushTarget   string   `json:"push_target"`
	Enabled      *bool    `json:"enabled"`
}

func (b *subscriptionBody) validate() string {
	if strings.TrimSpace(b.Name) == "" {
		return "name 不能为空"
	}
	if strings.TrimSpace(b.MediaTitle) == "" {
		return "media_title 不能为空"
	}
	if strings.TrimSpace(b.Site) == "" || strings.TrimSpace(b.RssURL) == "" {
		return "site / rss_url 不能为空"
	}
	if b.PushTarget != "u115" {
		return "push_target 仅支持 u115"
	}
	if b.SeasonNumber < 1 || b.SeasonNumber > 99 {
		return "season_number 必须在 1 到 99 之间"
	}
	if b.StartEpisode < 1 || b.StartEpisode > 9999 {
		return "start_episode 必须在 1 到 9999 之间"
	}
	return ""
}

func (b *subscriptionBody) toPayload() map[string]any {
	enabled := true
	if b.Enabled != nil {
		enabled = *b.Enabled
	}
	return map[string]any{
		"name": b.Name, "media_title": b.MediaTitle,
		"media_type": firstNonEmptyStr(b.MediaType, "tv"),
		"tmdb_id":    b.TmdbID, "poster_url": b.PosterURL,
		"site": b.Site, "rss_url": b.RssURL, "subgroup_name": b.SubgroupName,
		"season_number": maxIntValue(b.SeasonNumber, 1), "start_episode": maxIntValue(b.StartEpisode, 1),
		"keyword_all": encodeKeywordAll(b.KeywordAll),
		"push_target": b.PushTarget, "enabled": enabled,
	}
}

func maxIntValue(v, min int) int {
	if v < min {
		return min
	}
	return v
}

func handleListSubscriptions(w http.ResponseWriter, _ *http.Request) {
	rssStore := RT().GetRSSStore()
	if rssStore == nil {
		writeJSONError(w, http.StatusInternalServerError, "订阅存储不可用")
		return
	}
	// 存量订阅缓存缺失时惰性补全（有 TMDB key 才会发起）
	for _, record := range rssStore.ListSubscriptions() {
		if record.TmdbID != nil && *record.TmdbID > 0 {
			ensureSubscriptionTmdbCache(record.MediaType, *record.TmdbID)
		}
	}
	subscriptions := []map[string]any{}
	if joined := rssStore.ListSubscriptionsJoined(); len(joined) > 0 {
		for _, item := range joined {
			subscriptions = append(subscriptions, withRecentHits(rssStore, item))
		}
	} else {
		for _, record := range rssStore.ListSubscriptions() {
			subscriptions = append(subscriptions, serializeSubscription(rssStore, record))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "subscriptions": subscriptions})
}

func handleCreateSubscription(w http.ResponseWriter, r *http.Request) {
	rssStore := RT().GetRSSStore()
	if rssStore == nil {
		writeJSONError(w, http.StatusInternalServerError, "订阅存储不可用")
		return
	}
	var body subscriptionBody
	if err := jsonDecodeBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if msg := body.validate(); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	record, err := rssStore.CreateSubscription(body.toPayload())
	if err != nil {
		if err == store.ErrDuplicateSubscription {
			writeJSONError(w, http.StatusConflict, "订阅已存在，请勿重复创建相同规则")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if record.TmdbID != nil && *record.TmdbID > 0 {
		// 资源检索等直创流程不经过 parse-rss，这里补拉详情入缓存，
		// 创建响应与订阅卡片立即可显示中文标题/海报
		ensureSubscriptionTmdbCache(record.MediaType, *record.TmdbID)
	}
	appLog("subscription", "created", "SUCCESS", "已创建 RSS 订阅："+record.Name, map[string]any{
		"subscription_id": record.ID, "site": record.Site, "push_target": record.PushTarget,
	})
	payload := map[string]any{}
	if joined := rssStore.GetSubscriptionJoined(record.ID); joined != nil {
		payload = withRecentHits(rssStore, joined)
	} else {
		payload = serializeSubscription(rssStore, record)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "subscription": payload})
}

func handleTestSubscription(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Site         string   `json:"site"`
		RssURL       string   `json:"rss_url"`
		SeasonNumber int      `json:"season_number"`
		StartEpisode int      `json:"start_episode"`
		KeywordAll   []string `json:"keyword_all"`
	}
	if err := jsonDecodeBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	items, err := fetchFeedItems(body.Site, body.RssURL)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	result := filterMatchItems(items, body.SeasonNumber, body.StartEpisode, body.KeywordAll)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "summary": result["summary"], "matches": result["matches"],
	})
}

func handleParseSubscriptionRss(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RssURL string `json:"rss_url"`
	}
	if err := jsonDecodeBody(r, &body); err != nil || body.RssURL == "" {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": "rss_url 不能为空"})
		return
	}
	site, err := detectSiteFromURL(body.RssURL)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": err.Error()})
		return
	}
	items, err := fetchFeedItems(site, body.RssURL)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": err.Error()})
		return
	}
	if len(items) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": "RSS 源中没有找到任何条目"})
		return
	}
	first := items[0]
	title := anyStrValue(first["title"])
	analysis := analyzeTitle(title)
	if parseSuccess, _ := analysis["parse_success"].(bool); !parseSuccess {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": "无法解析标题：" + title})
		return
	}
	mediaTitle, _ := analysis["name"].(string)
	var tmdbID any
	var posterURL any
	mediaType := "tv"
	if mediaTitle != "" {
		if client := RT().GetTmdbClient(); client != nil {
			results := client.SearchRawMulti(mediaTitle)
			if len(results) > 0 {
				firstTMDB := results[0]
				id := jsonx.Int(firstTMDB, "tmdb_id", jsonx.Int(firstTMDB, "id", 0))
				if id > 0 {
					tmdbID = id
					if p := jsonx.Str(firstTMDB, "poster_path"); p != "" {
						posterURL = tmdb.ImageURL(p, "w500", RT().GetConfig().TmdbImageBaseURL())
					}
					if mt := jsonx.Str(firstTMDB, "media_type"); mt != "" {
						mediaType = mt
					}
					// 搜索命中后拉详情入缓存，订阅卡片立即可显示中文标题/海报
					ensureSubscriptionTmdbCache(mediaType, id)
				}
			}
		}
	}
	subgroupName, _ := analysis["resource_team"].(string)
	name := mediaTitle
	if subgroupName != "" {
		name = mediaTitle + " [" + subgroupName + "]"
	}
	startEpisode := 1
	if ep, ok := analysis["episode_number"].(int); ok {
		startEpisode = ep
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "name": name, "media_title": mediaTitle,
		"subgroup_name": subgroupName,
		"season_number": analysis["season_number"], "start_episode": startEpisode,
		"media_type": mediaType, "tmdb_id": tmdbID, "poster_url": posterURL, "site": site,
	})
}

func subscriptionIDFromPath(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("subscriptionID"), 10, 64)
}

func handleGetSubscription(w http.ResponseWriter, r *http.Request) {
	rssStore := RT().GetRSSStore()
	id, err := subscriptionIDFromPath(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "subscription_id 无效")
		return
	}
	if joined := rssStore.GetSubscriptionJoined(id); joined != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "subscription": withRecentHits(rssStore, joined)})
		return
	}
	record, _ := rssStore.GetSubscription(id)
	if record == nil {
		writeJSONError(w, http.StatusNotFound, "订阅不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "subscription": serializeSubscription(rssStore, record)})
}

func handleUpdateSubscription(w http.ResponseWriter, r *http.Request) {
	rssStore := RT().GetRSSStore()
	id, err := subscriptionIDFromPath(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "subscription_id 无效")
		return
	}
	var body subscriptionBody
	if err := jsonDecodeBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if msg := body.validate(); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	record, err := rssStore.UpdateSubscription(id, body.toPayload())
	if err != nil {
		if err == store.ErrDuplicateSubscription {
			writeJSONError(w, http.StatusConflict, "更新后的订阅规则与现有订阅重复")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if record == nil {
		writeJSONError(w, http.StatusNotFound, "订阅不存在")
		return
	}
	if record.TmdbID != nil && *record.TmdbID > 0 {
		// 更新可能补填/更换 tmdb_id，同步补拉详情入缓存
		ensureSubscriptionTmdbCache(record.MediaType, *record.TmdbID)
	}
	appLog("subscription", "updated", "SUCCESS", "已更新 RSS 订阅："+record.Name,
		map[string]any{"subscription_id": record.ID})
	var payload map[string]any
	if joined := rssStore.GetSubscriptionJoined(record.ID); joined != nil {
		payload = withRecentHits(rssStore, joined)
	} else {
		payload = serializeSubscription(rssStore, record)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "subscription": payload})
}

func handleDeleteSubscription(w http.ResponseWriter, r *http.Request) {
	rssStore := RT().GetRSSStore()
	id, err := subscriptionIDFromPath(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "subscription_id 无效")
		return
	}
	record, _ := rssStore.GetSubscription(id)
	if record == nil {
		writeJSONError(w, http.StatusNotFound, "订阅不存在")
		return
	}
	rssStore.DeleteSubscription(id)
	appLog("subscription", "deleted", "SUCCESS", "已删除 RSS 订阅："+record.Name,
		map[string]any{"subscription_id": id})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func handleCheckSubscription(w http.ResponseWriter, r *http.Request) {
	rssStore := RT().GetRSSStore()
	id, err := subscriptionIDFromPath(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "subscription_id 无效")
		return
	}
	record, _ := rssStore.GetSubscription(id)
	if record == nil {
		writeJSONError(w, http.StatusNotFound, "订阅不存在")
		return
	}
	result := checkSubscriptionRecord(rssStore, record, true)
	refreshed, _ := rssStore.GetSubscription(id)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "result": result, "subscription": serializeSubscription(rssStore, refreshed),
	})
}

// handleSubscriptionPostOp POST /api/subscriptions/{id} 与 {id}:check 分发
// （Go ServeMux 通配符段不支持后缀语法，这里在处理器内解析）。
func handleSubscriptionPostOp(w http.ResponseWriter, r *http.Request) {
	op := r.PathValue("subscriptionID")
	if strings.HasSuffix(op, ":check") {
		r.SetPathValue("subscriptionID", strings.TrimSuffix(op, ":check"))
		handleCheckSubscription(w, r)
		return
	}
	writeJSONError(w, http.StatusNotFound, "未知操作")
}

// subscriptionTmdbCachePrefetching 防止列表并发重复拉取详情。
var subscriptionTmdbCachePrefetching sync.Map

// ensureSubscriptionTmdbCache 确保订阅关联的 TMDB 详情已入实体缓存。
// parse-rss 搜索命中不会写库(与 Python 一致)，导致卡片标题回退到 RSS 原名；
// 这里在创建/列表时补拉一次详情，命中后 tmdb_media 有中文标题与海报。
// 返回 true 表示调用方应基于新缓存重新序列化。
func ensureSubscriptionTmdbCache(mediaType string, tmdbID int64) bool {
	if tmdbID <= 0 {
		return false
	}
	client := RT().GetTmdbClient()
	if client == nil {
		return false
	}
	if entry := RT().GetLibraryStore().GetTmdbEntry(mediaType, tmdbID); entry != nil && len(entry.RawJSON) > 0 {
		return false // 已有缓存
	}
	key := fmt.Sprintf("%s:%d", mediaType, tmdbID)
	if _, busy := subscriptionTmdbCachePrefetching.LoadOrStore(key, true); busy {
		return false
	}
	defer subscriptionTmdbCachePrefetching.Delete(key)
	path := fmt.Sprintf("/movie/%d", tmdbID)
	if mediaType == "tv" {
		path = fmt.Sprintf("/tv/%d", tmdbID)
	}
	// Get 走缓存接口：详情请求结果由 TmdbCache.Set 写入 tmdb_media
	if client.Get(path) == nil {
		return false
	}
	return true
}
