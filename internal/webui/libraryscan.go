// libraryscan.go —— 媒体库扫描服务（对应 webui/services/library_data.py + tmdb_service.py）。
package webui

import (
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"meta2cloud/internal/cloudfile"
	"meta2cloud/internal/config"
	"meta2cloud/internal/jsonx"
	"meta2cloud/internal/mediaparser"
	"meta2cloud/internal/titlematch"
	"meta2cloud/internal/tmdb"
	"meta2cloud/internal/webui/store"
)

// ── 前端数据模型（对应 schemas/library.py，JSON 键保持一致） ──

// EpisodeStatus 单集状态。
type EpisodeStatus struct {
	EpisodeNumber int    `json:"episode_number"`
	EpisodeTitle  string `json:"episode_title"`
	AirDate       string `json:"air_date"`
	InLibrary     bool   `json:"in_library"`
}

// SeasonStatus 季状态。
type SeasonStatus struct {
	SeasonNumber   int             `json:"season_number"`
	SeasonName     string          `json:"season_name"`
	PosterURL      *string         `json:"poster_url"`
	EpisodeCount   int             `json:"episode_count"`
	InLibraryCount int             `json:"in_library_count"`
	Episodes       []EpisodeStatus `json:"episodes"`
}

// MediaItem 媒体条目。
type MediaItem struct {
	TmdbID             int64          `json:"tmdb_id"`
	Title              string         `json:"title"`
	OriginalTitle      string         `json:"original_title"`
	Year               string         `json:"year"`
	MediaType          string         `json:"media_type"`
	PosterURL          *string        `json:"poster_url"`
	BackdropURL        *string        `json:"backdrop_url"`
	Overview           string         `json:"overview"`
	Rating             float64        `json:"rating"`
	Seasons            []SeasonStatus `json:"seasons,omitempty"`
	TotalEpisodes      *int           `json:"total_episodes,omitempty"`
	InLibraryEpisodes  *int           `json:"in_library_episodes,omitempty"`
	Status             *string        `json:"status,omitempty"`
	DriveFolderID      *string        `json:"drive_folder_id,omitempty"`
	ParentFolderID     *string        `json:"drive_parent_id,omitempty"`
	UpdatedAt          *string        `json:"updated_at,omitempty"`
	FolderModifiedTime *int64         `json:"folder_modified_time,omitempty"`
}

// libraryService 媒体库扫描域服务：网盘扫描（全量/增量）、身份解析与图片地址。
type libraryService struct {
	rt *Runtime
}

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ── 工具函数 ────────────────────────────────────────────────

var isoTimeRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`)

// normalizeModifiedTime 将 CloudFile.ModifiedTime（ISO 或数字串）转为秒级 Unix 时间戳。
func normalizeModifiedTime(value string) int64 {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return 0
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return n
	}
	if isoTimeRe.MatchString(raw) {
		normalized := raw
		if strings.HasSuffix(raw, "Z") {
			normalized = raw[:len(raw)-1] + "+00:00"
		}
		if t, err := time.Parse("2006-01-02T15:04:05-07:00", normalized); err == nil {
			return t.Unix()
		}
		if t, err := time.Parse("2006-01-02T15:04:05.000000-07:00", normalized); err == nil {
			return t.Unix()
		}
	}
	return 0
}

var (
	seasonEpFileRe   = regexp.MustCompile(`[Ss](\d+)[Ee](\d+)`)
	folderYearRe     = regexp.MustCompile(`\((\d{4})\)`)
	folderYearTailRe = regexp.MustCompile(`\s*\(\d{4}\)\s*$`)
	seasonFolderRe   = regexp.MustCompile(`(?i)Season\s+(\d+)`)
)

// ParseEpisodeFromFilename 从文件名解析 (season, episode)。
func ParseEpisodeFromFilename(filename string) (int, int, bool) {
	m := seasonEpFileRe.FindStringSubmatch(filename)
	if m == nil {
		return 0, 0, false
	}
	s, _ := strconv.Atoi(m[1])
	e, _ := strconv.Atoi(m[2])
	return s, e, true
}

func extractTitleYear(name string) (string, string) {
	year := ""
	if m := folderYearRe.FindStringSubmatch(name); m != nil {
		year = m[1]
	}
	title := strings.TrimSpace(folderYearTailRe.ReplaceAllString(name, ""))
	return title, year
}

// tmdbImageURL 用当前配置的图片根地址构建 URL。
func (s *libraryService) tmdbImageURL(path, size string) string {
	if path == "" {
		return ""
	}
	if size == "" {
		size = "original"
	}
	return tmdb.ImageURL(path, size, s.rt.GetConfig().TmdbImageBaseURL())
}

// ── 季集状态构建 ────────────────────────────────────────────

type epKey struct{ season, episode int }

// BuildSeasonsStatus 根据 TMDB 信息与已入库集构建 seasons 状态。
// 返回 (seasons, totalEpisodes, inLibraryEpisodes)。
func (s *libraryService) BuildSeasonsStatus(tmdbID int64, tmdbInfo map[string]any, driveEpisodes map[epKey]bool, useCache bool) ([]SeasonStatus, int, int) {
	if driveEpisodes == nil {
		driveEpisodes = map[epKey]bool{}
	}
	seasonsStatus := []SeasonStatus{}
	totalEps := 0
	inLibEps := 0

	for _, seasonRaw := range jsonx.MapSlice(tmdbInfo, "seasons") {
		if seasonRaw["season_number"] == nil {
			continue
		}
		seasonNum := int(jsonx.Int(seasonRaw, "season_number", 0))
		epCount := int(jsonx.Int(seasonRaw, "episode_count", 0))

		var seasonDetail map[string]any
		seasonCachePath := fmt.Sprintf("/tv/%d/season/%d", tmdbID, seasonNum)
		if useCache {
			if cache := s.rt.GetTmdbCache(); cache != nil {
				if cached, ok := cache.Get(seasonCachePath, s.rt.GetConfig().Tmdb.Language); ok {
					seasonDetail = cached
				}
			}
		}
		if seasonDetail == nil {
			if client := s.rt.GetTmdbClient(); client != nil {
				seasonDetail = client.GetSeasonDetail(int(tmdbID), seasonNum)
			}
		}

		episodes := []EpisodeStatus{}
		if seasonDetail != nil {
			for _, epRaw := range jsonx.MapSlice(seasonDetail, "episodes") {
				epNum := int(jsonx.Int(epRaw, "episode_number", 0))
				inLib := driveEpisodes[epKey{seasonNum, epNum}]
				title := jsonx.Str(epRaw, "name")
				if title == "" {
					title = "第 " + strconv.Itoa(epNum) + " 集"
				}
				episodes = append(episodes, EpisodeStatus{
					EpisodeNumber: epNum, EpisodeTitle: title,
					AirDate: jsonx.Str(epRaw, "air_date"), InLibrary: inLib,
				})
			}
		} else {
			for epNum := 1; epNum <= epCount; epNum++ {
				inLib := driveEpisodes[epKey{seasonNum, epNum}]
				episodes = append(episodes, EpisodeStatus{
					EpisodeNumber: epNum, EpisodeTitle: "第 " + strconv.Itoa(epNum) + " 集",
					InLibrary: inLib,
				})
			}
		}
		// 库里有、但 TMDB（主/季详情均旧）尚未登记的集号，补占位条目。
		// 新番追季场景：TMDB episode_count 滞后于实际入库，不补会出现
		// 「已入库 12/7」这类入库数大于总数的荒谬显示。
		knownNums := map[int]bool{}
		for _, ep := range episodes {
			knownNums[ep.EpisodeNumber] = true
		}
		var extraNums []int
		for k := range driveEpisodes {
			if k.season == seasonNum && !knownNums[k.episode] {
				extraNums = append(extraNums, k.episode)
			}
		}
		sort.Ints(extraNums)
		for _, epNum := range extraNums {
			episodes = append(episodes, EpisodeStatus{
				EpisodeNumber: epNum, EpisodeTitle: "第 " + strconv.Itoa(epNum) + " 集",
				InLibrary: true,
			})
		}

		// 总数取各来源最大值：主详情 episode_count 可能是早期抓取的旧值
		if len(episodes) > epCount {
			epCount = len(episodes)
		}
		totalEps += epCount

		sInLib := 0
		for _, ep := range episodes {
			if ep.InLibrary {
				sInLib++
			}
		}
		inLibEps += sInLib
		name := jsonx.Str(seasonRaw, "name")
		if name == "" {
			name = "Season " + strconv.Itoa(seasonNum)
		}
		poster := s.tmdbImageURL(jsonx.Str(seasonRaw, "poster_path"), "w500")
		seasonsStatus = append(seasonsStatus, SeasonStatus{
			SeasonNumber: seasonNum, SeasonName: name,
			PosterURL:    strPtrOrNil(poster),
			EpisodeCount: len(episodes), InLibraryCount: sInLib, Episodes: episodes,
		})
	}
	return seasonsStatus, totalEps, inLibEps
}

// ── 媒体身份识别 ────────────────────────────────────────────

func tmdbDetailPath(mediaType string, tmdbID int64) string {
	if mediaType == "tv" {
		return "/tv/" + strconv.FormatInt(tmdbID, 10)
	}
	return "/movie/" + strconv.FormatInt(tmdbID, 10)
}

// storedTitleCandidates 库条目里可参与标题比对的候选名（标题/原名/别名）。
func storedTitleCandidates(item map[string]any) []string {
	cands := []string{}
	for _, key := range []string{"title", "original_title"} {
		if s := jsonx.Str(item, key); s != "" {
			cands = append(cands, s)
		}
	}
	for _, name := range jsonx.MapSlice(item, "names") {
		if s := jsonx.Str(name, "name"); s != "" {
			cands = append(cands, s)
		}
	}
	if names, ok := item["names"].([]any); ok {
		for _, name := range names {
			if s, ok := name.(string); ok && s != "" {
				cands = append(cands, s)
			}
		}
	}
	return cands
}

// resolveTmdbInfo 库缓存优先，其次 API。
func (s *libraryService) resolveTmdbInfo(mediaType string, tmdbID int64) map[string]any {
	if tmdbID <= 0 {
		return nil
	}
	if entry := s.rt.GetLibraryStore().GetTmdbEntry(mediaType, tmdbID); entry != nil && len(entry.RawJSON) > 0 {
		return entry.RawJSON
	}
	if client := s.rt.GetTmdbClient(); client != nil {
		return client.Get(tmdbDetailPath(mediaType, tmdbID))
	}
	return nil
}

// searchTmdbIdentity 按标题搜索 TMDB。
func (s *libraryService) searchTmdbIdentity(mediaType, title, year string, cfg *config.Config) (int64, map[string]any) {
	if title == "" || !cfg.IsTmdbReady() {
		return 0, nil
	}
	client := s.rt.GetTmdbClient()
	if client == nil {
		return 0, nil
	}
	mtype := mediaparser.MediaMovie
	if mediaType == "tv" {
		mtype = mediaparser.MediaTV
	}
	found := client.SearchByName(title, year, mtype)
	if found == nil {
		return 0, nil
	}
	tmdbID := jsonx.Int(found, "tmdb_id", 0)
	if tmdbID == 0 {
		tmdbID = jsonx.Int(found, "id", 0)
	}
	if tmdbID <= 0 {
		return 0, nil
	}
	return tmdbID, found
}

// resolveMediaIdentity 多级回退识别媒体（库条目 → 标题匹配 → TMDB 缓存 → 入库历史 → TMDB 搜索）。
func (s *libraryService) resolveMediaIdentity(mediaType, driveFolderID, folderName string, cfg *config.Config) (int64, map[string]any) {
	libStore := s.rt.GetLibraryStore()
	if existing := libStore.GetLibraryItemByFolderID(driveFolderID); existing != nil {
		if tmdbID := jsonx.Int(existing, "tmdb_id", 0); tmdbID > 0 {
			// 沿用存量识别前先校验目录名：目录名与已识别标题差异过大
			// （衍生条目曾被误绑到正篇，如「未来福音 extra chorus」绑到「未来福音」）
			// 时丢弃旧 ID 重新识别，否则错误 ID 随库存记录永久延续且无从纠正
			if titlematch.TitlesMatch(folderName, storedTitleCandidates(existing)) {
				return tmdbID, s.resolveTmdbInfo(mediaType, tmdbID)
			}
			log.Printf("目录 %q 与已识别标题 %q 不一致，重新识别", folderName, jsonx.Str(existing, "title"))
		}
	}
	title, year := extractTitleYear(folderName)
	if title == "" {
		return 0, nil
	}
	if matched := libStore.FindLibraryItemByTitleYear(mediaType, title, year); matched != nil {
		if tmdbID := jsonx.Int(matched, "tmdb_id", 0); tmdbID > 0 {
			return tmdbID, s.resolveTmdbInfo(mediaType, tmdbID)
		}
	}
	if cached := libStore.FindTmdbEntryByTitleYear(mediaType, title, year); cached != nil {
		if cached.TmdbID > 0 {
			if len(cached.RawJSON) > 0 {
				return cached.TmdbID, cached.RawJSON
			}
			return cached.TmdbID, s.resolveTmdbInfo(mediaType, cached.TmdbID)
		}
	}
	if ingest := s.rt.GetIngestStore(); ingest != nil {
		if match := ingest.FindMatchByTitleYear(mediaType, title, year); match != nil {
			if match.TmdbID > 0 {
				return match.TmdbID, s.resolveTmdbInfo(mediaType, match.TmdbID)
			}
		}
	}
	if tmdbID, info := s.searchTmdbIdentity(mediaType, title, year, cfg); tmdbID > 0 && info != nil {
		return tmdbID, info
	}
	return 0, nil
}

// ── 扫描 ────────────────────────────────────────────────────

func roundTo1(v float64) float64 {
	return float64(int64(v*10+0.5)) / 10
}

func (s *libraryService) scanSingleMovieFolder(_ cloudfile.Provider, folder cloudfile.CloudFile, cfg *config.Config) *MediaItem {
	mtime := normalizeModifiedTime(folder.ModifiedTime)
	tmdbID, tmdbInfo := s.resolveMediaIdentity("movie", folder.ID, folder.Name, cfg)
	if tmdbInfo != nil {
		poster := s.tmdbImageURL(jsonx.Str(tmdbInfo, "poster_path"), "w500")
		backdrop := s.tmdbImageURL(jsonx.Str(tmdbInfo, "backdrop_path"), "")
		year := jsonx.Str(tmdbInfo, "release_date")
		if len(year) > 4 {
			year = year[:4]
		}
		title := jsonx.Str(tmdbInfo, "title")
		if title == "" {
			title = folder.Name
		}
		fid := folder.ID
		m := mtime
		return &MediaItem{
			TmdbID: tmdbID, Title: title, OriginalTitle: jsonx.Str(tmdbInfo, "original_title"),
			Year: year, MediaType: "movie",
			PosterURL: strPtrOrNil(poster), BackdropURL: strPtrOrNil(backdrop),
			Overview:      jsonx.Str(tmdbInfo, "overview"),
			Rating:        roundTo1(jsonx.Float(tmdbInfo, "vote_average", 0)),
			DriveFolderID: &fid, FolderModifiedTime: &m,
			ParentFolderID: strPtrOrNil(folder.ParentID),
		}
	}
	title, year := extractTitleYear(folder.Name)
	fid := folder.ID
	m := mtime
	return &MediaItem{
		TmdbID: 0, Title: title, Year: year, MediaType: "movie",
		DriveFolderID: &fid, FolderModifiedTime: &m,
		ParentFolderID: strPtrOrNil(folder.ParentID),
	}
}

// categoryScanRoot 分类布局下的扫描根：先按名解析媒体库根目录，再在其下定位一级分类目录。
// 任一环节未找到返回空串（上层按空串跳过该类型扫描）。
func (s *libraryService) categoryScanRoot(client cloudfile.Provider, isTV bool) string {
	cats := s.rt.GetCategories()
	if cats == nil {
		return ""
	}
	rootID, ok := cloudfile.FindFolderByName(client, s.rt.GetConfig().ActiveRootFolderID())
	if !ok {
		return ""
	}
	name := cats.Dirs.TV
	if !isTV {
		name = cats.Dirs.Movie
	}
	folders, err := client.ListFiles(rootID, 500)
	if err != nil {
		return ""
	}
	for _, f := range folders {
		if f.IsFolder() && f.Name == name {
			return f.ID
		}
	}
	return ""
}

// mediaCategoryLevel2 指定类型的二级分类目录名集合（未启用返回 nil）。
func (s *libraryService) mediaCategoryLevel2(isTV bool) map[string]bool {
	cats := s.rt.GetCategories()
	if cats == nil {
		return nil
	}
	names := map[string]bool{}
	rules := cats.TV
	if !isTV {
		rules = cats.Movie
	}
	for _, r := range rules {
		if r.Name != "" {
			names[r.Name] = true
		}
	}
	return names
}

// expandCategoryFolders 分类启用时展开媒体根下的分类目录（透传层），
// 返回分类目录的子目录 + 其余目录，供后续按媒体目录扫描。
func expandCategoryFolders(client cloudfile.Provider, folders []cloudfile.CloudFile, level2Names map[string]bool) []cloudfile.CloudFile {
	if len(level2Names) == 0 {
		return folders
	}
	out := make([]cloudfile.CloudFile, 0, len(folders))
	for _, f := range folders {
		if !f.IsFolder() || !level2Names[f.Name] {
			out = append(out, f)
			continue
		}
		children, err := client.ListFiles(f.ID, 500)
		if err != nil {
			continue
		}
		for _, c := range children {
			if c.IsFolder() {
				out = append(out, c)
			}
		}
	}
	return out
}

func (s *libraryService) ScanMovies(client cloudfile.Provider, cfg *config.Config) []MediaItem {
	movieRoot := s.categoryScanRoot(client, false)
	if movieRoot == "" {
		return []MediaItem{}
	}
	folders, err := client.ListFiles(movieRoot, 500)
	if err != nil {
		return []MediaItem{}
	}
	folders = expandCategoryFolders(client, folders, s.mediaCategoryLevel2(false))
	movies := []MediaItem{}
	for _, folder := range folders {
		if !folder.IsFolder() {
			continue
		}
		if m := s.scanSingleMovieFolder(client, folder, cfg); m != nil {
			movies = append(movies, *m)
		}
	}
	log.Printf("扫描到 %d 个电影文件夹", len(movies))
	return movies
}

// tvLeafModifiedTime 剧集增量时间按 Season 叶子目录计算，缺失时退回剧名目录。
func tvLeafModifiedTime(showFolder cloudfile.CloudFile, showFiles []cloudfile.CloudFile) int64 {
	showMtime := normalizeModifiedTime(showFolder.ModifiedTime)
	maxMtime := showMtime
	for _, item := range showFiles {
		if item.IsFolder() && strings.HasPrefix(item.Name, "Season") {
			if m := normalizeModifiedTime(item.ModifiedTime); m > maxMtime {
				maxMtime = m
			}
		}
	}
	return maxMtime
}

func (s *libraryService) scanSingleTVFolder(client cloudfile.Provider, cfg *config.Config, showFolder cloudfile.CloudFile, showFiles []cloudfile.CloudFile, useCache bool) *MediaItem {
	if showFiles == nil {
		if files, err := client.ListFiles(showFolder.ID, 200); err == nil {
			showFiles = files
		}
	}
	mtime := tvLeafModifiedTime(showFolder, showFiles)
	tmdbID, tmdbInfo := s.resolveMediaIdentity("tv", showFolder.ID, showFolder.Name, cfg)

	// 已入库集集合
	driveEpisodes := collectFolderEpisodes(client, showFolder.ID, showFiles)
	// 同一部剧在网盘上可能存在多个目录（历史整理用了不同译名，如
	// 「令和的斑小姐」与「令和妖神斑小姐」），把同 tmdb_id 其他目录
	// 的集数并入，避免入库进度被拆分显示
	if tmdbID > 0 {
		for _, otherFolderID := range s.rt.GetLibraryStore().ListFolderIDsByTmdb("tv", tmdbID) {
			if otherFolderID == showFolder.ID {
				continue
			}
			for k := range collectFolderEpisodes(client, otherFolderID, nil) {
				driveEpisodes[k] = true
			}
		}
	}

	seasonsStatus := []SeasonStatus{}
	totalEps, inLibEps := 0, 0
	if tmdbInfo != nil {
		seasonsStatus, totalEps, inLibEps = s.BuildSeasonsStatus(tmdbID, tmdbInfo, driveEpisodes, useCache)
	} else {
		seasonMap := map[int][]int{}
		for k := range driveEpisodes {
			seasonMap[k.season] = append(seasonMap[k.season], k.episode)
		}
		seasonNums := make([]int, 0, len(seasonMap))
		for s := range seasonMap {
			seasonNums = append(seasonNums, s)
		}
		sort.Ints(seasonNums)
		for _, seasonNum := range seasonNums {
			eps := seasonMap[seasonNum]
			sort.Ints(eps)
			episodes := make([]EpisodeStatus, 0, len(eps))
			for _, ep := range eps {
				episodes = append(episodes, EpisodeStatus{
					EpisodeNumber: ep, EpisodeTitle: "第 " + strconv.Itoa(ep) + " 集", InLibrary: true,
				})
			}
			inLibEps += len(eps)
			totalEps += len(eps)
			seasonsStatus = append(seasonsStatus, SeasonStatus{
				SeasonNumber: seasonNum, SeasonName: "Season " + strconv.Itoa(seasonNum),
				EpisodeCount: len(eps), InLibraryCount: len(eps), Episodes: episodes,
			})
		}
	}

	title := strings.TrimSpace(folderYearTailRe.ReplaceAllString(showFolder.Name, ""))
	year := ""
	if m := folderYearRe.FindStringSubmatch(showFolder.Name); m != nil {
		year = m[1]
	}
	originalTitle := ""
	overview := ""
	status := ""
	rating := 0.0
	var poster, backdrop *string
	if tmdbInfo != nil {
		title = jsonx.Str(tmdbInfo, "name")
		originalTitle = jsonx.Str(tmdbInfo, "original_name")
		if d := jsonx.Str(tmdbInfo, "first_air_date"); len(d) >= 4 {
			year = d[:4]
		}
		overview = jsonx.Str(tmdbInfo, "overview")
		status = jsonx.Str(tmdbInfo, "status")
		rating = roundTo1(jsonx.Float(tmdbInfo, "vote_average", 0))
		poster = strPtrOrNil(s.tmdbImageURL(jsonx.Str(tmdbInfo, "poster_path"), "w500"))
		backdrop = strPtrOrNil(s.tmdbImageURL(jsonx.Str(tmdbInfo, "backdrop_path"), ""))
	}
	fid := showFolder.ID
	m := mtime
	te, ie := totalEps, inLibEps
	return &MediaItem{
		TmdbID: tmdbID, Title: title, OriginalTitle: originalTitle, Year: year, MediaType: "tv",
		PosterURL: poster, BackdropURL: backdrop, Overview: overview, Rating: rating,
		Seasons: seasonsStatus, TotalEpisodes: &te, InLibraryEpisodes: &ie,
		Status: strPtrOrNil(status), DriveFolderID: &fid, FolderModifiedTime: &m,
		ParentFolderID: strPtrOrNil(showFolder.ParentID),
	}
}

// ScanTVShows 全量扫描剧集。
func (s *libraryService) ScanTVShows(client cloudfile.Provider, cfg *config.Config, useCache bool) []MediaItem {
	tvRoot := s.categoryScanRoot(client, true)
	if tvRoot == "" {
		return []MediaItem{}
	}
	folders, err := client.ListFiles(tvRoot, 500)
	if err != nil {
		return []MediaItem{}
	}
	folders = expandCategoryFolders(client, folders, s.mediaCategoryLevel2(true))
	shows := []MediaItem{}
	for _, showFolder := range folders {
		if !showFolder.IsFolder() {
			continue
		}
		if m := s.scanSingleTVFolder(client, cfg, showFolder, nil, useCache); m != nil {
			shows = append(shows, *m)
		}
	}
	log.Printf("扫描到 %d 个剧集文件夹", len(shows))
	return shows
}

// ScanMoviesIncremental 增量扫描电影（mtime 未变则跳过）。
func (s *libraryService) ScanMoviesIncremental(client cloudfile.Provider, cfg *config.Config, storedMtimes map[string]int64) ([]MediaItem, map[string]int64) {
	movieRoot := s.categoryScanRoot(client, false)
	if movieRoot == "" {
		return []MediaItem{}, map[string]int64{}
	}
	folders, err := client.ListFiles(movieRoot, 500)
	if err != nil {
		return []MediaItem{}, map[string]int64{}
	}
	folders = expandCategoryFolders(client, folders, s.mediaCategoryLevel2(false))
	movies := []MediaItem{}
	currentMtimes := map[string]int64{}
	for _, folder := range folders {
		if !folder.IsFolder() {
			continue
		}
		currentMtime := normalizeModifiedTime(folder.ModifiedTime)
		currentMtimes[folder.ID] = currentMtime
		storedMtime := storedMtimes[folder.ID]
		if storedMtime > 0 && currentMtime > 0 && currentMtime <= storedMtime {
			continue
		}
		if m := s.scanSingleMovieFolder(client, folder, cfg); m != nil {
			movies = append(movies, *m)
		}
	}
	return movies, currentMtimes
}

// ScanTVShowsIncremental 增量扫描剧集。
func (s *libraryService) ScanTVShowsIncremental(client cloudfile.Provider, cfg *config.Config, storedMtimes map[string]int64) ([]MediaItem, map[string]int64) {
	tvRoot := s.categoryScanRoot(client, true)
	if tvRoot == "" {
		return []MediaItem{}, map[string]int64{}
	}
	folders, err := client.ListFiles(tvRoot, 500)
	if err != nil {
		return []MediaItem{}, map[string]int64{}
	}
	folders = expandCategoryFolders(client, folders, s.mediaCategoryLevel2(true))
	shows := []MediaItem{}
	currentMtimes := map[string]int64{}

	// 第一遍：按 mtime 判定需要重扫的目录
	changed := map[string]bool{}
	folderByID := map[string]cloudfile.CloudFile{}
	for _, showFolder := range folders {
		if !showFolder.IsFolder() {
			continue
		}
		folderByID[showFolder.ID] = showFolder
		showFiles, err := client.ListFiles(showFolder.ID, 200)
		if err != nil {
			continue
		}
		currentMtime := tvLeafModifiedTime(showFolder, showFiles)
		currentMtimes[showFolder.ID] = currentMtime
		storedMtime := storedMtimes[showFolder.ID]
		if storedMtime > 0 && currentMtime > 0 && currentMtime <= storedMtime {
			continue
		}
		changed[showFolder.ID] = true
	}

	// 第二遍：同剧兄弟目录联动重扫。某目录变化时，同 tmdb_id 的其他目录
	// 也必须重扫——跨目录集数聚合只发生在 scanSingleTVFolder 内，若兄弟
	// 目录被 mtime 跳过，会保留旧的未聚合值（表现为集数统计不更新）。
	if libStore := s.rt.GetLibraryStore(); libStore != nil {
		for folderID := range changed {
			if tmdbID, ok := libStore.GetTmdbIDByFolderID(folderID); ok && tmdbID > 0 {
				for _, sib := range libStore.ListFolderIDsByTmdb("tv", tmdbID) {
					if sib != folderID {
						changed[sib] = true
					}
				}
			}
		}
	}

	for _, showFolder := range folders {
		if !showFolder.IsFolder() || !changed[showFolder.ID] {
			continue
		}
		var showFiles []cloudfile.CloudFile
		if files, err := client.ListFiles(showFolder.ID, 200); err == nil {
			showFiles = files
		}
		if m := s.scanSingleTVFolder(client, cfg, showFolder, showFiles, true); m != nil {
			shows = append(shows, *m)
		}
	}
	return shows, currentMtimes
}

// mediaItemToRow MediaItem → 库行结构（store.LibraryMediaRow，键经 json tag 编译期约束）。
func mediaItemToRow(m MediaItem) store.LibraryMediaRow {
	row := store.LibraryMediaRow{
		TmdbID: m.TmdbID, Title: m.Title, OriginalTitle: m.OriginalTitle,
		Year: m.Year, MediaType: m.MediaType,
		PosterURL: m.PosterURL, BackdropURL: m.BackdropURL,
		Overview: m.Overview, Rating: m.Rating,
		Status: m.Status, DriveFolderID: strDeref(m.DriveFolderID),
		DriveParentID: m.ParentFolderID,
	}
	if m.TotalEpisodes != nil {
		v := int64(*m.TotalEpisodes)
		row.TotalEpisodes = &v
	}
	if m.InLibraryEpisodes != nil {
		v := int64(*m.InLibraryEpisodes)
		row.InLibraryEpisodes = &v
	}
	if m.FolderModifiedTime != nil {
		row.FolderModifiedTime = m.FolderModifiedTime
	}
	if m.Seasons != nil {
		row.Seasons = make([]map[string]any, 0, len(m.Seasons))
		for _, s := range m.Seasons {
			episodes := make([]map[string]any, 0, len(s.Episodes))
			for _, e := range s.Episodes {
				episodes = append(episodes, map[string]any{
					"episode_number": e.EpisodeNumber, "episode_title": e.EpisodeTitle,
					"air_date": e.AirDate, "in_library": e.InLibrary,
				})
			}
			season := map[string]any{
				"season_number": s.SeasonNumber, "season_name": s.SeasonName,
				"episode_count": s.EpisodeCount, "in_library_count": s.InLibraryCount,
				"episodes": episodes,
			}
			if s.PosterURL != nil {
				season["poster_url"] = *s.PosterURL
			}
			row.Seasons = append(row.Seasons, season)
		}
	}
	return row
}

func strDeref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// RefreshLibrary 刷新媒体库（增量/全量），对应 services/pipeline._do_refresh_library。
func (s *libraryService) RefreshLibrary(incremental bool) map[string]any {
	client := s.rt.GetStorageProvider()
	cfg := s.rt.GetConfig()
	libStore := s.rt.GetLibraryStore()
	if client == nil {
		return map[string]any{"error": "存储 Provider 不可用"}
	}
	if incremental {
		storedMtimes := libStore.GetAllFolderModifiedTimes()
		movies, movieMtimes := s.ScanMoviesIncremental(client, cfg, storedMtimes)
		tvShows, tvMtimes := s.ScanTVShowsIncremental(client, cfg, storedMtimes)
		allMtimes := map[string]int64{}
		for k, v := range movieMtimes {
			allMtimes[k] = v
		}
		for k, v := range tvMtimes {
			allMtimes[k] = v
		}
		items := append(mediaItemsToRows(movies), mediaItemsToRows(tvShows)...)
		diff, err := libStore.SaveIncremental(items, allMtimes)
		if err != nil {
			log.Printf("刷新媒体库：增量保存失败：%v", err)
		}
		if _, err := libStore.MarkMissingFolders(mapKeys(allMtimes)); err != nil {
			log.Printf("刷新媒体库：标记缺失文件夹失败：%v", err)
		}
		return map[string]any{
			"new_items": diff.NewItems, "updated_items": diff.UpdatedItems,
			"total": diff.Total, "scanned_at": diff.ScannedAt,
		}
	}
	movies := s.ScanMovies(client, cfg)
	tvShows := s.ScanTVShows(client, cfg, false)
	diff := libStore.SaveSnapshot(mediaItemsToRows(movies), mediaItemsToRows(tvShows))
	return map[string]any{
		"new_movies": diff.NewMovies, "new_tv": diff.NewTV,
		"total_movies": diff.TotalMovies, "total_tv": diff.TotalTV,
		"scanned_at": diff.ScannedAt,
	}
}

func mediaItemsToRows(items []MediaItem) []store.LibraryMediaRow {
	out := make([]store.LibraryMediaRow, 0, len(items))
	for _, m := range items {
		out = append(out, mediaItemToRow(m))
	}
	return out
}

func mapKeys(m map[string]int64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// collectFolderEpisodes 收集一个剧集目录下所有 Season 子目录中
// 可解析出集号的视频文件，返回 {(season, episode)} 集合。
// showFiles 为 nil 时自动列目录。
func collectFolderEpisodes(client cloudfile.Provider, folderID string, showFiles []cloudfile.CloudFile) map[epKey]bool {
	episodes := map[epKey]bool{}
	if showFiles == nil {
		if files, err := client.ListFiles(folderID, 200); err == nil {
			showFiles = files
		}
	}
	for _, seasonFolder := range showFiles {
		if !seasonFolder.IsFolder() || !strings.HasPrefix(seasonFolder.Name, "Season") {
			continue
		}
		m := seasonFolderRe.FindStringSubmatch(seasonFolder.Name)
		if m == nil {
			continue
		}
		seasonNum, _ := strconv.Atoi(m[1])
		seasonFiles, err := client.ListFiles(seasonFolder.ID, 500)
		if err != nil {
			continue
		}
		for _, file := range seasonFiles {
			if file.IsVideo() {
				if s, e, ok := ParseEpisodeFromFilename(file.Name); ok && s == seasonNum {
					episodes[epKey{s, e}] = true
				}
			}
		}
	}
	return episodes
}
