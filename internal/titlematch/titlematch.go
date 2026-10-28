// Package titlematch 媒体标题的规范化与模糊匹配（纯字符串算法，不涉存储）。
// 供媒体库查找（store）与扫描对账（webui）共用：标题变体交集，或包含关系
// 且多出部分无足轻重时视为同一标题。
package titlematch

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var yearSuffixRe = regexp.MustCompile(`\s*\(\d{4}\)\s*$`)
var whitespaceRe = regexp.MustCompile(`\s+`)
var nonAlnumRe = regexp.MustCompile(`[^0-9a-z\x{4e00}-\x{9fff}]`)
var titleSplitRe = regexp.MustCompile(`[~～:：|／/]+`)

// normalizeLookupTitle 标题规范化：剥离年份后缀、压空白、转小写（保留空格）。
func normalizeLookupTitle(title string) string {
	raw := strings.TrimSpace(strings.ToLower(title))
	raw = yearSuffixRe.ReplaceAllString(raw, "")
	raw = whitespaceRe.ReplaceAllString(raw, " ")
	return raw
}

// compactLookupTitle 在规范化基础上去除非字母数字汉字字符。
func compactLookupTitle(title string) string {
	return nonAlnumRe.ReplaceAllString(normalizeLookupTitle(title), "")
}

// LookupTitleVariants 标题的全部匹配变体（整体与分隔符切分后的各段）。
func LookupTitleVariants(title string) map[string]bool {
	raw := strings.TrimSpace(title)
	variants := map[string]bool{}
	if raw == "" {
		return variants
	}
	variants[normalizeLookupTitle(raw)] = true
	variants[compactLookupTitle(raw)] = true
	for _, part := range titleSplitRe.Split(raw, -1) {
		if n := normalizeLookupTitle(part); n != "" {
			variants[n] = true
		}
		if c := compactLookupTitle(part); c != "" {
			variants[c] = true
		}
	}
	delete(variants, "")
	return variants
}

// TitlesMatch 标题模糊匹配（变体交集，或包含关系且多出部分无足轻重）。
func TitlesMatch(query string, candidates []string) bool {
	queryVariants := LookupTitleVariants(query)
	if len(queryVariants) == 0 {
		return false
	}
	candidateVariants := map[string]bool{}
	for _, candidate := range candidates {
		for v := range LookupTitleVariants(candidate) {
			candidateVariants[v] = true
		}
	}
	if len(candidateVariants) == 0 {
		return false
	}
	for q := range queryVariants {
		if candidateVariants[q] {
			return true
		}
	}
	for q := range queryVariants {
		for c := range candidateVariants {
			if len(q) < 4 || len(c) < 4 {
				continue
			}
			if strings.Contains(c, q) || strings.Contains(q, c) {
				// 某个变体对多出部分过长时继续尝试其他变体（normalize 保留空格、
				// compact 去除，数字缀可能只在 compact 变体上成立）
				if containmentExtraOK(q, c) {
					return true
				}
			}
		}
	}
	return false
}

// containmentExtraOK 包含关系下，多出部分为空、纯数字（年份/序号）或 ≤3 字缀词
// 时视为同一标题；长缀是衍生条目特征（"未来福音" vs "未来福音 extra chorus"），
// 不视为同一标题——否则续作/特别篇会误绑到正篇的库记录。
func containmentExtraOK(a, b string) bool {
	long, short := a, b
	if utf8.RuneCountInString(long) < utf8.RuneCountInString(short) {
		long, short = short, long
	}
	extra := strings.Replace(long, short, "", 1)
	if extra == "" {
		return true
	}
	rs := []rune(extra)
	if len(rs) <= 4 {
		allDigit := true
		for _, r := range rs {
			if r < '0' || r > '9' {
				allDigit = false
				break
			}
		}
		if allDigit {
			return true
		}
	}
	return len(rs) <= 3
}
