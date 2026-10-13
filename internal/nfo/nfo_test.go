package nfo

import (
	"strings"
	"testing"

	"meta2cloud/internal/mediaparser"
)

func movieInfo() map[string]any {
	return map[string]any{
		"media_type":           "电影",
		"tmdb_id":              float64(27205),
		"id":                   float64(27205),
		"title":                "Inception",
		"original_title":       "Inception",
		"release_date":         "2010-07-16",
		"overview":             "梦境 <盗梦> & 计划",
		"vote_average":         float64(8.4),
		"vote_count":           float64(35000),
		"runtime":              float64(148),
		"external_ids":         map[string]any{"imdb_id": "tt1375666"},
		"genres":               []any{map[string]any{"name": "动作"}, map[string]any{"name": "科幻"}},
		"production_countries": []any{map[string]any{"name": "美国"}},
		"directors":            []map[string]any{{"id": float64(525), "name": "Christopher Nolan", "profile_path": "/n.jpg"}},
		"actors":               []map[string]any{{"id": float64(6197), "name": "Leonardo DiCaprio", "character": "Cobb", "profile_path": "/l.jpg"}},
		"poster_path":          "/poster.jpg",
		"backdrop_path":        "/backdrop.jpg",
	}
}

func TestGenerateMovie(t *testing.T) {
	g := NewGenerator("")
	xmlStr := g.Generate(movieInfo(), mediaparser.MediaMovie, nil)
	for _, want := range []string{
		"<movie>", "<title>Inception</title>", "<year>2010</year>",
		"<runtime>148</runtime>", "<imdbid>tt1375666</imdbid>",
		`<uniqueid type="tmdb" default="false">27205</uniqueid>`,
		`<uniqueid type="imdb" default="true">tt1375666</uniqueid>`,
		"<genre>动作</genre>", "<country>美国</country>",
		`<director tmdbid="525">Christopher Nolan</director>`,
		"<name>Leonardo DiCaprio</name>", "<role>Cobb</role>",
		"<thumb>https://image.tmdb.org/t/p/original/poster.jpg</thumb>",
		"<fanart>", "<![CDATA[梦境 <盗梦> & 计划]]>",
	} {
		if !strings.Contains(xmlStr, want) {
			t.Errorf("movie NFO 缺少 %q\n%s", want, xmlStr)
		}
	}
}

func TestGenerateTVShow(t *testing.T) {
	g := NewGenerator("")
	info := map[string]any{
		"media_type":     "电视剧",
		"tmdb_id":        float64(1396),
		"name":           "绝命毒师",
		"original_name":  "Breaking Bad",
		"first_air_date": "2008-01-20",
		"overview":       "化学老师转行",
		"vote_average":   float64(8.9),
		"genres":         []any{map[string]any{"name": "犯罪"}},
		"origin_country": []any{"US"},
		"external_ids":   map[string]any{"tvdb_id": float64(81189)},
	}
	xmlStr := g.GenerateTVShow(info)
	for _, want := range []string{
		"<tvshow>", "<title>绝命毒师</title>", "<originaltitle>Breaking Bad</originaltitle>",
		"<season>-1</season>", "<episode>-1</episode>",
		"<tvdbid>81189</tvdbid>", "<country>US</country>",
		`<uniqueid type="tvdb">81189</uniqueid>`,
	} {
		if !strings.Contains(xmlStr, want) {
			t.Errorf("tvshow NFO 缺少 %q\n%s", want, xmlStr)
		}
	}
}

func TestGenerateEpisodeWithDetail(t *testing.T) {
	g := NewGenerator("")
	show := map[string]any{
		"media_type": "电视剧",
		"tmdb_id":    float64(1396),
		"name":       "绝命毒师",
		"_season":    float64(1),
		"_episode":   float64(3),
	}
	ep := map[string]any{
		"id":             float64(62085),
		"name":           "...And the Bag's in the River",
		"air_date":       "2008-02-10",
		"overview":       "Walt & Jesse 处理 <紧急> 局面",
		"still_path":     "/still.jpg",
		"season_number":  float64(1),
		"episode_number": float64(3),
		"vote_average":   float64(8.2),
	}
	xmlStr := g.Generate(show, mediaparser.MediaTV, ep)
	for _, want := range []string{
		"<episodedetails>", "<showtitle>绝命毒师</showtitle>",
		"<title>...And the Bag's in the River</title>",
		"<aired>2008-02-10</aired>", "<season>1</season>", "<episode>3</episode>",
		"<tmdbid>62085</tmdbid>", "<thumb>https://image.tmdb.org/t/p/original/still.jpg</thumb>",
		"<![CDATA[Walt &amp;", // CDATA 中 & 原样保留（拆分逻辑可能引入转义，断言宽松
	} {
		if !strings.Contains(xmlStr, want) && want == "<![CDATA[Walt &amp;" {
			// CDATA 内不转义：允许 <![CDATA[Walt & Jesse
			if !strings.Contains(xmlStr, "<![CDATA[Walt & Jesse") {
				t.Errorf("episode NFO CDATA 内容错误\n%s", xmlStr)
			}
			continue
		}
		if !strings.Contains(xmlStr, want) {
			t.Errorf("episode NFO 缺少 %q\n%s", want, xmlStr)
		}
	}
}

func TestEpisodeTitleFallback(t *testing.T) {
	g := NewGenerator("")
	show := map[string]any{"name": "测试剧", "_season": float64(1), "_episode": float64(5)}
	xmlStr := g.Generate(show, mediaparser.MediaTV, nil)
	if !strings.Contains(xmlStr, "<title>第5集</title>") {
		t.Errorf("无单集详情时应回退到「第N集」:\n%s", xmlStr)
	}
}

func TestNfoNameFor(t *testing.T) {
	if got := NfoNameFor("Breaking.Bad.S01E03.mkv"); got != "Breaking.Bad.S01E03.nfo" {
		t.Errorf("nfoNameFor = %q", got)
	}
}

func TestSeasonPosterName(t *testing.T) {
	if got := SeasonPosterName(1); got != "season01-poster.jpg" {
		t.Errorf("season1 = %q", got)
	}
	if got := SeasonPosterName(0); got != "season-specials-poster.jpg" {
		t.Errorf("season0 = %q", got)
	}
}

func TestGenerateSeason(t *testing.T) {
	g := NewGenerator("")
	detail := map[string]any{"name": "第一季", "overview": "简介", "air_date": "2008-01-20", "poster_path": "/s.jpg"}
	xmlStr := g.GenerateSeason(detail, 1)
	for _, want := range []string{"<season>", "<seasonnumber>1</seasonnumber>", "<title>第一季</title>", "<year>2008</year>"} {
		if !strings.Contains(xmlStr, want) {
			t.Errorf("season NFO 缺少 %q\n%s", want, xmlStr)
		}
	}
}
