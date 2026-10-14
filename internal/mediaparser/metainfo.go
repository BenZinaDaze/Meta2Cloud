// metainfo.go —— 解析入口，对外暴露的主要接口（MetaInfo / MetaInfoPath）。
package mediaparser

import (
	"regexp"
	"strconv"
	"strings"
)

// 内嵌 ID 标签提取（{[tmdbid=xxx;type=xxx;s=x;e=x]} 等）。
var tagPatterns = []*regexp.Regexp{
	// {[tmdbid=xxx;type=xxx;s=x;e=x]}
	regexp.MustCompile(`(?i)\{?\[tmdbid=(?P<tmdbid>\d+)(?:;type=(?P<type>[a-zA-Z]+))?(?:;s=(?P<s>\d+))?(?:;e=(?P<e>\d+))?]\}?`),
	// [tmdbid=xxx] / [tmdb=xxx]
	regexp.MustCompile(`(?i)\[tmdb(?:id)?=(?P<tmdbid>\d+)]`),
	// {tmdbid=xxx} / {tmdb=xxx}
	regexp.MustCompile(`(?i)\{tmdb(?:id)?=(?P<tmdbid>\d+)}`),
	// [doubanid=xxx]
	regexp.MustCompile(`(?i)\[douban(?:id)?=(?P<doubanid>\d+)]`),
}

type embeddedTags struct {
	tmdbID   int
	doubanID string
	mtype    MediaType
	season   *int
	episode  *int
}

func findMetaTags(title string) embeddedTags {
	var tags embeddedTags
	for _, pat := range tagPatterns {
		m := pat.FindStringSubmatch(title)
		if m == nil {
			continue
		}
		names := pat.SubexpNames()
		for i, name := range names {
			if i == 0 || m[i] == "" {
				continue
			}
			switch name {
			case "tmdbid":
				if tags.tmdbID == 0 {
					tags.tmdbID, _ = strconv.Atoi(m[i])
				}
			case "doubanid":
				if tags.doubanID == "" {
					tags.doubanID = m[i]
				}
			case "type":
				if tags.mtype == MediaUnknown {
					tags.mtype = MediaTypeFromAgent(m[i])
				}
			case "s":
				if tags.season == nil {
					n, _ := strconv.Atoi(m[i])
					tags.season = &n
				}
			case "e":
				if tags.episode == nil {
					n, _ := strconv.Atoi(m[i])
					tags.episode = &n
				}
			}
		}
	}
	return tags
}

func stripMetaTags(title string) string {
	for _, pat := range tagPatterns {
		title = pat.ReplaceAllString(title, "")
	}
	return strings.TrimSpace(title)
}

// MetaInfo 从标题字符串创建元数据对象。
//
// title 为媒体标题或文件名（带或不带后缀均可）；
// subtitle 为副标题/描述（可选，用于补充季集信息）；
// isfile 指示是否为文件名；
// customWords 为自定义识别词列表；rgMatcher 为自定义字幕组匹配器（可空）。
func MetaInfo(title string, subtitle string, isfile bool, customWords []string, rgMatcher *ReleaseGroupsMatcher) *MetaBase {
	if title == "" {
		return &NewMetaVideo("", subtitle, isfile, rgMatcher).MetaBase
	}

	// 1. 自定义词预处理
	matcher := NewWordsMatcher(customWords)
	prepared, appliedWords := matcher.Prepare(title)
	title = prepared

	// 2. 提取内嵌 ID 标签
	tags := findMetaTags(title)
	title = stripMetaTags(title)

	// 3. 去掉媒体文件后缀
	if isfile {
		lower := strings.ToLower(title)
		for _, ext := range MediaExts() {
			if strings.HasSuffix(lower, ext) {
				title = title[:len(title)-len(ext)]
				break
			}
		}
	}

	// 4. 选择解析器
	var meta *MetaBase
	if IsAnime(title) {
		meta = &NewMetaAnime(title, subtitle, isfile, rgMatcher).MetaBase
	} else {
		meta = &NewMetaVideo(title, subtitle, isfile, rgMatcher).MetaBase
	}

	// 5. 回填内嵌信息
	meta.ApplyWords = appliedWords
	if tags.tmdbID != 0 {
		meta.TmdbID = tags.tmdbID
	}
	if tags.doubanID != "" {
		meta.DoubanID = tags.doubanID
	}
	if tags.mtype != MediaUnknown {
		meta.Type = tags.mtype
	}
	if tags.season != nil && meta.BeginSeason == nil {
		meta.BeginSeason = tags.season
		meta.TotalSeason = 1
	}
	if tags.episode != nil && meta.BeginEpisode == nil {
		meta.BeginEpisode = tags.episode
		meta.TotalEpisode = 1
	}
	return meta
}

// MetaInfoPath 从文件路径创建元数据对象（文件名 + 父目录 + 上上级目录 merge）。
func MetaInfoPath(path string, customWords []string, rgMatcher *ReleaseGroupsMatcher) *MetaBase {
	if path == "" {
		return &NewMetaVideo("", "", false, rgMatcher).MetaBase
	}
	path = strings.TrimRight(path, "/\\")
	if path == "" {
		path = "/"
	}

	// 文件名（去后缀）
	stem := fileStem(path)
	fileMeta := MetaInfo(stem, "", true, customWords, rgMatcher)

	// 父目录
	parentName := parentDirName(path)
	if parentName != "" {
		if shouldUseParentTitleForFileStem(stem, parentName, fileMeta) {
			fileMeta.CnName = ""
			fileMeta.EnName = ""
		}
		dirMeta := MetaInfo(parentName, "", false, customWords, rgMatcher)
		fileMeta.Merge(dirMeta)
	}

	// 上上级目录
	rootName := grandparentDirName(path)
	if rootName != "" && rootName != parentName {
		rootMeta := MetaInfo(rootName, "", false, customWords, rgMatcher)
		fileMeta.Merge(rootMeta)
	}
	return fileMeta
}

func fileStem(path string) string {
	base := path
	if i := strings.LastIndexAny(path, "/\\"); i >= 0 {
		base = path[i+1:]
	}
	if i := strings.LastIndex(base, "."); i > 0 {
		return base[:i]
	}
	return base
}

func parentDirName(path string) string {
	idx := strings.LastIndexAny(path, "/\\")
	if idx < 0 {
		return ""
	}
	parent := strings.TrimRight(path[:idx], "/\\")
	if j := strings.LastIndexAny(parent, "/\\"); j >= 0 {
		return parent[j+1:]
	}
	return parent
}

func grandparentDirName(path string) string {
	idx := strings.LastIndexAny(path, "/\\")
	if idx < 0 {
		return ""
	}
	parent := strings.TrimRight(path[:idx], "/\\")
	if parent == "" {
		return ""
	}
	return parentDirName(parent + "/x")
}
