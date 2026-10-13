// logstore.go —— 应用日志存储（对应 webui/log_store.py）。
// 独立 app_logs.db，WAL 模式，按保留天数清理。
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const logDDL = `
CREATE TABLE IF NOT EXISTS app_logs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	ts TEXT NOT NULL,
	category TEXT NOT NULL DEFAULT '',
	event TEXT NOT NULL DEFAULT '',
	level TEXT NOT NULL DEFAULT 'INFO',
	message TEXT NOT NULL DEFAULT '',
	details TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_logs_ts ON app_logs(ts);
CREATE INDEX IF NOT EXISTS idx_logs_category ON app_logs(category);
CREATE INDEX IF NOT EXISTS idx_logs_level ON app_logs(level);
`

// LogStore 应用日志存储。
type LogStore struct {
	db            *sql.DB
	mu            sync.Mutex
	retentionDays int
}

// OpenLogStore 打开日志库（directory 下 app_logs.db）。
// legacyPath 指向旧 JSONL 文件时执行一次性迁移。
func OpenLogStore(directory string, retentionDays int, legacyPath string) (*LogStore, error) {
	if retentionDays < 1 {
		retentionDays = 7
	}
	_ = os.MkdirAll(directory, 0o755)
	dbPath := filepath.Join(directory, "app_logs.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(logDDL); err != nil {
		db.Close()
		return nil, err
	}
	s := &LogStore{db: db, retentionDays: retentionDays}
	if legacyPath != "" {
		if _, err := os.Stat(legacyPath); err == nil {
			s.migrateJSONL(legacyPath)
		}
	}
	return s, nil
}

// Close 关闭。
func (s *LogStore) Close() error { return s.db.Close() }

// SetRetentionDays 更新保留天数。
func (s *LogStore) SetRetentionDays(days int) {
	if days < 1 {
		days = 7
	}
	s.mu.Lock()
	s.retentionDays = days
	s.mu.Unlock()
}

// migrateJSONL 迁移旧 JSONL 日志后删除源文件。
func (s *LogStore) migrateJSONL(legacyPath string) {
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		return
	}
	type legacyItem map[string]any
	var args []any
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var item legacyItem
		if json.Unmarshal([]byte(line), &item) != nil {
			continue
		}
		ts, _ := item["ts"].(string)
		if ts == "" {
			ts = time.Now().UTC().Format(time.RFC3339)
		}
		category, _ := item["category"].(string)
		event, _ := item["event"].(string)
		level, _ := item["level"].(string)
		if level == "" {
			level = "INFO"
		}
		message, _ := item["message"].(string)
		details, _ := json.Marshal(item["details"])
		args = append(args, ts, category, event, level, message, string(details))
	}
	if len(args) > 0 {
		_, _ = s.db.Exec(
			"INSERT INTO app_logs (ts, category, event, level, message, details) VALUES (?, ?, ?, ?, ?, ?)",
			args...)
	}
	_ = os.Remove(legacyPath)
}

// LogEntry 日志条目。
type LogEntry struct {
	ID       int64          `json:"id"`
	Ts       string         `json:"ts"`
	Category string         `json:"category"`
	Event    string         `json:"event"`
	Level    string         `json:"level"`
	Message  string         `json:"message"`
	Details  map[string]any `json:"details"`
}

// Write 写入一条日志。
func (s *LogStore) Write(category, event, level, message string, details map[string]any) LogEntry {
	ts := time.Now().UTC().Format("2006-01-02T15:04:05.000000-07:00")
	if level == "" {
		level = "INFO"
	}
	level = strings.ToUpper(level)
	if details == nil {
		details = map[string]any{}
	}
	detailsJSON, _ := json.Marshal(details)
	s.mu.Lock()
	_, _ = s.db.Exec(
		"INSERT INTO app_logs (ts, category, event, level, message, details) VALUES (?, ?, ?, ?, ?, ?)",
		ts, category, event, level, message, string(detailsJSON))
	s.mu.Unlock()
	return LogEntry{Ts: ts, Category: category, Event: event, Level: level, Message: message, Details: details}
}

// Read 读取日志（id 倒序，limit 1~1000）。
func (s *LogStore) Read(limit int, category, level string, sinceID int64) []LogEntry {
	s.mu.Lock()
	s.cleanupExpired()
	s.mu.Unlock()

	if limit < 1 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	conditions := []string{"id > ?"}
	params := []any{sinceID}
	if category != "" {
		conditions = append(conditions, "category = ?")
		params = append(params, category)
	}
	if level != "" {
		conditions = append(conditions, "level = ?")
		params = append(params, strings.ToUpper(level))
	}
	query := fmt.Sprintf(
		"SELECT id, ts, category, event, level, message, details FROM app_logs WHERE %s ORDER BY id DESC LIMIT ?",
		strings.Join(conditions, " AND "))
	rows, err := s.db.Query(query, append(params, limit)...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var items []LogEntry
	for rows.Next() {
		var e LogEntry
		var detailsJSON string
		if err := rows.Scan(&e.ID, &e.Ts, &e.Category, &e.Event, &e.Level, &e.Message, &detailsJSON); err != nil {
			continue
		}
		e.Details = map[string]any{}
		_ = json.Unmarshal([]byte(detailsJSON), &e.Details)
		items = append(items, e)
	}
	return items
}

// Summary 近期日志摘要。
func (s *LogStore) Summary(recentLimit int) map[string]any {
	if recentLimit <= 0 {
		recentLimit = 500
	}
	records := s.Read(recentLimit, "", "", 0)
	byCategory := map[string]int{}
	byLevel := map[string]int{}
	for _, item := range records {
		key := item.Category
		if key == "" {
			key = "unknown"
		}
		byCategory[key]++
		lv := item.Level
		if lv == "" {
			lv = "INFO"
		}
		byLevel[lv]++
	}
	s.mu.Lock()
	retention := s.retentionDays
	s.mu.Unlock()
	return map[string]any{
		"total":         len(records),
		"byCategory":    byCategory,
		"byLevel":       byLevel,
		"retentionDays": retention,
	}
}

// MaxID 当前最大日志 ID。
func (s *LogStore) MaxID() int64 {
	var id sql.NullInt64
	_ = s.db.QueryRow("SELECT MAX(id) FROM app_logs").Scan(&id)
	return id.Int64
}

func (s *LogStore) cleanupExpired() {
	cutoff := time.Now().AddDate(0, 0, -s.retentionDays).Format("2006-01-02T15:04:05")
	_, _ = s.db.Exec("DELETE FROM app_logs WHERE ts < ?", cutoff)
}
