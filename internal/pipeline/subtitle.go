// subtitle.go —— 字幕文件匹配器（对应 Python 版 core/subtitle_matcher.py）。
//
// 功能：从字幕文件名提取语言标签、将字幕与视频匹配、生成字幕目标文件名。
package pipeline

import (
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"

	"meta2cloud/internal/cloudfile"
)

// SubtitleMatch 字幕匹配结果。
type SubtitleMatch struct {
	SubtitleFile cloudfile.CloudFile // 原字幕文件
	LanguageTags []string            // 语言标签列表 ['zh-CN', 'forced']
	TargetName   string              // 目标字幕文件名
	Confidence   float64             // 匹配置信度 0.0-1.0
}

// SubtitleMatcher 字幕匹配器。
type SubtitleMatcher struct{}

// episodePattern 集数匹配模式（用于判断是否为单集字幕）。
var episodePattern = regexp.MustCompile(`(?i)[Ss](\d{1,2})[Ee](\d{1,3})|[Ee](\d{1,3})|第(\d{1,3})[集话]`)

// languageCodeMap 语言代码标准化。
var languageCodeMap = map[string]string{
	// 简体中文 -> zh-CN
	"chs": "zh-CN", "sc": "zh-CN", "simplified": "zh-CN", "zh-cn": "zh-CN",
	"简体": "zh-CN", "简中": "zh-CN", "gb": "zh-CN",
	// 繁体中文 -> zh-TW
	"cht": "zh-TW", "tc": "zh-TW", "traditional": "zh-TW", "zh-tw": "zh-TW",
	"繁体": "zh-TW", "繁中": "zh-TW", "big5": "zh-TW",
	// 中文通用 -> zh
	"zh": "zh", "chinese": "zh", "中文": "zh", "中字": "zh", "中英": "zh",
	// 英文
	"en": "en", "eng": "en", "english": "en", "英": "en", "英字": "en",
	// 日文
	"ja": "ja", "jp": "ja", "japanese": "ja", "日": "ja", "日字": "ja",
	// 韩文
	"ko": "ko", "kr": "ko", "korean": "ko", "韩": "ko", "韩字": "ko",
}

const langTagAlternation =
// 语言标签全集（后缀与括号两种模式共用）
`chs|cht|sc|tc|gb|big5|simplified|traditional|` +
	`zh-cn|zh-tw|zh|chinese|简体|繁体|简中|繁中|中文|中字|中英|` +
	`en|eng|english|英|英字|` +
	`ja|jp|japanese|日|日字|` +
	`ko|kr|korean|韩|韩字|` +
	`default|forced|sdh|hi|commentary`

// suffixLangPattern 匹配末尾语言标签（只从文件名末尾匹配，避免误匹配标题词）。
var suffixLangPattern = regexp.MustCompile(`(?i)[\._-](` + langTagAlternation + `)$`)

// bracketLangPattern 匹配括号形式语言标签（可出现在任意位置）。
var bracketLangPattern = regexp.MustCompile(`(?i)[\[【](` + langTagAlternation + `)[\]】]`)

var (
	sepCollapseRe = regexp.MustCompile(`[\._-]+`)
	emptyBracketA = regexp.MustCompile(`\[\s*\]`)
	emptyBracketB = regexp.MustCompile(`【\s*】`)
)

// ParseSubtitleTags 解析字幕文件名，提取语言标签。
// 返回 (清理后的视频名, 语言标签列表)。
//
// 例："Movie.2024.chs.forced" -> ("Movie.2024", ["zh-CN", "forced"])。
func (m *SubtitleMatcher) ParseSubtitleTags(subtitleStem string) (string, []string) {
	var tags []string
	seen := map[string]bool{}
	addTag := func(normalized string) {
		if !seen[normalized] {
			seen[normalized] = true
			tags = append(tags, normalized)
		}
	}
	cleanName := subtitleStem

	// 先处理括号形式的语言标签
	var bracketTags []string
	for {
		loc := bracketLangPattern.FindStringSubmatchIndex(cleanName)
		if loc == nil {
			break
		}
		tag := strings.ToLower(cleanName[loc[2]:loc[3]])
		if normalized, ok := languageCodeMap[tag]; ok {
			tag = normalized
		}
		if !contains(bracketTags, tag) {
			bracketTags = append(bracketTags, tag)
		}
		cleanName = cleanName[:loc[0]] + cleanName[loc[1]:]
	}

	// 从末尾开始处理后缀形式的语言标签（收集后反转以保持原始顺序）
	var suffixTags []string
	for {
		loc := suffixLangPattern.FindStringSubmatchIndex(cleanName)
		if loc == nil {
			break
		}
		tag := strings.ToLower(cleanName[loc[2]:loc[3]])
		if normalized, ok := languageCodeMap[tag]; ok {
			tag = normalized
		}
		if !contains(suffixTags, tag) {
			suffixTags = append(suffixTags, tag)
		}
		cleanName = cleanName[:loc[0]]
	}

	// 最终顺序：反转后的后缀标签 + 括号标签
	for i, j := 0, len(suffixTags)-1; i < j; i, j = i+1, j-1 {
		suffixTags[i], suffixTags[j] = suffixTags[j], suffixTags[i]
	}
	for _, t := range suffixTags {
		addTag(t)
	}
	for _, t := range bracketTags {
		addTag(t)
	}

	// 清理多余的分隔符和空括号
	cleanName = strings.Trim(sepCollapseRe.ReplaceAllString(cleanName, "."), ".")
	cleanName = emptyBracketA.ReplaceAllString(cleanName, "")
	cleanName = emptyBracketB.ReplaceAllString(cleanName, "")
	cleanName = strings.Trim(cleanName, ".")

	return cleanName, tags
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func fileStemOf(name string) string {
	if i := strings.LastIndex(name, "."); i > 0 {
		return name[:i]
	}
	return name
}

// MatchSubtitleToVideo 将字幕匹配到视频。
//
// 匹配策略：精确匹配 → 字幕名是视频名前缀 → 视频名是字幕名前缀 → 边界包含匹配。
// 不匹配返回 nil。
func (m *SubtitleMatcher) MatchSubtitleToVideo(subtitle, video cloudfile.CloudFile) *SubtitleMatch {
	subtitleStem := fileStemOf(subtitle.Name)
	videoStem := fileStemOf(video.Name)
	videoStemGuess, languageTags := m.ParseSubtitleTags(subtitleStem)

	lowerVideo := strings.ToLower(videoStem)
	lowerGuess := strings.ToLower(videoStemGuess)
	lowerSubtitle := strings.ToLower(subtitleStem)

	// 精确匹配
	if lowerVideo == lowerGuess {
		return &SubtitleMatch{
			SubtitleFile: subtitle,
			LanguageTags: languageTags,
			TargetName:   m.BuildTargetName(video.Name, subtitle.Extension(), languageTags),
			Confidence:   1.0,
		}
	}

	// 字幕名是视频名的前缀（字幕省略了发行标签）
	// 例如：video=Movie.2024.1080p.WEB-DL.mkv, subtitle=Movie.2024.chs.srt
	if strings.HasPrefix(lowerVideo, lowerGuess+".") || strings.HasPrefix(lowerVideo, lowerGuess+"_") || strings.HasPrefix(lowerVideo, lowerGuess+"-") {
		// 季级字幕不应匹配到单集视频：视频有集数但字幕没有则不匹配
		videoHasEpisode := episodePattern.MatchString(videoStem)
		subtitleHasEpisode := episodePattern.MatchString(videoStemGuess)
		if !(videoHasEpisode && !subtitleHasEpisode) {
			confidence := 0.7 + 0.3*(float64(len(videoStemGuess))/float64(len(videoStem)))
			return &SubtitleMatch{
				SubtitleFile: subtitle,
				LanguageTags: languageTags,
				TargetName:   m.BuildTargetName(video.Name, subtitle.Extension(), languageTags),
				Confidence:   confidence,
			}
		}
	}

	// 视频名是字幕名的前缀（如 Video.mkv -> Video.chs.srt）
	if strings.HasPrefix(lowerSubtitle, lowerVideo+".") || strings.HasPrefix(lowerSubtitle, lowerVideo+"_") || strings.HasPrefix(lowerSubtitle, lowerVideo+"-") {
		confidence := float64(len(videoStem)) / float64(len(subtitleStem))
		return &SubtitleMatch{
			SubtitleFile: subtitle,
			LanguageTags: languageTags,
			TargetName:   m.BuildTargetName(video.Name, subtitle.Extension(), languageTags),
			Confidence:   confidence,
		}
	}

	// 包含匹配：需确保边界匹配，避免 S01E01 匹配到 S01E010
	if idx := strings.Index(lowerGuess, lowerVideo); idx >= 0 && lowerVideo != "" {
		beforeOK := idx == 0 || strings.ContainsRune("._-", rune(lowerGuess[idx-1]))
		afterIdx := idx + len(lowerVideo)
		afterOK := afterIdx == len(lowerGuess) || strings.ContainsRune("._-", rune(lowerGuess[afterIdx]))
		if beforeOK && afterOK {
			confidence := float64(len(videoStem)) / float64(len(lowerGuess))
			if confidence >= 0.5 {
				return &SubtitleMatch{
					SubtitleFile: subtitle,
					LanguageTags: languageTags,
					TargetName:   m.BuildTargetName(video.Name, subtitle.Extension(), languageTags),
					Confidence:   confidence,
				}
			}
		}
	}
	return nil
}

// FindSubtitlesForVideo 找出某个视频的所有匹配字幕。
// 对目标文件名去重保留置信度最高者，再按语言优先级排序。
func (m *SubtitleMatcher) FindSubtitlesForVideo(video cloudfile.CloudFile, allSubtitles []cloudfile.CloudFile) []SubtitleMatch {
	var allMatches []*SubtitleMatch
	for i := range allSubtitles {
		if match := m.MatchSubtitleToVideo(allSubtitles[i], video); match != nil && match.Confidence >= 0.5 {
			allMatches = append(allMatches, match)
		}
	}

	// 按目标名分组，保留每组中置信度最高的
	bestByTarget := map[string]*SubtitleMatch{}
	for _, match := range allMatches {
		if existing, ok := bestByTarget[match.TargetName]; !ok || match.Confidence > existing.Confidence {
			bestByTarget[match.TargetName] = match
		}
	}
	matches := make([]SubtitleMatch, 0, len(bestByTarget))
	for _, match := range bestByTarget {
		matches = append(matches, *match)
	}

	// 按语言优先级排序
	priority := map[string]int{"zh-CN": 0, "zh-TW": 1, "zh": 2, "en": 3, "ja": 4, "ko": 5}
	sort.SliceStable(matches, func(i, j int) bool {
		pi, pj := 99, 99
		if len(matches[i].LanguageTags) > 0 {
			pi = priority[matches[i].LanguageTags[0]]
		}
		if len(matches[j].LanguageTags) > 0 {
			pj = priority[matches[j].LanguageTags[0]]
		}
		return pi < pj
	})
	return matches
}

// BuildTargetName 构建字幕目标文件名：视频名 + 语言标签 + 字幕扩展名。
func (m *SubtitleMatcher) BuildTargetName(videoName, subtitleExt string, languageTags []string) string {
	videoStem := fileStemOf(videoName)
	if len(languageTags) > 0 {
		// 过滤掉非语言标签用于命名（forced, sdh 等保留）
		var parts []string
		for _, t := range languageTags {
			if t != "default" {
				parts = append(parts, t)
			}
		}
		if len(parts) > 0 {
			return fmt.Sprintf("%s.%s%s", videoStem, strings.Join(parts, "."), subtitleExt)
		}
	}
	return videoStem + subtitleExt
}

// ── 视频字幕随视频移动（自 pipeline.go 归位） ────────────────

// ── 字幕移动 ────────────────────────────────────────────────

func (p *Pipeline) moveSubtitlesForVideo(video cloudfile.CloudFile, cleanVideoName string, targetFolder *cloudfile.CloudFile, subtitleIndex map[string][]cloudfile.CloudFile) {
	videoParent := video.ParentID
	if videoParent == "" && len(video.Parents) > 0 {
		videoParent = video.Parents[0]
	}
	candidates := subtitleIndex[videoParent]
	if len(candidates) == 0 {
		return
	}

	matcher := &SubtitleMatcher{}
	matches := matcher.FindSubtitlesForVideo(video, candidates)
	if len(matches) == 0 {
		return
	}

	processedIDs := map[string]bool{}
	for i := range matches {
		match := &matches[i]
		targetSubtitleName := matcher.BuildTargetName(cleanVideoName, match.SubtitleFile.Extension(), match.LanguageTags)

		if targetFolder == nil {
			continue
		}

		// 字幕是否已在目标位置
		if match.SubtitleFile.ParentID == targetFolder.ID && match.SubtitleFile.Name == targetSubtitleName {
			processedIDs[match.SubtitleFile.ID] = true
			langStr := strings.Join(match.LanguageTags, ".")
			if langStr == "" {
				langStr = "default"
			}
			p.log(fmt.Sprintf("      字幕：%s (%s) [已就位]", targetSubtitleName, langStr), "INFO")
			continue
		}

		// 目标位置已存在同名字幕
		existingSub, err := p.provider.FindFile(targetSubtitleName, targetFolder.ID)
		if err == nil && existingSub != nil {
			if existingSub.ID == match.SubtitleFile.ID {
				continue
			}
			if p.replaceExistingVideo {
				if _, err := p.provider.TrashFile(existingSub.ID); err != nil {
					log.Printf("移除已存在字幕失败 [%s]: %v", targetSubtitleName, err)
					continue
				}
				p.log(fmt.Sprintf("      字幕：移除已存在的 %s", targetSubtitleName), "INFO")
			} else {
				log.Printf("目标位置已存在字幕 %s，跳过", targetSubtitleName)
				continue
			}
		}

		if _, err := p.provider.MoveFile(match.SubtitleFile.ID, targetFolder.ID, targetSubtitleName); err != nil {
			log.Printf("移动字幕失败 [%s]: %v", match.SubtitleFile.Name, err)
			continue
		}
		processedIDs[match.SubtitleFile.ID] = true

		langStr := strings.Join(match.LanguageTags, ".")
		if langStr == "" {
			langStr = "default"
		}
		p.log(fmt.Sprintf("      字幕：%s (%s) ✓", targetSubtitleName, langStr), "SUCCESS")
	}

	// 从索引中移除已处理的字幕，避免重复匹配
	if len(processedIDs) > 0 {
		if list, ok := subtitleIndex[videoParent]; ok {
			var remaining []cloudfile.CloudFile
			for _, sub := range list {
				if !processedIDs[sub.ID] {
					remaining = append(remaining, sub)
				}
			}
			subtitleIndex[videoParent] = remaining
		}
	}
}
