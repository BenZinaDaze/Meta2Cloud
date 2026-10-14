// filemanager.go —— 文件管理服务（对应 webui/services/file_manager.py）。
// Google Drive 根目录映射已移除，仅支持 pan115（根目录 "0"）。
package webui

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"meta2cloud/internal/cloudfile"
	"meta2cloud/internal/u115"
)

func providerRootID() string {
	if RT().GetConfig().ActiveStorageName() == "pan115" {
		return "0"
	}
	return "root"
}

func normalizeFileManagerFolderID(folderID string) string {
	rootID := providerRootID()
	if folderID == "" || folderID == "root" {
		return rootID
	}
	return folderID
}

func cloudFileToResponse(f cloudfile.CloudFile) map[string]any {
	fileType := "file"
	if f.IsFolder() {
		fileType = "folder"
	}
	var size any
	if f.Size != nil {
		size = *f.Size
	}
	return map[string]any{
		"id": f.ID, "name": f.Name, "file_type": fileType, "size": size,
		"modified_time": f.ModifiedTime, "parent_id": f.ParentID,
		"mime_type": f.MimeType,
		"is_video":  f.IsVideo(), "is_subtitle": f.IsSubtitle(),
		"extension": f.Extension(), "extra": f.Extra,
	}
}

func breadcrumb(id, name string) map[string]string {
	return map[string]string{"id": id, "name": name}
}

func buildFolderPath(folderID, rootID, folderName string) []map[string]string {
	path := []map[string]string{breadcrumb(rootID, "根目录")}
	if folderID != rootID {
		if folderName != "" {
			path = append(path, breadcrumb(folderID, folderName))
		} else {
			path = append(path, breadcrumb(folderID, folderID))
		}
	}
	return path
}

// buildPath 从文件向上追溯构建完整路径面包屑（深度上限防数据成环时死循环）。
func buildPath(provider cloudfile.Provider, file cloudfile.CloudFile, rootID string) []map[string]string {
	var path []map[string]string
	current := file
	for depth := 0; depth < 64; depth++ {
		path = append([]map[string]string{breadcrumb(current.ID, current.Name)}, path...)
		if current.ParentID != "" && current.ParentID != rootID {
			parent, err := provider.GetFile(current.ParentID)
			if err != nil {
				break
			}
			current = parent
		} else {
			break
		}
	}
	return append([]map[string]string{breadcrumb(rootID, "根目录")}, path...)
}

// ── HTTP 处理器 ─────────────────────────────────────────────

func handleListFiles(w http.ResponseWriter, r *http.Request) {
	provider := RT().GetStorageProvider()
	if provider == nil {
		writeJSONError(w, http.StatusInternalServerError, "存储 Provider 不可用")
		return
	}
	q := r.URL.Query()
	folderID := q.Get("folder_id")
	folderName := q.Get("folder_name")
	search := q.Get("search")
	providerName := RT().GetConfig().ActiveStorageName()
	rootID := providerRootID()
	normalized := normalizeFileManagerFolderID(folderID)

	items, err := provider.ListFiles(normalized, 1000)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if search != "" {
		needle := strings.ToLower(search)
		var filtered []map[string]any
		for _, f := range items {
			if strings.Contains(strings.ToLower(f.Name), needle) {
				filtered = append(filtered, cloudFileToResponse(f))
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items": filtered, "folder_id": normalized,
			"folder_name": "搜索结果: " + search, "parent_id": nil,
			"provider": providerName, "root_id": rootID,
			"path": []map[string]string{breadcrumb(rootID, "根目录")},
		})
		return
	}

	responseItems := make([]map[string]any, 0, len(items))
	for _, f := range items {
		responseItems = append(responseItems, cloudFileToResponse(f))
	}
	displayName := folderName
	if displayName == "" {
		displayName = "根目录"
	}
	var parentID any
	if normalized != rootID && len(items) > 0 {
		parentID = items[0].ParentID
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": responseItems, "folder_id": normalized,
		"folder_name": displayName, "parent_id": parentID,
		"provider": providerName, "root_id": rootID,
		"path": buildFolderPath(normalized, rootID, folderName),
	})
}

func handleGetFile(w http.ResponseWriter, r *http.Request) {
	provider := RT().GetStorageProvider()
	fileID := r.PathValue("fileID")
	file, err := provider.GetFile(fileID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	response := cloudFileToResponse(file)
	var path []map[string]string
	if r.URL.Query().Get("include_path") == "true" {
		path = buildPath(provider, file, providerRootID())
	}
	response["path"] = path
	writeJSON(w, http.StatusOK, response)
}

func handleCreateFile(w http.ResponseWriter, r *http.Request) {
	provider := RT().GetStorageProvider()
	var body struct {
		Name     string `json:"name"`
		ParentID string `json:"parent_id"`
	}
	if err := jsonDecodeBody(r, &body); err != nil || body.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "name 不能为空")
		return
	}
	folder, err := provider.CreateFolder(body.Name, normalizeFileManagerFolderID(body.ParentID))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cloudFileToResponse(folder))
}

func handleUpdateFile(w http.ResponseWriter, r *http.Request) {
	provider := RT().GetStorageProvider()
	fileID := r.PathValue("fileID")
	var body struct {
		Name     string `json:"name"`
		ParentID string `json:"parent_id"`
	}
	_ = jsonDecodeBody(r, &body)
	var file cloudfile.CloudFile
	var err error
	switch {
	case body.Name != "" && body.ParentID != "":
		file, err = provider.MoveFile(fileID, body.ParentID, body.Name)
	case body.Name != "":
		file, err = provider.RenameFile(fileID, body.Name)
	case body.ParentID != "":
		file, err = provider.MoveFile(fileID, body.ParentID, "")
	default:
		writeJSONError(w, http.StatusBadRequest, "至少需要提供 name 或 parent_id")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cloudFileToResponse(file))
}

func handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	provider := RT().GetStorageProvider()
	if err := provider.DeleteFile(r.PathValue("fileID")); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func handleFileBatch(w http.ResponseWriter, r *http.Request) {
	provider := RT().GetStorageProvider()
	var body struct {
		Action   string   `json:"action"`
		FileIDs  []string `json:"file_ids"`
		ParentID string   `json:"parent_id"`
	}
	if err := jsonDecodeBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	count := 0
	switch body.Action {
	case "delete":
		for _, fileID := range body.FileIDs {
			if err := provider.DeleteFile(fileID); err != nil {
				if isRateLimit(err) {
					log.Printf("删除操作触发限流，已处理 %d/%d 个: %v", count, len(body.FileIDs), err)
					break
				}
				log.Printf("删除文件 %s 失败: %v", fileID, err)
				continue
			}
			count++
		}
		writeJSON(w, http.StatusOK, map[string]any{"action": "delete", "deleted_count": count})
	case "move":
		if body.ParentID == "" {
			writeJSONError(w, http.StatusBadRequest, "move 操作需要提供 parent_id")
			return
		}
		for _, fileID := range body.FileIDs {
			if _, err := provider.MoveFile(fileID, body.ParentID, ""); err != nil {
				if isRateLimit(err) {
					log.Printf("移动操作触发限流，已处理 %d/%d 个: %v", count, len(body.FileIDs), err)
					break
				}
				log.Printf("移动文件 %s 失败: %v", fileID, err)
				continue
			}
			count++
		}
		writeJSON(w, http.StatusOK, map[string]any{"action": "move", "moved_count": count})
	default:
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("未知操作: %s", body.Action))
	}
}

func isRateLimit(err error) bool {
	return u115.IsRateLimit(err)
}
