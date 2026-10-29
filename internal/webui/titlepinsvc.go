// titlepinsvc.go —— 整理名称（标题钉扎）查询与改用最新译名。
package webui

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"meta2cloud/internal/jsonx"
	"meta2cloud/internal/mediaparser"
	"meta2cloud/internal/strutil"
)

// titlePinItem 整理名称列表条目。
type titlePinItem struct {
	MediaType    string   `json:"media_type"`
	TmdbID       int64    `json:"tmdb_id"`
	PinnedTitle  string   `json:"pinned_title"`
	PinnedYear   string   `json:"pinned_year"`
	Source       string   `json:"source"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
	CurrentTitle string   `json:"current_title"` // TMDB 当前译名（缓存，可为空）
	CurrentYear  string   `json:"current_year"`
	Drifted      bool     `json:"drifted"`
	FolderIDs    []string `json:"folder_ids"`
}

func (s *Server) handleListTitlePins(w http.ResponseWriter, _ *http.Request) {
	ls := s.rt.GetLibraryStore()
	pins, err := ls.ListTitlePins()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 批量取 TMDB 当前译名与网盘目录（单条扫描），替代逐条查询
	keys := make([]string, 0, len(pins))
	for _, pin := range pins {
		keys = append(keys, pin.MediaType+":"+strconv.FormatInt(pin.TmdbID, 10))
	}
	tmdbEntries := ls.GetTmdbEntriesByKeys(keys)
	folderGroups := ls.ListFolderIDsGroupedByTmdb()

	items := make([]titlePinItem, 0, len(pins))
	for _, pin := range pins {
		key := pin.MediaType + ":" + strconv.FormatInt(pin.TmdbID, 10)
		item := titlePinItem{
			MediaType:   pin.MediaType,
			TmdbID:      pin.TmdbID,
			PinnedTitle: pin.PinnedTitle,
			PinnedYear:  pin.PinnedYear,
			Source:      pin.Source,
			CreatedAt:   pin.CreatedAt,
			UpdatedAt:   pin.UpdatedAt,
			FolderIDs:   folderGroups[key],
		}
		if entry := tmdbEntries[key]; entry != nil {
			item.CurrentTitle = entry.Title
			date := entry.FirstAirDate
			if pin.MediaType == "movie" {
				date = entry.ReleaseDate
			}
			item.CurrentYear = firstN4Str(date)
			item.Drifted = item.CurrentTitle != "" && item.CurrentTitle != pin.PinnedTitle
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleAdoptTitlePin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MediaType string `json:"media_type"`
		TmdbID    int64  `json:"tmdb_id"`
	}
	if err := jsonDecodeBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if body.TmdbID <= 0 || (body.MediaType != "movie" && body.MediaType != "tv") {
		writeJSONError(w, http.StatusBadRequest, "media_type 或 tmdb_id 无效")
		return
	}
	ls := s.rt.GetLibraryStore()
	provider := s.rt.GetStorageProvider()
	if provider == nil {
		writeJSONError(w, http.StatusInternalServerError, "存储 Provider 不可用")
		return
	}

	// 1. 取 TMDB 当前译名（实时优先，缓存兜底）
	title, year, err := s.fetchCurrentTmdbTitle(body.MediaType, body.TmdbID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	// 2. 重命名网盘中与旧钉扎名一致的目录
	oldPin, hasPin := ls.GetTitlePin(body.MediaType, body.TmdbID)
	renamed := 0
	if hasPin {
		oldName := movieFolderName(oldPin.PinnedTitle, oldPin.PinnedYear)
		newName := movieFolderName(title, year)
		if newName != "" && newName != oldName {
			for _, fid := range ls.ListFolderIDsByTmdb(body.MediaType, body.TmdbID) {
				cf, err := provider.GetFile(fid)
				if err != nil || cf.Trashed || cf.Name != oldName {
					continue
				}
				if _, err := provider.RenameFile(fid, newName); err != nil {
					log.Printf("采用新译名：重命名目录失败 [%s]: %v", cf.Name, err)
					continue
				}
				renamed++
			}
		}
	}

	// 3. 更新钉扎记录
	if err := ls.AdoptTitlePin(body.MediaType, body.TmdbID, title, year); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	oldTitle := title
	if hasPin {
		oldTitle = oldPin.PinnedTitle
	}
	s.rt.appLog("library", "title_pin_adopt", "SUCCESS",
		fmt.Sprintf("已采用 TMDB 最新译名：%s → %s", oldTitle, title), map[string]any{
			"media_type": body.MediaType, "tmdb_id": body.TmdbID,
			"title": title, "renamed_folders": renamed,
		})

	// 4. 触发整理（合并可能残留的重复目录）与 STRM 同步
	s.rt.SchedulePipeline(0)
	s.rt.strmSvc.TriggerStrmSync("title-pin-adopt")

	writeJSON(w, http.StatusOK, map[string]any{
		"media_type": body.MediaType, "tmdb_id": body.TmdbID,
		"title": title, "year": year, "renamed_folders": renamed,
	})
}

// handleDeleteTitlePin 解除整理名称固定（POST /api/title-pins:delete）。
// 仅删除固定名称记录，网盘目录不受影响；下次整理将重新确定名称
// （最早入库记录 → TMDB 匹配结果）。
func (s *Server) handleDeleteTitlePin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MediaType string `json:"media_type"`
		TmdbID    int64  `json:"tmdb_id"`
	}
	if err := jsonDecodeBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if body.TmdbID <= 0 || (body.MediaType != "movie" && body.MediaType != "tv") {
		writeJSONError(w, http.StatusBadRequest, "media_type 或 tmdb_id 无效")
		return
	}
	ls := s.rt.GetLibraryStore()
	exists, err := ls.DeleteTitlePin(body.MediaType, body.TmdbID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		writeJSONError(w, http.StatusNotFound, "整理名称记录不存在")
		return
	}
	s.rt.appLog("library", "title_pin_delete", "SUCCESS",
		"已解除整理名称固定",
		map[string]any{"media_type": body.MediaType, "tmdb_id": body.TmdbID})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// fetchCurrentTmdbTitle 实时获取 TMDB 当前译名与年份（缓存兜底）。
func (s *Server) fetchCurrentTmdbTitle(mediaType string, tmdbID int64) (string, string, error) {
	if client := s.rt.GetTmdbClient(); client != nil {
		mtype := mediaparser.MediaTypeFromAgent(mediaType)
		if detail := client.GetByID(mtype, int(tmdbID)); detail != nil {
			title := strutil.FirstNonEmpty(jsonx.Str(detail, "name"), jsonx.Str(detail, "title"))
			date := strutil.FirstNonEmpty(jsonx.Str(detail, "first_air_date"), jsonx.Str(detail, "release_date"))
			if title != "" {
				return title, firstN4Str(date), nil
			}
		}
	}
	if entry := s.rt.GetLibraryStore().GetTmdbEntry(mediaType, tmdbID); entry != nil && entry.Title != "" {
		date := entry.FirstAirDate
		if mediaType == "movie" {
			date = entry.ReleaseDate
		}
		return entry.Title, firstN4Str(date), nil
	}
	return "", "", fmt.Errorf("获取 TMDB 当前译名失败：tmdb_id=%d", tmdbID)
}

func firstN4Str(date string) string {
	if len(date) >= 4 {
		return date[:4]
	}
	return date
}
