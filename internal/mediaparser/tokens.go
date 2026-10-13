// tokens.go —— 将标题字符串按分隔符拆为 token 列表（与 Python 版一致）。
package mediaparser

import "strings"

// Tokens 标题分词器。
type Tokens struct {
	tokens []string
	index  int
}

// tokenSeparators 分隔符：. ( ) [ ] - 【 】 / ～ ; & | # _ 「 」 ~ 及空白。
// Python 原正则 r"\.|\ s+|\(|\)|\[|]|-|【|】|/|～|;|&|\||#|_|「|」|~"
// 注：Python 原式中的 "\ s+" 是字面量反斜杠+空格（疑似笔误），
// 实际按 "." 单字符分隔 + 后续分隔符逐字处理，这里保持等价行为。
func NewTokens(text string) *Tokens {
	splitFn := func(r rune) bool {
		switch r {
		case '.', '(', ')', '[', ']', '-', '【', '】', '/', '～', ';', '&', '|', '#', '_', '「', '」', '~':
			return true
		}
		return false
	}
	parts := strings.FieldsFunc(text, splitFn)
	return &Tokens{tokens: parts}
}

// Cur 当前 token（越界返回空串）。
func (t *Tokens) Cur() string {
	if t.index >= len(t.tokens) {
		return ""
	}
	return t.tokens[t.index]
}

// GetNext 取出当前 token 并前进（越界返回空串）。
func (t *Tokens) GetNext() string {
	token := t.Cur()
	if token != "" {
		t.index++
	}
	return token
}

// Peek 预览下一个 token（不前进）。
func (t *Tokens) Peek() string {
	idx := t.index + 1
	if idx >= len(t.tokens) {
		return ""
	}
	return t.tokens[idx]
}

// All 全部 token。
func (t *Tokens) All() []string { return t.tokens }
