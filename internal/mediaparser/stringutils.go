// cn2an.go / stringutils.go 合并 —— 中文数字转换与字符串工具
// （替代 Python 版 cn2an、zhconv、Pinyin2Hanzi 三个依赖的所需子集）。
package mediaparser

import (
	"regexp"
	"strings"
	"unicode"
)

// ── 中文数字 → 阿拉伯数字（cn2an smart 模式子集） ──────────

var cnDigitValues = map[rune]int64{
	'零': 0, '〇': 0, '一': 1, '壹': 1, '二': 2, '贰': 2, '两': 2, '三': 3, '叁': 3,
	'四': 4, '肆': 4, '五': 5, '伍': 5, '六': 6, '陆': 6, '七': 7, '柒': 7,
	'八': 8, '捌': 8, '九': 9, '玖': 9,
	'0': 0, '1': 1, '2': 2, '3': 3, '4': 4, '5': 5, '6': 6, '7': 7, '8': 8, '9': 9,
}

var cnUnitValues = map[rune]int64{'十': 10, '拾': 10, '百': 100, '佰': 100, '千': 1000, '仟': 1000}

// Cn2AnSmart 中文/阿拉伯混合数字解析（cn2an smart 模式子集）。
// 支持纯阿拉伯数字、标准中文数字（十/百/千/万）与混合形式。
// 解析失败返回 (0, false)。
func Cn2AnSmart(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	// 纯阿拉伯数字快速路径
	if isAllDigits(s) {
		var n int64
		for _, ch := range s {
			n = n*10 + int64(ch-'0')
		}
		return n, true
	}

	var total, section, digit int64
	hasDigit := false // 当前 section 是否已累计过数值
	for _, ch := range s {
		if d, ok := cnDigitValues[ch]; ok {
			digit = d
			hasDigit = true
			continue
		}
		unit, ok := cnUnitValues[ch]
		if !ok {
			return 0, false
		}
		if unit == 10 || unit == 100 || unit == 1000 {
			base := digit
			if !hasDigit || digit == 0 {
				base = 1 // "十" 单独出现 = 10，"百" 前无数字 = 100
			}
			section += base * unit
			digit = 0
			hasDigit = false
		} else if unit == 10000 {
			total = (total + section + digit) * 10000
			section = 0
			digit = 0
			hasDigit = false
		}
	}
	result := total + section + digit
	if result == 0 && s != "0" && !strings.ContainsAny(s, "0零〇") {
		return 0, false
	}
	return result, true
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

// ── 繁体 → 简体（zhconv 子集，覆盖解析所需字符集） ────────

// tradChars 与 simpChars 逐字对照（长度必须一致）。
const tradChars = "動畫紀錄電視連續劇韓陸亞華語綜藝盤廣東島話們時聞" +
	"聲優內兩雙國際級壓縣廳質網頁學習實驗現線讀寫編輯刪傳輸導師場館賽車輪" +
	"飛機員長強調結構讓認識證據歷愛戀離獨寵記憶錢銀號碼圖書報紙雜誌節樂會" +
	"戲觀眾製發佈訊處統運維護戰鬥勝敗隊選擇風雲陰圓懸驚靈異獸龍鳳鳥魚蟲樹" +
	"葉藍紅綠黃顏響鳴靜開關閉滿淺寬輕舊壞醜淨髒齊亂齣籃負贏擊擋殺藥醫針劑" +
	"檢診療險灣鐘鋼鐵錯馬驅體龜"

const simpChars = "动画纪记录电视连续剧韩陆亚华语综艺盘广东岛话们时闻" +
	"声优内两双国际级压县厅质网页学习实验现线读写编辑删传输导师场馆赛车轮" +
	"飞机员强调结构让认识证据历爱恋离独宠记忆钱银号码图书报纸杂志节乐会" +
	"戏观众制发布讯处统运维护战斗胜败队选择风云阴圆悬惊灵异兽龙凤鸟鱼虫树" +
	"叶蓝红绿黄颜响鸣静开关闭满浅宽轻旧坏丑净脏齐乱出篮负赢击挡杀药医针剂" +
	"检诊疗险湾钟钢铁错马驱体龟"

var tradToSimp = func() map[rune]rune {
	trad := []rune(tradChars)
	simp := []rune(simpChars)
	m := make(map[rune]rune, len(trad))
	for i, ch := range trad {
		m[ch] = simp[i]
	}
	return m
}()

// ToSimplified 繁体转简体（仅覆盖解析所需字符集，未映射字符原样返回）。
func ToSimplified(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, ch := range s {
		if simp, ok := tradToSimp[ch]; ok {
			b.WriteRune(simp)
		} else {
			b.WriteRune(ch)
		}
	}
	return b.String()
}

// ── 拼音检测（Pinyin2Hanzi.is_pinyin 子集） ─────────────────

// pinyinSyllables 常用无调拼音音节表。
var pinyinSyllables = map[string]bool{}

var pinyinSyllableList = []string{
	"a", "o", "e", "i", "u", "v", "ai", "ei", "ui", "ao", "ou", "iu", "ie", "ve",
	"er", "an", "en", "in", "un", "vn", "ang", "eng", "ing", "ong",
	"ba", "bo", "bai", "bei", "bao", "ban", "ben", "bang", "beng", "bi", "bie",
	"biao", "bian", "bin", "bing", "bu",
	"po", "pai", "pei", "pao", "tou", "pou", "pan", "pen", "pang", "peng", "pi",
	"pie", "piao", "pian", "pin", "ping", "pu",
	"ma", "mo", "me", "mai", "mei", "mao", "mou", "man", "men", "mang", "meng",
	"mi", "mie", "miao", "miu", "mian", "min", "ming", "mu",
	"fa", "fo", "fei", "fou", "fan", "fen", "fang", "feng", "fu",
	"da", "de", "dai", "dei", "dao", "dou", "dan", "den", "dang", "deng", "dong",
	"di", "die", "diao", "diu", "dian", "ding", "du", "duo", "dui", "duan", "dun",
	"ta", "te", "tai", "tao", "tan", "tang", "teng", "ti", "tie", "tiao", "tian",
	"ting", "tu", "tuo", "tui", "tuan", "tun", "ta",
	"na", "ne", "nai", "nei", "nao", "nou", "nan", "nen", "nang", "neng", "ning",
	"ni", "nie", "niao", "niu", "nian", "nin", "niang", "nu", "nuo", "nuan", "nvn",
	"lan", "gen", "lang", "leng", "li", "lia", "lie", "liao", "liu", "lian", "lin",
	"liang", "lu", "luo", "luan", "lun", "lve", "lo", "la", "le", "lai", "lei",
	"lao", "lou", "long", "lang",
	"ga", "ge", "gai", "gei", "gao", "gou", "gan", "gen", "gang", "geng", "gong",
	"gu", "gua", "guo", "gui", "guan", "gun", "guang",
	"ka", "ke", "kai", "kei", "kao", "kou", "kan", "ken", "kang", "keng", "kong",
	"ku", "kua", "kuo", "kui", "kuan", "kun", "kuang",
	"ha", "he", "hai", "hei", "hao", "hou", "han", "hen", "hang", "heng", "hong",
	"hu", "hua", "huo", "hui", "huan", "hun", "huang",
	"jia", "jie", "jiao", "jiu", "jian", "jin", "jiang", "jing", "jiong", "ju",
	"jue", "juan", "jun", "ji",
	"qi", "qia", "qie", "qiao", "qiu", "qian", "qin", "qiang", "qing", "qiong",
	"qu", "que", "quan", "qun",
	"xia", "xie", "xiao", "xiu", "xian", "xin", "xiang", "xing", "xiong", "xu",
	"xue", "xuan", "xun", "xi",
	"zha", "zhe", "zhei", "zhao", "zhou", "zhan", "zhen", "zhang", "zheng", "zhong",
	"zhu", "zhua", "zhuo", "zhui", "zhuan", "zhun", "zhuang",
	"cha", "che", "chai", "chao", "chou", "chan", "chen", "chang", "cheng", "chong",
	"chu", "chua", "chuo", "chui", "chuan", "chun", "chuang",
	"sha", "she", "shei", "shao", "shou", "shan", "shen", "shang", "sheng", "shu",
	"shua", "shuo", "shui", "shuan", "shun", "shuang",
	"re", "rao", "rou", "ran", "ren", "rang", "reng", "rong", "ru", "rua", "ruo",
	"rui", "ruan", "run",
	"za", "ze", "zei", "zao", "zou", "zan", "zen", "zang", "zeng", "zong", "zu",
	"zuo", "zui", "zuan", "zun", "zi", "ci", "ca", "ce", "cai", "cao", "cou",
	"can", "cen", "cang", "ceng", "cong", "cu", "cuo", "cui", "cuan", "cun",
	"sa", "se", "sao", "sou", "san", "sen", "sang", "seng", "song", "su", "suo",
	"sui", "suan", "sun", "si",
	"ya", "yo", "ye", "yao", "you", "yan", "yin", "yang", "ying", "yong", "wu",
	"wa", "wo", "wai", "wei", "wan", "wen", "wang", "weng", "yu", "yue", "yuan",
	"yun",
}

func init() {
	for _, s := range pinyinSyllableList {
		pinyinSyllables[s] = true
	}
}

// IsPinyinWord 判断单词是否可完全切分为合法拼音音节。
func IsPinyinWord(word string) bool {
	if word == "" {
		return false
	}
	n := len(word)
	// DP: dp[i] 表示前 i 个字符可切分
	dp := make([]bool, n+1)
	dp[0] = true
	for i := 1; i <= n; i++ {
		for j := i - 1; j >= 0 && i-j <= 6; j-- {
			if dp[j] && pinyinSyllables[word[j:i]] {
				dp[i] = true
				break
			}
		}
	}
	return dp[n]
}

// IsPinyin 判断字符串（可含空格分隔的多个词）是否全部为拼音。
func IsPinyin(s string) bool {
	if s == "" {
		return false
	}
	for _, word := range strings.Fields(s) {
		if !IsPinyinWord(strings.ToLower(word)) {
			return false
		}
	}
	return true
}

// ── 字符判断（StringUtils 子集） ────────────────────────────

// IsChinese 判断是否含有中文。
func IsChinese(s string) bool {
	for _, ch := range s {
		if ch >= 0x4e00 && ch <= 0x9fff {
			return true
		}
	}
	return false
}

// IsAllChinese 判断是否全是中文（空格忽略）。
func IsAllChinese(s string) bool {
	for _, ch := range s {
		if ch == ' ' {
			continue
		}
		if ch >= 0x4e00 && ch <= 0x9fff {
			continue
		}
		return false
	}
	return true
}

// IsJapanese 判断是否含有日文假名。
func IsJapanese(s string) bool {
	for _, ch := range s {
		if (ch >= 0x3040 && ch <= 0x309F) || (ch >= 0x30A0 && ch <= 0x30FF) {
			return true
		}
	}
	return false
}

// StrTitle 大写每个单词首字母。
func StrTitle(s string) string {
	if s == "" {
		return s
	}
	prevSpace := true
	return strings.Map(func(r rune) rune {
		if r == ' ' {
			prevSpace = true
			return r
		}
		if prevSpace {
			prevSpace = false
			return unicode.ToUpper(r)
		}
		return r
	}, s)
}

// ── 搜索关键字拆分（StringUtils.get_keyword） ───────────────

var (
	kwTypeRe    = regexp.MustCompile(`^(电视剧|动漫|\s+电视剧|\s+动漫)`)
	kwTypeStrip = regexp.MustCompile(`^(电影|电视剧|动漫|\s+电影|\s+电视剧|\s+动漫)`)
	kwSeasonRe  = regexp.MustCompile(`(?i)第\s*([0-9一二三四五六七八九十]+)\s*季`)
	kwEpisodeRe = regexp.MustCompile(`(?i)第\s*([0-9一二三四五六七八九十百零]+)\s*集`)
	kwYearRe    = regexp.MustCompile(`[\s(]+(\d{4})[\s)]*`)
	kwCleanRe   = regexp.MustCompile(`(?i)第\s*[0-9一二三四五六七八九十]+\s*季|第\s*[0-9一二三四五六七八九十百零]+\s*集|[\s(]+(\d{4})[\s)]*`)
)

// KeywordResult GetKeyword 返回结构。
type KeywordResult struct {
	Mtype      MediaType
	Keyword    string
	SeasonNum  *int
	EpisodeNum *int
	Year       string
	Content    string
}

// GetKeyword 从搜索关键字中拆分年份、季、集、类型。
func GetKeyword(content string) KeywordResult {
	var result KeywordResult
	if content == "" {
		return result
	}
	result.Content = content

	if kwTypeRe.MatchString(content) {
		result.Mtype = MediaTV
	}
	content = strings.TrimSpace(kwTypeStrip.ReplaceAllString(content, ""))

	if m := kwSeasonRe.FindStringSubmatch(content); m != nil {
		result.Mtype = MediaTV
		if n, ok := Cn2AnSmart(m[1]); ok {
			result.SeasonNum = intPtr(int(n))
		}
	}
	if m := kwEpisodeRe.FindStringSubmatch(content); m != nil {
		result.Mtype = MediaTV
		if n, ok := Cn2AnSmart(m[1]); ok {
			result.EpisodeNum = intPtr(int(n))
			if result.EpisodeNum != nil && result.SeasonNum == nil {
				result.SeasonNum = intPtr(1)
			}
		}
	}
	if m := kwYearRe.FindStringSubmatch(content); m != nil {
		result.Year = m[1]
	}

	result.Keyword = strings.TrimSpace(kwCleanRe.ReplaceAllString(content, ""))
	result.Keyword = regexp.MustCompile(`\s+`).ReplaceAllString(result.Keyword, " ")
	if result.Keyword == "" {
		result.Keyword = result.Year
	}
	return result
}
