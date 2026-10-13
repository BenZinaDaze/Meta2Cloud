// u115auto.go —— 115 云下载自动整理状态存储（对应 webui/u115_auto_organize_store.py）。
// 记录已观察到的云下载任务完成状态，避免重复触发整理流程。
package store

import (
	"database/sql"
	"os"
	"time"
)

const u115AutoDDL = `
CREATE TABLE IF NOT EXISTS u115_auto_organize_task_state (
	task_key          TEXT PRIMARY KEY,
	info_hash         TEXT NOT NULL DEFAULT '',
	task_name         TEXT NOT NULL DEFAULT '',
	status            INTEGER NOT NULL DEFAULT 0,
	percent_done      REAL NOT NULL DEFAULT 0,
	wp_path_id        TEXT,
	file_id           TEXT,
	first_seen_at     TEXT NOT NULL,
	last_seen_at      TEXT NOT NULL,
	finished_seen_at  TEXT,
	triggered_at      TEXT
);
`

// U115AutoTaskState 任务观察状态。
type U115AutoTaskState struct {
	TaskKey        string
	InfoHash       string
	TaskName       string
	Status         int
	PercentDone    float64
	WpPathID       *string
	FileID         *string
	FirstSeenAt    string
	LastSeenAt     string
	FinishedSeenAt *string
	TriggeredAt    *string
}

// U115AutoStore 自动整理状态存储。
type U115AutoStore struct {
	db *sql.DB
}

// OpenU115AutoStore 打开（独立 u115_auto_organize.db）。
func OpenU115AutoStore(dbPath string) (*U115AutoStore, error) {
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
	if _, err := db.Exec(u115AutoDDL); err != nil {
		db.Close()
		return nil, err
	}
	return &U115AutoStore{db: db}, nil
}

// Close 关闭。
func (s *U115AutoStore) Close() error { return s.db.Close() }

const u115AutoSelect = `SELECT task_key, info_hash, task_name, status, percent_done, wp_path_id, file_id,
	first_seen_at, last_seen_at, finished_seen_at, triggered_at
	FROM u115_auto_organize_task_state`

func scanU115Auto(row *sql.Row) *U115AutoTaskState {
	var st U115AutoTaskState
	err := row.Scan(&st.TaskKey, &st.InfoHash, &st.TaskName, &st.Status, &st.PercentDone,
		&st.WpPathID, &st.FileID, &st.FirstSeenAt, &st.LastSeenAt, &st.FinishedSeenAt, &st.TriggeredAt)
	if err != nil {
		return nil
	}
	return &st
}

// Get 按任务键读取状态。
func (s *U115AutoStore) Get(taskKey string) *U115AutoTaskState {
	return scanU115Auto(s.db.QueryRow(u115AutoSelect+" WHERE task_key = ?", taskKey))
}

// UpsertObservation 写入/更新一次任务观察。
func (s *U115AutoStore) UpsertObservation(taskKey, infoHash, taskName string, status int, percentDone float64, wpPathID, fileID *string, isFinished bool) *U115AutoTaskState {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000000-07:00")
	current := s.Get(taskKey)
	if current == nil {
		var finishedSeenAt *string
		if isFinished {
			finishedSeenAt = &now
		}
		_, _ = s.db.Exec(`INSERT INTO u115_auto_organize_task_state(
			task_key, info_hash, task_name, status, percent_done, wp_path_id, file_id,
			first_seen_at, last_seen_at, finished_seen_at, triggered_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
			taskKey, infoHash, taskName, status, percentDone, wpPathID, fileID, now, now, finishedSeenAt)
		return s.Get(taskKey)
	}
	finishedSeenAt := current.FinishedSeenAt
	if isFinished && finishedSeenAt == nil {
		finishedSeenAt = &now
	}
	if !isFinished {
		finishedSeenAt = nil
	}
	_, _ = s.db.Exec(`UPDATE u115_auto_organize_task_state
		SET info_hash = ?, task_name = ?, status = ?, percent_done = ?, wp_path_id = ?, file_id = ?,
		    last_seen_at = ?, finished_seen_at = ?
		WHERE task_key = ?`,
		infoHash, taskName, status, percentDone, wpPathID, fileID, now, finishedSeenAt, taskKey)
	return s.Get(taskKey)
}

// MarkTriggered 标记已触发整理。
func (s *U115AutoStore) MarkTriggered(taskKey string) {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000000-07:00")
	_, _ = s.db.Exec(
		"UPDATE u115_auto_organize_task_state SET triggered_at = ? WHERE task_key = ?", now, taskKey)
}

// GetLastTriggered 最近触发的任务。
func (s *U115AutoStore) GetLastTriggered() *U115AutoTaskState {
	return scanU115Auto(s.db.QueryRow(
		u115AutoSelect + " WHERE triggered_at IS NOT NULL ORDER BY triggered_at DESC LIMIT 1"))
}
