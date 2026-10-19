package cloudfile

import (
	"testing"
)

func int64Ptr(v int64) *int64 { return &v }

func TestVideoByMime(t *testing.T) {
	f := CloudFile{ID: "1", Name: "test.dat", FileType: TypeFile, MimeType: "video/mp4"}
	if !f.IsVideo() {
		t.Error("MIME video/mp4 应识别为视频")
	}
}

func TestVideoByExtension(t *testing.T) {
	for _, ext := range []string{".mkv", ".mp4", ".avi", ".rmvb", ".ts", ".m2ts"} {
		f := CloudFile{ID: "1", Name: "movie" + ext, FileType: TypeFile}
		if !f.IsVideo() {
			t.Errorf("扩展名 %s 应识别为视频", ext)
		}
	}
}

func TestNonVideo(t *testing.T) {
	f := CloudFile{ID: "1", Name: "movie.zh-CN.srt", FileType: TypeFile}
	if f.IsVideo() {
		t.Error("字幕文件不应识别为视频")
	}
}

func TestFolder(t *testing.T) {
	f := CloudFile{ID: "1", Name: "Season 01", FileType: TypeFolder}
	if !f.IsFolder() || f.IsVideo() {
		t.Error("FOLDER 类型应识别为文件夹且不是视频")
	}
}

func TestFolderWithVideoExtensionIsNotVideo(t *testing.T) {
	f := CloudFile{ID: "1", Name: "fake.mkv", FileType: TypeFolder}
	if !f.IsFolder() || f.IsVideo() {
		t.Error("名字像视频的目录不能识别成视频文件")
	}
}

func TestExtension(t *testing.T) {
	f := CloudFile{ID: "1", Name: "Movie.MKV", FileType: TypeFile}
	if f.Extension() != ".mkv" {
		t.Errorf("extension = %q, want .mkv", f.Extension())
	}
}

func TestExtensionNoExt(t *testing.T) {
	f := CloudFile{ID: "1", Name: "README", FileType: TypeFile}
	if f.Extension() != "" {
		t.Errorf("无扩展名应返回空串， got %q", f.Extension())
	}
}

func TestParentsAndParentID(t *testing.T) {
	f := CloudFile{ID: "1", Name: "test.mkv", FileType: TypeFile, ParentID: "p1", Parents: []string{"p1", "p2"}}
	if f.ParentID != "p1" || len(f.Parents) != 2 || f.Parents[1] != "p2" {
		t.Error("parents 列表和 parent_id 应独立存储")
	}
}

func TestSubtitle(t *testing.T) {
	for _, ext := range []string{".srt", ".ass", ".ssa"} {
		f := CloudFile{ID: "1", Name: "sub" + ext, FileType: TypeFile}
		if !f.IsSubtitle() {
			t.Errorf("%s 应识别为字幕", ext)
		}
	}
	folder := CloudFile{ID: "1", Name: "dir.srt", FileType: TypeFolder}
	if folder.IsSubtitle() {
		t.Error("目录不应识别为字幕")
	}
}

func TestDefaults(t *testing.T) {
	f := CloudFile{ID: "1", Name: "test", FileType: TypeFile}
	if f.Size != nil || f.ModifiedTime != "" || f.ParentID != "" || f.MimeType != "" || f.Trashed {
		t.Error("零值默认不正确")
	}
}

func TestStringRepr(t *testing.T) {
	f := CloudFile{ID: "abcdefghijklmnop", Name: "test.mkv", FileType: TypeFile, Size: int64Ptr(100 * 1024 * 1024)}
	r := f.String()
	for _, want := range []string{"test.mkv", "🎬", "abcdefghijk"} {
		if !contains(r, want) {
			t.Errorf("String() = %q, 应包含 %q", r, want)
		}
	}
	folder := CloudFile{ID: "abcdefghijklmnop", Name: "Season 01", FileType: TypeFolder}
	if !contains(folder.String(), "📁") {
		t.Error("文件夹应显示 📁")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
