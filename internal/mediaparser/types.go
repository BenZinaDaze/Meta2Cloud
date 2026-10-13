// Package mediaparser —— 独立的媒体文件名解析库（Go 移植版）。
//
// 对应 Python 版 mediaparser 包。快速使用：
//
//	meta := mediaparser.MetaInfo("Breaking.Bad.S01E03.1080p.BluRay.HEVC-TTG", nil, false, nil, nil)
//	meta.Name       // "Breaking Bad"
//	meta.Season()   // "S01"
//	meta.Episode()  // "E03"
package mediaparser

import "strings"

// MediaType 媒体类型。
type MediaType int

const (
	MediaUnknown    MediaType = iota
	MediaMovie                // 电影
	MediaTV                   // 电视剧
	MediaCollection           // 系列
)

// String 中文显示名（与 Python 版 value 一致）。
func (m MediaType) String() string {
	switch m {
	case MediaMovie:
		return "电影"
	case MediaTV:
		return "电视剧"
	case MediaCollection:
		return "系列"
	default:
		return "未知"
	}
}

// ToAgent 转换为 TMDB agent 键（movie/tv），未知类型原样返回中文名。
func (m MediaType) ToAgent() string {
	switch m {
	case MediaMovie:
		return "movie"
	case MediaTV:
		return "tv"
	default:
		return m.String()
	}
}

// MediaTypeFromAgent 从 agent 键解析类型。
func MediaTypeFromAgent(key string) MediaType {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "movie":
		return MediaMovie
	case "tv":
		return MediaTV
	default:
		return MediaUnknown
	}
}

// 媒体文件扩展名（与 Python 版 RMT_MEDIAEXT 等一致）。
var mediaExts = []string{
	".mp4", ".mkv", ".ts", ".iso", ".rmvb", ".avi", ".mov", ".mpeg",
	".mpg", ".wmv", ".3gp", ".asf", ".m4v", ".flv", ".m2ts", ".strm", ".tp", ".f4v",
}

var subExts = []string{".srt", ".ass", ".ssa", ".sub"}

var audioExts = []string{".mp3", ".flac", ".wav", ".aac", ".ogg", ".m4a", ".wma"}

// MediaExts 全部媒体相关扩展名。
func MediaExts() []string {
	all := make([]string, 0, len(mediaExts)+len(subExts)+len(audioExts))
	all = append(all, mediaExts...)
	all = append(all, subExts...)
	all = append(all, audioExts...)
	return all
}
