// strmsvc.go —— STRM 同步服务：
// 遍历网盘映射目录生成/更新/清理本地 .strm 文件，记录同步历史（独立 strm.db）。
// .strm 内容指向本服务 302 端点（见 strmhttp.go），播放时实时换取 115 直链。
package webui

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"meta2cloud/internal/cloudfile"
	"meta2cloud/internal/config"
	"meta2cloud/internal/pipeline"
	"meta2cloud/internal/plexserver"
	"meta2cloud/internal/u115"
	"meta2cloud/internal/webui/store"
)

// strmWalkMaxDepth 网盘映射目录遍历深度上限。
const strmWalkMaxDepth = 8

// strmSidecarMaxBytes 伴随文件（外挂字幕等）单文件下载大小上限。
const strmSidecarMaxBytes = 20 << 20

// sidecarFetcher 下载网盘小文件内容的能力（由 Pan115Provider / 测试 fake 实现）。
type sidecarFetcher interface {
	FetchBytes(pickCode string, limit int64) ([]byte, error)
}

// ingestRunFlushIdle 入库钩子 run 的合并窗口：静默该时长后收尾 run。
const ingestRunFlushIdle = 20 * time.Second

// syncTrigger 值：manual / schedule / ingest / reclassify / title-pin-adopt / reidentify
// （后三者由各操作完成后以字面量传入 TriggerStrmSync）。
const (
	triggerManual   = "manual"
	triggerSchedule = "schedule"
	triggerIngest   = "ingest"
)

type strmService struct {
	rt      *Runtime
	mu      sync.Mutex
	running bool

	// ── 302 直链层：正/负缓存与并发信号量（见 strmhttp.go）──
	strmLinkCache *directLinkCache
	strmNegMu     sync.Mutex
	strmNeg       map[string]negativeEntry
	strmFetchSem  chan struct{}
	strmFetchWait time.Duration // 测试可调
	strmNegTTL    time.Duration

	// 入库钩子累积缓冲（同一批入库合并为一个 run）
	hookMu      sync.Mutex
	hookRunID   int64
	hookStats   strmRunStats
	hookChanges *strmChanges
	hookLastAt  time.Time
	hookFlusher *time.Timer
}

type strmRunStats struct {
	Created int
	Updated int
	Removed int
	Skipped int
}

// strmChanges 收集一轮同步中发生增/改/删的文件所属分类目录名
// （取自 categories.yaml 的一级目录名与各分类名，与路径段求交），
// 用于 strm.refresh_plex 开启时通知 Plex 只刷对应媒体库。
type strmChanges struct {
	names map[string]struct{}
	cats  map[string]struct{}
}

// newStrmChanges 由分类配置构建收集器（cats 为 nil 时仍可用，只是匹配不到分类）。
func newStrmChanges(cats *config.CategoriesConfig) *strmChanges {
	sc := &strmChanges{names: map[string]struct{}{}, cats: map[string]struct{}{}}
	if cats == nil {
		return sc
	}
	for _, d := range []string{cats.Dirs.TV, cats.Dirs.Movie, cats.Dirs.Other} {
		if d != "" {
			sc.names[d] = struct{}{}
		}
	}
	for _, r := range cats.TV {
		if r.Name != "" {
			sc.names[r.Name] = struct{}{}
		}
	}
	for _, r := range cats.Movie {
		if r.Name != "" {
			sc.names[r.Name] = struct{}{}
		}
	}
	return sc
}

// add 记录一条相对路径命中的分类名（nil 接收者安全；兼容 / 与本机分隔符）。
func (sc *strmChanges) add(relPath string) {
	if sc == nil {
		return
	}
	for _, seg := range strings.FieldsFunc(relPath, func(r rune) bool { return r == '/' || r == os.PathSeparator }) {
		if _, ok := sc.names[seg]; ok {
			sc.cats[seg] = struct{}{}
		}
	}
}

// list 返回本轮变更涉及的分类名（有序，保证日志与匹配稳定）。
func (sc *strmChanges) list() []string {
	if sc == nil || len(sc.cats) == 0 {
		return nil
	}
	out := make([]string, 0, len(sc.cats))
	for c := range sc.cats {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// ── URL 构造 ────────────────────────────────────────────────

// StrmSign 计算 302 访问签名（url_secret 非空时启用）。
func StrmSign(urlSecret, pickCode string) string {
	sum := sha256.Sum256([]byte(pickCode + ":" + urlSecret))
	return hex.EncodeToString(sum[:])[:32]
}

// BuildStrmURL 构造写入 .strm 的播放地址。
// 携带 file_name 让播放器/媒体服务器无需读取文件体即可识别扩展名（pickname 格式）。
// pickcode 统一转小写，与服务端 302 校验（先 lowercase 再验签）保持一致。
func BuildStrmURL(urlBase, urlSecret, pickCode, fileName string) string {
	pickCode = strings.ToLower(strings.TrimSpace(pickCode))
	u := strings.TrimRight(urlBase, "/") + "/strm/redirect?pickcode=" + url.QueryEscape(pickCode)
	if fileName != "" {
		u += "&file_name=" + url.QueryEscape(fileName)
	}
	if urlSecret != "" {
		u += "&sign=" + StrmSign(urlSecret, pickCode)
	}
	return u
}

func strmMediaExtSet(cfg *config.Config) map[string]struct{} {
	set := map[string]struct{}{}
	for _, ext := range cfg.Strm.MediaExt {
		ext = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ext), "."))
		if ext != "" {
			set[ext] = struct{}{}
		}
	}
	return set
}

func strmSidecarExtSet(cfg *config.Config) map[string]struct{} {
	set := map[string]struct{}{}
	for _, ext := range cfg.Strm.SidecarExt {
		ext = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ext), "."))
		if ext != "" {
			set[ext] = struct{}{}
		}
	}
	return set
}

func (rt *Runtime) pickCodeOf(f cloudfile.CloudFile) string {
	pc, _ := f.Extra["pick_code"].(string)
	return pc
}

// ── 同步入口 ────────────────────────────────────────────────

// TriggerStrmSync 异步触发一次全量同步（增量写库，已有且未变化的跳过）。
// 返回 false 表示已有同步在进行。
func (rt *Runtime) TriggerStrmSync(trigger string) bool {
	rt.strmSvc.mu.Lock()
	if rt.strmSvc.running {
		rt.strmSvc.mu.Unlock()
		return false
	}
	rt.strmSvc.running = true
	rt.strmSvc.mu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[strm] 同步 panic：%v", r)
			}
			rt.strmSvc.mu.Lock()
			rt.strmSvc.running = false
			rt.strmSvc.mu.Unlock()
		}()
		rt.runStrmSync(trigger)
	}()
	return true
}

// StrmSyncRunning 是否正在同步。
func (rt *Runtime) StrmSyncRunning() bool {
	rt.strmSvc.mu.Lock()
	defer rt.strmSvc.mu.Unlock()
	return rt.strmSvc.running
}

func (rt *Runtime) runStrmSync(trigger string) {
	cfg := rt.GetConfig()
	st := rt.GetStrmStore()
	if len(cfg.Strm.Mappings) == 0 {
		log.Printf("[strm] 未配置路径映射，跳过同步")
		return
	}
	if cfg.Strm.URLBase == "" {
		log.Printf("[strm] strm.url_base 未配置，无法生成播放地址，跳过同步")
		return
	}
	provider := rt.GetStorageProvider()
	if provider == nil {
		log.Printf("[strm] 存储 Provider 不可用，跳过同步")
		return
	}

	runID := st.StartSyncRun(trigger)
	stats := strmRunStats{}
	changes := newStrmChanges(rt.GetCategories())
	var syncErr error

	for _, mapping := range cfg.Strm.Mappings {
		created, updated, removed, skipped, err := rt.syncMapping(provider, st, cfg, mapping, runID, changes)
		stats.Created += created
		stats.Updated += updated
		stats.Removed += removed
		stats.Skipped += skipped
		if err != nil {
			syncErr = fmt.Errorf("映射 %s → %s：%w", mapping.PanFolderID, mapping.LocalPath, err)
			log.Printf("[strm] %s", syncErr)
			break // 单个映射失败即停，避免误删另一映射的失效文件
		}
	}
	// 保守原则：任一映射遍历失败时跳过孤儿对账等删除类操作，宁可不删不误删
	if syncErr == nil {
		stats.Removed += rt.sweepOrphanStrmFiles(st, cfg.Strm.Mappings, runID)
	}
	st.FinishSyncRun(runID, stats.Created, stats.Updated, stats.Removed, stats.Skipped, rt.errText(syncErr))
	log.Printf("[strm] 同步完成（%s）：新增 %d 更新 %d 删除 %d 跳过 %d",
		trigger, stats.Created, stats.Updated, stats.Removed, stats.Skipped)
	if syncErr == nil {
		rt.NotifyServiceHealth("115", true, "")
		rt.notifyPlexRefresh(stats.Created+stats.Updated+stats.Removed, changes)
	} else if u115.IsAuth(syncErr) {
		// 仅 115 授权失效才上报健康状态；瞬时网络/限流错误不告警
		rt.NotifyServiceHealth("115", false, "STRM 同步："+syncErr.Error())
	}
}

// errText 错误转字符串（nil → 空串）。
func (rt *Runtime) errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// notifyPlexRefresh strm 有实际变更后通知 Plex 扫描媒体库（strm.refresh_plex 开启时生效）。
// 变更涉及的分类名与 Plex 库名一致时只刷对应库，对不上则回退全部电影/剧集库。
// 异步执行且失败只记日志——不写进同步错误，避免触发保守跳过删除对账。
func (rt *Runtime) notifyPlexRefresh(changed int, changes *strmChanges) {
	if changed <= 0 || !rt.GetConfig().Strm.RefreshPlex {
		return
	}
	client := rt.GetPlexClient()
	if client == nil { // Plex 未配对
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				rt.appLog("strm", "plex_notify", "ERROR", fmt.Sprintf("通知 Plex 时程序异常：%v", r), nil)
				log.Printf("[strm] 通知 Plex 时程序异常：%v", r)
			}
		}()
		sections, err := client.Sections()
		if err != nil {
			rt.appLog("strm", "plex_notify", "ERROR", "通知 Plex 失败（获取媒体库列表）："+err.Error(),
				map[string]any{"changed": changed})
			log.Printf("[strm] 通知 Plex 失败（获取媒体库列表）：%v", err)
			if errors.Is(err, plexserver.ErrUnauthorized) {
				rt.NotifyServiceHealth("Plex", false, err.Error())
			}
			return
		}
		targets := make([]plexserver.Section, 0, len(sections))
		for _, s := range sections {
			if slices.Contains(changes.list(), s.Title) {
				targets = append(targets, s)
			}
		}
		if len(targets) == 0 { // 库名与分类名对不上：宁可多刷不漏刷
			targets = sections
		}
		failed := 0
		for _, s := range targets {
			if err := client.RefreshSection(s.Key); err != nil {
				failed++
				rt.appLog("strm", "plex_notify", "ERROR", fmt.Sprintf("通知 Plex 刷新媒体库 %s 失败：%v", s.Title, err),
					map[string]any{"section": s.Title})
				log.Printf("[strm] 通知 Plex 刷新媒体库 %s 失败：%v", s.Title, err)
				if errors.Is(err, plexserver.ErrUnauthorized) {
					rt.NotifyServiceHealth("Plex", false, err.Error())
				}
			}
		}
		cats := "全部媒体库"
		if changes != nil && len(changes.list()) > 0 {
			cats = strings.Join(changes.list(), "、")
		}
		msg := fmt.Sprintf("已通知 Plex 刷新 %d 个媒体库（变更分类：%s）", len(targets), cats)
		level := "SUCCESS"
		if failed > 0 {
			level = "WARN"
			msg += fmt.Sprintf("，%d 个失败", failed)
		} else {
			rt.NotifyServiceHealth("Plex", true, "")
		}
		rt.appLog("strm", "plex_notify", level, msg,
			map[string]any{"targets": len(targets), "failed": failed, "changed": changed})
		log.Printf("[strm] %s", msg)
	}()
}

// syncMapping 同步单个映射，返回统计。遍历成功后清理该映射下网盘已不存在的 .strm。
// ch 非 nil 时同时记录变更文件命中的分类名。
func (rt *Runtime) syncMapping(provider cloudfile.Provider, st *store.StrmStore, cfg *config.Config, mapping config.StrmMapping, runID int64, ch *strmChanges) (created, updated, removed, skipped int, err error) {
	extSet := strmMediaExtSet(cfg)
	sidecarSet := map[string]struct{}{}
	var fetcher sidecarFetcher
	if cfg.Strm.FetchSidecar {
		sidecarSet = strmSidecarExtSet(cfg)
		if f, ok := provider.(sidecarFetcher); ok {
			fetcher = f
		}
	}
	localRoot := filepath.Clean(mapping.LocalPath)
	seen := map[string]bool{}

	// 配置值为网盘根起算的相对路径（如 "Media/剧集"，兼容旧 ID），使用处解析为目录 ID
	panID, ok := cloudfile.ResolveFolderRefOrPath(provider, mapping.PanFolderID)
	if !ok {
		return 0, 0, 0, 0, fmt.Errorf("网盘目录 %q 不存在", mapping.PanFolderID)
	}

	var walk func(folderID, relPath string, depth int) error
	walk = func(folderID, relPath string, depth int) error {
		if depth > strmWalkMaxDepth {
			return nil
		}
		items, err := provider.ListAllFiles(folderID)
		if err != nil {
			return fmt.Errorf("列举网盘目录 %s 失败：%w", folderID, err)
		}
		for _, item := range items {
			if item.IsFolder() {
				// 蓝光原盘目录结构无法用单个 strm 表达，跳过
				if strings.EqualFold(item.Name, "BDMV") {
					continue
				}
				if err := walk(item.ID, path.Join(relPath, item.Name), depth+1); err != nil {
					return err
				}
				continue
			}
			ext := strings.TrimPrefix(item.Extension(), ".")
			pickCode := rt.pickCodeOf(item)
			_, isMedia := extSet[ext]
			_, isSidecar := sidecarSet[ext]
			switch {
			case isMedia:
				if pickCode == "" {
					continue
				}
				localPath := rt.strmLocalPath(localRoot, relPath, item.Name)
				switch rt.upsertStrmFile(st, cfg, mapping, item, relPath, pickCode, localRoot, runID) {
				case "created":
					created++
					ch.add(relPath)
				case "updated":
					updated++
					ch.add(relPath)
				default:
					skipped++
				}
				seen[localPath] = true
			case isSidecar:
				// 外挂字幕/元数据/图片：下载到本地与 .strm 同目录（镜像网盘命名）
				if fetcher == nil || pickCode == "" {
					continue
				}
				localPath := rt.sidecarLocalPath(localRoot, relPath, item.Name)
				switch rt.upsertSidecarFile(st, item, relPath, pickCode, localPath, fetcher, runID) {
				case "created":
					created++
					ch.add(relPath)
				case "updated":
					updated++
					ch.add(relPath)
				default:
					skipped++
				}
				seen[localPath] = true
			}
		}
		return nil
	}
	if err := walk(panID, "", 0); err != nil {
		return created, updated, removed, skipped, err
	}

	// 清理：该映射前缀下、本次遍历未见的记录（.strm 与伴随文件）→ 删除本地文件与记录
	for _, rec := range st.ListStrmFilesByLocalPrefix(localRoot + string(os.PathSeparator)) {
		if seen[rec.LocalPath] {
			continue
		}
		if err := os.Remove(rec.LocalPath); err != nil && !os.IsNotExist(err) {
			st.InsertSyncFileDetail(runID, rec.FileName, rec.PanPath, rec.LocalPath, "removed", false, err.Error())
			continue
		}
		st.DeleteStrmFile(rec.ID)
		st.InsertSyncFileDetail(runID, rec.FileName, rec.PanPath, rec.LocalPath, "removed", true, "")
		removed++
		ch.add(strings.TrimPrefix(rec.LocalPath, localRoot+string(os.PathSeparator)))
	}

	// 自内向外清理因网盘目录改名/合并而残留的本地空目录
	rt.pruneEmptyDirs(localRoot)
	return created, updated, removed, skipped, nil
}

// pruneEmptyDirs 自内向外删除本地根目录下的空目录（仅删除真正为空的目录；根目录本身保留）。
func (rt *Runtime) pruneEmptyDirs(root string) {
	var dirs []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || p == root {
			return nil
		}
		dirs = append(dirs, p)
		return nil
	})
	// 路径倒序保证子目录先于父目录处理；os.Remove 仅对空目录生效
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, dir := range dirs {
		if err := os.Remove(dir); err == nil {
			log.Printf("[strm] 清理本地空目录：%s", dir)
		}
	}
}

// sweepOrphanStrmFiles 全局孤儿对账：不属于任何映射本地根的记录
// （历史映射位置变更后的残留）删除本地文件与记录，返回清理数量。
// 每次同步末尾执行，映射换位置后下一次同步即自动清干净。
func (rt *Runtime) sweepOrphanStrmFiles(st *store.StrmStore, mappings []config.StrmMapping, runID int64) int {
	roots := make([]string, 0, len(mappings))
	for _, m := range mappings {
		if root := filepath.Clean(m.LocalPath); root != "" && root != "." {
			roots = append(roots, root)
		}
	}
	// 无有效受管根（配置异常/未就绪）时不动任何记录，避免全表误判为孤儿
	if len(roots) == 0 {
		return 0
	}
	removed := 0
	for _, rec := range st.ListAllStrmFiles() {
		if rt.isUnderAnyLocalRoot(rec.LocalPath, roots) {
			continue
		}
		if err := os.Remove(rec.LocalPath); err != nil && !os.IsNotExist(err) {
			st.InsertSyncFileDetail(runID, rec.FileName, rec.PanPath, rec.LocalPath, "removed", false, err.Error())
			continue
		}
		rt.pruneEmptyDirParents(filepath.Dir(rec.LocalPath), roots)
		st.DeleteStrmFile(rec.ID)
		st.InsertSyncFileDetail(runID, rec.FileName, rec.PanPath, rec.LocalPath, "removed", true, "")
		log.Printf("[strm] 清理映射外残留：%s", rec.LocalPath)
		removed++
	}
	return removed
}

// isUnderAnyLocalRoot 判断本地路径是否位于任一受管根之下（按路径段前缀比较）。
func (rt *Runtime) isUnderAnyLocalRoot(localPath string, roots []string) bool {
	for _, root := range roots {
		if localPath == root || strings.HasPrefix(localPath, root+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

// pruneEmptyDirParents 孤儿文件删除后自下而上清理空目录：
// 非空、抵达受管根或文件系统顶层即停（os.Remove 仅对空目录生效，天然安全）。
func (rt *Runtime) pruneEmptyDirParents(bottom string, roots []string) {
	dir := filepath.Clean(bottom)
	for dir != "." && dir != string(os.PathSeparator) && len(dir) > 1 && !rt.isUnderAnyLocalRoot(dir, roots) {
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

func (rt *Runtime) strmLocalPath(localRoot, relPath, fileName string) string {
	name := fileName
	if ext := path.Ext(fileName); ext != "" {
		name = strings.TrimSuffix(fileName, ext) + ".strm"
	} else {
		name = fileName + ".strm"
	}
	return rt.sidecarLocalPath(localRoot, relPath, name)
}

// sidecarLocalPath 伴随文件保持网盘原始命名（整理阶段已完成字幕改名与语言标注）。
func (rt *Runtime) sidecarLocalPath(localRoot, relPath, fileName string) string {
	if relPath == "" {
		return filepath.Join(localRoot, fileName)
	}
	return filepath.Join(localRoot, filepath.FromSlash(relPath), fileName)
}

// upsertStrmFile 为单个网盘视频文件写 .strm 并更新记录。
// 返回 created / updated / skipped。
func (rt *Runtime) upsertStrmFile(st *store.StrmStore, cfg *config.Config, mapping config.StrmMapping,
	f cloudfile.CloudFile, relPath, pickCode, localRoot string, runID int64) string {

	localPath := rt.strmLocalPath(localRoot, relPath, f.Name)
	panPath := "/" + path.Join(relPath, f.Name)

	size := int64(0)
	if f.Size != nil {
		size = *f.Size
	}
	record := store.StrmFile{
		PickCode:  pickCode,
		FileID:    f.ID,
		PanPath:   panPath,
		LocalPath: localPath,
		FileName:  f.Name,
		Size:      &size,
	}

	strmURL := BuildStrmURL(cfg.Strm.URLBase, cfg.Strm.URLSecret, pickCode, f.Name)
	action := st.UpsertStrmFile(record)
	if action == "skipped" {
		// 记录未变时仍比对内容：本地文件被删则补写，
		// url_base/url_secret 等变更导致内容不一致则重写
		data, readErr := os.ReadFile(localPath)
		switch {
		case readErr == nil && strings.TrimSpace(string(data)) == strmURL:
			return "skipped"
		case os.IsNotExist(readErr):
			action = "created"
		default:
			action = "updated"
		}
	}

	if err := rt.writeStrmFile(localPath, strmURL); err != nil {
		st.InsertSyncFileDetail(runID, f.Name, panPath, localPath, action, false, err.Error())
		return action
	}
	st.InsertSyncFileDetail(runID, f.Name, panPath, localPath, action, true, "")
	return action
}

func (rt *Runtime) writeStrmFile(localPath, content string) error {
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return fmt.Errorf("创建目录失败：%w", err)
	}
	return os.WriteFile(localPath, []byte(content+"\n"), 0o644)
}

// upsertSidecarFile 下载网盘伴随文件（外挂字幕等）到本地 .strm 同目录，
// 保持网盘原始命名（整理阶段已完成字幕改名与语言标注）。
// 增量策略与 .strm 一致：记录未变且本地文件大小一致则跳过，避免重复下载。
func (rt *Runtime) upsertSidecarFile(st *store.StrmStore, f cloudfile.CloudFile, relPath, pickCode, localPath string, fetcher sidecarFetcher, runID int64) string {
	panPath := "/" + path.Join(relPath, f.Name)
	size := int64(0)
	if f.Size != nil {
		size = *f.Size
	}
	record := store.StrmFile{
		PickCode:  pickCode,
		FileID:    f.ID,
		PanPath:   panPath,
		LocalPath: localPath,
		FileName:  f.Name,
		Size:      &size,
	}

	action := st.UpsertStrmFile(record)
	if action == "skipped" {
		if fi, err := os.Stat(localPath); err == nil && (f.Size == nil || fi.Size() == *f.Size) {
			return "skipped"
		}
		if _, err := os.Stat(localPath); os.IsNotExist(err) {
			action = "created"
		} else {
			action = "updated"
		}
	}

	data, err := fetcher.FetchBytes(pickCode, strmSidecarMaxBytes)
	if err != nil {
		st.InsertSyncFileDetail(runID, f.Name, panPath, localPath, action, false, "下载失败："+err.Error())
		return action
	}
	if err := rt.writeSidecarFile(localPath, data); err != nil {
		st.InsertSyncFileDetail(runID, f.Name, panPath, localPath, action, false, err.Error())
		return action
	}
	st.InsertSyncFileDetail(runID, f.Name, panPath, localPath, action, true, "")
	return action
}

func (rt *Runtime) writeSidecarFile(localPath string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return fmt.Errorf("创建目录失败：%w", err)
	}
	return os.WriteFile(localPath, data, 0o644)
}

// ── 入库钩子 ────────────────────────────────────────────────

// HandleVideoIngested 整理流水线入库成功后的回调（单文件实时生成 STRM）。
// 通过 TargetFolder 向上回溯父目录链定位映射根，计算相对路径。
// 返回 nil 表示 STRM 已生成；返回 error 表示生成失败或目标不在任何映射目录下。
func (rt *Runtime) HandleVideoIngested(result pipeline.ProcessResult) error {
	if result.Status != "ok" || result.TargetFolder == nil {
		return fmt.Errorf("整理结果无效")
	}
	pickCode := rt.pickCodeOf(result.File)
	if pickCode == "" {
		return fmt.Errorf("缺少 pick_code")
	}
	cfg := rt.GetConfig()
	if !cfg.Strm.Enabled || cfg.Strm.URLBase == "" {
		return fmt.Errorf("STRM 未启用")
	}
	st := rt.GetStrmStore()
	provider := rt.GetStorageProvider()
	if provider == nil {
		return fmt.Errorf("存储 Provider 不可用")
	}

	mapping, relPath, ok := rt.locateMapping(provider, cfg, *result.TargetFolder, result.TargetChain)
	if !ok {
		return fmt.Errorf("目标目录不在任何 STRM 映射下")
	}
	localRoot := filepath.Clean(mapping.LocalPath)

	// 视频同目录的伴随文件（外挂字幕等，整理阶段已随视频改名就位）
	var fetcher sidecarFetcher
	sidecarSet := map[string]struct{}{}
	if cfg.Strm.FetchSidecar {
		sidecarSet = strmSidecarExtSet(cfg)
		if f, ok := provider.(sidecarFetcher); ok {
			fetcher = f
		}
	}

	rt.strmSvc.hookMu.Lock()
	defer rt.strmSvc.hookMu.Unlock()
	rt.ensureIngestRunLocked()
	action := rt.upsertStrmFile(st, cfg, mapping, result.File, relPath, pickCode, localRoot, rt.strmSvc.hookRunID)
	switch action {
	case "created":
		rt.strmSvc.hookStats.Created++
		rt.strmSvc.hookChanges.add(relPath)
	case "updated":
		rt.strmSvc.hookStats.Updated++
		rt.strmSvc.hookChanges.add(relPath)
	default:
		rt.strmSvc.hookStats.Skipped++
	}
	if fetcher != nil {
		// 注意：hookMu 为非重入锁，upsertSidecarFile 内部不得再取本锁（其本身不取）
		if siblings, err := provider.ListAllFiles(result.TargetFolder.ID); err == nil {
			for _, item := range siblings {
				if item.IsFolder() {
					continue
				}
				ext := strings.TrimPrefix(item.Extension(), ".")
				if _, ok := sidecarSet[ext]; !ok {
					continue
				}
				sidePickCode := rt.pickCodeOf(item)
				if sidePickCode == "" {
					continue
				}
				sideLocal := rt.sidecarLocalPath(localRoot, relPath, item.Name)
				switch rt.upsertSidecarFile(st, item, relPath, sidePickCode, sideLocal, fetcher, rt.strmSvc.hookRunID) {
				case "created":
					rt.strmSvc.hookStats.Created++
					rt.strmSvc.hookChanges.add(relPath)
				case "updated":
					rt.strmSvc.hookStats.Updated++
					rt.strmSvc.hookChanges.add(relPath)
				default:
					rt.strmSvc.hookStats.Skipped++
				}
			}
		}
	}
	rt.strmSvc.hookLastAt = time.Now()
	rt.scheduleIngestFlush()
	return nil
}

// locateMapping 从目标文件夹沿 ParentID 向上回溯，找到包含它的映射与相对目录路径。
func (rt *Runtime) locateMapping(provider cloudfile.Provider, cfg *config.Config, target cloudfile.CloudFile, targetChain []cloudfile.CloudFile) (config.StrmMapping, string, bool) {
	mappingByRoot := map[string]config.StrmMapping{}
	mappingByPath := map[string]config.StrmMapping{}
	for _, m := range cfg.Strm.Mappings {
		mappingByRoot[m.PanFolderID] = m
		if key := cloudfile.NormalizeFolderPath(m.PanFolderID); key != "" {
			mappingByPath[key] = m
		}
	}
	// 0) 路径型映射优先：映射值为网盘根起算的相对路径，与链条节点的名字路径前缀匹配
	chainPath := ""
	for i := range targetChain {
		seg := cloudfile.NormalizeFolderPath(targetChain[i].Name)
		if chainPath == "" {
			chainPath = seg
		} else {
			chainPath += "/" + seg
		}
		if m, ok := mappingByPath[chainPath]; ok {
			parts := make([]string, 0, len(targetChain)-i-1)
			for _, node := range targetChain[i+1:] {
				parts = append(parts, node.Name)
			}
			return m, strings.Join(parts, "/"), true
		}
	}
	// 1) ID 型映射：管线传链（媒体库根/一级/二级/剧名/Season），本地匹配映射根，零网络调用
	chainIdx := -1
	for i := range targetChain {
		if _, ok := mappingByRoot[targetChain[i].ID]; ok {
			chainIdx = i
			break
		}
	}
	if chainIdx >= 0 {
		m := mappingByRoot[targetChain[chainIdx].ID]
		parts := make([]string, 0, len(targetChain)-chainIdx-1)
		for _, node := range targetChain[chainIdx+1:] {
			parts = append(parts, node.Name)
		}
		return m, strings.Join(parts, "/"), true
	}
	// 2) 回退：链条不可用（映射根在链条之外等特殊情况）时逐级向上查询网盘
	// 目录名链（自顶向下）：映射根之下、目标文件夹之上
	var chain []string
	cur := target
	for depth := 0; depth <= strmWalkMaxDepth; depth++ {
		if m, ok := mappingByRoot[cur.ID]; ok {
			relPath := ""
			if len(chain) > 0 {
				relPath = strings.Join(chain, "/")
			}
			return m, relPath, true
		}
		if cur.ParentID == "" || cur.ParentID == "0" {
			break
		}
		chain = append([]string{cur.Name}, chain...)
		parent, err := provider.GetFile(cur.ParentID)
		if err != nil {
			return config.StrmMapping{}, "", false
		}
		cur = parent
	}
	return config.StrmMapping{}, "", false
}

// ensureIngestRunLocked 取当前累积 run（窗口内复用，否则新建）。需已持 hookMu。
func (rt *Runtime) ensureIngestRunLocked() {
	st := rt.GetStrmStore()
	now := time.Now()
	if rt.strmSvc.hookRunID != 0 && now.Sub(rt.strmSvc.hookLastAt) <= ingestRunFlushIdle {
		return
	}
	rt.strmSvc.flushIngestRunLocked()
	rt.strmSvc.hookRunID = st.StartSyncRun(triggerIngest)
	rt.strmSvc.hookStats = strmRunStats{}
	rt.strmSvc.hookChanges = newStrmChanges(rt.GetCategories())
}

// scheduleIngestFlush 窗口静默后收尾入库 run。
func (rt *Runtime) scheduleIngestFlush() {
	if rt.strmSvc.hookFlusher != nil {
		rt.strmSvc.hookFlusher.Stop()
	}
	rt.strmSvc.hookFlusher = time.AfterFunc(ingestRunFlushIdle, func() {
		rt.strmSvc.hookMu.Lock()
		defer rt.strmSvc.hookMu.Unlock()
		rt.strmSvc.flushIngestRunLocked()
	})
}

// flushIngestRunLocked 收尾当前入库 run。需已持 hookMu。
func (s *strmService) flushIngestRunLocked() {
	if s.hookRunID == 0 {
		return
	}
	if st := s.rt.GetStrmStore(); st != nil {
		st.FinishSyncRun(s.hookRunID, s.hookStats.Created, s.hookStats.Updated, s.hookStats.Removed, s.hookStats.Skipped, "")
	}
	changed := s.hookStats.Created + s.hookStats.Updated + s.hookStats.Removed
	changes := s.hookChanges
	s.hookRunID = 0
	s.hookStats = strmRunStats{}
	s.hookChanges = nil
	s.rt.notifyPlexRefresh(changed, changes)
}

// ── HTTP 处理器（/api/strm/*） ───────────────────────────────

func (s *Server) queryIntDefault(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func (s *Server) syncRunPayload(r store.StrmSyncRun) map[string]any {
	return map[string]any{
		"id": r.ID, "started_at": r.StartedAt, "finished_at": r.FinishedAt,
		"trigger": r.Trigger, "created": r.Created, "updated": r.Updated,
		"removed": r.Removed, "skipped": r.Skipped, "error": r.Error,
	}
}

func (s *Server) handleStrmSyncStatus(w http.ResponseWriter, _ *http.Request) {
	cfg := s.rt.GetConfig()
	st := s.rt.GetStrmStore()
	payload := map[string]any{
		"enabled":     cfg.Strm.Enabled,
		"url_base":    cfg.Strm.URLBase,
		"mappings":    len(cfg.Strm.Mappings),
		"running":     s.rt.StrmSyncRunning(),
		"total_files": 0,
		"last_run":    nil,
	}
	if st != nil {
		payload["total_files"] = st.CountStrmFiles()
		if last := st.GetLastFinishedSyncRun(); last != nil {
			payload["last_run"] = s.syncRunPayload(*last)
		}
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleStrmSyncTrigger(w http.ResponseWriter, _ *http.Request) {
	cfg := s.rt.GetConfig()
	if !cfg.Strm.Enabled {
		writeJSONError(w, http.StatusBadRequest, "STRM 功能未启用，请先在配置中开启")
		return
	}
	if len(cfg.Strm.Mappings) == 0 {
		writeJSONError(w, http.StatusBadRequest, "未配置路径映射")
		return
	}
	if cfg.Strm.URLBase == "" {
		writeJSONError(w, http.StatusBadRequest, "strm.url_base 未配置")
		return
	}
	if !s.rt.TriggerStrmSync(triggerManual) {
		writeJSONError(w, http.StatusConflict, "同步已在进行中")
		return
	}
	s.rt.appLog("strm", "strm_sync_triggered", "INFO", "手动触发 STRM 同步", nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "同步已启动"})
}

func (s *Server) handleStrmSyncRuns(w http.ResponseWriter, r *http.Request) {
	st := s.rt.GetStrmStore()
	limit := s.queryIntDefault(r, "limit", 20)
	offset := s.queryIntDefault(r, "offset", 0)
	runs, total := st.ListSyncRuns(limit, offset)
	items := make([]map[string]any, 0, len(runs))
	for _, run := range runs {
		items = append(items, s.syncRunPayload(run))
	}
	writeJSON(w, http.StatusOK, map[string]any{"total": total, "runs": items})
}

func (s *Server) handleStrmSyncRunFiles(w http.ResponseWriter, r *http.Request) {
	st := s.rt.GetStrmStore()
	runID, err := strconv.ParseInt(r.PathValue("runID"), 10, 64)
	if err != nil || runID <= 0 {
		writeJSONError(w, http.StatusBadRequest, "无效的 run ID")
		return
	}
	files := st.ListSyncFiles(runID, s.queryIntDefault(r, "limit", 500))
	items := make([]map[string]any, 0, len(files))
	for _, f := range files {
		items = append(items, map[string]any{
			"id": f.ID, "run_id": f.RunID, "file_name": f.FileName,
			"pan_path": f.PanPath, "local_path": f.LocalPath,
			"action": f.Action, "success": f.Success, "error": f.Error,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": items})
}
