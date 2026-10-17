package config

import "testing"

func TestNormalizeStrmURLBase(t *testing.T) {
	cases := map[string]string{
		"meta2cloud:38765":            "http://meta2cloud:38765",
		" http://meta2cloud:38765/":   "http://meta2cloud:38765",
		"https://strm.example.com":    "https://strm.example.com",
		"https://strm.example.com/":   "https://strm.example.com",
		"  192.168.1.10:38765  ":      "http://192.168.1.10:38765",
		"":                            "",
		"   ":                         "",
	}
	for in, want := range cases {
		if got := normalizeStrmURLBase(in); got != want {
			t.Errorf("normalizeStrmURLBase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseStrmURLBaseNormalization(t *testing.T) {
	s := parseStrm(map[string]any{"enabled": true, "url_base": "meta2cloud:38765"})
	if s.URLBase != "http://meta2cloud:38765" {
		t.Errorf("URLBase = %q", s.URLBase)
	}
	s = parseStrm(map[string]any{"url_base": "http://a.b/"})
	if s.URLBase != "http://a.b" {
		t.Errorf("URLBase = %q", s.URLBase)
	}
}
