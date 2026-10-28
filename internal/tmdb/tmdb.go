// Package tmdb —— 独立的 TMDB 元数据获取模块（对应 Python 版 mediaparser/tmdb.py）。
package tmdb

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"meta2cloud/internal/jsonx"
	"meta2cloud/internal/mediaparser"
)

// redactURLError 去掉 *url.Error 携带的完整 URL（query 含 api_key），只保留底层错误，
// 防止 DNS/超时等失败时凭据随错误日志外泄。
func redactURLError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

func decodeJSON(raw []byte, out *map[string]any) error {
	return json.Unmarshal(raw, out)
}

const (
	baseURL       = "https://api.themoviedb.org/3"
	retryWaitSecs = 5 // 429 限流默认等待秒数
)

// Cache TMDB 响应缓存接口（webui 层用 SQLite 实现）。
type Cache interface {
	Get(path, language string) (map[string]any, bool)
	IsFailed(path, language string) bool
	Set(path string, data map[string]any, language string)
	SetFailed(path, language string)
}

// Client TMDB v3 API 客户端。
type Client struct {
	apiKey   string
	language string
	timeout  time.Duration
	cache    Cache
	hc       *http.Client

	// baseURLOverride 非空时覆盖 API 地址（仅测试使用）。
	baseURLOverride string
}

// NewClient 构造客户端。proxy 形如 "http://127.0.0.1:7890"，可为空。
func NewClient(apiKey, language, proxy string, timeoutSecs int, cache Cache) *Client {
	if language == "" {
		language = "zh-CN"
	}
	if timeoutSecs <= 0 {
		timeoutSecs = 10
	}
	transport := http.DefaultTransport
	if proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err == nil {
			transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		}
	}
	return &Client{
		apiKey:   apiKey,
		language: language,
		timeout:  time.Duration(timeoutSecs) * time.Second,
		cache:    cache,
		hc:       &http.Client{Timeout: time.Duration(timeoutSecs) * time.Second, Transport: transport},
	}
}

// ── 核心入口 ────────────────────────────────────────────────

// Recognize 根据解析结果查询 TMDB 并返回完整媒体信息字典；找不到返回 nil。
func (c *Client) Recognize(meta *mediaparser.MetaBase) map[string]any {
	// 1. 有内嵌 tmdbid 直接查
	if meta.TmdbID != 0 {
		if info := c.GetByID(meta.Type, meta.TmdbID); info != nil {
			return info
		}
	}

	// 2. 按名称搜索
	names := buildNameList(meta)
	mtype := meta.Type
	hasType := mtype != mediaparser.MediaUnknown
	for _, name := range names {
		if info := c.searchByName(name, meta.Year, mtype, hasType, meta.BeginSeason); info != nil {
			return info
		}
	}
	return nil
}

// GetByID 通过 TMDB ID 获取完整信息。mtype 未知时同时尝试电影和电视剧。
func (c *Client) GetByID(mtype mediaparser.MediaType, tmdbid int) map[string]any {
	switch mtype {
	case mediaparser.MediaMovie:
		return c.getMovieDetail(tmdbid)
	case mediaparser.MediaTV:
		return c.getTVDetail(tmdbid)
	default:
		if info := c.getMovieDetail(tmdbid); info != nil {
			return info
		}
		return c.getTVDetail(tmdbid)
	}
}

// GetByIDFresh 强制请求 TMDB 最新详情（绕过缓存读取，结果回写缓存）。
// 用于分类重判定等需要权威数据的场景：TMDB 流派可能变化，缓存值最多滞后一个 TTL。
func (c *Client) GetByIDFresh(mtype mediaparser.MediaType, tmdbid int) map[string]any {
	switch mtype {
	case mediaparser.MediaMovie:
		return c.movieDetail(tmdbid, false)
	case mediaparser.MediaTV:
		return c.tvDetail(tmdbid, false)
	default:
		if info := c.movieDetail(tmdbid, false); info != nil {
			return info
		}
		return c.tvDetail(tmdbid, false)
	}
}

// GetSeasonDetail 获取某季的完整信息（含所有集）。
func (c *Client) GetSeasonDetail(tmdbid, season int) map[string]any {
	return c.get(fmt.Sprintf("/tv/%d/season/%d", tmdbid, season), nil, true)
}

// GetEpisodeDetail 获取某集的完整信息。
func (c *Client) GetEpisodeDetail(tmdbid, season, episode int) map[string]any {
	return c.get(fmt.Sprintf("/tv/%d/season/%d/episode/%d", tmdbid, season, episode), nil, true)
}

// ── 搜索逻辑 ────────────────────────────────────────────────

func (c *Client) searchByName(name, year string, mtype mediaparser.MediaType, hasType bool, _ *int) map[string]any {
	if name == "" {
		return nil
	}
	yearRange := buildYearRange(year)

	if hasType {
		switch mtype {
		case mediaparser.MediaMovie:
			for _, y := range yearRange {
				if info := c.matchMovie(name, y); info != nil {
					return info
				}
			}
		case mediaparser.MediaTV:
			for _, y := range yearRange {
				if info := c.matchTV(name, y); info != nil {
					return info
				}
			}
		}
		return nil
	}
	// 类型未知：先电影再电视剧再 multi
	for _, y := range yearRange {
		if info := c.matchMovie(name, y); info != nil {
			return info
		}
	}
	for _, y := range yearRange {
		if info := c.matchTV(name, y); info != nil {
			return info
		}
	}
	return c.matchMulti(name)
}

func (c *Client) matchMovie(name, year string) map[string]any {
	params := map[string]string{"query": name}
	if year != "" {
		params["year"] = year
	}
	results := c.search("movie", params)
	if len(results) == 0 {
		return nil
	}
	sortResults(results, "release_date")
	var fastSkipped []map[string]any
	yearMatchCount := 0
	var firstYearMatch map[string]any
	for _, r := range results {
		rYear := firstN(jsonx.Str(r, "release_date"), 4)
		if year != "" && rYear != year {
			continue
		}
		yearMatchCount++
		if firstYearMatch == nil {
			firstYearMatch = r
		}
		if nameMatch(name, jsonx.Str(r, "title"), jsonx.Str(r, "original_title")) {
			return c.getMovieDetail(int(jsonx.Int(r, "id", 0)))
		}
		fastSkipped = append(fastSkipped, r)
	}
	// 降级：对前 3 条拉完整详情，比对译名/别名
	for i, r := range fastSkipped {
		if i >= 3 {
			break
		}
		detail := c.getMovieDetail(int(jsonx.Int(r, "id", 0)))
		if detail != nil && matchNameList(name, toStrSlice(detail["names"])) {
			return detail
		}
	}
	// 最终回退：同 matchTV——年份吻合的唯一候选直接采纳（民间译名场景）
	if yearMatchCount == 1 && firstYearMatch != nil {
		return c.getMovieDetail(int(jsonx.Int(firstYearMatch, "id", 0)))
	}
	return nil
}

func (c *Client) matchTV(name, year string) map[string]any {
	params := map[string]string{"query": name}
	if year != "" {
		params["first_air_date_year"] = year
	}
	results := c.search("tv", params)
	if len(results) == 0 {
		return nil
	}
	sortResults(results, "first_air_date")
	var yearMatches []map[string]any
	for _, r := range results {
		rYear := firstN(jsonx.Str(r, "first_air_date"), 4)
		if year != "" && rYear != year {
			continue
		}
		yearMatches = append(yearMatches, r)
		if nameMatch(name, jsonx.Str(r, "name"), jsonx.Str(r, "original_name")) {
			return c.getTVDetail(int(jsonx.Int(r, "id", 0)))
		}
		// 降级：拉完整详情，比对全部译名/别名
		detail := c.getTVDetail(int(jsonx.Int(r, "id", 0)))
		if detail != nil && matchNameList(name, toStrSlice(detail["names"])) {
			return detail
		}
	}
	// 最终回退：TMDB 模糊搜索已命中且年份吻合、仅剩唯一候选时直接采纳。
	// 场景：目录名是民间译名（如「令和妖神斑小姐」），TMDB 搜索能返回官方
	// 条目（「令和的斑小姐」），但民间译名不在主字段也不在别名表，精确
	// 匹配全部落空——唯一候选 + 年份吻合已足够可信。
	if len(yearMatches) == 1 {
		return c.getTVDetail(int(jsonx.Int(yearMatches[0], "id", 0)))
	}
	return nil
}

func (c *Client) matchMulti(name string) map[string]any {
	results := c.search("multi", map[string]string{"query": name})
	if len(results) == 0 {
		return nil
	}
	// movie 优先，其次按日期倒序
	sort.SliceStable(results, func(i, j int) bool {
		return multiSortKey(results[i]) > multiSortKey(results[j])
	})
	for _, r := range results {
		switch jsonx.Str(r, "media_type") {
		case "movie":
			if nameMatch(name, jsonx.Str(r, "title"), jsonx.Str(r, "original_title")) {
				return c.getMovieDetail(int(jsonx.Int(r, "id", 0)))
			}
			detail := c.getMovieDetail(int(jsonx.Int(r, "id", 0)))
			if detail != nil && matchNameList(name, toStrSlice(detail["names"])) {
				return detail
			}
		case "tv":
			if nameMatch(name, jsonx.Str(r, "name"), jsonx.Str(r, "original_name")) {
				return c.getTVDetail(int(jsonx.Int(r, "id", 0)))
			}
			detail := c.getTVDetail(int(jsonx.Int(r, "id", 0)))
			if detail != nil && matchNameList(name, toStrSlice(detail["names"])) {
				return detail
			}
		}
	}
	return nil
}

func multiSortKey(r map[string]any) string {
	prefix := "0"
	if jsonx.Str(r, "media_type") == "movie" {
		prefix = "1"
	}
	date := jsonx.Str(r, "release_date")
	if date == "" {
		date = jsonx.Str(r, "first_air_date")
	}
	return prefix + date
}

func sortResults(results []map[string]any, dateKey string) {
	sort.SliceStable(results, func(i, j int) bool {
		return jsonx.Str(results[i], dateKey) > jsonx.Str(results[j], dateKey)
	})
}

func firstN(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}

func toStrSlice(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// ── 详情获取 ────────────────────────────────────────────────

const appendFields = "credits,alternative_titles,translations,external_ids"

func checkAppendFields(data map[string]any, fields string) bool {
	if fields == "" {
		return true
	}
	for _, field := range strings.Split(fields, ",") {
		if data[field] == nil {
			return false
		}
	}
	return true
}

func (c *Client) getMovieDetail(tmdbid int) map[string]any {
	return c.movieDetail(tmdbid, true)
}

// movieDetail useCache=false 时绕过缓存读取并回写缓存（供需要最新数据的场景）。
func (c *Client) movieDetail(tmdbid int, useCache bool) map[string]any {
	extra := map[string]string{"append_to_response": appendFields}
	path := fmt.Sprintf("/movie/%d", tmdbid)
	data := c.get(path, extra, useCache)
	if data == nil {
		return nil
	}
	// 缓存数据可能不完整，缺少任意 append 字段时强制重新请求
	if !checkAppendFields(data, appendFields) {
		data = c.get(path, extra, false)
		if data == nil {
			return nil
		}
	}
	if c.cache != nil && !useCache {
		c.cache.Set(buildCachePath(path, extra), data, c.language)
	}
	data["media_type"] = mediaparser.MediaMovie.String()
	data["tmdb_id"] = data["id"]
	data["names"] = extractNames(data, mediaparser.MediaMovie)
	data["directors"] = extractDirectors(data)
	data["actors"] = extractActors(data)
	return data
}

func (c *Client) getTVDetail(tmdbid int) map[string]any {
	return c.tvDetail(tmdbid, true)
}

// tvDetail useCache=false 时绕过缓存读取并回写缓存（供需要最新数据的场景）。
func (c *Client) tvDetail(tmdbid int, useCache bool) map[string]any {
	extra := map[string]string{"append_to_response": appendFields}
	path := fmt.Sprintf("/tv/%d", tmdbid)
	data := c.get(path, extra, useCache)
	if data == nil {
		return nil
	}
	if !checkAppendFields(data, appendFields) {
		data = c.get(path, extra, false)
		if data == nil {
			return nil
		}
	}
	if c.cache != nil && !useCache {
		c.cache.Set(buildCachePath(path, extra), data, c.language)
	}
	data["media_type"] = mediaparser.MediaTV.String()
	data["tmdb_id"] = data["id"]
	data["names"] = extractNames(data, mediaparser.MediaTV)
	data["directors"] = extractDirectors(data)
	data["actors"] = extractActors(data)
	return data
}

// ── 名称匹配工具 ────────────────────────────────────────────

var nonNameCharRe = regexp.MustCompile(`[^a-zA-Z0-9\x{4e00}-\x{9fff}]`)

func normalizeName(s string) string {
	return strings.ToUpper(nonNameCharRe.ReplaceAllString(s, ""))
}

func nameMatch(query string, candidates ...string) bool {
	q := normalizeName(query)
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if q == normalizeName(c) {
			return true
		}
	}
	return false
}

func matchNameList(query string, candidates []string) bool {
	return nameMatch(query, candidates...)
}

func extractNames(data map[string]any, mtype mediaparser.MediaType) []string {
	names := []string{}
	seen := map[string]bool{}
	add := func(n string) {
		if n != "" && !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	alt := jsonx.Map(data, "alternative_titles")
	if mtype == mediaparser.MediaMovie {
		for _, t := range jsonx.MapSlice(alt, "titles") {
			add(jsonx.Str(t, "title"))
		}
	} else {
		for _, t := range jsonx.MapSlice(alt, "results") {
			add(jsonx.Str(t, "title"))
		}
	}
	trans := jsonx.Map(data, "translations")
	key := "title"
	if mtype != mediaparser.MediaMovie {
		key = "name"
	}
	for _, tr := range jsonx.MapSlice(trans, "translations") {
		add(jsonx.Str(jsonx.Map(tr, "data"), key))
	}
	return names
}

func extractDirectors(data map[string]any) []map[string]any {
	crew := jsonx.MapSlice(jsonx.Map(data, "credits"), "crew")
	directors := []map[string]any{}
	for _, p := range crew {
		if jsonx.Str(p, "job") == "Director" {
			directors = append(directors, map[string]any{
				"id":           p["id"],
				"name":         p["name"],
				"profile_path": p["profile_path"],
			})
		}
	}
	return directors
}

func extractActors(data map[string]any) []map[string]any {
	cast := jsonx.MapSlice(jsonx.Map(data, "credits"), "cast")
	actors := []map[string]any{}
	for i, p := range cast {
		if i >= 20 { // 只取前 20
			break
		}
		actors = append(actors, map[string]any{
			"id":           p["id"],
			"name":         p["name"],
			"character":    p["character"],
			"profile_path": p["profile_path"],
		})
	}
	return actors
}

func buildNameList(meta *mediaparser.MetaBase) []string {
	var names []string
	seen := map[string]bool{}
	add := func(n string) {
		if n == "" || seen[n] {
			return
		}
		seen[n] = true
		names = append(names, n)
	}
	for _, n := range []string{meta.CnName, meta.EnName} {
		if n == "" {
			continue
		}
		add(n)
		// 繁体 → 简体
		if simplified := mediaparser.ToSimplified(n); simplified != n {
			add(simplified)
		}
	}
	return names
}

func buildYearRange(year string) []string {
	if year == "" {
		return []string{""}
	}
	y, err := strconv.Atoi(year)
	if err != nil {
		return []string{year}
	}
	return []string{year, strconv.Itoa(y - 1), strconv.Itoa(y + 1)}
}

// ── HTTP 基础方法 ───────────────────────────────────────────

// Get 通用 GET 请求（走缓存），供 webui 层按路径取任意 TMDB 资源。
func (c *Client) Get(path string) map[string]any {
	return c.get(path, nil, true)
}

// SearchByName 按名称+年份+类型搜索（供媒体库扫描身份识别使用）。
func (c *Client) SearchByName(name, year string, mtype mediaparser.MediaType) map[string]any {
	hasType := mtype != mediaparser.MediaUnknown
	return c.searchByName(name, year, mtype, hasType, nil)
}

// SetTestBaseURL 覆盖 API 地址（仅测试使用）。
func (c *Client) SetTestBaseURL(baseURL string) { c.baseURLOverride = baseURL }

// SearchRawMulti 直接透传调用 /search/multi，返回原始 results（不走缓存避免结果陈旧）。
func (c *Client) SearchRawMulti(query string) []map[string]any {
	return c.search("multi", map[string]string{"query": query})
}

func (c *Client) search(endpoint string, params map[string]string) []map[string]any {
	data := c.get("/search/"+endpoint, params, false)
	if data == nil {
		return nil
	}
	return jsonx.MapSlice(data, "results")
}

func (c *Client) get(path string, extraParams map[string]string, useCache bool) map[string]any {
	params := url.Values{}
	params.Set("api_key", c.apiKey)
	params.Set("language", c.language)
	for k, v := range extraParams {
		params.Set(k, v)
	}
	cachePath := buildCachePath(path, extraParams)

	if c.cache != nil && useCache {
		if cached, ok := c.cache.Get(cachePath, c.language); ok {
			return cached
		}
		if c.cache.IsFailed(cachePath, c.language) {
			return nil
		}
	}

	target := baseURL + path
	if c.baseURLOverride != "" {
		target = c.baseURLOverride + path
	}
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequest(http.MethodGet, target, nil)
		if err != nil {
			return nil
		}
		// target 可能已带 query（Get("/movie/1?append_to_response=...")），合并非覆盖
		if req.URL.RawQuery != "" {
			existing, _ := url.ParseQuery(req.URL.RawQuery)
			for key, values := range params {
				for _, v := range values {
					existing.Set(key, v)
				}
			}
			req.URL.RawQuery = existing.Encode()
		} else {
			req.URL.RawQuery = params.Encode()
		}
		req.Header.Set("Accept", "application/json")
		resp, err := c.hc.Do(req)
		if err != nil {
			log.Printf("TMDB 请求失败 [%s %d]: %v", path, attempt+1, redactURLError(err))
			if attempt < 2 {
				time.Sleep(2 * time.Second)
			}
			continue
		}
		raw, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			wait, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
			if wait <= 0 {
				wait = retryWaitSecs
			}
			log.Printf("TMDB 限流，等待 %ds...", wait)
			time.Sleep(time.Duration(wait) * time.Second)
			continue
		case resp.StatusCode == http.StatusNotFound:
			return nil
		case resp.StatusCode >= 400:
			log.Printf("TMDB 请求失败 [%s]: HTTP %d", path, resp.StatusCode)
			if attempt < 2 {
				time.Sleep(2 * time.Second)
			}
			continue
		}
		if readErr != nil {
			log.Printf("TMDB 读取响应失败 [%s]: %v", path, readErr)
			continue
		}
		var data map[string]any
		if err := decodeJSON(raw, &data); err != nil || data == nil {
			log.Printf("TMDB 响应解析失败 [%s]: %v", path, err)
			continue
		}
		if c.cache != nil && useCache {
			c.cache.Set(cachePath, data, c.language)
		}
		return data
	}
	if c.cache != nil && useCache {
		c.cache.SetFailed(cachePath, c.language)
	}
	return nil
}

// buildCachePath 与 Python 版缓存 key 规则一致，确保同一份 SQLite 可复用。
func buildCachePath(path string, extraParams map[string]string) string {
	if len(extraParams) == 0 {
		return path
	}
	keys := make([]string, 0, len(extraParams))
	for k := range extraParams {
		if k == "api_key" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, url.QueryEscape(k)+"="+url.QueryEscape(extraParams[k]))
	}
	if len(pairs) == 0 {
		return path
	}
	return path + "?" + strings.Join(pairs, "&")
}

// ── 便利工具 ────────────────────────────────────────────────

// ImageURL 将 TMDB 图片相对路径转为完整 URL；path 为空返回空串。
func ImageURL(path, size, baseURLOverride string) string {
	if path == "" {
		return ""
	}
	if size == "" {
		size = "original"
	}
	base := strings.TrimRight(strings.TrimSpace(baseURLOverride), "/")
	if base == "" {
		base = "https://image.tmdb.org"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + "/t/p/" + size + path
}

// imagePathRe 匹配图片 URL 中的 /t/p/{size}{path} 部分。
var imagePathRe = regexp.MustCompile(`/t/p/[^/]+(/.*)$`)

// ExtractImagePath 从完整图片 URL 提取相对路径；已是相对路径时原样返回。
func ExtractImagePath(value string) string {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "/") {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	candidate := parsed.Path
	if candidate == "" {
		candidate = raw
	}
	if m := imagePathRe.FindStringSubmatch(candidate); m != nil {
		return m[1]
	}
	if strings.HasPrefix(raw, "/") {
		return raw
	}
	return ""
}
