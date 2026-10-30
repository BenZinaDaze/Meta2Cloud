// Package webui —— TitleIndex：media_title_pins / library_media 只读反查（自 traktsvc.go 拆出）。
package webui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"panoply/internal/watched"
)

// ── TitleIndex（media_title_pins / library_media 只读反查）──

var titleNormalizeRe = regexp.MustCompile(`\s+`)

// normalizeTitle 标题归一化：小写、收空白、全角括号转半角。
func normalizeTitle(s string) string {
	s = strings.ReplaceAll(s, "（", "(")
	s = strings.ReplaceAll(s, "）", ")")
	s = strings.TrimSpace(strings.ToLower(s))
	return titleNormalizeRe.ReplaceAllString(s, " ")
}

// titleIndex watched.TitleIndex 的实现（每轮同步构建一次，只读库）。
type titleIndex struct {
	byExact map[string]watched.MediaKey // mediaType|normTitle|year
	byTitle map[string][]identityEntry  // mediaType|normTitle → 候选（年份容错用）
}

type identityEntry struct {
	key  watched.MediaKey
	year int64
}

func titleIndexKey(mediaType, normTitle string, year int64) string {
	return fmt.Sprintf("%s|%s|%d", mediaType, normTitle, year)
}

// buildTitleIndex 从 LibraryStore 构建反查索引（只读，不写入 library.db）。
func (rt *Runtime) buildTitleIndex() watched.TitleIndex {
	idx := &titleIndex{
		byExact: map[string]watched.MediaKey{},
		byTitle: map[string][]identityEntry{},
	}
	libStore := rt.GetLibraryStore()
	add := func(mediaType, title, yearStr string, tmdbID int64) {
		if mediaType == "" || title == "" || tmdbID <= 0 {
			return
		}
		keyType := mediaType
		if keyType == "show" {
			keyType = "tv"
		}
		var year int64
		if y, err := strconv.ParseInt(strings.TrimSpace(yearStr), 10, 64); err == nil {
			year = y
		}
		norm := normalizeTitle(title)
		entry := identityEntry{key: watched.MediaKey{Type: keyType, TMDBID: tmdbID}, year: year}
		if year > 0 {
			exact := titleIndexKey(keyType, norm, year)
			if _, exists := idx.byExact[exact]; !exists {
				idx.byExact[exact] = entry.key
			}
		}
		listKey := keyType + "|" + norm
		idx.byTitle[listKey] = append(idx.byTitle[listKey], entry)
	}
	if pins, err := libStore.ListTitlePins(); err == nil {
		for _, pin := range pins {
			add(pin.MediaType, pin.PinnedTitle, pin.PinnedYear, pin.TmdbID)
		}
	}
	if items, err := libStore.ListMediaIdentities(); err == nil {
		for _, m := range items {
			add(m.MediaType, m.Title, m.Year, m.TmdbID)
		}
	}
	return idx
}

// Lookup 实现 watched.TitleIndex：精确年份优先，未中时同名单内年份差 ≤1 容错。
func (idx *titleIndex) Lookup(mediaType, title string, year int64) (watched.MediaKey, bool) {
	norm := normalizeTitle(title)
	if key, ok := idx.byExact[titleIndexKey(mediaType, norm, year)]; ok {
		return key, true
	}
	candidates := idx.byTitle[mediaType+"|"+norm]
	if len(candidates) == 0 {
		return watched.MediaKey{}, false
	}
	for _, c := range candidates {
		if c.year > 0 && abs64i(c.year-year) <= 1 {
			return c.key, true
		}
	}
	// 年份全对不上但同名唯一时仍接受（钉扎年份缺失的场景）
	if len(candidates) == 1 {
		return candidates[0].key, true
	}
	return watched.MediaKey{}, false
}

func abs64i(a int64) int64 {
	if a < 0 {
		return -a
	}
	return a
}
