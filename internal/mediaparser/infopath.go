// infopath.go —— 路径解析辅助（判断文件名词干是否应让位于父目录标题）。
package mediaparser

import (
	"regexp"

	"github.com/dlclark/regexp2"
)

// auxiliaryCnStemRe 辅助中文词全匹配（简繁/字幕/音轨等）。
var auxiliaryCnStemRe = regexp2.MustCompile(
	`^(双语|字幕|特效|内封|外挂|官译|简体|繁体|繁中|简中|中英|简英|多语|`+
		`国英|台粤|音轨|评论|国配|台配|粤语|韩语|日语|杜比|全景声|无损|中字|`+
		`国语|原声)+$`, 0)

var (
	stemLettersRe  = regexp.MustCompile(`[A-Za-z]{2,}`)
	stemSeasonEpRe = regexp.MustCompile(`[第共]\s*[0-9一二三四五六七八九十百零]+\s*[季集话話]`)
)

// shouldUseParentTitleForFileStem 文件名词干仅为辅助中文词时改用父目录标题。
func shouldUseParentTitleForFileStem(stem, parentDirName string, fileMeta *MetaBase) bool {
	if !fileMeta.IsFile || stem == "" || parentDirName == "" {
		return false
	}
	if fileMeta.TmdbID != 0 || fileMeta.DoubanID != "" {
		return false
	}
	if !stemLettersRe.MatchString(parentDirName) {
		return false
	}
	if !IsAllChinese(stem) {
		return false
	}
	if len([]rune(stem)) > 16 {
		return false
	}
	if ok, _ := auxiliaryCnStemRe.MatchString(stem); !ok {
		return false
	}
	if stemSeasonEpRe.MatchString(stem) {
		return false
	}
	return true
}
