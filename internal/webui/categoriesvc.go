// categoriesvc.go —— 媒体分类配置读写。
package webui

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"meta2cloud/internal/cloudfile"
	"meta2cloud/internal/config"
)

// handleGetCategories 回显分类配置。
func (s *Server) handleGetCategories(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.rt.GetCategories())
}

// handleSaveCategories 保存分类配置并预创建分类目录树。
func (s *Server) handleSaveCategories(w http.ResponseWriter, r *http.Request) {
	var cats config.CategoriesConfig
	if err := jsonDecodeBody(r, &cats); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := cats.Validate(); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	oldCats := s.rt.GetCategories()
	if err := s.rt.SetCategories(&cats); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 规则变化检测：供前端提示「是否将存量媒体按新规则重新归位」
	rulesChanged := categoriesChanged(oldCats, &cats)
	// 预创建分类目录树（幂等），保存后立即可在文件管理/网盘看到结构
	// 配置值为媒体库根目录名：先按名解析，根目录尚不存在则直接在网盘根下创建
	provider := s.rt.GetStorageProvider()
	rootName := strings.TrimSpace(s.rt.GetConfig().ActiveRootFolderID())
	if provider != nil && rootName != "" {
		rootID, rootOK := cloudfile.FindFolderByName(provider, rootName)
		if !rootOK {
			if dir, err := cloudfile.GetOrCreateFolder(provider, rootName, "0"); err == nil {
				rootID = dir.ID
			} else {
				log.Printf("预创建媒体库根目录失败 %q: %v", rootName, err)
			}
		}
		if rootID != "" {
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
	}
	appLog("library", "categories_save", "SUCCESS", "媒体分类配置已保存", map[string]any{
		"tv":    len(cats.TV),
		"movie": len(cats.Movie),
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "rules_changed": rulesChanged})
}

// categoriesChanged 判断分类规则是否变化（JSON 序列化对比，规避 nil/空切片差异）。
func categoriesChanged(a, b *config.CategoriesConfig) bool {
	if a == nil || b == nil {
		return a != b
	}
	ab, err1 := json.Marshal(a)
	bb, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return !bytes.Equal(ab, bb)
}
