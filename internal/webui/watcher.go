// watcher.go —— 后台定时任务（对应 webui/services/watcher.py）：
// 115 云下载自动整理 + RSS 订阅轮询 + STRM 定时同步，统一按 cron 表达式调度。
package webui

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

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
	u115AutoOnce    sync.Once
	rssOnce         sync.Once
	strmOnce        sync.Once
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
	if authorized, _ := u115OauthStatusPayload()["authorized"].(bool); !authorized {
		return
	}
	offline := offlineClient()
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

	if !SchedulePipeline(cfg.Telegram.DebounceSeconds) {
		appLog("u115", "auto_organize_deferred", "WARNING",
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
	appLog("u115", "auto_organize_triggered", "SUCCESS",
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
	u115AutoWatcher.mu.Lock()
	u115AutoWatcher.lastPollError = &msg
	u115AutoWatcher.mu.Unlock()
}

// ensureU115AutoOrganizeLoop 启动 115 自动整理监听（幂等；按 u115.auto_organize_cron 计划轮询）。
func ensureU115AutoOrganizeLoop() {
	u115AutoOnce.Do(func() {
		go func() {
			log.Printf("115 自动整理监听线程已启动（按 u115.auto_organize_cron 计划轮询；未配置 cron 时不定时）")
			cronWarned := "" // 已告警过的非法 cron：仅状态变化时告警，避免每 60s 重复刷日志
			for {
				spec := RT().GetConfig().U115.AutoOrganizeCron
				d, ok, err := cronNextDelay(spec, time.Now())
				if err != nil {
					if cronWarned != spec {
						appLog("u115", "auto_organize_cron_invalid", "ERROR",
							"自动整理 cron 表达式无效，60s 后重试："+err.Error(), map[string]any{"cron": spec})
						log.Printf("115 自动整理 cron 表达式无效（%q），60s 后重试", spec)
						cronWarned = spec
					}
					ok = false
				} else {
					cronWarned = "" // 恢复正常后重置，再次非法时重新告警
				}
				if !ok {
					d = 60 * time.Second // 未配置 cron（不定时）或表达式非法：空转等待，保持配置热响应
				}
				select {
				case <-u115AutoWatcher.stopCh:
					log.Printf("115 自动整理监听线程已停止")
					return
				case <-time.After(d):
				}
				if !ok {
					continue
				}
				func() {
					defer func() {
						if r := recover(); r != nil {
							msg := fmt.Sprintf("[panic] %v", r)
							u115AutoWatcher.mu.Lock()
							u115AutoWatcher.lastPollError = &msg
							u115AutoWatcher.mu.Unlock()
							appLog("u115", "auto_organize_poll_failed", "ERROR",
								"115 自动整理轮询失败: "+msg, map[string]any{"error": msg})
						}
					}()
					pollU115AutoOrganizeOnce()
				}()
			}
		}()
	})
}

// ensureRSSSubscriptionLoop 启动 RSS 订阅轮询（幂等；按 rss.poll_cron 计划轮询）。
func ensureRSSSubscriptionLoop() {
	rssOnce.Do(func() {
		go func() {
			log.Printf("RSS 订阅监听线程已启动（按 rss.poll_cron 计划轮询；未配置 cron 时不定时）")
			cronWarned := "" // 已告警过的非法 cron：仅状态变化时告警，避免每 60s 重复刷日志
			for {
				spec := getSubscriptionPollCron()
				d, ok, err := cronNextDelay(spec, time.Now())
				if err != nil {
					if cronWarned != spec {
						appLog("rss", "poll_cron_invalid", "ERROR",
							"RSS 订阅 cron 表达式无效，60s 后重试："+err.Error(), map[string]any{"cron": spec})
						log.Printf("RSS 订阅 cron 表达式无效（%q），60s 后重试", spec)
						cronWarned = spec
					}
					ok = false
				} else {
					cronWarned = "" // 恢复正常后重置，再次非法时重新告警
				}
				if !ok {
					d = 60 * time.Second // 未配置 cron（不定时）或表达式非法：空转等待，保持配置热响应
				}
				select {
				case <-rssWatcher.stopCh:
					log.Printf("RSS 订阅监听线程已停止")
					return
				case <-time.After(d):
				}
				if !ok {
					continue
				}
				func() {
					defer func() {
						if r := recover(); r != nil {
							msg := fmt.Sprintf("%v", r)
							rssWatcher.mu.Lock()
							rssWatcher.lastPollError = &msg
							rssWatcher.mu.Unlock()
						}
					}()
					nowStr := utcNowISO()
					rssWatcher.mu.Lock()
					rssWatcher.lastPolledAt = &nowStr
					rssWatcher.lastPollError = nil
					rssWatcher.mu.Unlock()
					summary := PollSubscriptionsOnce()
					rssWatcher.mu.Lock()
					rssWatcher.lastSummary = summary
					rssWatcher.mu.Unlock()
				}()
			}
		}()
	})
}

// ensureStrmSyncLoop 启动 STRM 定时增量同步（幂等；按 strm.sync_cron 计划执行，未配置 cron 时仅事件触发）。
func ensureStrmSyncLoop() {
	strmOnce.Do(func() {
		go func() {
			log.Printf("STRM 定时同步线程已启动（按 strm.sync_cron 计划执行；未配置 cron 时仅整理入库/手动/删除后触发）")
			cronWarned := "" // 已告警过的非法 cron：仅状态变化时告警，避免每 60s 重复刷日志
			for {
				cfg := RT().GetConfig()
				spec := cfg.Strm.SyncCron
				d, ok, err := cronNextDelay(spec, time.Now())
				if err != nil {
					if cronWarned != spec {
						appLog("strm", "sync_cron_invalid", "ERROR",
							"STRM 定时同步 cron 表达式无效，60s 后重试："+err.Error(), map[string]any{"cron": spec})
						log.Printf("STRM 定时同步 cron 表达式无效（%q），60s 后重试", spec)
						cronWarned = spec
					}
					ok = false
				} else {
					cronWarned = "" // 恢复正常后重置，再次非法时重新告警
				}
				if !ok {
					d = 60 * time.Second // 未配置 cron（仅事件触发）或表达式非法：空转等待，保持配置热响应
				}
				select {
				case <-strmWatcher.stopCh:
					log.Printf("STRM 定时同步线程已停止")
					return
				case <-time.After(d):
				}
				if !ok {
					continue
				}
				func() {
					defer func() {
						if r := recover(); r != nil {
							msg := fmt.Sprintf("%v", r)
							strmWatcher.mu.Lock()
							strmWatcher.lastPollError = &msg
							strmWatcher.mu.Unlock()
						}
					}()
					// 触发点现读配置：上面的快照在睡眠后可陈旧整个 cron 周期（如每天一次可达 24h），
					// 睡眠窗口内的 enabled/存储切换必须以执行时刻的配置为准
					cur := RT().GetConfig()
					if cur.Strm.Enabled && cur.ActiveStorageName() == "pan115" && !StrmSyncRunning() {
						TriggerStrmSync(triggerSchedule)
					}
				}()
			}
		}()
	})
}

// StartupBackgroundWatchers 启动全部后台任务。
func StartupBackgroundWatchers() {
	ensureU115AutoOrganizeLoop()
	ensureRSSSubscriptionLoop()
	ensureStrmSyncLoop()
	ensureTraktSyncLoop()
	ensureTraktSessionLoop()
	ensureTraktSSELoop()
}

// StopBackgroundWatchers 停止全部后台监听线程（幂等）。
// 供 Server.Close 与测试清理调用，避免线程在临时目录清理后仍惰性重建
// config/data（测试偶发 TempDir 清理竞态的根因）。
// 注意：停止后当前进程内不会重启（ensure*Loop 的 sync.Once 已消费）；
// 生产环境进程生命周期即线程生命周期，不受影响。
func StopBackgroundWatchers() {
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

	authorized, _ := u115OauthStatusPayload()["authorized"].(bool)
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

// RSSSubscriptionStatusPayload RSS 订阅轮询状态。
func RSSSubscriptionStatusPayload() map[string]any {
	summary := SubscriptionsStatusPayload()
	rssWatcher.mu.Lock()
	defer rssWatcher.mu.Unlock()
	summary["watcher_alive"] = watcherAlive(rssWatcher)
	summary["last_polled_at"] = rssWatcher.lastPolledAt
	summary["last_poll_error"] = rssWatcher.lastPollError
	summary["last_summary"] = rssWatcher.lastSummary
	return summary
}

func watcherAlive(w *watcherState) bool {
	select {
	case <-w.stopCh:
		return false
	default:
		return true
	}
}
