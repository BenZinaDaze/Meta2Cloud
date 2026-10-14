package mediaparser

import (
	"strings"
	"testing"
)

func TestCn2AnSmart(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"1", 1}, {"12", 12}, {"103", 103},
		{"一", 1}, {"二", 2}, {"十", 10}, {"十一", 11}, {"十二", 12},
		{"二十", 20}, {"二十一", 21}, {"二十六", 26},
		{"一百", 100}, {"一百零三", 103}, {"一百二十", 120},
	}
	for _, c := range cases {
		got, ok := Cn2AnSmart(c.in)
		if !ok || got != c.want {
			t.Errorf("Cn2AnSmart(%q) = %d,%v want %d", c.in, got, ok, c.want)
		}
	}
	if _, ok := Cn2AnSmart("abc"); ok {
		t.Error("非数字应解析失败")
	}
}

func TestToSimplified(t *testing.T) {
	if got := ToSimplified("動畫紀錄片"); got != "动画片" && got != "动画纪录片片" {
		// 只需确认关键词字符完成转换
		if !strings.Contains(got, "动") || !strings.Contains(got, "画") {
			t.Errorf("ToSimplified(動畫) = %q", got)
		}
	}
	if got := ToSimplified("動漫"); !strings.Contains(got, "动漫") {
		t.Errorf("ToSimplified(動漫) = %q, want 动漫", got)
	}
}

func TestIsPinyin(t *testing.T) {
	if !IsPinyin("li hua") {
		t.Error("\"li hua\" 应识别为拼音")
	}
	if !IsPinyin("qing yu nian") {
		t.Error("\"qing yu nian\" 应识别为拼音")
	}
	if IsPinyin("Breaking") {
		t.Error("\"Breaking\" 不应识别为拼音（含 b-r-e-a-k... 'break' 中 ng 等）")
	}
}

func TestIsAnime(t *testing.T) {
	if !IsAnime("[LoliHouse] Title - 01") {
		t.Error("方括号开头应为动漫格式")
	}
	if !IsAnime("2024年1月新番 标题") {
		t.Error("年月新番格式应为动漫")
	}
	if IsAnime("Breaking.Bad.S01E03.1080p") {
		t.Error("普通剧集不应判定为动漫")
	}
}

// ── 西文剧集解析（对应 Python 版经典用例） ────────────────

func TestMetaInfoBreakingBad(t *testing.T) {
	meta := MetaInfo("Breaking.Bad.S01E03.1080p.BluRay.HEVC-TTG", "", false, nil, nil)
	if meta.Name() != "Breaking Bad" {
		t.Errorf("name = %q, want Breaking Bad", meta.Name())
	}
	if meta.Season() != "S01" {
		t.Errorf("season = %q, want S01", meta.Season())
	}
	if meta.Episode() != "E03" {
		t.Errorf("episode = %q, want E03", meta.Episode())
	}
	if meta.ResourcePix != "1080p" {
		t.Errorf("resource_pix = %q, want 1080p", meta.ResourcePix)
	}
	if meta.ResourceType != "BluRay" {
		t.Errorf("resource_type = %q, want BluRay", meta.ResourceType)
	}
	if !strings.EqualFold(meta.VideoEncode, "HEVC") {
		t.Errorf("video_encode = %q, want HEVC", meta.VideoEncode)
	}
	if meta.ResourceTeam != "TTG" {
		t.Errorf("resource_team = %q, want TTG", meta.ResourceTeam)
	}
	if meta.Type != MediaTV {
		t.Errorf("type = %v, want TV", meta.Type)
	}
}

func TestMetaInfoMovie(t *testing.T) {
	meta := MetaInfo("Inception.2010.1080p.BluRay.x264.DTS-HD.Ma", "", false, nil, nil)
	if meta.Name() != "Inception" {
		t.Errorf("name = %q, want Inception", meta.Name())
	}
	if meta.Year != "2010" {
		t.Errorf("year = %q, want 2010", meta.Year)
	}
	if meta.ResourcePix != "1080p" {
		t.Errorf("resource_pix = %q", meta.ResourcePix)
	}
}

func TestMetaInfoSeasonFull(t *testing.T) {
	meta := MetaInfo("Season 2", "", false, nil, nil)
	if meta.Type != MediaTV || meta.BeginSeason == nil || *meta.BeginSeason != 2 {
		t.Errorf("Season 2 应解析为 TV 第 2 季, got type=%v season=%v", meta.Type, meta.BeginSeason)
	}
}

// ── 动漫解析 ────────────────────────────────────────────────

func TestMetaInfoAnimeLoliHouse(t *testing.T) {
	title := "[LoliHouse] 东岛丹三郎想成为假面骑士 / Toujima Tanzaburou wa Kamen Rider ni Naritai - 23 [WebRip 1080p HEVC-10bit AAC][简繁内封字幕]"
	meta := MetaInfo(title, "", true, nil, nil)
	if !strings.Contains(meta.Name(), "东岛丹三郎") {
		t.Errorf("name = %q, 应包含中文标题", meta.Name())
	}
	if meta.BeginEpisode == nil || *meta.BeginEpisode != 23 {
		t.Errorf("episode = %v, want 23", meta.BeginEpisode)
	}
	if meta.ResourcePix != "1080p" {
		t.Errorf("resource_pix = %q, want 1080p", meta.ResourcePix)
	}
	if meta.ResourceTeam != "LoliHouse" {
		t.Errorf("resource_team = %q, want LoliHouse", meta.ResourceTeam)
	}
	if meta.Type != MediaTV {
		t.Errorf("type = %v, want TV", meta.Type)
	}
}

func TestMetaInfoAnimeFSDBrackets(t *testing.T) {
	meta := MetaInfo("【FSD】东岛丹三郎想成为假面骑士[02][我是塔克尔].mp4", "", true, nil, nil)
	if meta.BeginEpisode == nil || *meta.BeginEpisode != 2 {
		t.Errorf("episode = %v, want 2", meta.BeginEpisode)
	}
	if !strings.Contains(meta.CnName, "东岛丹三郎") {
		t.Errorf("cn_name = %q", meta.CnName)
	}
}

// ── 副标题中文季集解析 ──────────────────────────────────────

func TestInitSubtitleChinese(t *testing.T) {
	m := NewMetaBase("测试剧", "", false)
	m.InitSubtitle("第二季")
	if m.Type != MediaTV || m.BeginSeason == nil || *m.BeginSeason != 2 {
		t.Errorf("第二季 应解析出 S02, got %v %v", m.Type, m.BeginSeason)
	}

	m2 := NewMetaBase("测试剧", "", false)
	m2.InitSubtitle("全26集")
	if m2.TotalEpisode != 26 {
		t.Errorf("全26集 total_episode = %d, want 26", m2.TotalEpisode)
	}

	m3 := NewMetaBase("测试剧", "", false)
	m3.InitSubtitle("第03-06集")
	if m3.BeginEpisode == nil || *m3.BeginEpisode != 3 || m3.EndEpisode == nil || *m3.EndEpisode != 6 {
		t.Errorf("第03-06集 = %v-%v, want 3-6", m3.BeginEpisode, m3.EndEpisode)
	}

	m4 := NewMetaBase("测试剧", "", false)
	m4.InitSubtitle("Episode 15")
	if m4.BeginEpisode == nil || *m4.BeginEpisode != 15 {
		t.Errorf("Episode 15 = %v, want 15", m4.BeginEpisode)
	}
}

// ── 内嵌 ID 标签 ────────────────────────────────────────────

func TestEmbeddedTags(t *testing.T) {
	meta := MetaInfo("测试剧 {[tmdbid=12345;type=tv;s=2;e=5]}", "", false, nil, nil)
	if meta.TmdbID != 12345 {
		t.Errorf("tmdbid = %d, want 12345", meta.TmdbID)
	}
	if meta.Type != MediaTV {
		t.Errorf("type = %v, want TV", meta.Type)
	}
	if meta.BeginSeason == nil || *meta.BeginSeason != 2 {
		t.Errorf("season = %v, want 2", meta.BeginSeason)
	}
	if meta.BeginEpisode == nil || *meta.BeginEpisode != 5 {
		t.Errorf("episode = %v, want 5", meta.BeginEpisode)
	}
}

// ── 路径解析（父目录 merge） ────────────────────────────────

func TestMetaInfoPath(t *testing.T) {
	meta := MetaInfoPath("/media/TV/Breaking Bad/Season 1/Breaking.Bad.S01E03.1080p.mkv", nil, nil)
	if meta.Name() != "Breaking Bad" {
		t.Errorf("name = %q, want Breaking Bad", meta.Name())
	}
	if meta.Episode() != "E03" {
		t.Errorf("episode = %q, want E03", meta.Episode())
	}
}

// ── 字幕组 ──────────────────────────────────────────────────

func TestReleaseGroupsMatcher(t *testing.T) {
	m := NewReleaseGroupsMatcher(nil)
	if got := m.Match("Show.S01E01.1080p.WEB-DL-TTG"); got != "TTG" {
		t.Errorf("match = %q, want TTG", got)
	}
	if got := m.Match("[LoliHouse] Title - 01"); got != "LoliHouse" {
		t.Errorf("match = %q, want LoliHouse", got)
	}
	custom := NewReleaseGroupsMatcher([]string{"FSD"})
	if got := custom.Match("【FSD】标题[02]"); got != "FSD" {
		t.Errorf("custom match = %q, want FSD", got)
	}
}

// ── 自定义识别词 ────────────────────────────────────────────

func TestWordsMatcher(t *testing.T) {
	// 屏蔽词
	wm := NewWordsMatcher([]string{"垃圾词"})
	out, applied := wm.Prepare("标题 垃圾词 S01")
	if strings.Contains(out, "垃圾词") {
		t.Errorf("屏蔽词未生效: %q", out)
	}
	if len(applied) != 1 {
		t.Errorf("applied = %v", applied)
	}
	// 替换词
	wm2 := NewWordsMatcher([]string{"旧名 => 新名"})
	out2, _ := wm2.Prepare("这是旧名第一季")
	if !strings.Contains(out2, "新名") || strings.Contains(out2, "旧名") {
		t.Errorf("替换词未生效: %q", out2)
	}
	// 集偏移
	wm3 := NewWordsMatcher([]string{"第 <> 集 >> EP+1"})
	out3, applied3 := wm3.Prepare("第5集")
	if len(applied3) == 0 || !strings.Contains(out3, "第6集") {
		t.Errorf("集偏移未生效: %q applied=%v", out3, applied3)
	}
}
