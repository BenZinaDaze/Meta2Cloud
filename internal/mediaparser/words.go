// words.go —— 自定义识别词预处理（屏蔽词/替换词/集偏移）。
package mediaparser

import (
	"regexp"
	"strconv"
	"strings"
)

// WordsMatcher 自定义识别词处理。
// 词条格式：
//   - 屏蔽词：  keyword
//   - 替换词：  old => new
//   - 集偏移：  before <> after >> EP+N 或 EP-N
//   - 组合：    old => new && before <> after >> EP+N
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

// Prepare 预处理标题：依次应用所有词条规则。
// 返回 (处理后的标题, 命中的词条列表)。
func (wm *WordsMatcher) Prepare(title string) (string, []string) {
	if title == "" || len(wm.wordList) == 0 {
		return title, nil
	}
	applied := []string{}
	for _, word := range wm.wordList {
		if strings.Contains(word, "&&") {
			// 组合：替换+偏移
			parts := strings.SplitN(word, "&&", 2)
			newTitle, hit := applyReplace(title, strings.TrimSpace(parts[0]))
			if hit {
				applied = append(applied, strings.TrimSpace(parts[0]))
				title = newTitle
			}
			newTitle, hit = applyOffset(title, strings.TrimSpace(parts[1]))
			if hit {
				applied = append(applied, strings.TrimSpace(parts[1]))
				title = newTitle
			}
		} else if strings.Contains(word, "=>") {
			newTitle, hit := applyReplace(title, word)
			if hit {
				applied = append(applied, word)
				title = newTitle
			}
		} else if strings.Contains(word, "<>") && strings.Contains(word, ">>") {
			newTitle, hit := applyOffset(title, word)
			if hit {
				applied = append(applied, word)
				title = newTitle
			}
		} else {
			// 屏蔽词
			re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(word))
			if err != nil {
				continue
			}
			newTitle := re.ReplaceAllString(title, "")
			if newTitle != title {
				applied = append(applied, word)
				title = newTitle
			}
		}
	}
	return strings.TrimSpace(title), applied
}

// applyReplace 应用替换词规则（old_w 作为正则，失败时退回字面量替换）。
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
	re, err := regexp.Compile("(?i)" + oldW)
	var newTitle string
	if err != nil {
		newTitle = strings.ReplaceAll(title, oldW, newW)
	} else {
		newTitle = re.ReplaceAllString(title, newW)
	}
	return newTitle, newTitle != title
}

var offsetRuleRe = regexp.MustCompile(`(?i)^(.+?)\s*<>\s*(.+?)\s*>>\s*EP([+-]\d+)$`)

// applyOffset 应用集偏移规则：before <> after >> EP±N。
func applyOffset(title, word string) (string, bool) {
	m := offsetRuleRe.FindStringSubmatch(word)
	if m == nil {
		return title, false
	}
	before := strings.TrimSpace(m[1])
	after := strings.TrimSpace(m[2])
	offset, err := strconv.Atoi(m[3])
	if err != nil {
		return title, false
	}
	pattern := "(?i)(" + regexp.QuoteMeta(before) + `)\s*(\d+)\s*(` + regexp.QuoteMeta(after) + ")"
	re, err := regexp.Compile(pattern)
	if err != nil {
		return title, false
	}
	loc := re.FindStringSubmatchIndex(title)
	if loc == nil {
		return title, false
	}
	// group 2 的范围
	numStart, numEnd := loc[4], loc[5]
	epNum, err := strconv.Atoi(title[numStart:numEnd])
	if err != nil {
		return title, false
	}
	epNum += offset
	if epNum < 0 {
		epNum = 0
	}
	return title[:numStart] + strconv.Itoa(epNum) + title[numEnd:], true
}

var animeFormatRe = regexp.MustCompile(`(?i)^\[.+\].+|\d{4}年?.{0,5}[番漫]|新番|动漫|動漫`)

// IsAnime 判断是否为动漫格式（带方括号形如 [字幕组] Title [编号]...）。
func IsAnime(title string) bool {
	if title == "" {
		return false
	}
	return animeFormatRe.MatchString(title)
}
