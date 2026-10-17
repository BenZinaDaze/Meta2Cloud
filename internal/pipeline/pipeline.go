// Package pipeline Meta2Cloud 完整流水线（对应 Python 版 core/pipeline.py）。
//
// 流程：扫描 → 解析 → TMDB（整剧+单集）→ NFO → 文件夹 → 图片 → 移动。
package pipeline

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"meta2cloud/internal/cloudfile"
	"meta2cloud/internal/config"
	"meta2cloud/internal/jsonx"
	"meta2cloud/internal/mediaparser"
	"meta2cloud/internal/nfo"
	"meta2cloud/internal/organizer"
	"meta2cloud/internal/tmdb"
)

// ProcessResult 单文件处理结果。
type ProcessResult struct {
	File         cloudfile.CloudFile
	Status       string // "ok" | "skipped" | "failed"
	Reason       string
	TargetFolder *cloudfile.CloudFile
	// TargetChain 自媒体库根到目标目录的节点链（含 ID 与实际目录名）。
	// 整理时链条本来就在手，STRM 单文件钩子用它本地计算相对路径，免网络反查。
	TargetChain []cloudfile.CloudFile
	NfoUploaded bool
	Moved       bool
}

// PipelineSummary 汇总。
type PipelineSummary struct {
	OKCount      int
	SkippedCount int
	FailedCount  int
}

// IngestRecord 入库历史记录（webui 层存储）。
type IngestRecord struct {
	MediaType     string
	TmdbID        int
	Title         string
	OriginalTitle string
	Year          string
	Season        *int
	Episode       *int
	EpisodeTitle  string
	PosterPath    string
	DriveFolderID string
	OriginalName  string
	Status        string // "success" | "no_tmdb" | "failed"
	ErrorMessage  string
}

// IngestStore 入库历史存储接口（webui 层实现）。
type IngestStore interface {
	RecordIngest(record IngestRecord) error
}

// TitlePinStore 标题钉扎存储接口（webui 层实现，可为 nil）。
// 记录每个媒体首次整理时的目录名，TMDB 后续改译名不再影响目录结构。
type TitlePinStore interface {
	// GetTitlePin 只读查询钉扎记录。
	GetTitlePin(mediaType string, tmdbID int64) (title, year string, ok bool)
	// ResolveTitlePin 返回钉扎标题/年份；无记录时按「最早入库历史 → fallback」落库。
	ResolveTitlePin(mediaType string, tmdbID int64, fallbackTitle, fallbackYear string) (title, year string, err error)
}

// LogFunc 日志回调 (level, message)。
type LogFunc func(level, message string)

// Pipeline 完整流水线。
type Pipeline struct {
	provider             cloudfile.Provider
	cfg                  *config.Config
	logFn                LogFunc
	dryRun               bool
	skipTmdb             bool
	skipImages           bool
	skipMetadataUpload   bool
	replaceExistingVideo bool

	organizer *organizer.MediaOrganizer

	// WebUI TMDB 缓存（可选，库不可用时为 nil）
	tmdbWriteCache tmdb.Cache
	tmdb           *tmdb.Client

	nfoGen      *nfo.Generator
	imgUploader *ImageUploader

	// 幂等去重集合
	tvshowNfoDone    map[string]bool
	seasonNfoDone    map[string]bool
	posterDone       map[string]bool
	seasonPosterDone map[string]bool
	// 剧名文件夹缓存：tmdb_id → 剧名文件夹 ID
	showFolderCache map[string]string
	// 季封面缓存："tmdb_id:season" → poster_path
	seasonPosterCache map[string]string
	// 季详情缓存："tmdb_id:season" → detail
	seasonDetailCache map[string]map[string]any
	// 字幕组匹配器
	rgMatcher *mediaparser.ReleaseGroupsMatcher
	// TG 通知收集
	notifyItems []notifyItem
	// 入库历史存储（可选）
	ingestStore IngestStore
	// 标题钉扎存储（可选）
	pinStore TitlePinStore
	// 重复目录分组来源（可选）
	dupSource DuplicateFolderSource
	// 媒体分类配置（可选；nil = 未启用分类布局）
	categories *config.CategoriesConfig
	// 分类目录缓存：父ID/目录名 → 目录ID
	categoryDirCache map[string]string
	// 入库成功回调（可选；dry-run 不触发，用于 STRM 生成等后处理，返回 error 表示失败/不适用）
	onVideoIngested func(ProcessResult) error
}

// Options Pipeline 构造参数。
type Options struct {
	DryRun     bool
	SkipTmdb   bool
	SkipImages bool
	LogFn      LogFunc
	// TmdbCache WebUI SQLite 缓存（可为 nil）
	TmdbCache tmdb.Cache
	// TmdbClient 注入自定义 TMDB 客户端（测试用；为 nil 时按配置构造）
	TmdbClient *tmdb.Client
	// IngestStore 入库历史存储（可为 nil）
	IngestStore IngestStore
	// TitlePinStore 标题钉扎存储（可为 nil）
	TitlePinStore TitlePinStore
	// DuplicateFolders 重复目录分组来源（可为 nil）
	DuplicateFolders DuplicateFolderSource
	// Categories 媒体分类配置（可为 nil = 未启用分类布局）
	Categories *config.CategoriesConfig
	// OnVideoIngested 单文件整理成功（入库）后回调，dry-run 不触发（可为 nil）；
	// 返回 nil 表示 STRM 等后处理成功
	OnVideoIngested func(ProcessResult) error
}

// New 构造流水线。
func New(provider cloudfile.Provider, cfg *config.Config, opts Options) *Pipeline {
	dryRun := opts.DryRun || cfg.Pipeline.DryRun
	skipTmdb := opts.SkipTmdb || cfg.Pipeline.SkipTMDB

	p := &Pipeline{
		provider:             provider,
		cfg:                  cfg,
		logFn:                opts.LogFn,
		dryRun:               dryRun,
		skipTmdb:             skipTmdb,
		skipImages:           opts.SkipImages,
		skipMetadataUpload:   cfg.Pipeline.SkipMetadataUpload,
		replaceExistingVideo: cfg.Pipeline.ReplaceExistingVideo,
		organizer: organizer.New(
			provider,
			cfg.ActiveRootFolderID(),
			dryRun,
		),
		tmdbWriteCache:    opts.TmdbCache,
		tvshowNfoDone:     map[string]bool{},
		seasonNfoDone:     map[string]bool{},
		posterDone:        map[string]bool{},
		seasonPosterDone:  map[string]bool{},
		showFolderCache:   map[string]string{},
		seasonPosterCache: map[string]string{},
		seasonDetailCache: map[string]map[string]any{},
		categoryDirCache:  map[string]string{},
		notifyItems:       []notifyItem{},
		ingestStore:       opts.IngestStore,
		pinStore:          opts.TitlePinStore,
		dupSource:         opts.DuplicateFolders,
		categories:        opts.Categories,
		onVideoIngested:   opts.OnVideoIngested,
	}

	if opts.TmdbClient != nil {
		p.tmdb = opts.TmdbClient
	} else if !skipTmdb && cfg.IsTmdbReady() {
		p.tmdb = tmdb.NewClient(cfg.Tmdb.APIKey, cfg.Tmdb.Language, cfg.Tmdb.Proxy, cfg.Tmdb.Timeout, opts.TmdbCache)
	}

	p.nfoGen = nfo.NewGenerator(cfg.TmdbImageBaseURL())
	if !opts.SkipImages && !dryRun {
		p.imgUploader = NewImageUploader(provider, cfg.TmdbImageBaseURL())
	}

	p.rgMatcher = mediaparser.NewReleaseGroupsMatcher(cfg.Parser.CustomReleaseGroups)
	return p
}

// ── 日志 ────────────────────────────────────────────────────

func inferLevel(message string) string {
	switch {
	case strings.Contains(message, "❌") || strings.Contains(message, "失败") || strings.Contains(message, "异常"):
		return "ERROR"
	case strings.Contains(message, "⚠") || strings.Contains(message, "跳过"):
		return "WARNING"
	case strings.Contains(message, "✓") || strings.Contains(message, "完成"):
		return "SUCCESS"
	}
	return "INFO"
}

func (p *Pipeline) log(message string, level ...string) {
	lv := ""
	if len(level) > 0 {
		lv = level[0]
	}
	if lv == "" {
		lv = inferLevel(message)
	}
	fmt.Println(message)
	if p.logFn != nil {
		p.logFn(lv, message)
	}
}

// ── 缓存工具 ────────────────────────────────────────────────

func (p *Pipeline) writeCache(path string, data map[string]any) {
	if p.tmdbWriteCache == nil || len(data) == 0 {
		return
	}
	p.tmdbWriteCache.Set(path, data, p.cfg.Tmdb.Language)
}

func tmdbIDOf(info map[string]any) string {
	id := jsonx.Int(info, "tmdb_id", 0)
	if id == 0 {
		id = jsonx.Int(info, "id", 0)
	}
	return strconv.FormatInt(id, 10)
}

func (p *Pipeline) getSeasonDetailCached(tmdbID string, seasonNum int) map[string]any {
	key := fmt.Sprintf("%s:%d", tmdbID, seasonNum)
	if _, ok := p.seasonDetailCache[key]; !ok {
		var sd map[string]any
		if p.tmdb != nil && tmdbID != "" {
			if id, err := strconv.Atoi(tmdbID); err == nil {
				sd = p.tmdb.GetSeasonDetail(id, seasonNum)
			}
		}
		if sd == nil {
			sd = map[string]any{}
		}
		p.seasonDetailCache[key] = sd
		if len(sd) > 0 {
			p.writeCache(fmt.Sprintf("/tv/%s/season/%d", tmdbID, seasonNum), sd)
		}
		if _, ok := p.seasonPosterCache[key]; !ok {
			p.seasonPosterCache[key] = jsonx.Str(sd, "poster_path")
		}
	}
	return p.seasonDetailCache[key]
}

// ── 主入口 ──────────────────────────────────────────────────

// Run 执行完整整理流程。
func (p *Pipeline) Run() PipelineSummary {
	// 扫描目录：配置值为网盘根起算的相对路径（兼容旧名字/ID），在使用处解析为目录 ID
	if p.cfg.ActiveScanFolderID() == "" {
		p.log("❌  配置错误：当前存储后端的下载目录未设置", "ERROR")
		return PipelineSummary{}
	}
	scanFolder, ok := cloudfile.ResolveFolderRefOrPath(p.provider, p.cfg.ActiveScanFolderID())
	if !ok {
		p.log(fmt.Sprintf("❌  配置错误：下载目录 %q 在网盘根目录下不存在（请检查分类配置与 115 目录）",
			p.cfg.ActiveScanFolderID()), "ERROR")
		return PipelineSummary{}
	}
	if p.cfg.ActiveRootFolderID() == "" {
		p.log("❌  配置错误：当前存储后端的媒体库根目录未设置", "ERROR")
		return PipelineSummary{}
	}
	// 无分类兜底模式：organizer 根目录（配置值为相对路径，兼容旧名字/ID）在此解析；分类模式下不使用 organizer 根
	if p.categories == nil {
		rootID, ok := cloudfile.ResolveFolderRefOrPath(p.provider, p.cfg.ActiveRootFolderID())
		if !ok {
			p.log(fmt.Sprintf("❌  配置错误：媒体库根目录 %q 在网盘根目录下不存在", p.cfg.ActiveRootFolderID()), "ERROR")
			return PipelineSummary{}
		}
		p.organizer.SetRootID(rootID)
	}

	p.log(strings.Repeat("=", 68))
	p.log("  Meta2Cloud 开始整理")
	if p.dryRun {
		p.log("  ⚠️  DRY-RUN 模式 — 不会操作网盘", "WARNING")
	}
	if p.skipTmdb || p.tmdb == nil {
		p.log("  ℹ️  跳过 TMDB — 不生成 NFO", "INFO")
	}
	if p.skipMetadataUpload {
		p.log("  ℹ️  跳过元数据上传 — 仅移动媒体文件，不上传任何 NFO 或图片", "INFO")
	}
	if p.skipImages {
		p.log("  ℹ️  跳过图片下载", "INFO")
	}
	p.log(strings.Repeat("=", 68))

	// 重复目录合并：历史译名差异导致同一媒体存在多个目录时，先收敛进钉扎目录
	p.mergeDuplicateFoldersPass()

	p.log(fmt.Sprintf("\n📂 扫描文件夹：%s（含子文件夹）", scanFolder))
	var allFiles []cloudfile.CloudFile
	_ = cloudfile.WalkAll(p.provider, scanFolder, 10, func(f cloudfile.CloudFile) error {
		allFiles = append(allFiles, f)
		return nil
	})
	var videos []cloudfile.CloudFile
	var subtitles []cloudfile.CloudFile
	for _, f := range allFiles {
		if f.IsVideo() {
			videos = append(videos, f)
		} else if p.cfg.Subtitle.Enabled && f.IsSubtitle() {
			subtitles = append(subtitles, f)
		}
	}

	// 构建字幕索引：parentID -> []CloudFile
	subtitleIndex := map[string][]cloudfile.CloudFile{}
	for _, sub := range subtitles {
		parent := sub.ParentID
		if parent == "" && len(sub.Parents) > 0 {
			parent = sub.Parents[0]
		}
		subtitleIndex[parent] = append(subtitleIndex[parent], sub)
	}

	if len(videos) == 0 {
		p.log("  （未找到视频文件）")
		p.log("\n🧹 检查并清理空子文件夹...")
		p.cleanupEmptyFolders(scanFolder, "", true)
		p.log("  空文件夹清理完毕。")
		return PipelineSummary{}
	}

	p.log(fmt.Sprintf("  找到 %d 个视频文件", len(videos)))
	if len(subtitles) > 0 {
		p.log(fmt.Sprintf("  找到 %d 个字幕文件", len(subtitles)))
	}
	p.log("")

	results := make([]ProcessResult, 0, len(videos))
	for idx, video := range videos {
		results = append(results, p.processOne(video, idx+1, len(videos), subtitleIndex))
	}

	p.printSummary(results)
	p.sendNotifications()

	p.log("\n🧹 检查并清理空子文件夹...")
	p.cleanupEmptyFolders(scanFolder, "", true)
	p.log("  空文件夹清理完毕。")

	summary := PipelineSummary{}
	for _, r := range results {
		switch r.Status {
		case "ok":
			summary.OKCount++
		case "skipped":
			summary.SkippedCount++
		case "failed":
			summary.FailedCount++
		}
	}
	return summary
}

// ── 空文件夹清理 ────────────────────────────────────────────

func (p *Pipeline) cleanupEmptyFolders(folderID, folderName string, isRoot bool) bool {
	if p.dryRun {
		return false
	}
	// 第一阶段：纯遍历，在内存中收集可删目录（判定不做任何删除，不受网盘删除异步生效影响）
	var deletable []struct{ id, name string }
	var walk func(id, name string, isRoot bool) bool
	walk = func(id, name string, isRoot bool) bool {
		items, err := p.provider.ListFiles(id, 100)
		if err != nil {
			log.Printf("获取文件夹内容失败 [%s]: %v", name, err)
			return false
		}
		isEmpty := true
		for _, item := range items {
			if item.IsFolder() {
				if !walk(item.ID, item.Name, false) {
					isEmpty = false
				}
			} else {
				isEmpty = false
			}
		}
		if isEmpty && !isRoot {
			deletable = append(deletable, struct{ id, name string }{id, name})
			return true
		}
		return false
	}
	walk(folderID, folderName, isRoot)

	// 第二阶段：自底向上删除（收集顺序即子先父后）；网盘删除异步生效，
	// 父目录可能因子目录刚删除被短暂拒绝，带退避重试
	for _, f := range deletable {
		if err := p.trashFolderWithRetry(f.id); err != nil {
			log.Printf("清理空文件夹失败 [%s]: %v", f.name, err)
			continue
		}
		p.log(fmt.Sprintf("      🗑️  清理空文件夹：%s", f.name), "INFO")
	}
	return len(deletable) > 0
}

// trashFolderWithRetry 删除目录：失败时按 1s/2s 退避重试两次，避开网盘删除的异步生效窗口。
func (p *Pipeline) trashFolderWithRetry(id string) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<uint(attempt-1)) * time.Second)
		}
		if _, err = p.provider.TrashFile(id); err == nil {
			return nil
		}
	}
	return err
}

// ── 单文件处理 ──────────────────────────────────────────────

var unsafeFilenameRe = regexp.MustCompile(`[\\/:*?"<>|]`)

func safeFilename(name string) string {
	return strings.TrimSpace(unsafeFilenameRe.ReplaceAllString(name, ""))
}

func fileExt(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i:]
	}
	return ""
}

// buildCleanName 根据 TMDB 数据生成标准化文件名。
// 电影：Inception (2010).mkv；剧集：Breaking Bad - S01E03.mkv。
func buildCleanName(info map[string]any, isTV bool, seasonNum int, episodeNum *int, ext string) string {
	if isTV {
		title := firstNonEmpty(jsonx.Str(info, "name"), jsonx.Str(info, "title"), "Unknown")
		epStr := fmt.Sprintf("S%02d", seasonNum)
		if episodeNum != nil {
			epStr = fmt.Sprintf("S%02dE%02d", seasonNum, *episodeNum)
		}
		return fmt.Sprintf("%s - %s%s", safeFilename(title), epStr, ext)
	}
	title := firstNonEmpty(jsonx.Str(info, "title"), jsonx.Str(info, "name"), "Unknown")
	year := jsonx.Str(info, "release_date")
	if len(year) >= 4 {
		year = year[:4]
	}
	yearStr := ""
	if year != "" {
		yearStr = " (" + year + ")"
	}
	return fmt.Sprintf("%s%s%s", safeFilename(title), yearStr, ext)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// resolvePinnedTitle 查询标题钉扎；dry-run 只读不写（避免污染钉扎表）。
func (p *Pipeline) resolvePinnedTitle(isTV bool, tmdbInfo map[string]any, fallbackTitle, fallbackYear string) (string, string) {
	if p.pinStore == nil {
		return "", ""
	}
	mediaType := "movie"
	if isTV {
		mediaType = "tv"
	}
	id, err := strconv.ParseInt(tmdbIDOf(tmdbInfo), 10, 64)
	if err != nil || id <= 0 {
		return "", ""
	}
	if p.dryRun {
		if title, year, ok := p.pinStore.GetTitlePin(mediaType, id); ok {
			return title, year
		}
		return "", ""
	}
	title, year, err := p.pinStore.ResolveTitlePin(mediaType, id, fallbackTitle, fallbackYear)
	if err != nil || title == "" {
		return "", ""
	}
	return title, year
}

// buildCleanNameFromMeta 与 buildCleanName 等价，但标题/年份取自 meta（可能为钉扎名）。
func buildCleanNameFromMeta(meta *mediaparser.MetaBase, isTV bool, seasonNum int, episodeNum *int, ext string) string {
	title := meta.Name()
	if title == "" {
		return ""
	}
	if isTV {
		epStr := fmt.Sprintf("S%02d", seasonNum)
		if episodeNum != nil {
			epStr = fmt.Sprintf("S%02dE%02d", seasonNum, *episodeNum)
		}
		return fmt.Sprintf("%s - %s%s", safeFilename(title), epStr, ext)
	}
	yearStr := ""
	if meta.Year != "" {
		yearStr = " (" + meta.Year + ")"
	}
	return fmt.Sprintf("%s%s%s", safeFilename(title), yearStr, ext)
}

func (p *Pipeline) processOne(video cloudfile.CloudFile, idx, total int, subtitleIndex map[string][]cloudfile.CloudFile) ProcessResult {
	p.log(fmt.Sprintf("[%d/%d] 🎬  %s", idx, total, video.Name), "INFO")
	result := ProcessResult{File: video, Status: "ok"}

	// Step 2: 解析文件名
	meta := mediaparser.MetaInfo(video.Name, "", true, p.cfg.Parser.CustomWords, p.rgMatcher)
	if meta.Name() == "" {
		result.Status = "skipped"
		result.Reason = "解析失败（无法识别标题）"
		p.log("      ⚠️   "+result.Reason, "WARNING")
		return result
	}

	isTV := meta.Type == mediaparser.MediaTV
	mediaLabel := "Movie"
	if isTV {
		mediaLabel = "TV"
	}
	seasonNum := 1
	if s := meta.SeasonSeq(); s != "" {
		seasonNum, _ = strconv.Atoi(s)
	}
	var episodeNum *int
	if s := meta.EpisodeSeq(); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			episodeNum = &n
		}
	}
	seasonInfo := ""
	if isTV {
		ep := "?"
		if episodeNum != nil {
			ep = strconv.Itoa(*episodeNum)
		}
		seasonInfo = fmt.Sprintf("  S%02dE%s", seasonNum, ep)
	}
	p.log(fmt.Sprintf("      解析：%s (%s)  [%s]%s", meta.Name(), orDefault(meta.Year, "?"), mediaLabel, seasonInfo), "INFO")

	// Step 3: TMDB 整剧/整片元数据
	var tmdbInfo map[string]any
	if p.tmdb != nil {
		tmdbInfo = p.tmdb.Recognize(meta)
		if tmdbInfo != nil {
			tmdbTitle := firstNonEmpty(jsonx.Str(tmdbInfo, "title"), jsonx.Str(tmdbInfo, "name"))
			tmdbID := tmdbIDOf(tmdbInfo)
			tmdbYear := firstN4(firstNonEmpty(jsonx.Str(tmdbInfo, "release_date"), jsonx.Str(tmdbInfo, "first_air_date")))
			p.log(fmt.Sprintf("      TMDB：%s (%s)  tmdb_id=%s", tmdbTitle, tmdbYear, tmdbID), "SUCCESS")
			if isTV {
				tmdbInfo["_season"] = float64(seasonNum)
				if episodeNum != nil {
					tmdbInfo["_episode"] = float64(*episodeNum)
				}
			}
			cacheType := "movie"
			if isTV {
				cacheType = "tv"
			}
			p.writeCache(fmt.Sprintf("/%s/%s", cacheType, tmdbID), tmdbInfo)
		} else {
			p.log("      TMDB：未找到元数据", "WARNING")
			p.notifyItems = append(p.notifyItems, notifyItem{
				Title: meta.Name(), Year: meta.Year, IsTV: isTV,
				Season: seasonNum, Episode: episodeNum,
				PosterPath: "", TmdbID: "", NoTmdb: true, OriginalName: video.Name,
			})
			if !p.cfg.Pipeline.MoveOnTMDBMiss {
				result.Status = "skipped"
				result.Reason = "TMDB 未找到，moveOnTmdbMiss=false"
				return result
			}
		}
	}

	// Step 4: TMDB 单集详情（TV 专用）
	var episodeDetail map[string]any
	if isTV && tmdbInfo != nil && p.tmdb != nil && episodeNum != nil {
		if id, err := strconv.Atoi(tmdbIDOf(tmdbInfo)); err == nil {
			episodeDetail = p.tmdb.GetEpisodeDetail(id, seasonNum, *episodeNum)
		}
		if episodeDetail != nil {
			p.log(fmt.Sprintf("      单集：%s  (S%02dE%02d)", jsonx.Str(episodeDetail, "name"), seasonNum, *episodeNum), "INFO")
		}
	}

	// Step 5: 用 TMDB 数据更新 Meta，以便 Organizer 创建正确的中文目录
	if tmdbInfo != nil {
		tmdbTitle := firstNonEmpty(jsonx.Str(tmdbInfo, "name"), jsonx.Str(tmdbInfo, "title"), meta.Name())
		tmdbYear := firstN4(firstNonEmpty(jsonx.Str(tmdbInfo, "release_date"), jsonx.Str(tmdbInfo, "first_air_date")))
		// 标题钉扎：目录/文件名沿用首次整理名，TMDB 译名变更不再产生新目录
		if pinnedTitle, pinnedYear := p.resolvePinnedTitle(isTV, tmdbInfo, tmdbTitle, tmdbYear); pinnedTitle != "" {
			if pinnedTitle != tmdbTitle {
				p.log(fmt.Sprintf("      📌 TMDB 译名已变更（%s → %s），沿用首次整理目录名", tmdbTitle, pinnedTitle), "INFO")
			}
			tmdbTitle = pinnedTitle
			if pinnedYear != "" {
				tmdbYear = pinnedYear
			}
		}
		meta.SetName(safeFilename(tmdbTitle))
		if tmdbYear != "" {
			meta.Year = tmdbYear
		}
	}

	// Step 5.5: 生成 NFO 内容（NFO 正文用 TMDB 当前译名，文件名用 meta 钉扎名）
	var nfoContent, nfoName, cleanName string
	if tmdbInfo != nil {
		cleanName = buildCleanNameFromMeta(meta, isTV, seasonNum, episodeNum, fileExt(video.Name))
		nfoContent = p.nfoGen.Generate(tmdbInfo, meta.Type, episodeDetail)
		if cleanName != "" {
			nfoName = strings.TrimSuffix(cleanName, fileExt(cleanName)) + ".nfo"
		} else {
			nfoName = nfo.NfoNameFor(video.Name)
		}
	}

	// Step 6: 创建目标文件夹（分类启用时先解析分类目录链：媒体库根/一级/二级）
	var targetFolder *cloudfile.CloudFile
	var targetChain []cloudfile.CloudFile
	var topFolderID string
	baseID := ""
	if p.categories != nil {
		level1, level2 := "", ""
		if tmdbInfo != nil {
			level1, level2 = p.categories.Resolve(isTV, tmdbGenreIDs(tmdbInfo), jsonx.Str(tmdbInfo, "original_language"))
		} else {
			// 无 TMDB 元数据：落"其他"一级目录（无法判定二级）
			level1 = p.categories.Dirs.Other
		}
		if !p.dryRun {
			if nodes, bid, err := p.ensureCategoryChain(level1, level2); err == nil {
				baseID = bid
				targetChain = nodes
			} else {
				log.Printf("分类目录创建失败，回退默认根目录：%v", err)
			}
		}
		if level2 != "" {
			p.log(fmt.Sprintf("      分类：%s/%s", level1, level2), "INFO")
		} else {
			p.log(fmt.Sprintf("      分类：%s", level1), "INFO")
		}
	}
	if !p.dryRun {
		var orgChain []cloudfile.CloudFile
		if baseID != "" {
			orgChain, targetFolder = p.organizer.EnsureFolderWithChainUnder(baseID, meta, video.Name)
		} else {
			orgChain, targetFolder = p.organizer.EnsureFolderWithChainForMeta(meta, video.Name)
		}
		if targetFolder == nil {
			result.Status = "failed"
			result.Reason = "文件夹创建失败"
			p.log("      ❌  "+result.Reason, "ERROR")
			return result
		}
		targetChain = append(targetChain, orgChain...)
		result.TargetFolder = targetFolder
		result.TargetChain = targetChain
		if isTV && tmdbInfo != nil {
			tmdbIDKey := tmdbIDOf(tmdbInfo)
			if cached, ok := p.showFolderCache[tmdbIDKey]; ok && tmdbIDKey != "" {
				topFolderID = cached
			} else if len(targetFolder.Parents) > 0 {
				topFolderID = targetFolder.Parents[0]
				if tmdbIDKey != "" {
					p.showFolderCache[tmdbIDKey] = topFolderID
				}
			}
		} else {
			topFolderID = targetFolder.ID
		}
		folderPath := p.organizer.FolderPathForMeta(meta)
		p.log(fmt.Sprintf("      文件夹：%s  [%s...]", folderPath, firstN12(targetFolder.ID)), "SUCCESS")
	} else {
		folderPath := p.organizer.FolderPathForMeta(meta)
		p.log(fmt.Sprintf("      文件夹：%s  [dry-run，不创建]", folderPath), "INFO")
	}

	// Step 7.5: 同名文件预检
	pendingReplaceID := ""
	if targetFolder != nil && !p.dryRun {
		targetName := firstNonEmpty(cleanName, video.Name)
		existing, err := p.provider.FindFile(targetName, targetFolder.ID)
		if err == nil && existing != nil {
			if existing.ID == video.ID {
				result.Status = "skipped"
				result.Reason = "文件已在目标位置"
				p.log("      同名预检：文件已在目标位置，跳过", "INFO")
			} else if p.replaceExistingVideo {
				pendingReplaceID = existing.ID
				p.log("      同名预检：将替换已有文件", "INFO")
			} else {
				result.Status = "skipped"
				result.Reason = "目标位置已存在同名文件"
				p.log("      同名预检：跳过（目标位置已存在同名文件）", "WARNING")
				return result
			}
		}
	}

	// Step 8-11: 元数据上传
	uploadMetadata := func() {
		if p.skipMetadataUpload {
			p.log("      元数据：跳过（skipMetadataUpload 已开启）", "INFO")
			return
		}
		// Step 8: 单集/电影 NFO
		if nfoContent != "" && nfoName != "" {
			if !p.dryRun && targetFolder != nil {
				if _, err := p.provider.UploadText(nfoContent, nfoName, targetFolder.ID, "text/xml", true); err != nil {
					log.Printf("上传单集 NFO 失败：%v", err)
					p.log(fmt.Sprintf("      NFO：上传失败 — %v", err), "ERROR")
				} else {
					result.NfoUploaded = true
					p.log(fmt.Sprintf("      NFO：%s  ✓", nfoName), "SUCCESS")
				}
			} else {
				p.log(fmt.Sprintf("      NFO：%s  [dry-run，不上传]", nfoName), "INFO")
			}
		} else if !p.skipTmdb && tmdbInfo == nil {
			p.log("      NFO：跳过（TMDB 无数据）", "INFO")
		}

		// Step 9: tvshow.nfo → 剧名文件夹
		if isTV && tmdbInfo != nil {
			if !p.dryRun && topFolderID != "" && !p.tvshowNfoDone[topFolderID] {
				xmlStr := p.nfoGen.GenerateTVShow(tmdbInfo)
				if _, err := p.provider.UploadText(xmlStr, "tvshow.nfo", topFolderID, "text/xml", true); err != nil {
					log.Printf("上传 tvshow.nfo 失败：%v", err)
					p.log(fmt.Sprintf("      tvshow.nfo：失败 — %v", err), "ERROR")
				} else {
					p.tvshowNfoDone[topFolderID] = true
					p.log("      tvshow.nfo：→ 剧名文件夹  ✓", "SUCCESS")
				}
			} else if p.dryRun && topFolderID == "" {
				p.log("      tvshow.nfo：→ 剧名文件夹  [dry-run]", "INFO")
			}
		}

		// Step 10: season.nfo → Season 文件夹
		if isTV && tmdbInfo != nil && targetFolder != nil {
			seasonFolderID := targetFolder.ID
			if !p.dryRun && !p.seasonNfoDone[seasonFolderID] {
				seasonDetail := p.getSeasonDetailCached(tmdbIDOf(tmdbInfo), seasonNum)
				xmlStr := p.nfoGen.GenerateSeason(seasonDetail, seasonNum)
				if _, err := p.provider.UploadText(xmlStr, "season.nfo", seasonFolderID, "text/xml", true); err != nil {
					log.Printf("上传 season.nfo 失败：%v", err)
					p.log(fmt.Sprintf("      season.nfo：失败 — %v", err), "ERROR")
				} else {
					p.seasonNfoDone[seasonFolderID] = true
					p.log(fmt.Sprintf("      season.nfo：→ Season %d 文件夹  ✓", seasonNum), "SUCCESS")
				}
			} else if p.dryRun && !p.seasonNfoDone[seasonFolderID] {
				p.log(fmt.Sprintf("      season.nfo：→ Season %d 文件夹  [dry-run]", seasonNum), "INFO")
			}
		}

		// Step 11: poster.jpg / fanart.jpg
		if tmdbInfo != nil && p.imgUploader != nil && targetFolder != nil {
			imgTopID := firstNonEmpty(topFolderID, targetFolder.ID)
			if !p.posterDone[imgTopID] {
				uploadedAny := false
				if poster := jsonx.Str(tmdbInfo, "poster_path"); poster != "" {
					if f := p.imgUploader.UploadPoster(poster, imgTopID); f != nil {
						p.log("      poster.jpg：✓", "SUCCESS")
						uploadedAny = true
					}
				}
				if fanart := jsonx.Str(tmdbInfo, "backdrop_path"); fanart != "" {
					if f := p.imgUploader.UploadFanart(fanart, imgTopID); f != nil {
						p.log("      fanart.jpg：✓", "SUCCESS")
						uploadedAny = true
					}
				}
				if uploadedAny {
					p.posterDone[imgTopID] = true
				}
			}
		} else if tmdbInfo != nil && p.dryRun {
			p.log("      poster.jpg / fanart.jpg：[dry-run，不下载]", "INFO")
		}

		// Step 11.5: season poster（季封面）
		if isTV && tmdbInfo != nil && p.imgUploader != nil && targetFolder != nil && topFolderID != "" {
			seasonPosterKey := fmt.Sprintf("%s:s%d", topFolderID, seasonNum)
			if !p.seasonPosterDone[seasonPosterKey] {
				seasonDetail := p.getSeasonDetailCached(tmdbIDOf(tmdbInfo), seasonNum)
				if sp := jsonx.Str(seasonDetail, "poster_path"); sp != "" {
					if f := p.imgUploader.UploadSeasonPoster(sp, seasonNum, topFolderID); f != nil {
						p.log(fmt.Sprintf("      season%02d-poster.jpg：✓", seasonNum), "SUCCESS")
					}
				}
				p.seasonPosterDone[seasonPosterKey] = true
			}
		}
	}

	if pendingReplaceID == "" && result.Status != "failed" {
		uploadMetadata()
	}

	// Step 12: 移动视频文件（同时改名为标准格式）
	if !p.dryRun && targetFolder != nil && result.Status == "ok" {
		if pendingReplaceID != "" {
			if _, err := p.provider.TrashFile(pendingReplaceID); err != nil {
				result.Status = "failed"
				result.Reason = fmt.Sprintf("移除同名文件失败：%v", err)
				p.log("      移动：失败 — 无法移除同名文件", "ERROR")
				return result
			}
			p.log("      移动：已移除同名文件", "INFO")
		}
		if _, err := p.provider.MoveFile(video.ID, targetFolder.ID, cleanName); err != nil {
			result.Status = "failed"
			result.Reason = fmt.Sprintf("移动失败：%v", err)
			if pendingReplaceID != "" {
				result.Reason += fmt.Sprintf("（已移除的旧文件 ID: %s）", pendingReplaceID)
			}
			p.log(fmt.Sprintf("      移动：失败 — %v", err), "ERROR")
			return result
		}
		result.Moved = true
		result.Status = "ok"
		// 本地更新 result.File（新名字 + 新父目录）供 STRM 钩子使用；
		// video 保持原始元数据——后续字幕匹配依赖原始发布名；pick_code 移动/改名不变
		if renamed := cleanName != "" && cleanName != video.Name; renamed {
			updated := video
			updated.Name = cleanName
			updated.ParentID = targetFolder.ID
			updated.Parents = []string{targetFolder.ID}
			result.File = updated
			p.log(fmt.Sprintf("      移动+改名：%s  ✓", cleanName), "SUCCESS")
		} else {
			updated := video
			updated.ParentID = targetFolder.ID
			updated.Parents = []string{targetFolder.ID}
			result.File = updated
			p.log("      移动：✓", "SUCCESS")
		}
	} else if result.Status == "ok" {
		targetName := firstNonEmpty(cleanName, video.Name)
		p.log(fmt.Sprintf("      移动：%s  [dry-run，不移动]", targetName), "INFO")
	}

	// Step 12.5: 替换模式下移动成功后上传元数据
	if pendingReplaceID != "" && result.Moved {
		uploadMetadata()
	}

	// Step 13: 移动关联的字幕文件（dry-run 下也预览；视频被跳过也处理）
	if len(subtitleIndex) > 0 && result.Status != "failed" {
		p.moveSubtitlesForVideo(video, firstNonEmpty(cleanName, video.Name), targetFolder, subtitleIndex)
		p.log("", "INFO")
	}

	p.recordIngest(&result, meta, tmdbInfo, seasonNum, episodeNum, episodeDetail)

	// 入库成功回调（STRM 生成等后处理；dry-run 与失败/跳过不触发）
	strmOK := false
	if p.onVideoIngested != nil && result.Status == "ok" && !p.dryRun && result.TargetFolder != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[pipeline] 入库后处理回调异常：%v", r)
				}
			}()
			strmOK = p.onVideoIngested(result) == nil
		}()
	}

	// 收集入库通知数据
	if result.Status == "ok" && tmdbInfo != nil {
		tmdbIDForSP := tmdbIDOf(tmdbInfo)
		genres, animation := tmdbGenres(tmdbInfo)
		p.notifyItems = append(p.notifyItems, notifyItem{
			Title: meta.Name(), Year: meta.Year, IsTV: isTV,
			Season: seasonNum, Episode: episodeNum,
			EpTitle:      jsonx.Str(episodeDetail, "name"),
			PosterPath:   jsonx.Str(tmdbInfo, "poster_path"),
			BackdropPath: jsonx.Str(tmdbInfo, "backdrop_path"),
			TmdbID:       tmdbIDForSP,
			NoTmdb:       false,
			Rating:       jsonx.Float(tmdbInfo, "vote_average", 0),
			Genres:       genres,
			Country:      mediaCountryLabel(jsonx.Str(tmdbInfo, "original_language"), isTV, animation),
			SeasonTotal:  p.seasonEpisodeCount(tmdbIDForSP, seasonNum),
			Overview:     jsonx.Str(tmdbInfo, "overview"),
			StrmOK:       strmOK,
		})
	}

	return result
}

func (p *Pipeline) recordIngest(result *ProcessResult, meta *mediaparser.MetaBase, tmdbInfo map[string]any, seasonNum int, episodeNum *int, episodeDetail map[string]any) {
	// dry-run 未实际入库，不写历史
	if p.ingestStore == nil || p.dryRun {
		return
	}
	isTV := meta.Type == mediaparser.MediaTV
	record := IngestRecord{
		MediaType:    "movie",
		Title:        meta.Name(),
		Year:         meta.Year,
		Season:       nil,
		Episode:      nil,
		OriginalName: result.File.Name,
	}
	if isTV {
		record.MediaType = "tv"
		record.Season = &seasonNum
		record.Episode = episodeNum
	}
	switch {
	case result.Status == "ok" && tmdbInfo != nil:
		id, _ := strconv.Atoi(tmdbIDOf(tmdbInfo))
		record.TmdbID = id
		record.Title = firstNonEmpty(jsonx.Str(tmdbInfo, "name"), jsonx.Str(tmdbInfo, "title"), meta.Name())
		record.OriginalTitle = firstNonEmpty(jsonx.Str(tmdbInfo, "original_name"), jsonx.Str(tmdbInfo, "original_title"))
		record.Year = firstN4(firstNonEmpty(jsonx.Str(tmdbInfo, "first_air_date"), jsonx.Str(tmdbInfo, "release_date")))
		record.EpisodeTitle = jsonx.Str(episodeDetail, "name")
		record.PosterPath = jsonx.Str(tmdbInfo, "poster_path")
		if result.TargetFolder != nil {
			record.DriveFolderID = result.TargetFolder.ID
		}
		record.Status = "success"
	case result.Status == "ok":
		record.Status = "no_tmdb"
	case result.Status == "failed":
		if tmdbInfo != nil {
			record.TmdbID, _ = strconv.Atoi(tmdbIDOf(tmdbInfo))
		}
		record.Status = "failed"
		record.ErrorMessage = result.Reason
	default:
		return // skipped 不记录
	}
	if err := p.ingestStore.RecordIngest(record); err != nil {
		log.Printf("记录入库历史失败：%v", err)
	}
}

func (p *Pipeline) printSummary(results []ProcessResult) {
	ok, skipped, failed := 0, 0, 0
	for _, r := range results {
		switch r.Status {
		case "ok":
			ok++
		case "skipped":
			skipped++
		case "failed":
			failed++
		}
	}
	p.log(strings.Repeat("=", 68), "INFO")
	p.log("  完成", "INFO")
	level := "SUCCESS"
	if failed > 0 {
		level = "ERROR"
	} else if skipped > 0 {
		level = "WARNING"
	}
	p.log(fmt.Sprintf("  ✓  成功：%d    ⚠  跳过：%d    ✗  失败：%d", ok, skipped, failed), level)
	if skipped > 0 {
		p.log("\n  跳过的文件：", "WARNING")
		for _, r := range results {
			if r.Status == "skipped" {
				p.log(fmt.Sprintf("    - %s  （%s）", r.File.Name, r.Reason), "WARNING")
			}
		}
	}
	if failed > 0 {
		p.log("\n  失败的文件：", "ERROR")
		for _, r := range results {
			if r.Status == "failed" {
				p.log(fmt.Sprintf("    - %s  （%s）", r.File.Name, r.Reason), "ERROR")
			}
		}
	}
	p.log(strings.Repeat("=", 68), "INFO")
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func firstN4(s string) string {
	if len(s) >= 4 {
		return s[:4]
	}
	return s
}

func firstN12(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
