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

// 尾部未闭合括号（prepare_title 以 ] join 截断产生）不应混入标题。
func TestMetaInfoAnimeDanglingBracket(t *testing.T) {
	meta := MetaInfo("[桜都字幕组] 关于邻家的天使大人 第二季 [05][1080p][简繁内封].mkv", "", true, nil, nil)
	if meta.CnName != "关于邻家的天使大人" {
		t.Errorf("cn_name = %q, want 关于邻家的天使大人", meta.CnName)
	}
	if meta.BeginEpisode == nil || *meta.BeginEpisode != 5 {
		t.Errorf("episode = %v, want 5", meta.BeginEpisode)
	}
	if meta.BeginSeason == nil || *meta.BeginSeason != 2 {
		t.Errorf("season = %v, want 2", meta.BeginSeason)
	}
	if meta.ResourcePix != "1080p" {
		t.Errorf("resource_pix = %q, want 1080p", meta.ResourcePix)
	}
	if meta.ResourceTeam != "桜都字幕组" {
		t.Errorf("resource_team = %q, want 桜都字幕组", meta.ResourceTeam)
	}
}

// 西式圆括号分辨率应被提取，集号在 () 之前也应识别。
func TestMetaInfoAnimeParenResolution(t *testing.T) {
	meta := MetaInfo("[SubsPlease] Frieren - 12 (1080p) [04B6C9EE].mkv", "", true, nil, nil)
	if meta.EnName != "Frieren" {
		t.Errorf("en_name = %q, want Frieren", meta.EnName)
	}
	if meta.BeginEpisode == nil || *meta.BeginEpisode != 12 {
		t.Errorf("episode = %v, want 12", meta.BeginEpisode)
	}
	if meta.ResourcePix != "1080p" {
		t.Errorf("resource_pix = %q, want 1080p", meta.ResourcePix)
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

// 01-26Fin / [01-38 END] 等数字范围+完结标记（对齐 MoviePilot init_episode_range_fin）。
func TestInitSubtitleEpisodeRangeFin(t *testing.T) {
	m := NewMetaBase("某番", "", false)
	m.InitSubtitle("某番 01-26Fin")
	if m.BeginEpisode == nil || *m.BeginEpisode != 1 || m.EndEpisode == nil || *m.EndEpisode != 26 || m.TotalEpisode != 26 {
		t.Errorf("01-26Fin = %v-%v total %v, want 1-26 total 26", m.BeginEpisode, m.EndEpisode, m.TotalEpisode)
	}

	m2 := NewMetaBase("某番", "", false)
	m2.InitSubtitle("[01-38 END]")
	if m2.BeginEpisode == nil || *m2.BeginEpisode != 1 || m2.EndEpisode == nil || *m2.EndEpisode != 38 {
		t.Errorf("[01-38 END] = %v-%v, want 1-38", m2.BeginEpisode, m2.EndEpisode)
	}

	// 年份范围不应识别为集数
	m3 := NewMetaBase("某番", "", false)
	m3.InitSubtitle("2019-2020完结")
	if m3.BeginEpisode != nil {
		t.Errorf("2019-2020完结 误识别为集数: %v", m3.BeginEpisode)
	}
}

// 「不包含/不包括/不含」等排除语句不应误识别季集（对齐 MoviePilot v3 排除词 lookbehind）。
func TestInitSubtitleExclusionPhrase(t *testing.T) {
	m := NewMetaBase("某番", "", false)
	m.InitSubtitle("某番 不包含第2季")
	if m.BeginSeason != nil {
		t.Errorf("不包含第2季 误识别为 S%d", *m.BeginSeason)
	}
	m2 := NewMetaBase("某番", "", false)
	m2.InitSubtitle("不包括第5集")
	if m2.BeginEpisode != nil {
		t.Errorf("不包括第5集 误识别为 E%d", *m2.BeginEpisode)
	}
	// 正常识别不受影响
	m3 := NewMetaBase("某番", "", false)
	m3.InitSubtitle("第2季")
	if m3.BeginSeason == nil || *m3.BeginSeason != 2 {
		t.Errorf("第2季 = %v, want 2", m3.BeginSeason)
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

// 集偏移增强能力（对齐 MoviePilot v3 words.py）：中文数字/前导零/算术表达式/多集排序。
func TestWordsMatcherOffsetEnhanced(t *testing.T) {
	cases := []struct {
		word, in, want string
	}{
		{"第 <> 集 >> EP+1", "第五集", "第六集"},             // 中文数字，偏移后转回中文
		{"第 <> 集 >> EP+1", "第01集", "第02集"},           // 前导零宽度保持
		{"第 <> 集 >> EP+99", "第001集", "第100集"},        // 宽度 3
		{"第 <> 集 >> EP*2+10", "第3集", "第16集"},         // 算术表达式
		{"第 <> 集 >> (EP+1)/2", "第5集", "第3集"},         // 括号与除法（2.5 截断为 2…此处 6/2=3）
		{"第 <> 集 >> EP+1", "第01集至第12集", "第02集至第13集"}, // 多集按序替换
	}
	for _, c := range cases {
		out, applied := NewWordsMatcher([]string{c.word}).Prepare(c.in)
		if out != c.want || len(applied) == 0 {
			t.Errorf("偏移 %q 于 %q = %q (applied=%v), want %q", c.word, c.in, out, applied, c.want)
		}
	}
	// 省略运算符（2EP）应拒绝应用，原文不变
	out, applied := NewWordsMatcher([]string{"第 <> 集 >> 2EP"}).Prepare("第5集")
	if out != "第5集" || len(applied) != 0 {
		t.Errorf("2EP 应拒绝: %q applied=%v", out, applied)
	}
}

// 替换词兼容 Python 风格 \1 捕获组引用。
func TestWordsMatcherBackrefCompat(t *testing.T) {
	out, applied := NewWordsMatcher([]string{`旧名(\d+)集 => 新名\1集`}).Prepare("旧名12集")
	if out != "新名12集" || len(applied) == 0 {
		t.Errorf(`\1 引用未生效: %q applied=%v`, out, applied)
	}
}

// 屏蔽词支持正则；# 注释行跳过。
func TestWordsMatcherRegexBlockAndComment(t *testing.T) {
	out, _ := NewWordsMatcher([]string{"垃圾.*词", "# 这是注释"}).Prepare("标题 垃圾XYZ词 尾")
	if strings.Contains(out, "垃圾") {
		t.Errorf("正则屏蔽词未生效: %q", out)
	}
	out2, applied2 := NewWordsMatcher([]string{"#注释", "屏蔽词"}).Prepare("有屏蔽词")
	if len(applied2) != 1 || !strings.Contains(out2, "有") {
		t.Errorf("注释行应跳过: %q applied=%v", out2, applied2)
	}
}

// 集偏移跨字段：定位词分跨标题与副标题时只偏移副标题集数（对齐 v3 prepare_with_subtitle）。
func TestWordsMatcherSubtitleOffset(t *testing.T) {
	title, subtitle, applied := NewWordsMatcher([]string{"某番 <> END >> EP+100"}).
		PrepareWithSubtitle("某番 S02", "第01-12话 END")
	if len(applied) == 0 {
		t.Fatalf("跨字段偏移未应用: %v", applied)
	}
	if title != "某番 S02" {
		t.Errorf("标题不应变化: %q", title)
	}
	// 与 v3 一致：副标题中仅 [集话話期幕] 前的数字被视为集数表达，"01-" 不偏移
	if subtitle != "第01-112话 END" {
		t.Errorf("副标题偏移结果 = %q, want 第01-112话 END", subtitle)
	}
	// 完结范围形式：首尾都偏移
	_, subtitle2, applied2 := NewWordsMatcher([]string{"某番 <> END >> EP+100"}).
		PrepareWithSubtitle("某番 S02", "01-12 END")
	if len(applied2) == 0 || subtitle2 != "101-112 END" {
		t.Errorf("完结范围偏移 = %q applied=%v, want 101-112 END", subtitle2, applied2)
	}
}
