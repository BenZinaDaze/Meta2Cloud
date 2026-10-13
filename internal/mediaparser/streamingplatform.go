// streamingplatform.go —— 流媒体平台简称/全称匹配（完整移植 Python 版表）。
package mediaparser

import "strings"

// streamingPlatforms 平台表：(简称, 全称)。
var streamingPlatforms = [][2]string{
	{"AMZN", "Amazon"}, {"NF", "Netflix"}, {"ATVP", "Apple TV+"},
	{"iT", "iTunes"}, {"DSNP", "Disney+"}, {"HS", "Hotstar"},
	{"APPS", "Disney+ MENA"}, {"PMTP", "Paramount+"}, {"HMAX", "Max"},
	{"", "Max"}, {"HULU", "Hulu Networks"}, {"MA", "Movies Anywhere"},
	{"BCORE", "Bravia Core"}, {"MS", "Microsoft Store"}, {"SHO", "Showtime"},
	{"STAN", "Stan"}, {"PCOK", "Peacock"}, {"SKST", "SkyShowtime"},
	{"NOW", "Now"}, {"FXTL", "Foxtel Now"}, {"BNGE", "Binge"},
	{"CRKL", "Crackle"}, {"RKTN", "Rakuten TV"}, {"ALL4", "Channel 4"},
	{"AS", "Adult Swim"}, {"BRTB", "Brtb TV"}, {"CNLP", "Canal+"},
	{"CRIT", "Criterion Channel"}, {"DSCP", "Discovery+"}, {"FOOD", "Food Network"},
	{"MUBI", "Mubi"}, {"PLAY", "Google Play"}, {"YT", "YouTube"},
	{"", "friDay"}, {"", "KKTV"}, {"", "ofiii"}, {"", "LiTV"}, {"", "MyVideo"},
	{"Hami", "Hami Video"}, {"HamiVideo", "Hami Video"}, {"MW", "meWATCH"},
	{"CATCHPLAY", "CATCHPLAY+"}, {"CPP", "CATCHPLAY+"}, {"LINETV", "LINE TV"},
	{"VIU", "Viu"}, {"IQ", ""}, {"", "WeTV"}, {"ABMA", "Abema"},
	{"ADN", ""}, {"AT-X", ""}, {"Baha", ""}, {"BG", "B-Global"},
	{"CR", "Crunchyroll"}, {"", "DMM"}, {"FOD", ""}, {"FUNi", "Funimation"},
	{"HIDI", "HIDIVE"}, {"UNXT", "U-NEXT"}, {"FAA", "Filmarchiv Austria"},
	{"CC", "Comedy Central"}, {"iP", "BBC iPlayer"}, {"9NOW", "9Now"},
	{"ABC", ""}, {"", "AMC"}, {"", "ZEE5"}, {"", "WAVO"}, {"SHAHID", "Shahid"},
	{"Flixole", "FlixOlé"}, {"TOU", "Ici TOU.TV"}, {"ROKU", "Roku"},
	{"KNPY", "Kanopy"}, {"SNXT", "Sun NXT"}, {"CUR", "Curiosity Stream"},
	{"MY5", "Channel 5"}, {"AHA", "aha"}, {"WOWP", "WOW Presents Plus"},
	{"JC", "JioCinema"}, {"", "Dekkoo"}, {"FILMZIE", "Filmzie"},
	{"HoiChoi", "Hoichoi"}, {"VIKI", "Rakuten Viki"}, {"SF", "SF Anytime"},
	{"PLEX", "Plex"}, {"SHDR", "Shudder"}, {"CRAV", "Crave"},
	{"CPE", "Cineplex Entertainment"}, {"JF HC", ""}, {"JF", ""}, {"JFFP", ""},
	{"VIAP", "Viaplay"}, {"TUBI", "TubiTV"}, {"", "PBS"}, {"PBSK", "PBS KIDS"},
	{"LGP", "Lionsgate Play"}, {"", "CTV"}, {"", "Cineverse"}, {"LN", "Love Nature"},
	{"MP", "Movistar Plus+"}, {"RUNTIME", "Runtime"}, {"STZ", "STARZ"},
	{"FUBO", "fuboTV"}, {"TENK", "Tënk"}, {"KNOW", "Knowledge Network"},
	{"TVO", "tvo"}, {"", "OVID"}, {"CBC", "CBC Gem"}, {"FANDOR", "fandor"},
	{"CW", "The CW"}, {"FREE", "Freeform"}, {"AE", "A&E"}, {"LIFE", "Lifetime"},
	{"WWEN", "WWE Network"}, {"CMAX", "Cinemax"}, {"HLMK", "Hallmark"},
	{"BYU", "BYUtv"}, {"", "ViX"}, {"VICE", "Viceland"}, {"", "TVING"},
	{"USAN", "USA Network"}, {"FOX", ""}, {"", "TCM"}, {"BRAV", "BravoTV"},
	{"", "TNT"}, {"", "ZDF"}, {"", "IndieFlix"}, {"", "TLC"}, {"", "HGTV"},
	{"ANPL", "Animal Planet"}, {"TRVL", "Travel Channel"}, {"", "VH1"},
	{"SAINA", "Saina Play"}, {"SP", "Saina Play"}, {"OXGN", "Oxygen"},
	{"PSN", "PlayStation Network"}, {"PMNT", "Paramount Network"},
	{"FAWESOME", "Fawesome"}, {"KLASSIKI", "Klassiki"}, {"STRP", "Star+"},
	{"NATG", "National Geographic"}, {"REVEEL", "Reveel"}, {"FYI", "FYI Network"},
	{"WatchiT", "WATCH IT"}, {"ITVX", "ITV"}, {"GAIA", "Gaia"},
	{"", "FlixLatino"}, {"CNNP", "CNN+"}, {"TROMA", "Troma"}, {"IVI", "Ivi"},
	{"A3P", "Atresplayer"}, {"7PLUS", "7plus"}, {"", "SBS"}, {"TEN", "10Play"},
	{"AUBC", ""}, {"DSNY", "Disney Networks"}, {"OSN", "OSN+"},
	{"SVT", "Sveriges Television"}, {"LACINETEK", "LaCinetek"}, {"", "Maxdome"},
	{"RTL", "RTL+"}, {"ARTE", "Arte"}, {"JOYN", "Joyn"}, {"TV2", "TV 2"},
	{"3SAT", "3sat"}, {"FILMINGO", "filmingo"}, {"", "WOW"}, {"OKKO", "Okko"},
	{"", "Go3"}, {"ARGP", "Argo"}, {"VOYO", "Voyo"}, {"VMAX", "vivamax"},
	{"FILMIN", "Filmin"}, {"", "Mitele"}, {"", "ARD"}, {"BK", "Bentkey"},
	{"BOOM", "Boomerang"}, {"", "CBS"}, {"CLBI", "Club illico"}, {"CMOR", "C More"},
	{"CMT", ""}, {"", "CNBC"}, {"COOK", "Cooking Channel"}, {"CWS", "CW Seed"},
	{"DCU", "DC Universe"}, {"DDY", "Digiturk Dilediğin Yerde"},
	{"DEST", "Destination America"}, {"DISC", "Discovery Channel"},
	{"DW", "DailyWire+"}, {"DLWP", "DailyWire+"}, {"DPLY", "dplay"},
	{"DRPO", "Dropout"}, {"EPIX", "EPIX MGM+"}, {"ESQ", "Esquire"}, {"ETV", "E!"},
	{"FBWatch", "Facebook Watch"}, {"FPT", "FPT Play"}, {"FTV", "France.tv"},
	{"GLOB", "GloboSat Play"}, {"GLBO", "Globoplay"}, {"GO90", "go90"},
	{"HIST", "History Channel"}, {"HPLAY", "Hungama Play"}, {"KS", "Kaleidescape"},
	{"", "MBC"}, {"MMAX", "ManoramaMAX"}, {"MNBC", "MSNBC"},
	{"MTOD", "Motor Trend OnDemand"}, {"NBC", ""}, {"NBLA", "Nebula"},
	{"NICK", "Nickelodeon"}, {"ODK", "OnDemandKorea"}, {"POGO", "PokerGO"},
	{"PUHU", "puhutv"}, {"QIBI", "Quibi"}, {"RTE", "RTÉ"}, {"SESO", "Seeso"},
	{"SPIK", "Spike"}, {"SS", "Simply South"}, {"SYFY", "SyFy"},
	{"TIMV", "TIMvision"}, {"TK", "Tentkotta"}, {"", "TV4"}, {"TVL", "TV Land"},
	{"", "TVNZ"}, {"", "UKTV"}, {"VLCT", "Discovery Velocity"}, {"VMEO", "Vimeo"},
	{"VRV", "VRV Defunct"}, {"WTCH", "Watcha"}, {"", "NowPlayer"},
	{"HuluJP", "Hulu Networks"}, {"Gaga", "GagaOOLala"}, {"MyTVS", "MyTVSuper"},
	{"", "BBC"}, {"NowE", "Now E"}, {"WAVVE", "Wavve"}, {"SE", ""},
	{"", "BritBox"}, {"AOD", "Anime on Demand"}, {"AF", ""}, {"BCH", "Bandai Channel"},
	{"VMJ", "VideoMarket"}, {"LFTL", "Laftel"}, {"WAKA", "Wakanim"},
	{"WAKANIM", "Wakanim"}, {"AO", "AnimeOnegai"}, {"", "Lemino"},
	{"VIDIO", "Vidio"}, {"TVER", "TVer"}, {"", "MBS"}, {"LFTLNET", "Laftel"},
	{"JONU", "Jonu Play"}, {"PlutoTV", "Pluto TV"}, {"AbemaTV", "Abema"},
	{"", "dTV"}, {"NYMEY", "Nymey"}, {"SMNS", "SAMANSA"}, {"CTHP", "CATCHPLAY+"},
	{"HBOGO", "HBO GO"}, {"HBO", "HBO"}, {"FPTP", "FPT Play"},
	{"", "LOCIPO"}, {"DANT", "DANET"}, {"OV", "OceanVeil"},
}

// StreamingPlatforms 流媒体平台查询器。
type StreamingPlatforms struct {
	cache map[string]string
}

// NewStreamingPlatforms 构造并预建索引。
func NewStreamingPlatforms() *StreamingPlatforms {
	cache := map[string]string{}
	for _, pair := range streamingPlatforms {
		short, full := pair[0], pair[1]
		canonical := full
		if canonical == "" {
			canonical = short
		}
		if canonical == "" {
			continue
		}
		for _, alias := range []string{short, full} {
			if alias != "" {
				cache[strings.ToUpper(alias)] = canonical
			}
		}
	}
	return &StreamingPlatforms{cache: cache}
}

// GetStreamingPlatformName 按简称取全称。
func (s *StreamingPlatforms) GetStreamingPlatformName(code string) string {
	if code == "" {
		return ""
	}
	return s.cache[strings.ToUpper(code)]
}

// IsStreamingPlatform 是否为已知平台简称/全称。
func (s *StreamingPlatforms) IsStreamingPlatform(name string) bool {
	if name == "" {
		return false
	}
	_, ok := s.cache[strings.ToUpper(name)]
	return ok
}
