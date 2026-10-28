// librarydelete.go —— 媒体联动删除：
// DELETE /api/library/media/{folderID} 删除网盘目录及其衍生数据
// （STRM 文件与记录、库条目、可选联动取消订阅），并追加删除日志；
// GET /api/library/media/{folderID} 返回删除预览（前端确认弹窗展示用）。
package webui

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"

	"meta2cloud/internal/cloudfile"
	"meta2cloud/internal/jsonx"
	"meta2cloud/internal/webui/store"
)

// libraryMediaRef 待删条目的关键字段（必须在删库前读出，删除后即查不到）。
type libraryMediaRef struct {
	FolderID      string
	MediaType     string
	TmdbID        int64
	Title         string
	OriginalTitle string
	Year          string
	PosterURL     string
}

// loadLibraryMediaRef 读取库条目关键字段（不存在返回 nil）。
func loadLibraryMediaRef(folderID string) *libraryMediaRef {
	item := RT().GetLibraryStore().GetLibraryItemByFolderID(folderID)
	if item == nil {
		return nil
	}
	return &libraryMediaRef{
		FolderID:      folderID,
		MediaType:     jsonx.Str(item, "media_type"),
		TmdbID:        jsonx.Int(item, "tmdb_id", 0),
		Title:         jsonx.Str(item, "title"),
		OriginalTitle: jsonx.Str(item, "original_title"),
		Year:          jsonx.Str(item, "year"),
		PosterURL:     jsonx.Str(item, "poster_url"),
	}
}

// listMatchedSubscriptions 查条目匹配的订阅（含已停用，未关联 TMDB 时恒为空）。
// 停用订阅也须纳入：否则停用订阅指向的媒体被删后将永久残留。
func listMatchedSubscriptions(ref *libraryMediaRef) []*store.SubscriptionRecord {
	if ref == nil || ref.TmdbID <= 0 {
		return nil
	}
	rssStore := RT().GetRSSStore()
	return rssStore.ListSubscriptionsByTmdb(ref.MediaType, ref.TmdbID)
}

// folderConfirmedMissing 探测目录是否「确认已不存在」。
// 仅未找到（ErrNotFound）才算；网络失败、限流等瞬时错误无法证明目录已删，
// 返回 false 交由调用方按失败中止（数据安全优先：本地数据一概不动）。
func folderConfirmedMissing(provider cloudfile.Provider, folderID string) bool {
	_, err := provider.GetFile(folderID)
	return errors.Is(err, cloudfile.ErrNotFound)
}

// collectFolderFileIDs WalkAll 收集目录下全部条目 ID（含子目录）。
// 遍历失败且目录确认已不存在（GetFile 探测命中 ErrNotFound）时视为已删除：
// 空集 + exists=false（幂等）；其余遍历错误原样上抛。
func collectFolderFileIDs(provider cloudfile.Provider, folderID string) (fileIDs map[string]bool, exists bool, err error) {
	fileIDs = map[string]bool{}
	if err := cloudfile.WalkAll(provider, folderID, strmWalkMaxDepth, func(f cloudfile.CloudFile) error {
		fileIDs[f.ID] = true
		return nil
	}); err != nil {
		if folderConfirmedMissing(provider, folderID) {
			return map[string]bool{}, false, nil
		}
		return nil, true, err
	}
	return fileIDs, true, nil
}

// listStrmFilesByFileIDs 从全量 STRM 记录中筛出 file_id 命中的条目
// （file_id 无索引，走全量扫描后内存过滤，数据量可接受）。
func listStrmFilesByFileIDs(st *store.StrmStore, fileIDs map[string]bool) []store.StrmFile {
	if len(fileIDs) == 0 {
		return nil
	}
	var out []store.StrmFile
	for _, rec := range st.ListAllStrmFiles() {
		if fileIDs[rec.FileID] {
			out = append(out, rec)
		}
	}
	return out
}

// removeStrmByFileIDs 删除命中的本地 .strm 文件与映射记录，返回清理数量。
// 本地文件已不存在视为成功；删除失败则保留记录，交由 after-delete 对账重试。
func removeStrmByFileIDs(st *store.StrmStore, fileIDs map[string]bool) int {
	removed := 0
	for _, rec := range listStrmFilesByFileIDs(st, fileIDs) {
		if err := os.Remove(rec.LocalPath); err != nil && !os.IsNotExist(err) {
			log.Printf("[library] 删除本地 STRM 失败 %s：%v", rec.LocalPath, err)
			continue
		}
		st.DeleteStrmFile(rec.ID)
		removed++
	}
	return removed
}

// recordDeletionIngest 追加 status='deleted' 的删除日志
// （title/tmdb_id/drive_folder_id 照抄条目字段，不改动原入库记录）。
func recordDeletionIngest(ref *libraryMediaRef) {
	ingest := RT().GetIngestStore()
	if _, err := ingest.RecordIngest(store.IngestRecord{
		MediaType:     ref.MediaType,
		TmdbID:        ref.TmdbID,
		Title:         ref.Title,
		OriginalTitle: ref.OriginalTitle,
		Year:          ref.Year,
		PosterPath:    ref.PosterURL,
		DriveFolderID: ref.FolderID,
		Status:        "deleted",
	}); err != nil {
		log.Printf("[library] 写入删除日志失败 [%s]：%v", ref.FolderID, err)
	}
}

// cancelMatchedSubscriptions 逐条删除匹配的订阅（含已停用；命中记录随外键级联），返回取消数量。
func cancelMatchedSubscriptions(subs []*store.SubscriptionRecord) int {
	cancelled := 0
	rssStore := RT().GetRSSStore()
	for _, sub := range subs {
		if !rssStore.DeleteSubscription(sub.ID) {
			continue
		}
		appLog("subscription", "deleted", "SUCCESS", "已删除 RSS 订阅："+sub.Name,
			map[string]any{"subscription_id": sub.ID})
		cancelled++
	}
	return cancelled
}

// subscriptionSummaries 预览用的订阅摘要列表。
func subscriptionSummaries(subs []*store.SubscriptionRecord) []map[string]any {
	out := []map[string]any{}
	for _, sub := range subs {
		out = append(out, map[string]any{
			"id": sub.ID, "name": sub.Name, "media_title": sub.MediaTitle,
			"media_type": sub.MediaType, "tmdb_id": sub.TmdbID,
			"site": sub.Site, "subgroup_name": sub.SubgroupName, "season_number": sub.SeasonNumber,
		})
	}
	return out
}

// handleLibraryMediaPreview GET /api/library/media/{folderID} 删除预览。
func handleLibraryMediaPreview(w http.ResponseWriter, r *http.Request) {
	folderID := r.PathValue("folderID")
	if folderID == "" {
		writeJSONError(w, http.StatusBadRequest, "缺少 drive_folder_id")
		return
	}
	ref := loadLibraryMediaRef(folderID)
	if ref == nil {
		writeJSONError(w, http.StatusNotFound, "媒体库条目不存在")
		return
	}
	resp := map[string]any{
		"drive_folder_id": folderID,
		"title":           ref.Title,
		"media_type":      ref.MediaType,
		"tmdb_id":         ref.TmdbID,
		"subscriptions":   subscriptionSummaries(listMatchedSubscriptions(ref)),
	}
	// strm_count 仅在可准确统计时返回：Provider 不可用或目录无法遍历
	// （含目录已被外部删除）时省略，避免前端展示误导性的「0 个」
	if provider := RT().GetStorageProvider(); provider != nil {
		if fileIDs, exists, err := collectFolderFileIDs(provider, folderID); err == nil && exists {
			if st := RT().GetStrmStore(); st != nil {
				resp["strm_count"] = len(listStrmFilesByFileIDs(st, fileIDs))
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleLibraryMediaDelete DELETE /api/library/media/{folderID} 联动删除。
// 流程（网盘删除失败即中止、本地一概不动；目录已不存在视为成功继续）：
// 收集网盘文件 → 删网盘目录 → 清 STRM → 删库条目 → 记删除日志
// → 可选取消订阅 → STRM 对账兜底。
func handleLibraryMediaDelete(w http.ResponseWriter, r *http.Request) {
	folderID := r.PathValue("folderID")
	if folderID == "" {
		writeJSONError(w, http.StatusBadRequest, "缺少 drive_folder_id")
		return
	}
	libStore := RT().GetLibraryStore()
	// 1. 删前读出条目与匹配订阅（删除后即查不到）
	ref := loadLibraryMediaRef(folderID)
	if ref == nil {
		writeJSONError(w, http.StatusNotFound, "媒体库条目不存在")
		return
	}
	subs := listMatchedSubscriptions(ref)

	appLog("library", "delete_start", "INFO", "开始移出媒体库："+ref.Title,
		map[string]any{"folder_id": folderID, "media_type": ref.MediaType, "tmdb_id": ref.TmdbID})

	provider := RT().GetStorageProvider()
	if provider == nil {
		writeJSONError(w, http.StatusInternalServerError, "存储 Provider 不可用")
		return
	}
	// 2+3. WalkAll 收集网盘文件 ID，再删整个目录；网盘删除失败即中止
	fileIDs, exists, err := collectFolderFileIDs(provider, folderID)
	if err != nil {
		appLog("library", "delete_walk", "ERROR", "遍历网盘目录失败："+err.Error(),
			map[string]any{"folder_id": folderID})
		writeJSONError(w, http.StatusInternalServerError, "遍历网盘目录失败："+err.Error())
		return
	}
	if exists {
		appLog("library", "delete_walk", "INFO", fmt.Sprintf("已收集网盘文件 %d 个", len(fileIDs)),
			map[string]any{"folder_id": folderID})
		if err := provider.DeleteFile(folderID); err != nil {
			// 目录确认已不存在视为成功继续（幂等）；其余失败（含探测遇
			// 网络/限流等无法区分的瞬时错误）即中止，本地一概不动
			if !folderConfirmedMissing(provider, folderID) {
				appLog("library", "delete_pan", "ERROR", "删除网盘目录失败："+err.Error(),
					map[string]any{"folder_id": folderID})
				writeJSONError(w, http.StatusInternalServerError, "删除网盘目录失败："+err.Error())
				return
			}
			appLog("library", "delete_pan", "INFO", "网盘目录已不存在，无需删除",
				map[string]any{"folder_id": folderID})
		} else {
			appLog("library", "delete_pan", "SUCCESS", "网盘目录已删除",
				map[string]any{"folder_id": folderID})
		}
	} else {
		appLog("library", "delete_walk", "INFO", "网盘目录已不存在，无需删除",
			map[string]any{"folder_id": folderID})
	}

	// 4. 按 FileID 集合清理 STRM（本地文件 + 记录）
	removedStrm := 0
	if st := RT().GetStrmStore(); st != nil {
		removedStrm = removeStrmByFileIDs(st, fileIDs)
	}
	appLog("library", "delete_strm", "SUCCESS", fmt.Sprintf("已清理 STRM %d 条", removedStrm),
		map[string]any{"folder_id": folderID})
	// 5. 删库条目
	libStore.DeleteLibraryItem(folderID)
	appLog("library", "delete_item", "SUCCESS", "库条目已删除",
		map[string]any{"folder_id": folderID})
	// 6. 追加删除日志
	recordDeletionIngest(ref)
	// 7. 可选联动取消匹配订阅
	cancelledSubs := 0
	if r.URL.Query().Get("cancel_subscription") == "true" {
		cancelledSubs = cancelMatchedSubscriptions(subs)
		if cancelledSubs > 0 {
			appLog("library", "delete_subs", "SUCCESS", fmt.Sprintf("已联动取消订阅 %d 条", cancelledSubs),
				map[string]any{"folder_id": folderID})
		}
	}
	// 7.5 STRM 已即时清理时直接通知 Plex 扫库（无分类信息，回退刷新全部
	// 电影/剧集库）；删除后对账同步通常无变更，不会再触发重复通知
	if removedStrm > 0 {
		notifyPlexRefresh(1, nil)
	}
	// 8. STRM 对账兜底（异步触发，失败只留日志；同步进行中则留待后续同步）
	if !TriggerStrmSync("after-delete") {
		appLog("library", "delete_sync", "WARN", "STRM 同步进行中，本次删除的清理将在后续同步中自动完成",
			map[string]any{"folder_id": folderID})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":                 true,
		"drive_folder_id":         folderID,
		"removed_strm":            removedStrm,
		"cancelled_subscriptions": cancelledSubs,
	})
}
