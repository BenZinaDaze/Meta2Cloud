package webui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPruneEmptyDirs(t *testing.T) {
	rt := InitRuntime(t.TempDir())
	root := t.TempDir()
	// 空目录链（应自内向外全部删除）
	empty := filepath.Join(root, "旧剧名 (2024)", "Season 01")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	// 含文件的目录（应保留）
	keep := filepath.Join(root, "在播剧 (2025)", "Season 01")
	if err := os.MkdirAll(keep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keep, "ep1.strm"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt.pruneEmptyDirs(root)

	if _, err := os.Stat(empty); !os.IsNotExist(err) {
		t.Fatal("空目录应被删除")
	}
	if _, err := os.Stat(filepath.Join(root, "旧剧名 (2024)")); !os.IsNotExist(err) {
		t.Fatal("腾空后的父目录也应被删除")
	}
	if _, err := os.Stat(filepath.Join(keep, "ep1.strm")); err != nil {
		t.Fatal("非空目录内容不应被删除")
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatal("根目录不应被删除")
	}
}
