// Package pipeline Meta2Cloud 完整流水线（对应 Python 版 core/pipeline.py）。
//
// 流程：扫描 → 解析 → TMDB（整剧+单集）→ NFO → 文件夹 → 图片 → 移动。
package pipeline

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
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
	NfoUploaded  bool
	Moved        bool
}

// PipelineSummary 汇总。
type PipelineSummary struct {
	OKCount      int
	SkippedCount int
	FailedCount  int
}

// notifyItem Telegram 入库通知条目。
type notifyItem struct {
	Title        string
	Year         string
	IsTV         bool
	Season       int
	Episode      *int
	EpTitle      string
	PosterPath   string // 竖版海报（无横版图时兜底）
	BackdropPath string // 横版背景图（通知图片首选）
	TmdbID       string
	NoTmdb       bool
	OriginalName string
	// 富通知字段（来自 tmdbInfo 与 STRM 钩子）
	Rating      float64
	Genres      []string
	Country     string
	SeasonTotal int
	Overview    string
	StrmOK      bool
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

// DuplicateFolderSource 重复目录分组来源接口（webui 层实现，可为 nil）。
type DuplicateFolderSource interface {
	DuplicateFolderGroups() []DuplicateFolderGroup
}

// DuplicateFolderInfo 重复组内的单个网盘目录。
type DuplicateFolderInfo struct {
	ID           string
	ModifiedTime int64
}

// DuplicateFolderGroup 同一媒体对应的多个网盘目录分组。
// Folders 按目录修改时间升序排列（最旧的在最前）。
type DuplicateFolderGroup struct {
	MediaType string
	TmdbID    int64
	Folders   []DuplicateFolderInfo
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
			cfg.ActiveMovieRootID(),
			cfg.ActiveTvRootID(),
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
		notifyItems:       []notifyItem{},
		ingestStore:       opts.IngestStore,
		pinStore:          opts.TitlePinStore,
		dupSource:         opts.DuplicateFolders,
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
	scanFolder := p.cfg.ActiveScanFolderID()
	if scanFolder == "" {
		p.log("❌  配置错误：当前存储后端的扫描目录未设置", "ERROR")
		return PipelineSummary{}
	}
	if p.cfg.ActiveRootFolderID() == "" {
		p.log("❌  配置错误：当前存储后端的媒体库根目录未设置", "ERROR")
		return PipelineSummary{}
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

// ── 字幕移动 ────────────────────────────────────────────────

func (p *Pipeline) moveSubtitlesForVideo(video cloudfile.CloudFile, cleanVideoName string, targetFolder *cloudfile.CloudFile, subtitleIndex map[string][]cloudfile.CloudFile) {
	videoParent := video.ParentID
	if videoParent == "" && len(video.Parents) > 0 {
		videoParent = video.Parents[0]
	}
	candidates := subtitleIndex[videoParent]
	if len(candidates) == 0 {
		return
	}

	matcher := &SubtitleMatcher{}
	matches := matcher.FindSubtitlesForVideo(video, candidates)
	if len(matches) == 0 {
		return
	}

	processedIDs := map[string]bool{}
	for i := range matches {
		match := &matches[i]
		targetSubtitleName := matcher.BuildTargetName(cleanVideoName, match.SubtitleFile.Extension(), match.LanguageTags)

		if p.dryRun || targetFolder == nil {
			p.log(fmt.Sprintf("      字幕：%s  [dry-run，不移动]", targetSubtitleName), "INFO")
			continue
		}

		// 字幕是否已在目标位置
		if match.SubtitleFile.ParentID == targetFolder.ID && match.SubtitleFile.Name == targetSubtitleName {
			processedIDs[match.SubtitleFile.ID] = true
			langStr := strings.Join(match.LanguageTags, ".")
			if langStr == "" {
				langStr = "default"
			}
			p.log(fmt.Sprintf("      字幕：%s (%s) [已就位]", targetSubtitleName, langStr), "INFO")
			continue
		}

		// 目标位置已存在同名字幕
		existingSub, err := p.provider.FindFile(targetSubtitleName, targetFolder.ID)
		if err == nil && existingSub != nil {
			if existingSub.ID == match.SubtitleFile.ID {
				continue
			}
			if p.replaceExistingVideo {
				if _, err := p.provider.TrashFile(existingSub.ID); err != nil {
					log.Printf("移除已存在字幕失败 [%s]: %v", targetSubtitleName, err)
					continue
				}
				p.log(fmt.Sprintf("      字幕：移除已存在的 %s", targetSubtitleName), "INFO")
			} else {
				log.Printf("目标位置已存在字幕 %s，跳过", targetSubtitleName)
				continue
			}
		}

		if _, err := p.provider.MoveFile(match.SubtitleFile.ID, targetFolder.ID, targetSubtitleName); err != nil {
			log.Printf("移动字幕失败 [%s]: %v", match.SubtitleFile.Name, err)
			continue
		}
		processedIDs[match.SubtitleFile.ID] = true

		langStr := strings.Join(match.LanguageTags, ".")
		if langStr == "" {
			langStr = "default"
		}
		p.log(fmt.Sprintf("      字幕：%s (%s) ✓", targetSubtitleName, langStr), "SUCCESS")
	}

	// 从索引中移除已处理的字幕，避免重复匹配
	if len(processedIDs) > 0 {
		if list, ok := subtitleIndex[videoParent]; ok {
			var remaining []cloudfile.CloudFile
			for _, sub := range list {
				if !processedIDs[sub.ID] {
					remaining = append(remaining, sub)
				}
			}
			subtitleIndex[videoParent] = remaining
		}
	}
}

// ── 空文件夹清理 ────────────────────────────────────────────

func (p *Pipeline) cleanupEmptyFolders(folderID, folderName string, isRoot bool) bool {
	if p.dryRun {
		return false
	}
	items, err := p.provider.ListFiles(folderID, 100)
	if err != nil {
		log.Printf("获取文件夹内容失败 [%s]: %v", folderID, err)
		return false
	}
	isEmpty := true
	for _, item := range items {
		if item.IsFolder() {
			if !p.cleanupEmptyFolders(item.ID, item.Name, false) {
				isEmpty = false
			}
		} else {
			isEmpty = false
		}
	}
	if isEmpty && !isRoot {
		if _, err := p.provider.TrashFile(folderID); err != nil {
			log.Printf("清理空文件夹失败 [%s]: %v", folderName, err)
			return false
		}
		p.log(fmt.Sprintf("      🗑️  清理空文件夹：%s", folderName), "INFO")
	}
	return isEmpty
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

// ── 重复目录合并 ────────────────────────────────────────────

// mergeDuplicateFoldersPass 把同一媒体因译名差异产生的多个目录合并进钉扎目录。
// 每轮每个媒体只处理一次；dry-run 只记录日志。
func (p *Pipeline) mergeDuplicateFoldersPass() {
	if p.dupSource == nil || p.pinStore == nil {
		return
	}
	if !p.cfg.Pipeline.MergeDuplicatesEnabled() {
		return
	}
	groups := p.dupSource.DuplicateFolderGroups()
	if len(groups) == 0 {
		return
	}
	p.log("\n🔀 检查重复目录...")
	merged := 0
	for _, g := range groups {
		if p.mergeDuplicateGroup(g) {
			merged++
		}
	}
	if merged > 0 {
		p.log(fmt.Sprintf("  共合并 %d 个重复目录", merged), "SUCCESS")
	}
}

// mergeDuplicateGroup 合并一个媒体分组的重复目录，返回是否有目录被回收。
func (p *Pipeline) mergeDuplicateGroup(g DuplicateFolderGroup) bool {
	var pinTitle, pinYear string
	if p.dryRun {
		// dry-run 只读，不落库
		title, year, ok := p.pinStore.GetTitlePin(g.MediaType, g.TmdbID)
		if ok {
			pinTitle, pinYear = title, year
		}
	} else {
		title, year, err := p.pinStore.ResolveTitlePin(g.MediaType, g.TmdbID, "", "")
		if err == nil {
			pinTitle, pinYear = title, year
		}
	}

	rootID := p.cfg.ActiveMovieRootID()
	if g.MediaType == "tv" {
		rootID = p.cfg.ActiveTvRootID()
	}
	if rootID == "" {
		rootID = p.cfg.ActiveRootFolderID()
	}
	if rootID == "" {
		return false
	}
	children, err := p.provider.ListFiles(rootID, 500)
	if err != nil {
		log.Printf("合并重复目录：列举根目录失败 [%s]: %v", rootID, err)
		return false
	}
	byID := map[string]cloudfile.CloudFile{}
	for _, c := range children {
		byID[c.ID] = c
	}

	// 钉扎名仍为空（无钉扎记录、无入库历史）：取组内最旧目录的实际名称作为初始名落库
	if pinTitle == "" {
		for _, f := range g.Folders { // 已按修改时间升序，最旧在前
			c, ok := byID[f.ID]
			if !ok || c.Trashed {
				continue
			}
			parsedTitle, parsedYear := splitFolderName(c.Name)
			if parsedTitle == "" {
				continue
			}
			if p.dryRun {
				pinTitle, pinYear = parsedTitle, parsedYear
			} else if t, y, err := p.pinStore.ResolveTitlePin(g.MediaType, g.TmdbID, parsedTitle, parsedYear); err == nil {
				pinTitle, pinYear = t, y
			}
			if pinTitle != "" {
				p.log(fmt.Sprintf("      📌 以最旧目录命名钉扎：%s", pinnedFolderName(pinTitle, pinYear)), "INFO")
				break
			}
		}
	}
	if pinTitle == "" {
		p.log(fmt.Sprintf("  ⚠ 跳过重复组 tmdb_id=%d：无法确定钉扎名（钉扎表、入库历史与组内目录均为空）", g.TmdbID), "WARNING")
		return false
	}

	targetName := pinnedFolderName(pinTitle, pinYear)
	var target *cloudfile.CloudFile
	for _, c := range children {
		if c.Name == targetName {
			cp := c
			target = &cp
			break
		}
	}
	if target == nil {
		// 钉扎目录不存在（可能已被手动改名），跳过等待下次整理建立
		p.log(fmt.Sprintf("  ⚠ 跳过重复组 tmdb_id=%d：钉扎目录 %q 不在根目录下（可能已被改名）", g.TmdbID, targetName), "WARNING")
		return false
	}
	recycled := false
	for _, f := range g.Folders {
		if f.ID == target.ID {
			continue
		}
		candidate, exists := byID[f.ID]
		if !exists || candidate.Trashed {
			continue
		}
		if p.mergeFolderInto(candidate, *target) {
			recycled = true
		}
	}
	return recycled
}

// mergeFolderInto 把 candidate 目录内容搬入 target（镜像子目录结构，同名文件跳过）。
// 腾空（或仅剩元数据）后整个目录进回收站；返回是否完成回收。
func (p *Pipeline) mergeFolderInto(candidate, target cloudfile.CloudFile) bool {
	p.log(fmt.Sprintf("  🔀 合并重复目录：%s → %s", candidate.Name, target.Name))
	moved, skipped := p.moveFolderContents(candidate.ID, target.ID)
	if moved == 0 && skipped == 0 {
		return false
	}
	if p.dryRun {
		p.log(fmt.Sprintf("      [dry-run] 将移动 %d 个文件，跳过 %d 个同名文件", moved, skipped), "INFO")
		return false
	}
	p.log(fmt.Sprintf("      移动 %d 个文件，跳过 %d 个同名文件", moved, skipped), "INFO")
	if p.folderHasContent(candidate.ID) {
		p.log(fmt.Sprintf("      ⚠ 目录内仍有视频/字幕残留，保留目录：%s", candidate.Name), "WARNING")
		return false
	}
	if _, err := p.provider.TrashFile(candidate.ID); err != nil {
		log.Printf("回收重复目录失败 [%s]: %v", candidate.Name, err)
		p.log(fmt.Sprintf("      ❌ 回收目录失败：%v", err), "ERROR")
		return false
	}
	p.log(fmt.Sprintf("      🗑️ 已回收重复目录：%s（可在网盘回收站恢复）", candidate.Name), "SUCCESS")
	// 清理搬迁过程中在 target 下可能产生的空目录
	p.cleanupEmptyFolders(target.ID, target.Name, false)
	return true
}

// moveFolderContents 递归把 src 目录内容搬入 dst（镜像子目录，同名文件跳过）。
func (p *Pipeline) moveFolderContents(srcID, dstID string) (moved, skipped int) {
	items, err := p.provider.ListFiles(srcID, 200)
	if err != nil {
		log.Printf("合并重复目录：列举目录失败 [%s]: %v", srcID, err)
		return 0, 0
	}
	for _, item := range items {
		if item.IsFolder() {
			subDst := dstID
			if !p.dryRun {
				sub, err := cloudfile.GetOrCreateFolder(p.provider, item.Name, dstID)
				if err != nil {
					log.Printf("合并重复目录：创建子目录失败 %q: %v", item.Name, err)
					skipped++
					continue
				}
				subDst = sub.ID
			}
			m, s := p.moveFolderContents(item.ID, subDst)
			moved += m
			skipped += s
			continue
		}
		if p.dryRun {
			moved++
			continue
		}
		if existing, err := p.provider.FindFile(item.Name, dstID); err == nil && existing != nil {
			p.log(fmt.Sprintf("      跳过同名文件：%s", item.Name), "INFO")
			skipped++
			continue
		}
		if _, err := p.provider.MoveFile(item.ID, dstID, item.Name); err != nil {
			log.Printf("合并重复目录：移动文件失败 [%s]: %v", item.Name, err)
			skipped++
			continue
		}
		moved++
	}
	return moved, skipped
}

// folderHasContent 目录（含子目录）内是否还有视频或字幕文件。
func (p *Pipeline) folderHasContent(folderID string) bool {
	items, err := p.provider.ListFiles(folderID, 200)
	if err != nil {
		return true // 出错时保守处理，不回收
	}
	for _, item := range items {
		if item.IsFolder() {
			if p.folderHasContent(item.ID) {
				return true
			}
			continue
		}
		if item.IsVideo() || item.IsSubtitle() {
			return true
		}
	}
	return false
}

// pinnedFolderName 钉扎目录名：标题 (年份)。
func pinnedFolderName(title, year string) string {
	if year != "" {
		return fmt.Sprintf("%s (%s)", title, year)
	}
	return title
}

// tmdbGenres 提取 TMDB 流派名列表，并判断是否为动画类型。
func tmdbGenres(info map[string]any) (names []string, animation bool) {
	genres, _ := info["genres"].([]any)
	for _, g := range genres {
		gm, ok := g.(map[string]any)
		if !ok {
			continue
		}
		name := jsonx.Str(gm, "name")
		if name == "" {
			continue
		}
		names = append(names, name)
		if name == "动画" || name == "Animation" {
			animation = true
		}
	}
	return names, animation
}

// mediaCountryMap 原始语言 → 国家/地区展示名。
var mediaCountryMap = map[string]string{
	"ja": "日本", "en": "美国", "ko": "韩国", "cn": "中国", "tw": "台湾",
	"hk": "香港", "fr": "法国", "gb": "英国", "th": "泰国", "es": "西班牙",
}

// mediaCountryLabel 语言与类型 → 展示标签（如「日番 · 日本」「电影 · 美国」）。
func mediaCountryLabel(lang string, isTV, animation bool) string {
	country := mediaCountryMap[lang]
	if country == "" {
		return ""
	}
	kind := "电影"
	if isTV {
		switch {
		case animation && lang == "ja":
			kind = "日番"
		case animation:
			kind = "动画剧集"
		default:
			kind = "剧集"
		}
	}
	return kind + " · " + country
}

// seasonEpisodeCount 某季的集数（来自季详情缓存）。
func (p *Pipeline) seasonEpisodeCount(tmdbIDStr string, season int) int {
	if tmdbIDStr == "" {
		return 0
	}
	sd := p.getSeasonDetailCached(tmdbIDStr, season)
	eps, _ := sd["episodes"].([]any)
	return len(eps)
}

var folderYearRe = regexp.MustCompile(`^(.*?)\s*\((\d{4})\)$`)

// splitFolderName 把目录名拆为标题与年份（"标题 (2026)" → "标题", "2026"）。
func splitFolderName(name string) (title, year string) {
	name = strings.TrimSpace(name)
	if m := folderYearRe.FindStringSubmatch(name); m != nil {
		return strings.TrimSpace(m[1]), m[2]
	}
	return name, ""
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

	// Step 6: 创建目标文件夹
	var targetFolder *cloudfile.CloudFile
	topFolderID := ""
	if !p.dryRun {
		targetFolder = p.organizer.EnsureFolderForMeta(meta, video.Name)
		if targetFolder == nil {
			result.Status = "failed"
			result.Reason = "文件夹创建失败"
			p.log("      ❌  "+result.Reason, "ERROR")
			return result
		}
		result.TargetFolder = targetFolder
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
		if cleanName != "" && cleanName != video.Name {
			p.log(fmt.Sprintf("      移动+改名：%s  ✓", cleanName), "SUCCESS")
		} else {
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

// ── Telegram 通知 ───────────────────────────────────────────

func (p *Pipeline) sendNotifications() {
	tg := p.cfg.Telegram
	if tg.BotToken == "" || tg.ChatID == "" || len(p.notifyItems) == 0 {
		return
	}
	log.Printf("准备发送 TG 入库通知，共 %d 个条目", len(p.notifyItems))

	// 按 tmdb_id 分组（同一剧/电影合并为一条消息）
	type itemGroup struct {
		items []notifyItem
	}
	groups := map[string]*itemGroup{}
	var order []string
	posterMap := map[string]string{}
	for _, item := range p.notifyItems {
		key := item.TmdbID
		if key == "" {
			key = item.Title + "_" + item.Year
		}
		if _, ok := groups[key]; !ok {
			groups[key] = &itemGroup{}
			order = append(order, key)
		}
		groups[key].items = append(groups[key].items, item)
		if _, ok := posterMap[key]; !ok && item.PosterPath != "" {
			posterMap[key] = item.PosterPath
		}
	}

	for _, key := range order {
		items := groups[key].items
		first := items[0]
		// 通知图片优先横版背景图，竖版海报兜底
		poster := first.BackdropPath
		if poster == "" {
			poster = posterMap[key]
		}

		if first.NoTmdb {
			icon := "🎬"
			if first.IsTV {
				icon = "📺"
			}
			lines := []string{fmt.Sprintf("%s <b>TMDB 未找到元数据，文件未整理</b>", icon)}
			for _, it := range items {
				name := firstNonEmpty(it.OriginalName, it.Title)
				lines = append(lines, fmt.Sprintf("• <code>%s</code>", name))
			}
			lines = append(lines, "\n请检查文件名后手动触发重新整理")
			p.sendTgPhoto(tg.BotToken, tg.ChatID, "", strings.Join(lines, "\n"))
			continue
		}

		p.sendTgPhoto(tg.BotToken, tg.ChatID, poster, p.buildNotifyCaption(first, items))
	}
}

// ── 通知文案 ────────────────────────────────────────────────

type notifySeasonSeg struct {
	Season int
	Range  string // "S01E01-E03" / "S01E01、E05" / "S01"
	Total  int    // 本季总集数
}

// seasonSegments 按季聚合集号并生成区间文案（季号升序，连续集号压缩为区间）。
func seasonSegments(items []notifyItem) []notifySeasonSeg {
	bySeason := map[int][]*notifyItem{}
	seasonTotal := map[int]int{}
	var seasons []int
	seen := map[int]bool{}
	for i := range items {
		it := &items[i]
		if !seen[it.Season] {
			seen[it.Season] = true
			seasons = append(seasons, it.Season)
		}
		bySeason[it.Season] = append(bySeason[it.Season], it)
		if it.SeasonTotal > seasonTotal[it.Season] {
			seasonTotal[it.Season] = it.SeasonTotal
		}
	}
	sort.Ints(seasons)

	segs := make([]notifySeasonSeg, 0, len(seasons))
	for _, s := range seasons {
		nums := make([]int, 0, len(bySeason[s]))
		hasEpisode := false
		for _, it := range bySeason[s] {
			if it.Episode != nil {
				nums = append(nums, *it.Episode)
				hasEpisode = true
			}
		}
		sort.Ints(nums)
		prefix := fmt.Sprintf("S%02d", s)
		rng := prefix
		if hasEpisode {
			var parts []string
			start, prev := nums[0], nums[0]
			appendPart := func(end int) {
				if start == end {
					parts = append(parts, fmt.Sprintf("E%02d", start))
				} else {
					parts = append(parts, fmt.Sprintf("E%02d-E%02d", start, end))
				}
			}
			for _, n := range nums[1:] {
				if n == prev+1 {
					prev = n
					continue
				}
				appendPart(prev)
				start, prev = n, n
			}
			appendPart(prev)
			rng = prefix + strings.Join(parts, "、")
		}
		segs = append(segs, notifySeasonSeg{Season: s, Range: rng, Total: seasonTotal[s]})
	}
	return segs
}

// hashtagOf 去掉空白与 HTML 特殊字符，生成 Telegram 话题标签。
func hashtagOf(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '　', '#', '<', '>', '&':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// buildNotifyCaption 构造富入库通知（HTML 格式）：
//
//	✅ 入库完成 · 标题 (2026)  S01E01-E02
//	📺 S01E01-E02 · 本季共 12 集
//	⭐ 7.7 · 日番 · 日本
//	📦 入库 2 个文件 · STRM 已生成
//	#标题 #动画 #喜剧
//	<blockquote>简介</blockquote>
func (p *Pipeline) buildNotifyCaption(first notifyItem, items []notifyItem) string {
	var b strings.Builder
	b.WriteString("✅ 入库完成 · " + html.EscapeString(first.Title))
	if first.Year != "" {
		b.WriteString(fmt.Sprintf(" (%s)", first.Year))
	}

	segs := seasonSegments(items)
	if first.IsTV && len(segs) > 0 {
		ranges := make([]string, 0, len(segs))
		for _, seg := range segs {
			ranges = append(ranges, seg.Range)
		}
		b.WriteString("  " + strings.Join(ranges, " ｜ "))
	}
	b.WriteString("\n")

	if first.IsTV {
		for _, seg := range segs {
			line := "📺 " + seg.Range
			if seg.Total > 0 {
				line += fmt.Sprintf(" · 本季共 %d 集", seg.Total)
			}
			b.WriteString(line + "\n")
		}
	}

	if first.Rating > 0 {
		line := fmt.Sprintf("⭐ %.1f", first.Rating)
		if first.Country != "" {
			line += " · " + html.EscapeString(first.Country)
		}
		b.WriteString(line + "\n")
	}

	strmOKCount := 0
	for _, it := range items {
		if it.StrmOK {
			strmOKCount++
		}
	}
	filesLine := fmt.Sprintf("📦 入库 %d 个文件", len(items))
	if p.onVideoIngested != nil {
		switch {
		case strmOKCount == len(items):
			filesLine += " · STRM 已生成"
		case strmOKCount > 0:
			filesLine += fmt.Sprintf(" · STRM %d/%d", strmOKCount, len(items))
		default:
			filesLine += " · STRM 未生成"
		}
	}
	b.WriteString(filesLine + "\n")

	var tags []string
	addTag := func(s string) {
		if s = hashtagOf(s); s != "" {
			tags = append(tags, "#"+s)
		}
	}
	addTag(first.Title)
	for _, g := range first.Genres {
		addTag(g)
	}
	if len(tags) > 1 {
		b.WriteString(strings.Join(tags, " ") + "\n")
	}

	if first.Overview != "" {
		ov := strings.TrimSpace(first.Overview)
		b.WriteString("<blockquote expandable>" + html.EscapeString(ov) + "</blockquote>")
	}
	return strings.TrimRight(b.String(), "\n")
}

// postTgMethod 调用一个 Telegram Bot API 方法，返回是否成功（失败记日志）。
func (p *Pipeline) postTgMethod(token, method string, payload map[string]any, timeoutSecs int) bool {
	resp, err := postJSON(fmt.Sprintf("https://api.telegram.org/bot%s/%s", token, method), payload, timeoutSecs)
	if err != nil {
		log.Printf("Telegram %s 请求异常：%v", method, err)
		return false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if json.Unmarshal(body, &result) == nil && !result.OK {
		log.Printf("Telegram %s 发送失败：%s", method, string(body))
		return false
	}
	return true
}

// sendTgPhoto 发送带海报/背景图的通知；图片发送失败时降级为纯文本，通知不丢。
func (p *Pipeline) sendTgPhoto(token, chatID, posterPath, caption string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Telegram 通知异常：%v", r)
		}
	}()
	if posterPath != "" {
		ok := p.postTgMethod(token, "sendPhoto", map[string]any{
			"chat_id":    chatID,
			"photo":      tmdb.ImageURL(posterPath, "w780", p.cfg.TmdbImageBaseURL()),
			"caption":    caption,
			"parse_mode": "HTML",
		}, 15)
		if ok {
			return
		}
		log.Printf("图片通知发送失败，降级为纯文本通知")
	}
	p.postTgMethod(token, "sendMessage", map[string]any{
		"chat_id":    chatID,
		"text":       caption,
		"parse_mode": "HTML",
	}, 10)
}

func postJSON(url string, payload any, timeoutSecs int) (*http.Response, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: time.Duration(timeoutSecs) * time.Second}
	return client.Do(req)
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
