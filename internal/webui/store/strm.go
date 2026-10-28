// strm.go —— STRM 生成与 302 播放相关存储（独立 strm.db，不影响 library.db）。
// 记录网盘文件 ↔ 本地 .strm 文件映射，以及每次同步的 run 级历史与文件级明细。
package store

import (
	"database/sql"
	"log"
	"os"
	"time"
)

const strmDDL = `
CREATE TABLE IF NOT EXISTS strm_files (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	pick_code   TEXT    NOT NULL,
	file_id     TEXT    NOT NULL,
	pan_path    TEXT    NOT NULL,
	local_path  TEXT    NOT NULL UNIQUE,
	file_name   TEXT    NOT NULL,
	size        INTEGER,
	sync_time   TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_strm_files_pick_code ON strm_files(pick_code);

CREATE TABLE IF NOT EXISTS strm_sync_runs (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	started_at    TEXT NOT NULL,
	finished_at   TEXT,
	trigger_type  TEXT NOT NULL DEFAULT 'manual',
	created       INTEGER NOT NULL DEFAULT 0,
	updated       INTEGER NOT NULL DEFAULT 0,
	removed       INTEGER NOT NULL DEFAULT 0,
	skipped       INTEGER NOT NULL DEFAULT 0,
	error         TEXT
);

CREATE TABLE IF NOT EXISTS strm_sync_files (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id     INTEGER NOT NULL,
	file_name  TEXT,
	pan_path   TEXT,
	local_path TEXT,
	action     TEXT NOT NULL,
	success    INTEGER NOT NULL DEFAULT 1,
	error      TEXT
);

CREATE INDEX IF NOT EXISTS idx_strm_sync_files_run ON strm_sync_files(run_id);

CREATE TABLE IF NOT EXISTS strm_playback_log (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	played_at   TEXT    NOT NULL,
	pick_code   TEXT    NOT NULL,
	file_name   TEXT,
	user_agent  TEXT,
	cache_hit   INTEGER NOT NULL DEFAULT 0,
	duration_ms INTEGER NOT NULL DEFAULT 0,
	status      INTEGER NOT NULL,
	error       TEXT
);

CREATE INDEX IF NOT EXISTS idx_strm_playback_played_at ON strm_playback_log(played_at DESC);
CREATE INDEX IF NOT EXISTS idx_strm_playback_pick ON strm_playback_log(pick_code);
`

// StrmFile 一条网盘文件 ↔ 本地 .strm 映射。
type StrmFile struct {
	ID        int64
	PickCode  string
	FileID    string
	PanPath   string
	LocalPath string
	FileName  string
	Size      *int64
	SyncTime  string
}

// StrmSyncRun 一次同步的统计。
type StrmSyncRun struct {
	ID         int64
	StartedAt  string
	FinishedAt *string
	Trigger    string
	Created    int
	Updated    int
	Removed    int
	Skipped    int
	Error      *string
}

// StrmSyncFile 同步明细中的一条文件记录。
type StrmSyncFile struct {
	ID        int64
	RunID     int64
	FileName  *string
	PanPath   *string
	LocalPath *string
	Action    string
	Success   bool
	Error     *string
}

// StrmStore STRM 存储独立库。
type StrmStore struct {
	db *sql.DB
}

// OpenStrmStore 打开独立 strm.db。
func OpenStrmStore(dbPath string) (*StrmStore, error) {
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
	if _, err := db.Exec(strmDDL); err != nil {
		db.Close()
		return nil, err
	}
	s := &StrmStore{db: db}
	s.failStaleRunningRuns()
	return s, nil
}

// Close 关闭。
func (s *StrmStore) Close() error { return s.db.Close() }

func strmNow() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000000-07:00") }

// ── strm_files ──────────────────────────────────────────────

const strmFileSelect = `SELECT id, pick_code, file_id, pan_path, local_path, file_name, size, sync_time FROM strm_files`

func scanStrmFile(row interface{ Scan(...any) error }) *StrmFile {
	var f StrmFile
	if err := row.Scan(&f.ID, &f.PickCode, &f.FileID, &f.PanPath, &f.LocalPath, &f.FileName, &f.Size, &f.SyncTime); err != nil {
		return nil
	}
	return &f
}

// GetStrmFileByLocalPath 按本地路径读取。
func (s *StrmStore) GetStrmFileByLocalPath(localPath string) *StrmFile {
	return scanStrmFile(s.db.QueryRow(strmFileSelect+" WHERE local_path = ?", localPath))
}

// CountStrmFiles 映射总数。
func (s *StrmStore) CountStrmFiles() int {
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM strm_files").Scan(&n); err != nil {
		return 0
	}
	return n
}

// ListAllStrmFiles 全量列出 STRM 记录（孤儿对账用）。
func (s *StrmStore) ListAllStrmFiles() []StrmFile {
	rows, err := s.db.Query(strmFileSelect)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []StrmFile{}
	for rows.Next() {
		if f := scanStrmFile(rows); f != nil {
			out = append(out, *f)
		}
	}
	return out
}

// UpsertStrmFile 按 local_path 幂等写入映射。
// 返回动作："created"（新增）/ "updated"（内容变化重写）/ "skipped"（无变化）。
func (s *StrmStore) UpsertStrmFile(f StrmFile) string {
	now := strmNow()
	current := s.GetStrmFileByLocalPath(f.LocalPath)
	if current == nil {
		_, _ = s.db.Exec(`INSERT INTO strm_files(pick_code, file_id, pan_path, local_path, file_name, size, sync_time)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			f.PickCode, f.FileID, f.PanPath, f.LocalPath, f.FileName, f.Size, now)
		return "created"
	}
	if current.PickCode == f.PickCode && current.FileID == f.FileID &&
		current.PanPath == f.PanPath && current.FileName == f.FileName &&
		sizeEqual(current.Size, f.Size) {
		return "skipped"
	}
	_, _ = s.db.Exec(`UPDATE strm_files
		SET pick_code = ?, file_id = ?, pan_path = ?, file_name = ?, size = ?, sync_time = ?
		WHERE local_path = ?`,
		f.PickCode, f.FileID, f.PanPath, f.FileName, f.Size, now, f.LocalPath)
	return "updated"
}

func sizeEqual(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// DeleteStrmFile 删除映射记录。
func (s *StrmStore) DeleteStrmFile(id int64) {
	_, _ = s.db.Exec("DELETE FROM strm_files WHERE id = ?", id)
}

// ListStrmFilesByLocalPrefix 列出本地路径带指定前缀的映射（用于按映射清理失效条目）。
func (s *StrmStore) ListStrmFilesByLocalPrefix(prefix string) []StrmFile {
	rows, err := s.db.Query(strmFileSelect+" WHERE local_path LIKE ? ORDER BY local_path", prefix+"%")
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []StrmFile{}
	for rows.Next() {
		if f := scanStrmFile(rows); f != nil {
			out = append(out, *f)
		}
	}
	return out
}

// ── 同步历史 ────────────────────────────────────────────────

// StartSyncRun 建一次同步 run，返回 run ID。
func (s *StrmStore) StartSyncRun(trigger string) int64 {
	res, err := s.db.Exec(
		"INSERT INTO strm_sync_runs(started_at, trigger_type) VALUES (?, ?)", strmNow(), trigger)
	if err != nil {
		return 0
	}
	id, _ := res.LastInsertId()
	return id
}

// FinishSyncRun 收尾一次同步 run。
func (s *StrmStore) FinishSyncRun(runID int64, created, updated, removed, skipped int, errMsg string) {
	var errArg any
	if errMsg != "" {
		errArg = errMsg
	}
	_, _ = s.db.Exec(`UPDATE strm_sync_runs
		SET finished_at = ?, created = ?, updated = ?, removed = ?, skipped = ?, error = ?
		WHERE id = ?`, strmNow(), created, updated, removed, skipped, errArg, runID)
}

// failStaleRunningRuns 把上次进程遗留的「进行中」同步记录收尾为失败。
// 进程崩溃/被杀时无人调用 FinishSyncRun，这些行会永远停留在进行中；存储打开即清理。
func (s *StrmStore) failStaleRunningRuns() {
	res, err := s.db.Exec(`UPDATE strm_sync_runs
		SET finished_at = ?, error = '进程重启中断，同步未完成'
		WHERE finished_at IS NULL`, strmNow())
	if err != nil {
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("已将 %d 条中断的同步记录标记为失败", n)
	}
}

// InsertSyncFileDetail 写一条文件级明细（忽略失败，明细不阻塞同步）。
func (s *StrmStore) InsertSyncFileDetail(runID int64, fileName, panPath, localPath, action string, success bool, errMsg string) {
	if runID == 0 {
		return
	}
	var nameArg, panArg, pathArg, errArg any
	if fileName != "" {
		nameArg = fileName
	}
	if panPath != "" {
		panArg = panPath
	}
	if localPath != "" {
		pathArg = localPath
	}
	if errMsg != "" {
		errArg = errMsg
	}
	successInt := 0
	if success {
		successInt = 1
	}
	_, _ = s.db.Exec(`INSERT INTO strm_sync_files(run_id, file_name, pan_path, local_path, action, success, error)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, runID, nameArg, panArg, pathArg, action, successInt, errArg)
}

// GetLastFinishedSyncRun 最近一次已完成的同步。
func (s *StrmStore) GetLastFinishedSyncRun() *StrmSyncRun {
	return scanStrmRun(s.db.QueryRow(
		"SELECT id, started_at, finished_at, trigger_type, created, updated, removed, skipped, error FROM strm_sync_runs WHERE finished_at IS NOT NULL ORDER BY id DESC LIMIT 1"))
}

// ListSyncRuns 分页列出同步历史（新→旧）。
func (s *StrmStore) ListSyncRuns(limit, offset int) ([]StrmSyncRun, int) {
	var total int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM strm_sync_runs").Scan(&total)
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.Query(`SELECT id, started_at, finished_at, trigger_type, created, updated, removed, skipped, error
		FROM strm_sync_runs ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, total
	}
	defer rows.Close()
	out := []StrmSyncRun{}
	for rows.Next() {
		if r := scanStrmRun(rows); r != nil {
			out = append(out, *r)
		}
	}
	return out, total
}

func scanStrmRun(row interface{ Scan(...any) error }) *StrmSyncRun {
	var r StrmSyncRun
	if err := row.Scan(&r.ID, &r.StartedAt, &r.FinishedAt, &r.Trigger,
		&r.Created, &r.Updated, &r.Removed, &r.Skipped, &r.Error); err != nil {
		return nil
	}
	return &r
}

// ListSyncFiles 列出某次同步的文件明细。
func (s *StrmStore) ListSyncFiles(runID int64, limit int) []StrmSyncFile {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.db.Query(`SELECT id, run_id, file_name, pan_path, local_path, action, success, error
		FROM strm_sync_files WHERE run_id = ? ORDER BY id LIMIT ?`, runID, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []StrmSyncFile{}
	for rows.Next() {
		var f StrmSyncFile
		if err := rows.Scan(&f.ID, &f.RunID, &f.FileName, &f.PanPath, &f.LocalPath, &f.Action, &f.Success, &f.Error); err != nil {
			continue
		}
		out = append(out, f)
	}
	return out
}

// ── 播放记录 ────────────────────────────────────────────────

// playbackRetention 播放记录保留上限（超出时裁剪最旧记录）。
const playbackRetention = 5000

// StrmPlaybackLog 一次 302 播放跳转的记录。
// PanPath / LocalPath 为查询时按 pick_code 关联 strm_files 得到的播放内容路径，记录可能已被清理。
type StrmPlaybackLog struct {
	ID         int64
	PlayedAt   string
	PickCode   string
	FileName   *string
	UserAgent  *string
	CacheHit   bool
	DurationMs int
	Status     int
	Error      *string
	// 关联信息（列表查询时填充）
	PanPath   *string
	LocalPath *string
}

// StrmPlaybackStats 播放统计。
type StrmPlaybackStats struct {
	Total        int     `json:"total"`
	Today        int     `json:"today"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	CacheHitRate float64 `json:"cache_hit_rate"`
	Clients      int     `json:"distinct_clients"`
}

// InsertPlaybackLog 写一条播放记录（error 非空时记录失败原因）。
func (s *StrmStore) InsertPlaybackLog(pickCode string, fileName, userAgent *string, cacheHit bool, durationMs, status int, errMsg string) {
	successInt := 0
	if cacheHit {
		successInt = 1
	}
	var errArg any
	if errMsg != "" {
		errArg = errMsg
	}
	_, _ = s.db.Exec(`INSERT INTO strm_playback_log(played_at, pick_code, file_name, user_agent, cache_hit, duration_ms, status, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		strmNow(), pickCode, fileName, userAgent, successInt, durationMs, status, errArg)
	s.maybePrunePlayback()
}

// maybePrunePlayback 每 100 次写入裁剪一次超限旧记录（COUNT 概率性触发，成本低）。
func (s *StrmStore) maybePrunePlayback() {
	if time.Now().UnixNano()%100 != 0 {
		return
	}
	_, _ = s.db.Exec(`DELETE FROM strm_playback_log WHERE id NOT IN (
		SELECT id FROM strm_playback_log ORDER BY id DESC LIMIT ?)`, playbackRetention)
}

// ListPlaybackLogs 分页列出播放记录（新→旧），按 pick_code 关联映射路径。
func (s *StrmStore) ListPlaybackLogs(limit, offset int) ([]StrmPlaybackLog, int) {
	var total int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM strm_playback_log").Scan(&total)
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.Query(`SELECT p.id, p.played_at, p.pick_code, p.file_name, p.user_agent,
			p.cache_hit, p.duration_ms, p.status, p.error, f.pan_path, f.local_path
		FROM strm_playback_log p
		LEFT JOIN strm_files f ON f.pick_code = p.pick_code
		ORDER BY p.id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, total
	}
	defer rows.Close()
	out := []StrmPlaybackLog{}
	for rows.Next() {
		var l StrmPlaybackLog
		var cacheHit int
		if err := rows.Scan(&l.ID, &l.PlayedAt, &l.PickCode, &l.FileName, &l.UserAgent,
			&cacheHit, &l.DurationMs, &l.Status, &l.Error, &l.PanPath, &l.LocalPath); err != nil {
			continue
		}
		l.CacheHit = cacheHit == 1
		out = append(out, l)
	}
	return out, total
}

// GetPlaybackStats 播放统计：总数、当日（以本地时区自然日）、平均直链延迟（仅实时获取）、缓存命中率。
func (s *StrmStore) GetPlaybackStats() StrmPlaybackStats {
	stats := StrmPlaybackStats{}
	_ = s.db.QueryRow("SELECT COUNT(*) FROM strm_playback_log").Scan(&stats.Total)
	// 本地自然日零点（用户视角的"今天"）转 UTC 存储格式比较
	now := time.Now()
	localDayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	dayStartUTC := localDayStart.UTC().Format("2006-01-02T15:04:05.000000-07:00")
	_ = s.db.QueryRow("SELECT COUNT(*) FROM strm_playback_log WHERE played_at >= ?", dayStartUTC).Scan(&stats.Today)
	_ = s.db.QueryRow(`SELECT COALESCE(AVG(duration_ms), 0) FROM strm_playback_log
		WHERE cache_hit = 0 AND status = 302`).Scan(&stats.AvgLatencyMs)
	var redirects, cacheHits int
	_ = s.db.QueryRow("SELECT COUNT(*), COALESCE(SUM(cache_hit), 0) FROM strm_playback_log WHERE status = 302").Scan(&redirects, &cacheHits)
	if redirects > 0 {
		stats.CacheHitRate = float64(cacheHits) / float64(redirects)
	}
	_ = s.db.QueryRow(`SELECT COUNT(DISTINCT user_agent) FROM strm_playback_log WHERE user_agent IS NOT NULL`).Scan(&stats.Clients)
	return stats
}
