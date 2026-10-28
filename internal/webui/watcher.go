// watcher.go —— 后台定时任务（对应 webui/services/watcher.py）：
// 115 云下载自动整理 + RSS 订阅轮询 + STRM 定时同步，统一按 cron 表达式调度。
package webui

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"

	"meta2cloud/internal/config"
	"meta2cloud/internal/u115"
)

// cronNextDelay 计算距下一次 cron 触发的等待时长（服务器本地时区，5 段式，支持 @every 简写）。
// spec 为空串返回 ok=false（未配置定时）；表达式非法返回 err。
// 另两类畸形同样按非法处理（否则 time.After(负值/1s) 会让后台循环热空转）：
//   - 合法但永不触发的表达式（如 0 0 30 2 *、0 0 31 4 *）：robfig Next 返回零时刻，
//     Sub 出负数等待，循环会被立即反复触发；
//   - @every 间隔小于 1 分钟（robfig 向上取整后可打出秒级节奏，如 @every 500ms ≙ 1s），
//     延续旧 *_interval_seconds 的 60s 下限，防止全量同步/轮询被打成秒级。
//
// 校验（validateCronField）与四个后台循环共用本函数，行为保持一致。
func cronNextDelay(spec string, now time.Time) (time.Duration, bool, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0, false, nil
	}
	sched, err := cron.ParseStandard(spec)
	if err != nil {
		return 0, false, err
	}
	if delay, isDelay := sched.(cron.ConstantDelaySchedule); isDelay && delay.Delay < time.Minute {
		return 0, false, fmt.Errorf("@every 间隔不得小于 1 分钟（当前 %v）", delay.Delay)
	}
	next := sched.Next(now)
	if next.IsZero() || next.Sub(now) <= 0 {
		return 0, false, fmt.Errorf("表达式合法但永远不触发（如 2 月 30 日这类不存在的日期）")
	}
	return next.Sub(now), true, nil
}

type watcherState struct {
	mu                sync.Mutex
	stopCh            chan struct{}
	stopped           bool
	lastPolledAt      *string
	lastPollError     *string
	lastTriggeredTask map[string]any
	lastSummary       map[string]any
}

var (
	u115AutoWatcher = &watcherState{stopCh: make(chan struct{})}
	rssWatcher      = &watcherState{stopCh: make(chan struct{})}
	strmWatcher     = &watcherState{stopCh: make(chan struct{})}
)

// u115TaskKey 任务稳定键（info_hash → file_id → 名称+时间+大小）。
func u115TaskKey(task u115.OfflineTask) string {
	if task.InfoHash != "" {
		return "info_hash:" + task.InfoHash
	}
	if task.FileID != nil {
		return "file_id:" + *task.FileID
	}
	addTime := int64(0)
	if task.AddTime != nil {
		addTime = *task.AddTime
	}
	return fmt.Sprintf("name:%s|add:%d|size:%d", task.Name, addTime, task.Size)
}

func parseUTCTime(s *string) (time.Time, bool) {
	if s == nil || *s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		"2006-01-02T15:04:05.000000-07:00", "2006-01-02T15:04:05-07:00", time.RFC3339,
	} {
		if t, err := time.Parse(layout, *s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func utcNowISO() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000000-07:00") }

// pollU115AutoOrganizeOnce 单次轮询 115 云下载任务完成状态。
func pollU115AutoOrganizeOnce() {
	cfg := RT().GetConfig()
	u115AutoWatcher.mu.Lock()
	nowStr := utcNowISO()
	u115AutoWatcher.lastPolledAt = &nowStr
	u115AutoWatcher.lastPollError = nil
	u115AutoWatcher.mu.Unlock()

	if cfg.ActiveStorageName() != "pan115" || !cfg.U115.AutoOrganizeEnabled || cfg.U115.DownloadFolderID == "" {
		return
	}
	auth := RT().u115OauthStatusPayload()
	if authorized, _ := auth["authorized"].(bool); !authorized {
		// 最常见失效形态：本地 token 过期且自动刷新被拒——调 API 前即上报失效，
		// 避免早退导致永不告警（token 文件缺失等未配置形态不在此告警，防止误报）；
		// 刷新成功/未过期则继续轮询，恢复上报由 GetTaskList 成功点负责，此处不误报 OK
		if expired, _ := auth["token_expired"].(bool); expired {
			reason := "115 授权已失效（刷新被拒）"
			if refreshErr := anyStrValue(auth["refresh_error"]); refreshErr != "" {
				reason += "：" + refreshErr
			}
			NotifyServiceHealth("115", false, reason)
		}
		return
	}
	offline := RT().offlineClient()
	if offline == nil {
		return
	}
	// 配置值为云下载目录名，按名解析为目录 ID（兼容旧 ID 值）后再与任务的 WpPathID 比较
	dlFolderID := resolveFolderID(cfg.U115.DownloadFolderID)
	taskPage, err := offline.GetTaskList(1)
	if err != nil {
		recordU115PollError(err)
		return
	}
	NotifyServiceHealth("115", true, "")
	stableSeconds := cfg.U115.AutoOrganizeStableSeconds
	if stableSeconds < 0 {
		stableSeconds = 0
	}
	now := time.Now()
	autoStore := RT().GetU115AutoStore()
	var completed []u115.OfflineTask

	for _, task := range taskPage.Tasks {
		if task.WpPathID != nil && *task.WpPathID != "" && *task.WpPathID != dlFolderID {
			continue
		}
		state := autoStore.UpsertObservation(
			u115TaskKey(task), task.InfoHash, task.Name, task.Status, task.PercentDone,
			task.WpPathID, task.FileID, task.IsFinished())
		if !task.IsFinished() || state.TriggeredAt != nil || state.FinishedSeenAt == nil {
			continue
		}
		if finishedAt, ok := parseUTCTime(state.FinishedSeenAt); ok {
			if now.Sub(finishedAt).Seconds() < float64(stableSeconds) {
				continue
			}
		}
		completed = append(completed, task)
	}
	if len(completed) == 0 {
		return
	}

	if !RT().SchedulePipeline(cfg.Telegram.DebounceSeconds) {
		RT().appLog("u115", "auto_organize_deferred", "WARNING",
			"检测到 115 云下载任务完成，但当前整理流程忙碌，等待后续轮询重试", map[string]any{
				"count": len(completed), "tasks": summarizeTasks(completed),
			})
		return
	}
	for _, task := range completed {
		autoStore.MarkTriggered(u115TaskKey(task))
	}
	triggeredPayload := map[string]any{
		"triggered_at": utcNowISO(), "count": len(completed),
		"tasks": summarizeTasks(completed),
	}
	u115AutoWatcher.mu.Lock()
	u115AutoWatcher.lastTriggeredTask = triggeredPayload
	u115AutoWatcher.mu.Unlock()
	RT().appLog("u115", "auto_organize_triggered", "SUCCESS",
		fmt.Sprintf("检测到 %d 个 115 云下载任务完成，已触发自动整理", len(completed)), triggeredPayload)
}

func summarizeTasks(tasks []u115.OfflineTask) []map[string]any {
	out := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, map[string]any{
			"info_hash": task.InfoHash, "name": task.Name,
			"file_id": task.FileID, "wp_path_id": task.WpPathID,
		})
	}
	return out
}

func recordU115PollError(err error) {
	msg := ""
	if u115.IsRateLimit(err) {
		msg = "115 自动整理轮询进入冷却：" + err.Error()
	} else {
		msg = fmt.Sprintf("[%T] %v", err, err)
	}
	// 仅授权失效属于配置问题（需重新授权）；限流/网络抖动等瞬时错误不告警
	if u115.IsAuth(err) {
		NotifyServiceHealth("115", false, "自动整理轮询认证失败："+err.Error())
	}
	u115AutoWatcher.mu.Lock()
	u115AutoWatcher.lastPollError = &msg
	u115AutoWatcher.mu.Unlock()
}

// rssHealthState RSS 轮询健康判定：连续 2 次 summary.errors（抓取错误数）> 0 才视为
// 订阅源失效（推送类错误单列 push_errors，不计入；窗口状态仅存于 watcher 循环局部，
// 进程重启即清零），任一次干净轮询即恢复。
type rssHealthState struct{ consecutiveErrors int }

// update 依据本轮轮询 summary 更新计数，返回是否需要上报（report）、结论（ok）与失效原因。
func (s *rssHealthState) update(summary map[string]any) (report, ok bool, reason string) {
	errCount, _ := summary["errors"].(int)
	if errCount <= 0 {
		s.consecutiveErrors = 0
		return true, true, ""
	}
	s.consecutiveErrors++
	if s.consecutiveErrors < 2 {
		return false, false, ""
	}
	reason, _ = summary["first_error"].(string)
	return true, false, reason
}

// ── cron 调度管理（开关开启才存在执行循环）──────────────────

// schedulesStopped 全局停止后不再随配置变更重启（测试/关机路径）。
var schedulesStopped atomic.Bool

// cronLoop 单个功能的 cron 调度生命周期管理：开关开启才启动执行循环，
// 关闭即停（睡眠中的循环经 stop 立即退出，不等到触发点）。
type cronLoop struct {
	name    string
	mu      sync.Mutex
	stopCh  chan struct{}
	running bool
	halted  bool // 全局停止后不再重启
}

// apply 依开关状态启停执行循环（幂等；可在任意 goroutine 调用）。
func (m *cronLoop) apply(start bool, run func(stop <-chan struct{})) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.halted {
		return
	}
	if !start {
		if m.running {
			close(m.stopCh)
			m.running = false
		}
		return
	}
	if m.running {
		return
	}
	m.stopCh = make(chan struct{})
	stop := m.stopCh
	m.running = true
	go func() {
		defer func() {
			m.mu.Lock()
			if m.running && m.stopCh == stop {
				m.running = false
			}
			m.mu.Unlock()
		}()
		run(stop)
	}()
}

// halt 全局停止：此后 apply 不再启动。
func (m *cronLoop) halt() {
	m.mu.Lock()
	m.halted = true
	if m.running {
		close(m.stopCh)
		m.running = false
	}
	m.mu.Unlock()
}

var (
	traktCronLoop = &cronLoop{name: "观看同步"}
	strmCronLoop  = &cronLoop{name: "STRM 定时同步"}
	u115CronLoop  = &cronLoop{name: "115 自动整理"}
	rssCronLoop   = &cronLoop{name: "订阅轮询"}
)

// ApplyWatchSchedules 依当前配置启停各 cron 调度（配置变更检测点调用，幂等）：
// 开关开启才存在执行循环，关闭即停。
func ApplyWatchSchedules(cfg *config.Config) {
	if schedulesStopped.Load() {
		return
	}
	traktCronLoop.apply(cfg.Trakt.Enabled && cfg.Trakt.SyncCron != "", runTraktCron)
	strmCronLoop.apply(cfg.Strm.Enabled && cfg.ActiveStorageName() == "pan115" && cfg.Strm.SyncCron != "", runStrmCron)
	u115CronLoop.apply(cfg.ActiveStorageName() == "pan115" && cfg.U115.AutoOrganizeEnabled && cfg.U115.DownloadFolderID != "" && cfg.U115.AutoOrganizeCron != "", runU115Cron)
	rssCronLoop.apply(RT().getSubscriptionPollCron() != "", runRSSCron)
}

// runTraktCron 观看同步 cron 执行器。配对未完成（token 等不随配置文件变更）
// 时跳过本轮并提示，下个 cron 点重试。
func runTraktCron(stop <-chan struct{}) {
	RT().appLog("trakt", "sync_schedule_started", "INFO", "观看同步定时线程已启动", nil)
	cronWarned := ""
	readyWarned := false
	for {
		spec := RT().GetConfig().Trakt.SyncCron
		d, ok, err := cronNextDelay(spec, time.Now())
		if err != nil {
			if cronWarned != spec {
				RT().appLog("trakt", "sync_cron_invalid", "ERROR",
					"观看同步 cron 表达式无效，修正配置后自动恢复："+err.Error(), map[string]any{"cron": spec})
				cronWarned = spec
			}
			return // 修正配置触发重载后由调度管理重启
		}
		if !ok {
			return // 未配置 cron：无可调度内容
		}
		timer := time.NewTimer(d)
		select {
		case <-stop:
			timer.Stop()
			RT().appLog("trakt", "sync_schedule_stopped", "INFO", "观看同步定时线程已停止", nil)
			return
		case <-timer.C:
		}
		nowStr := utcNowISO()
		traktWatcher.mu.Lock()
		traktWatcher.lastPolledAt = &nowStr
		traktWatcher.mu.Unlock()
		cfg := RT().GetConfig()
		if !cfg.IsPlexReady() || !cfg.IsTraktReady() {
			if !readyWarned {
				RT().appLog("trakt", "sync_schedule_skipped", "INFO",
					"观看同步定时触发跳过（Plex/Trakt 配对未完成），下个 cron 点重试", nil)
				readyWarned = true
			}
			continue
		}
		readyWarned = false
		if ready, halted := RT().waitPlexReachable(stop); halted {
			return
		} else if ready {
			RT().TriggerTraktSync(triggerSchedule)
		}
	}
}

// runStrmCron STRM 定时同步 cron 执行器。
func runStrmCron(stop <-chan struct{}) {
	RT().appLog("strm", "strm_sync_started", "INFO", "STRM 定时同步线程已启动", nil)
	cronWarned := ""
	for {
		spec := RT().GetConfig().Strm.SyncCron
		d, ok, err := cronNextDelay(spec, time.Now())
		if err != nil {
			if cronWarned != spec {
				RT().appLog("strm", "sync_cron_invalid", "ERROR",
					"STRM 定时同步 cron 表达式无效，修正配置后自动恢复："+err.Error(), map[string]any{"cron": spec})
				cronWarned = spec
			}
			return
		}
		if !ok {
			return
		}
		timer := time.NewTimer(d)
		select {
		case <-stop:
			timer.Stop()
			RT().appLog("strm", "strm_sync_stopped", "INFO", "STRM 定时同步线程已停止", nil)
			return
		case <-timer.C:
		}
		// 触发点现读配置（执行期间配置可能已变更）
		cur := RT().GetConfig()
		if cur.Strm.Enabled && cur.ActiveStorageName() == "pan115" && !RT().StrmSyncRunning() {
			RT().TriggerStrmSync(triggerSchedule)
		}
	}
}

// runU115Cron 115 自动整理 cron 执行器。
func runU115Cron(stop <-chan struct{}) {
	RT().appLog("u115", "u115_auto_started", "INFO", "115 自动整理监听线程已启动", nil)
	cronWarned := ""
	for {
		spec := RT().GetConfig().U115.AutoOrganizeCron
		d, ok, err := cronNextDelay(spec, time.Now())
		if err != nil {
			if cronWarned != spec {
				RT().appLog("u115", "auto_organize_cron_invalid", "ERROR",
					"自动整理 cron 表达式无效，修正配置后自动恢复："+err.Error(), map[string]any{"cron": spec})
				cronWarned = spec
			}
			return
		}
		if !ok {
			return
		}
		timer := time.NewTimer(d)
		select {
		case <-stop:
			timer.Stop()
			RT().appLog("u115", "u115_auto_stopped", "INFO", "115 自动整理监听线程已停止", nil)
			return
		case <-timer.C:
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					msg := fmt.Sprintf("[panic] %v", r)
					u115AutoWatcher.mu.Lock()
					u115AutoWatcher.lastPollError = &msg
					u115AutoWatcher.mu.Unlock()
					RT().appLog("u115", "auto_organize_poll_failed", "ERROR",
						"115 自动整理轮询失败: "+msg, map[string]any{"error": msg})
				}
			}()
			pollU115AutoOrganizeOnce()
		}()
	}
}

// runRSSCron 订阅轮询 cron 执行器。
func runRSSCron(stop <-chan struct{}) {
	RT().appLog("rss", "rss_poll_started", "INFO", "RSS 订阅监听线程已启动", nil)
	cronWarned := ""
	rssHealth := rssHealthState{} // 连续失败计数（本次调度生命周期内）
	for {
		spec := RT().getSubscriptionPollCron()
		d, ok, err := cronNextDelay(spec, time.Now())
		if err != nil {
			if cronWarned != spec {
				RT().appLog("rss", "poll_cron_invalid", "ERROR",
					"订阅轮询 cron 表达式无效，修正配置后自动恢复："+err.Error(), map[string]any{"cron": spec})
				cronWarned = spec
			}
			return
		}
		if !ok {
			return
		}
		timer := time.NewTimer(d)
		select {
		case <-stop:
			timer.Stop()
			RT().appLog("rss", "rss_poll_stopped", "INFO", "RSS 订阅监听线程已停止", nil)
			return
		case <-timer.C:
		}
		nowStr := utcNowISO()
		rssWatcher.mu.Lock()
		rssWatcher.lastPolledAt = &nowStr
		rssWatcher.lastPollError = nil
		rssWatcher.mu.Unlock()
		summary := RT().PollSubscriptionsOnce()
		rssWatcher.mu.Lock()
		rssWatcher.lastSummary = summary
		rssWatcher.mu.Unlock()
		if report, ok2, reason := rssHealth.update(summary); report {
			NotifyServiceHealth("订阅源", ok2, reason)
		}
	}
}

// StartupBackgroundWatchers 启动全部后台任务：非 cron 类监听线程常驻，
// cron 类调度由配置变更检测按开关启停（见 ApplyWatchSchedules）。
func StartupBackgroundWatchers() {
	RT().ensureTraktSessionLoop()
	RT().ensureTraktSSELoop()
	ApplyWatchSchedules(RT().GetConfig())
}

// StopBackgroundWatchers 停止全部后台监听线程（幂等）。
// 供 Server.Close 与测试清理调用，避免线程在临时目录清理后仍惰性重建
// config/data（测试偶发 TempDir 清理竞态的根因）。
// 注意：停止后当前进程内 cron 调度不会随配置变更重启；
// 生产环境进程生命周期即线程生命周期，不受影响。
func StopBackgroundWatchers() {
	schedulesStopped.Store(true)
	traktCronLoop.halt()
	strmCronLoop.halt()
	u115CronLoop.halt()
	rssCronLoop.halt()
	for _, w := range []*watcherState{u115AutoWatcher, rssWatcher, strmWatcher, traktWatcher, traktSessionWatcher, traktSSEWatcher} {
		w.mu.Lock()
		if !w.stopped {
			w.stopped = true
			close(w.stopCh)
		}
		w.mu.Unlock()
	}
}

// U115AutoOrganizeStatusPayload 115 自动整理状态。
func U115AutoOrganizeStatusPayload() map[string]any {
	cfg := RT().GetConfig()
	autoStore := RT().GetU115AutoStore()
	var lastTriggered map[string]any
	if autoStore != nil {
		if last := autoStore.GetLastTriggered(); last != nil {
			lastTriggered = map[string]any{
				"triggered_at": last.TriggeredAt, "count": 1,
				"tasks": []map[string]any{{
					"info_hash": last.InfoHash, "name": last.TaskName,
					"file_id": last.FileID, "wp_path_id": last.WpPathID,
				}},
			}
		}
	}
	u115AutoWatcher.mu.Lock()
	if u115AutoWatcher.lastTriggeredTask != nil {
		lastTriggered = u115AutoWatcher.lastTriggeredTask
	}
	lastPolled := u115AutoWatcher.lastPolledAt
	lastErr := u115AutoWatcher.lastPollError
	u115AutoWatcher.mu.Unlock()

	authorized, _ := RT().u115OauthStatusPayload()["authorized"].(bool)
	return map[string]any{
		"enabled":            cfg.U115.AutoOrganizeEnabled && cfg.ActiveStorageName() == "pan115",
		"storage_primary":    cfg.ActiveStorageName(),
		"authorized":         authorized,
		"watcher_alive":      watcherAlive(u115AutoWatcher),
		"poll_cron":          cfg.U115.AutoOrganizeCron,
		"stable_seconds":     cfg.U115.AutoOrganizeStableSeconds,
		"download_folder_id": cfg.U115.DownloadFolderID,
		"last_polled_at":     lastPolled,
		"last_poll_error":    lastErr,
		"last_triggered":     lastTriggered,
	}
}

func watcherAlive(w *watcherState) bool {
	select {
	case <-w.stopCh:
		return false
	default:
		return true
	}
}
