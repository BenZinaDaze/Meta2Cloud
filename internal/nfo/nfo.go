// Package nfo —— Plex / Infuse / Kodi 兼容 NFO 生成器（对应 Python 版 nfo/generator.py）。
//
// 生成类型：
//
//	电影  → <movie>           与视频文件同名
//	单集  → <episodedetails>  与视频文件同名，支持单集真实标题/简介
//	整剧  → <tvshow>          放在剧名文件夹根目录（tvshow.nfo）
//	季    → <season>          放在 Season 子文件夹（season.nfo）
package nfo

import (
	"fmt"
	"strconv"
	"strings"

	"meta2cloud/internal/jsonx"
	"meta2cloud/internal/mediaparser"
	"meta2cloud/internal/tmdb"
)

// tmdbPersonBase TMDB 人物页前缀。
const tmdbPersonBase = "https://www.themoviedb.org/person/"

// ── 轻量 XML 构建（支持 CDATA / 属性 / 缩进输出） ──────────

type xmlEl struct {
	tag      string
	text     string
	cdata    bool
	attrs    [][2]string
	children []*xmlEl
}

func el(tag string) *xmlEl { return &xmlEl{tag: tag} }

func (e *xmlEl) setAttr(k, v string) *xmlEl {
	e.attrs = append(e.attrs, [2]string{k, v})
	return e
}

func (e *xmlEl) add(child *xmlEl) *xmlEl {
	e.children = append(e.children, child)
	return e
}

// addText 添加文本子元素（text 空时跳过，与 Python _add 一致）。
func (e *xmlEl) addText(tag, text string) *xmlEl {
	if strings.TrimSpace(text) == "" {
		return e
	}
	return e.add(el(tag).withText(text))
}

func (e *xmlEl) withText(text string) *xmlEl {
	e.text = text
	return e
}

// addCDATA 添加带 CDATA 的子元素（空文本也保留，与 Python _add_cdata 一致）。
func (e *xmlEl) addCDATA(tag, text string) *xmlEl {
	child := el(tag)
	child.cdata = true
	child.text = text
	return e.add(child)
}

// xmlEscape 属性值转义（含引号）。
func xmlEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// xmlEscapeText 文本节点转义（与 minidom 一致：仅 & < >，引号原样保留）。
func xmlEscapeText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// render 递归渲染。CDATA 内容包含 ]]> 时拆分转义。
func (e *xmlEl) render(b *strings.Builder, depth int) {
	indent := strings.Repeat("  ", depth)
	b.WriteString(indent)
	b.WriteString("<" + e.tag)
	for _, attr := range e.attrs {
		b.WriteString(" " + attr[0] + "=\"" + xmlEscape(attr[1]) + "\"")
	}
	if len(e.children) == 0 && e.text == "" && !e.cdata {
		b.WriteString("/>\n")
		return
	}
	b.WriteString(">")
	if e.cdata {
		text := strings.ReplaceAll(e.text, "]]>", "]]]]><![CDATA[>")
		b.WriteString("<![CDATA[" + text + "]]>")
	} else if e.text != "" {
		b.WriteString(xmlEscapeText(e.text))
	}
	if len(e.children) > 0 {
		b.WriteString("\n")
		for _, child := range e.children {
			child.render(b, depth+1)
		}
		b.WriteString(indent)
	}
	b.WriteString("</" + e.tag + ">\n")
}

func (e *xmlEl) toXML() string {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>\n")
	e.render(&b, 0)
	return b.String()
}

// ── NfoGenerator ────────────────────────────────────────────

// Generator 将 TmdbClient 返回的字典转为 NFO XML 字符串。
type Generator struct {
	tmdbImageBaseURL string
}

// NewGenerator 构造。tmdbImageBaseURL 可为空（使用默认 image.tmdb.org）。
func NewGenerator(tmdbImageBaseURL string) *Generator {
	return &Generator{tmdbImageBaseURL: tmdbImageBaseURL}
}

// Generate 生成单集/电影 NFO XML。
// mediaType 未知时从 tmdb_info["media_type"] 读取；episodeDetail 为单集详情（TV 专用）。
func (g *Generator) Generate(info map[string]any, mtype mediaparser.MediaType, episodeDetail map[string]any) string {
	if mtype == mediaparser.MediaUnknown {
		mtype = mediaTypeFromStr(jsonx.Str(info, "media_type"))
	}
	if mtype == mediaparser.MediaTV {
		return g.buildEpisodeNFO(info, episodeDetail)
	}
	return g.buildMovieNFO(info)
}

func mediaTypeFromStr(s string) mediaparser.MediaType {
	switch s {
	case mediaparser.MediaTV.String():
		return mediaparser.MediaTV
	case mediaparser.MediaMovie.String():
		return mediaparser.MediaMovie
	default:
		return mediaparser.MediaUnknown
	}
}

// GenerateTVShow 生成 tvshow.nfo（<tvshow> 根标签）。
func (g *Generator) GenerateTVShow(info map[string]any) string {
	root := el("tvshow")
	title := firstNonEmpty2(jsonx.Str(info, "name"), jsonx.Str(info, "title"))
	origTitle := firstNonEmpty2(jsonx.Str(info, "original_name"), jsonx.Str(info, "original_title"))
	overview := jsonx.Str(info, "overview")

	root.addText("title", title)
	root.addText("originaltitle", origTitle)
	root.addText("showtitle", title)
	root.addText("year", extractYear(info, mediaparser.MediaTV))
	root.addText("premiered", jsonx.Str(info, "first_air_date"))
	root.addCDATA("plot", overview)
	root.addCDATA("outline", overview)
	root.addText("rating", formatNum(info["vote_average"]))
	root.addText("votes", formatNum(info["vote_count"]))

	// 整剧约定：-1 表示不指定（Kodi/NFO 规范）
	root.addText("season", "-1")
	root.addText("episode", "-1")

	// MPAA 内容分级
	for _, cr := range jsonx.MapSlice(jsonx.Map(info, "content_ratings"), "results") {
		if jsonx.Str(cr, "iso_3166_1") == "US" {
			root.addText("mpaa", jsonx.Str(cr, "rating"))
			break
		}
	}

	g.addIDsTV(root, info)

	for _, genre := range jsonx.MapSlice(info, "genres") {
		root.addText("genre", jsonx.Str(genre, "name"))
	}

	g.addCountries(root, info)
	g.addDirectors(root, peopleList(info["directors"]))
	g.addActors(root, peopleList(info["actors"]))
	g.addImages(root, info)

	return root.toXML()
}

// GenerateSeason 生成 season.nfo（<season> 根标签）。
func (g *Generator) GenerateSeason(seasonDetail map[string]any, seasonNumber int) string {
	root := el("season")
	overview := jsonx.Str(seasonDetail, "overview")
	airDate := jsonx.Str(seasonDetail, "air_date")
	title := jsonx.Str(seasonDetail, "name")
	if title == "" {
		title = fmt.Sprintf("Season %d", seasonNumber)
	}

	root.addCDATA("plot", overview)
	root.addCDATA("outline", overview)
	root.addText("title", title)
	root.addText("premiered", airDate)
	root.addText("releasedate", airDate)
	if airDate != "" {
		root.addText("year", airDate[:min(4, len(airDate))])
	}
	root.addText("seasonnumber", strconv.Itoa(seasonNumber))

	if poster := jsonx.Str(seasonDetail, "poster_path"); poster != "" {
		root.addText("thumb", tmdb.ImageURL(poster, "original", g.tmdbImageBaseURL))
	}
	return root.toXML()
}

// NfoNameFor 视频文件名 → 同名 NFO 文件名。
func NfoNameFor(videoFilename string) string {
	if i := strings.LastIndex(videoFilename, "."); i > 0 {
		return videoFilename[:i] + ".nfo"
	}
	return videoFilename + ".nfo"
}

// SeasonPosterName 返回 Plex/Infuse 标准的季封面文件名，如 season01-poster.jpg。
func SeasonPosterName(season int) string {
	if season == 0 {
		return "season-specials-poster.jpg"
	}
	return fmt.Sprintf("season%02d-poster.jpg", season)
}

// ── 电影 NFO ────────────────────────────────────────────────

func (g *Generator) buildMovieNFO(info map[string]any) string {
	root := el("movie")
	overview := jsonx.Str(info, "overview")

	root.addText("title", firstNonEmpty2(jsonx.Str(info, "title"), jsonx.Str(info, "name")))
	root.addText("originaltitle", firstNonEmpty2(jsonx.Str(info, "original_title"), jsonx.Str(info, "original_name")))
	root.addText("premiered", jsonx.Str(info, "release_date"))
	root.addText("year", extractYear(info, mediaparser.MediaMovie))
	root.addCDATA("plot", overview)
	root.addCDATA("outline", overview)
	root.addText("tagline", jsonx.Str(info, "tagline"))
	root.addText("rating", formatNum(info["vote_average"]))
	root.addText("votes", formatNum(info["vote_count"]))

	if runtime := formatNum(info["runtime"]); runtime != "" {
		root.addText("runtime", runtime)
	}

	// MPAA 内容分级
	for _, country := range jsonx.MapSlice(jsonx.Map(info, "release_dates"), "results") {
		if jsonx.Str(country, "iso_3166_1") != "US" {
			continue
		}
		for _, rel := range jsonx.MapSlice(country, "release_dates") {
			if cert := jsonx.Str(rel, "certification"); cert != "" {
				root.addText("mpaa", cert)
				break
			}
		}
		break
	}

	g.addIDsMovie(root, info)

	for _, genre := range jsonx.MapSlice(info, "genres") {
		root.addText("genre", jsonx.Str(genre, "name"))
	}
	for _, country := range jsonx.MapSlice(info, "production_countries") {
		root.addText("country", jsonx.Str(country, "name"))
	}
	g.addDirectors(root, peopleList(info["directors"]))
	g.addActors(root, peopleList(info["actors"]))
	g.addImages(root, info)
	return root.toXML()
}

// ── 单集 NFO ────────────────────────────────────────────────

func (g *Generator) buildEpisodeNFO(showInfo, ep map[string]any) string {
	root := el("episodedetails")
	if ep == nil {
		ep = map[string]any{}
	}
	showTitle := firstNonEmpty2(jsonx.Str(showInfo, "name"), jsonx.Str(showInfo, "title"))

	// 单集标题：优先真实单集名，没有时用「第N集」，避免与 showtitle 重复
	epTitle := jsonx.Str(ep, "name")
	if epTitle == "" {
		epNum := formatNum(showInfo["_episode"])
		if epNum == "" {
			epNum = formatNum(ep["episode_number"])
		}
		if epNum != "" && epNum != "0" {
			epTitle = "第" + epNum + "集"
		} else {
			epTitle = showTitle
		}
	}
	root.addText("showtitle", showTitle)
	root.addText("title", epTitle)
	root.addText("originaltitle", firstNonEmpty2(jsonx.Str(ep, "name"), jsonx.Str(showInfo, "original_name")))

	airDate := firstNonEmpty2(jsonx.Str(ep, "air_date"), jsonx.Str(showInfo, "first_air_date"))
	root.addText("aired", airDate)
	root.addText("premiered", airDate)
	if airDate != "" {
		root.addText("year", airDate[:min(4, len(airDate))])
	}

	// 评分：优先单集评分
	rating := formatNum(ep["vote_average"])
	if rating == "" {
		rating = formatNum(showInfo["vote_average"])
	}
	root.addText("rating", rating)

	epOverview := firstNonEmpty2(jsonx.Str(ep, "overview"), jsonx.Str(showInfo, "overview"))
	root.addCDATA("plot", epOverview)
	root.addCDATA("outline", epOverview)

	season := formatNum(showInfo["_season"])
	if season == "" {
		season = formatNum(ep["season_number"])
	}
	episode := formatNum(showInfo["_episode"])
	if episode == "" {
		episode = formatNum(ep["episode_number"])
	}
	if season != "" {
		root.addText("season", season)
	}
	if episode != "" {
		root.addText("episode", episode)
	}

	// IDs（单集用单集自己的 TMDB ID）
	epTmdbID := formatNum(ep["id"])
	showTmdbID := firstNonEmpty2(formatNum(showInfo["tmdb_id"]), formatNum(showInfo["id"]))
	extIDs := jsonx.Map(showInfo, "external_ids")
	tvdb := formatNum2(extIDs["tvdb_id"])

	if epTmdbID != "" {
		root.addText("tmdbid", epTmdbID)
		root.add(el("uniqueid").setAttr("type", "tmdb").setAttr("default", "true").withText(epTmdbID))
	} else if showTmdbID != "" {
		root.addText("tmdbid", showTmdbID)
		root.add(el("uniqueid").setAttr("type", "tmdb").setAttr("default", "true").withText(showTmdbID))
	}
	if tvdb != "" {
		root.addText("tvdbid", tvdb)
		root.add(el("uniqueid").setAttr("type", "tvdb").withText(tvdb))
	}

	for _, genre := range jsonx.MapSlice(showInfo, "genres") {
		root.addText("genre", jsonx.Str(genre, "name"))
	}
	g.addCountries(root, showInfo)

	// 导演：优先单集导演
	if len(ep) > 0 {
		var epDirectors []map[string]any
		for _, p := range jsonx.MapSlice(ep, "crew") {
			if jsonx.Str(p, "job") == "Director" {
				epDirectors = append(epDirectors, map[string]any{
					"id": p["id"], "name": p["name"], "profile_path": p["profile_path"],
				})
			}
		}
		g.addDirectors(root, append(epDirectors, peopleList(showInfo["directors"])...))
	} else {
		g.addDirectors(root, peopleList(showInfo["directors"]))
	}

	// 演员：单集客串演员 + 整剧主演
	if len(ep) > 0 {
		var guestStars []map[string]any
		for _, p := range jsonx.MapSlice(ep, "guest_stars") {
			if jsonx.Str(p, "known_for_department") == "Acting" {
				guestStars = append(guestStars, map[string]any{
					"id": p["id"], "name": p["name"], "character": p["character"], "profile_path": p["profile_path"],
				})
			}
		}
		g.addActors(root, append(guestStars, peopleList(showInfo["actors"])...))
	} else {
		g.addActors(root, peopleList(showInfo["actors"]))
	}

	// 封面：优先单集剧照，回退整剧 poster
	if still := jsonx.Str(ep, "still_path"); still != "" {
		root.addText("thumb", tmdb.ImageURL(still, "original", g.tmdbImageBaseURL))
	} else if poster := jsonx.Str(showInfo, "poster_path"); poster != "" {
		root.addText("thumb", tmdb.ImageURL(poster, "original", g.tmdbImageBaseURL))
	}
	return root.toXML()
}

// ── 共享辅助 ────────────────────────────────────────────────

// addIDsMovie 电影 ID 块：IMDB 有时优先 default=true。
func (g *Generator) addIDsMovie(root *xmlEl, info map[string]any) {
	tmdbID := firstNonEmpty2(formatNum(info["tmdb_id"]), formatNum(info["id"]))
	extIDs := jsonx.Map(info, "external_ids")
	imdb := firstNonEmpty2(formatNum2(extIDs["imdb_id"]), formatNum2(info["imdb_id"]))

	root.addText("tmdbid", tmdbID)
	if imdb != "" {
		root.addText("imdbid", imdb)
	}
	if tmdbID != "" {
		def := "true"
		if imdb != "" {
			def = "false"
		}
		root.add(el("uniqueid").setAttr("type", "tmdb").setAttr("default", def).withText(tmdbID))
	}
	if imdb != "" {
		root.add(el("uniqueid").setAttr("type", "imdb").setAttr("default", "true").withText(imdb))
	}
}

// addIDsTV 电视剧 ID 块。
func (g *Generator) addIDsTV(root *xmlEl, info map[string]any) {
	tmdbID := firstNonEmpty2(formatNum(info["tmdb_id"]), formatNum(info["id"]))
	extIDs := jsonx.Map(info, "external_ids")
	tvdb := formatNum2(extIDs["tvdb_id"])

	root.addText("tmdbid", tmdbID)
	if tvdb != "" {
		root.addText("tvdbid", tvdb)
	}
	if tmdbID != "" {
		root.add(el("uniqueid").setAttr("type", "tmdb").setAttr("default", "true").withText(tmdbID))
	}
	if tvdb != "" {
		root.add(el("uniqueid").setAttr("type", "tvdb").withText(tvdb))
	}
}

// peopleList 把 directors/actors 字段（内部构造的 []map 或 JSON []any）统一为 []map。
func peopleList(v any) []map[string]any {
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

func (g *Generator) addDirectors(root *xmlEl, directors []map[string]any) {
	for _, d := range directors {
		root.add(el("director").setAttr("tmdbid", formatNum2(d["id"])).withText(jsonx.Str(d, "name")))
	}
}

func (g *Generator) addActors(root *xmlEl, actors []map[string]any) {
	for _, actor := range actors {
		xactor := el("actor")
		xactor.addText("name", jsonx.Str(actor, "name"))
		xactor.addText("type", "Actor")
		xactor.addText("role", jsonx.Str(actor, "character"))
		xactor.addText("tmdbid", formatNum2(actor["id"]))
		if profile := jsonx.Str(actor, "profile_path"); profile != "" {
			xactor.addText("thumb", tmdb.ImageURL(profile, "original", g.tmdbImageBaseURL))
		}
		if id := formatNum2(actor["id"]); id != "" {
			xactor.addText("profile", tmdbPersonBase+id)
		}
		root.add(xactor)
	}
}

func (g *Generator) addCountries(root *xmlEl, info map[string]any) {
	countries := info["production_countries"]
	if countries == nil {
		countries = info["origin_country"]
	}
	switch list := countries.(type) {
	case []any:
		for _, c := range list {
			if s, ok := c.(string); ok {
				root.addText("country", s)
			} else if d, ok := c.(map[string]any); ok {
				root.addText("country", jsonx.Str(d, "name"))
			}
		}
	case []string:
		for _, s := range list {
			root.addText("country", s)
		}
	}
}

func (g *Generator) addImages(root *xmlEl, info map[string]any) {
	if poster := jsonx.Str(info, "poster_path"); poster != "" {
		root.addText("thumb", tmdb.ImageURL(poster, "original", g.tmdbImageBaseURL))
	}
	if backdrop := jsonx.Str(info, "backdrop_path"); backdrop != "" {
		fanart := el("fanart")
		fanart.addText("thumb", tmdb.ImageURL(backdrop, "original", g.tmdbImageBaseURL))
		root.add(fanart)
	}
}

func extractYear(info map[string]any, mtype mediaparser.MediaType) string {
	date := jsonx.Str(info, "release_date")
	if mtype != mediaparser.MediaMovie {
		date = jsonx.Str(info, "first_air_date")
	}
	if len(date) >= 4 {
		return date[:4]
	}
	return ""
}

// formatNum 数字值（float64/int）转字符串；nil/零值返回空串（对齐 Python `or ""` 语义）。
func formatNum(v any) string {
	return formatNumNonZero(v, true)
}

func formatNum2(v any) string {
	return formatNumNonZero(v, false)
}

func formatNumNonZero(v any, emptyOnZero bool) string {
	switch n := v.(type) {
	case float64:
		if n == 0 && emptyOnZero {
			return ""
		}
		return strconv.FormatFloat(n, 'f', -1, 64)
	case int:
		if n == 0 && emptyOnZero {
			return ""
		}
		return strconv.Itoa(n)
	case int64:
		if n == 0 && emptyOnZero {
			return ""
		}
		return strconv.FormatInt(n, 10)
	case string:
		return n
	}
	return ""
}

func firstNonEmpty2(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
