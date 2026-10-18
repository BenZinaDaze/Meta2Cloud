// Package store WebUI SQLite 存储层（对应 Python 版 webui/*_store.py）。
//
// library.go —— 实体化媒体存储（tmdb_media / library_media / app_state）。
// 表结构与 Python 版完全一致，可直接复用既有 library.db。
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"meta2cloud/internal/jsonx"
	"meta2cloud/internal/tmdb"
)

const libraryDDL = `
CREATE TABLE IF NOT EXISTS tmdb_media (
    media_type          TEXT    NOT NULL,
    tmdb_id             INTEGER NOT NULL,
    language            TEXT    NOT NULL DEFAULT '',
    title               TEXT    NOT NULL DEFAULT '',
    original_title      TEXT    NOT NULL DEFAULT '',
    original_language   TEXT    NOT NULL DEFAULT '',
    overview            TEXT    NOT NULL DEFAULT '',
    poster_path         TEXT    NOT NULL DEFAULT '',
    backdrop_path       TEXT    NOT NULL DEFAULT '',
    release_date        TEXT    NOT NULL DEFAULT '',
    first_air_date      TEXT    NOT NULL DEFAULT '',
    status              TEXT    NOT NULL DEFAULT '',
    vote_average        REAL    NOT NULL DEFAULT 0,
    vote_count          INTEGER NOT NULL DEFAULT 0,
    raw_json            TEXT    NOT NULL DEFAULT '{}',
    season_details_json TEXT    NOT NULL DEFAULT '{}',
    episode_details_json TEXT   NOT NULL DEFAULT '{}',
    last_synced_at      TEXT    NOT NULL,
    expires_at          REAL    NOT NULL DEFAULT 0,
    PRIMARY KEY (media_type, tmdb_id)
);

CREATE TABLE IF NOT EXISTS library_media (
    drive_folder_id      TEXT    PRIMARY KEY,
    media_type           TEXT    NOT NULL,
    tmdb_id              INTEGER NOT NULL DEFAULT 0,
    title                TEXT    NOT NULL DEFAULT '',
    original_title       TEXT    NOT NULL DEFAULT '',
    year                 TEXT    NOT NULL DEFAULT '',
    poster_url           TEXT,
    backdrop_url         TEXT,
    overview             TEXT    NOT NULL DEFAULT '',
    rating               REAL    NOT NULL DEFAULT 0,
    status               TEXT    NOT NULL DEFAULT '',
    total_episodes       INTEGER,
    in_library_episodes  INTEGER,
    raw_json             TEXT    NOT NULL DEFAULT '{}',
    in_library           INTEGER NOT NULL DEFAULT 1,
    folder_modified_time INTEGER NOT NULL DEFAULT 0,
    parent_id            TEXT    NOT NULL DEFAULT '',
    last_scanned_at      TEXT    NOT NULL,
    updated_at           TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_library_media_tmdb
ON library_media(media_type, tmdb_id)
WHERE tmdb_id > 0;

CREATE INDEX IF NOT EXISTS idx_library_media_media_type
ON library_media(media_type);

CREATE INDEX IF NOT EXISTS idx_tmdb_media_title
ON tmdb_media(title);

CREATE TABLE IF NOT EXISTS app_state (
    key    TEXT PRIMARY KEY,
    value  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS media_title_pins (
    media_type   TEXT    NOT NULL,
    tmdb_id      INTEGER NOT NULL,
    pinned_title TEXT    NOT NULL,
    pinned_year  TEXT    NOT NULL DEFAULT '',
    source       TEXT    NOT NULL DEFAULT '',
    created_at   TEXT    NOT NULL,
    updated_at   TEXT    NOT NULL,
    PRIMARY KEY (media_type, tmdb_id)
);
`

const scanAtKey = "library.last_scanned_at"

// LibraryStore 实体化媒体存储。
type LibraryStore struct {
	db *sql.DB
	mu sync.Mutex

	// ImageBaseURL 注入 TMDB 图片根地址（hydrate 时使用）。
	ImageBaseURL func() string
}

// OpenLibraryStore 打开（或创建）媒体实体库。
func OpenLibraryStore(dbPath string) (*LibraryStore, error) {
	if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	// 单连接 + WAL，与 Python 版单连接行为一致，避免写锁竞争
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(libraryDDL); err != nil {
		db.Close()
		return nil, err
	}
	s := &LibraryStore{db: db, ImageBaseURL: func() string { return "" }}
	if err := s.runMigrations(); err != nil {
		db.Close()
		return nil, err
	}
	log.Printf("媒体实体存储启动：%s", dbPath)
	return s, nil
}

// Close 关闭。
func (s *LibraryStore) Close() error { return s.db.Close() }

func (s *LibraryStore) runMigrations() error {
	rows, err := s.db.Query("PRAGMA table_info(library_media)")
	if err != nil {
		return err
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return err
		}
		columns[name] = true
	}
	rows.Close()
	if !columns["folder_modified_time"] {
		if _, err := s.db.Exec("ALTER TABLE library_media ADD COLUMN folder_modified_time INTEGER NOT NULL DEFAULT 0"); err != nil {
			return err
		}
		log.Printf("已为 library_media 表新增 folder_modified_time 字段")
	}
	if !columns["parent_id"] {
		if _, err := s.db.Exec("ALTER TABLE library_media ADD COLUMN parent_id TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
		log.Printf("已为 library_media 表新增 parent_id 字段")
	}
	return nil
}

func utcNow() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000000-07:00")
}

// ── 标题匹配工具（与 Python 版一致） ──────────────────────

var yearSuffixRe = regexp.MustCompile(`\s*\(\d{4}\)\s*$`)
var whitespaceRe = regexp.MustCompile(`\s+`)
var nonAlnumRe = regexp.MustCompile(`[^0-9a-z\x{4e00}-\x{9fff}]`)
var titleSplitRe = regexp.MustCompile(`[~～:：|／/]+`)

func normalizeLookupTitle(title string) string {
	raw := strings.TrimSpace(title)
	raw = yearSuffixRe.ReplaceAllString(raw, "")
	raw = whitespaceRe.ReplaceAllString(raw, " ")
	return strings.ToLower(raw)
}

func compactLookupTitle(title string) string {
	return nonAlnumRe.ReplaceAllString(normalizeLookupTitle(title), "")
}

func lookupTitleVariants(title string) map[string]bool {
	raw := strings.TrimSpace(title)
	variants := map[string]bool{}
	if raw == "" {
		return variants
	}
	variants[normalizeLookupTitle(raw)] = true
	variants[compactLookupTitle(raw)] = true
	for _, part := range titleSplitRe.Split(raw, -1) {
		if n := normalizeLookupTitle(part); n != "" {
			variants[n] = true
		}
		if c := compactLookupTitle(part); c != "" {
			variants[c] = true
		}
	}
	delete(variants, "")
	return variants
}

// TitlesMatch 标题模糊匹配（变体交集或 ≥4 字符的包含关系）。
func TitlesMatch(query string, candidates []string) bool {
	queryVariants := lookupTitleVariants(query)
	if len(queryVariants) == 0 {
		return false
	}
	candidateVariants := map[string]bool{}
	for _, candidate := range candidates {
		for v := range lookupTitleVariants(candidate) {
			candidateVariants[v] = true
		}
	}
	if len(candidateVariants) == 0 {
		return false
	}
	for q := range queryVariants {
		if candidateVariants[q] {
			return true
		}
	}
	for q := range queryVariants {
		for c := range candidateVariants {
			if len(q) < 4 || len(c) < 4 {
				continue
			}
			if strings.Contains(c, q) || strings.Contains(q, c) {
				return true
			}
		}
	}
	return false
}

// ── app_state ───────────────────────────────────────────────

func (s *LibraryStore) setState(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO app_state(key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *LibraryStore) getState(key string) (string, bool) {
	var value string
	err := s.db.QueryRow("SELECT value FROM app_state WHERE key = ?", key).Scan(&value)
	if err != nil {
		return "", false
	}
	return value, true
}

// ── 图片 URL 规范化 / 补水 ─────────────────────────────────

func (s *LibraryStore) normalizeLibraryPayload(payload map[string]any) map[string]any {
	payload["poster_url"] = tmdb.ExtractImagePath(anyStr(payload["poster_url"]))
	payload["backdrop_url"] = tmdb.ExtractImagePath(anyStr(payload["backdrop_url"]))
	if seasons, ok := payload["seasons"].([]any); ok {
		for i, season := range seasons {
			if d, ok := season.(map[string]any); ok {
				d["poster_url"] = tmdb.ExtractImagePath(anyStr(d["poster_url"]))
				seasons[i] = d
			}
		}
	}
	return payload
}

func (s *LibraryStore) hydrateLibraryPayload(payload map[string]any) map[string]any {
	baseURL := ""
	if s.ImageBaseURL != nil {
		baseURL = s.ImageBaseURL()
	}
	payload["poster_url"] = tmdb.ImageURL(tmdb.ExtractImagePath(anyStr(payload["poster_url"])), "w500", baseURL)
	payload["backdrop_url"] = tmdb.ImageURL(tmdb.ExtractImagePath(anyStr(payload["backdrop_url"])), "", baseURL)
	if seasons, ok := payload["seasons"].([]any); ok {
		for i, season := range seasons {
			if d, ok := season.(map[string]any); ok {
				d["poster_url"] = tmdb.ImageURL(tmdb.ExtractImagePath(anyStr(d["poster_url"])), "w500", baseURL)
				seasons[i] = d
			}
		}
	}
	return payload
}

func anyStr(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case *string:
		if s != nil {
			return *s
		}
	}
	return ""
}

func marshalJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// ── TMDB 缓存 ───────────────────────────────────────────────

// TmdbMediaRow tmdb_media 行。
type TmdbMediaRow struct {
	MediaType         string
	TmdbID            int64
	Language          string
	Title             string
	OriginalTitle     string
	OriginalLanguage  string
	Overview          string
	PosterPath        string
	BackdropPath      string
	ReleaseDate       string
	FirstAirDate      string
	Status            string
	VoteAverage       float64
	VoteCount         int64
	RawJSON           map[string]any
	SeasonDetailsJSON map[string]any
	EpisodeDetails    map[string]any
	LastSyncedAt      string
	ExpiresAt         float64
}

// GetTmdbEntry 读取 TMDB 实体缓存；不存在返回 nil。
func (s *LibraryStore) GetTmdbEntry(mediaType string, tmdbID int64) *TmdbMediaRow {
	row := s.db.QueryRow(`SELECT media_type, tmdb_id, language, title, original_title, original_language,
		overview, poster_path, backdrop_path, release_date, first_air_date, status, vote_average, vote_count,
		raw_json, season_details_json, episode_details_json, last_synced_at, expires_at
		FROM tmdb_media WHERE media_type = ? AND tmdb_id = ?`, mediaType, tmdbID)
	var r TmdbMediaRow
	var rawJSON, seasonJSON, episodeJSON string
	err := row.Scan(&r.MediaType, &r.TmdbID, &r.Language, &r.Title, &r.OriginalTitle, &r.OriginalLanguage,
		&r.Overview, &r.PosterPath, &r.BackdropPath, &r.ReleaseDate, &r.FirstAirDate, &r.Status,
		&r.VoteAverage, &r.VoteCount, &rawJSON, &seasonJSON, &episodeJSON, &r.LastSyncedAt, &r.ExpiresAt)
	if err != nil {
		return nil
	}
	r.RawJSON = decodeJSONObject(rawJSON)
	r.SeasonDetailsJSON = decodeJSONObject(seasonJSON)
	r.EpisodeDetails = decodeJSONObject(episodeJSON)
	return &r
}

func decodeJSONObject(s string) map[string]any {
	out := map[string]any{}
	if s != "" {
		_ = json.Unmarshal([]byte(s), &out)
	}
	return out
}

// UpsertTmdbDetail 写入/更新 TMDB 主信息。
func (s *LibraryStore) UpsertTmdbDetail(mediaType string, tmdbID int64, language string, data map[string]any, expiresAt float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.GetTmdbEntry(mediaType, tmdbID)
	seasons := map[string]any{}
	episodes := map[string]any{}
	if current != nil {
		seasons = current.SeasonDetailsJSON
		episodes = current.EpisodeDetails
	}
	_, err := s.db.Exec(`INSERT INTO tmdb_media(
		media_type, tmdb_id, language, title, original_title, original_language,
		overview, poster_path, backdrop_path, release_date, first_air_date,
		status, vote_average, vote_count, raw_json, season_details_json,
		episode_details_json, last_synced_at, expires_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(media_type, tmdb_id) DO UPDATE SET
		language = excluded.language, title = excluded.title,
		original_title = excluded.original_title, original_language = excluded.original_language,
		overview = excluded.overview, poster_path = excluded.poster_path,
		backdrop_path = excluded.backdrop_path, release_date = excluded.release_date,
		first_air_date = excluded.first_air_date, status = excluded.status,
		vote_average = excluded.vote_average, vote_count = excluded.vote_count,
		raw_json = excluded.raw_json, season_details_json = excluded.season_details_json,
		episode_details_json = excluded.episode_details_json,
		last_synced_at = excluded.last_synced_at, expires_at = excluded.expires_at`,
		mediaType, tmdbID, language,
		firstNonEmpty(jsonx.Str(data, "title"), jsonx.Str(data, "name")),
		firstNonEmpty(jsonx.Str(data, "original_title"), jsonx.Str(data, "original_name")),
		jsonx.Str(data, "original_language"),
		jsonx.Str(data, "overview"),
		jsonx.Str(data, "poster_path"),
		jsonx.Str(data, "backdrop_path"),
		jsonx.Str(data, "release_date"),
		jsonx.Str(data, "first_air_date"),
		jsonx.Str(data, "status"),
		jsonx.Float(data, "vote_average", 0),
		jsonx.Int(data, "vote_count", 0),
		marshalJSON(data),
		marshalJSON(seasons),
		marshalJSON(episodes),
		utcNow(),
		expiresAt,
	)
	return err
}

// UpsertTmdbSeasonDetail 写入/更新某季详情。
func (s *LibraryStore) UpsertTmdbSeasonDetail(tmdbID int64, language string, seasonNumber int, data map[string]any, expiresAt float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.GetTmdbEntry("tv", tmdbID)
	seasons := map[string]any{}
	episodes := map[string]any{}
	if current != nil {
		seasons = current.SeasonDetailsJSON
		episodes = current.EpisodeDetails
	}
	seasons[fmt.Sprintf("%d", seasonNumber)] = data
	_, err := s.db.Exec(`INSERT INTO tmdb_media(
		media_type, tmdb_id, language, raw_json, season_details_json,
		episode_details_json, last_synced_at, expires_at
	) VALUES ('tv', ?, ?, '{}', ?, ?, ?, ?)
	ON CONFLICT(media_type, tmdb_id) DO UPDATE SET
		language = excluded.language, season_details_json = excluded.season_details_json,
		episode_details_json = excluded.episode_details_json,
		last_synced_at = excluded.last_synced_at, expires_at = excluded.expires_at`,
		tmdbID, language, marshalJSON(seasons), marshalJSON(episodes), utcNow(), expiresAt)
	return err
}

// UpsertTmdbEpisodeDetail 写入/更新某集详情。
func (s *LibraryStore) UpsertTmdbEpisodeDetail(tmdbID int64, language string, seasonNumber, episodeNumber int, data map[string]any, expiresAt float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.GetTmdbEntry("tv", tmdbID)
	seasons := map[string]any{}
	episodes := map[string]any{}
	if current != nil {
		seasons = current.SeasonDetailsJSON
		episodes = current.EpisodeDetails
	}
	episodes[fmt.Sprintf("%d:%d", seasonNumber, episodeNumber)] = data
	_, err := s.db.Exec(`INSERT INTO tmdb_media(
		media_type, tmdb_id, language, raw_json, season_details_json,
		episode_details_json, last_synced_at, expires_at
	) VALUES ('tv', ?, ?, '{}', ?, ?, ?, ?)
	ON CONFLICT(media_type, tmdb_id) DO UPDATE SET
		language = excluded.language, season_details_json = excluded.season_details_json,
		episode_details_json = excluded.episode_details_json,
		last_synced_at = excluded.last_synced_at, expires_at = excluded.expires_at`,
		tmdbID, language, marshalJSON(seasons), marshalJSON(episodes), utcNow(), expiresAt)
	return err
}

// EvictTmdbExpired 清理过期 TMDB 缓存，返回删除行数。
func (s *LibraryStore) EvictTmdbExpired() (int64, error) {
	res, err := s.db.Exec("DELETE FROM tmdb_media WHERE expires_at > 0 AND expires_at < ?", time.Now().UTC().Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// TmdbStats 缓存统计。
func (s *LibraryStore) TmdbStats() map[string]int64 {
	var total, valid sql.NullInt64
	err := s.db.QueryRow(
		"SELECT COUNT(*), SUM(CASE WHEN expires_at >= ? THEN 1 ELSE 0 END) FROM tmdb_media",
		time.Now().UTC().Unix()).Scan(&total, &valid)
	if err != nil {
		return map[string]int64{"total": 0, "valid": 0, "expired": 0}
	}
	t := total.Int64
	v := valid.Int64
	return map[string]int64{"total": t, "valid": v, "expired": t - v}
}

// ── 媒体库快照 ──────────────────────────────────────────────

// Snapshot 媒体库快照。
type Snapshot struct {
	ScannedAt   string           `json:"scanned_at"`
	Movies      []map[string]any `json:"movies"`
	TvShows     []map[string]any `json:"tv_shows"`
	TotalMovies int              `json:"total_movies"`
	TotalTV     int              `json:"total_tv"`
}

// GetSnapshot 读取快照；未扫描过返回 nil。
func (s *LibraryStore) GetSnapshot(sortBy, sortOrder string) *Snapshot {
	scannedAt, ok := s.getState(scanAtKey)
	if !ok {
		return nil
	}
	sortColumn := "updated_at"
	switch sortBy {
	case "updated_at", "year", "title", "folder_modified_time":
		sortColumn = sortBy
	}
	order := "ASC"
	if strings.EqualFold(sortOrder, "desc") {
		order = "DESC"
	}
	titleOrder := "title COLLATE NOCASE ASC"
	if sortColumn == "title" {
		titleOrder = "title COLLATE NOCASE " + order
	}
	query := fmt.Sprintf(`SELECT media_type, raw_json, updated_at, folder_modified_time
		FROM library_media WHERE in_library = 1
		ORDER BY media_type ASC, %s %s, %s`, sortColumn, order, titleOrder)
	rows, err := s.db.Query(query)
	if err != nil {
		return &Snapshot{ScannedAt: scannedAt, Movies: []map[string]any{}, TvShows: []map[string]any{}}
	}
	defer rows.Close()
	snap := &Snapshot{ScannedAt: scannedAt, Movies: []map[string]any{}, TvShows: []map[string]any{}}
	// 同一部剧存在多个目录（历史译名差异，如「令和的斑小姐」与「令和妖神斑小姐」）
	// 时按 (media_type, tmdb_id>0) 合并为一张卡片：取最近更新的条目展示，
	// 集数取各目录最大值，避免同一部剧出现多张卡片、且详情与卡片对不上。
	seenMovie := map[int64]bool{}
	seenTV := map[int64]bool{}
	for rows.Next() {
		var mediaType, rawJSON, updatedAt string
		var folderModified int64
		if err := rows.Scan(&mediaType, &rawJSON, &updatedAt, &folderModified); err != nil {
			continue
		}
		payload := decodeJSONObject(rawJSON)
		payload = s.hydrateLibraryPayload(payload)
		payload["updated_at"] = updatedAt
		payload["folder_modified_time"] = folderModified

		target := &snap.TvShows
		seen := seenTV
		if mediaType == "movie" {
			target = &snap.Movies
			seen = seenMovie
		}
		tmdbID := int64(jsonx.Int(payload, "tmdb_id", 0))
		if tmdbID > 0 {
			if seen[tmdbID] {
				mergeDedupedEntry(*target, tmdbID, payload)
				continue
			}
			seen[tmdbID] = true
		}
		*target = append(*target, payload)
	}
	snap.TotalMovies = len(snap.Movies)
	snap.TotalTV = len(snap.TvShows)
	return snap
}

// LastScanned 最近一次扫描时间。
func (s *LibraryStore) LastScanned() (string, bool) {
	return s.getState(scanAtKey)
}

// ── 快照分页查询 ────────────────────────────────────────────

// SnapshotQuery 媒体库快照分页查询。
type SnapshotQuery struct {
	MediaType string // "" 全部 / movie / tv
	SortBy    string // updated_at | year | title | folder_modified_time
	SortOrder string
	Keyword   string // 标题/原始标题/年份模糊匹配
	Page      int    // 从 1 开始
	PageSize  int
}

// SnapshotPage 分页快照：单页去重条目与去重后总数。
type SnapshotPage struct {
	ScannedAt   string
	Movies      []map[string]any
	TvShows     []map[string]any
	TotalMovies int
	TotalTV     int
}

// libraryDedupKey 同一媒体多目录合并键：tmdb>0 按实体，否则按目录。
const libraryDedupKey = `CASE WHEN tmdb_id > 0 THEN 't' || tmdb_id ELSE 'f' || drive_folder_id END`

func snapshotSortColumn(sortBy string) string {
	switch sortBy {
	case "updated_at", "year", "title", "folder_modified_time":
		return sortBy
	}
	return "updated_at"
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// snapshotWhere 构造快照过滤条件；mediaType 非空时覆盖查询中的类型过滤。
func snapshotWhere(q SnapshotQuery, mediaType string) (string, []any) {
	w := "in_library = 1"
	args := []any{}
	mt := q.MediaType
	if mediaType != "" {
		mt = mediaType
	}
	if mt == "movie" || mt == "tv" {
		w += " AND media_type = ?"
		args = append(args, mt)
	}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		like := "%" + escapeLike(kw) + "%"
		w += " AND (title LIKE ? ESCAPE '\\' OR original_title LIKE ? ESCAPE '\\' OR CAST(year AS TEXT) LIKE ? ESCAPE '\\')"
		args = append(args, like, like, like)
	}
	return w, args
}

// GetSnapshotPage 分页读取媒体库快照。同 tmdb 多目录的合并语义与 GetSnapshot 一致
// （代表条目取最近更新，集数/时间戳各目录取最大值），仅对去重后的结果分页。
func (s *LibraryStore) GetSnapshotPage(q SnapshotQuery) *SnapshotPage {
	scannedAt, ok := s.getState(scanAtKey)
	if !ok {
		return nil
	}
	page := q.Page
	if page < 1 {
		page = 1
	}
	size := q.PageSize
	if size < 1 {
		size = 60
	}
	if size > 200 {
		size = 200
	}
	offset := (page - 1) * size

	sortColumn := snapshotSortColumn(q.SortBy)
	order := "ASC"
	if strings.EqualFold(q.SortOrder, "desc") {
		order = "DESC"
	}
	titleOrder := "title COLLATE NOCASE ASC"
	if sortColumn == "title" {
		titleOrder = "title COLLATE NOCASE " + order
	}

	// 去重后总数（两类各自统计：统计卡始终展示全量总数，不受当前页类型过滤影响）
	totalCount := func(mt string) int {
		w, a := snapshotWhere(q, mt)
		var n int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM (SELECT 1 FROM library_media WHERE `+w+` GROUP BY `+libraryDedupKey+`)`, a...).Scan(&n)
		return n
	}

	// 单页键：窗口函数取每个合并键最新一行，再按排序取当页
	w, a := snapshotWhere(q, "")
	keysQuery := `SELECT media_type, tmdb_id, drive_folder_id FROM (
		SELECT media_type, tmdb_id, drive_folder_id, title, year, updated_at, folder_modified_time,
			ROW_NUMBER() OVER (PARTITION BY ` + libraryDedupKey + ` ORDER BY updated_at DESC, drive_folder_id DESC) AS rn
		FROM library_media WHERE ` + w + `
	) WHERE rn = 1 ORDER BY media_type ASC, ` + sortColumn + ` ` + order + `, ` + titleOrder + ` LIMIT ? OFFSET ?`
	keyArgs := append(append([]any{}, a...), size, offset)
	keyRows, err := s.db.Query(keysQuery, keyArgs...)
	if err != nil {
		return &SnapshotPage{ScannedAt: scannedAt, Movies: []map[string]any{}, TvShows: []map[string]any{}}
	}
	type pageKey struct {
		mediaType string
		tmdbID    int64
		folderID  string
	}
	keys := []pageKey{}
	for keyRows.Next() {
		var k pageKey
		if err := keyRows.Scan(&k.mediaType, &k.tmdbID, &k.folderID); err != nil {
			continue
		}
		keys = append(keys, k)
	}
	keyRows.Close()

	// 取当页键对应的全部行（含同 tmdb 的重复目录），组内合并
	tmdbIDs := map[string][]any{} // media_type → ids
	folderIDs := []any{}
	for _, k := range keys {
		if k.tmdbID > 0 {
			tmdbIDs[k.mediaType] = append(tmdbIDs[k.mediaType], k.tmdbID)
		} else {
			folderIDs = append(folderIDs, k.folderID)
		}
	}
	loadRows := func(query string, args []any) *sql.Rows {
		rows, err := s.db.Query(query, args...)
		if err != nil {
			return nil
		}
		return rows
	}
	pageRows := map[string][]map[string]any{} // dedupKey → 组内载荷（ hydrated ）
	handleRow := func(mediaType, rawJSON, updatedAt string, folderModified int64) {
		payload := decodeJSONObject(rawJSON)
		payload = s.hydrateLibraryPayload(payload)
		payload["updated_at"] = updatedAt
		payload["folder_modified_time"] = folderModified
		tmdbID := int64(jsonx.Int(payload, "tmdb_id", 0))
		key := "f" + jsonx.Str(payload, "drive_folder_id")
		if tmdbID > 0 {
			key = "t" + strconv.FormatInt(tmdbID, 10)
		}
		pageRows[key] = append(pageRows[key], payload)
	}
	for mt, ids := range tmdbIDs {
		if len(ids) == 0 {
			continue
		}
		ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
		args2 := append([]any{mt}, ids...)
		rows := loadRows(`SELECT media_type, raw_json, updated_at, folder_modified_time
			FROM library_media WHERE in_library = 1 AND media_type = ? AND tmdb_id IN (`+ph+`)`, args2)
		if rows == nil {
			continue
		}
		for rows.Next() {
			var mt2, rawJSON, updatedAt string
			var folderModified int64
			if rows.Scan(&mt2, &rawJSON, &updatedAt, &folderModified) == nil {
				handleRow(mt2, rawJSON, updatedAt, folderModified)
			}
		}
		rows.Close()
	}
	if len(folderIDs) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(folderIDs)), ",")
		rows := loadRows(`SELECT media_type, raw_json, updated_at, folder_modified_time
			FROM library_media WHERE in_library = 1 AND drive_folder_id IN (`+ph+`)`, folderIDs)
		if rows != nil {
			for rows.Next() {
				var mt2, rawJSON, updatedAt string
				var folderModified int64
				if rows.Scan(&mt2, &rawJSON, &updatedAt, &folderModified) == nil {
					handleRow(mt2, rawJSON, updatedAt, folderModified)
				}
			}
			rows.Close()
		}
	}

	// 组装：代表条目 = 组内最新更新（页键即按最新选出），重复目录并入（集数/时间戳取最大）
	pickLatest := func(rows []map[string]any) map[string]any {
		latest := rows[0]
		for _, r := range rows[1:] {
			if s, _ := r["updated_at"].(string); s > strOfAny(latest["updated_at"]) {
				latest = r
			}
		}
		return latest
	}
	snap := &SnapshotPage{ScannedAt: scannedAt, Movies: []map[string]any{}, TvShows: []map[string]any{}}
	for _, k := range keys {
		key := "f" + k.folderID
		if k.tmdbID > 0 {
			key = "t" + strconv.FormatInt(k.tmdbID, 10)
		}
		rowsFor := pageRows[key]
		if len(rowsFor) == 0 {
			continue
		}
		rep := pickLatest(rowsFor)
		target := &snap.TvShows
		if k.mediaType == "movie" {
			target = &snap.Movies
		}
		*target = append(*target, rep)
		if k.tmdbID > 0 {
			repKey := strOfAny(rep["drive_folder_id"])
			for _, dup := range rowsFor {
				if strOfAny(dup["drive_folder_id"]) == repKey {
					continue
				}
				mergeDedupedEntry(*target, k.tmdbID, dup)
			}
		}
	}

	// 当页内存排序（与 SQL 排序键一致；页内数据量小，排序成本低）
	inMemorySort := func(items []map[string]any) {
		sort.SliceStable(items, func(i, j int) bool {
			a, b := items[i], items[j]
			less := false
			switch sortColumn {
			case "title":
				less = strings.ToLower(strOfAny(a["title"])) < strings.ToLower(strOfAny(b["title"]))
			case "year":
				less = strOfAny(a["year"]) < strOfAny(b["year"])
			case "folder_modified_time":
				less = jsonx.Int(a, "folder_modified_time", 0) < jsonx.Int(b, "folder_modified_time", 0)
			default:
				less = strOfAny(a["updated_at"]) < strOfAny(b["updated_at"])
			}
			if order == "DESC" {
				return !less && aValNotEqual(a, b, sortColumn)
			}
			return less
		})
	}
	inMemorySort(snap.Movies)
	inMemorySort(snap.TvShows)

	snap.TotalMovies = totalCount("movie")
	snap.TotalTV = totalCount("tv")
	return snap
}

// aValNotEqual 判断排序键是否不等（DESC 时排除相等元素被翻转）。
func aValNotEqual(a, b map[string]any, sortColumn string) bool {
	switch sortColumn {
	case "title":
		return strings.ToLower(strOfAny(a["title"])) != strings.ToLower(strOfAny(b["title"]))
	case "year":
		return strOfAny(a["year"]) != strOfAny(b["year"])
	case "folder_modified_time":
		return jsonx.Int(a, "folder_modified_time", 0) != jsonx.Int(b, "folder_modified_time", 0)
	}
	return strOfAny(a["updated_at"]) != strOfAny(b["updated_at"])
}

// GetAllFolderModifiedTimes 全部条目的文件夹修改时间。
func (s *LibraryStore) GetAllFolderModifiedTimes() map[string]int64 {
	rows, err := s.db.Query("SELECT drive_folder_id, folder_modified_time FROM library_media")
	if err != nil {
		return map[string]int64{}
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var id string
		var mtime sql.NullInt64
		if err := rows.Scan(&id, &mtime); err == nil {
			out[id] = mtime.Int64
		}
	}
	return out
}

// SaveDiff 全量保存结果。
type SaveDiff struct {
	NewMovies   int    `json:"new_movies"`
	NewTV       int    `json:"new_tv"`
	TotalMovies int    `json:"total_movies"`
	TotalTV     int    `json:"total_tv"`
	ScannedAt   string `json:"scanned_at"`
}

// SaveSnapshot 全量保存媒体库快照。
func (s *LibraryStore) SaveSnapshot(movies, tvShows []map[string]any) SaveDiff {
	s.mu.Lock()
	defer s.mu.Unlock()
	oldRows, err := s.db.Query("SELECT media_type, tmdb_id FROM library_media WHERE tmdb_id > 0")
	oldMovies := map[int64]bool{}
	oldTV := map[int64]bool{}
	if err == nil {
		defer oldRows.Close()
		for oldRows.Next() {
			var mediaType string
			var tmdbID int64
			if oldRows.Scan(&mediaType, &tmdbID) == nil {
				if mediaType == "movie" {
					oldMovies[tmdbID] = true
				} else {
					oldTV[tmdbID] = true
				}
			}
		}
		oldRows.Close()
	}

	now := utcNow()
	newMovies := map[int64]bool{}
	for _, m := range movies {
		if id := jsonx.Int(m, "tmdb_id", 0); id > 0 {
			newMovies[id] = true
		}
	}
	newTV := map[int64]bool{}
	for _, t := range tvShows {
		if id := jsonx.Int(t, "tmdb_id", 0); id > 0 {
			newTV[id] = true
		}
	}

	_, _ = s.db.Exec("DELETE FROM library_media")
	for _, payload := range append(append([]map[string]any{}, movies...), tvShows...) {
		mtime := jsonx.Int(payload, "folder_modified_time", 0)
		s.upsertLibraryItem(payload, now, mtime)
	}
	_ = s.setState(scanAtKey, now)

	newMovieCount := 0
	for id := range newMovies {
		if !oldMovies[id] {
			newMovieCount++
		}
	}
	newTVCount := 0
	for id := range newTV {
		if !oldTV[id] {
			newTVCount++
		}
	}
	diff := SaveDiff{
		NewMovies: newMovieCount, NewTV: newTVCount,
		TotalMovies: len(movies), TotalTV: len(tvShows), ScannedAt: now,
	}
	log.Printf("媒体库实体快照已保存：%d 部电影，%d 部剧集（新增 %d / %d）",
		diff.TotalMovies, diff.TotalTV, diff.NewMovies, diff.NewTV)
	return diff
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func (s *LibraryStore) upsertLibraryItem(payload map[string]any, scannedAt string, folderModifiedTime int64) {
	payload = s.normalizeLibraryPayload(payload)
	driveFolderID := jsonx.Str(payload, "drive_folder_id")
	if driveFolderID == "" {
		return
	}
	totalEps := nullableInt(payload["total_episodes"])
	inLibEps := nullableInt(payload["in_library_episodes"])
	_, err := s.db.Exec(`INSERT INTO library_media(
		drive_folder_id, media_type, tmdb_id, title, original_title, year,
		poster_url, backdrop_url, overview, rating, status,
		total_episodes, in_library_episodes, raw_json, in_library,
		folder_modified_time, parent_id, last_scanned_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?)
	ON CONFLICT(drive_folder_id) DO UPDATE SET
		media_type = excluded.media_type, tmdb_id = excluded.tmdb_id,
		title = excluded.title, original_title = excluded.original_title,
		year = excluded.year, poster_url = excluded.poster_url,
		backdrop_url = excluded.backdrop_url, overview = excluded.overview,
		rating = excluded.rating, status = excluded.status,
		total_episodes = excluded.total_episodes, in_library_episodes = excluded.in_library_episodes,
		raw_json = excluded.raw_json, in_library = excluded.in_library,
		folder_modified_time = excluded.folder_modified_time,
		parent_id = COALESCE(NULLIF(excluded.parent_id, ''), library_media.parent_id),
		last_scanned_at = excluded.last_scanned_at, updated_at = excluded.updated_at`,
		driveFolderID,
		jsonx.Str(payload, "media_type"),
		jsonx.Int(payload, "tmdb_id", 0),
		jsonx.Str(payload, "title"),
		jsonx.Str(payload, "original_title"),
		jsonx.Str(payload, "year"),
		nullableString(payload["poster_url"]),
		nullableString(payload["backdrop_url"]),
		jsonx.Str(payload, "overview"),
		jsonx.Float(payload, "rating", 0),
		jsonx.Str(payload, "status"),
		totalEps,
		inLibEps,
		marshalJSON(payload),
		folderModifiedTime,
		jsonx.Str(payload, "drive_parent_id"),
		scannedAt,
		scannedAt,
	)
	if err != nil {
		log.Printf("upsert library item 失败 [%s]: %v", driveFolderID, err)
	}
}

func nullableInt(v any) any {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int:
		return int64(n)
	case int64:
		return n
	case *int64:
		if n == nil {
			return nil
		}
		return *n
	case *int:
		if n == nil {
			return nil
		}
		return int64(*n)
	}
	return nil
}

func nullableString(v any) any {
	switch s := v.(type) {
	case string:
		if tmdb.ExtractImagePath(s) != "" {
			return tmdb.ExtractImagePath(s)
		}
		return s
	case *string:
		// 结构体字段直传 map 时是指针，不能静默丢成 NULL
		if s == nil {
			return nil
		}
		if tmdb.ExtractImagePath(*s) != "" {
			return tmdb.ExtractImagePath(*s)
		}
		return *s
	}
	return nil
}

// IncrementalDiff 增量保存结果。
type IncrementalDiff struct {
	NewItems     int    `json:"new_items"`
	UpdatedItems int    `json:"updated_items"`
	Total        int    `json:"total"`
	ScannedAt    string `json:"scanned_at"`
}

// SaveIncremental 增量保存媒体库条目。
func (s *LibraryStore) SaveIncremental(items []map[string]any, folderMtimes map[string]int64) IncrementalDiff {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := utcNow()
	newCount, updatedCount, total := 0, 0, 0
	for _, payload := range items {
		driveFolderID := jsonx.Str(payload, "drive_folder_id")
		if driveFolderID == "" {
			continue
		}
		total++
		var existing string
		err := s.db.QueryRow("SELECT drive_folder_id FROM library_media WHERE drive_folder_id = ?", driveFolderID).Scan(&existing)
		if err == nil {
			updatedCount++
		} else {
			newCount++
		}
		mtime := folderMtimes[driveFolderID]
		s.upsertLibraryItem(payload, now, mtime)
	}
	_ = s.setState(scanAtKey, now)
	log.Printf("媒体库增量更新完成：%d 项（新增 %d，更新 %d）", total, newCount, updatedCount)
	return IncrementalDiff{NewItems: newCount, UpdatedItems: updatedCount, Total: total, ScannedAt: now}
}

// MarkMissingFolders 标记不在库中的文件夹，返回更新行数。
func (s *LibraryStore) MarkMissingFolders(existingFolderIDs []string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var res sql.Result
	var err error
	if len(existingFolderIDs) == 0 {
		res, err = s.db.Exec("UPDATE library_media SET in_library = 0, updated_at = ? WHERE in_library = 1", utcNow())
	} else {
		placeholders := strings.Repeat("?,", len(existingFolderIDs))
		placeholders = placeholders[:len(placeholders)-1]
		args := make([]any, 0, len(existingFolderIDs)+1)
		args = append(args, utcNow())
		for _, id := range existingFolderIDs {
			args = append(args, id)
		}
		res, err = s.db.Exec(
			fmt.Sprintf("UPDATE library_media SET in_library = 0, updated_at = ? WHERE drive_folder_id NOT IN (%s) AND in_library = 1", placeholders),
			args...)
	}
	if err != nil {
		return 0
	}
	n, _ := res.RowsAffected()
	return n
}

// PatchItem 局部更新库条目。
func (s *LibraryStore) PatchItem(driveFolderID string, updates map[string]any) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	var rawJSON string
	err := s.db.QueryRow("SELECT raw_json FROM library_media WHERE drive_folder_id = ?", driveFolderID).Scan(&rawJSON)
	if err != nil {
		log.Printf("PatchItem: 未找到 drive_folder_id=%s", driveFolderID)
		return false
	}
	payload := decodeJSONObject(rawJSON)
	for k, v := range updates {
		payload[k] = v
	}
	scannedAt, _ := s.LastScanned()
	if scannedAt == "" {
		scannedAt = utcNow()
	}
	s.upsertLibraryItem(payload, scannedAt, jsonx.Int(payload, "folder_modified_time", 0))
	return true
}

// GetLibraryItemByFolderID 按文件夹 ID 取库条目。
func (s *LibraryStore) GetLibraryItemByFolderID(driveFolderID string) map[string]any {
	var rawJSON string
	err := s.db.QueryRow("SELECT raw_json FROM library_media WHERE drive_folder_id = ? LIMIT 1", driveFolderID).Scan(&rawJSON)
	if err != nil {
		return nil
	}
	return s.hydrateLibraryPayload(decodeJSONObject(rawJSON))
}

// FindLibraryItemByTitleYear 按标题+年份查找库条目。
func (s *LibraryStore) FindLibraryItemByTitleYear(mediaType, title, year string) map[string]any {
	if len(lookupTitleVariants(title)) == 0 {
		return nil
	}
	rows, err := s.db.Query(`SELECT raw_json FROM library_media
		WHERE media_type = ? AND tmdb_id > 0 AND year = ?
		ORDER BY in_library DESC, updated_at DESC, folder_modified_time DESC`, mediaType, year)
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var rawJSON string
		if rows.Scan(&rawJSON) != nil {
			continue
		}
		payload := decodeJSONObject(rawJSON)
		candidates := []string{jsonx.Str(payload, "title"), jsonx.Str(payload, "original_title")}
		for _, name := range jsonx.MapSlice(payload, "names") {
			candidates = append(candidates, jsonx.Str(name, "name"))
			if s, ok := name["name"].(string); ok {
				_ = s
			}
		}
		// names 在 raw_json 顶层为字符串数组
		if names, ok := payload["names"].([]any); ok {
			for _, name := range names {
				if s, ok := name.(string); ok {
					candidates = append(candidates, s)
				}
			}
		}
		var nonEmpty []string
		for _, c := range candidates {
			if c != "" {
				nonEmpty = append(nonEmpty, c)
			}
		}
		if TitlesMatch(title, nonEmpty) {
			return s.hydrateLibraryPayload(payload)
		}
	}
	return nil
}

// FindTmdbEntryByTitleYear 按标题+年份查找 TMDB 缓存条目。
func (s *LibraryStore) FindTmdbEntryByTitleYear(mediaType, title, year string) *TmdbMediaRow {
	if len(lookupTitleVariants(title)) == 0 {
		return nil
	}
	rows, err := s.db.Query(`SELECT * FROM tmdb_media WHERE media_type = ?
		ORDER BY CASE WHEN ? != '' AND substr(COALESCE(first_air_date, release_date, ''), 1, 4) = ? THEN 0 ELSE 1 END,
		last_synced_at DESC`, mediaType, year, year)
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var r TmdbMediaRow
		var rawJSON, seasonJSON, episodeJSON string
		if err := rows.Scan(&r.MediaType, &r.TmdbID, &r.Language, &r.Title, &r.OriginalTitle,
			&r.OriginalLanguage, &r.Overview, &r.PosterPath, &r.BackdropPath, &r.ReleaseDate,
			&r.FirstAirDate, &r.Status, &r.VoteAverage, &r.VoteCount,
			&rawJSON, &seasonJSON, &episodeJSON, &r.LastSyncedAt, &r.ExpiresAt); err != nil {
			continue
		}
		r.RawJSON = decodeJSONObject(rawJSON)
		r.SeasonDetailsJSON = decodeJSONObject(seasonJSON)
		r.EpisodeDetails = decodeJSONObject(episodeJSON)
		candidates := []string{r.Title, r.OriginalTitle,
			jsonx.Str(r.RawJSON, "title"), jsonx.Str(r.RawJSON, "name"),
			jsonx.Str(r.RawJSON, "original_title"), jsonx.Str(r.RawJSON, "original_name")}
		if names, ok := r.RawJSON["names"].([]any); ok {
			for _, name := range names {
				if s, ok := name.(string); ok {
					candidates = append(candidates, s)
				}
			}
		}
		var nonEmpty []string
		for _, c := range candidates {
			if c != "" {
				nonEmpty = append(nonEmpty, c)
			}
		}
		if TitlesMatch(title, nonEmpty) {
			return &r
		}
	}
	return nil
}

// DeleteLibraryItem 删除库条目。
func (s *LibraryStore) DeleteLibraryItem(driveFolderID string) bool {
	res, err := s.db.Exec("DELETE FROM library_media WHERE drive_folder_id = ?", driveFolderID)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

// GetLibraryItemByTmdb 按 TMDB ID 取库条目。
func (s *LibraryStore) GetLibraryItemByTmdb(mediaType string, tmdbID int64) map[string]any {
	var rawJSON string
	// 同 tmdb_id 可能存在多个目录条目：取最近更新的，保证与快照合并展示的卡片一致
	err := s.db.QueryRow(`SELECT raw_json FROM library_media
		WHERE media_type = ? AND tmdb_id = ?
		ORDER BY updated_at DESC, drive_folder_id ASC LIMIT 1`, mediaType, tmdbID).Scan(&rawJSON)
	if err != nil {
		return nil
	}
	return s.hydrateLibraryPayload(decodeJSONObject(rawJSON))
}

// ListLibraryItems 列出全部（或指定类型）库条目。
func (s *LibraryStore) ListLibraryItems(mediaType string) []map[string]any {
	var rows *sql.Rows
	var err error
	if mediaType != "" {
		rows, err = s.db.Query("SELECT raw_json FROM library_media WHERE media_type = ? ORDER BY year DESC, title COLLATE NOCASE ASC", mediaType)
	} else {
		rows, err = s.db.Query("SELECT raw_json FROM library_media ORDER BY media_type ASC, year DESC, title COLLATE NOCASE ASC")
	}
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var rawJSON string
		if rows.Scan(&rawJSON) == nil {
			out = append(out, s.hydrateLibraryPayload(decodeJSONObject(rawJSON)))
		}
	}
	return out
}

// GetJoinedMediaItem 合并库条目与 TMDB 缓存（库中无则回退 TMDB 缓存）。
func (s *LibraryStore) GetJoinedMediaItem(mediaType string, tmdbID int64) map[string]any {
	if item := s.GetLibraryItemByTmdb(mediaType, tmdbID); item != nil {
		item["in_library"] = true
		return item
	}
	entry := s.GetTmdbEntry(mediaType, tmdbID)
	if entry == nil {
		return nil
	}
	baseURL := ""
	if s.ImageBaseURL != nil {
		baseURL = s.ImageBaseURL()
	}
	raw := entry.RawJSON
	posterPath := firstNonEmpty(jsonx.Str(raw, "poster_path"), entry.PosterPath)
	backdropPath := firstNonEmpty(jsonx.Str(raw, "backdrop_path"), entry.BackdropPath)
	year := firstNonEmpty(jsonx.Str(raw, "release_date"), jsonx.Str(raw, "first_air_date"))
	if len(year) > 4 {
		year = year[:4]
	}
	payload := map[string]any{}
	for k, v := range raw {
		payload[k] = v
	}
	payload["tmdb_id"] = tmdbID
	payload["media_type"] = mediaType
	payload["title"] = firstNonEmpty(jsonx.Str(raw, "title"), jsonx.Str(raw, "name"), entry.Title)
	payload["original_title"] = firstNonEmpty(jsonx.Str(raw, "original_title"), jsonx.Str(raw, "original_name"), entry.OriginalTitle)
	payload["year"] = year
	payload["poster_url"] = tmdb.ImageURL(posterPath, "w500", baseURL)
	payload["backdrop_url"] = tmdb.ImageURL(backdropPath, "", baseURL)
	payload["overview"] = firstNonEmpty(jsonx.Str(raw, "overview"), entry.Overview)
	payload["rating"] = jsonx.Float(raw, "vote_average", entry.VoteAverage)
	payload["status"] = firstNonEmpty(jsonx.Str(raw, "status"), entry.Status)
	payload["in_library"] = false
	return payload
}

// GetStats 媒体库统计。
func (s *LibraryStore) GetStats() map[string]any {
	snap := s.GetSnapshot("folder_modified_time", "desc")
	if snap == nil {
		return map[string]any{
			"total_movies": 0, "total_tv_shows": 0,
			"total_episodes_in_library": 0, "total_episodes_on_tmdb": 0,
			"completion_rate": 0.0,
		}
	}
	var totalEpsTmdb, totalEpsLib int64
	for _, show := range snap.TvShows {
		totalEpsTmdb += jsonx.Int(show, "total_episodes", 0)
		totalEpsLib += jsonx.Int(show, "in_library_episodes", 0)
	}
	rate := 0.0
	if totalEpsTmdb > 0 {
		rate = float64(int64(float64(totalEpsLib)/float64(totalEpsTmdb)*1000+0.5)) / 10
	}
	return map[string]any{
		"total_movies":              snap.TotalMovies,
		"total_tv_shows":            snap.TotalTV,
		"total_episodes_in_library": totalEpsLib,
		"total_episodes_on_tmdb":    totalEpsTmdb,
		"completion_rate":           rate,
	}
}

// GetTmdbEntriesByKeys 批量读取 TMDB 实体缓存，key 形如 "tv:123"。
// 按主键 (media_type, tmdb_id) 行值 IN 分块查询，替代逐条查询（消 N+1）。
func (s *LibraryStore) GetTmdbEntriesByKeys(keys []string) map[string]*TmdbMediaRow {
	out := map[string]*TmdbMediaRow{}
	const chunk = 300
	for start := 0; start < len(keys); start += chunk {
		end := start + chunk
		if end > len(keys) {
			end = len(keys)
		}
		placeholders := strings.TrimSuffix(strings.Repeat("(?,?),", end-start), ",")
		args := []any{}
		for _, key := range keys[start:end] {
			mt, id, ok := strings.Cut(key, ":")
			if !ok {
				continue
			}
			args = append(args, mt, id)
		}
		if len(args) == 0 {
			continue
		}
		rows, err := s.db.Query(`SELECT media_type, tmdb_id, language, title, original_title, original_language,
			overview, poster_path, backdrop_path, release_date, first_air_date, status, vote_average, vote_count,
			raw_json, season_details_json, episode_details_json, last_synced_at, expires_at
			FROM tmdb_media WHERE (media_type, tmdb_id) IN (`+placeholders+`)`, args...)
		if err != nil {
			continue
		}
		for rows.Next() {
			var r TmdbMediaRow
			var rawJSON, seasonJSON, episodeJSON string
			if err := rows.Scan(&r.MediaType, &r.TmdbID, &r.Language, &r.Title, &r.OriginalTitle, &r.OriginalLanguage,
				&r.Overview, &r.PosterPath, &r.BackdropPath, &r.ReleaseDate, &r.FirstAirDate, &r.Status,
				&r.VoteAverage, &r.VoteCount, &rawJSON, &seasonJSON, &episodeJSON, &r.LastSyncedAt, &r.ExpiresAt); err != nil {
				continue
			}
			r.RawJSON = decodeJSONObject(rawJSON)
			r.SeasonDetailsJSON = decodeJSONObject(seasonJSON)
			r.EpisodeDetails = decodeJSONObject(episodeJSON)
			out[r.MediaType+":"+strconv.FormatInt(r.TmdbID, 10)] = &r
		}
		rows.Close()
	}
	return out
}

// ListFolderIDsGroupedByTmdb 全库在库条目的目录 ID 按 "media_type:tmdb_id" 分组（单条扫描）。
func (s *LibraryStore) ListFolderIDsGroupedByTmdb() map[string][]string {
	rows, err := s.db.Query(`SELECT media_type, tmdb_id, drive_folder_id
		FROM library_media WHERE tmdb_id > 0 AND in_library = 1`)
	if err != nil {
		return map[string][]string{}
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var mt, folderID string
		var tmdbID int64
		if err := rows.Scan(&mt, &tmdbID, &folderID); err == nil {
			key := mt + ":" + strconv.FormatInt(tmdbID, 10)
			out[key] = append(out[key], folderID)
		}
	}
	return out
}

// LibraryRef 可分类库条目的轻量引用（重归类用）。
type LibraryRef struct {
	FolderID  string
	ParentID  string // 当前父目录 ID（网盘侧实际位置的本地记录，随扫描/移动维护）
	MediaType string
	TmdbID    int64
	Title     string
}

// ListClassifiableItems 在库且已关联 TMDB 的条目（轻量列，无 JSON 解码）。
func (s *LibraryStore) ListClassifiableItems() []LibraryRef {
	rows, err := s.db.Query(`SELECT drive_folder_id, parent_id, media_type, tmdb_id, title
		FROM library_media WHERE in_library = 1 AND tmdb_id > 0
		ORDER BY media_type ASC, title COLLATE NOCASE ASC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []LibraryRef{}
	for rows.Next() {
		var r LibraryRef
		if err := rows.Scan(&r.FolderID, &r.ParentID, &r.MediaType, &r.TmdbID, &r.Title); err == nil {
			out = append(out, r)
		}
	}
	return out
}

// UpdateMediaParentID 更新条目的父目录记录（重新归类/修正识别移动成功后调用）。
func (s *LibraryStore) UpdateMediaParentID(driveFolderID, parentID string) {
	if driveFolderID == "" {
		return
	}
	if _, err := s.db.Exec("UPDATE library_media SET parent_id = ?, updated_at = ? WHERE drive_folder_id = ?",
		parentID, utcNow(), driveFolderID); err != nil {
		log.Printf("更新 parent_id 失败 [%s]: %v", driveFolderID, err)
	}
}

// GetMediaRefByFolderID 返回目录对应库条目的媒体类型、TMDB ID 与父目录记录。
func (s *LibraryStore) GetMediaRefByFolderID(folderID string) (mediaType string, tmdbID int64, parentID string, ok bool) {
	err := s.db.QueryRow("SELECT media_type, tmdb_id, parent_id FROM library_media WHERE drive_folder_id = ?",
		folderID).Scan(&mediaType, &tmdbID, &parentID)
	return mediaType, tmdbID, parentID, err == nil
}

// ListFolderIDsByTmdb 返回关联到同一 (mediaType, tmdbID) 的全部目录 ID。
// 同一部剧存在多个目录（历史译名差异）时用于集数聚合。
func (s *LibraryStore) ListFolderIDsByTmdb(mediaType string, tmdbID int64) []string {
	rows, err := s.db.Query(
		"SELECT drive_folder_id FROM library_media WHERE media_type = ? AND tmdb_id = ?",
		mediaType, tmdbID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			out = append(out, id)
		}
	}
	return out
}

// mergeDedupedEntry 将同 tmdb_id 的重复条目并入已展示的那条：
// 集数/时间戳取最大值，保留首个（最近更新）条目的标题与目录 ID。
func strOfAny(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func mergeDedupedEntry(items []map[string]any, tmdbID int64, dup map[string]any) {
	for _, item := range items {
		if int64(jsonx.Int(item, "tmdb_id", 0)) != tmdbID {
			continue
		}
		for _, field := range []string{"in_library_episodes", "total_episodes", "folder_modified_time"} {
			if jsonx.Int(dup, field, 0) > jsonx.Int(item, field, 0) {
				item[field] = dup[field]
			}
		}
		if s, _ := dup["updated_at"].(string); s > strOfAny(item["updated_at"]) {
			item["updated_at"] = s
		}
		return
	}
}

// GetTmdbIDByFolderID 读取库条目关联的 TMDB ID（无条目或未关联返回 false）。
func (s *LibraryStore) GetTmdbIDByFolderID(driveFolderID string) (int64, bool) {
	var tmdbID int64
	err := s.db.QueryRow(
		"SELECT tmdb_id FROM library_media WHERE drive_folder_id = ?", driveFolderID).Scan(&tmdbID)
	if err != nil || tmdbID <= 0 {
		return 0, false
	}
	return tmdbID, true
}
