// Package strutil 通用字符串小工具（收敛各包复制的 firstNonEmpty/orDefault 变体）。
package strutil

// FirstNonEmpty 返回第一个非空串；全空返回空串。
func FirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// OrDefault 空串回退默认值。
func OrDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
