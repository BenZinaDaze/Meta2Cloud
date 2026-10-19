package organizer

import "testing"

func TestFolderPathForTV(t *testing.T) {
	o := New(nil, "root")
	// 无年份信息时只有片名
	if got := o.FolderPathFor("Breaking.Bad.S01E03.1080p.BluRay.HEVC-TTG.mkv", true); got != "Breaking Bad/Season 01" {
		t.Errorf("tv path = %q, want Breaking Bad/Season 01", got)
	}
}

func TestFolderPathForMovieWithYear(t *testing.T) {
	o := New(nil, "root")
	if got := o.FolderPathFor("Inception.2010.1080p.BluRay.x264.mkv", true); got != "Inception (2010)" {
		t.Errorf("movie path = %q, want Inception (2010)", got)
	}
}

func TestFolderPathForPath(t *testing.T) {
	o := New(nil, "root")
	got := o.FolderPathForPath("/media/TV/Breaking Bad/Season 1/Breaking.Bad.S01E03.mkv")
	if got != "Breaking Bad/Season 01" {
		t.Errorf("path = %q, want Breaking Bad/Season 01", got)
	}
}

func TestFolderPathForUnparsable(t *testing.T) {
	o := New(nil, "root")
	if got := o.FolderPathFor("", true); got != "" {
		t.Errorf("空名应返回空串, got %q", got)
	}
}
