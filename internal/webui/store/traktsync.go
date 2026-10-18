// traktsync.go —— Plex ↔ Trakt 观看同步的影子状态存储。
//
// 独立 traktsync.db，不与 library.db 等任何现有库/表混用（本功能对其他库零写入；
// 唯一跨库动作是 webui 层只读查询 media_title_pins 做身份反查）。
//
// sync_items 语义（与 syncengine.Shadow 注释一致）：
//   - 影子 = 每侧「最后确认态」，仅在干净读取或远端写成功后更新
//   - Trakt reset = 行存在且 trakt 侧已看但本轮枚举缺席；从未看过 = 无行或 watched 为 NULL
//   - *_seen=0（缺席本轮枚举）≠ 取消观看，引擎会跳过该侧 diff
package store

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"meta2cloud/internal/syncengine"
	"meta2cloud/internal/watched"
)

const traktSyncDDL = `
CREATE TABLE IF NOT EXISTS sync_items (
	media_type   TEXT    NOT NULL,              -- 'movie' | 'tv'
	tmdb_id      INTEGER NOT NULL,
	season       INTEGER NOT NULL DEFAULT 0,    -- 电影为 0
	episode      INTEGER NOT NULL DEFAULT 0,    -- 电影为 0
	plex_ref             TEXT,                  -- ratingKey 缓存（会话轮询/日志用，不承担索引职责）
	plex_seen            INTEGER NOT NULL DEFAULT 0,
	plex_watched         INTEGER,               -- NULL=未知；1=已看
	plex_plays           INTEGER NOT NULL DEFAULT 0,
	plex_last_watched_at INTEGER NOT NULL DEFAULT 0,  -- unix 秒
	trakt_seen            INTEGER NOT NULL DEFAULT 0,
	trakt_watched         INTEGER,               -- NULL=未知；1=已看
	trakt_plays           INTEGER NOT NULL DEFAULT 0,
	trakt_last_watched_at INTEGER NOT NULL DEFAULT 0,  -- unix 秒
	session_pushed_at     INTEGER NOT NULL DEFAULT 0,  -- 会话轮询最近推送时刻（防重）
	updated_at    TEXT NOT NULL,
	PRIMARY KEY (media_type, tmdb_id, season, episode)
);

CREATE INDEX IF NOT EXISTS idx_sync_items_plex_ref ON sync_items(plex_ref);

CREATE TABLE IF NOT EXISTS sync_runs (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	started_at    TEXT NOT NULL,
	finished_at   TEXT,
	trigger_type  TEXT NOT NULL DEFAULT 'manual',
	to_trakt      INTEGER NOT NULL DEFAULT 0,
	to_plex       INTEGER NOT NULL DEFAULT 0,
	conflicts     INTEGER NOT NULL DEFAULT 0,
	unresolved    INTEGER NOT NULL DEFAULT 0,
	skipped       INTEGER NOT NULL DEFAULT 0,
	error         TEXT
);

CREATE TABLE IF NOT EXISTS sync_run_items (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id     INTEGER NOT NULL,
	media_type TEXT, tmdb_id INTEGER, season INTEGER, episode INTEGER, title TEXT,
	direction  TEXT NOT NULL,               -- to_plex | to_trakt | unresolved
	action     TEXT NOT NULL,               -- watched | unwatched | unresolved
	success    INTEGER NOT NULL DEFAULT 1,
	error      TEXT
);

CREATE INDEX IF NOT EXISTS idx_sync_run_items_run ON sync_run_items(run_id);

CREATE TABLE IF NOT EXISTS sync_sessions (
	session_key    TEXT PRIMARY KEY,
	plex_ref       TEXT NOT NULL,
	media_type     TEXT NOT NULL,
	tmdb_id        INTEGER NOT NULL,
	season         INTEGER NOT NULL DEFAULT 0,
	episode        INTEGER NOT NULL DEFAULT 0,
	title          TEXT,
	view_offset_ms INTEGER NOT NULL DEFAULT 0,
	duration_ms    INTEGER NOT NULL DEFAULT 0,
	max_progress   REAL NOT NULL DEFAULT 0,
	misses         INTEGER NOT NULL DEFAULT 0,
	updated_at     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sync_meta (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`

// TraktSyncStore 观看同步影子存储（独立 traktsync.db）。
// 实现 syncengine.ShadowStore。
type TraktSyncStore struct {
	db *sql.DB
}

// OpenTraktSyncStore 打开独立 traktsync.db。
func OpenTraktSyncStore(dbPath string) (*TraktSyncStore, error) {
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
	if _, err := db.Exec(traktSyncDDL); err != nil {
		db.Close()
		return nil, err
	}
	s := &TraktSyncStore{db: db}
	s.failStaleRunningRuns()
	return s, nil
}

// Close 关闭。
func (s *TraktSyncStore) Close() error { return s.db.Close() }

func traktSyncNow() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000000-07:00") }

func itemKeyValues(k watched.MediaKey) (string, int64, int, int) {
	return k.Type, k.TMDBID, k.Season, k.Episode
}

// ── sync_items（syncengine.ShadowStore）─────────────────────

const syncItemSelect = `SELECT media_type, tmdb_id, season, episode, plex_ref,
	plex_seen, plex_watched, plex_plays, plex_last_watched_at,
	trakt_seen, trakt_watched, trakt_plays, trakt_last_watched_at, session_pushed_at
	FROM sync_items`

func scanShadowItem(row interface{ Scan(...any) error }) (watched.MediaKey, syncengine.Shadow, error) {
	var (
		key       watched.MediaKey
		sh        syncengine.Shadow
		plexRef   sql.NullString
		plexWatch sql.NullBool
		traktWatch sql.NullBool
	)
	err := row.Scan(&key.Type, &key.TMDBID, &key.Season, &key.Episode, &plexRef,
		&sh.PlexSeen, &plexWatch, &sh.PlexPlays, &sh.PlexLastAt,
		&sh.TraktSeen, &traktWatch, &sh.TraktPlays, &sh.TraktLastAt, &sh.SessionPushedAt)
	if err != nil {
		return key, sh, err
	}
	if plexRef.Valid {
		sh.PlexRef = plexRef.String
	}
	if plexWatch.Valid {
		sh.PlexWatched = &plexWatch.Bool
	}
	if traktWatch.Valid {
		sh.TraktWatched = &traktWatch.Bool
	}
	sh.Key = key
	return key, sh, nil
}

// AllShadow 读取全部影子行。
func (s *TraktSyncStore) AllShadow() (map[watched.MediaKey]syncengine.Shadow, error) {
	rows, err := s.db.Query(syncItemSelect)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[watched.MediaKey]syncengine.Shadow{}
	for rows.Next() {
		key, sh, err := scanShadowItem(rows)
		if err != nil {
			continue
		}
		out[key] = sh
	}
	return out, rows.Err()
}

// GetShadow 按键读取影子行。
func (s *TraktSyncStore) GetShadow(key watched.MediaKey) (syncengine.Shadow, bool, error) {
	mediaType, tmdbID, season, episode := itemKeyValues(key)
	row := s.db.QueryRow(syncItemSelect+" WHERE media_type = ? AND tmdb_id = ? AND season = ? AND episode = ?",
		mediaType, tmdbID, season, episode)
	_, sh, err := scanShadowItem(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return syncengine.Shadow{}, false, nil
		}
		return syncengine.Shadow{}, false, err
	}
	return sh, true, nil
}

// SaveShadow 批量保存影子行（upsert；事务保证批内一致）。
func (s *TraktSyncStore) SaveShadow(items []syncengine.Shadow) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := traktSyncNow()
	for _, sh := range items {
		mediaType, tmdbID, season, episode := itemKeyValues(sh.Key)
		var plexRef any
		if sh.PlexRef != "" {
			plexRef = sh.PlexRef
		}
		var plexWatch, traktWatch any
		if sh.PlexWatched != nil {
			plexWatch = *sh.PlexWatched
		}
		if sh.TraktWatched != nil {
			traktWatch = *sh.TraktWatched
		}
		seenInt := func(v bool) int {
			if v {
				return 1
			}
			return 0
		}
		if _, err := tx.Exec(`INSERT INTO sync_items(
				media_type, tmdb_id, season, episode, plex_ref,
				plex_seen, plex_watched, plex_plays, plex_last_watched_at,
				trakt_seen, trakt_watched, trakt_plays, trakt_last_watched_at,
				session_pushed_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(media_type, tmdb_id, season, episode) DO UPDATE SET
				plex_ref = excluded.plex_ref,
				plex_seen = excluded.plex_seen,
				plex_watched = excluded.plex_watched,
				plex_plays = excluded.plex_plays,
				plex_last_watched_at = excluded.plex_last_watched_at,
				trakt_seen = excluded.trakt_seen,
				trakt_watched = excluded.trakt_watched,
				trakt_plays = excluded.trakt_plays,
				trakt_last_watched_at = excluded.trakt_last_watched_at,
				session_pushed_at = excluded.session_pushed_at,
				updated_at = excluded.updated_at`,
			mediaType, tmdbID, season, episode, plexRef,
			seenInt(sh.PlexSeen), plexWatch, sh.PlexPlays, sh.PlexLastAt,
			seenInt(sh.TraktSeen), traktWatch, sh.TraktPlays, sh.TraktLastAt,
			sh.SessionPushedAt, now); err != nil {
			return fmt.Errorf("保存影子 %s 失败：%w", sh.Key, err)
		}
	}
	return tx.Commit()
}

// UpdateTraktWatched 会话轮询专用：只进不退地更新 trakt 侧影子与推送时间戳
// （行不存在则先建行，两侧其余字段保持未知态，待下轮全量同步补齐）。
func (s *TraktSyncStore) UpdateTraktWatched(key watched.MediaKey, at int64, sessionPushedAt int64) error {
	sh, ok, err := s.GetShadow(key)
	if err != nil {
		return err
	}
	if !ok {
		sh = syncengine.Shadow{Key: key}
	}
	sh.TraktSeen = true
	sh.TraktWatched = ptrBool(true)
	if at > sh.TraktLastAt {
		sh.TraktLastAt = at
	}
	if sessionPushedAt > sh.SessionPushedAt {
		sh.SessionPushedAt = sessionPushedAt
	}
	return s.SaveShadow([]syncengine.Shadow{sh})
}

// GetShadowByPlexRef 按 ratingKey 查影子行（会话轮询的身份快路径）。
func (s *TraktSyncStore) GetShadowByPlexRef(ref string) (syncengine.Shadow, bool, error) {
	row := s.db.QueryRow(syncItemSelect+" WHERE plex_ref = ? LIMIT 1", ref)
	_, sh, err := scanShadowItem(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return syncengine.Shadow{}, false, nil
		}
		return syncengine.Shadow{}, false, err
	}
	return sh, true, nil
}

func ptrBool(v bool) *bool { return &v }

// ── sync_runs ───────────────────────────────────────────────

// StartSyncRun 建一次同步 run，返回 run ID。
func (s *TraktSyncStore) StartSyncRun(trigger string) (int64, error) {
	res, err := s.db.Exec(
		"INSERT INTO sync_runs(started_at, trigger_type) VALUES (?, ?)", traktSyncNow(), trigger)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// FinishSyncRun 收尾一次同步 run。
func (s *TraktSyncStore) FinishSyncRun(runID int64, summary syncengine.RunSummary) error {
	var errArg any
	if summary.Err != "" {
		errArg = summary.Err
	}
	_, err := s.db.Exec(`UPDATE sync_runs
		SET finished_at = ?, to_trakt = ?, to_plex = ?, conflicts = ?, unresolved = ?, skipped = ?, error = ?
		WHERE id = ?`,
		traktSyncNow(), summary.ToTrakt, summary.ToPlex, summary.Conflicts,
		summary.Unresolved, summary.Skipped, errArg, runID)
	return err
}

// failStaleRunningRuns 把进程崩溃遗留的「进行中」run 收尾为失败。
func (s *TraktSyncStore) failStaleRunningRuns() {
	res, err := s.db.Exec(`UPDATE sync_runs
		SET finished_at = ?, error = '进程重启中断，同步未完成'
		WHERE finished_at IS NULL`, traktSyncNow())
	if err != nil {
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("已收尾 %d 条遗留的进行中观看同步记录", n)
	}
}

// TraktSyncRun 一次同步 run 的统计（对外载荷）。
type TraktSyncRun struct {
	ID        int64
	StartedAt string
	FinishedAt *string
	Trigger   string
	ToTrakt   int
	ToPlex    int
	Conflicts int
	Unresolved int
	Skipped   int
	Error     *string
}

const syncRunSelect = `SELECT id, started_at, finished_at, trigger_type,
	to_trakt, to_plex, conflicts, unresolved, skipped, error FROM sync_runs`

func scanSyncRun(row interface{ Scan(...any) error }) *TraktSyncRun {
	var r TraktSyncRun
	if err := row.Scan(&r.ID, &r.StartedAt, &r.FinishedAt, &r.Trigger,
		&r.ToTrakt, &r.ToPlex, &r.Conflicts, &r.Unresolved, &r.Skipped, &r.Error); err != nil {
		return nil
	}
	return &r
}

// GetLastFinishedSyncRun 最近一次已完成的同步。
func (s *TraktSyncStore) GetLastFinishedSyncRun() *TraktSyncRun {
	return scanSyncRun(s.db.QueryRow(syncRunSelect+" WHERE finished_at IS NOT NULL ORDER BY id DESC LIMIT 1"))
}

// ListSyncRuns 按 limit/offset 列出同步历史（新→旧）。
func (s *TraktSyncStore) ListSyncRuns(limit, offset int) ([]TraktSyncRun, int, error) {
	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM sync_runs").Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(syncRunSelect+" ORDER BY id DESC LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []TraktSyncRun{}
	for rows.Next() {
		if r := scanSyncRun(rows); r != nil {
			out = append(out, *r)
		}
	}
	return out, total, rows.Err()
}

// ── sync_run_items ──────────────────────────────────────────

// InsertRunItem 写一条逐键明细（忽略失败，明细不阻塞同步）。
func (s *TraktSyncStore) InsertRunItem(runID int64, item syncengine.RunItem) error {
	if runID == 0 {
		return nil
	}
	var (
		mediaType any
		tmdbID    any
		season    any
		episode   any
		title     any
		errArg    any
	)
	if item.Key.Type != "" {
		mediaType = item.Key.Type
		tmdbID = item.Key.TMDBID
		season = item.Key.Season
		episode = item.Key.Episode
	}
	if item.Title != "" {
		title = item.Title
	}
	if item.Err != "" {
		errArg = item.Err
	}
	successInt := 0
	if item.Success {
		successInt = 1
	}
	_, err := s.db.Exec(`INSERT INTO sync_run_items(run_id, media_type, tmdb_id, season, episode, title, direction, action, success, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		runID, mediaType, tmdbID, season, episode, title, item.Direction, item.Action, successInt, errArg)
	return err
}

// TraktSyncRunItem 同步明细条目。
type TraktSyncRunItem struct {
	ID        int64
	RunID     int64
	MediaType *string
	TmdbID    *int64
	Season    *int
	Episode   *int
	Title     *string
	Direction string
	Action    string
	Success   bool
	Error     *string
}

// ListRunItems 列出一次 run 的明细（新→旧，限 limit 条）。
func (s *TraktSyncStore) ListRunItems(runID int64, limit int) ([]TraktSyncRunItem, error) {
	rows, err := s.db.Query(`SELECT id, run_id, media_type, tmdb_id, season, episode, title, direction, action, success, error
		FROM sync_run_items WHERE run_id = ? ORDER BY id DESC LIMIT ?`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TraktSyncRunItem{}
	for rows.Next() {
		var it TraktSyncRunItem
		var success int
		if err := rows.Scan(&it.ID, &it.RunID, &it.MediaType, &it.TmdbID, &it.Season, &it.Episode,
			&it.Title, &it.Direction, &it.Action, &success, &it.Error); err != nil {
			continue
		}
		it.Success = success == 1
		out = append(out, it)
	}
	return out, rows.Err()
}

// ── sync_sessions（会话轮询状态，丢失可容忍）────────────────

// TraktSyncSession 一条进行中播放的观察状态。
type TraktSyncSession struct {
	SessionKey   string
	PlexRef      string
	MediaType    string
	TmdbID       int64
	Season       int
	Episode      int
	Title        *string
	ViewOffsetMS int64
	DurationMS   int64
	MaxProgress  float64
	Misses       int
	UpdatedAt    string
}

const syncSessionSelect = `SELECT session_key, plex_ref, media_type, tmdb_id, season, episode,
	title, view_offset_ms, duration_ms, max_progress, misses, updated_at FROM sync_sessions`

func scanSyncSession(row interface{ Scan(...any) error }) *TraktSyncSession {
	var sess TraktSyncSession
	if err := row.Scan(&sess.SessionKey, &sess.PlexRef, &sess.MediaType, &sess.TmdbID, &sess.Season,
		&sess.Episode, &sess.Title, &sess.ViewOffsetMS, &sess.DurationMS, &sess.MaxProgress,
		&sess.Misses, &sess.UpdatedAt); err != nil {
		return nil
	}
	return &sess
}

// GetSyncSession 按会话键读取。
func (s *TraktSyncStore) GetSyncSession(sessionKey string) *TraktSyncSession {
	return scanSyncSession(s.db.QueryRow(syncSessionSelect+" WHERE session_key = ?", sessionKey))
}

// ListSyncSessions 全部会话观察行。
func (s *TraktSyncStore) ListSyncSessions() []TraktSyncSession {
	rows, err := s.db.Query(syncSessionSelect)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []TraktSyncSession{}
	for rows.Next() {
		if sess := scanSyncSession(rows); sess != nil {
			out = append(out, *sess)
		}
	}
	return out
}

// UpsertSyncSession 写入/更新一次会话观察（progress 取历史最大）。
func (s *TraktSyncStore) UpsertSyncSession(sess TraktSyncSession) {
	current := s.GetSyncSession(sess.SessionKey)
	now := traktSyncNow()
	if current != nil && current.MaxProgress > sess.MaxProgress {
		sess.MaxProgress = current.MaxProgress
	}
	var titleArg any
	if sess.Title != nil {
		titleArg = *sess.Title
	}
	if current == nil {
		_, _ = s.db.Exec(`INSERT INTO sync_sessions(
				session_key, plex_ref, media_type, tmdb_id, season, episode, title,
				view_offset_ms, duration_ms, max_progress, misses, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?)`,
			sess.SessionKey, sess.PlexRef, sess.MediaType, sess.TmdbID, sess.Season, sess.Episode, titleArg,
			sess.ViewOffsetMS, sess.DurationMS, sess.MaxProgress, now)
		return
	}
	_, _ = s.db.Exec(`UPDATE sync_sessions
		SET plex_ref = ?, media_type = ?, tmdb_id = ?, season = ?, episode = ?, title = ?,
		    view_offset_ms = ?, duration_ms = ?, max_progress = ?, misses = 0, updated_at = ?
		WHERE session_key = ?`,
		sess.PlexRef, sess.MediaType, sess.TmdbID, sess.Season, sess.Episode, titleArg,
		sess.ViewOffsetMS, sess.DurationMS, sess.MaxProgress, now, sess.SessionKey)
}

// IncrementSessionMiss 会话键缺席本次轮询时递增 miss 计数，返回累计值。
func (s *TraktSyncStore) IncrementSessionMiss(sessionKey string) int {
	if _, err := s.db.Exec("UPDATE sync_sessions SET misses = misses + 1 WHERE session_key = ?", sessionKey); err != nil {
		return 0
	}
	if sess := s.GetSyncSession(sessionKey); sess != nil {
		return sess.Misses
	}
	return 0
}

// DeleteSyncSession 删除会话观察行。
func (s *TraktSyncStore) DeleteSyncSession(sessionKey string) {
	_, _ = s.db.Exec("DELETE FROM sync_sessions WHERE session_key = ?", sessionKey)
}

// ── sync_meta（引擎元数据）──────────────────────────────────

// SetMeta 写入元数据键值。
func (s *TraktSyncStore) SetMeta(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO sync_meta(key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// GetMeta 读取元数据值；不存在返回空串。
func (s *TraktSyncStore) GetMeta(key string) string {
	var v string
	_ = s.db.QueryRow("SELECT value FROM sync_meta WHERE key = ?", key).Scan(&v)
	return v
}

// metaKeyTraktPushedAt 最近一次成功向 Trakt 写入观看状态的时刻（unix 秒）。
const metaKeyTraktPushedAt = "trakt.pushed_at"

// SetTraktPushedAt 记录最近一次成功向 Trakt 写入的时刻。
func (s *TraktSyncStore) SetTraktPushedAt(at int64) error {
	return s.SetMeta(metaKeyTraktPushedAt, strconv.FormatInt(at, 10))
}

// TraktPushedAt 读取最近一次成功向 Trakt 写入的时刻；从未写过返回 0。
func (s *TraktSyncStore) TraktPushedAt() (int64, error) {
	v := s.GetMeta(metaKeyTraktPushedAt)
	if v == "" {
		return 0, nil
	}
	return strconv.ParseInt(v, 10, 64)
}
