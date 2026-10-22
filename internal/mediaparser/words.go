// words.go —— 自定义识别词预处理（屏蔽词/替换词/集偏移，对齐 MoviePilot v3 words.py）。
package mediaparser

import (
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/dlclark/regexp2"
)

// WordsMatcher 自定义识别词处理。
// 词条格式：
//   - 注释：    以 # 开头的行跳过
//   - 屏蔽词：  keyword（正则优先，编译失败退回字面量）
//   - 替换词：  old => new（old 为正则；替换串兼容 Python 风格 \1 与 Go 风格 $1 捕获组）
//   - 集偏移：  前定位 <> 后定位 >> 表达式（EP 为集数占位符，支持 + - * / % 与括号，
//     集数 token 含中文数字；偏移保持前导零宽度，中文集数偏移后仍转回中文）
//   - 组合：    old => new && 前定位 <> 后定位 >> 表达式（替换命中后才执行偏移）
type WordsMatcher struct {
	wordList []string
}

// NewWordsMatcher 构造。customWords 每行一个词条。
func NewWordsMatcher(customWords []string) *WordsMatcher {
	words := []string{}
	for _, w := range customWords {
		w = strings.TrimSpace(w)
		if w != "" {
			words = append(words, w)
		}
	}
	return &WordsMatcher{wordList: words}
}

// Prepare 预处理标题。返回 (处理后的标题, 命中的词条列表)。
func (wm *WordsMatcher) Prepare(title string) (string, []string) {
	newTitle, _, applied := wm.PrepareWithSubtitle(title, "")
	return newTitle, applied
}

// PrepareWithSubtitle 预处理标题与副标题：集偏移在标题未命中时可跨字段作用于副标题。
func (wm *WordsMatcher) PrepareWithSubtitle(title, subtitle string) (string, string, []string) {
	if title == "" || len(wm.wordList) == 0 {
		return title, subtitle, nil
	}
	applied := []string{}
	for _, word := range wm.wordList {
		w := strings.TrimSpace(word)
		if w == "" || strings.HasPrefix(w, "#") {
			continue
		}
		switch {
		case strings.Contains(w, "&&"):
			// 组合：替换 + 偏移（替换命中后才执行偏移，与 Python 版一致）
			parts := strings.SplitN(w, "&&", 2)
			newTitle, hit := applyReplace(title, strings.TrimSpace(parts[0]))
			if !hit {
				continue
			}
			title = newTitle
			newTitle, newSubtitle, hit := applyOffsetWithSubtitle(title, subtitle, strings.TrimSpace(parts[1]))
			if !hit {
				applied = append(applied, w)
				continue
			}
			title, subtitle = newTitle, newSubtitle
			applied = append(applied, w)
		case strings.Contains(w, "=>"):
			newTitle, hit := applyReplace(title, w)
			if hit {
				applied = append(applied, w)
				title = newTitle
			}
		case strings.Contains(w, "<>") && strings.Contains(w, ">>"):
			newTitle, newSubtitle, hit := applyOffsetWithSubtitle(title, subtitle, w)
			if hit {
				applied = append(applied, w)
				title, subtitle = newTitle, newSubtitle
			}
		default:
			// 屏蔽词
			newTitle, hit := applyBlock(title, w)
			if hit {
				applied = append(applied, w)
				title = newTitle
			}
		}
	}
	return strings.TrimSpace(title), subtitle, applied
}

// ── 正则编译缓存（同一规则在批量识别链路中反复使用） ──────────

var (
	stdReCache sync.Map // string -> *regexp.Regexp
	re2ReCache sync.Map // string -> *regexp2.Regexp
)

func compileStdCache(pattern string) (*regexp.Regexp, error) {
	if v, ok := stdReCache.Load(pattern); ok {
		return v.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	stdReCache.Store(pattern, re)
	return re, nil
}

func compileRe2Cache(pattern string) (*regexp2.Regexp, error) {
	if v, ok := re2ReCache.Load(pattern); ok {
		return v.(*regexp2.Regexp), nil
	}
	re, err := regexp2.Compile(pattern, regexp2.IgnoreCase)
	if err != nil {
		return nil, err
	}
	re2ReCache.Store(pattern, re)
	return re, nil
}

// findAllRe2Matches 迭代取全部匹配（regexp2 无 FindAllStringMatch）。
func findAllRe2Matches(re *regexp2.Regexp, s string) ([]*regexp2.Match, error) {
	var out []*regexp2.Match
	m, err := re.FindStringMatch(s)
	if err != nil {
		return nil, err
	}
	for m != nil {
		out = append(out, m)
		m, err = re.FindNextMatch(m)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

// ── 替换词 / 屏蔽词 ─────────────────────────────────────────

var backrefRe = regexp.MustCompile(`\\(\d+)`)

// applyReplace 应用替换词规则（old 为正则，失败退回字面量替换）。
func applyReplace(title, word string) (string, bool) {
	parts := strings.SplitN(word, "=>", 2)
	if len(parts) != 2 {
		return title, false
	}
	oldW := strings.TrimSpace(parts[0])
	newW := strings.TrimSpace(parts[1])
	if oldW == "" {
		return title, false
	}
	re, err := compileStdCache("(?i)" + oldW)
	var newTitle string
	if err != nil {
		newTitle = strings.ReplaceAll(title, oldW, newW)
	} else {
		// 兼容 Python 风格 \1 捕获组引用（用 ${1} 界定组名，避免与后续字符粘连）
		newTitle = re.ReplaceAllString(title, backrefRe.ReplaceAllString(newW, "$${1}"))
	}
	return newTitle, newTitle != title
}

// applyBlock 屏蔽词：正则优先（与替换词同策略），编译失败退回字面量。
func applyBlock(title, word string) (string, bool) {
	re, err := compileStdCache("(?i)" + word)
	if err != nil {
		re, err = regexp.Compile("(?i)" + regexp.QuoteMeta(word))
		if err != nil {
			return title, false
		}
	}
	newTitle := re.ReplaceAllString(title, "")
	return newTitle, newTitle != title
}

// ── 集偏移 ──────────────────────────────────────────────────

// applyOffsetWithSubtitle 集偏移：先在标题中定位；未命中时尝试副标题，
// 前后定位词分跨标题与副标题时只偏移副标题中的集数表达（避开季数/年份）。
func applyOffsetWithSubtitle(title, subtitle, word string) (string, string, bool) {
	front, back, offset, ok := parseOffsetWord(word)
	if !ok {
		return title, subtitle, false
	}
	newTitle, hit := applyOffset(title, front, back, offset)
	if hit {
		return newTitle, subtitle, true
	}
	if subtitle == "" {
		return title, subtitle, false
	}
	newSubtitle, hit := applyOffset(subtitle, front, back, offset)
	if hit {
		return title, newSubtitle, true
	}
	if !locatorsMatch(title+" "+subtitle, front, back) {
		return title, subtitle, false
	}
	newSubtitle, hit = applyOffsetSubtitleSpans(subtitle, offset)
	if hit {
		return title, newSubtitle, true
	}
	return title, subtitle, false
}

// parseOffsetWord 拆分「前定位 <> 后定位 >> 偏移表达式」。
func parseOffsetWord(word string) (front, back, offset string, ok bool) {
	idx := strings.Index(word, "<>")
	if idx < 0 {
		return "", "", "", false
	}
	rest := word[idx+2:]
	gt := strings.Index(rest, ">>")
	if gt < 0 {
		return "", "", "", false
	}
	front = strings.TrimSpace(word[:idx])
	back = strings.TrimSpace(rest[:gt])
	offset = strings.TrimSpace(rest[gt+2:])
	return front, back, offset, offset != ""
}

// locatorsMatch 判断前后定位词是否在文本中同时出现。
func locatorsMatch(text, front, back string) bool {
	if front != "" {
		re, err := compileStdCache(front)
		if err != nil || !re.MatchString(text) {
			return false
		}
	}
	if back != "" {
		re, err := compileStdCache(back)
		if err != nil || !re.MatchString(text) {
			return false
		}
	}
	return true
}

// episodeTokenClass 集数 token：阿拉伯数字与常见中文数字。
const episodeTokenClass = `[0-9一二三四五六七八九十百零]+`

// applyOffset 定位词之间的集数整体偏移（对齐 Python 版 __episode_offset：
// 多集数按偏移方向排序替换，避免相邻数字互相干扰）。
func applyOffset(text, front, back, offset string) (string, bool) {
	if !locatorsMatch(text, front, back) {
		return text, false
	}
	tokRe, err := compileRe2Cache(`(?<=(?:` + front + `).*?)` + episodeTokenClass + `(?=.*?(?:` + back + `))`)
	if err != nil {
		return text, false
	}
	matches, err := findAllRe2Matches(tokRe, text)
	if err != nil || len(matches) == 0 {
		return text, false
	}
	type pair struct {
		orig, new string
		val       int
	}
	var pairs []pair
	shrink := false
	for _, m := range matches {
		orig := m.String()
		n, ok := Cn2AnSmart(orig)
		if !ok {
			continue
		}
		val, err := evalEpisodeOffset(offset, int(n))
		if err != nil {
			return text, false
		}
		switch {
		case int(n) > val:
			shrink = true
		case int(n) < val:
			shrink = false
		}
		pairs = append(pairs, pair{orig: orig, new: formatEpisodeOffset(orig, val), val: val})
	}
	if len(pairs) == 0 {
		return text, false
	}
	// 集数向前偏移（变小）按升序处理，向后偏移按降序处理
	if shrink {
		sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].val < pairs[j].val })
	} else {
		sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].val > pairs[j].val })
	}
	for _, p := range pairs {
		repRe, err := compileRe2Cache(`(?<=(?:` + front + `).*?)` + regexp2.Escape(p.orig) + `(?=.*?(?:` + back + `))`)
		if err != nil {
			continue
		}
		text, _ = repRe.Replace(text, p.new, -1, -1)
	}
	return text, true
}

// ── 副标题集数表达偏移（对齐 Python 版 __episode_offset_subtitle） ──

var (
	// 与 metabase subtitleEpisodeRangeFinRe 同源，但提取首尾两个集数 span
	subEpRangeBeginRe = regexp2.MustCompile(`(?<!\d)\[?\s*(\d{1,4})\s*-\s*(\d{1,4})\s*(?:(?:Fin|End)(?![a-z0-9])|完结(?![一-鿿]))(?:\s*\](?!\d)|(?!\s*(?:\]\d|\d))\s*)`, regexp2.IgnoreCase)
	subEpRe           = regexp2.MustCompile(`(?<![全共])([0-9一二三四五六七八九十百零]+)\s*[集话話期幕](?!\s*[全共])`, regexp2.IgnoreCase)
	subEpTitleRe      = regexp2.MustCompile(`(?<![A-Za-z0-9_])Episode\s+(\d{1,4})(?![A-Za-z0-9_])`, regexp2.IgnoreCase)
	subEpTokenRe      = regexp2.MustCompile(`(?<![A-Za-z0-9_])(?:EP|E)(\d{1,4})(?![A-Za-z0-9_])`, regexp2.IgnoreCase)
)

type textSpan struct{ start, end int }

// subtitleEpisodeSpans 提取副标题中会被识别为集数的数字范围。
// 注意：regexp2 的 Match.Index 为 rune 下标，返回的 span 一律按 rune 计。
func subtitleEpisodeSpans(subtitle string) []textSpan {
	var spans []textSpan
	add := func(start, end int) {
		if start < 0 || end <= start {
			return
		}
		for _, old := range spans {
			if start < old.end && end > old.start {
				return
			}
		}
		spans = append(spans, textSpan{start, end})
	}
	if ms, _ := findAllRe2Matches(subEpRangeBeginRe, subtitle); ms != nil {
		for _, m := range ms {
			g1, g2 := m.GroupByNumber(1), m.GroupByNumber(2)
			if g1 != nil && g2 != nil {
				add(g1.Index, g1.Index+g1.Length)
				add(g2.Index, g2.Index+g2.Length)
			}
		}
	}
	for _, re := range []*regexp2.Regexp{subEpRe, subEpTitleRe, subEpTokenRe} {
		if ms, _ := findAllRe2Matches(re, subtitle); ms != nil {
			for _, m := range ms {
				g := m.GroupByNumber(1)
				if g != nil {
					add(g.Index, g.Index+g.Length)
				}
			}
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	return spans
}

// applyOffsetSubtitleSpans 只偏移副标题中的集数表达，避免修改副标题里的季数或年份。
func applyOffsetSubtitleSpans(subtitle, offset string) (string, bool) {
	spans := subtitleEpisodeSpans(subtitle)
	if len(spans) == 0 {
		return subtitle, false
	}
	runes := []rune(subtitle)
	type repl struct {
		start, end int
		text       string
	}
	var replacements []repl
	for _, s := range spans {
		if s.end > len(runes) {
			continue
		}
		orig := string(runes[s.start:s.end])
		n, ok := Cn2AnSmart(orig)
		if !ok {
			continue
		}
		val, err := evalEpisodeOffset(offset, int(n))
		if err != nil {
			return subtitle, false
		}
		replacements = append(replacements, repl{s.start, s.end, formatEpisodeOffset(orig, val)})
	}
	if len(replacements) == 0 {
		return subtitle, false
	}
	for i := len(replacements) - 1; i >= 0; i-- {
		r := replacements[i]
		runes = append(runes[:r.start], append([]rune(r.text), runes[r.end:]...)...)
	}
	return string(runes), true
}

// ── 偏移表达式求值（白名单算术语法） ────────────────────────

var (
	epTokenRe     = regexp2.MustCompile(`(?<![A-Za-z0-9_])EP(?![A-Za-z0-9_])`, regexp2.None)
	epImplicitRe  = regexp.MustCompile(`(?:\d|\))\s*EP|EP\s*(?:\d|\()`)
	leadingZeroRe = regexp.MustCompile(`^0+`)
	asciiDigitsRe = regexp.MustCompile(`^\d+$`)
)

// evalEpisodeOffset 计算集数偏移表达式（EP 替换为集数后按 + - * / % 求值）。
func evalEpisodeOffset(expr string, episode int) (int, error) {
	if epImplicitRe.MatchString(expr) {
		return 0, errors.New("EP 表达式不支持省略运算符")
	}
	if m, err := epTokenRe.FindStringMatch(expr); err == nil && m != nil {
		expr, _ = epTokenRe.Replace(expr, strconv.Itoa(episode), -1, -1)
	} else if strings.Contains(strings.ToUpper(expr), "EP") {
		return 0, errors.New("EP 占位符格式不正确")
	}
	p := &offsetParser{src: expr}
	v, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	p.skipWS()
	if p.i != len(p.src) {
		return 0, errors.New("偏移表达式存在多余字符")
	}
	return int(v), nil
}

type offsetParser struct {
	src string
	i   int
}

func (p *offsetParser) skipWS() {
	for p.i < len(p.src) && (p.src[p.i] == ' ' || p.src[p.i] == '\t') {
		p.i++
	}
}

func (p *offsetParser) parseExpr() (float64, error) {
	left, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		p.skipWS()
		if p.i < len(p.src) && (p.src[p.i] == '+' || p.src[p.i] == '-') {
			op := p.src[p.i]
			p.i++
			right, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			if op == '+' {
				left += right
			} else {
				left -= right
			}
			continue
		}
		return left, nil
	}
}

func (p *offsetParser) parseTerm() (float64, error) {
	left, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for {
		p.skipWS()
		if p.i < len(p.src) && (p.src[p.i] == '*' || p.src[p.i] == '/' || p.src[p.i] == '%') {
			op := p.src[p.i]
			p.i++
			right, err := p.parseFactor()
			if err != nil {
				return 0, err
			}
			switch op {
			case '*':
				left *= right
			case '/':
				if right == 0 {
					return 0, errors.New("偏移表达式除零")
				}
				left /= right
			case '%':
				if right == 0 {
					return 0, errors.New("偏移表达式除零")
				}
				left = float64(int64(left) % int64(right))
			}
			continue
		}
		return left, nil
	}
}

func (p *offsetParser) parseFactor() (float64, error) {
	p.skipWS()
	if p.i < len(p.src) && (p.src[p.i] == '+' || p.src[p.i] == '-') {
		op := p.src[p.i]
		p.i++
		v, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		if op == '-' {
			v = -v
		}
		return v, nil
	}
	return p.parseAtom()
}

func (p *offsetParser) parseAtom() (float64, error) {
	p.skipWS()
	if p.i < len(p.src) && p.src[p.i] == '(' {
		p.i++
		v, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		p.skipWS()
		if p.i >= len(p.src) || p.src[p.i] != ')' {
			return 0, errors.New("偏移表达式括号不匹配")
		}
		p.i++
		return v, nil
	}
	start := p.i
	for p.i < len(p.src) && p.src[p.i] >= '0' && p.src[p.i] <= '9' {
		p.i++
	}
	if p.i == start {
		return 0, errors.New("偏移表达式仅支持数字、EP、括号和基础算术运算符")
	}
	return strconv.ParseFloat(p.src[start:p.i], 64)
}

// ── 偏移结果格式化 ──────────────────────────────────────────

// formatEpisodeOffset 按原集数字符串格式返回偏移后的集数（对齐 Python 版
// _format_episode_offset：阿拉伯数字保持前导零宽度，中文数字转回中文）。
func formatEpisodeOffset(orig string, val int) string {
	if !asciiDigitsRe.MatchString(orig) {
		return intToCnLow(val)
	}
	width := 0
	if leadingZeroRe.MatchString(orig) {
		width = len(orig)
	}
	if val < 0 {
		return "-" + zfill(strconv.Itoa(-val), width)
	}
	return zfill(strconv.Itoa(val), width)
}

func zfill(s string, width int) string {
	for len(s) < width {
		s = "0" + s
	}
	return s
}

var cnLowDigits = []string{"零", "一", "二", "三", "四", "五", "六", "七", "八", "九"}

// intToCnLow 数字转中文小写（an2cn low 子集，覆盖 0-999）。
func intToCnLow(n int) string {
	if n < 0 {
		return "-" + intToCnLow(-n)
	}
	if n < 10 {
		return cnLowDigits[n]
	}
	if n < 20 {
		return "十" + trailingCnDigit(n%10)
	}
	if n < 100 {
		return cnLowDigits[n/10] + "十" + trailingCnDigit(n%10)
	}
	if n < 1000 {
		s := cnLowDigits[n/100] + "百"
		r := n % 100
		if r == 0 {
			return s
		}
		if r < 10 {
			return s + "零" + cnLowDigits[r]
		}
		return s + intToCnLow(r)
	}
	return strconv.Itoa(n)
}

func trailingCnDigit(d int) string {
	if d == 0 {
		return ""
	}
	return cnLowDigits[d]
}

var animeFormatRe = regexp.MustCompile(`(?i)^\[.+\].+|\d{4}年?.{0,5}[番漫]|新番|动漫|動漫`)

// IsAnime 判断是否为动漫格式（带方括号形如 [字幕组] Title [编号]...）。
func IsAnime(title string) bool {
	if title == "" {
		return false
	}
	return animeFormatRe.MatchString(title)
}
