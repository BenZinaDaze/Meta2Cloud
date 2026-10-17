package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCategoriesResolve(t *testing.T) {
	c := DefaultCategories()

	// 日语动画 → 动漫
	l1, l2 := c.Resolve(true, []int{16}, "ja")
	if l1 != "剧集" || l2 != "动漫" {
		t.Fatalf("resolve = %s/%s", l1, l2)
	}
	// 中文动画 → 国漫
	if _, l2 = c.Resolve(true, []int{16}, "zh"); l2 != "国漫" {
		t.Fatalf("国漫 = %s", l2)
	}
	// 非动画剧集 → 兜底「普通电视剧」
	if _, l2 = c.Resolve(true, []int{18}, "en"); l2 != "普通电视剧" {
		t.Fatalf("兜底 = %s", l2)
	}
	// 动画电影（不分语言）→ 动漫电影
	if _, l2 = c.Resolve(false, []int{16}, "en"); l2 != "动漫电影" {
		t.Fatalf("动漫电影 = %s", l2)
	}
	// 普通电影 → 兜底
	if _, l2 = c.Resolve(false, []int{28}, "en"); l2 != "普通电影" {
		t.Fatalf("普通电影 = %s", l2)
	}
}

func TestCategoriesLoadSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "categories.yaml")

	// 文件不存在 = 未启用
	if _, err := LoadCategories(path); !os.IsNotExist(err) {
		t.Fatalf("期望 ErrNotExist，得到 %v", err)
	}

	if err := SaveCategories(path, DefaultCategories()); err != nil {
		t.Fatal(err)
	}
	c, err := LoadCategories(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Dirs.TV != "剧集" || len(c.TV) != 3 || c.TV[0].Name != "动漫" {
		t.Fatalf("round-trip 不一致: %+v", c)
	}
}

func TestCategoriesValidate(t *testing.T) {
	c := DefaultCategories()
	c.TV[1].Name = c.TV[0].Name // 重名
	if err := c.Validate(); err == nil {
		t.Fatal("重名应报错")
	}
	c.TV[1].Name = "  " // 空名
	if err := c.Validate(); err == nil {
		t.Fatal("空目录名应报错")
	}
}
