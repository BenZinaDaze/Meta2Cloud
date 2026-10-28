// reclassifysvc.go —— 全库按当前分类规则重新归位（批量）。
//
// 干跑判定纯本地：目标分类由实体缓存（tmdb_media.RawJSON）的流派/语言算出，零 TMDB/网盘调用。
// 执行阶段：先按 TMDB 最新详情重新判定全部分类（绕缓存，结果落缓存）；
// 条目当前位置读库中 parent_id（随扫描与移动维护），归类正确的条目零 115 请求；
// 仅 parent_id 缺失时做一次遍历回填。之后只对错位条目移动
// （每部最多 1 次移动调用，受全项目 QPS 节拍约束）。
package webui

import (
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"meta2cloud/internal/cloudfile"
	"meta2cloud/internal/config"
	"meta2cloud/internal/jsonx"
	"meta2cloud/internal/mediaparser"
	"meta2cloud/internal/pipeline"
	"meta2cloud/internal/webui/store"
)

type reclassifyProgress struct {
	Running    bool     `json:"running"`
	Total      int      `json:"total"`
	Processed  int      `json:"processed"`
	Moved      int      `json:"moved"`
	Failed     int      `json:"failed"`
	Moves      []string `json:"moves,omitempty"`  // 已归位明细「标题：原分类 → 新分类」（截断保留前 50 条）
	Errors     []string `json:"errors,omitempty"` // 逐条失败原因（截断保留前 50 条）
	StartedAt  string   `json:"started_at,omitempty"`
	FinishedAt string   `json:"finished_at,omitempty"`
	Error      string   `json:"error,omitempty"`
}

// reclassifyFailures 失败原因收集（上限 50 条，超出只计数）。
func reclassifyFailures(state *reclassifyProgress, reason string) {
	if len(state.Errors) < 50 {
		state.Errors = append(state.Errors, reason)
	}
}

var (
	reclassifyMu  sync.Mutex
	reclassifyRun reclassifyProgress
)

func reclassifyState() reclassifyProgress {
	reclassifyMu.Lock()
	defer reclassifyMu.Unlock()
	return reclassifyRun
}

// resolveCategoryFor 按规则解析分类；info 缺失（无缓存）时走兜底规则。
func resolveCategoryFor(categories *config.CategoriesConfig, isTV bool, info map[string]any) (string, string) {
	if info == nil {
		info = map[string]any{}
	}
	return categories.Resolve(isTV, pipeline.TmdbGenreIDs(info), jsonx.Str(info, "original_language"))
}

func categoryLabel(level1, level2 string) string {
	if level2 != "" && level2 != level1 {
		return level1 + "/" + level2
	}
	return level1
}

// ensureCategoryChainLevels 幂等创建分类链，返回二级（或一级）目录 ID。
func ensureCategoryChainLevels(provider cloudfile.Provider, rootID, level1, level2 string) (string, error) {
	l1, err := cloudfile.GetOrCreateFolder(provider, level1, rootID)
	if err != nil {
		return "", err
	}
	if level2 == "" || level2 == level1 {
		return l1.ID, nil
	}
	l2, err := cloudfile.GetOrCreateFolder(provider, level2, l1.ID)
	if err != nil {
		return "", err
	}
	return l2.ID, nil
}

// classifyTargetParent 单媒体目标分类父目录（修正识别流程用）。
func (s *Server) classifyTargetParent(provider cloudfile.Provider, categories *config.CategoriesConfig, rootID string, isTV bool, info map[string]any) (string, string, error) {
	level1, level2 := resolveCategoryFor(categories, isTV, info)
	parent, err := ensureCategoryChainLevels(provider, rootID, level1, level2)
	if err != nil {
		return "", "", err
	}
	return parent, categoryLabel(level1, level2), nil
}

// reclassifyItem 归位计划中的单个条目。
type reclassifyItem struct {
	FolderID string
	ParentID string // 库中记录的当前父目录 ID（空 = 未知，需遍历定位）
	Title    string
	IsTV     bool
	Level1   string
	Level2   string
	Category string
	TmdbID   int64
	Cached   bool // TMDB 缓存是否存在（缺失时按兜底规则判定）
}

// reclassifyPlan 干跑/执行共用的归位计划（判定纯本地）。
type reclassifyPlan struct {
	Items   []reclassifyItem
	Targets map[string]int // 分类 → 条目数
}

func libraryRefKey(mt string, tmdbID int64) string {
	return mt + ":" + strconv.FormatInt(tmdbID, 10)
}

func (s *Server) buildReclassifyPlan(categories *config.CategoriesConfig, ls *store.LibraryStore) *reclassifyPlan {
	items := ls.ListClassifiableItems()
	keys := make([]string, 0, len(items))
	for _, it := range items {
		keys = append(keys, libraryRefKey(it.MediaType, it.TmdbID))
	}
	entries := ls.GetTmdbEntriesByKeys(keys)

	plan := &reclassifyPlan{Items: make([]reclassifyItem, 0, len(items)), Targets: map[string]int{}}
	for _, it := range items {
		entry := entries[libraryRefKey(it.MediaType, it.TmdbID)]
		isTV := it.MediaType == "tv"
		level1, level2 := resolveCategoryFor(categories, isTV, entryInfo(entry))
		cat := categoryLabel(level1, level2)
		plan.Targets[cat]++
		plan.Items = append(plan.Items, reclassifyItem{
			FolderID: it.FolderID, ParentID: it.ParentID, Title: it.Title, IsTV: isTV,
			Level1: level1, Level2: level2, Category: cat,
			TmdbID: it.TmdbID, Cached: entry != nil,
		})
	}
	return plan
}

// entryInfo 从缓存实体提取分类判定所需字段（完整 TMDB 原始载荷，含 genres）。
func entryInfo(entry *store.TmdbMediaRow) map[string]any {
	if entry == nil {
		return nil
	}
	return entry.RawJSON
}

// handleCategoriesReclassifyDry 干跑统计：按当前规则与缓存流派统计各分类目标分布（零网盘调用）。
func (s *Server) handleCategoriesReclassifyDry(w http.ResponseWriter, _ *http.Request) {
	categories := s.rt.GetCategories()
	rootRef := strings.TrimSpace(s.rt.GetConfig().ActiveRootFolderID())
	if categories == nil || rootRef == "" {
		writeJSONError(w, http.StatusBadRequest, "请先配置媒体库根目录与分类规则")
		return
	}
	ls := s.rt.GetLibraryStore()
	plan := s.buildReclassifyPlan(categories, ls)

	type previewItem struct {
		Title    string `json:"title"`
		Category string `json:"category"`
	}
	preview := []previewItem{}
	cats := make([]string, 0, len(plan.Targets))
	for c := range plan.Targets {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	for _, c := range cats {
		for _, it := range plan.Items {
			if len(preview) >= 50 {
				break
			}
			if it.Category == c {
				preview = append(preview, previewItem{Title: it.Title, Category: c})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total":        len(plan.Items),
		"uncached":     totalUncached(plan.Items),
		"targets":      plan.Targets,
		"preview":      preview,
		"root_missing": false,
	})
}

func totalUncached(items []reclassifyItem) int {
	n := 0
	for _, it := range items {
		if !it.Cached {
			n++
		}
	}
	return n
}

// handleCategoriesReclassify 启动后台全库归位。
func (s *Server) handleCategoriesReclassify(w http.ResponseWriter, _ *http.Request) {
	categories := s.rt.GetCategories()
	rootRef := strings.TrimSpace(s.rt.GetConfig().ActiveRootFolderID())
	if categories == nil || rootRef == "" {
		writeJSONError(w, http.StatusBadRequest, "请先配置媒体库根目录与分类规则")
		return
	}
	ls := s.rt.GetLibraryStore()
	provider := s.rt.GetStorageProvider()
	if provider == nil {
		writeJSONError(w, http.StatusInternalServerError, "存储 Provider 不可用")
		return
	}
	reclassifyMu.Lock()
	if reclassifyRun.Running {
		reclassifyMu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"started": false, "state": reclassifyRun})
		return
	}
	plan := s.buildReclassifyPlan(categories, ls)
	reclassifyRun = reclassifyProgress{
		Running: true, Total: len(plan.Items),
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	reclassifyMu.Unlock()

	go s.runReclassify(provider, ls, categories, rootRef, plan)
	writeJSON(w, http.StatusOK, map[string]any{"started": true, "total": len(plan.Items)})
}

func (s *Server) runReclassify(provider cloudfile.Provider, ls *store.LibraryStore, categories *config.CategoriesConfig, rootRef string, plan *reclassifyPlan) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[reclassify] panic：%v", r)
			reclassifyMu.Lock()
			reclassifyRun.Running = false
			reclassifyRun.Error = fmt.Sprintf("内部异常：%v", r)
			reclassifyMu.Unlock()
		}
	}()

	rootID, ok := cloudfile.ResolveFolderRefOrPath(provider, rootRef)
	if !ok {
		reclassifyMu.Lock()
		reclassifyRun.Running = false
		reclassifyRun.Error = "媒体库根目录不存在：" + rootRef
		reclassifyMu.Unlock()
		return
	}

	// 0. 按 TMDB 最新详情重新判定全部分类（绕缓存强制请求，结果落缓存）。
	// TMDB 流派可能变化、缓存最多滞后一个 TTL，权威判定必须现查；
	// TMDB 无严格 QPS（客户端自带 429 重试），全量刷新成本可忽略。失败保持原判定。
	if tmdbClient := s.rt.GetTmdbClient(); tmdbClient != nil {
		log.Printf("[reclassify] 按 TMDB 最新详情刷新 %d 项分类判定...", len(plan.Items))
		for i := range plan.Items {
			it := &plan.Items[i]
			mtype := mediaparser.MediaMovie
			if it.IsTV {
				mtype = mediaparser.MediaTV
			}
			if info := tmdbClient.GetByIDFresh(mtype, int(it.TmdbID)); len(info) > 0 {
				it.Level1, it.Level2 = resolveCategoryFor(categories, it.IsTV, info)
				it.Category = categoryLabel(it.Level1, it.Level2)
				it.Cached = true
			}
		}
	}

	// 1. 当前父目录：优先用库中记录（零网盘读取）；有缺失时回退一次遍历定位并回填
	curByID := map[string]string{}
	needWalk := false
	for _, it := range plan.Items {
		if it.ParentID == "" {
			needWalk = true
			break
		}
	}
	if needWalk {
		log.Printf("[reclassify] 存在未知父目录的条目，遍历媒体库定位（之后无需再遍历）")
		parentMap := map[string]string{}
		walkAllFolders(provider, rootID, parentMap, 0)
		curByID = parentMap
		if ls != nil {
			for i := range plan.Items {
				it := &plan.Items[i]
				if it.ParentID == "" {
					if p, ok := curByID[it.FolderID]; ok && p != "" {
						it.ParentID = p
						ls.UpdateMediaParentID(it.FolderID, p)
					}
				}
			}
		}
	}

	// 2. 幂等创建计划涉及的分类目录链
	parentByCategory := map[string]string{}
	for _, it := range plan.Items {
		if _, ok := parentByCategory[it.Category]; ok {
			continue
		}
		parent, err := ensureCategoryChainLevels(provider, rootID, it.Level1, it.Level2)
		if err != nil {
			reason := fmt.Sprintf("创建分类目录 %s 失败：%v", it.Category, err)
			log.Printf("[reclassify] %s", reason)
			reclassifyMu.Lock()
			reclassifyFailures(&reclassifyRun, reason)
			reclassifyMu.Unlock()
			continue
		}
		parentByCategory[it.Category] = parent
	}

	// 3. 逐条判定并移动
	// 反查表：父目录 ID → 分类名（已归位明细展示「原分类 → 新分类」用）
	categoryByParent := map[string]string{}
	for cat, pid := range parentByCategory {
		categoryByParent[pid] = cat
	}
	var moved, failed int
	for i := range plan.Items {
		it := &plan.Items[i]
		target, ok := parentByCategory[it.Category]
		if !ok {
			failed++
			continue
		}
		cur := curByID[it.FolderID]
		if cur == "" {
			cur = it.ParentID
		}
		processed := func() {
			reclassifyMu.Lock()
			reclassifyRun.Processed++
			reclassifyRun.Moved = moved
			reclassifyRun.Failed = failed
			reclassifyMu.Unlock()
		}
		if cur == target {
			processed()
			continue
		}
		if _, err := provider.MoveFile(it.FolderID, target, ""); err != nil {
			failed++
			reason := fmt.Sprintf("%s：%v", it.Title, err)
			log.Printf("[reclassify] 移动失败 %s", reason)
			reclassifyMu.Lock()
			reclassifyFailures(&reclassifyRun, reason)
			reclassifyMu.Unlock()
		} else {
			moved++
			curByID[it.FolderID] = target
			if ls != nil {
				ls.UpdateMediaParentID(it.FolderID, target)
			}
			note := it.Title + " → " + it.Category
			if from, ok := categoryByParent[cur]; ok && from != it.Category {
				note = it.Title + "：" + from + " → " + it.Category
			}
			log.Printf("[reclassify] 已归位 %s", note)
			reclassifyMu.Lock()
			if len(reclassifyRun.Moves) < 50 {
				reclassifyRun.Moves = append(reclassifyRun.Moves, note)
			}
			reclassifyMu.Unlock()
		}
		processed()
		if moved > 0 && moved%20 == 0 {
			log.Printf("[reclassify] 进度：%d/%d（已移动 %d）", i+1, len(plan.Items), moved)
		}
	}

	reclassifyMu.Lock()
	reclassifyRun.Running = false
	reclassifyRun.Moved = moved
	reclassifyRun.Failed = failed
	reclassifyRun.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	runErrors := append([]string{}, reclassifyRun.Errors...)
	if reclassifyRun.Error != "" {
		runErrors = append(runErrors, reclassifyRun.Error)
	}
	runMoves := append([]string{}, reclassifyRun.Moves...)
	reclassifyMu.Unlock()

	s.rt.TriggerStrmSync("reclassify")
	level := "SUCCESS"
	if failed > 0 {
		level = "WARNING"
	}
	details := map[string]any{
		"total": len(plan.Items), "moved": moved, "failed": failed,
	}
	if len(runMoves) > 0 {
		details["moves"] = runMoves
	}
	if len(runErrors) > 0 {
		details["errors"] = runErrors
	}
	appLog("library", "reclassify", level,
		fmt.Sprintf("全库重新归类完成：共 %d 项，移动 %d 项，失败 %d 项", len(plan.Items), moved, failed), details)
	log.Printf("[reclassify] 完成：共 %d 项，移动 %d，失败 %d", len(plan.Items), moved, failed)
}

// walkAllFolders 递归收集媒体库根下全部目录的父子关系（深度上限防环）。
func walkAllFolders(provider cloudfile.Provider, folderID string, parentMap map[string]string, depth int) {
	if depth > 8 {
		return
	}
	items, err := provider.ListAllFiles(folderID)
	if err != nil {
		log.Printf("[reclassify] 遍历目录失败 %s: %v", folderID, err)
		return
	}
	for _, it := range items {
		if !it.IsFolder() {
			continue
		}
		parentMap[it.ID] = it.ParentID
		walkAllFolders(provider, it.ID, parentMap, depth+1)
	}
}

// handleCategoriesReclassifyStatus 归位进度查询。
func (s *Server) handleCategoriesReclassifyStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, reclassifyState())
}

// handleMediaClassification 单媒体分类核对：当前分类（库中 parent_id 记录 → 分类树标签）
// 与按 TMDB 最新流派/规则应属分类。供修正识别弹窗展示「当前 vs 应属」。
func (s *Server) handleMediaClassification(w http.ResponseWriter, r *http.Request) {
	folderID := strings.TrimSpace(r.URL.Query().Get("folder_id"))
	if folderID == "" {
		writeJSONError(w, http.StatusBadRequest, "folder_id 不能为空")
		return
	}
	categories := s.rt.GetCategories()
	ls := s.rt.GetLibraryStore()
	provider := s.rt.GetStorageProvider()
	if categories == nil || provider == nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}
	mediaType, tmdbID, parentID, ok := ls.GetMediaRefByFolderID(folderID)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "媒体条目不存在")
		return
	}
	info, err := s.fetchTmdbInfoForClassification(tmdbID, mediaType)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "error": err.Error()})
		return
	}
	rootRef := strings.TrimSpace(s.rt.GetConfig().ActiveRootFolderID())
	tree := currentCategoryLabels(provider, categories, rootRef)
	if tree == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"available": false, "error": "分类目录定位失败（接口限流或目录不存在）",
		})
		return
	}
	// 当前分类优先用库中记录（零网盘请求）；无记录时查一次实际位置并回写自愈
	current := ""
	if parentID != "" {
		current = tree.labels[parentID]
	} else if cf, err := provider.GetFile(folderID); err == nil && cf.ParentID != "" {
		current = tree.labels[cf.ParentID]
		ls.UpdateMediaParentID(folderID, cf.ParentID)
	}
	expected1, expected2 := resolveCategoryFor(categories, mediaType == "tv", info)
	writeJSON(w, http.StatusOK, map[string]any{
		"available": true,
		"current":   current,
		"expected":  categoryLabel(expected1, expected2),
	})
}

// categoryTree 目录 ID → 分类标签映射（「一级」或「一级/二级」）。
type categoryTree struct {
	rootRef string
	labels  map[string]string
	at      time.Time
}

var (
	classTreeMu    sync.Mutex
	classTreeCache *categoryTree
)

// currentCategoryLabels 枚举媒体库根下的分类树建 ID→标签映射。
// 短 TTL 缓存：分类目录很少变动，避免修正识别弹窗每次都重新枚举耗 QPS；
// 分类规则改动后标签最多滞后 5 分钟。
func currentCategoryLabels(provider cloudfile.Provider, categories *config.CategoriesConfig, rootRef string) *categoryTree {
	if rootRef == "" {
		return nil
	}
	classTreeMu.Lock()
	defer classTreeMu.Unlock()
	if classTreeCache != nil && classTreeCache.rootRef == rootRef && time.Since(classTreeCache.at) < 5*time.Minute {
		return classTreeCache
	}
	rootID, ok := cloudfile.ResolveFolderRefOrPath(provider, rootRef)
	if !ok {
		return nil
	}
	children, err := provider.ListFiles(rootID, 500)
	if err != nil {
		return nil
	}
	byName := map[string]string{}
	for _, c := range children {
		if c.IsFolder() {
			byName[c.Name] = c.ID
		}
	}
	tree := &categoryTree{rootRef: rootRef, labels: map[string]string{}, at: time.Now()}
	if otherID, ok := byName[categories.Dirs.Other]; ok && otherID != "" {
		tree.labels[otherID] = categories.Dirs.Other
	}
	for _, level1 := range []string{categories.Dirs.TV, categories.Dirs.Movie} {
		if level1 == "" {
			continue
		}
		l1ID, ok := byName[level1]
		if !ok {
			continue
		}
		tree.labels[l1ID] = level1
		l1Children, err := provider.ListFiles(l1ID, 500)
		if err != nil {
			continue
		}
		for _, c := range l1Children {
			if c.IsFolder() {
				tree.labels[c.ID] = level1 + "/" + c.Name
			}
		}
	}
	classTreeCache = tree
	return tree
}

// fetchTmdbInfoForClassification 分类判定取 TMDB 最新详情（缓存最多滞后一个 TTL，
// 与批量重新归类的执行判定同标准）；最新请求失败时回退本地缓存条目。
func (s *Server) fetchTmdbInfoForClassification(tmdbID int64, mediaType string) (map[string]any, error) {
	if client := s.rt.GetTmdbClient(); client != nil {
		mtype := mediaparser.MediaMovie
		if mediaType == "tv" {
			mtype = mediaparser.MediaTV
		}
		if info := client.GetByIDFresh(mtype, int(tmdbID)); len(info) > 0 {
			return info, nil
		}
	}
	if ls := s.rt.GetLibraryStore(); ls != nil {
		if entry := ls.GetTmdbEntry(mediaType, tmdbID); entry != nil && entry.RawJSON != nil {
			return entry.RawJSON, nil
		}
	}
	return nil, fmt.Errorf("TMDB 详情不可用（tmdb_id=%d）", tmdbID)
}
