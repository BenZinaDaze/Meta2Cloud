// image_uploader.go —— 从 TMDB 下载图片并上传到云存储（对应 Python 版 nfo/image_uploader.py）。
//
// 图片类型（对齐 Plex / Infuse 标准）：
//
//	poster.jpg           媒体文件夹根目录（电影或剧名文件夹）
//	fanart.jpg           媒体文件夹根目录
//	season01-poster.jpg  剧名文件夹根目录（季封面）
//	episode-thumb.jpg    Season 子文件夹（单集剧照，可选）
package pipeline

import (
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"meta2cloud/internal/cloudfile"
	"meta2cloud/internal/nfo"
	"meta2cloud/internal/tmdb"
)

var extMime = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
}

// ImageUploader 从 TMDB 图片 CDN 下载图片，并上传到指定的云存储文件夹。
type ImageUploader struct {
	provider         cloudfile.Provider
	hc               *http.Client
	overwrite        bool
	tmdbImageBaseURL string
}

// NewImageUploader 构造。
func NewImageUploader(provider cloudfile.Provider, tmdbImageBaseURL string) *ImageUploader {
	return &ImageUploader{
		provider:         provider,
		hc:               &http.Client{Timeout: 15 * time.Second},
		overwrite:        true,
		tmdbImageBaseURL: tmdbImageBaseURL,
	}
}

// UploadPoster 下载并上传封面到 folderID。
func (u *ImageUploader) UploadPoster(posterPath, folderID string) *cloudfile.CloudFile {
	return u.downloadAndUpload(posterPath, "poster.jpg", folderID)
}

// UploadFanart 下载并上传背景图到 folderID。
func (u *ImageUploader) UploadFanart(backdropPath, folderID string) *cloudfile.CloudFile {
	return u.downloadAndUpload(backdropPath, "fanart.jpg", folderID)
}

// UploadSeasonPoster 下载并上传季封面到剧名文件夹（Season 0 → season-specials-poster.jpg）。
func (u *ImageUploader) UploadSeasonPoster(posterPath string, season int, folderID string) *cloudfile.CloudFile {
	return u.downloadAndUpload(posterPath, nfo.SeasonPosterName(season), folderID)
}

// UploadEpisodeThumb 下载并上传单集剧照到 Season 文件夹。
func (u *ImageUploader) UploadEpisodeThumb(stillPath, folderID string) *cloudfile.CloudFile {
	return u.downloadAndUpload(stillPath, "episode-thumb.jpg", folderID)
}

// downloadAndUpload 下载 TMDB 图片并上传；失败返回 nil。
func (u *ImageUploader) downloadAndUpload(tmdbPath, filename, folderID string) *cloudfile.CloudFile {
	if tmdbPath == "" {
		return nil
	}
	url := tmdb.ImageURL(tmdbPath, "original", u.tmdbImageBaseURL)
	data := u.fetch(url)
	if data == nil {
		return nil
	}
	uploaded, err := u.provider.UploadBytes(data, filename, folderID, guessMime(filename), u.overwrite)
	if err != nil {
		log.Printf("图片上传失败 [%s]: %v", filename, err)
		return nil
	}
	return &uploaded
}

func (u *ImageUploader) fetch(url string) []byte {
	resp, err := u.hc.Get(url)
	if err != nil {
		log.Printf("图片下载失败 [%s]: %v", url, err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("图片下载失败 [%s]: HTTP %d", url, resp.StatusCode)
		return nil
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("图片下载失败 [%s]: %v", url, err)
		return nil
	}
	return data
}

func guessMime(filename string) string {
	if i := strings.LastIndex(filename, "."); i >= 0 {
		if mime, ok := extMime[strings.ToLower(filename[i:])]; ok {
			return mime
		}
	}
	return "image/jpeg"
}
