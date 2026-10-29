// traktsync.go —— Plex ↔ Trakt 观看同步的同步状态存储。
//
// 独立 traktsync.db，不与 library.db 等任何现有库/表混用（本功能对其他库零写入；
// 唯一跨库动作是 webui 层只读查询 media_title_pins 做身份反查）。
//
// sync_items 语义（与 syncengine.Shadow 注释一致）：
//   - 同步状态 = 每侧「最后确认态」，仅在干净读取或远端写成功后更新
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
	anchor       TEXT    NOT NULL DEFAULT 'tmdb', -- 身份锚：'tmdb' | 'tvdb'（剧集缺 tmdb 时）
	tmdb_id      INTEGER NOT NULL,              -- 锚 ID（列名沿用；anchor='tvdb' 时存 tvdb id）
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
	session_pushed_at     INTEGER NOT NULL DEFAULT 0,  -- 会话路径最近一次推送尝试时刻
	updated_at    TEXT NOT NULL,
	PRIMARY KEY (media_type, anchor, tmdb_id, season, episode)
);

CREATE INDEX IF NOT EXISTS idx_sync_items_plex_ref ON sync_items(plex_ref);

-- 外部 ID 映射表（1b 身份归一化）：把 tmdb/tvdb/imdb 三系 ID 归并到同一媒体，
-- 键规范为 tmdb 优先、剧集缺 tmdb 时回退 tvdb。
CREATE TABLE IF NOT EXISTS sync_id_map (
	sid        INTEGER PRIMARY KEY AUTOINCREMENT,
	media_type TEXT    NOT NULL,
	tmdb_id    INTEGER NOT NULL DEFAULT 0,
	tvdb_id    INTEGER NOT NULL DEFAULT 0,
	imdb_id    TEXT    NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_id_map_tmdb ON sync_id_map(media_type, tmdb_id) WHERE tmdb_id > 0;
CREATE UNIQUE INDEX IF NOT EXISTS idx_id_map_tvdb ON sync_id_map(media_type, tvdb_id) WHERE tvdb_id > 0;
CREATE UNIQUE INDEX IF NOT EXISTS idx_id_map_imdb ON sync_id_map(media_type, imdb_id) WHERE imdb_id <> '';

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
	error         TEXT,
	title         TEXT,
	progress      REAL
);

CREATE TABLE IF NOT EXISTS sync_run_items (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id     INTEGER NOT NULL,
	media_type TEXT, tmdb_id INTEGER, season INTEGER, episode INTEGER, title TEXT,
	anchor     TEXT NOT NULL DEFAULT 'tmdb',
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
	anchor         TEXT NOT NULL DEFAULT 'tmdb',
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

CREATE TABLE IF NOT EXISTS sync_ignores (
	media_type TEXT NOT NULL,             -- movie | tv（媒体级：整部电影/整部剧）
	anchor     TEXT NOT NULL DEFAULT 'tmdb',
	tmdb_id    INTEGER NOT NULL,           -- 锚 ID（anchor='tvdb' 时存 tvdb id）
	title      TEXT,                       -- 忽略时快照，列表展示用
	reason     TEXT,                       -- 触发忽略的错误（如 Trakt 无法识别 tmdb id）
	created_at TEXT NOT NULL,
	PRIMARY KEY (media_type, anchor, tmdb_id)
);

CREATE TABLE IF NOT EXISTS trakt_show_cache (
	anchor          TEXT NOT NULL,        -- 'tmdb' | 'tvdb'
	anchor_id       INTEGER NOT NULL,
	plays           INTEGER NOT NULL,     -- 剧级指纹：/sync/watched/shows 的 plays
	last_watched_at TEXT NOT NULL,        -- 剧级指纹：last_watched_at（原样 Zeit 串）
	episodes        TEXT NOT NULL,        -- 上次逐集结果 JSON [{season,episode,last_at}]
	updated_at      TEXT NOT NULL,        -- 缓存写入时刻（TTL 兜底用）
	PRIMARY KEY (anchor, anchor_id)
);
`

// TraktSyncStore 观看同步同步状态存储（独立 traktsync.db）。
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
	// 既有库补列（v2.4.0 的表没有 title/progress）：列已存在时报错忽略
	// 版本化迁移（见 migrate.go）：
	//   v1 sync_runs 补 title/progress（v2.4.0 前的表没有）；
	//   v2 身份锚——sync_items/sync_ignores 主键加入 anchor（SQLite 不能改主键，
	//      整表重建，重建语句带显式列清单可安全重放），sync_sessions/sync_run_items 补 anchor 列。
	// 基线按历史应用顺序探测：有 progress 记 v1，progress+anchor 记 v2，
	// 版本管理引入前的存量库不重放。
	baseline := func(db *sql.DB) (int, error) {
		hasProgress, err := columnExists(db, "sync_runs", "progress")
		if err != nil || !hasProgress {
			return 0, err
		}
		hasAnchor, err := columnExists(db, "sync_items", "anchor")
		if err != nil || !hasAnchor {
			return 1, err
		}
		return 2, nil
	}
	if err := migrateStoreSchema(db, "traktsync", baseline, []migration{
		{
			version: 1, name: "sync_runs.title/progress",
			stmts: []string{
				"ALTER TABLE sync_runs ADD COLUMN title TEXT",
				"ALTER TABLE sync_runs ADD COLUMN progress REAL",
			},
		},
		{
			version: 2, name: "身份锚 anchor", legacyTolerant: true,
			stmts: []string{
				`ALTER TABLE sync_items RENAME TO sync_items_old;
CREATE TABLE sync_items (
	media_type   TEXT    NOT NULL,
	anchor       TEXT    NOT NULL DEFAULT 'tmdb',
	tmdb_id      INTEGER NOT NULL,
	season       INTEGER NOT NULL DEFAULT 0,
	episode      INTEGER NOT NULL DEFAULT 0,
	plex_ref             TEXT,
	plex_seen            INTEGER NOT NULL DEFAULT 0,
	plex_watched         INTEGER,
	plex_plays           INTEGER NOT NULL DEFAULT 0,
	plex_last_watched_at INTEGER NOT NULL DEFAULT 0,
	trakt_seen            INTEGER NOT NULL DEFAULT 0,
	trakt_watched         INTEGER,
	trakt_plays           INTEGER NOT NULL DEFAULT 0,
	trakt_last_watched_at INTEGER NOT NULL DEFAULT 0,
	session_pushed_at     INTEGER NOT NULL DEFAULT 0,
	updated_at    TEXT NOT NULL,
	PRIMARY KEY (media_type, anchor, tmdb_id, season, episode)
);
INSERT INTO sync_items SELECT media_type, 'tmdb', tmdb_id, season, episode, plex_ref,
	plex_seen, plex_watched, plex_plays, plex_last_watched_at,
	trakt_seen, trakt_watched, trakt_plays, trakt_last_watched_at, session_pushed_at, updated_at
	FROM sync_items_old;
DROP TABLE sync_items_old;
CREATE INDEX IF NOT EXISTS idx_sync_items_plex_ref ON sync_items(plex_ref);`,
				`ALTER TABLE sync_ignores RENAME TO sync_ignores_old;
CREATE TABLE sync_ignores (
	media_type TEXT NOT NULL,
	anchor     TEXT NOT NULL DEFAULT 'tmdb',
	tmdb_id    INTEGER NOT NULL,
	title      TEXT,
	reason     TEXT,
	created_at TEXT NOT NULL,
	PRIMARY KEY (media_type, anchor, tmdb_id)
);
INSERT INTO sync_ignores SELECT media_type, 'tmdb', tmdb_id, title, reason, created_at FROM sync_ignores_old;
DROP TABLE sync_ignores_old;`,
				"ALTER TABLE sync_sessions ADD COLUMN anchor TEXT NOT NULL DEFAULT 'tmdb'",
				"ALTER TABLE sync_run_items ADD COLUMN anchor TEXT NOT NULL DEFAULT 'tmdb'",
			},
		},
	}); err != nil {
		db.Close()
		return nil, err
	}
	s := &TraktSyncStore{db: db}
	s.failStaleRunningRuns()
	s.purgeLegacySentinelRuns()
	s.repairSessionPushItems()
	return s, nil
}

// Close 关闭。
func (s *TraktSyncStore) Close() error { return s.db.Close() }

func traktSyncNow() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000000-07:00") }

// columnExists 表列是否存在（迁移探测用）。
func columnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull int
		var dfltValue any
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dfltValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// mediaKeyValues 键 → 存储四元组（锚类型 + 锚 ID）。
func mediaKeyValues(k watched.MediaKey) (string, string, int64, int, int) {
	if k.TVDBID > 0 {
		return k.Type, "tvdb", k.TVDBID, k.Season, k.Episode
	}
	return k.Type, "tmdb", k.TMDBID, k.Season, k.Episode
}

// ── sync_items（syncengine.ShadowStore）─────────────────────

const syncItemSelect = `SELECT media_type, anchor, tmdb_id, season, episode, plex_ref,
	plex_seen, plex_watched, plex_plays, plex_last_watched_at,
	trakt_seen, trakt_watched, trakt_plays, trakt_last_watched_at, session_pushed_at
	FROM sync_items`

func scanShadowItem(row interface{ Scan(...any) error }) (watched.MediaKey, syncengine.Shadow, error) {
	var (
		key        watched.MediaKey
		anchor     string
		anchorID   int64
		sh         syncengine.Shadow
		plexRef    sql.NullString
		plexWatch  sql.NullBool
		traktWatch sql.NullBool
	)
	err := row.Scan(&key.Type, &anchor, &anchorID, &key.Season, &key.Episode, &plexRef,
		&sh.PlexSeen, &plexWatch, &sh.PlexPlays, &sh.PlexLastAt,
		&sh.TraktSeen, &traktWatch, &sh.TraktPlays, &sh.TraktLastAt, &sh.SessionPushedAt)
	if err != nil {
		return key, sh, err
	}
	if anchor == "tvdb" {
		key.TVDBID = anchorID
	} else {
		key.TMDBID = anchorID
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

// AllShadow 读取全部状态记录。
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

// GetShadow 按键读取状态记录。
func (s *TraktSyncStore) GetShadow(key watched.MediaKey) (syncengine.Shadow, bool, error) {
	mediaType, anchor, anchorID, season, episode := mediaKeyValues(key)
	row := s.db.QueryRow(syncItemSelect+" WHERE media_type = ? AND anchor = ? AND tmdb_id = ? AND season = ? AND episode = ?",
		mediaType, anchor, anchorID, season, episode)
	_, sh, err := scanShadowItem(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return syncengine.Shadow{}, false, nil
		}
		return syncengine.Shadow{}, false, err
	}
	return sh, true, nil
}

// SaveShadow 批量保存状态记录（upsert；事务保证批内一致）。
func (s *TraktSyncStore) SaveShadow(items []syncengine.Shadow) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := traktSyncNow()
	for _, sh := range items {
		mediaType, anchor, anchorID, season, episode := mediaKeyValues(sh.Key)
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
				media_type, anchor, tmdb_id, season, episode, plex_ref,
				plex_seen, plex_watched, plex_plays, plex_last_watched_at,
				trakt_seen, trakt_watched, trakt_plays, trakt_last_watched_at,
				session_pushed_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(media_type, anchor, tmdb_id, season, episode) DO UPDATE SET
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
			mediaType, anchor, anchorID, season, episode, plexRef,
			seenInt(sh.PlexSeen), plexWatch, sh.PlexPlays, sh.PlexLastAt,
			seenInt(sh.TraktSeen), traktWatch, sh.TraktPlays, sh.TraktLastAt,
			sh.SessionPushedAt, now); err != nil {
			return fmt.Errorf("保存同步状态 %s 失败：%w", sh.Key, err)
		}
	}
	return tx.Commit()
}

// UpdateTraktWatched 会话轮询专用：只进不退地更新 trakt 侧同步状态与推送时间戳
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

// MarkSessionPushFailure 记录一次失败的会话推送尝试（防重闸退避用）：
// 会话推送的失败不做即时重试——SSE 每 ~10s 一条事件，无闸会无限重试刷屏。
// 只推进 SessionPushedAt，不改动观看状态与观察值；无行时新建。
func (s *TraktSyncStore) MarkSessionPushFailure(key watched.MediaKey, at int64) error {
	sh, ok, err := s.GetShadow(key)
	if err != nil {
		return err
	}
	if !ok {
		sh = syncengine.Shadow{Key: key}
	}
	if at > sh.SessionPushedAt {
		sh.SessionPushedAt = at
	}
	return s.SaveShadow([]syncengine.Shadow{sh})
}

// GetShadowByPlexRef 按 ratingKey 查状态记录（会话轮询的身份快路径）。
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
		log.Printf("已将 %d 条中断的观看同步记录标记为失败", n)
	}
}

// purgeLegacySentinelRuns 删除已下线的 Plex 观看哨兵（plex_activity）遗留记录
// 及其明细，使同步列表不残留已不存在触发来源的条目。
func (s *TraktSyncStore) purgeLegacySentinelRuns() {
	res, err := s.db.Exec(`DELETE FROM sync_runs WHERE trigger_type = 'plex_activity'`)
	if err != nil {
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		_, _ = s.db.Exec("DELETE FROM sync_run_items WHERE run_id NOT IN (SELECT id FROM sync_runs)")
		log.Printf("已清理 %d 条 plex_activity 类型的遗留观看同步记录", n)
	}
}

// repairSessionPushItems 修正 v2.4.1 的记录缺陷：实时推送（sse/poll）成功时
// 明细的 success 误写为 0，页面点开后显示为无原因的「失败」。按 run 级
// 成功标志（to_trakt=1 且无错误）回填明细；幂等，修复后新数据不再命中。
func (s *TraktSyncStore) repairSessionPushItems() {
	_, _ = s.db.Exec(`UPDATE sync_run_items SET success = 1
		WHERE run_id IN (SELECT id FROM sync_runs
			WHERE trigger_type IN ('sse', 'poll') AND error IS NULL AND to_trakt = 1)`)
}

// TraktSyncRun 一次同步 run 的统计（对外载荷）。
// Title/Progress 仅实时推送（sse/poll）记录使用：单条推送的媒体与进度。
type TraktSyncRun struct {
	ID         int64
	StartedAt  string
	FinishedAt *string
	Trigger    string
	ToTrakt    int
	ToPlex     int
	Conflicts  int
	Unresolved int
	Skipped    int
	Error      *string
	Title      *string
	Progress   *float64
}

const syncRunSelect = `SELECT id, started_at, finished_at, trigger_type,
	to_trakt, to_plex, conflicts, unresolved, skipped, error, title, progress FROM sync_runs`

func scanSyncRun(row interface{ Scan(...any) error }) *TraktSyncRun {
	var r TraktSyncRun
	if err := row.Scan(&r.ID, &r.StartedAt, &r.FinishedAt, &r.Trigger,
		&r.ToTrakt, &r.ToPlex, &r.Conflicts, &r.Unresolved, &r.Skipped, &r.Error,
		&r.Title, &r.Progress); err != nil {
		return nil
	}
	return &r
}

// GetLastFinishedSyncRun 最近一次已完成的同步。
func (s *TraktSyncStore) GetLastFinishedSyncRun() *TraktSyncRun {
	return scanSyncRun(s.db.QueryRow(syncRunSelect + " WHERE finished_at IS NOT NULL ORDER BY id DESC LIMIT 1"))
}

// syncRunMeaningfulWHERE 有实际内容才算「有记录价值」：run 落下第一条
// 明细（含未识别/失败明细）即出现在列表——「同步一条显示一条」自然成立，
// 结束后仍有内容则保留；实时推送记录（title 非空）与失败 run 恒显示。
// 零变化的全量同步自始至终不出现（避免「出现又消失」的观感），只留应用日志。
const syncRunMeaningfulWHERE = `WHERE (to_trakt > 0 OR to_plex > 0 OR unresolved > 0 OR error IS NOT NULL
	OR title IS NOT NULL
	OR EXISTS (SELECT 1 FROM sync_run_items ri WHERE ri.run_id = sync_runs.id))`

// ListSyncRuns 按 limit/offset 列出有实际内容的同步历史（新→旧）。
func (s *TraktSyncStore) ListSyncRuns(limit, offset int) ([]TraktSyncRun, int, error) {
	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM sync_runs " + syncRunMeaningfulWHERE).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(syncRunSelect+" "+syncRunMeaningfulWHERE+" ORDER BY id DESC LIMIT ? OFFSET ?", limit, offset)
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

// RecordSessionPushRun 记录一次实时推送（sse/poll）为独立 run：
// 成功与失败都落记录（含失败原因与进度），供「Trakt 同步」页展示。
func (s *TraktSyncStore) RecordSessionPushRun(source string, item syncengine.RunItem, progress float64, success bool, errMsg string) error {
	now := traktSyncNow()
	var errArg any
	var titleArg any
	if errMsg != "" {
		errArg = errMsg
	}
	if item.Title != "" {
		titleArg = item.Title
	}
	res, err := s.db.Exec(
		`INSERT INTO sync_runs(started_at, finished_at, trigger_type, to_trakt, error, title, progress)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		now, now, source, boolInt(success), errArg, titleArg, progress)
	if err != nil {
		return err
	}
	runID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	item.Success = success
	if !success && item.Err == "" {
		item.Err = errMsg
	}
	return s.InsertRunItem(runID, item)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ── sync_run_items ──────────────────────────────────────────

// InsertRunItem 写一条逐键明细（忽略失败，明细不阻塞同步）。
func (s *TraktSyncStore) InsertRunItem(runID int64, item syncengine.RunItem) error {
	if runID == 0 {
		return nil
	}
	var (
		mediaType any
		anchor    any
		tmdbID    any
		season    any
		episode   any
		title     any
		errArg    any
	)
	if item.Key.Type != "" {
		mediaType = item.Key.Type
		_, a, id, _, _ := mediaKeyValues(item.Key)
		anchor = a
		tmdbID = id
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
	_, err := s.db.Exec(`INSERT INTO sync_run_items(run_id, media_type, anchor, tmdb_id, season, episode, title, direction, action, success, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		runID, mediaType, anchor, tmdbID, season, episode, title, item.Direction, item.Action, successInt, errArg)
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
	Anchor       string // 'tmdb' | 'tvdb'
	TmdbID       int64  // 锚 ID（Anchor='tvdb' 时为 tvdb id）
	Season       int
	Episode      int
	Title        *string
	ViewOffsetMS int64
	DurationMS   int64
	MaxProgress  float64
	Misses       int
	UpdatedAt    string
}

// Key 会话的媒体键（无身份时 TMDBID/TVDBID 均为 0）。
func (s TraktSyncSession) Key() watched.MediaKey {
	if s.Anchor == "tvdb" {
		return watched.MediaKey{Type: s.MediaType, TVDBID: s.TmdbID, Season: s.Season, Episode: s.Episode}
	}
	return watched.MediaKey{Type: s.MediaType, TMDBID: s.TmdbID, Season: s.Season, Episode: s.Episode}
}

const syncSessionSelect = `SELECT session_key, plex_ref, media_type, anchor, tmdb_id, season, episode,
	title, view_offset_ms, duration_ms, max_progress, misses, updated_at FROM sync_sessions`

func scanSyncSession(row interface{ Scan(...any) error }) *TraktSyncSession {
	var sess TraktSyncSession
	if err := row.Scan(&sess.SessionKey, &sess.PlexRef, &sess.MediaType, &sess.Anchor, &sess.TmdbID, &sess.Season,
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
	if sess.Anchor == "" {
		sess.Anchor = "tmdb"
	}
	if current == nil {
		_, _ = s.db.Exec(`INSERT INTO sync_sessions(
				session_key, plex_ref, media_type, anchor, tmdb_id, season, episode, title,
				view_offset_ms, duration_ms, max_progress, misses, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?)`,
			sess.SessionKey, sess.PlexRef, sess.MediaType, sess.Anchor, sess.TmdbID, sess.Season, sess.Episode, titleArg,
			sess.ViewOffsetMS, sess.DurationMS, sess.MaxProgress, now)
		return
	}
	_, _ = s.db.Exec(`UPDATE sync_sessions
		SET plex_ref = ?, media_type = ?, anchor = ?, tmdb_id = ?, season = ?, episode = ?, title = ?,
		    view_offset_ms = ?, duration_ms = ?, max_progress = ?, misses = 0, updated_at = ?
		WHERE session_key = ?`,
		sess.PlexRef, sess.MediaType, sess.Anchor, sess.TmdbID, sess.Season, sess.Episode, titleArg,
		sess.ViewOffsetMS, sess.DurationMS, sess.MaxProgress, now, sess.SessionKey)
}

// GetTraktShowCache 取逐剧观看进度缓存（剧级指纹 + 上次逐集结果）。
// 实现 trakt.ShowProgressCache。
func (s *TraktSyncStore) GetTraktShowCache(anchor string, anchorID int64) (plays int, lastWatchedAt, episodesJSON, fetchedAt string, ok bool) {
	var p int
	err := s.db.QueryRow("SELECT plays, last_watched_at, episodes, updated_at FROM trakt_show_cache WHERE anchor = ? AND anchor_id = ?",
		anchor, anchorID).Scan(&p, &lastWatchedAt, &episodesJSON, &fetchedAt)
	if err != nil {
		return 0, "", "", "", false
	}
	return p, lastWatchedAt, episodesJSON, fetchedAt, true
}

// SaveTraktShowCache 写逐剧观看进度缓存。实现 trakt.ShowProgressCache。
func (s *TraktSyncStore) SaveTraktShowCache(anchor string, anchorID int64, plays int, lastWatchedAt, episodesJSON string) {
	_, _ = s.db.Exec(`INSERT INTO trakt_show_cache(anchor, anchor_id, plays, last_watched_at, episodes, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(anchor, anchor_id) DO UPDATE SET
			plays = excluded.plays, last_watched_at = excluded.last_watched_at,
			episodes = excluded.episodes, updated_at = excluded.updated_at`,
		anchor, anchorID, plays, lastWatchedAt, episodesJSON, traktSyncNow())
}

// BackdateSession 设定会话观察行的更新时间与缺席计数（测试与数据修正用：
// 模拟「上一段进程生命周期遗留的僵尸行」）。
func (s *TraktSyncStore) BackdateSession(sessionKey string, at time.Time, misses int) error {
	_, err := s.db.Exec("UPDATE sync_sessions SET updated_at = ?, misses = ? WHERE session_key = ?",
		at.UTC().Format("2006-01-02T15:04:05.000000-07:00"), misses, sessionKey)
	return err
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

// ── sync_id_map（外部 ID 归一化，实现 watched.IDMapper）─────

// ResolveKey 把一组外部 ID 归并为规范键（tmdb 锚优先；剧集缺 tmdb 回退 tvdb），
// 并把新见 ID 并入映射。同一媒体的多条映射行（如一侧只见 tvdb、另一侧见
// tmdb+tvdb）会被合并为一行，保证两侧归并出同一个键。
// 实现 watched.IDMapper。
func (s *TraktSyncStore) ResolveKey(mediaType string, ids watched.MediaIDs, season, episode int) (watched.MediaKey, bool) {
	if ids.TMDB <= 0 && ids.TVDB <= 0 && ids.IMDB == "" {
		return watched.MediaKey{}, false
	}
	type row struct {
		sid    int64
		tmdbID int64
		tvdbID int64
		imdbID string
	}
	var matched []row
	rows, err := s.db.Query(`SELECT sid, tmdb_id, tvdb_id, imdb_id FROM sync_id_map WHERE media_type = ?
		AND (tmdb_id = ? OR tvdb_id = ? OR imdb_id = ?)`,
		mediaType, ids.TMDB, ids.TVDB, ids.IMDB)
	if err == nil {
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.sid, &r.tmdbID, &r.tvdbID, &r.imdbID); err == nil {
				matched = append(matched, r)
			}
		}
		rows.Close()
	}
	if err != nil || len(matched) > 1 {
		// 多行同指一媒体（历史数据分叉）：保留第一行，删除其余后合并
		for _, r := range matched[1:] {
			_, _ = s.db.Exec("DELETE FROM sync_id_map WHERE sid = ?", r.sid)
		}
	}
	if len(matched) == 0 {
		res, ierr := s.db.Exec(`INSERT INTO sync_id_map(media_type, tmdb_id, tvdb_id, imdb_id) VALUES (?, ?, ?, ?)`,
			mediaType, ids.TMDB, ids.TVDB, ids.IMDB)
		if ierr != nil {
			return watched.MediaKey{}, false
		}
		_ = res
	} else {
		_, _ = s.db.Exec(`UPDATE sync_id_map SET
			tmdb_id = CASE WHEN tmdb_id = 0 THEN ? ELSE tmdb_id END,
			tvdb_id = CASE WHEN tvdb_id = 0 THEN ? ELSE tvdb_id END,
			imdb_id = CASE WHEN imdb_id = '' THEN ? ELSE imdb_id END
			WHERE sid = ?`,
			ids.TMDB, ids.TVDB, ids.IMDB, matched[0].sid)
	}
	// 以合并后的完整 ID 集规范成键
	var merged watched.MediaIDs
	if len(matched) > 0 {
		merged = watched.MediaIDs{TMDB: matched[0].tmdbID, TVDB: matched[0].tvdbID, IMDB: matched[0].imdbID}
	}
	if merged.TMDB == 0 {
		merged.TMDB = ids.TMDB
	}
	if merged.TVDB == 0 {
		merged.TVDB = ids.TVDB
	}
	if merged.IMDB == "" {
		merged.IMDB = ids.IMDB
	}
	return watched.CanonicalKey(mediaType, merged, season, episode)
}

// IDsOf 返回键对应的全部已知 ID（组多 ID 推送载荷用）。实现 watched.IDMapper。
func (s *TraktSyncStore) IDsOf(key watched.MediaKey) watched.MediaIDs {
	var ids watched.MediaIDs
	if key.TVDBID > 0 {
		if err := s.db.QueryRow("SELECT tmdb_id, tvdb_id, imdb_id FROM sync_id_map WHERE media_type = ? AND tvdb_id = ?",
			key.Type, key.TVDBID).Scan(&ids.TMDB, &ids.TVDB, &ids.IMDB); err != nil {
			return watched.MediaIDs{TVDB: key.TVDBID}
		}
		return ids
	}
	if err := s.db.QueryRow("SELECT tmdb_id, tvdb_id, imdb_id FROM sync_id_map WHERE media_type = ? AND tmdb_id = ?",
		key.Type, key.TMDBID).Scan(&ids.TMDB, &ids.TVDB, &ids.IMDB); err != nil {
		return watched.MediaIDs{TMDB: key.TMDBID}
	}
	return ids
}

// ── sync_ignores（手动忽略清单）──────────────────────────────

// TraktSyncIgnore 一条忽略记录（媒体级：整部电影/整部剧）。
type TraktSyncIgnore struct {
	MediaType string
	Anchor    string // 'tmdb' | 'tvdb'
	TmdbID    int64  // 锚 ID
	Title     *string
	Reason    *string
	CreatedAt string
}

// AddSyncIgnore 添加/更新媒体级忽略（upsert，重复添加仅刷新标题与原因）。
func (s *TraktSyncStore) AddSyncIgnore(mediaType, anchor string, tmdbID int64, title, reason string) error {
	if anchor != "tvdb" {
		anchor = "tmdb"
	}
	var titleArg, reasonArg any
	if title != "" {
		titleArg = title
	}
	if reason != "" {
		reasonArg = reason
	}
	_, err := s.db.Exec(`INSERT INTO sync_ignores(media_type, anchor, tmdb_id, title, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(media_type, anchor, tmdb_id) DO UPDATE SET title = excluded.title, reason = excluded.reason`,
		mediaType, anchor, tmdbID, titleArg, reasonArg, traktSyncNow())
	return err
}

// RemoveSyncIgnore 解除忽略，媒体重新参与同步。
func (s *TraktSyncStore) RemoveSyncIgnore(mediaType, anchor string, tmdbID int64) error {
	if anchor != "tvdb" {
		anchor = "tmdb"
	}
	_, err := s.db.Exec("DELETE FROM sync_ignores WHERE media_type = ? AND anchor = ? AND tmdb_id = ?", mediaType, anchor, tmdbID)
	return err
}

// ListSyncIgnores 全部忽略记录（新→旧）。
func (s *TraktSyncStore) ListSyncIgnores() []TraktSyncIgnore {
	rows, err := s.db.Query("SELECT media_type, anchor, tmdb_id, title, reason, created_at FROM sync_ignores ORDER BY created_at DESC")
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []TraktSyncIgnore{}
	for rows.Next() {
		var it TraktSyncIgnore
		if err := rows.Scan(&it.MediaType, &it.Anchor, &it.TmdbID, &it.Title, &it.Reason, &it.CreatedAt); err != nil {
			return nil
		}
		out = append(out, it)
	}
	return out
}

// AllIgnores 忽略清单的引擎视图：媒体级 MediaKey（Season/Episode 为零）集合。
// 实现 syncengine.IgnoreProvider。
func (s *TraktSyncStore) AllIgnores() (map[watched.MediaKey]bool, error) {
	rows, err := s.db.Query("SELECT media_type, anchor, tmdb_id FROM sync_ignores")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[watched.MediaKey]bool{}
	for rows.Next() {
		var (
			k      watched.MediaKey
			anchor string
			id     int64
		)
		if err := rows.Scan(&k.Type, &anchor, &id); err != nil {
			return nil, err
		}
		if anchor == "tvdb" {
			k.TVDBID = id
		} else {
			k.TMDBID = id
		}
		out[k] = true
	}
	return out, rows.Err()
}

// IsIgnoredKey 媒体是否被忽略（实时推送等非引擎路径使用）。
func (s *TraktSyncStore) IsIgnoredKey(key watched.MediaKey) bool {
	mediaType, anchor, anchorID, _, _ := mediaKeyValues(key)
	var one int
	return s.db.QueryRow("SELECT 1 FROM sync_ignores WHERE media_type = ? AND anchor = ? AND tmdb_id = ?",
		mediaType, anchor, anchorID).Scan(&one) == nil
}

// TraktFailingMedia 反复同步失败的媒体聚合（忽略候选）。
type TraktFailingMedia struct {
	MediaType string
	Anchor    string
	TmdbID    int64
	Title     *string
	Fails     int
	LastError *string
	LastAt    string
}

// FailingMediaCandidates 近 days 天失败 ≥2 次且未被忽略的媒体，按失败次数降序——
// 持续失败多为 Trakt 侧数据缺映射（id 无法识别），建议用户手动忽略。
func (s *TraktSyncStore) FailingMediaCandidates(days int) []TraktFailingMedia {
	cutoff := time.Now().AddDate(0, 0, -days).UTC().Format("2006-01-02T15:04:05.000000-07:00")
	rows, err := s.db.Query(`
		SELECT ri.media_type, COALESCE(ri.anchor, 'tmdb'), ri.tmdb_id, MAX(ri.title), COUNT(*), MAX(ri.error), MAX(r.started_at)
		FROM sync_run_items ri
		JOIN sync_runs r ON r.id = ri.run_id
		WHERE ri.success = 0 AND ri.media_type IS NOT NULL AND ri.tmdb_id IS NOT NULL
			AND r.started_at >= ?
			AND NOT EXISTS (SELECT 1 FROM sync_ignores ig
				WHERE ig.media_type = ri.media_type AND ig.anchor = COALESCE(ri.anchor, 'tmdb') AND ig.tmdb_id = ri.tmdb_id)
		GROUP BY ri.media_type, COALESCE(ri.anchor, 'tmdb'), ri.tmdb_id
		HAVING COUNT(*) >= 2
		ORDER BY COUNT(*) DESC
		LIMIT 50`, cutoff)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []TraktFailingMedia{}
	for rows.Next() {
		var c TraktFailingMedia
		if err := rows.Scan(&c.MediaType, &c.Anchor, &c.TmdbID, &c.Title, &c.Fails, &c.LastError, &c.LastAt); err != nil {
			return nil
		}
		out = append(out, c)
	}
	return out
}
