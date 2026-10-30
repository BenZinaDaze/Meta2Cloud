// ingest.go —— 入库历史记录存储（对应 webui/ingest_store.py）。
// 与 library.db 同库（ingest_history 表）。
package store

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"panoply/internal/titlematch"
)

const ingestDDL = `
CREATE TABLE IF NOT EXISTS ingest_history (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    media_type      TEXT    NOT NULL,
    tmdb_id         INTEGER NOT NULL DEFAULT 0,
    title           TEXT    NOT NULL,
    original_title  TEXT    NOT NULL DEFAULT '',
    year            TEXT    NOT NULL DEFAULT '',
    season          INTEGER DEFAULT NULL,
    episode         INTEGER DEFAULT NULL,
    episode_title   TEXT    NOT NULL DEFAULT '',
    poster_path     TEXT    NOT NULL DEFAULT '',
    drive_folder_id TEXT    NOT NULL DEFAULT '',
    original_name   TEXT    NOT NULL DEFAULT '',
    status          TEXT    NOT NULL DEFAULT 'success',
    error_message   TEXT    NOT NULL DEFAULT '',
    ingested_at     TEXT    NOT NULL,
    created_at      TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_ingest_history_media_type
ON ingest_history(media_type);

CREATE INDEX IF NOT EXISTS idx_ingest_history_tmdb_id
ON ingest_history(media_type, tmdb_id);

CREATE INDEX IF NOT EXISTS idx_ingest_history_ingested_at
ON ingest_history(ingested_at DESC);
`

// IngestRecord 入库历史条目（与 pipeline.IngestRecord 字段对齐）。
type IngestRecord struct {
	MediaType     string
	TmdbID        int64
	Title         string
	OriginalTitle string
	Year          string
	Season        *int
	Episode       *int
	EpisodeTitle  string
	PosterPath    string
	DriveFolderID string
	OriginalName  string
	Status        string
	ErrorMessage  string
	IngestedAt    string // 为空取当前时间
}

// IngestStore 入库历史存储。
type IngestStore struct {
	db *sql.DB
}

func sqliteDir(dbPath string) string {
	if i := strings.LastIndexByte(dbPath, '/'); i > 0 {
		return dbPath[:i]
	}
	return ""
}

// OpenIngestStore 在指定 SQLite 库上打开（通常是 library.db）。
func OpenIngestStore(dbPath string) (*IngestStore, error) {
	if dir := sqliteDir(dbPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(ingestDDL); err != nil {
		db.Close()
		return nil, err
	}
	log.Printf("入库历史存储启动：%s", dbPath)
	return &IngestStore{db: db}, nil
}

// Close 关闭。
func (s *IngestStore) Close() error { return s.db.Close() }

// RecordIngest 写入一条入库记录，返回行 ID。
func (s *IngestStore) RecordIngest(r IngestRecord) (int64, error) {
	now := utcNow()
	ingestedAt := r.IngestedAt
	if ingestedAt == "" {
		ingestedAt = now
	}
	res, err := s.db.Exec(`INSERT INTO ingest_history(
		media_type, tmdb_id, title, original_title, year,
		season, episode, episode_title, poster_path,
		drive_folder_id, original_name, status, error_message,
		ingested_at, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.MediaType, r.TmdbID, r.Title, r.OriginalTitle, r.Year,
		r.Season, r.Episode, r.EpisodeTitle, r.PosterPath,
		r.DriveFolderID, r.OriginalName, r.Status, r.ErrorMessage,
		ingestedAt, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// IngestQuery 查询条件。
type IngestQuery struct {
	Page      int
	PageSize  int
	MediaType string
	TmdbID    int64
	Status    string
	StartDate string
	EndDate   string
	Keyword   string
}

// IngestRecordRow 入库历史行（GET /api/ingest/history 与 /api/ingest/recent 的条目形状；
// PosterURL 由 webui 层按 PosterPath 拼接，无图不出键）。
type IngestRecordRow struct {
	ID            int64  `json:"id"`
	MediaType     string `json:"media_type"`
	TmdbID        int64  `json:"tmdb_id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	Year          string `json:"year"`
	Season        *int64 `json:"season"`
	Episode       *int64 `json:"episode"`
	EpisodeTitle  string `json:"episode_title"`
	PosterPath    string `json:"poster_path"`
	DriveFolderID string `json:"drive_folder_id"`
	OriginalName  string `json:"original_name"`
	Status        string `json:"status"`
	ErrorMessage  string `json:"error_message"`
	IngestedAt    string `json:"ingested_at"`
	CreatedAt     string `json:"created_at"`
	PosterURL     string `json:"poster_url,omitempty"`
}

// IngestQueryResult 查询结果。
type IngestQueryResult struct {
	Items      []IngestRecordRow `json:"items"`
	Pagination map[string]any    `json:"pagination"`
}

func scanIngestRow(row interface{ Scan(...any) error }) *IngestRecordRow {
	var id int64
	var mediaType, title, originalTitle, year, episodeTitle, posterPath string
	var driveFolderID, originalName, status, errorMessage, ingestedAt, createdAt string
	var tmdbID int64
	var season, episode sql.NullInt64
	if err := row.Scan(&id, &mediaType, &tmdbID, &title, &originalTitle, &year,
		&season, &episode, &episodeTitle, &posterPath, &driveFolderID,
		&originalName, &status, &errorMessage, &ingestedAt, &createdAt); err != nil {
		return nil
	}
	item := IngestRecordRow{
		ID: id, MediaType: mediaType, TmdbID: tmdbID,
		Title: title, OriginalTitle: originalTitle, Year: year,
		EpisodeTitle: episodeTitle, PosterPath: posterPath,
		DriveFolderID: driveFolderID, OriginalName: originalName,
		Status: status, ErrorMessage: errorMessage,
		IngestedAt: ingestedAt, CreatedAt: createdAt,
	}
	if season.Valid {
		item.Season = &season.Int64
	}
	if episode.Valid {
		item.Episode = &episode.Int64
	}
	return &item
}

const ingestSelect = `SELECT id, media_type, tmdb_id, title, original_title, year,
	season, episode, episode_title, poster_path, drive_folder_id, original_name,
	status, error_message, ingested_at, created_at FROM ingest_history`

// Query 分页查询。
func (s *IngestStore) Query(q IngestQuery) IngestQueryResult {
	page := q.Page
	if page < 1 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var conditions []string
	var params []any
	if q.MediaType != "" {
		conditions = append(conditions, "media_type = ?")
		params = append(params, q.MediaType)
	}
	if q.TmdbID > 0 {
		conditions = append(conditions, "tmdb_id = ?")
		params = append(params, q.TmdbID)
	}
	if q.Status != "" {
		conditions = append(conditions, "status = ?")
		params = append(params, q.Status)
	}
	if q.StartDate != "" {
		conditions = append(conditions, "ingested_at >= ?")
		params = append(params, q.StartDate)
	}
	if q.EndDate != "" {
		conditions = append(conditions, "ingested_at <= ?")
		params = append(params, q.EndDate)
	}
	if q.Keyword != "" {
		// 转义 LIKE 通配符（与 library 的 escapeLike 同一套语义），防 %/_ 注入全表模糊匹配
		like := "%" + escapeLike(q.Keyword) + "%"
		conditions = append(conditions, "(title LIKE ? ESCAPE '\\' OR original_title LIKE ? ESCAPE '\\')")
		params = append(params, like, like)
	}
	where := "1=1"
	if len(conditions) > 0 {
		where = strings.Join(conditions, " AND ")
	}

	var total int64
	_ = s.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM ingest_history WHERE %s", where), params...).Scan(&total)

	args := append(append([]any{}, params...), pageSize, offset)
	rows, err := s.db.Query(
		fmt.Sprintf("%s WHERE %s ORDER BY ingested_at DESC LIMIT ? OFFSET ?", ingestSelect, where),
		args...)
	if err != nil {
		return IngestQueryResult{Items: []IngestRecordRow{}, Pagination: map[string]any{
			"page": page, "page_size": pageSize, "total": 0, "total_pages": 1,
		}}
	}
	defer rows.Close()
	items := []IngestRecordRow{}
	for rows.Next() {
		if item := scanIngestRow(rows); item != nil {
			items = append(items, *item)
		}
	}
	totalPages := 1
	if total > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return IngestQueryResult{Items: items, Pagination: map[string]any{
		"page": page, "page_size": pageSize, "total": total, "total_pages": totalPages,
	}}
}

// GetRecent 最近 N 条。
func (s *IngestStore) GetRecent(limit int) []IngestRecordRow {
	rows, err := s.db.Query(fmt.Sprintf("%s ORDER BY ingested_at DESC LIMIT ?", ingestSelect), limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []IngestRecordRow{}
	for rows.Next() {
		if item := scanIngestRow(rows); item != nil {
			out = append(out, *item)
		}
	}
	return out
}

// ListByFolder 取该媒体目录的全部入库记录（修正识别改写用，按时间升序）。
// 仅成功记录带 drive_folder_id（recordIngest 落库语义），失败/无 TMDB 行不会命中。
func (s *IngestStore) ListByFolder(mediaType, driveFolderID string) []IngestRecordRow {
	rows, err := s.db.Query(
		fmt.Sprintf("%s WHERE media_type = ? AND drive_folder_id = ? ORDER BY ingested_at ASC, id ASC", ingestSelect),
		mediaType, driveFolderID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []IngestRecordRow{}
	for rows.Next() {
		if item := scanIngestRow(rows); item != nil {
			out = append(out, *item)
		}
	}
	return out
}

// UpdateIdentity 改写一条入库记录的识别身份（修正识别后调用）。
// 入库时间与原始文件名是历史事实，保留不动。
func (s *IngestStore) UpdateIdentity(id int64, tmdbID int64, title, originalTitle, year, posterPath, episodeTitle string) error {
	_, err := s.db.Exec(`UPDATE ingest_history
		SET tmdb_id = ?, title = ?, original_title = ?, year = ?, poster_path = ?, episode_title = ?
		WHERE id = ?`,
		tmdbID, title, originalTitle, year, posterPath, episodeTitle, id)
	return err
}

// FindMatchByTitleYear 按标题+年份查找成功记录。
func (s *IngestStore) FindMatchByTitleYear(mediaType, title, year string) *IngestRecordRow {
	if len(titlematch.LookupTitleVariants(title)) == 0 {
		return nil
	}
	rows, err := s.db.Query(fmt.Sprintf(`%s
		WHERE media_type = ? AND tmdb_id > 0 AND status = 'success' AND year = ?
		ORDER BY ingested_at DESC, id DESC`, ingestSelect), mediaType, year)
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		item := scanIngestRow(rows)
		if item == nil {
			continue
		}
		candidates := []string{item.Title, item.OriginalTitle}
		var nonEmpty []string
		for _, c := range candidates {
			if c != "" {
				nonEmpty = append(nonEmpty, c)
			}
		}
		if titlematch.TitlesMatch(title, nonEmpty) {
			return item
		}
	}
	return nil
}

func strOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// Stats 近 N 天统计。
// IngestStats 入库统计（GET /api/ingest/stats 响应）。
type IngestStats struct {
	Days       int   `json:"days"`
	Total      int64 `json:"total"`
	Movies     int64 `json:"movies"`
	TVEpisodes int64 `json:"tv_episodes"`
	Success    int64 `json:"success"`
	Failed     int64 `json:"failed"`
	NoTmdb     int64 `json:"no_tmdb"`
	Deleted    int64 `json:"deleted"`
}

func (s *IngestStore) Stats(days int) IngestStats {
	if days <= 0 {
		days = 7
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02T15:04:05")
	var total, movies, tvEpisodes, success, failed, noTmdb, deleted sql.NullInt64
	err := s.db.QueryRow(`SELECT
		COUNT(*),
		COUNT(CASE WHEN media_type = 'movie' THEN 1 END),
		COUNT(CASE WHEN media_type = 'tv' THEN 1 END),
		COUNT(CASE WHEN status = 'success' THEN 1 END),
		COUNT(CASE WHEN status = 'failed' THEN 1 END),
		COUNT(CASE WHEN status = 'no_tmdb' THEN 1 END),
		COUNT(CASE WHEN status = 'deleted' THEN 1 END)
		FROM ingest_history WHERE ingested_at >= ?`, cutoff).Scan(
		&total, &movies, &tvEpisodes, &success, &failed, &noTmdb, &deleted)
	if err != nil {
		return IngestStats{Days: days}
	}
	return IngestStats{
		Days: days, Total: total.Int64, Movies: movies.Int64,
		TVEpisodes: tvEpisodes.Int64, Success: success.Int64,
		Failed: failed.Int64, NoTmdb: noTmdb.Int64,
		Deleted: deleted.Int64,
	}
}

// DailyStats 近 N 天按天聚合入库量（UTC 日期；只返回有记录的日期，空缺天由前端补零）。
// DayStat 按天聚合入库量（GET /api/ingest/stats/daily 元素；只返回有记录的日期，空缺天由前端补零）。
type DayStat struct {
	Date   string `json:"date"`
	Total  int64  `json:"total"`
	Movies int64  `json:"movies"`
}

func (s *IngestStore) DailyStats(days int) []DayStat {
	if days <= 0 {
		days = 30
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	rows, err := s.db.Query(`SELECT substr(ingested_at, 1, 10) AS day,
		COUNT(*),
		COUNT(CASE WHEN media_type = 'movie' THEN 1 END)
		FROM ingest_history WHERE substr(ingested_at, 1, 10) >= ?
		GROUP BY day ORDER BY day`, cutoff)
	if err != nil {
		log.Printf("查询入库日统计失败: %v", err)
		return nil
	}
	defer rows.Close()
	out := []DayStat{}
	for rows.Next() {
		var day string
		var total, movies sql.NullInt64
		if err := rows.Scan(&day, &total, &movies); err != nil {
			continue
		}
		out = append(out, DayStat{Date: day, Total: total.Int64, Movies: movies.Int64})
	}
	return out
}
