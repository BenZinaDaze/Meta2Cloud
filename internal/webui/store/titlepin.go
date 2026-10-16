// titlepin.go —— 媒体标题钉扎存储（media_title_pins 表，与 library.db 同库）。
//
// TMDB 译名可能随时间变化；钉扎表记录每个媒体首次整理时的目录名，
// 后续整理沿用该名字，避免译名变更产生重复目录。
package store

import (
	"database/sql"
	"errors"
	"log"
)

// TitlePin 标题钉扎记录。
type TitlePin struct {
	MediaType   string `json:"media_type"`
	TmdbID      int64  `json:"tmdb_id"`
	PinnedTitle string `json:"pinned_title"`
	PinnedYear  string `json:"pinned_year"`
	Source      string `json:"source"` // ingest_history | tmdb | manual
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// DuplicateFolderGroup 同一媒体关联的多个网盘目录（历史译名差异导致）。
type DuplicateFolderGroup struct {
	MediaType string
	TmdbID    int64
	FolderIDs []string
}

// GetTitlePin 读取钉扎记录（无记录返回 false）。
func (s *LibraryStore) GetTitlePin(mediaType string, tmdbID int64) (TitlePin, bool) {
	row := s.db.QueryRow(`SELECT media_type, tmdb_id, pinned_title, pinned_year, source, created_at, updated_at
		FROM media_title_pins WHERE media_type = ? AND tmdb_id = ?`, mediaType, tmdbID)
	var p TitlePin
	if err := row.Scan(&p.MediaType, &p.TmdbID, &p.PinnedTitle, &p.PinnedYear, &p.Source, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return TitlePin{}, false
	}
	return p, true
}

// ResolveTitlePin 返回该媒体的钉扎标题/年份，无记录时按以下顺序确定并落库：
// 最早一条成功入库记录的标题（= 初始整理目录名）→ fallbackTitle（非空时）。
// 首次落库后不再被后续调用覆盖（ON CONFLICT DO NOTHING）。
func (s *LibraryStore) ResolveTitlePin(mediaType string, tmdbID int64, fallbackTitle, fallbackYear string) (string, string, error) {
	if tmdbID <= 0 {
		return "", "", nil
	}
	if pin, ok := s.GetTitlePin(mediaType, tmdbID); ok {
		return pin.PinnedTitle, pin.PinnedYear, nil
	}
	title, year, ok := s.earliestIngestTitle(mediaType, tmdbID)
	source := "ingest_history"
	if !ok {
		if fallbackTitle == "" {
			return "", "", nil
		}
		title, year, source = fallbackTitle, fallbackYear, "tmdb"
	}
	if err := s.insertTitlePin(mediaType, tmdbID, title, year, source); err != nil {
		return "", "", err
	}
	return title, year, nil
}

// AdoptTitlePin 手动采用新译名：覆盖既有钉扎记录。
func (s *LibraryStore) AdoptTitlePin(mediaType string, tmdbID int64, title, year string) error {
	if tmdbID <= 0 || title == "" {
		return errors.New("无效的钉扎参数")
	}
	now := utcNow()
	_, err := s.db.Exec(`INSERT INTO media_title_pins
		(media_type, tmdb_id, pinned_title, pinned_year, source, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'manual', ?, ?)
		ON CONFLICT(media_type, tmdb_id) DO UPDATE SET
		pinned_title = excluded.pinned_title, pinned_year = excluded.pinned_year,
		source = 'manual', updated_at = excluded.updated_at`,
		mediaType, tmdbID, title, year, now, now)
	return err
}

// ListTitlePins 全量列出钉扎记录（按媒体类型、标题排序）。
func (s *LibraryStore) ListTitlePins() ([]TitlePin, error) {
	rows, err := s.db.Query(`SELECT media_type, tmdb_id, pinned_title, pinned_year, source, created_at, updated_at
		FROM media_title_pins ORDER BY media_type, pinned_title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pins := []TitlePin{}
	for rows.Next() {
		var p TitlePin
		if err := rows.Scan(&p.MediaType, &p.TmdbID, &p.PinnedTitle, &p.PinnedYear, &p.Source, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		pins = append(pins, p)
	}
	return pins, rows.Err()
}

// ListDuplicateFolderGroups 返回同一 (media_type, tmdb_id) 关联多个网盘目录的分组。
func (s *LibraryStore) ListDuplicateFolderGroups() ([]DuplicateFolderGroup, error) {
	rows, err := s.db.Query(`SELECT media_type, tmdb_id, COUNT(*) AS c
		FROM library_media WHERE tmdb_id > 0 AND in_library = 1
		GROUP BY media_type, tmdb_id HAVING c > 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := []DuplicateFolderGroup{}
	for rows.Next() {
		var g DuplicateFolderGroup
		var count int
		if err := rows.Scan(&g.MediaType, &g.TmdbID, &count); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// 先关闭 rows 释放连接（单连接池），再逐组查目录 ID
	rows.Close()
	for i := range groups {
		groups[i].FolderIDs = s.ListFolderIDsByTmdb(groups[i].MediaType, groups[i].TmdbID)
	}
	return groups, nil
}

func (s *LibraryStore) insertTitlePin(mediaType string, tmdbID int64, title, year, source string) error {
	now := utcNow()
	_, err := s.db.Exec(`INSERT INTO media_title_pins
		(media_type, tmdb_id, pinned_title, pinned_year, source, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(media_type, tmdb_id) DO NOTHING`,
		mediaType, tmdbID, title, year, source, now, now)
	return err
}

// earliestIngestTitle 查询该媒体最早一条成功入库记录的标题/年份。
func (s *LibraryStore) earliestIngestTitle(mediaType string, tmdbID int64) (string, string, bool) {
	var title, year string
	err := s.db.QueryRow(`SELECT title, year FROM ingest_history
		WHERE media_type = ? AND tmdb_id = ? AND status = 'success'
		ORDER BY ingested_at ASC, id ASC LIMIT 1`, mediaType, tmdbID).Scan(&title, &year)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("查询最早入库记录失败 tmdb_id=%d: %v", tmdbID, err)
		}
		return "", "", false
	}
	return title, year, true
}
