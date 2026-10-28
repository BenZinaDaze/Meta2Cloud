// metavideo.go —— 普通影视文件名/种子名解析器（完整移植 Python 版 MetaVideo）。
package mediaparser

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/dlclark/regexp2"
)

var (
	seasonRe  = regexp.MustCompile(`(?i)S(\d{3})|^S(\d{1,3})$|S(\d{1,3})E`)
	episodeRe = regexp.MustCompile(`(?i)EP?(\d{2,4})$|^EP?(\d{1,4})$|^S\d{1,2}EP?(\d{1,4})$|S\d{2}EP?(\d{2,4})`)
	partRe    = regexp.MustCompile(`(?i)(^PART[0-9ABI]{0,2}$|^CD[0-9]{0,2}$|^DVD[0-9]{0,2}$|^DISK[0-9]{0,2}$|^DISC[0-9]{0,2}$)`)
	// 罗马数字（Python 版含 lookahead，此处等价改写：先限字符集再整串匹配）
	romanNumeralChars = "MDCLXVI"
	romanNumeralsRe   = regexp.MustCompile(`^(?:M*(C[MD]|D?C{0,3})(X[CL]|L?X{0,3})(I[XV]|V?I{0,3}))$`)
	sourceRe          = regexp.MustCompile(`(?i)^(BLURAY|HDTV|UHDTV|HDDVD|WEBRIP|DVDRIP|BDRIP|BLU|WEB|BD|HDRip|REMUX|UHD)$`)
	effectRe          = regexp.MustCompile(`(?i)^(SDR|HDR\d*|DOLBY|DOVI|DV|3D|REPACK|HLG|HDR10(\+|Plus)|EDR|HQ)$`)
	resourcesTypeRe   = regexp.MustCompile(`(?i)(` + `^BLURAY$|^HDTV$|^UHDTV$|^HDDVD$|^WEBRIP$|^DVDRIP$|^BDRIP$|^BLU$|^WEB$|^BD$|^HDRip$|^REMUX$|^UHD$` + `|` + `^SDR$|^HDR\d*$|^DOLBY$|^DOVI$|^DV$|^3D$|^REPACK$|^HLG$|^HDR10(\+|Plus)$|^EDR$|^HQ$` + `)`)
	nameNoBeginRe     = regexp.MustCompile(`^[\[【].+?[\]】]`)
	nameNoChineseRe   = regexp.MustCompile(`(?i).*版|.*字幕`)
	resourcesPixRe    = regexp.MustCompile(`(?i)([SBUHD]*\d{3,4}[PI]+)|(\d{3,4}X\d{3,4})`)
	resourcesPixRe2   = regexp.MustCompile(`(?i)^([248]+K)`)
	videoEncodeRe     = regexp.MustCompile(`(?i)^(H26[45])$|^(x26[45])$|^(AVC)$|^(HEVC)$|^(VC\d?)$|^(MPEG\d?)$|^(Xvid)$|^(DivX)$|^(AV1)$|^(HDR\d*)$|^(AVS(\+|[23]))$`)
	audioEncodeRe     = regexp.MustCompile(`(?i)^(DTS\d?|DTSHD|DTSHDMA|Atmos|TrueHD\d?|AC3|\dAudios?|DDP\d?|DD\+\d?|DD\d?|LPCM\d?|AAC\d?|FLAC\d?|HD\d?|MA\d?|HR\d?|Opus\d?|Vorbis\d?|AV[3S]A)$`)
	fpsRe             = regexp.MustCompile(`(?i)(\d{2,3})FPS`)
	dateRe            = regexp.MustCompile(`\d{4}[\s._-]\d{1,2}[\s._-]\d{1,2}`)
	yearRangeRe       = regexp.MustCompile(`([\s.]+)(\d{4})-(\d{4})`)
	seasonFullRe      = regexp.MustCompile(`(?i)^(?:Season\s+|S)(\d{1,3})$`)
	diySubRe          = regexp.MustCompile(`D[Ii]Y`)
	diyTitleRe        = regexp.MustCompile(`-D[Ii]Y@`)
	seasonWordRe      = regexp.MustCompile(`(?i)SEASON$`)
)

var nameSeWords = map[string]bool{"共": true, "第": true, "季": true, "集": true, "话": true, "話": true, "期": true}
var nameMovieWordsRe = regexp.MustCompile(`(?i)剧场版|劇場版|电影版|電影版`)
var nameNoStringRe = regexp.MustCompile(`(?i)` + strings.Join([]string{
	`^PTS`, `^JADE`, `^AOD`, `^CHC`, `^[A-Z]{1,4}TV[\-0-9UVHDK]*`,
	`HBO$`, `\s+HBO`, `\d{1,2}th`, `\d{1,2}bit`, `NETFLIX`, `AMAZON`, `IMAX`, `^3D`, `\s+3D`, `^BBC\s+`, `\s+BBC`, `BBC$`, `DISNEY\+?`, `XXX`, `\s+DC$`,
	`[第\s共]+[0-9一二三四五六七八九十\-\s]+季`,
	`[第\s共]+[0-9一二三四五六七八九十百零\-\s]+[集话話]`,
	`连载`, `日剧`, `美剧`, `电视剧`, `动画片`, `动漫`, `欧美`, `西德`, `日韩`, `超高清`, `高清`, `无水印`, `下载`, `蓝光`, `翡翠台`, `梦幻天堂·龙网`, `★?\d*月?新番`,
	`最终季`, `合集`, `[多中国英葡法俄日韩德意西印泰台港粤双文语简繁体特效内封官译外挂]+字幕`, `版本`, `出品`, `台版`, `港版`, `\w+字幕组`, `\w+字幕社`,
	`未删减版`, `UNCUT$`, `UNRATE$`, `WITH EXTRAS$`, `RERIP$`, `SUBBED$`, `PROPER$`, `REPACK$`, `SEASON$`, `EPISODE$`, `Complete$`, `Extended$`, `Extended Version$`,
	`S\d{2}\s*-\s*S\d{2}`, `S\d{2}`, `\s+S\d{1,2}`, `EP?\d{2,4}\s*-\s*EP?\d{2,4}`, `EP?\d{2,4}`, `\s+EP?\d{1,4}`,
	`CD[\s.]*[1-9]`, `DVD[\s.]*[1-9]`, `DISK[\s.]*[1-9]`, `DISC[\s.]*[1-9]`,
	`[248]K`, `\d{3,4}[PIX]+`,
	`CD[\s.]*[1-9]`, `DVD[\s.]*[1-9]`, `DISK[\s.]*[1-9]`, `DISC[\s.]*[1-9]`, `\s+GB`,
}, "|"))

// MetaVideo 识别电影、电视剧。
type MetaVideo struct {
	MetaBase
	stopNameFlag   bool
	stopCnnameFlag bool
	lastToken      string
	lastTokenType  string
	continueFlag   bool
	unknownNameStr string
	source         string
	effect         []string
	index          int
}

// NewMetaVideo 解析标题。
func NewMetaVideo(title string, subtitle string, isfile bool, rgMatcher *ReleaseGroupsMatcher) *MetaVideo {
	m := &MetaVideo{}
	m.MetaBase = *NewMetaBase(title, subtitle, isfile)
	if title == "" {
		return m
	}
	originalTitle := title

	// 状态初始化（与 Python 版 __init__ 对齐，continueFlag 初始为 true）
	m.source = ""
	m.effect = nil
	m.index = 0
	m.stopNameFlag = false
	m.stopCnnameFlag = false
	m.lastToken = ""
	m.lastTokenType = ""
	m.continueFlag = true
	m.unknownNameStr = ""

	// 纯数字文件名
	if isfile && isAllDigits(title) && len(title) < 5 {
		n, _ := strconv.Atoi(title)
		m.BeginEpisode = intPtr(n)
		m.Type = MediaTV
		return m
	}

	// Season xx / Sxx 格式直接返回
	if mt := seasonFullRe.FindStringSubmatch(title); mt != nil {
		m.Type = MediaTV
		n, _ := strconv.Atoi(mt[1])
		m.BeginSeason = intPtr(n)
		m.TotalSeason = 1
		return m
	}

	// 预处理
	title = nameNoBeginRe.ReplaceAllString(title, "")
	title = yearRangeRe.ReplaceAllString(title, "$1$2")
	title = removeSizeWithLookahead(title)
	title = dateRe.ReplaceAllString(title, "")

	tokens := NewTokens(title)
	platforms := NewStreamingPlatforms()

	for token := tokens.GetNext(); token != ""; token = tokens.GetNext() {
		m.index++
		m.initPart(token, tokens)
		if m.continueFlag {
			m.initName(token)
		}
		if m.continueFlag {
			m.initYear(token)
		}
		if m.continueFlag {
			m.initResourcePix(token)
		}
		if m.continueFlag {
			m.initSeason(token)
		}
		if m.continueFlag {
			m.initEpisode(token)
		}
		if m.continueFlag {
			m.initResourceType(token)
		}
		if m.continueFlag {
			m.initWebSource(token, tokens, platforms)
		}
		if m.continueFlag {
			m.initVideoEncode(token)
		}
		if m.continueFlag {
			m.initAudioEncode(token)
		}
		if m.continueFlag {
			m.initFPS(token)
		}
		m.continueFlag = true
	}

	if len(m.effect) > 0 {
		// 反转（Python: self._effect.reverse()）
		for i, j := 0, len(m.effect)-1; i < j; i, j = i+1, j-1 {
			m.effect[i], m.effect[j] = m.effect[j], m.effect[i]
		}
		m.ResourceEffect = strings.Join(m.effect, " ")
	}
	if m.source != "" {
		m.ResourceType = strings.TrimSpace(m.source)
	}

	// 原盘 DIY
	if m.ResourceType != "" && strings.Contains(m.ResourceType, "BluRay") {
		if (m.Subtitle != "" && diySubRe.MatchString(m.Subtitle)) || diyTitleRe.MatchString(originalTitle) {
			m.ResourceType = m.ResourceType + " DIY"
		}
	}

	// 副标题
	m.InitSubtitle(m.OrgString)
	if !m.subtitleFlag && m.Subtitle != "" {
		m.InitSubtitle(m.Subtitle)
	}

	// 清理名称
	m.CnName = m.fixName(m.CnName)
	m.EnName = StrTitle(m.fixName(m.EnName))

	if m.Part != "" && strings.ToUpper(m.Part) == "PART" {
		m.Part = ""
	}

	// 拼音→中文
	if m.CnName == "" && m.EnName != "" && m.Subtitle != "" {
		if IsPinyin(m.EnName) {
			cn := getTitleFromDescription(m.Subtitle)
			if cn != "" && len(cn) == len(strings.Fields(m.EnName)) {
				m.CnName = cn
			}
		}
	}

	// 字幕组
	matcher := rgMatcher
	if matcher == nil {
		matcher = NewReleaseGroupsMatcher(nil)
	}
	if team := matcher.Match(originalTitle); team != "" {
		m.ResourceTeam = team
	}
	return m
}

// removeSizeWithLookahead 等价 Python r"[0-9.]+\s*[MGT]i?B(?![A-Z]+)"：
// 移除后不跟大写字母的体积标注。lookahead 需 regexp2。
var sizeLookaheadRe = regexp2.MustCompile(`[0-9.]+\s*[MGT]i?B(?![A-Z]+)`, regexp2.IgnoreCase)

func removeSizeWithLookahead(title string) string {
	out, _ := sizeLookaheadRe.Replace(title, "", -1, -1)
	return out
}

func getTitleFromDescription(description string) string {
	if description == "" {
		return ""
	}
	titles := regexp.MustCompile(`[\s/|]+`).Split(description, -1)
	if len(titles) > 0 && IsChinese(titles[0]) {
		return titles[0]
	}
	return ""
}

// fixName 清理名称中的资源关键词；纯数字且无其他信息时并入集号。
func (m *MetaVideo) fixName(name string) string {
	if name == "" {
		return name
	}
	name = strings.TrimSpace(nameNoStringRe.ReplaceAllString(name, ""))
	name = regexp.MustCompile(`\s+`).ReplaceAllString(name, " ")
	if isAllDigits(name) {
		n, _ := strconv.Atoi(name)
		if n < 1800 && m.Year == "" && m.BeginSeason == nil && m.ResourcePix == "" &&
			m.ResourceType == "" && m.AudioEncode == "" && m.VideoEncode == "" {
			if m.BeginEpisode == nil {
				m.BeginEpisode = intPtr(n)
				return ""
			}
			if m.IsInEpisode([]int{n}) && m.BeginSeason == nil {
				return ""
			}
		}
	}
	return name
}

// firstSubmatch 取多分组匹配中的第一个非空组。
func firstSubmatch(sub []string) string {
	for _, s := range sub[1:] {
		if s != "" {
			return s
		}
	}
	return ""
}

func isRomanNumeral(token string) bool {
	if token == "" {
		return false
	}
	for _, ch := range token {
		if !strings.ContainsRune(romanNumeralChars, ch) {
			return false
		}
	}
	return romanNumeralsRe.MatchString(token)
}

func (m *MetaVideo) initName(token string) {
	if token == "" {
		return
	}
	if m.unknownNameStr != "" {
		if m.CnName == "" {
			if m.EnName == "" {
				m.EnName = m.unknownNameStr
			} else if m.unknownNameStr != m.Year {
				m.EnName = m.EnName + " " + m.unknownNameStr
			}
			m.lastTokenType = "enname"
		}
		m.unknownNameStr = ""
	}
	if m.stopNameFlag {
		return
	}
	if strings.ToUpper(token) == "AKA" {
		m.continueFlag = false
		m.stopNameFlag = true
		return
	}
	if nameSeWords[token] {
		m.lastTokenType = "name_se_words"
		return
	}
	if IsChinese(token) {
		m.lastTokenType = "cnname"
		if m.CnName == "" {
			m.CnName = token
		} else if !m.stopCnnameFlag {
			if nameMovieWordsRe.MatchString(token) ||
				(!nameNoChineseRe.MatchString(token) && !containsAnySeWord(token)) {
				m.CnName = m.CnName + " " + token
			}
			m.stopCnnameFlag = true
		}
		return
	}

	isRoman := isRomanNumeral(token)
	if isAllDigits(token) || isRoman {
		if m.lastTokenType == "name_se_words" {
			return
		}
		if m.Name() != "" {
			if strings.HasPrefix(token, "0") {
				return
			}
			if isAllDigits(token) {
				if _, err := strconv.Atoi(token); err != nil {
					return
				}
			}
			tokenNum, _ := strconv.Atoi(token)
			if !isRoman && m.lastTokenType == "cnname" && tokenNum < 1900 {
				return
			}
			if (isAllDigits(token) && len(token) < 4) || isRoman {
				switch m.lastTokenType {
				case "cnname":
					m.CnName = m.CnName + " " + token
				case "enname":
					m.EnName = m.EnName + " " + token
				}
				m.continueFlag = false
			} else if isAllDigits(token) && len(token) == 4 {
				if m.unknownNameStr == "" {
					m.unknownNameStr = token
				}
			}
		} else {
			if m.unknownNameStr == "" {
				m.unknownNameStr = token
			}
		}
		return
	}
	if seasonRe.MatchString(token) {
		if m.EnName != "" && seasonWordRe.MatchString(m.EnName) {
			m.EnName += " "
		}
		m.stopNameFlag = true
		return
	}
	if episodeRe.MatchString(token) || resourcesTypeRe.MatchString(token) || resourcesPixRe.MatchString(token) {
		m.stopNameFlag = true
		return
	}
	// 媒体扩展名 token 跳过（Python: ("."+token).lower() in MEDIA_EXTS）
	if containsStr(MediaExts(), strings.ToLower("."+token)) {
		return
	}
	if m.EnName != "" {
		m.EnName = m.EnName + " " + token
	} else {
		m.EnName = token
	}
	m.lastTokenType = "enname"
}

func containsAnySeWord(token string) bool {
	for _, ch := range token {
		if nameSeWords[string(ch)] {
			return true
		}
	}
	return false
}

func (m *MetaVideo) initPart(token string, tokens *Tokens) {
	if m.Name() == "" {
		return
	}
	if m.Year == "" && m.BeginSeason == nil && m.BeginEpisode == nil && m.ResourcePix == "" && m.ResourceType == "" {
		return
	}
	mt := partRe.FindStringSubmatch(token)
	if mt == nil {
		return
	}
	if m.Part == "" {
		m.Part = mt[1]
	}
	nxt := tokens.Cur()
	if nxt != "" {
		take := (isAllDigits(nxt) && (len(nxt) == 1 || (len(nxt) == 2 && strings.HasPrefix(nxt, "0")))) ||
			containsStr([]string{"A", "B", "C", "I", "II", "III"}, strings.ToUpper(nxt))
		if take {
			m.Part = m.Part + nxt
			tokens.GetNext()
		}
	}
	m.lastTokenType = "part"
	m.continueFlag = false
}

func (m *MetaVideo) initYear(token string) {
	if m.Name() == "" {
		return
	}
	if !isAllDigits(token) || len(token) != 4 {
		return
	}
	n, _ := strconv.Atoi(token)
	if n <= 1900 || n >= 2050 {
		return
	}
	if m.Year != "" {
		if m.EnName != "" {
			m.EnName = strings.TrimSpace(m.EnName) + " " + m.Year
		} else if m.CnName != "" {
			m.CnName = m.CnName + " " + m.Year
		}
	} else if m.EnName != "" && seasonWordRe.MatchString(m.EnName) {
		m.EnName += " "
	}
	m.Year = token
	m.lastTokenType = "year"
	m.continueFlag = false
	m.stopNameFlag = true
}

func (m *MetaVideo) initResourcePix(token string) {
	if m.Name() == "" {
		return
	}
	subs := resourcesPixRe.FindAllStringSubmatch(token, -1)
	if len(subs) > 0 {
		m.lastTokenType = "pix"
		m.continueFlag = false
		m.stopNameFlag = true
		for _, sub := range subs {
			pix := firstSubmatch(sub)
			if pix != "" && m.ResourcePix == "" {
				m.ResourcePix = strings.ToLower(pix)
				break
			}
		}
		if m.ResourcePix != "" && isAllDigits(m.ResourcePix) && !strings.ContainsAny(m.ResourcePix[len(m.ResourcePix)-1:], "kpi") {
			m.ResourcePix = m.ResourcePix + "p"
		}
		return
	}
	if mt := resourcesPixRe2.FindStringSubmatch(token); mt != nil {
		m.lastTokenType = "pix"
		m.continueFlag = false
		m.stopNameFlag = true
		if m.ResourcePix == "" {
			m.ResourcePix = strings.ToLower(mt[1])
		}
	}
}

func (m *MetaVideo) initSeason(token string) {
	subs := seasonRe.FindAllStringSubmatch(token, -1)
	if len(subs) > 0 {
		m.lastTokenType = "season"
		m.Type = MediaTV
		m.stopNameFlag = true
		m.continueFlag = true
		for _, sub := range subs {
			seStr := ""
			for _, s := range sub[1:] {
				if s != "" && isAllDigits(s) {
					seStr = s
					break
				}
			}
			if seStr == "" {
				break
			}
			se, _ := strconv.Atoi(seStr)
			if m.BeginSeason == nil {
				m.BeginSeason = intPtr(se)
				m.TotalSeason = 1
			} else if se > *m.BeginSeason {
				m.EndSeason = intPtr(se)
				m.TotalSeason = *m.EndSeason - *m.BeginSeason + 1
				if m.IsFile && m.TotalSeason > 1 {
					m.EndSeason = nil
					m.TotalSeason = 1
				}
			}
		}
		return
	}
	if isAllDigits(token) {
		if m.lastTokenType == "SEASON" && m.BeginSeason == nil && len(token) < 3 {
			n, _ := strconv.Atoi(token)
			m.BeginSeason = intPtr(n)
			m.TotalSeason = 1
			m.lastTokenType = "season"
			m.stopNameFlag = true
			m.continueFlag = false
			m.Type = MediaTV
		}
		return
	}
	if strings.ToUpper(token) == "SEASON" && m.BeginSeason == nil {
		m.lastTokenType = "SEASON"
		return
	}
	if m.Type == MediaTV && m.BeginSeason == nil {
		m.BeginSeason = intPtr(1)
	}
}

func (m *MetaVideo) initEpisode(token string) {
	subs := episodeRe.FindAllStringSubmatch(token, -1)
	if len(subs) > 0 {
		m.lastTokenType = "episode"
		m.continueFlag = false
		m.stopNameFlag = true
		m.Type = MediaTV
		for _, sub := range subs {
			seStr := ""
			for _, s := range sub[1:] {
				if s != "" && isAllDigits(s) {
					seStr = s
					break
				}
			}
			if seStr == "" {
				break
			}
			se, _ := strconv.Atoi(seStr)
			if m.BeginEpisode == nil {
				m.BeginEpisode = intPtr(se)
				m.TotalEpisode = 1
			} else if se > *m.BeginEpisode {
				m.EndEpisode = intPtr(se)
				m.TotalEpisode = *m.EndEpisode - *m.BeginEpisode + 1
				if m.IsFile && m.TotalEpisode > 2 {
					m.EndEpisode = nil
					m.TotalEpisode = 1
				}
			}
		}
		return
	}
	if isAllDigits(token) {
		n, err := strconv.Atoi(token)
		if err != nil {
			return
		}
		if m.BeginEpisode != nil && m.EndEpisode == nil && len(token) < 5 && n > *m.BeginEpisode && m.lastTokenType == "episode" {
			m.EndEpisode = intPtr(n)
			m.TotalEpisode = *m.EndEpisode - *m.BeginEpisode + 1
			if m.IsFile && m.TotalEpisode > 2 {
				m.EndEpisode = nil
				m.TotalEpisode = 1
			}
			m.continueFlag = false
			m.Type = MediaTV
		} else if m.BeginEpisode == nil && len(token) > 1 && len(token) < 4 &&
			m.lastTokenType != "year" && m.lastTokenType != "videoencode" && token != m.unknownNameStr {
			m.BeginEpisode = intPtr(n)
			m.TotalEpisode = 1
			m.lastTokenType = "episode"
			m.continueFlag = false
			m.stopNameFlag = true
			m.Type = MediaTV
		} else if m.lastTokenType == "EPISODE" && m.BeginEpisode == nil && len(token) < 5 {
			m.BeginEpisode = intPtr(n)
			m.TotalEpisode = 1
			m.lastTokenType = "episode"
			m.continueFlag = false
			m.stopNameFlag = true
			m.Type = MediaTV
		}
		return
	}
	if strings.ToUpper(token) == "EPISODE" {
		m.lastTokenType = "EPISODE"
	}
}

func (m *MetaVideo) initResourceType(token string) {
	if m.Name() == "" {
		return
	}
	upper := strings.ToUpper(token)
	switch {
	case upper == "DL" && m.lastTokenType == "source" && m.lastToken == "WEB":
		m.source = "WEB-DL"
		m.continueFlag = false
		return
	case upper == "RAY" && m.lastTokenType == "source" && m.lastToken == "BLU":
		if m.source == "UHD" {
			m.source = "UHD BluRay"
		} else {
			m.source = "BluRay"
		}
		m.continueFlag = false
		return
	case upper == "WEBDL":
		m.source = "WEB-DL"
		m.continueFlag = false
		return
	case upper == "REMUX" && m.source == "BluRay":
		m.source = "BluRay REMUX"
		m.continueFlag = false
		return
	case upper == "BLURAY" && m.source == "UHD":
		m.source = "UHD BluRay"
		m.continueFlag = false
		return
	}
	if mt := sourceRe.FindStringSubmatch(token); mt != nil {
		m.lastTokenType = "source"
		m.continueFlag = false
		m.stopNameFlag = true
		if m.source == "" {
			m.source = mt[1]
			m.lastToken = strings.ToUpper(m.source)
		}
		return
	}
	if mt := effectRe.FindStringSubmatch(token); mt != nil {
		m.lastTokenType = "effect"
		m.continueFlag = false
		m.stopNameFlag = true
		effect := mt[1]
		if !containsStr(m.effect, effect) {
			m.effect = append(m.effect, effect)
		}
		m.lastToken = strings.ToUpper(effect)
	}
}

var webTokens = map[string]bool{"WEB": true, "DL": true, "WEBDL": true, "WEBRIP": true}

func (m *MetaVideo) initWebSource(token string, tokens *Tokens, platforms *StreamingPlatforms) {
	if m.Name() == "" {
		return
	}
	platformName := ""
	queryRange := 1
	var prevToken, nextToken string
	if m.index-2 >= 0 && m.index-2 < len(tokens.All()) {
		prevToken = tokens.All()[m.index-2]
	}
	nextToken = tokens.Peek()

	if platforms.IsStreamingPlatform(token) {
		platformName = platforms.GetStreamingPlatformName(token)
	} else {
		for _, adj := range []struct {
			tok    string
			isNext bool
		}{{prevToken, false}, {nextToken, true}} {
			if adj.tok == "" || platformName != "" {
				continue
			}
			for _, sep := range []string{" ", "-"} {
				combined := token + sep + adj.tok
				if !adj.isNext {
					combined = adj.tok + sep + token
				}
				if platforms.IsStreamingPlatform(combined) {
					platformName = platforms.GetStreamingPlatformName(combined)
					queryRange = 2
					if adj.isNext {
						tokens.GetNext()
					}
					break
				}
			}
		}
	}
	if platformName == "" {
		return
	}
	start := m.index - queryRange - queryRange
	if start < 0 {
		start = 0
	}
	end := m.index - 1 + queryRange + 1
	all := tokens.All()
	if end > len(all) {
		end = len(all)
	}
	for i := start; i < end; i++ {
		if all[i] != "" && webTokens[strings.ToUpper(all[i])] {
			m.WebSource = platformName
			m.continueFlag = false
			return
		}
	}
}

func (m *MetaVideo) initVideoEncode(token string) {
	if m.Name() == "" {
		return
	}
	if m.Year == "" && m.ResourcePix == "" && m.ResourceType == "" && m.BeginSeason == nil && m.BeginEpisode == nil {
		return
	}
	if mt := videoEncodeRe.FindStringSubmatch(token); mt != nil {
		m.continueFlag = false
		m.stopNameFlag = true
		m.lastTokenType = "videoencode"
		if m.VideoEncode == "" {
			// 分组：1=(H26[45]) 2=(x26[45])，其余走整段匹配
			if mt[1] != "" {
				m.VideoEncode = strings.ToUpper(mt[1])
			} else if mt[2] != "" {
				m.VideoEncode = strings.ToLower(mt[2])
			} else {
				m.VideoEncode = strings.ToUpper(mt[0])
			}
			m.lastToken = m.VideoEncode
		} else if m.VideoEncode == "10bit" {
			m.VideoEncode = strings.ToUpper(mt[0]) + " 10bit"
			m.lastToken = strings.ToUpper(mt[0])
		}
		return
	}
	upper := strings.ToUpper(token)
	switch {
	case upper == "H" || upper == "X":
		m.continueFlag = false
		m.stopNameFlag = true
		m.lastTokenType = "videoencode"
		if upper == "H" {
			m.lastToken = "H"
		} else {
			m.lastToken = "x"
		}
	case (token == "264" || token == "265") && m.lastTokenType == "videoencode" && (m.lastToken == "H" || m.lastToken == "X"):
		m.VideoEncode = m.lastToken + token
	case isAllDigits(token) && m.lastTokenType == "videoencode" && (m.lastToken == "VC" || m.lastToken == "MPEG"):
		m.VideoEncode = m.lastToken + token
	case upper == "10BIT":
		m.lastTokenType = "videoencode"
		if m.VideoEncode == "" {
			m.VideoEncode = "10bit"
		} else {
			m.VideoEncode = m.VideoEncode + " 10bit"
		}
	}
}

func (m *MetaVideo) initAudioEncode(token string) {
	if m.Name() == "" {
		return
	}
	if m.Year == "" && m.ResourcePix == "" && m.ResourceType == "" && m.BeginSeason == nil && m.BeginEpisode == nil {
		return
	}
	if mt := audioEncodeRe.FindStringSubmatch(token); mt != nil {
		m.continueFlag = false
		m.stopNameFlag = true
		m.lastTokenType = "audioencode"
		m.lastToken = strings.ToUpper(mt[1])
		if m.AudioEncode == "" {
			m.AudioEncode = mt[1]
		} else {
			sep := " "
			if strings.ToUpper(m.AudioEncode) == "DTS" {
				sep = "-"
			}
			m.AudioEncode = m.AudioEncode + sep + mt[1]
		}
		return
	}
	if isAllDigits(token) && m.lastTokenType == "audioencode" {
		if m.AudioEncode != "" {
			if isAllDigits(m.lastToken) {
				m.AudioEncode = m.AudioEncode + "." + token
			} else if lastChar := m.AudioEncode[len(m.AudioEncode)-1:]; lastChar >= "0" && lastChar <= "9" {
				m.AudioEncode = m.AudioEncode[:len(m.AudioEncode)-1] + " " + lastChar + "." + token
			} else {
				m.AudioEncode = m.AudioEncode + " " + token
			}
		}
		m.lastToken = token
	}
}

func (m *MetaVideo) initFPS(token string) {
	if m.Name() == "" {
		return
	}
	if mt := fpsRe.FindStringSubmatch(token); mt != nil && isAllDigits(mt[1]) {
		n, _ := strconv.Atoi(mt[1])
		m.continueFlag = false
		m.stopNameFlag = true
		m.lastTokenType = "fps"
		m.FPS = intPtr(n)
		m.lastToken = mt[1] + "FPS"
	}
}
