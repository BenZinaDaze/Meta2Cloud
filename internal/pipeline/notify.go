// notify.go —— Telegram 入库通知（数据采集、文案构建与发送）。
package pipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"

	"meta2cloud/internal/config"
	"meta2cloud/internal/jsonx"
	"meta2cloud/internal/strutil"
	"meta2cloud/internal/tmdb"
)

// redactURLError 去掉 *url.Error 携带的完整 URL（路径含 bot token），只保留底层错误，
// 防止请求失败时凭据随错误日志与上层调用方日志外泄。
func redactURLError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// notifyItem Telegram 入库通知条目。
type notifyItem struct {
	Title        string
	Year         string
	IsTV         bool
	Season       int
	Episode      *int
	EpTitle      string
	PosterPath   string // 竖版海报（无横版图时兜底）
	BackdropPath string // 横版背景图（通知图片首选）
	TmdbID       string
	NoTmdb       bool
	OriginalName string
	// 富通知字段（来自 tmdbInfo 与 STRM 钩子）
	Rating      float64
	Genres      []string
	Country     string
	SeasonTotal int
	Overview    string
	StrmOK      bool
}

// tmdbGenres 提取 TMDB 流派名列表，并判断是否为动画类型。
func tmdbGenres(info map[string]any) (names []string, animation bool) {
	genres, _ := info["genres"].([]any)
	for _, g := range genres {
		gm, ok := g.(map[string]any)
		if !ok {
			continue
		}
		name := jsonx.Str(gm, "name")
		if name == "" {
			continue
		}
		names = append(names, name)
		if name == "动画" || name == "Animation" {
			animation = true
		}
	}
	return names, animation
}

// mediaCountryMap 原始语言 → 国家/地区展示名。
var mediaCountryMap = map[string]string{
	"ja": "日本", "en": "美国", "ko": "韩国", "cn": "中国", "tw": "台湾",
	"hk": "香港", "fr": "法国", "gb": "英国", "th": "泰国", "es": "西班牙",
}

// mediaCountryLabel 语言与类型 → 展示标签（如「日番 · 日本」「电影 · 美国」）。
func mediaCountryLabel(lang string, isTV, animation bool) string {
	country := mediaCountryMap[lang]
	if country == "" {
		return ""
	}
	kind := "电影"
	if isTV {
		switch {
		case animation && lang == "ja":
			kind = "日番"
		case animation:
			kind = "动画剧集"
		default:
			kind = "剧集"
		}
	}
	return kind + " · " + country
}

// seasonEpisodeCount 某季的集数（来自季详情缓存）。
func (p *Pipeline) seasonEpisodeCount(tmdbIDStr string, season int) int {
	if tmdbIDStr == "" {
		return 0
	}
	sd := p.getSeasonDetailCached(tmdbIDStr, season)
	eps, _ := sd["episodes"].([]any)
	return len(eps)
}

// ── Telegram 通知 ───────────────────────────────────────────

func (p *Pipeline) sendNotifications() {
	tg := p.cfg.Telegram
	if tg.BotToken == "" || tg.ChatID == "" || len(p.notifyItems) == 0 {
		return
	}
	log.Printf("准备发送 TG 入库通知，共 %d 个条目", len(p.notifyItems))

	// 按 tmdb_id 分组（同一剧/电影合并为一条消息）
	type itemGroup struct {
		items []notifyItem
	}
	groups := map[string]*itemGroup{}
	var order []string
	posterMap := map[string]string{}
	for _, item := range p.notifyItems {
		key := item.TmdbID
		if key == "" {
			key = item.Title + "_" + item.Year
		}
		if _, ok := groups[key]; !ok {
			groups[key] = &itemGroup{}
			order = append(order, key)
		}
		groups[key].items = append(groups[key].items, item)
		if _, ok := posterMap[key]; !ok && item.PosterPath != "" {
			posterMap[key] = item.PosterPath
		}
	}

	for _, key := range order {
		items := groups[key].items
		first := items[0]
		// 通知图片优先横版背景图，竖版海报兜底
		poster := first.BackdropPath
		if poster == "" {
			poster = posterMap[key]
		}

		if first.NoTmdb {
			icon := "🎬"
			if first.IsTV {
				icon = "📺"
			}
			lines := []string{fmt.Sprintf("%s <b>TMDB 未找到元数据，文件未整理</b>", icon)}
			for _, it := range items {
				name := strutil.FirstNonEmpty(it.OriginalName, it.Title)
				lines = append(lines, fmt.Sprintf("• <code>%s</code>", name))
			}
			lines = append(lines, "\n请检查文件名后手动触发重新整理")
			p.sendTgPhoto(tg.BotToken, tg.ChatID, "", strings.Join(lines, "\n"))
			continue
		}

		p.sendTgPhoto(tg.BotToken, tg.ChatID, poster, p.buildNotifyCaption(first, items))
	}
}

// ── 通知文案 ────────────────────────────────────────────────

type notifySeasonSeg struct {
	Season int
	Range  string // "S01E01-E03" / "S01E01、E05" / "S01"
	Total  int    // 本季总集数
}

// seasonSegments 按季聚合集号并生成区间文案（季号升序，连续集号压缩为区间）。
func seasonSegments(items []notifyItem) []notifySeasonSeg {
	bySeason := map[int][]*notifyItem{}
	seasonTotal := map[int]int{}
	var seasons []int
	seen := map[int]bool{}
	for i := range items {
		it := &items[i]
		if !seen[it.Season] {
			seen[it.Season] = true
			seasons = append(seasons, it.Season)
		}
		bySeason[it.Season] = append(bySeason[it.Season], it)
		if it.SeasonTotal > seasonTotal[it.Season] {
			seasonTotal[it.Season] = it.SeasonTotal
		}
	}
	sort.Ints(seasons)

	segs := make([]notifySeasonSeg, 0, len(seasons))
	for _, s := range seasons {
		nums := make([]int, 0, len(bySeason[s]))
		hasEpisode := false
		for _, it := range bySeason[s] {
			if it.Episode != nil {
				nums = append(nums, *it.Episode)
				hasEpisode = true
			}
		}
		sort.Ints(nums)
		prefix := fmt.Sprintf("S%02d", s)
		rng := prefix
		if hasEpisode {
			var parts []string
			start, prev := nums[0], nums[0]
			appendPart := func(end int) {
				if start == end {
					parts = append(parts, fmt.Sprintf("E%02d", start))
				} else {
					parts = append(parts, fmt.Sprintf("E%02d-E%02d", start, end))
				}
			}
			for _, n := range nums[1:] {
				if n == prev+1 {
					prev = n
					continue
				}
				appendPart(prev)
				start, prev = n, n
			}
			appendPart(prev)
			rng = prefix + strings.Join(parts, "、")
		}
		segs = append(segs, notifySeasonSeg{Season: s, Range: rng, Total: seasonTotal[s]})
	}
	return segs
}

// hashtagOf 生成 Telegram 话题标签：去掉空白与全部标点/符号（含全角），
// 仅保留文字与数字，避免标签被标点截断。
func hashtagOf(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// buildNotifyCaption 构造富入库通知（HTML 格式）：
//
//	✅ 入库完成 · 标题 (2026)  S01E01-E02
//	📺 S01E01-E02 · 本季共 12 集
//	⭐ 7.7 · 日番 · 日本
//	📦 入库 2 个文件 · STRM 已生成
//	#标题 #动画 #喜剧
//	<blockquote>简介</blockquote>
func (p *Pipeline) buildNotifyCaption(first notifyItem, items []notifyItem) string {
	var b strings.Builder
	b.WriteString("✅ 入库完成 · " + html.EscapeString(first.Title))
	if first.Year != "" {
		b.WriteString(fmt.Sprintf(" (%s)", first.Year))
	}

	segs := seasonSegments(items)
	if first.IsTV && len(segs) > 0 {
		ranges := make([]string, 0, len(segs))
		for _, seg := range segs {
			ranges = append(ranges, seg.Range)
		}
		b.WriteString("  " + strings.Join(ranges, " ｜ "))
	}
	b.WriteString("\n")

	if first.IsTV {
		for _, seg := range segs {
			line := "📺 " + seg.Range
			if seg.Total > 0 {
				line += fmt.Sprintf(" · 本季共 %d 集", seg.Total)
			}
			b.WriteString(line + "\n")
		}
	}

	if first.Rating > 0 {
		line := fmt.Sprintf("⭐ %.1f", first.Rating)
		if first.Country != "" {
			line += " · " + html.EscapeString(first.Country)
		}
		b.WriteString(line + "\n")
	}

	strmOKCount := 0
	for _, it := range items {
		if it.StrmOK {
			strmOKCount++
		}
	}
	filesLine := fmt.Sprintf("📦 入库 %d 个文件", len(items))
	if p.onVideoIngested != nil {
		switch {
		case strmOKCount == len(items):
			filesLine += " · STRM 已生成"
		case strmOKCount > 0:
			filesLine += fmt.Sprintf(" · STRM %d/%d", strmOKCount, len(items))
		default:
			filesLine += " · STRM 未生成"
		}
	}
	b.WriteString(filesLine + "\n")

	var tags []string
	addTag := func(s string) {
		if s = hashtagOf(s); s != "" {
			tags = append(tags, "#"+s)
		}
	}
	addTag(first.Title)
	for _, g := range first.Genres {
		addTag(g)
	}
	if len(tags) > 1 {
		b.WriteString(strings.Join(tags, " ") + "\n")
	}

	// 分集子标题（可折叠引用，按季/集排序，无标题的集不列）
	if first.IsTV {
		sorted := make([]notifyItem, len(items))
		copy(sorted, items)
		sort.SliceStable(sorted, func(i, j int) bool {
			if sorted[i].Season != sorted[j].Season {
				return sorted[i].Season < sorted[j].Season
			}
			ei, ej := 0, 0
			if sorted[i].Episode != nil {
				ei = *sorted[i].Episode
			}
			if sorted[j].Episode != nil {
				ej = *sorted[j].Episode
			}
			return ei < ej
		})
		var epLines []string
		for _, it := range sorted {
			if it.EpTitle == "" {
				continue
			}
			label := fmt.Sprintf("S%02d", it.Season)
			if it.Episode != nil {
				label += fmt.Sprintf("E%02d", *it.Episode)
			}
			epLines = append(epLines, fmt.Sprintf("• %s  %s", label, html.EscapeString(it.EpTitle)))
		}
		if len(epLines) > 0 {
			b.WriteString("<blockquote expandable>" + strings.Join(epLines, "\n") + "</blockquote>\n")
		}
	}

	if first.Overview != "" {
		ov := strings.TrimSpace(first.Overview)
		b.WriteString("<blockquote expandable>" + html.EscapeString(ov) + "</blockquote>")
	}
	return strings.TrimRight(b.String(), "\n")
}

// ── 对外发送能力（供任意模块复用） ──────────────────────────

// SendTelegramMessage 用配置的 Bot 发送一条 HTML 文本通知。
// 与整理流程解耦，供 STRM 同步、订阅、定时任务等任意模块复用。
func SendTelegramMessage(tg config.Telegram, text string) error {
	if tg.BotToken == "" || tg.ChatID == "" {
		return fmt.Errorf("Telegram Bot Token 或 Chat ID 未配置")
	}
	return postTelegram(tg, "sendMessage", map[string]any{
		"chat_id":    tg.ChatID,
		"text":       text,
		"parse_mode": "HTML",
	}, 10)
}

// SendTelegramPhoto 用配置的 Bot 发送一条带图片的 HTML 通知。
// imageURL 为完整图片地址（如 TMDB 横版背景图），拉取失败时返回错误。
func SendTelegramPhoto(tg config.Telegram, imageURL, caption string) error {
	if tg.BotToken == "" || tg.ChatID == "" {
		return fmt.Errorf("Telegram Bot Token 或 Chat ID 未配置")
	}
	return postTelegram(tg, "sendPhoto", map[string]any{
		"chat_id":    tg.ChatID,
		"photo":      imageURL,
		"caption":    caption,
		"parse_mode": "HTML",
	}, 15)
}

// postTelegram 调用一个 Telegram Bot API 方法（失败时记日志并返回错误）。
func postTelegram(tg config.Telegram, method string, payload map[string]any, timeoutSecs int) error {
	resp, err := postJSON(fmt.Sprintf("https://api.telegram.org/bot%s/%s", tg.BotToken, method), payload, timeoutSecs)
	if err != nil {
		err = fmt.Errorf("请求异常：%w", redactURLError(err))
		log.Printf("Telegram %s %v", method, err)
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if json.Unmarshal(body, &result) == nil && !result.OK {
		err = fmt.Errorf("%s", strings.TrimSpace(string(body)))
		log.Printf("Telegram %s 发送失败：%s", method, string(body))
		return err
	}
	return nil
}

// sendTgPhoto 发送带海报/背景图的通知；图片发送失败时降级为纯文本，通知不丢。
func (p *Pipeline) sendTgPhoto(token, chatID, posterPath, caption string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Telegram 通知异常：%v", r)
		}
	}()
	tg := config.Telegram{BotToken: token, ChatID: chatID}
	if posterPath != "" {
		imageURL := tmdb.ImageURL(posterPath, "w780", p.cfg.TmdbImageBaseURL())
		if err := SendTelegramPhoto(tg, imageURL, caption); err == nil {
			return
		}
		log.Printf("图片通知发送失败，降级为纯文本通知")
	}
	if err := SendTelegramMessage(tg, caption); err != nil {
		log.Printf("纯文本通知发送失败：%v", err)
	}
}

func postJSON(url string, payload any, timeoutSecs int) (*http.Response, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: time.Duration(timeoutSecs) * time.Second}
	return client.Do(req)
}
