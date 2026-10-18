// watcher.go —— 后台轮询任务（对应 webui/services/watcher.py）：
// 115 云下载自动整理监听 + RSS 订阅轮询。
package webui

import (
	"fmt"
	"log"
	"sync"
	"time"

	"meta2cloud/internal/u115"
)

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

// ensureU115AutoOrganizeLoop 启动 115 自动整理监听（幂等）。
func ensureU115AutoOrganizeLoop() {
	u115AutoOnce.Do(func() {
		go func() {
			log.Printf("115 自动整理监听线程已启动")
			for {
				sleepSeconds := 45
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
					cfg := RT().GetConfig()
					if s := cfg.U115.AutoOrganizePollSeconds; s >= 10 {
						sleepSeconds = s
					}
					pollU115AutoOrganizeOnce()
				}()
				select {
				case <-u115AutoWatcher.stopCh:
					log.Printf("115 自动整理监听线程已停止")
					return
				case <-time.After(time.Duration(sleepSeconds) * time.Second):
				}
			}
		}()
	})
}

// ensureRSSSubscriptionLoop 启动 RSS 订阅轮询（幂等）。
func ensureRSSSubscriptionLoop() {
	rssOnce.Do(func() {
		go func() {
			log.Printf("RSS 订阅监听线程已启动")
			for {
				sleepSeconds := getSubscriptionPollSeconds()
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
				select {
				case <-rssWatcher.stopCh:
					log.Printf("RSS 订阅监听线程已停止")
					return
				case <-time.After(time.Duration(sleepSeconds) * time.Second):
				}
			}
		}()
	})
}

// ensureStrmSyncLoop 启动 STRM 定时增量同步（幂等；sync_interval_seconds < 60 时不开轮询）。
func ensureStrmSyncLoop() {
	strmOnce.Do(func() {
		go func() {
			log.Printf("STRM 定时同步线程已启动")
			for {
				cfg := RT().GetConfig()
				// sync_interval_seconds < 60 表示关闭定时同步，仅手动/入库触发
				interval := cfg.Strm.SyncIntervalSeconds
				enabled := cfg.Strm.Enabled && cfg.ActiveStorageName() == "pan115" && interval >= 60
				if interval < 60 {
					interval = 60 // 空转等待下限，保持响应配置变更
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
					if enabled && !StrmSyncRunning() {
						TriggerStrmSync(triggerSchedule)
					}
				}()
				select {
				case <-strmWatcher.stopCh:
					log.Printf("STRM 定时同步线程已停止")
					return
				case <-time.After(time.Duration(interval) * time.Second):
				}
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
}

// StopBackgroundWatchers 停止全部后台监听线程（幂等）。
// 供 Server.Close 与测试清理调用，避免线程在临时目录清理后仍惰性重建
// config/data（测试偶发 TempDir 清理竞态的根因）。
// 注意：停止后当前进程内不会重启（ensure*Loop 的 sync.Once 已消费）；
// 生产环境进程生命周期即线程生命周期，不受影响。
func StopBackgroundWatchers() {
	for _, w := range []*watcherState{u115AutoWatcher, rssWatcher, strmWatcher, traktWatcher, traktSessionWatcher} {
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
		"poll_seconds":       cfg.U115.AutoOrganizePollSeconds,
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
