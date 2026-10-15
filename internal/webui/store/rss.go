// rss.go —— RSS 订阅存储（对应 webui/rss_subscription_store.py）。
//
// 生产环境订阅与媒体库同库（library.db）；mediaDBPath 不同时通过 ATTACH 跨库 JOIN。
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"meta2cloud/internal/tmdb"
)

const rssDDL = `
CREATE TABLE IF NOT EXISTS rss_subscriptions (
	id                INTEGER PRIMARY KEY AUTOINCREMENT,
	name              TEXT NOT NULL,
	media_title       TEXT NOT NULL,
	media_type        TEXT NOT NULL DEFAULT 'tv',
	tmdb_id           INTEGER,
	poster_url        TEXT,
	site              TEXT NOT NULL,
	rss_url           TEXT NOT NULL,
	subgroup_name     TEXT NOT NULL DEFAULT '',
	season_number     INTEGER NOT NULL DEFAULT 1,
	start_episode     INTEGER NOT NULL DEFAULT 1,
	keyword_all       TEXT NOT NULL DEFAULT '[]',
	push_target       TEXT NOT NULL,
	enabled           INTEGER NOT NULL DEFAULT 1,
	last_checked_at   TEXT,
	last_matched_at   TEXT,
	last_error        TEXT,
	created_at        TEXT NOT NULL,
	updated_at        TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS rss_subscription_hits (
	id                INTEGER PRIMARY KEY AUTOINCREMENT,
	subscription_id   INTEGER NOT NULL,
	episode_title     TEXT NOT NULL,
	season_number     INTEGER,
	episode_number    INTEGER,
	torrent_url       TEXT,
	magnet_url        TEXT,
	published_at      TEXT,
	dedupe_key        TEXT NOT NULL,
	pushed_to         TEXT NOT NULL,
	push_status       TEXT NOT NULL,
	push_error        TEXT,
	created_at        TEXT NOT NULL,
	FOREIGN KEY(subscription_id) REFERENCES rss_subscriptions(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_rss_subscription_hits_dedupe
ON rss_subscription_hits(subscription_id, dedupe_key);

CREATE UNIQUE INDEX IF NOT EXISTS idx_rss_subscription_unique
ON rss_subscriptions(
	media_type, tmdb_id, site, rss_url, subgroup_name,
	season_number, start_episode, push_target
);
`

// ErrDuplicateSubscription 重复订阅（对应 sqlite3.IntegrityError("duplicate rss subscription")）。
var ErrDuplicateSubscription = errors.New("duplicate rss subscription")

// SubscriptionRecord 订阅条目。
type SubscriptionRecord struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	MediaTitle    string  `json:"media_title"`
	MediaType     string  `json:"media_type"`
	TmdbID        *int64  `json:"tmdb_id"`
	PosterURL     *string `json:"poster_url"`
	Site          string  `json:"site"`
	RssURL        string  `json:"rss_url"`
	SubgroupName  string  `json:"subgroup_name"`
	SeasonNumber  int     `json:"season_number"`
	StartEpisode  int     `json:"start_episode"`
	KeywordAll    string  `json:"keyword_all"`
	PushTarget    string  `json:"push_target"`
	Enabled       bool    `json:"enabled"`
	LastCheckedAt *string `json:"last_checked_at"`
	LastMatchedAt *string `json:"last_matched_at"`
	LastError     *string `json:"last_error"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

// HitRecord 订阅命中条目。
type HitRecord struct {
	ID             int64   `json:"id"`
	SubscriptionID int64   `json:"subscription_id"`
	EpisodeTitle   string  `json:"episode_title"`
	SeasonNumber   *int    `json:"season_number"`
	EpisodeNumber  *int    `json:"episode_number"`
	TorrentURL     *string `json:"torrent_url"`
	MagnetURL      *string `json:"magnet_url"`
	PublishedAt    *string `json:"published_at"`
	DedupeKey      string  `json:"dedupe_key"`
	PushedTo       string  `json:"pushed_to"`
	PushStatus     string  `json:"push_status"`
	PushError      *string `json:"push_error"`
	CreatedAt      string  `json:"created_at"`
}

// RSSStore RSS 订阅存储。
type RSSStore struct {
	db            *sql.DB
	mu            sync.Mutex
	libraryDBPath string
	sameDB        bool
	mediaAttached bool

	// ImageBaseURL 注入图片根地址（joined 序列化用）。
	ImageBaseURL func() string
}

// OpenRSSStore 打开订阅库。libraryDBPath 与 dbPath 相同时共享库。
func OpenRSSStore(dbPath, libraryDBPath string) (*RSSStore, error) {
	if libraryDBPath == "" {
		libraryDBPath = dbPath
	}
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
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(rssDDL); err != nil {
		db.Close()
		return nil, err
	}
	s := &RSSStore{
		db:            db,
		libraryDBPath: libraryDBPath,
		sameDB:        absEqual(dbPath, libraryDBPath),
		ImageBaseURL:  func() string { return "" },
	}
	// 同库模式下 JOIN 依赖媒体表；library DDL 全为 IF NOT EXISTS，幂等补建
	if s.sameDB {
		if _, err := db.Exec(libraryDDL); err != nil {
			db.Close()
			return nil, err
		}
	}
	if err := s.migrateSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func absEqual(a, b string) bool {
	absA, _ := filepath.Abs(a)
	absB, _ := filepath.Abs(b)
	return absA == absB
}

// Close 关闭。
func (s *RSSStore) Close() error { return s.db.Close() }

func (s *RSSStore) migrateSchema() error {
	rows, err := s.db.Query("PRAGMA table_info(rss_subscriptions)")
	if err != nil {
		return err
	}
	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		columns[name] = true
	}
	rows.Close()
	if !columns["poster_url"] {
		if _, err := s.db.Exec("ALTER TABLE rss_subscriptions ADD COLUMN poster_url TEXT"); err != nil {
			return err
		}
	}
	return nil
}

const subscriptionSelect = `SELECT id, name, media_title, media_type, tmdb_id, poster_url, site, rss_url,
	subgroup_name, season_number, start_episode, keyword_all, push_target, enabled,
	last_checked_at, last_matched_at, last_error, created_at, updated_at
	FROM rss_subscriptions`

func scanSubscription(row interface{ Scan(...any) error }) *SubscriptionRecord {
	var r SubscriptionRecord
	var enabled int
	var posterURL sql.NullString
	if err := row.Scan(&r.ID, &r.Name, &r.MediaTitle, &r.MediaType, &r.TmdbID, &posterURL,
		&r.Site, &r.RssURL, &r.SubgroupName, &r.SeasonNumber, &r.StartEpisode,
		&r.KeywordAll, &r.PushTarget, &enabled,
		&r.LastCheckedAt, &r.LastMatchedAt, &r.LastError, &r.CreatedAt, &r.UpdatedAt); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("读取订阅行失败: %v", err)
		}
		return nil
	}
	r.Enabled = enabled != 0
	if posterURL.Valid && posterURL.String != "" {
		p := tmdb.ExtractImagePath(posterURL.String)
		r.PosterURL = &p
	}
	return &r
}

const hitSelect = `SELECT id, subscription_id, episode_title, season_number, episode_number,
	torrent_url, magnet_url, published_at, dedupe_key, pushed_to, push_status, push_error, created_at
	FROM rss_subscription_hits`

func scanHit(row interface{ Scan(...any) error }) *HitRecord {
	var r HitRecord
	if err := row.Scan(&r.ID, &r.SubscriptionID, &r.EpisodeTitle, &r.SeasonNumber, &r.EpisodeNumber,
		&r.TorrentURL, &r.MagnetURL, &r.PublishedAt, &r.DedupeKey, &r.PushedTo,
		&r.PushStatus, &r.PushError, &r.CreatedAt); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("读取订阅命中行失败: %v", err)
		}
		return nil
	}
	return &r
}

func (s *RSSStore) ensureMediaDBAttached() bool {
	if s.sameDB {
		return true
	}
	if s.mediaAttached {
		return true
	}
	if s.libraryDBPath == "" {
		return false
	}
	if _, err := os.Stat(s.libraryDBPath); err != nil {
		return false
	}
	if _, err := s.db.Exec("ATTACH DATABASE ? AS media_db", s.libraryDBPath); err != nil {
		return false
	}
	s.mediaAttached = true
	return true
}

func (s *RSSStore) subscriptionJoinSelect() string {
	mediaSchema := "media_db"
	if s.sameDB {
		mediaSchema = "main"
	}
	return fmt.Sprintf(`
		SELECT
			s.id, s.name, s.media_title, s.media_type, s.tmdb_id, s.poster_url, s.site, s.rss_url,
			s.subgroup_name, s.season_number, s.start_episode, s.keyword_all, s.push_target, s.enabled,
			s.last_checked_at, s.last_matched_at, s.last_error, s.created_at, s.updated_at,
			COALESCE(hit_stats.hit_count, 0) AS hit_count,
			tm.title AS tm_title, tm.original_title AS tm_original_title, tm.overview AS tm_overview,
			tm.poster_path AS tm_poster_path, tm.backdrop_path AS tm_backdrop_path,
			tm.release_date AS tm_release_date, tm.first_air_date AS tm_first_air_date,
			tm.status AS tm_status, tm.vote_average AS tm_vote_average,
			lib.drive_folder_id AS lib_drive_folder_id, lib.title AS lib_title, lib.year AS lib_year,
			lib.poster_url AS lib_poster_url, lib.total_episodes AS lib_total_episodes,
			lib.in_library_episodes AS lib_in_library_episodes
		FROM rss_subscriptions AS s
		LEFT JOIN (
			SELECT subscription_id, COUNT(*) AS hit_count
			FROM rss_subscription_hits GROUP BY subscription_id
		) AS hit_stats ON hit_stats.subscription_id = s.id
		LEFT JOIN %[1]s.tmdb_media AS tm
			ON tm.media_type = s.media_type AND tm.tmdb_id = s.tmdb_id
		LEFT JOIN %[1]s.library_media AS lib
			ON lib.drive_folder_id = (
				SELECT lm.drive_folder_id FROM %[1]s.library_media AS lm
				WHERE lm.media_type = s.media_type AND lm.tmdb_id = s.tmdb_id
				ORDER BY lm.in_library DESC, lm.updated_at DESC, lm.drive_folder_id ASC
				LIMIT 1
			)`, mediaSchema)
}

// serializeJoined 把 JOIN 行序列化为前端所需的订阅详情（含 TMDB/库状态）。
func (s *RSSStore) serializeJoined(row interface{ Scan(...any) error }) map[string]any {
	baseURL := ""
	if s.ImageBaseURL != nil {
		baseURL = s.ImageBaseURL()
	}
	var r SubscriptionRecord
	var enabled int
	var posterURL sql.NullString
	var hitCount int64
	var tmTitle, tmOriginalTitle, tmOverview, tmPoster, tmBackdrop, tmRelease, tmFirstAir, tmStatus sql.NullString
	var tmVote sql.NullFloat64
	var libFolder, libTitle, libYear, libPoster sql.NullString
	var libTotal, libInLib sql.NullInt64
	if err := row.Scan(&r.ID, &r.Name, &r.MediaTitle, &r.MediaType, &r.TmdbID, &posterURL,
		&r.Site, &r.RssURL, &r.SubgroupName, &r.SeasonNumber, &r.StartEpisode,
		&r.KeywordAll, &r.PushTarget, &enabled,
		&r.LastCheckedAt, &r.LastMatchedAt, &r.LastError, &r.CreatedAt, &r.UpdatedAt,
		&hitCount,
		&tmTitle, &tmOriginalTitle, &tmOverview, &tmPoster, &tmBackdrop,
		&tmRelease, &tmFirstAir, &tmStatus, &tmVote,
		&libFolder, &libTitle, &libYear, &libPoster, &libTotal, &libInLib); err != nil {
		// 扫描失败会让整条订阅从列表静默消失，必须留痕
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("读取订阅（含 TMDB/库状态）失败: %v", err)
		}
		return nil
	}

	var keywords any = []any{}
	if r.KeywordAll != "" {
		var parsed []any
		if json.Unmarshal([]byte(r.KeywordAll), &parsed) == nil {
			keywords = parsed
		}
	}
	posterPath := ""
	if posterURL.Valid {
		posterPath = tmdb.ExtractImagePath(posterURL.String)
	}
	payload := map[string]any{
		"id": r.ID, "name": r.Name, "media_title": r.MediaTitle, "media_type": r.MediaType,
		"tmdb_id":    r.TmdbID,
		"poster_url": tmdb.ImageURL(posterPath, "w500", baseURL),
		"site":       r.Site, "rss_url": r.RssURL, "subgroup_name": r.SubgroupName,
		"season_number": r.SeasonNumber, "start_episode": r.StartEpisode,
		"keyword_all": keywords, "push_target": r.PushTarget,
		"enabled":         enabled != 0,
		"last_checked_at": r.LastCheckedAt, "last_matched_at": r.LastMatchedAt,
		"last_error": r.LastError, "created_at": r.CreatedAt, "updated_at": r.UpdatedAt,
		"hit_count": hitCount,
		"tmdb":      nil,
		"library":   nil,
	}
	if tmTitle.Valid {
		posterPath := tmPoster.String
		backdropPath := tmBackdrop.String
		releaseDate := tmRelease.String
		if releaseDate == "" {
			releaseDate = tmFirstAir.String
		}
		payload["tmdb"] = map[string]any{
			"tmdb_id":        r.TmdbID,
			"media_type":     r.MediaType,
			"title":          tmTitle.String,
			"original_title": tmOriginalTitle.String,
			"overview":       tmOverview.String,
			"poster_path":    posterPath,
			"poster_url":     tmdb.ImageURL(posterPath, "w500", baseURL),
			"backdrop_path":  backdropPath,
			"backdrop_url":   tmdb.ImageURL(backdropPath, "", baseURL),
			"release_date":   releaseDate,
			"status":         tmStatus.String,
			"rating":         round1(tmVote.Float64),
		}
	}
	if libFolder.Valid {
		payload["library"] = map[string]any{
			"in_library":          true,
			"drive_folder_id":     libFolder.String,
			"title":               libTitle.String,
			"year":                libYear.String,
			"poster_url":          tmdb.ImageURL(tmdb.ExtractImagePath(libPoster.String), "w500", baseURL),
			"total_episodes":      nullableInt64(libTotal),
			"in_library_episodes": nullableInt64(libInLib),
		}
	}
	return payload
}

func nullableInt64(v sql.NullInt64) any {
	if v.Valid {
		return v.Int64
	}
	return nil
}

func round1(v float64) float64 {
	return float64(int64(v*10+0.5)) / 10
}

// SubscriptionPayload 创建/更新订阅的载荷。
type SubscriptionPayload map[string]any

func (s *RSSStore) findDuplicateSubscriptionID(payload map[string]any, excludeID *int64) (int64, bool) {
	mediaType := "tv"
	if v, ok := payload["media_type"].(string); ok && v != "" {
		mediaType = v
	}
	tmdbID := nullableInt64FromAny(payload["tmdb_id"])
	subgroup := ""
	if v, ok := payload["subgroup_name"].(string); ok {
		subgroup = v
	}
	seasonNumber := intFromAny(payload["season_number"], 1)
	startEpisode := intFromAny(payload["start_episode"], 1)
	var args []any
	args = append(args, mediaType,
		payload["site"], payload["rss_url"], subgroup,
		seasonNumber, startEpisode, payload["push_target"],
		tmdbID, tmdbID)
	if excludeID != nil {
		args = append(args, *excludeID, *excludeID)
	} else {
		args = append(args, nil, nil)
	}
	var id int64
	err := s.db.QueryRow(`SELECT id FROM rss_subscriptions
		WHERE media_type = ? AND site = ? AND rss_url = ? AND subgroup_name = ?
		  AND season_number = ? AND start_episode = ? AND push_target = ?
		  AND ((tmdb_id IS NULL AND ? IS NULL) OR tmdb_id = ?)
		  AND (? IS NULL OR id != ?) LIMIT 1`, args...).Scan(&id)
	if err != nil {
		return 0, false
	}
	return id, true
}

func nullableInt64FromAny(v any) any {
	switch n := v.(type) {
	case float64:
		if n == 0 {
			return nil
		}
		return int64(n)
	case int:
		if n == 0 {
			return nil
		}
		return int64(n)
	case int64:
		if n == 0 {
			return nil
		}
		return n
	case *int64:
		// HTTP 处理器 payload 里的指针类型（subscriptionBody.toPayload）不能静默丢成 NULL
		if n == nil || *n == 0 {
			return nil
		}
		return *n
	}
	return nil
}

func intFromAny(v any, def int) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case *int:
		if n != nil {
			return *n
		}
	case *int64:
		if n != nil {
			return int(*n)
		}
	}
	return def
}

func boolFromAny(v any, def bool) bool {
	switch b := v.(type) {
	case bool:
		return b
	case int:
		return b != 0
	case float64:
		return b != 0
	case *bool:
		if b != nil {
			return *b
		}
	}
	return def
}

// CreateSubscription 创建订阅（重复时返回 ErrDuplicateSubscription）。
func (s *RSSStore) CreateSubscription(payload map[string]any) (*SubscriptionRecord, error) {
	if _, dup := s.findDuplicateSubscriptionID(payload, nil); dup {
		return nil, ErrDuplicateSubscription
	}
	now := utcNow()
	mediaType := "tv"
	if v, ok := payload["media_type"].(string); ok && v != "" {
		mediaType = v
	}
	enabled := 1
	if !boolFromAny(payload["enabled"], true) {
		enabled = 0
	}
	posterURL := anyStr(payload["poster_url"])
	if posterURL != "" {
		posterURL = tmdb.ExtractImagePath(posterURL)
	}
	res, err := s.db.Exec(`INSERT INTO rss_subscriptions(
		name, media_title, media_type, tmdb_id, poster_url, site, rss_url, subgroup_name,
		season_number, start_episode, keyword_all, push_target, enabled,
		last_checked_at, last_matched_at, last_error, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, ?, ?)`,
		payload["name"], payload["media_title"], mediaType,
		nullableInt64FromAny(payload["tmdb_id"]), nullableStringOr(posterURL),
		payload["site"], payload["rss_url"], payload["subgroup_name"],
		intFromAny(payload["season_number"], 1), intFromAny(payload["start_episode"], 1),
		payload["keyword_all"], payload["push_target"], enabled, now, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetSubscription(id)
}

func nullableStringOr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ListSubscriptions 全部订阅。
func (s *RSSStore) ListSubscriptions() []*SubscriptionRecord {
	rows, err := s.db.Query(subscriptionSelect + " ORDER BY updated_at DESC, id DESC")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*SubscriptionRecord
	for rows.Next() {
		if r := scanSubscription(rows); r != nil {
			out = append(out, r)
		}
	}
	return out
}

// ListEnabledSubscriptions 启用中的订阅。
func (s *RSSStore) ListEnabledSubscriptions() []*SubscriptionRecord {
	rows, err := s.db.Query(subscriptionSelect + " WHERE enabled = 1 ORDER BY updated_at DESC, id DESC")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*SubscriptionRecord
	for rows.Next() {
		if r := scanSubscription(rows); r != nil {
			out = append(out, r)
		}
	}
	return out
}

// GetSubscription 按 ID 取订阅。
func (s *RSSStore) GetSubscription(id int64) (*SubscriptionRecord, error) {
	return scanSubscription(s.db.QueryRow(subscriptionSelect+" WHERE id = ?", id)), nil
}

// ListSubscriptionsJoined 带 TMDB/库状态的订阅列表。
func (s *RSSStore) ListSubscriptionsJoined() []map[string]any {
	if !s.ensureMediaDBAttached() {
		return nil
	}
	rows, err := s.db.Query(s.subscriptionJoinSelect() + " ORDER BY s.updated_at DESC, s.id DESC")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		if payload := s.serializeJoined(rows); payload != nil {
			out = append(out, payload)
		}
	}
	return out
}

// GetSubscriptionJoined 单条带状态的订阅详情。
func (s *RSSStore) GetSubscriptionJoined(id int64) map[string]any {
	if !s.ensureMediaDBAttached() {
		return nil
	}
	return s.serializeJoined(s.db.QueryRow(s.subscriptionJoinSelect()+" WHERE s.id = ? LIMIT 1", id))
}

// UpdateSubscription 更新订阅（nil 表示不存在；重复返回 ErrDuplicateSubscription）。
func (s *RSSStore) UpdateSubscription(id int64, payload map[string]any) (*SubscriptionRecord, error) {
	current := scanSubscription(s.db.QueryRow(subscriptionSelect+" WHERE id = ?", id))
	if current == nil {
		return nil, nil
	}
	merged := map[string]any{
		"name": current.Name, "media_title": current.MediaTitle, "media_type": current.MediaType,
		"tmdb_id": current.TmdbID, "poster_url": current.PosterURL,
		"site": current.Site, "rss_url": current.RssURL, "subgroup_name": current.SubgroupName,
		"season_number": current.SeasonNumber, "start_episode": current.StartEpisode,
		"keyword_all": current.KeywordAll, "push_target": current.PushTarget,
		"enabled": current.Enabled,
	}
	for k, v := range payload {
		merged[k] = v
	}
	exclude := id
	if _, dup := s.findDuplicateSubscriptionID(merged, &exclude); dup {
		return nil, ErrDuplicateSubscription
	}
	posterURL := ""
	if s2, ok := merged["poster_url"].(*string); ok && s2 != nil {
		posterURL = *s2
	} else if s2, ok := merged["poster_url"].(string); ok {
		posterURL = s2
	}
	if posterURL != "" {
		posterURL = tmdb.ExtractImagePath(posterURL)
	}
	enabled := 1
	if !boolFromAny(merged["enabled"], false) {
		enabled = 0
	}
	_, err := s.db.Exec(`UPDATE rss_subscriptions
		SET name = ?, media_title = ?, media_type = ?, tmdb_id = ?, poster_url = ?, site = ?, rss_url = ?, subgroup_name = ?,
		    season_number = ?, start_episode = ?, keyword_all = ?, push_target = ?, enabled = ?, updated_at = ?
		WHERE id = ?`,
		merged["name"], merged["media_title"], merged["media_type"],
		nullableInt64FromAny(merged["tmdb_id"]), nullableStringOr(posterURL),
		merged["site"], merged["rss_url"], merged["subgroup_name"],
		intFromAny(merged["season_number"], 1), intFromAny(merged["start_episode"], 1),
		merged["keyword_all"], merged["push_target"], enabled, utcNow(), id)
	if err != nil {
		return nil, err
	}
	return s.GetSubscription(id)
}

// DeleteSubscription 删除订阅。
func (s *RSSStore) DeleteSubscription(id int64) bool {
	res, err := s.db.Exec("DELETE FROM rss_subscriptions WHERE id = ?", id)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

// MarkChecked 标记检查时间/错误。
func (s *RSSStore) MarkChecked(id int64, checkedAt, errStr string) {
	if checkedAt == "" {
		checkedAt = utcNow()
	}
	var errVal any
	if errStr != "" {
		errVal = errStr
	}
	_, _ = s.db.Exec(
		"UPDATE rss_subscriptions SET last_checked_at = ?, last_error = ?, updated_at = ? WHERE id = ?",
		checkedAt, errVal, utcNow(), id)
}

// MarkMatched 标记命中时间。
func (s *RSSStore) MarkMatched(id int64, matchedAt string) {
	if matchedAt == "" {
		matchedAt = utcNow()
	}
	_, _ = s.db.Exec(
		"UPDATE rss_subscriptions SET last_matched_at = ?, updated_at = ? WHERE id = ?",
		matchedAt, matchedAt, id)
}

// RecordHit 记录命中（dedupe 唯一索引冲突时返回 ErrDuplicateSubscription 语义错误）。
func (s *RSSStore) RecordHit(payload map[string]any) (*HitRecord, error) {
	res, err := s.db.Exec(`INSERT INTO rss_subscription_hits(
		subscription_id, episode_title, season_number, episode_number, torrent_url, magnet_url,
		published_at, dedupe_key, pushed_to, push_status, push_error, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		payload["subscription_id"], payload["episode_title"],
		nullableInt64FromAny(payload["season_number"]), nullableInt64FromAny(payload["episode_number"]),
		payload["torrent_url"], payload["magnet_url"], payload["published_at"],
		payload["dedupe_key"], payload["pushed_to"], payload["push_status"],
		payload["push_error"], utcNow())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrDuplicateSubscription
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetHit(id)
}

// GetHit 按 ID 取命中。
func (s *RSSStore) GetHit(id int64) (*HitRecord, error) {
	return scanHit(s.db.QueryRow(hitSelect+" WHERE id = ?", id)), nil
}

// HitExists 命中是否已存在。
func (s *RSSStore) HitExists(subscriptionID int64, dedupeKey string) bool {
	var one int
	err := s.db.QueryRow(
		"SELECT 1 FROM rss_subscription_hits WHERE subscription_id = ? AND dedupe_key = ? LIMIT 1",
		subscriptionID, dedupeKey).Scan(&one)
	return err == nil
}

// ListHits 最近命中。
func (s *RSSStore) ListHits(subscriptionID int64, limit int) []*HitRecord {
	if limit <= 0 {
		limit = 5
	}
	rows, err := s.db.Query(
		hitSelect+" WHERE subscription_id = ? ORDER BY created_at DESC, id DESC LIMIT ?",
		subscriptionID, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []*HitRecord{}
	for rows.Next() {
		if h := scanHit(rows); h != nil {
			out = append(out, h)
		}
	}
	return out
}

// CountHits 命中计数。
func (s *RSSStore) CountHits(subscriptionID int64) int64 {
	var n int64
	_ = s.db.QueryRow(
		"SELECT COUNT(*) FROM rss_subscription_hits WHERE subscription_id = ?", subscriptionID).Scan(&n)
	return n
}
