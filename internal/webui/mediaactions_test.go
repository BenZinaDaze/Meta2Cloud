// mediaactions_test.go —— 修正识别文件改名：目录内视频按标准命名重命名，字幕随基名同步。
package webui

import (
	"strings"
	"testing"

	"meta2cloud/internal/cloudfile"
)

func fileName(t *testing.T, fp *fakeWebProvider, id string) string {
	t.Helper()
	cf, err := fp.GetFile(id)
	if err != nil {
		t.Fatalf("GetFile(%s): %v", id, err)
	}
	return cf.Name
}

// 电影目录：唯一视频与字幕改为「标题 (年份)」基名
func TestRenameLibraryFilesMovie(t *testing.T) {
	fp := newFakeWebProvider()
	root := fp.addFolder("root", "0")
	movie := fp.addFolder("旧名 (2020)", root.ID)
	video := fp.addFile("旧名.1080p.WEB-DL.mkv", movie.ID)
	sub := fp.addFile("旧名.1080p.WEB-DL.chs.srt", movie.ID)
	nfo := fp.addFile("movie.nfo", movie.ID)

	n, errs := renameLibraryFiles(fp.provider(), movie.ID, "movie",
		map[string]any{"title": "新名", "year": "2020"})
	if len(errs) != 0 {
		t.Fatalf("errs = %v", errs)
	}
	if n != 2 {
		t.Fatalf("renamed = %d, want 2（视频+字幕）", n)
	}
	if got := fileName(t, fp, video.ID); got != "新名 (2020).mkv" {
		t.Errorf("video name = %q", got)
	}
	if got := fileName(t, fp, sub.ID); got != "新名 (2020).chs.srt" {
		t.Errorf("subtitle name = %q", got)
	}
	if got := fileName(t, fp, nfo.ID); got != "movie.nfo" {
		t.Errorf("nfo 不应改名: %q", got)
	}

	// 幂等：已是标准命名时零改名
	n, errs = renameLibraryFiles(fp.provider(), movie.ID, "movie",
		map[string]any{"title": "新名", "year": "2020"})
	if n != 0 || len(errs) != 0 {
		t.Fatalf("二次执行 renamed = %d, errs = %v", n, errs)
	}
}

// 电影目录多视频：无法确定命名，整体跳过并给出原因
func TestRenameLibraryFilesMovieMultipleVideos(t *testing.T) {
	fp := newFakeWebProvider()
	root := fp.addFolder("root", "0")
	movie := fp.addFolder("旧名 (2020)", root.ID)
	fp.addFile("旧名.1080p.mkv", movie.ID)
	fp.addFile("旧名.2160p.mkv", movie.ID)

	n, errs := renameLibraryFiles(fp.provider(), movie.ID, "movie",
		map[string]any{"title": "新名", "year": "2020"})
	if n != 0 {
		t.Fatalf("renamed = %d, want 0", n)
	}
	if len(errs) != 1 || !strings.Contains(errs[0], "2 个视频文件") {
		t.Fatalf("errs = %v", errs)
	}
}

// 剧集目录：Season 内视频改为「标题 - SxxEyy」，字幕跟随；无法识别季集的跳过
func TestRenameLibraryFilesTV(t *testing.T) {
	fp := newFakeWebProvider()
	root := fp.addFolder("root", "0")
	show := fp.addFolder("旧剧 (2021)", root.ID)
	season1 := fp.addFolder("Season 01", show.ID)
	ep1 := fp.addFile("旧剧 - S01E01.mkv", season1.ID)
	sub1 := fp.addFile("旧剧 - S01E01.zh.srt", season1.ID)
	ep2 := fp.addFile("新剧 - S01E02.mkv", season1.ID) // 已是标准名，应保持并计入零改动
	extra := fp.addFile("特典.mp4", season1.ID)        // 无季集信息，跳过并报原因
	season2 := fp.addFolder("Season 02", show.ID)
	ep5 := fp.addFile("旧剧 - S02E05.mkv", season2.ID)
	fp.addFile("旧剧花絮", show.ID) // 媒体目录根下的文件不动

	n, errs := renameLibraryFiles(fp.provider(), show.ID, "tv", map[string]any{"title": "新剧"})
	if n != 3 {
		t.Fatalf("renamed = %d, want 3（E01 视频+字幕、S02E05）", n)
	}
	if len(errs) != 1 || !strings.Contains(errs[0], "特典.mp4") {
		t.Fatalf("errs = %v", errs)
	}
	if got := fileName(t, fp, ep1.ID); got != "新剧 - S01E01.mkv" {
		t.Errorf("ep1 name = %q", got)
	}
	if got := fileName(t, fp, sub1.ID); got != "新剧 - S01E01.zh.srt" {
		t.Errorf("sub1 name = %q", got)
	}
	if got := fileName(t, fp, ep2.ID); got != "新剧 - S01E02.mkv" {
		t.Errorf("ep2 应保持标准名: %q", got)
	}
	if got := fileName(t, fp, extra.ID); got != "特典.mp4" {
		t.Errorf("extra 不应改名: %q", got)
	}
	if got := fileName(t, fp, ep5.ID); got != "新剧 - S02E05.mkv" {
		t.Errorf("ep5 name = %q", got)
	}
}

// 字幕基名与视频不一致时不动（避免误改无关字幕）
func TestRenameSiblingSubtitlesIgnoresForeignStem(t *testing.T) {
	fp := newFakeWebProvider()
	root := fp.addFolder("root", "0")
	dir := fp.addFolder("d", root.ID)
	subs := []cloudfile.CloudFile{
		fp.addFile("无关字幕.srt", dir.ID),
	}
	n, errs := renameSiblingSubtitles(fp.provider(), subs, "旧剧 - S01E01", "新剧 - S01E01")
	if n != 0 || len(errs) != 0 {
		t.Fatalf("n = %d, errs = %v", n, errs)
	}
	if got := fileName(t, fp, subs[0].ID); got != "无关字幕.srt" {
		t.Errorf("无关字幕被误改: %q", got)
	}
}
