// health_test.go —— 服务健康状态机（失效/恢复通知、24h 冷却、TG 降级）与
// RSS 连续失败判定（连续 2 次）的单测。
package webui

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"meta2cloud/internal/config"
	"meta2cloud/internal/pipeline"
	"meta2cloud/internal/scraper"
	"meta2cloud/internal/u115"
	"meta2cloud/internal/webui/store"
)

// fakeClock 可手动推进的时钟（rt.healthNow 注入用）。
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock(start time.Time) *fakeClock { return &fakeClock{t: start} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// tgRecorder 记录健康通知发送（healthSendTG 注入用）。
type tgRecorder struct {
	mu    sync.Mutex
	texts []string
	tries int
	err   error // 非 nil 时模拟发送失败
}

func (r *tgRecorder) send(_ config.Telegram, text string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tries++
	if r.err != nil {
		return r.err
	}
	r.texts = append(r.texts, text)
	return nil
}

func (r *tgRecorder) sent() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.texts...)
}

func (r *tgRecorder) attempts() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tries
}

// setupHealthTest 初始化运行时、清空健康状态并注入可替换依赖；返回发送记录器。
func setupHealthTest(t *testing.T) (*tgRecorder, *Runtime) {
	t.Helper()
	rt := InitRuntime(t.TempDir())
	rt.resetHealthState()
	rec := &tgRecorder{}
	rt.healthSendTG = rec.send
	rt.healthTGConfig = func() config.Telegram { return config.Telegram{BotToken: "tok", ChatID: "cid"} }
	t.Cleanup(func() {
		rt.healthSendTG = pipeline.SendTelegramMessage
		rt.healthTGConfig = func() config.Telegram { return rt.GetConfig().Telegram }
		rt.healthNow = time.Now
		rt.resetHealthState()
	})
	return rec, rt
}

func (rt *Runtime) resetHealthState() {
	rt.healthMu.Lock()
	defer rt.healthMu.Unlock()
	rt.healthServices = map[string]*serviceHealth{}
}

func healthLogEvents(rt *Runtime, category string) []store.LogEntry {
	ls := rt.GetLogStore()
	if ls == nil {
		return nil
	}
	return ls.Read(200, category, "", 0)
}

// TestNotifyServiceHealthStateMachine 失效边沿发一次、24h 内不重复、跨 24h 再提醒、恢复边沿。
func TestNotifyServiceHealthStateMachine(t *testing.T) {
	rec, rt := setupHealthTest(t)
	clock := newFakeClock(time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC))
	rt.healthNow = clock.Now

	// 失效边沿：发一次失效通知
	rt.NotifyServiceHealth("115", false, "token 已过期")
	if got := rec.sent(); len(got) != 1 {
		t.Fatalf("失效边沿应发 1 条通知，实际 %d：%v", len(got), got)
	} else if !containsAll(got[0], "⚠️", "115", "配置失效", "token 已过期") {
		t.Errorf("失效通知文案不完整：%q", got[0])
	}

	// 持续失效（24h 内）：不重复通知
	clock.Advance(23 * time.Hour)
	rt.NotifyServiceHealth("115", false, "token 仍失效")
	if got := rec.sent(); len(got) != 1 {
		t.Fatalf("24h 内持续失效不应重复通知，实际 %d：%v", len(got), got)
	}

	// 跨过 24h：再提醒一次
	clock.Advance(2 * time.Hour)
	rt.NotifyServiceHealth("115", false, "token 仍失效")
	if got := rec.sent(); len(got) != 2 {
		t.Fatalf("跨 24h 应再提醒一次，实际 %d：%v", len(got), got)
	}

	// 恢复边沿：发恢复通知
	clock.Advance(1 * time.Hour)
	rt.NotifyServiceHealth("115", true, "")
	if got := rec.sent(); len(got) != 3 || !containsAll(got[2], "✅", "115", "已恢复") {
		t.Fatalf("恢复边沿应发恢复通知，实际 %d：%v", len(got), got)
	}

	// 持续正常：无操作
	rt.NotifyServiceHealth("115", true, "")
	rt.NotifyServiceHealth("115", true, "")
	if got := rec.sent(); len(got) != 3 {
		t.Fatalf("持续正常不应发通知，实际 %d：%v", len(got), got)
	}

	// 应用日志可见：失效 ERROR 与恢复 INFO
	logs := healthLogEvents(rt, "u115")
	var failedEvt, recoveredEvt bool
	for _, e := range logs {
		switch e.Event {
		case "health_failed":
			if e.Level == "ERROR" {
				failedEvt = true
			}
		case "health_recovered":
			if e.Level == "INFO" {
				recoveredEvt = true
			}
		}
	}
	if !failedEvt || !recoveredEvt {
		t.Errorf("应用日志应含 health_failed(ERROR) 与 health_recovered(INFO)，实际 %+v", logs)
	}
}

// TestNotifyServiceHealthCooldownBoundary 恰好达到 24h 冷却（>= 语义）应再次提醒，
// 差 1 分钟则仍在冷却内不提醒。
func TestNotifyServiceHealthCooldownBoundary(t *testing.T) {
	rec, rt := setupHealthTest(t)
	clock := newFakeClock(time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC))
	rt.healthNow = clock.Now

	rt.NotifyServiceHealth("Plex", false, "token 失效") // 失效边沿：立即通知
	clock.Advance(healthNotifyCooldown - time.Minute)
	rt.NotifyServiceHealth("Plex", false, "token 仍失效")
	if got := rec.sent(); len(got) != 1 {
		t.Fatalf("冷却差 1 分钟不应重复通知，实际 %d：%v", len(got), got)
	}

	clock.Advance(time.Minute) // 恰好 24h
	rt.NotifyServiceHealth("Plex", false, "token 仍失效")
	if got := rec.sent(); len(got) != 2 || !containsAll(got[1], "token 仍失效") {
		t.Fatalf("恰好达到 24h（>= 语义）应再提醒一次，实际 %d：%v", len(got), got)
	}
}

// TestNotifyServiceHealthPerServiceIndependent 各服务状态互不影响。
func TestNotifyServiceHealthPerServiceIndependent(t *testing.T) {
	rec, rt := setupHealthTest(t)
	rt.healthNow = newFakeClock(time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)).Now

	rt.NotifyServiceHealth("Plex", false, "token 失效")
	rt.NotifyServiceHealth("Trakt", true, "") // 正常服务上报无操作
	rt.NotifyServiceHealth("Trakt", false, "刷新被拒")
	rt.NotifyServiceHealth("订阅源", false, "连续两次抓取失败")

	got := rec.sent()
	if len(got) != 3 {
		t.Fatalf("应发 3 条失效通知，实际 %d：%v", len(got), got)
	}
	wantServices := []string{"Plex", "Trakt", "订阅源"}
	for i, svc := range wantServices {
		if !containsAll(got[i], "⚠️", svc) {
			t.Errorf("第 %d 条通知应属于 %s：%q", i, svc, got[i])
		}
	}
}

// TestNotifyServiceHealthTGUnconfigured TG 未配置时静默降级为仅应用日志。
func TestNotifyServiceHealthTGUnconfigured(t *testing.T) {
	rec, rt := setupHealthTest(t)
	rt.healthTGConfig = func() config.Telegram { return config.Telegram{} } // 未配置
	rt.healthNow = newFakeClock(time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)).Now

	rt.NotifyServiceHealth("Plex", false, "token 失效")
	rt.NotifyServiceHealth("Plex", true, "")
	if got := rec.sent(); len(got) != 0 || rec.attempts() != 0 {
		t.Fatalf("TG 未配置不应尝试发送，实际尝试 %d 次：%v", rec.attempts(), got)
	}
	// 失效/恢复均应留应用日志
	events := map[string]bool{}
	for _, e := range healthLogEvents(rt, "plex") {
		events[e.Event] = true
	}
	if !events["health_failed"] || !events["health_recovered"] {
		t.Errorf("TG 未配置时仍应写应用日志，实际 %+v", events)
	}
}

// TestNotifyServiceHealthTGSendFailure TG 发送失败只记 WARN 日志，不影响状态机（不重复发送）。
func TestNotifyServiceHealthTGSendFailure(t *testing.T) {
	rec, rt := setupHealthTest(t)
	rec.err = errors.New("网络不可达")
	rt.healthNow = newFakeClock(time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)).Now

	rt.NotifyServiceHealth("Trakt", false, "刷新被拒")
	rt.NotifyServiceHealth("Trakt", false, "刷新仍被拒") // 24h 内：不再尝试发送
	if rec.attempts() != 1 {
		t.Fatalf("发送失败后状态机仍应记冷却，24h 内只应尝试 1 次，实际 %d", rec.attempts())
	}
	var warnSeen bool
	for _, e := range healthLogEvents(rt, "trakt") {
		if e.Event == "health_notify_failed" && e.Level == "WARNING" {
			warnSeen = true
		}
	}
	if !warnSeen {
		t.Error("TG 发送失败应记 health_notify_failed(WARN) 应用日志")
	}
}

// TestNotifyServiceHealthConcurrency 多循环 goroutine 并发上报不死锁、不 panic（-race 验证）。
func TestNotifyServiceHealthConcurrency(t *testing.T) {
	rec, rt := setupHealthTest(t)
	rt.healthNow = newFakeClock(time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)).Now

	services := []string{"115", "Plex", "Trakt", "订阅源"}
	var wg sync.WaitGroup
	for i, svc := range services {
		for j := 0; j < 20; j++ {
			wg.Add(1)
			go func(svc string, j, i int) {
				defer wg.Done()
				if (i+j)%2 == 0 {
					rt.NotifyServiceHealth(svc, false, fmt.Sprintf("并发原因 %d", j))
				} else {
					rt.NotifyServiceHealth(svc, true, "")
				}
			}(svc, j, i)
		}
	}
	wg.Wait()

	// 收尾：全部恢复后再次上报正常，不应产生任何新通知（稳定不变量）
	for _, svc := range services {
		rt.NotifyServiceHealth(svc, true, "")
	}
	settled := len(rec.sent())
	for _, svc := range services {
		rt.NotifyServiceHealth(svc, true, "")
	}
	if got := len(rec.sent()); got != settled {
		t.Errorf("全部恢复后继续上报正常仍有新通知：%d → %d", settled, got)
	}
}

// TestPollSubscriptionsOnceCarriesFirstError 轮询失败时 summary 携带首条失败原因，供健康判定上报。
func TestPollSubscriptionsOnceCarriesFirstError(t *testing.T) {
	_, rt := setupHealthTest(t)
	rssStore := rt.GetRSSStore()
	if rssStore == nil {
		t.Fatal("RSS 存储不可用")
	}
	// mikan 链接缺少 bangumiId：确定性失败，不依赖外网
	if _, err := rssStore.CreateSubscription(map[string]any{
		"name": "坏链订阅", "media_title": "测试", "media_type": "tv",
		"site": "mikan", "rss_url": "https://mikanani.me/RSS/Classic",
		"subgroup_name": "TestGroup", "keyword_all": "[]",
		"push_target": "u115", "enabled": true,
	}); err != nil {
		t.Fatal(err)
	}
	summary := rt.PollSubscriptionsOnce()
	if errs, _ := summary["errors"].(int); errs != 1 {
		t.Fatalf("errors = %v, want 1", summary["errors"])
	}
	if fe, _ := summary["first_error"].(string); fe == "" {
		t.Errorf("first_error 应携带首条失败原因，实际 %q", summary["first_error"])
	}
}

// TestRSSHealthState 连续 2 次 errors>0 才判定失效；任一次干净轮询即恢复。
func TestRSSHealthState(t *testing.T) {
	var st rssHealthState

	// 第 1 次失败：不判定（窗口未满）
	if report, _, _ := st.update(map[string]any{"errors": 1, "first_error": "e1"}); report {
		t.Fatal("第 1 次失败不应上报")
	}
	// 第 2 次失败：判定失效并携带最新失败原因
	report, ok, reason := st.update(map[string]any{"errors": 1, "first_error": "e2"})
	if !report || ok || reason != "e2" {
		t.Fatalf("连续第 2 次失败应上报失效：report=%v ok=%v reason=%q", report, ok, reason)
	}
	// 第 3 次仍失败：继续上报失效
	if report, ok, _ := st.update(map[string]any{"errors": 2, "first_error": "e3"}); !report || ok {
		t.Fatal("持续失败应继续上报失效")
	}
	// 一次干净轮询：立即恢复
	if report, ok, _ := st.update(map[string]any{"errors": 0, "first_error": ""}); !report || !ok {
		t.Fatal("干净轮询应上报恢复")
	}
	// 推送类错误不计入（errors=0、push_errors=1）：与干净轮询同判（恢复），不误报订阅源失效
	if report, ok, _ := st.update(map[string]any{"errors": 0, "push_errors": 1, "first_error": ""}); !report || !ok {
		t.Fatal("仅推送类错误应视为干净轮询（恢复）")
	}
	// 恢复后单次失败：重新从 0 计数
	if report, _, _ := st.update(map[string]any{"errors": 1, "first_error": "e4"}); report {
		t.Fatal("恢复后首败不应立即上报")
	}
}

// ── 订阅推送错误归因（缺口：推送失败误归因订阅源） ──────────

// fakeMikanSpider 可编程 mikan 爬虫（注入确定性条目，测试不访问外网）。
type fakeMikanSpider struct{ items []scraper.MagnetItem }

func (s fakeMikanSpider) SiteID() string                                  { return "mikan" }
func (s fakeMikanSpider) SiteName() string                                { return "Mikan（测试）" }
func (s fakeMikanSpider) RssHosts() []string                              { return []string{"mikanani.me"} }
func (s fakeMikanSpider) SearchMedia(string) ([]scraper.MediaItem, error) { return nil, nil }
func (s fakeMikanSpider) GetEpisodes(string, string) ([]scraper.MagnetItem, error) {
	return s.items, nil
}

// mikanTestSpider 取全局工厂中的 *MikanSpider。未注册或类型异常即 Fatal——
// 静默跳过会让工厂为空这类回归（原 bug）在远离病根处才暴露。
func mikanTestSpider(t *testing.T) *scraper.MikanSpider {
	t.Helper()
	spider, err := scraper.DefaultFactory().GetSpider("mikan")
	if err != nil {
		t.Fatalf("默认工厂未注册 mikan：%v", err)
	}
	mikan, ok := spider.(*scraper.MikanSpider)
	if !ok {
		t.Fatalf("mikan spider 类型异常：%T", spider)
	}
	return mikan
}

// useFakeMikanSpider 以假爬虫替换全局 mikan 注册，测试结束恢复原爬虫。
func useFakeMikanSpider(t *testing.T, items []scraper.MagnetItem) {
	t.Helper()
	factory := scraper.DefaultFactory()
	prev, err := factory.GetSpider("mikan")
	if err != nil {
		t.Fatalf("默认工厂未注册 mikan：%v", err)
	}
	t.Cleanup(func() { factory.Register(prev) })
	factory.Register(fakeMikanSpider{items: items})
}

// TestPollSubscriptionsOncePushErrorsExcludedFromHealth 抓取成功 + 115 未授权推送失败时：
// summary.errors 必须为 0（健康判定只认抓取错误，连续失败计数不被推送错误污染）、
// push_errors 单列；「115 未授权」改路上报 115 服务健康，而非误报订阅源失效。
func TestPollSubscriptionsOncePushErrorsExcludedFromHealth(t *testing.T) {
	rec, rt := setupHealthTest(t)
	useFakeMikanSpider(t, []scraper.MagnetItem{{
		Title: "Test Anime S01E01 1080p", MagnetURL: "magnet:?xt=urn:btih:abc", Site: "mikan",
	}})
	rssStore := rt.GetRSSStore()
	if rssStore == nil {
		t.Fatal("RSS 存储不可用")
	}
	if _, err := rssStore.CreateSubscription(map[string]any{
		"name": "推送失败订阅", "media_title": "Test Anime", "media_type": "tv",
		"site": "mikan", "rss_url": "https://mikanani.me/RSS/Classic?bangumiId=333",
		"subgroup_name": "TestGroup", "keyword_all": "[]",
		"push_target": "u115", "enabled": true,
	}); err != nil {
		t.Fatal(err)
	}

	// 测试运行时无 115 token 文件：pushItem 必走「115 未授权」分支
	summary := rt.PollSubscriptionsOnce()
	if errs, _ := summary["errors"].(int); errs != 0 {
		t.Fatalf("推送失败不应计入 errors（健康判定只认抓取错误），实际 %v", summary["errors"])
	}
	if perrs, _ := summary["push_errors"].(int); perrs != 1 {
		t.Fatalf("push_errors = %v, want 1", summary["push_errors"])
	}
	got := rec.sent()
	if len(got) != 1 || !containsAll(got[0], "115", "订阅推送：115 未授权") {
		t.Fatalf("115 未授权推送失败应改路上报 115 服务健康，实际 %v", got)
	}

	// 同一份 summary 喂给订阅源健康判定应为干净轮询（立即恢复）
	if report, ok, reason := (&rssHealthState{}).update(summary); !report || !ok || reason != "" {
		t.Fatalf("仅推送失败应视为干净轮询：report=%v ok=%v reason=%q", report, ok, reason)
	}
}

// TestReportSubscriptionPushHealthClassification 推送失败健康归因：
// 115 授权类改路上报 115 服务；限流（429）/网络/条目类静默不告警。
func TestReportSubscriptionPushHealthClassification(t *testing.T) {
	rec, rt := setupHealthTest(t)

	// 115 未授权哨兵：失效边沿上报
	rt.reportSubscriptionPushHealth(errPush115Unauthorized)
	if got := rec.sent(); len(got) != 1 || !containsAll(got[0], "订阅推送：115 未授权") {
		t.Fatalf("115 未授权推送失败应上报 115 健康，实际 %v", got)
	}

	// 真实 API 返回的授权错误同样归类为 115 失效（同服务 24h 冷却内不重发，仅更新原因）
	rt.reportSubscriptionPushHealth(fmt.Errorf("推送云下载失败：%w",
		&u115.APIError{Kind: u115.KindAuth, Message: "token 失效"}))
	if got := rec.sent(); len(got) != 1 {
		t.Fatalf("冷却内重复上报不应再发送，实际 %v", got)
	}
	rt.healthMu.Lock()
	reason := rt.healthServices["115"].reason
	rt.healthMu.Unlock()
	if !strings.Contains(reason, "token 失效") {
		t.Fatalf("冷却内上报应更新失效原因，实际 %q", reason)
	}

	// 限流（115 429）/网络抖动/条目缺地址：静默
	rt.reportSubscriptionPushHealth(&u115.APIError{Kind: u115.KindRateLimit, Message: "已达到当前访问上限，进入冷却"})
	rt.reportSubscriptionPushHealth(errors.New("网络不可达"))
	rt.reportSubscriptionPushHealth(errors.New("RSS 条目缺少下载地址"))
	if got := rec.sent(); len(got) != 1 {
		t.Fatalf("限流/网络/条目类推送失败应静默，实际 %v", got)
	}
}

// containsAll 断言 s 包含 subs 中全部子串。
func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
