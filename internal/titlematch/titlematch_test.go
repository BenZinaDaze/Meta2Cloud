// titlematch_test.go —— 标题模糊匹配行为测试（自 store 移入）。
package titlematch

import "testing"

func TestTitlesMatch(t *testing.T) {
	if !TitlesMatch("Breaking Bad", []string{"Breaking  Bad"}) {
		t.Error("忽略空白后应匹配")
	}
	if !TitlesMatch("绝命毒师", []string{"绝命毒师 (2008)"}) {
		t.Error("年份后缀应被剥离")
	}
	if TitlesMatch("ABC", []string{"XYZ"}) {
		t.Error("不同标题不应匹配")
	}
	// 短于 4 字符不做包含匹配
	if TitlesMatch("AB", []string{"ABCD"}) {
		t.Error("短标题包含匹配应被禁止")
	}
}
