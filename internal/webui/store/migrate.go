// migrate.go —— SQLite schema 版本化迁移。
//
// 版本号记录在共享库的 schema_versions 表（store 名 → 已应用到的版本）而非
// PRAGMA user_version：library.db 由 LibraryStore / RSSStore / IngestStore 共用，
// user_version 是整库单值，多 store 会互相覆盖。
//
// 存量库对齐：版本管理引入前的库没有版本行，其历史迁移（缺列即 ALTER、整表
// 重建）已由旧代码应用且部分不可重放——接入时用 baseline 探测函数把这类库
// 对齐到已等效的版本，之后的迁移按版本号顺序正常执行。
package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// migration 单个版本化迁移。stmts 在单事务内执行（SQLite 支持事务性 DDL）。
type migration struct {
	version int
	name    string
	stmts   []string
	// legacyTolerant 历史迁移重放容忍：存量库打开时 DDL 会按当前结构补建缺失
	// 的表，历史迁移随之重放时可能撞 "duplicate column"（新表已带该列）。
	// 置真的迁移遇到该错误跳过该条语句。仅限接入版本管理前的历史迁移；
	// 新迁移（v3+）必须精确，不得依赖此容忍。
	legacyTolerant bool
}

// migrateStoreSchema 把 store 的 schema 版本从当前推进到 len(migrations)。
// baseline 仅在版本行缺失（首次接入）时调用，返回存量库已等效应用的迁移数。
func migrateStoreSchema(db *sql.DB, store string, baseline func(*sql.DB) (int, error), migrations []migration) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_versions (
		store   TEXT    NOT NULL PRIMARY KEY,
		version INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("%s: 创建 schema_versions 表失败：%w", store, err)
	}
	var current int
	err := db.QueryRow("SELECT version FROM schema_versions WHERE store = ?", store).Scan(&current)
	switch {
	case err == sql.ErrNoRows:
		n, berr := baseline(db)
		if berr != nil {
			return fmt.Errorf("%s: 探测存量 schema 版本失败：%w", store, berr)
		}
		current = n
	case err != nil:
		return fmt.Errorf("%s: 读取 schema 版本失败：%w", store, err)
	}
	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("%s: 迁移 v%d 开启事务失败：%w", store, m.version, err)
		}
		for _, stmt := range m.stmts {
			if _, err := tx.Exec(stmt); err != nil {
				if m.legacyTolerant && isDuplicateColumnErr(err) {
					continue
				}
				_ = tx.Rollback()
				return fmt.Errorf("%s: 迁移 v%d（%s）失败：%w", store, m.version, m.name, err)
			}
		}
		if _, err := tx.Exec(`INSERT INTO schema_versions(store, version) VALUES (?, ?)
			ON CONFLICT(store) DO UPDATE SET version = excluded.version`, store, m.version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("%s: 迁移 v%d 写版本号失败：%w", store, m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("%s: 迁移 v%d 提交失败：%w", store, m.version, err)
		}
	}
	return nil
}

// isDuplicateColumnErr SQLite 加列撞已有列（modernc 驱动的错误文案）。
// 历史迁移重放场景专用，见 migration.legacyTolerant。
func isDuplicateColumnErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate column")
}

// baselineColumns 常用基线探测：目标列全部存在返回 n，否则 0（从 v1 重放——
// 只适用于可重放的加列迁移；整表重建类迁移必须按实际历史版本探测）。
func baselineColumns(db *sql.DB, n int, table string, columns ...string) (int, error) {
	for _, col := range columns {
		has, err := columnExists(db, table, col)
		if err != nil {
			return 0, err
		}
		if !has {
			return 0, nil
		}
	}
	return n, nil
}
