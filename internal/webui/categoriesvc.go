// categoriesvc.go —— 媒体分类配置读写。
package webui

import (
	"net/http"

	"meta2cloud/internal/config"
)

type categoriesPayload struct {
	Enabled bool                     `json:"enabled"`
	Config  *config.CategoriesConfig `json:"config"`
}

func decodeCategoriesPayload(r *http.Request) (bool, *config.CategoriesConfig, error) {
	var body categoriesPayload
	if err := jsonDecodeBody(r, &body); err != nil {
		return false, nil, err
	}
	if body.Config == nil {
		body.Config = config.DefaultCategories()
	}
	if body.Enabled {
		if err := body.Config.Validate(); err != nil {
			return false, nil, err
		}
	}
	return body.Enabled, body.Config, nil
}

// handleGetCategories 回显分类配置（未启用时返回默认规则供编辑）。
func handleGetCategories(w http.ResponseWriter, _ *http.Request) {
	cats := RT().GetCategories()
	if cats == nil {
		writeJSON(w, http.StatusOK, categoriesPayload{Enabled: false, Config: config.DefaultCategories()})
		return
	}
	writeJSON(w, http.StatusOK, categoriesPayload{Enabled: true, Config: cats})
}

// handleSaveCategories 保存分类配置（启用 = 写 categories.yaml；停用 = 删除该文件）。
func handleSaveCategories(w http.ResponseWriter, r *http.Request) {
	enabled, cats, err := decodeCategoriesPayload(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if enabled {
		err = RT().SetCategories(cats)
	} else {
		err = RT().ClearCategories()
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if enabled {
		appLog("library", "categories_save", "SUCCESS", "媒体分类配置已启用", nil)
	} else {
		appLog("library", "categories_save", "SUCCESS", "媒体分类已停用（回到电影根/剧集根布局）", nil)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": enabled})
}
