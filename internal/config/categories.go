// categories.go —— 媒体分类规则（categories.yaml，独立于主配置）。
//
// 文件存在即启用两级分类布局：媒体库根/一级(剧集|电影|其他)/二级/标题…；
// 文件不存在时整理与扫描维持原有 电影根/剧集根 行为（零迁移、向后兼容）。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// CategoriesFilename 分类配置文件名（与主配置同目录）。
const CategoriesFilename = "categories.yaml"

// CategoryRule 二级分类规则：按列表顺序匹配，首个命中的生效；
// 最后一条规则兼任兜底（所有规则未命中时落此目录）。
type CategoryRule struct {
	Name      string   `yaml:"name" json:"name"`
	GenreIDs  []int    `yaml:"genre_ids,omitempty" json:"genre_ids,omitempty"`
	Languages []string `yaml:"languages,omitempty" json:"languages,omitempty"`
}

// CategoryDirs 一级目录名（挂在媒体库根下）。
type CategoryDirs struct {
	TV    string `yaml:"tv" json:"tv"`
	Movie string `yaml:"movie" json:"movie"`
	Other string `yaml:"other" json:"other"`
}

// CategoriesConfig 媒体分类配置。
type CategoriesConfig struct {
	Dirs  CategoryDirs   `yaml:"dirs" json:"dirs"`
	TV    []CategoryRule `yaml:"tv" json:"tv"`
	Movie []CategoryRule `yaml:"movie" json:"movie"`
}

// DefaultCategories 默认分类（动漫/国漫/动漫电影按动画流派 16 推导，不分语言）。
func DefaultCategories() *CategoriesConfig {
	return &CategoriesConfig{
		Dirs: CategoryDirs{TV: "剧集", Movie: "电影", Other: "其他"},
		TV: []CategoryRule{
			{Name: "动漫", GenreIDs: []int{16}, Languages: []string{"ja"}},
			{Name: "国漫", GenreIDs: []int{16}, Languages: []string{"zh", "cn", "tw"}},
			{Name: "普通电视剧"},
		},
		Movie: []CategoryRule{
			{Name: "动漫电影", GenreIDs: []int{16}},
			{Name: "普通电影"},
		},
	}
}

// LoadCategories 读取分类配置；文件不存在返回 (nil, os.ErrNotExist) = 未启用。
func LoadCategories(path string) (*CategoriesConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c CategoriesConfig
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	c.normalize()
	return &c, nil
}

// SaveCategories 写入分类配置（临时文件 + rename 原子替换）。
func SaveCategories(path string, c *CategoriesConfig) error {
	c.normalize()
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Validate 保存前校验：目录名非空、同类型内不重复。
func (c *CategoriesConfig) Validate() error {
	check := func(list []CategoryRule, kind string) error {
		seen := map[string]bool{}
		for _, r := range list {
			name := strings.TrimSpace(r.Name)
			if name == "" {
				return fmt.Errorf("%s分类存在未填写目录名的规则", kind)
			}
			if seen[name] {
				return fmt.Errorf("%s分类目录名重复：%s", kind, name)
			}
			seen[name] = true
		}
		return nil
	}
	if err := check(c.TV, "剧集"); err != nil {
		return err
	}
	return check(c.Movie, "电影")
}

// Resolve 返回该媒体的一级/二级目录名。
// 规则按序首条命中；全部未命中落最后一条规则（兜底）；列表为空落一级根（异常配置，调用方告警）。
func (c *CategoriesConfig) Resolve(isTV bool, genreIDs []int, lang string) (level1, level2 string) {
	rules, level1 := c.rulesFor(isTV)
	lang = strings.ToLower(strings.TrimSpace(lang))
	for _, rule := range rules {
		if ruleMatches(rule, genreIDs, lang) {
			return level1, rule.Name
		}
	}
	if len(rules) > 0 {
		return level1, rules[len(rules)-1].Name
	}
	return level1, ""
}

func (c *CategoriesConfig) rulesFor(isTV bool) ([]CategoryRule, string) {
	if isTV {
		return c.TV, c.Dirs.TV
	}
	return c.Movie, c.Dirs.Movie
}

func ruleMatches(rule CategoryRule, genreIDs []int, lang string) bool {
	if len(rule.GenreIDs) > 0 {
		hit := false
		for _, want := range rule.GenreIDs {
			for _, got := range genreIDs {
				if got == want {
					hit = true
					break
				}
			}
			if hit {
				break
			}
		}
		if !hit {
			return false
		}
	}
	if len(rule.Languages) > 0 {
		hit := false
		for _, l := range rule.Languages {
			if strings.EqualFold(strings.TrimSpace(l), lang) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return true
}

// normalize 补齐一级目录名默认值并清理空白。
func (c *CategoriesConfig) normalize() {
	if c.Dirs.TV = strings.TrimSpace(c.Dirs.TV); c.Dirs.TV == "" {
		c.Dirs.TV = "剧集"
	}
	if c.Dirs.Movie = strings.TrimSpace(c.Dirs.Movie); c.Dirs.Movie == "" {
		c.Dirs.Movie = "电影"
	}
	if c.Dirs.Other = strings.TrimSpace(c.Dirs.Other); c.Dirs.Other == "" {
		c.Dirs.Other = "其他"
	}
	for i := range c.TV {
		c.TV[i].Name = strings.TrimSpace(c.TV[i].Name)
	}
	for i := range c.Movie {
		c.Movie[i].Name = strings.TrimSpace(c.Movie[i].Name)
	}
}
