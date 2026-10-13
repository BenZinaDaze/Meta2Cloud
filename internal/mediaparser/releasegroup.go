// releasegroup.go —— 识别字幕组/发布组（完整移植 Python 版正则表）。
package mediaparser

import (
	"strings"
	"sync"

	"github.com/dlclark/regexp2"
)

// releaseGroups 内置组（正则片段），键仅作分组注释。
var releaseGroups = [][]string{
	{"FF(?:(?:A|WE)B|CD|E(?:DU|B)|TV)"},            // 0ff
	{"Audies", "AD(?:Audio|E(?:book|)|Music|Web)"}, // audiences
	{"BeiTai"},                         // beitai
	{"Bts(?:CHOOL|HD|PAD|TV)", "Zone"}, // btschool
	{"CarPT"},                          // carpt
	{"CHD(?:Bits|PAD|(?:|HK)TV|WEB|)", "StBOX", "OneHD", "Lee", "xiaopie"}, // chdbits
	{"(?:(?:iNT|(?:HALFC|Mini(?:S|H|FH)D))-|)TLF"},                         // eastgame
	{"(?:DG|GBWE)B"},                             // gainbound
	{"Hares(?:(?:M|T)V|Web|)"},                   // hares
	{"HDA(?:pad|rea|TV)", "EPiC"},                // hdarea
	{"HDC(?:hina|TV|)", "k9611", "tudou", "iHD"}, // hdchina
	{"D(?:ream|BTV)", "(?:HD|QHstudI)o"},         // hddolby
	{"beAst(?:TV|)"},                             // hdfans
	{"HDH(?:ome|Pad|TV|WEB|)"},                   // hdhome
	{"HDPT(?:Web|)"},                             // hdpt
	{"HDS(?:ky|TV|Pad|WEB|)", "AQLJ"},            // hdsky
	{"HDZ(?:one|)"},                              // hdzone
	{"HHWEB"},                                    // hhanclub
	{"HTPT"},                                     // htpt
	{"FRDS", "Yumi", "cXcY"},                     // keepfrds
	{"L(?:eague(?:(?:C|H)D|(?:M|T)V|NF|WEB)|HD)", "i18n", "CiNT"},     // lemonhd
	{"MTeam(?:TV|)", "MPAD", "MWeb"},                                  // mteam
	{"Our(?:Bits|TV)", "FLTTH", "Ao", "PbK", "MGs", "iLove(?:HD|TV)"}, // ourbits
	{"Panda", "AilMWeb"},                     // panda
	{"PiGo(?:NF|(?:H|WE)B)"},                 // piggo
	{"PTer(?:DIY|Game|(?:M|T)V|WEB|)"},       // pterclub
	{"PTH(?:Audio|eBook|music|ome|tv|WEB|)"}, // pthome
	{"PTsbao", "OPS", "F(?:Fans(?:AIeNcE|BD|D(?:VD|IY)|TV|WEB)|HDMv)", "SGXT"}, // ptsbao
	{"PuTao"},                   // putao
	{"CMCT(?:V|)"},              // springsunday
	{"Shark(?:WEB|DIY|TV|MV|)"}, // sharkpt
	{"TJUPT"},                   // tjupt
	{"TTG", "WiKi", "NGB", "DoA", "(?:ARi|ExRE)N"}, // totheglory
	{
		"B(?:MDru|eyondHD|TN)", "C(?:fandora|trlhd|MRG)", "DON", "EVO", "FLUX",
		"HONE(?:yG|)", "N(?:oGroup|T(?:b|G))", "PandaMoon", "SMURF",
		"T(?:EPES|aengoo|rollHD )",
	}, // others
	{
		"ANi", "HYSUB", "KTXP", "LoliHouse", "MCE", "Nekomoe kissaten",
		"SweetSub", "MingY", "(?:Lilith|NC)-Raws",
		"织梦字幕组", "枫叶字幕组", "猎户手抄部", "喵萌奶茶屋", "漫猫字幕社",
		"霜庭云花Sub", "北宇治字幕组", "氢气烤肉架", "云歌字幕组", "萌樱字幕组",
		"极影字幕社", "悠哈璃羽字幕社", "❀拨雪寻春❀",
		"沸羊羊(?:制作|字幕组)", "(?:桜|樱)都字幕组",
	}, // anime
	{"FROG(?:E|Web|)"},   // forge
	{"UB(?:its|WEB|TV)"}, // ubits
	{"Sakurato"},         // sakurato
}

var (
	releaseGroupMu sync.Mutex
	releaseGroupRe = map[string]*regexp2.Regexp{}
)

// ReleaseGroupsMatcher 识别制作组、字幕组。
type ReleaseGroupsMatcher struct {
	patternStr string
	re         *regexp2.Regexp
}

// NewReleaseGroupsMatcher 构造。customGroups 为额外正则片段列表。
func NewReleaseGroupsMatcher(customGroups []string) *ReleaseGroupsMatcher {
	all := make([]string, 0, 128)
	for _, group := range releaseGroups {
		all = append(all, group...)
	}
	for _, g := range customGroups {
		if g != "" {
			all = append(all, g)
		}
	}
	pattern := strings.Join(all, "|")
	if pattern == "" {
		return &ReleaseGroupsMatcher{}
	}
	return &ReleaseGroupsMatcher{patternStr: pattern, re: getReleaseGroupRe(pattern)}
}

// getReleaseGroupRe 按模式串缓存编译结果（进程内通常仅 1~2 种组合）。
func getReleaseGroupRe(pattern string) *regexp2.Regexp {
	releaseGroupMu.Lock()
	defer releaseGroupMu.Unlock()
	if re, ok := releaseGroupRe[pattern]; ok {
		return re
	}
	// 与 Python 版一致：前导分隔符 lookbehind + 尾部分隔符 lookahead
	re := regexp2.MustCompile(`(?<=[-@\[￡【&])(?:(?:`+pattern+`))(?=$|[@.\s\]\[】&])`, regexp2.IgnoreCase)
	releaseGroupRe[pattern] = re
	return re
}

// Match 匹配资源标题中的字幕组，多个用 @ 分隔。
func (m *ReleaseGroupsMatcher) Match(title string) string {
	if title == "" || m.patternStr == "" || m.re == nil {
		return ""
	}
	searchTitle := title + " "
	unique := []string{}
	mt, err := m.re.FindStringMatch(searchTitle)
	for err == nil && mt != nil {
		s := mt.String()
		if !containsStr(unique, s) {
			unique = append(unique, s)
		}
		mt, err = m.re.FindNextMatch(mt)
	}
	return strings.Join(unique, "@")
}

func containsStr(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
