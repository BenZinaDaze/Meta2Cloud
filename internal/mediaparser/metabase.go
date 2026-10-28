// metabase.go —— 媒体元数据基类，存放所有解析结果字段及副标题中文格式解析。
package mediaparser

import (
	"strconv"
	"strings"

	"github.com/dlclark/regexp2"
)

// MetaBase 媒体信息基类（对应 Python 版 MetaBase dataclass）。
type MetaBase struct {
	IsFile         bool
	Title          string
	OrgString      string
	Subtitle       string
	Type           MediaType
	CnName         string
	EnName         string
	Year           string
	TotalSeason    int
	BeginSeason    *int
	EndSeason      *int
	TotalEpisode   int
	BeginEpisode   *int
	EndEpisode     *int
	Part           string
	ResourceType   string
	ResourceEffect string
	ResourcePix    string
	ResourceTeam   string
	Customization  string
	WebSource      string
	VideoEncode    string
	AudioEncode    string
	ApplyWords     []string
	TmdbID         int
	DoubanID       string
	FPS            *int

	// 副标题解析辅助
	subtitleFlag bool
}

// NewMetaBase 基础构造。
func NewMetaBase(title string, subtitle string, isfile bool) *MetaBase {
	m := &MetaBase{}
	if title == "" {
		return m
	}
	m.OrgString = strings.TrimSpace(title)
	if subtitle != "" {
		m.Subtitle = strings.TrimSpace(subtitle)
	}
	m.IsFile = isfile
	return m
}

// ── 名称属性 ────────────────────────────────────────────────

// Name 展示名：全中文中文名优先，其次英文名，再退中文名。
func (m *MetaBase) Name() string {
	if m.CnName != "" && IsAllChinese(m.CnName) {
		return m.CnName
	}
	if m.EnName != "" {
		return m.EnName
	}
	return m.CnName
}

// SetName 按语言设置名称（全中文 → cn_name，否则 en_name）。
func (m *MetaBase) SetName(name string) {
	if IsAllChinese(name) {
		m.CnName = name
	} else {
		m.EnName = name
		m.CnName = ""
	}
}

// ── 季属性 ──────────────────────────────────────────────────

func pad2(n int) string {
	s := strconv.Itoa(n)
	if len(s) < 2 {
		return "0" + s
	}
	return s
}

// Season 季字符串，如 "S01"、"S01-S02"。
func (m *MetaBase) Season() string {
	if m.BeginSeason != nil {
		if m.EndSeason == nil {
			return "S" + pad2(*m.BeginSeason)
		}
		return "S" + pad2(*m.BeginSeason) + "-S" + pad2(*m.EndSeason)
	}
	if m.Type == MediaTV {
		return "S01"
	}
	return ""
}

// Sea 有明确季号时返回 Season，否则空。
func (m *MetaBase) Sea() string {
	if m.BeginSeason != nil {
		return m.Season()
	}
	return ""
}

// SeasonSeq 季数字字符串。
func (m *MetaBase) SeasonSeq() string {
	if m.BeginSeason != nil {
		return strconv.Itoa(*m.BeginSeason)
	}
	if m.Type == MediaTV {
		return "1"
	}
	return ""
}

// SeasonList 季号列表。
func (m *MetaBase) SeasonList() []int {
	if m.BeginSeason == nil {
		if m.Type == MediaTV {
			return []int{1}
		}
		return nil
	}
	if m.EndSeason != nil {
		list := []int{}
		for i := *m.BeginSeason; i <= *m.EndSeason; i++ {
			list = append(list, i)
		}
		return list
	}
	return []int{*m.BeginSeason}
}

// ── 集属性 ──────────────────────────────────────────────────

// Episode 集字符串，如 "E03"、"E03-E05"。
func (m *MetaBase) Episode() string {
	if m.BeginEpisode != nil {
		if m.EndEpisode == nil {
			return "E" + pad2(*m.BeginEpisode)
		}
		return "E" + pad2(*m.BeginEpisode) + "-E" + pad2(*m.EndEpisode)
	}
	return ""
}

// EpisodeList 集号列表。
func (m *MetaBase) EpisodeList() []int {
	if m.BeginEpisode == nil {
		return nil
	}
	if m.EndEpisode != nil {
		list := []int{}
		for i := *m.BeginEpisode; i <= *m.EndEpisode; i++ {
			list = append(list, i)
		}
		return list
	}
	return []int{*m.BeginEpisode}
}

// Episodes 全部集号 "E03E04E05" 形式。
func (m *MetaBase) Episodes() string {
	eps := m.EpisodeList()
	parts := make([]string, 0, len(eps))
	for _, ep := range eps {
		parts = append(parts, pad2(ep))
	}
	return "E" + strings.Join(parts, "E")
}

// EpisodeSeqs 集号范围 "3" 或 "3-6"。
func (m *MetaBase) EpisodeSeqs() string {
	eps := m.EpisodeList()
	if len(eps) == 0 {
		return ""
	}
	if len(eps) == 1 {
		return strconv.Itoa(eps[0])
	}
	return strconv.Itoa(eps[0]) + "-" + strconv.Itoa(eps[len(eps)-1])
}

// EpisodeSeq 首集号字符串。
func (m *MetaBase) EpisodeSeq() string {
	eps := m.EpisodeList()
	if len(eps) == 0 {
		return ""
	}
	return strconv.Itoa(eps[0])
}

// SeasonEpisode "S01 E03" 形式。
func (m *MetaBase) SeasonEpisode() string {
	if m.Type != MediaTV {
		return ""
	}
	s, e := m.Season(), m.Episode()
	if s != "" && e != "" {
		return s + " " + e
	}
	return s + e
}

// ── 资源属性 ────────────────────────────────────────────────

// ResourceTerm 资源术语（类型+特效+分辨率）。
func (m *MetaBase) ResourceTerm() string {
	parts := []string{}
	for _, p := range []string{m.ResourceType, m.ResourceEffect, m.ResourcePix} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " ")
}

// Edition 版本（类型+特效）。
func (m *MetaBase) Edition() string {
	parts := []string{}
	for _, p := range []string{m.ResourceType, m.ResourceEffect} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " ")
}

// ReleaseGroup 字幕组。
func (m *MetaBase) ReleaseGroup() string { return m.ResourceTeam }

// VideoTerm 视频编码。
func (m *MetaBase) VideoTerm() string { return m.VideoEncode }

// AudioTerm 音频编码。
func (m *MetaBase) AudioTerm() string { return m.AudioEncode }

// FrameRate 帧率。
func (m *MetaBase) FrameRate() *int { return m.FPS }

// ── 包含判断 ────────────────────────────────────────────────

func toInt(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// IsInSeason 判断是否包含指定季（单值或列表）。
func (m *MetaBase) IsInSeason(seasons []int) bool {
	var metaS []int
	if m.EndSeason != nil && m.BeginSeason != nil {
		for i := *m.BeginSeason; i <= *m.EndSeason; i++ {
			metaS = append(metaS, i)
		}
	} else if m.BeginSeason != nil {
		metaS = []int{*m.BeginSeason}
	} else {
		metaS = []int{1}
	}
	set := map[int]bool{}
	for _, s := range metaS {
		set[s] = true
	}
	for _, s := range seasons {
		if !set[s] {
			return false
		}
	}
	return true
}

// IsInEpisode 判断是否包含指定集（单值或列表）。
func (m *MetaBase) IsInEpisode(episodes []int) bool {
	var metaE []int
	if m.EndEpisode != nil && m.BeginEpisode != nil {
		for i := *m.BeginEpisode; i <= *m.EndEpisode; i++ {
			metaE = append(metaE, i)
		}
	} else if m.BeginEpisode != nil {
		metaE = []int{*m.BeginEpisode}
	} else {
		return false
	}
	set := map[int]bool{}
	for _, e := range metaE {
		set[e] = true
	}
	for _, e := range episodes {
		if !set[e] {
			return false
		}
	}
	return true
}

// ── Setter ──────────────────────────────────────────────────

func intPtr(n int) *int { return &n }

// SetSeason 设置季（单值或首尾范围）。
func (m *MetaBase) SetSeason(sea []int) {
	if len(sea) == 0 {
		return
	}
	if len(sea) == 1 {
		m.BeginSeason = intPtr(sea[0])
		m.EndSeason = nil
	} else {
		m.BeginSeason = intPtr(sea[0])
		m.EndSeason = intPtr(sea[len(sea)-1])
	}
}

// SetEpisode 设置集（单值或首尾范围）。
func (m *MetaBase) SetEpisode(ep []int) {
	if len(ep) == 0 {
		return
	}
	if len(ep) == 1 {
		m.BeginEpisode = intPtr(ep[0])
		m.EndEpisode = nil
	} else {
		m.BeginEpisode = intPtr(ep[0])
		m.EndEpisode = intPtr(ep[len(ep)-1])
		m.TotalEpisode = *m.EndEpisode - *m.BeginEpisode + 1
	}
}

// SetEpisodes 按首尾设置集。
func (m *MetaBase) SetEpisodes(begin, end int) {
	if begin != 0 {
		m.BeginEpisode = intPtr(begin)
	}
	if end != 0 {
		m.EndEpisode = intPtr(end)
	}
	if m.BeginEpisode != nil && m.EndEpisode != nil {
		m.TotalEpisode = *m.EndEpisode - *m.BeginEpisode + 1
	}
}

// ── Merge ───────────────────────────────────────────────────

// Merge 用另一份元数据补齐空缺字段。
func (m *MetaBase) Merge(o *MetaBase) {
	if m.Type == MediaUnknown && o.Type != MediaUnknown {
		m.Type = o.Type
	}
	if m.Name() == "" {
		m.CnName = o.CnName
		m.EnName = o.EnName
	}
	if m.Year == "" {
		m.Year = o.Year
	}
	if m.Type == MediaTV && m.BeginSeason == nil {
		m.BeginSeason = o.BeginSeason
		m.EndSeason = o.EndSeason
		m.TotalSeason = o.TotalSeason
	}
	if m.Type == MediaTV && m.BeginEpisode == nil {
		m.BeginEpisode = o.BeginEpisode
		m.EndEpisode = o.EndEpisode
		m.TotalEpisode = o.TotalEpisode
	}
	if m.ResourceType == "" {
		m.ResourceType = o.ResourceType
	}
	if m.ResourcePix == "" {
		m.ResourcePix = o.ResourcePix
	}
	if m.ResourceTeam == "" {
		m.ResourceTeam = o.ResourceTeam
	}
	if m.Customization == "" {
		m.Customization = o.Customization
	}
	if m.ResourceEffect == "" {
		m.ResourceEffect = o.ResourceEffect
	}
	if m.VideoEncode == "" {
		m.VideoEncode = o.VideoEncode
	}
	if m.AudioEncode == "" {
		m.AudioEncode = o.AudioEncode
	}
	if m.FPS == nil {
		m.FPS = o.FPS
	}
	if m.Part == "" {
		m.Part = o.Part
	}
	if m.TmdbID == 0 && o.TmdbID != 0 {
		m.TmdbID = o.TmdbID
	}
	if m.DoubanID == "" && o.DoubanID != "" {
		m.DoubanID = o.DoubanID
	}
}

// ── 副标题中文格式解析 ──────────────────────────────────────

// 副标题解析正则（含 lookaround，使用 regexp2）。
var (
	titleEpisodeRe = regexp2.MustCompile(`Episode\s+(\d{1,4})`, regexp2.IgnoreCase)
	//nolint:unused // 与 Python 版字段对齐保留
	subtitleSeasonRe = regexp2.MustCompile(`(?<!(?:不包含|不包括|不含)\s*)(?<![全共]\s*)[第\s]+([0-9一二三四五六七八九十S\-]+)\s*季(?!\s*[全共])`, regexp2.IgnoreCase)
	//nolint:unused
	subtitleSeasonAllRe = regexp2.MustCompile(`[全共]\s*([0-9一二三四五六七八九十]+)\s*季`, regexp2.IgnoreCase)
	//nolint:unused
	subtitleEpisodeRe = regexp2.MustCompile(`(?<!(?:不包含|不包括|不含)\s*)(?<![全共]\s*)[第\s]+([0-9一二三四五六七八九十百零EP]+)\s*[集话話期幕](?!\s*[全共])`, regexp2.IgnoreCase)
	//nolint:unused
	subtitleEpisodeBetweenRe = regexp2.MustCompile(`[第]*\s*([0-9一二三四五六七八九十百零]+)\s*[集话話期幕]?\s*-\s*第*\s*([0-9一二三四五六七八九十百零]+)\s*[集话話期幕]`, regexp2.IgnoreCase)
	//nolint:unused
	subtitleEpisodeAllRe = regexp2.MustCompile(`([0-9一二三四五六七八九十百零]+)\s*集\s*全|[全共]\s*([0-9一二三四五六七八九十百零]+)\s*[集话話期幕]`, regexp2.IgnoreCase)
	// 结尾分支显式区分有无右方括号，避免可选括号回溯后绕过数字后缀边界
	subtitleEpisodeRangeFinRe = regexp2.MustCompile(`(?<!\d)\[?\s*(\d{1,4})\s*-\s*(\d{1,4})\s*(?:(?:Fin|End)(?![a-z0-9])|完结(?![一-鿿]))(?:\s*\](?!\d)|(?!\s*(?:\]\d|\d))\s*)`, regexp2.IgnoreCase)
	//nolint:unused
	subtitleKeywordRe = regexp2.MustCompile(`[全第季集话話期幕]`, regexp2.IgnoreCase)
)

func firstGroup(m *regexp2.Match) string {
	if m == nil {
		return ""
	}
	g := m.GroupByNumber(1)
	if g == nil {
		return ""
	}
	return g.String()
}

// cnNumberToInt 中文/阿拉伯混合数字解析，失败返回 0, false。
// 与 Python 版 cn2an(..., mode="smart") + int() 行为对齐：解析失败即中止。
func cnNumberToInt(s string) (int, bool) {
	n, ok := Cn2AnSmart(s)
	if !ok {
		return 0, false
	}
	return int(n), true
}

// InitSubtitle 解析副标题中的中文季集信息。
func (m *MetaBase) InitSubtitle(titleText string) {
	if titleText == "" {
		return
	}
	titleText = " " + titleText + " "

	if mt, _ := titleEpisodeRe.FindStringMatch(titleText); mt != nil {
		episode, err := strconv.Atoi(firstGroup(mt))
		if err != nil {
			return
		}
		if episode >= 10000 {
			return
		}
		if m.BeginEpisode == nil {
			m.BeginEpisode = intPtr(episode)
			m.TotalEpisode = 1
		}
		m.Type = MediaTV
		m.subtitleFlag = true
		return
	}

	if ok, _ := subtitleKeywordRe.MatchString(titleText); !ok {
		// 副标题无中文季集标记时，仍识别 01-26Fin 等数字范围+完结标记
		m.initEpisodeRangeFin(titleText)
		return
	}

	// 全x季
	if mt, _ := subtitleSeasonAllRe.FindStringMatch(titleText); mt != nil {
		seasonAll := firstGroup(mt)
		if seasonAll != "" && m.BeginSeason == nil && m.BeginEpisode == nil {
			total, ok := cnNumberToInt(strings.TrimSpace(seasonAll))
			if !ok {
				return
			}
			m.TotalSeason = total
			m.BeginSeason = intPtr(1)
			m.EndSeason = intPtr(m.TotalSeason)
			m.Type = MediaTV
			m.subtitleFlag = true
		}
		return
	}

	// 第x季
	if mt, _ := subtitleSeasonRe.FindStringMatch(titleText); mt != nil {
		seasons := firstGroup(mt)
		if seasons == "" {
			return
		}
		seasons = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(seasons), "S", ""))
		var beginSeason, endSeason int
		hasEnd := false
		parts := strings.Split(seasons, "-")
		n, ok := cnNumberToInt(strings.TrimSpace(parts[0]))
		if !ok {
			return
		}
		beginSeason = n
		if len(parts) > 1 {
			n2, ok := cnNumberToInt(strings.TrimSpace(parts[1]))
			if !ok {
				return
			}
			endSeason = n2
			hasEnd = true
		}
		if beginSeason > 100 || (hasEnd && endSeason > 100) {
			return
		}
		if m.BeginSeason == nil && beginSeason != 0 {
			m.BeginSeason = intPtr(beginSeason)
			m.TotalSeason = 1
		}
		if hasEnd && m.BeginSeason != nil && m.EndSeason == nil && endSeason != *m.BeginSeason {
			m.EndSeason = intPtr(endSeason)
			m.TotalSeason = *m.EndSeason - *m.BeginSeason + 1
		}
		m.Type = MediaTV
		m.subtitleFlag = true
	}

	// 第x-x集
	if mt, _ := subtitleEpisodeBetweenRe.FindStringMatch(titleText); mt != nil {
		g1 := firstGroup(mt)
		g2 := ""
		if g := mt.GroupByNumber(2); g != nil {
			g2 = g.String()
		}
		beginEp, ok := cnNumberToInt(strings.TrimSpace(g1))
		if !ok {
			return
		}
		endEp, ok := cnNumberToInt(strings.TrimSpace(g2))
		if !ok {
			return
		}
		if beginEp >= 10000 || endEp >= 10000 {
			return
		}
		if m.BeginEpisode == nil && beginEp != 0 {
			m.BeginEpisode = intPtr(beginEp)
			m.TotalEpisode = 1
		}
		if m.BeginEpisode != nil && m.EndEpisode == nil && endEp != *m.BeginEpisode {
			m.EndEpisode = intPtr(endEp)
			m.TotalEpisode = *m.EndEpisode - *m.BeginEpisode + 1
		}
		m.Type = MediaTV
		m.subtitleFlag = true
		return
	}

	// 第x集
	if mt, _ := subtitleEpisodeRe.FindStringMatch(titleText); mt != nil {
		episodes := firstGroup(mt)
		if episodes == "" {
			return
		}
		episodes = strings.ToUpper(episodes)
		episodes = strings.ReplaceAll(episodes, "E", "")
		episodes = strings.ReplaceAll(strings.TrimSpace(episodes), "P", "")
		var beginEp, endEp int
		hasEnd := false
		parts := strings.Split(episodes, "-")
		n, ok := cnNumberToInt(strings.TrimSpace(parts[0]))
		if !ok {
			return
		}
		beginEp = n
		if len(parts) > 1 {
			n2, ok := cnNumberToInt(strings.TrimSpace(parts[1]))
			if !ok {
				return
			}
			endEp = n2
			hasEnd = true
		}
		if beginEp >= 10000 || (hasEnd && endEp >= 10000) {
			return
		}
		if m.BeginEpisode == nil && beginEp != 0 {
			m.BeginEpisode = intPtr(beginEp)
			m.TotalEpisode = 1
		}
		if hasEnd && m.BeginEpisode != nil && m.EndEpisode == nil && endEp != *m.BeginEpisode {
			m.EndEpisode = intPtr(endEp)
			m.TotalEpisode = *m.EndEpisode - *m.BeginEpisode + 1
		}
		m.Type = MediaTV
		m.subtitleFlag = true
		return
	}

	// x集全
	if mt, _ := subtitleEpisodeAllRe.FindStringMatch(titleText); mt != nil {
		episodeAll := firstGroup(mt)
		if episodeAll == "" {
			if g := mt.GroupByNumber(2); g != nil {
				episodeAll = g.String()
			}
		}
		if episodeAll != "" && m.BeginEpisode == nil {
			total, ok := cnNumberToInt(strings.TrimSpace(episodeAll))
			if !ok {
				return
			}
			m.TotalEpisode = total
			m.Type = MediaTV
			m.subtitleFlag = true
		}
	}

	// 01-26Fin 等数字范围+完结标记
	m.initEpisodeRangeFin(titleText)
}

// initEpisodeRangeFin 识别 01-26Fin / [01-38 END] 等"数字范围+完结标记"格式的集数信息。
func (m *MetaBase) initEpisodeRangeFin(titleText string) {
	mt, _ := subtitleEpisodeRangeFinRe.FindStringMatch(titleText)
	if mt == nil {
		return
	}
	beginE, err1 := strconv.Atoi(mt.GroupByNumber(1).String())
	endE, err2 := strconv.Atoi(mt.GroupByNumber(2).String())
	if err1 != nil || err2 != nil {
		return
	}
	if beginE < 1 || beginE > endE || endE >= 10000 {
		return
	}
	// 两个数字都落在常见年份区间时视为年份范围而非集数（如 2019-2020完结）
	if beginE >= 1900 && endE <= 2155 {
		return
	}
	if m.BeginEpisode == nil {
		m.BeginEpisode = intPtr(beginE)
		m.EndEpisode = intPtr(endE)
		m.TotalEpisode = endE
		m.Type = MediaTV
		m.subtitleFlag = true
	}
}
