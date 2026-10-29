package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// 版本化迁移框架：新库按序应用并记录版本；存量库按基线对齐不重放历史迁移；
// 同库多 store 版本互不干扰；legacyTolerant 迁移重放时容忍重复列。
func TestMigrateStoreSchemaFreshDB(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatal(err)
	}
	migrations := []migration{
		{version: 1, name: "add a", stmts: []string{"ALTER TABLE items ADD COLUMN a TEXT NOT NULL DEFAULT ''"}},
		{version: 2, name: "add b", stmts: []string{"ALTER TABLE items ADD COLUMN b TEXT NOT NULL DEFAULT ''"}},
	}
	if err := migrateStoreSchema(db, "t1", func(*sql.DB) (int, error) { return 0, nil }, migrations); err != nil {
		t.Fatalf("迁移失败：%v", err)
	}
	var version int
	if err := db.QueryRow("SELECT version FROM schema_versions WHERE store='t1'").Scan(&version); err != nil || version != 2 {
		t.Fatalf("版本应记录为 2，实际 %d（err=%v）", version, err)
	}
	// 重复打开（幂等）：不重放、不报错
	if err := migrateStoreSchema(db, "t1", func(*sql.DB) (int, error) { return 0, nil }, migrations); err != nil {
		t.Fatalf("重复迁移应幂等：%v", err)
	}
}

func TestMigrateStoreSchemaBaselineSkipsHistory(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE items (id INTEGER PRIMARY KEY, a TEXT NOT NULL DEFAULT '')"); err != nil {
		t.Fatal(err)
	}
	// 基线探测 v1 已等效应用 → 只执行 v2
	migrations := []migration{
		{version: 1, name: "add a", stmts: []string{"ALTER TABLE items ADD COLUMN a TEXT NOT NULL DEFAULT ''"}},
		{version: 2, name: "add b", stmts: []string{"ALTER TABLE items ADD COLUMN b TEXT NOT NULL DEFAULT ''"}},
	}
	baseline := func(*sql.DB) (int, error) { return 1, nil }
	if err := migrateStoreSchema(db, "t2", baseline, migrations); err != nil {
		t.Fatalf("迁移失败：%v", err)
	}
	var hasB int
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('items') WHERE name='b'").Scan(&hasB); err != nil || hasB != 1 {
		t.Fatalf("v2 应已应用（b 列存在），hasB=%d err=%v", hasB, err)
	}
	var version int
	_ = db.QueryRow("SELECT version FROM schema_versions WHERE store='t2'").Scan(&version)
	if version != 2 {
		t.Fatalf("版本应记录为 2，实际 %d", version)
	}
}

// 同一 DB 文件多个 store 共存（library.db 的真实形态）：版本按 store 名隔离。
func TestMigrateStoreSchemaSharedDBIsolation(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "shared.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE s1t (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE s2t (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	m1 := []migration{{version: 1, name: "s1 add x", stmts: []string{"ALTER TABLE s1t ADD COLUMN x TEXT NOT NULL DEFAULT ''"}}}
	m2 := []migration{
		{version: 1, name: "s2 add y", stmts: []string{"ALTER TABLE s2t ADD COLUMN y TEXT NOT NULL DEFAULT ''"}},
		{version: 2, name: "s2 add z", stmts: []string{"ALTER TABLE s2t ADD COLUMN z TEXT NOT NULL DEFAULT ''"}},
	}
	noop := func(*sql.DB) (int, error) { return 0, nil }
	if err := migrateStoreSchema(db, "store1", noop, m1); err != nil {
		t.Fatal(err)
	}
	if err := migrateStoreSchema(db, "store2", noop, m2); err != nil {
		t.Fatal(err)
	}
	var v1, v2 int
	if err := db.QueryRow("SELECT version FROM schema_versions WHERE store='store1'").Scan(&v1); err != nil || v1 != 1 {
		t.Fatalf("store1 版本应为 1，实际 %d err=%v", v1, err)
	}
	if err := db.QueryRow("SELECT version FROM schema_versions WHERE store='store2'").Scan(&v2); err != nil || v2 != 2 {
		t.Fatalf("store2 版本应为 2（不被 store1 覆盖），实际 %d err=%v", v2, err)
	}
}

func TestMigrateStoreSchemaLegacyTolerant(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// 模拟 DDL 先行补建出已带列的新表，历史迁移重放撞重复列
	if _, err := db.Exec("CREATE TABLE items (id INTEGER PRIMARY KEY, a TEXT NOT NULL DEFAULT '')"); err != nil {
		t.Fatal(err)
	}
	migrations := []migration{
		{version: 1, name: "add a", legacyTolerant: true,
			stmts: []string{"ALTER TABLE items ADD COLUMN a TEXT NOT NULL DEFAULT ''"}},
		{version: 2, name: "add b", legacyTolerant: true,
			stmts: []string{"ALTER TABLE items ADD COLUMN b TEXT NOT NULL DEFAULT ''"}},
	}
	if err := migrateStoreSchema(db, "t3", func(*sql.DB) (int, error) { return 0, nil }, migrations); err != nil {
		t.Fatalf("容忍重复列的迁移应成功：%v", err)
	}
	var hasB int
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('items') WHERE name='b'").Scan(&hasB); err != nil || hasB != 1 {
		t.Fatalf("v2 应正常应用，hasB=%d err=%v", hasB, err)
	}
}
