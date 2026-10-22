// anitomy.go —— 动漫文件名结构化解析（anitopy 常用子集的 Go 实现）。
//
// 覆盖主流发布格式：
//
//	[LoliHouse] 标题 / Title - 23 [WebRip 1080p HEVC-10bit AAC][简繁字幕]
//	【FSD】标题[02][话名].mp4
//	[Group] Title S2 - 01v2 [1920x1080].mkv
//	标题 第02话 [X264]
//	[桜都字幕组] 标题 第二季 [05][1080p][简繁内封].mkv（末尾未闭合括号按组截断）
//	[SubsPlease] Title - 12 (1080p) [04B6C9EE].mkv（西式圆括号分辨率/校验码）
//
// 提取字段与 anitopy 命名一致：anime_title / anime_season / episode_number /
// anime_type / anime_year / video_resolution / release_group / video_term / audio_term。
package mediaparser

import (
	"regexp"
	"strings"
)

// AnitomyResult 动漫文件名解析结果（对应 anitopy.parse 返回字典）。
type AnitomyResult struct {
	AnimeTitle      string
	AnimeSeasons    []string
	EpisodeNumbers  []string
	AnimeType       string
	AnimeYear       string
	VideoResolution string
	ReleaseGroup    string
	VideoTerm       string
	AudioTerm       string
}

var (
	// 括号分组（扫描方式提取，见 scanGroups）
	// 纯集号（可带 v2 / .5 后缀，支持 - 分隔范围与多集）
	epOnlyRe = regexp.MustCompile(`^\d{1,4}(?:\.5|v\d{1,2})?$`)
	epListRe = regexp.MustCompile(`^\d{1,4}(?:[v.]\d{1,2})?(?:\s*[-&~]\s*\d{1,4})?(?:\s*[,+&]\s*\d{1,4})*$`)
	// “ - 23” 集号分隔
	dashEpRe = regexp.MustCompile(`\s+-\s+(\d{1,4}(?:\.5|v\d{1,2})?)\s*$`)
	// 第N话/集
	cnEpRe = regexp.MustCompile(`第\s*(\d{1,4})\s*[话話集]`)
	// EP 前缀
	epPrefixRe = regexp.MustCompile(`(?i)^EP?(\d{1,4})$`)
	// 分辨率
	resolutionRe = regexp.MustCompile(`(?i)(\d{3,4}[pi]|\d{3,4}x\d{3,4})`)
	// 视频/音频编码
	anitomyVideoRe = regexp.MustCompile(`(?i)(H\.?26[45]|x\.?26[45]|HEVC|AVC|AV1|XviD|DivX|MPEG-?2?|VC-?1|10bit|8bit)`)
	anitomyAudioRe = regexp.MustCompile(`(?i)(AAC(?:\d\.\d)?|FLAC|AC-?3|E-?AC-?3|DDP?[+]?2?\.?\d?|DTS(?:-HD)?(?:\s?MA)?|TrueHD|Atmos|Opus|MP3|PCM)`)
	// 季
	seasonTextRe = regexp.MustCompile(`(?i)(?:Season|S)\s*(\d{1,2})\b`)
	// 年份
	yearReAnitomy = regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	// 类型
	animeTypeRe = regexp.MustCompile(`(?i)\b(TV|Movie|OVA|ONA|SP|Web|Special)\b`)
)

// 视频分辨率括号内容判定
func classifyBracket(content string, r *AnitomyResult) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return true // 空括号忽略
	}
	lower := strings.ToLower(content)

	// 纯集号
	if epListRe.MatchString(content) || epPrefixRe.MatchString(content) {
		for _, part := range regexp.MustCompile(`[\s,&+~-]+`).Split(content, -1) {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			part = strings.TrimPrefix(part, "EP")
			part = strings.TrimPrefix(part, "ep")
			if epOnlyRe.MatchString(part) {
				r.EpisodeNumbers = append(r.EpisodeNumbers, part)
			}
		}
		return true
	}
	// 第N话
	if m := cnEpRe.FindStringSubmatch(content); m != nil {
		r.EpisodeNumbers = append(r.EpisodeNumbers, m[1])
		return true
	}
	// 分辨率（可能混有其他词：WebRip 1080p HEVC-10bit AAC）
	if m := resolutionRe.FindStringSubmatch(content); m != nil && r.VideoResolution == "" {
		r.VideoResolution = strings.ToLower(m[1])
	}
	// 视频/音频编码
	if m := anitomyVideoRe.FindStringSubmatch(content); m != nil && r.VideoTerm == "" {
		r.VideoTerm = m[1]
	}
	if m := anitomyAudioRe.FindStringSubmatch(content); m != nil && r.AudioTerm == "" {
		r.AudioTerm = m[1]
	}
	// 年份
	if m := yearReAnitomy.FindStringSubmatch(content); m != nil && r.AnimeYear == "" && strings.HasPrefix(content, m[1]) {
		r.AnimeYear = m[1]
	}
	_ = lower
	return false
}

// scanGroups 扫描标题中的 [] / () 分组（同等对待，与 anitopy 一致）。
// 返回括号外文本片段与各分组内容；末尾未闭合的组（如 prepare_title 以 ] join
// 截断产生的 "[简繁内封"）按闭合处理，避免残留混入标题。
func scanGroups(title string) (freeParts, groups []string) {
	last := 0
	for i := 0; i < len(title); i++ {
		open := title[i]
		if open != '[' && open != '(' {
			continue
		}
		closeCh := byte(']')
		if open == '(' {
			closeCh = ')'
		}
		rel := strings.IndexByte(title[i+1:], closeCh)
		freeParts = append(freeParts, title[last:i])
		if rel < 0 {
			// 未闭合的尾部组：截到字符串末尾
			groups = append(groups, title[i+1:])
			return freeParts, groups
		}
		groups = append(groups, title[i+1:i+1+rel])
		i += 1 + rel
		last = i + 1
	}
	freeParts = append(freeParts, title[last:])
	return freeParts, groups
}

// AnitomyParse 解析动漫文件名。
func AnitomyParse(title string) AnitomyResult {
	var r AnitomyResult
	if title == "" {
		return r
	}

	// 1. 提取括号分组（[] 与 () 同等对待，含末尾未闭合组）
	freeParts, groups := scanGroups(title)
	firstContentKept := false
	for _, content := range groups {
		classified := classifyBracket(content, &r)
		// 首个非分类括号 = 字幕组
		if !classified && !firstContentKept && r.ReleaseGroup == "" && !resolutionRe.MatchString(content) {
			r.ReleaseGroup = strings.TrimSpace(content)
			firstContentKept = true
		}
	}
	freeText := strings.Join(freeParts, " ")

	// 2. 免括号文本解析
	freeText = strings.Join(strings.Fields(freeText), " ")

	// “ - 23” 尾部集号
	if m := dashEpRe.FindStringSubmatchIndex(freeText); m != nil {
		r.EpisodeNumbers = append(r.EpisodeNumbers, freeText[m[2]:m[3]])
		freeText = strings.TrimSpace(freeText[:m[0]])
	}
	// 第N话
	if m := cnEpRe.FindStringSubmatch(freeText); m != nil {
		r.EpisodeNumbers = append(r.EpisodeNumbers, m[1])
		freeText = strings.TrimSpace(strings.Replace(freeText, m[0], "", 1))
	}
	// 季
	if m := seasonTextRe.FindStringSubmatch(freeText); m != nil {
		r.AnimeSeasons = append(r.AnimeSeasons, m[1])
		freeText = strings.TrimSpace(strings.Replace(freeText, m[0], "", 1))
	}
	// 年份（独立 token）
	if m := yearReAnitomy.FindStringSubmatch(freeText); m != nil {
		r.AnimeYear = m[1]
		freeText = strings.TrimSpace(strings.Replace(freeText, m[0], "", 1))
	}
	// 类型
	if m := animeTypeRe.FindStringSubmatch(freeText); m != nil && r.AnimeType == "" {
		r.AnimeType = m[1]
	}

	// 3. 标题：剩余文本去掉分辨率/编码残留词
	titleText := freeText
	titleText = resolutionRe.ReplaceAllString(titleText, "")
	titleText = anitomyVideoRe.ReplaceAllString(titleText, "")
	titleText = anitomyAudioRe.ReplaceAllString(titleText, "")
	titleText = regexp.MustCompile(`(?i)\b(WebRip|Web-?DL|BluRay|BDRip|B-?Global|Ma10p)\b`).ReplaceAllString(titleText, "")
	titleText = strings.Join(strings.Fields(titleText), " ")
	titleText = strings.Trim(titleText, " -_")
	r.AnimeTitle = titleText

	// 分辨率规范化：1920x1080 → 1080p
	if r.VideoResolution != "" && strings.Contains(r.VideoResolution, "x") {
		parts := strings.SplitN(r.VideoResolution, "x", 2)
		r.VideoResolution = parts[1] + "p"
	}
	return r
}
