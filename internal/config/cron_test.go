package config

import (
	"testing"
	"time"
)

// CronNextDelay 单测：固定 time.Date，不依赖真实时钟。
func TestCronNextDelayValid(t *testing.T) {
	now := time.Date(2026, 1, 5, 10, 3, 17, 0, time.UTC)

	// */5 落到下一个 5 分钟边界
	d, ok, err := CronNextDelay("*/5 * * * *", now)
	if err != nil || !ok {
		t.Fatalf("合法表达式不应报错，ok=%v err=%v", ok, err)
	}
	if want := time.Date(2026, 1, 5, 10, 5, 0, 0, time.UTC).Sub(now); d != want {
		t.Errorf("*/5 期望等待 %v，实际 %v", want, d)
	}

	// 每天 08:30：当前 10:03 已过，应为次日 08:30
	d, ok, err = CronNextDelay("30 8 * * *", now)
	if err != nil || !ok {
		t.Fatalf("合法表达式不应报错，ok=%v err=%v", ok, err)
	}
	if want := time.Date(2026, 1, 6, 8, 30, 0, 0, time.UTC).Sub(now); d != want {
		t.Errorf("30 8 * * * 期望等待 %v，实际 %v", want, d)
	}

	// @every 简写
	d, ok, err = CronNextDelay("@every 30m", now)
	if err != nil || !ok {
		t.Fatalf("@every 不应报错，ok=%v err=%v", ok, err)
	}
	if d != 30*time.Minute {
		t.Errorf("@every 30m 期望等待 30m，实际 %v", d)
	}
}

func TestCronNextDelayEmptySpec(t *testing.T) {
	// 空串/纯空白＝未配置定时，走空转分支（ok=false，无错误）
	for _, spec := range []string{"", "   "} {
		d, ok, err := CronNextDelay(spec, time.Date(2026, 1, 5, 10, 3, 17, 0, time.UTC))
		if err != nil {
			t.Errorf("空串不应报错，实际 %v", err)
		}
		if ok {
			t.Errorf("空串应返回 ok=false，spec=%q", spec)
		}
		if d != 0 {
			t.Errorf("空串等待时长应为 0，实际 %v", d)
		}
	}
}

func TestCronNextDelayInvalidSpec(t *testing.T) {
	for _, spec := range []string{"99 * * * *", "not-a-cron"} {
		d, ok, err := CronNextDelay(spec, time.Date(2026, 1, 5, 10, 3, 17, 0, time.UTC))
		if err == nil {
			t.Errorf("非法表达式应返回错误，spec=%q", spec)
		}
		if ok {
			t.Errorf("非法表达式应返回 ok=false，spec=%q", spec)
		}
		if d != 0 {
			t.Errorf("非法表达式等待时长应为 0，实际 %v", d)
		}
	}
}

// 合法但永不触发的表达式：robfig ParseStandard 通过、Next 返回零时刻，
// 若不防护会得到负数等待时长，后台循环 time.After(负值) 立即触发 → 热空转。
func TestCronNextDelayNeverFiring(t *testing.T) {
	for _, spec := range []string{"0 0 30 2 *", "0 0 31 4 *"} {
		d, ok, err := CronNextDelay(spec, time.Date(2026, 1, 5, 10, 3, 17, 0, time.UTC))
		if err == nil {
			t.Errorf("永不触发表达式应返回错误，spec=%q", spec)
		}
		if ok {
			t.Errorf("永不触发表达式应返回 ok=false，spec=%q", spec)
		}
		if d != 0 {
			t.Errorf("永不触发表达式等待时长应为 0，实际 %v", d)
		}
	}
}

// @every 间隔下限 1 分钟（延续旧 *_interval_seconds 的 60s 下限）：
// robfig 会把 <1s 向上取整为 1 秒节奏，不设下限则全量同步/轮询可被配置成秒级。
func TestCronNextDelayEveryFloor(t *testing.T) {
	for _, spec := range []string{"@every 0s", "@every 500ms", "@every 1s", "@every 30s"} {
		if _, ok, err := CronNextDelay(spec, time.Date(2026, 1, 5, 10, 3, 17, 0, time.UTC)); err == nil || ok {
			t.Errorf("@every 低于 1 分钟应按非法处理，spec=%q ok=%v err=%v", spec, ok, err)
		}
	}
	// 边界值放行
	if _, ok, err := CronNextDelay("@every 1m", time.Date(2026, 1, 5, 10, 3, 17, 0, time.UTC)); err != nil || !ok {
		t.Errorf("@every 1m 应放行，ok=%v err=%v", ok, err)
	}
}
