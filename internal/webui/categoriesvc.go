// categoriesvc.go —— 媒体分类配置读写。
package webui

import (
	"log"
	"net/http"

	"meta2cloud/internal/cloudfile"
	"meta2cloud/internal/config"
)

// handleGetCategories 回显分类配置。
func handleGetCategories(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, RT().GetCategories())
}

// handleSaveCategories 保存分类配置并预创建分类目录树。
func handleSaveCategories(w http.ResponseWriter, r *http.Request) {
	var cats config.CategoriesConfig
	if err := jsonDecodeBody(r, &cats); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := cats.Validate(); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := RT().SetCategories(&cats); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 预创建分类目录树（幂等），保存后立即可在文件管理/网盘看到结构
	provider := RT().GetStorageProvider()
	rootID := RT().GetConfig().ActiveRootFolderID()
	if provider != nil && rootID != "" {
		ensure := func(parentID, name string) string {
			dir, err := cloudfile.GetOrCreateFolder(provider, name, parentID)
			if err != nil {
				log.Printf("预创建分类目录失败 %q: %v", name, err)
				return ""
			}
			return dir.ID
		}
		for _, pair := range []struct {
			level1 string
			rules  []config.CategoryRule
		}{
			{cats.Dirs.TV, cats.TV},
			{cats.Dirs.Movie, cats.Movie},
		} {
			l1 := ensure(rootID, pair.level1)
			for _, rule := range pair.rules {
				if l1 != "" && rule.Name != "" && rule.Name != pair.level1 {
					ensure(l1, rule.Name)
				}
			}
		}
		ensure(rootID, cats.Dirs.Other)
	}
	appLog("library", "categories_save", "SUCCESS", "媒体分类配置已保存", map[string]any{
		"tv":    len(cats.TV),
		"movie": len(cats.Movie),
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
