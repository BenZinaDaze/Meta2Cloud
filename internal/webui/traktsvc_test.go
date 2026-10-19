package webui

import (
	"testing"

	"meta2cloud/internal/plexserver"
)

// TestSentinelFingerprint 观看哨兵指纹：标记/取消标记均应改变指纹。
func TestSentinelFingerprint(t *testing.T) {
	top := []plexserver.Metadata{
		{RatingKey: "814", LastViewedAt: 1792349545},
		{RatingKey: "815"},
	}
	base := sentinelFingerprint(top)
	if base == "" {
		t.Fatal("指纹不应为空")
	}
	// 新标记一条（top 变化 + 时间戳）
	if got := sentinelFingerprint([]plexserver.Metadata{
		{RatingKey: "900", LastViewedAt: 1792349600},
		top[0], top[1],
	}); got == base {
		t.Error("新标记应改变指纹")
	}
	// 取消标记（时间戳清零）
	if got := sentinelFingerprint([]plexserver.Metadata{
		{RatingKey: "814"}, top[1],
	}); got == base {
		t.Error("取消标记应改变指纹")
	}
	// 无变化
	if got := sentinelFingerprint(top); got != base {
		t.Error("无变化时指纹应一致")
	}
}
