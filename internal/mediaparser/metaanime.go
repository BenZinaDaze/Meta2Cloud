// metaanime.go —— 动漫文件名解析器（对应 Python 版 MetaAnime）。
package mediaparser

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/dlclark/regexp2"
)

var animeNoWords = map[string]bool{"CHS&CHT": true, "MP4": true, "GB MP4": true, "WEB-DL": true}

var animeNameNoStringRe = regexp.MustCompile(`(?i)` + strings.Join([]string{
	`S\d{2}\s*-\s*S\d{2}`, `S\d{2}`, `\s+S\d{1,2}`, `EP?\d{2,4}\s*-\s*EP?\d{2,4}`,
	`EP?\d{2,4}`, `\s+EP?\d{1,4}`, `\s+GB`,
}, "|"))

var animeFpsRe = regexp.MustCompile(`(?i)(\d{2,3})FPS`)

// 动漫类型关键词（prepare_title 中检测首括号组）
var animeCategoryRe = regexp.MustCompile(`[动漫画纪录片电影视连续剧集日美韩中港台海外亚洲华语大陆综艺原盘高清]{2,}|TV|Animation|Movie|Documentar|Anime`)

// MetaAnime 识别动漫。
type MetaAnime struct {
	MetaBase
}

// NewMetaAnime 解析动漫标题。
func NewMetaAnime(title string, subtitle string, isfile bool, rgMatcher *ReleaseGroupsMatcher) *MetaAnime {
	m := &MetaAnime{}
	m.MetaBase = *NewMetaBase(title, subtitle, isfile)
	if title == "" {
		return m
	}
	originalTitle := title

	anitomyOrigin := AnitomyParse(title)
	title = prepareAnimeTitle(title)
	info := AnitomyParse(title)

	// 名称（带 anitopy 同款回退链）
	name := info.AnimeTitle
	if name == "" || animeNoWords[name] || (len([]rune(name)) < 5 && !IsChinese(name)) {
		info = AnitomyParse("[ANIME]" + title)
		name = info.AnimeTitle
	}
	if name == "" || animeNoWords[name] || (len([]rune(name)) < 5 && !IsChinese(name)) {
		if nm := regexp.MustCompile(`\[(.+?)]`).FindStringSubmatch(title); nm != nil {
			name = strings.TrimSpace(nm[1])
		}
	}

	if name != "" {
		splitFlag := true
		if strings.Contains(name, "/") {
			parts := strings.Split(name, "/")
			if IsChinese(parts[0]) {
				m.CnName = parts[0]
				if len(parts) > 1 {
					m.EnName = parts[1]
				}
				splitFlag = false
			} else if IsChinese(parts[len(parts)-1]) {
				m.CnName = parts[len(parts)-1]
				if len(parts) > 1 {
					m.EnName = parts[0]
				}
				splitFlag = false
			} else {
				name = parts[len(parts)-1]
			}
		}
		if splitFlag {
			lastType := ""
			for _, word := range strings.Split(name, " ") {
				if word == "" {
					continue
				}
				if strings.HasSuffix(word, "]") {
					word = word[:len(word)-1]
				}
				if isAllDigits(word) {
					if lastType == "cn" && m.CnName != "" {
						m.CnName = m.CnName + " " + word
					} else if lastType == "en" && m.EnName != "" {
						m.EnName = m.EnName + " " + word
					}
				} else if IsChinese(word) {
					if m.CnName != "" {
						m.CnName = m.CnName + " " + word
					} else {
						m.CnName = word
					}
					lastType = "cn"
				} else {
					if m.EnName != "" {
						m.EnName = m.EnName + " " + word
					} else {
						m.EnName = word
					}
					lastType = "en"
				}
			}
		}
	}

	if m.CnName != "" {
		kw := GetKeyword(m.CnName)
		if kw.Keyword != "" {
			m.CnName = strings.TrimSpace(animeNameNoStringRe.ReplaceAllString(kw.Keyword, ""))
		}
	}
	if m.EnName != "" {
		m.EnName = StrTitle(strings.TrimSpace(animeNameNoStringRe.ReplaceAllString(m.EnName, "")))
	}

	// 年份
	if isAllDigits(info.AnimeYear) {
		m.Year = info.AnimeYear
	}

	// 季号
	if len(info.AnimeSeasons) > 0 {
		beginS, _ := strconv.Atoi(info.AnimeSeasons[0])
		if beginS != 0 {
			m.BeginSeason = intPtr(beginS)
			if len(info.AnimeSeasons) > 1 {
				if endS, _ := strconv.Atoi(info.AnimeSeasons[len(info.AnimeSeasons)-1]); endS != beginS {
					m.EndSeason = intPtr(endS)
					m.TotalSeason = *m.EndSeason - *m.BeginSeason + 1
				} else {
					m.TotalSeason = 1
				}
			} else {
				m.TotalSeason = 1
			}
			m.Type = MediaTV
		}
	}

	// 集号
	if len(info.EpisodeNumbers) > 0 {
		if beginE, err := strconv.Atoi(cleanEpisodeNumber(info.EpisodeNumbers[0])); err == nil && beginE != 0 {
			m.BeginEpisode = intPtr(beginE)
			if len(info.EpisodeNumbers) > 1 {
				if endE, err := strconv.Atoi(cleanEpisodeNumber(info.EpisodeNumbers[len(info.EpisodeNumbers)-1])); err == nil && endE != beginE {
					m.EndEpisode = intPtr(endE)
					m.TotalEpisode = *m.EndEpisode - *m.BeginEpisode + 1
				} else {
					m.TotalEpisode = 1
				}
			} else {
				m.TotalEpisode = 1
			}
		}
		m.Type = MediaTV
	}

	// 类型
	if m.Type == MediaUnknown {
		animeType := info.AnimeType
		if strings.ToUpper(animeType) == "TV" {
			m.Type = MediaTV
		} else {
			m.Type = MediaMovie
		}
	}

	// 分辨率
	if info.VideoResolution != "" {
		if strings.Contains(strings.ToLower(info.VideoResolution), "x") {
			parts := regexp.MustCompile(`[Xx]`).Split(info.VideoResolution, -1)
			m.ResourcePix = parts[len(parts)-1] + "p"
		} else {
			m.ResourcePix = strings.ToLower(info.VideoResolution)
		}
		if isAllDigits(m.ResourcePix) {
			m.ResourcePix = m.ResourcePix + "p"
		}
	}

	// 字幕组
	matcher := rgMatcher
	if matcher == nil {
		matcher = NewReleaseGroupsMatcher(nil)
	}
	if team := matcher.Match(originalTitle); team != "" {
		m.ResourceTeam = team
	} else if anitomyOrigin.ReleaseGroup != "" {
		m.ResourceTeam = anitomyOrigin.ReleaseGroup
	}

	// 视频/音频编码
	m.VideoEncode = info.VideoTerm
	m.AudioEncode = info.AudioTerm

	// 帧率
	if mt := animeFpsRe.FindStringSubmatch(originalTitle); mt != nil && isAllDigits(mt[1]) {
		n, _ := strconv.Atoi(mt[1])
		m.FPS = intPtr(n)
	}

	// 副标题
	m.InitSubtitle(m.OrgString)
	if !m.subtitleFlag && m.Subtitle != "" {
		m.InitSubtitle(m.Subtitle)
	}

	if m.Type == MediaUnknown {
		m.Type = MediaTV
	}
	return m
}

var animePrefixRe = regexp.MustCompile(`新番|月?番|[日美国][漫剧]`)
var animePrefixStripRe = regexp.MustCompile(`.*番.|.*[日美国][漫剧].`)
var animeSizeRe = regexp2MustCompile(`[0-9.]+\s*[MGT]i?B(?![A-Z]+)`)
var animeTvNumRe = regexp.MustCompile(`(?i)\[TV\s+(\d{1,4})`)
var anime4kRe = regexp.MustCompile(`(?i)\[4k]`)
var animeBracketNumRe = regexp.MustCompile(`(?i)\[\d+`)
var animeMixedDigitRe = regexp.MustCompile(`[\d|#:：\-()（）\x{4e00}-\x{9fff}]`)

func regexp2MustCompile(pattern string) *regexp2.Regexp {
	return regexp2.MustCompile(pattern, regexp2.IgnoreCase)
}

// cleanEpisodeNumber 去掉集号后缀（v2 / .5）再转整数。
func cleanEpisodeNumber(s string) string {
	for _, suffix := range []string{"v2", "v3", "v4", ".5"} {
		if idx := strings.Index(s, suffix); idx > 0 {
			return s[:idx]
		}
	}
	return s
}

// prepareAnimeTitle 与 Python 版 __prepare_title 对齐。
func prepareAnimeTitle(title string) string {
	if title == "" {
		return title
	}
	title = strings.ReplaceAll(title, "【", "[")
	title = strings.ReplaceAll(title, "】", "]")
	title = strings.TrimSpace(title)

	if match := animePrefixRe.FindStringIndex(title); match != nil {
		if match[1] < len(title)-1 {
			title = animePrefixStripRe.ReplaceAllString(title, "")
		} else {
			title = title[:strings.LastIndex(title, "[")]
		}
	}

	firstItem := strings.SplitN(title, "]", 2)[0]
	if firstItem != "" && animeCategoryRe.MatchString(ToSimplified(firstItem)) {
		// 去掉首个括号组
		title = strings.TrimSpace(regexp.MustCompile(`^[^]]*]`).ReplaceAllString(title, ""))
	}

	title, _ = animeSizeRe.Replace(title, "", -1, -1)
	title = animeTvNumRe.ReplaceAllString(title, "[$1")
	title = anime4kRe.ReplaceAllString(title, "2160p")

	names := strings.Split(title, "]")
	if len(names) > 1 && !strings.Contains(title, "- ") {
		var titles []string
		for _, name := range names {
			if name == "" {
				continue
			}
			lc := ""
			if strings.HasPrefix(name, "[") {
				lc = "["
				name = name[1:]
			}
			if name != "" && strings.Contains(name, "/") {
				last := strings.TrimSpace(strings.Split(name, "/")[len(strings.Split(name, "/"))-1])
				first := strings.TrimSpace(strings.Split(name, "/")[0])
				if last != "" {
					titles = append(titles, lc+last)
				} else {
					titles = append(titles, lc+first)
				}
			} else if name != "" {
				if IsChinese(name) && !IsAllChinese(name) {
					if !animeBracketNumRe.MatchString(name) {
						name = strings.TrimSpace(animeMixedDigitRe.ReplaceAllString(name, ""))
					}
					if name == "" || isAllDigits(strings.TrimSpace(name)) {
						continue
					}
				}
				if name == "[" {
					titles = append(titles, "")
					continue
				}
				titles = append(titles, lc+strings.TrimSpace(name))
			}
		}
		return strings.Join(titles, "]")
	}
	return title
}
