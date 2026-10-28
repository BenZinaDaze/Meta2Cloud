// Package webui —— Plex 播放事件实时推送（SSE）与会话轮询兜底循环（自 traktsvc.go 拆出）。
package webui

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"meta2cloud/internal/plexserver"
	"meta2cloud/internal/trakt"
	"meta2cloud/internal/watched"
	"meta2cloud/internal/webui/store"
)

// ── 实时推送（Plex SSE）─────────────────────────────────────

// traktSSEState SSE 连接状态（状态接口展示用）。
var traktSSEState struct {
	mu        sync.Mutex
	connected bool
}

func setSSEConnected(connected bool) {
	traktSSEState.mu.Lock()
	traktSSEState.connected = connected
	traktSSEState.mu.Unlock()
}

func sseConnected() bool {
	traktSSEState.mu.Lock()
	defer traktSSEState.mu.Unlock()
	return traktSSEState.connected
}

// sseEventQueueSize SSE 事件异步分发的有界队列长度：
// 处理慢（如 Trakt 冷却）时丢弃最旧事件——进度事件几秒后自然再来，
// 且 45s 轮询兜底，丢事件不影响正确性，只保证连接读循环不被卡死。
const sseEventQueueSize = 64

// stoppedOffsetTolerance stopped 报文 offset 相对观察轨迹的容差：小量超出
// 视为正常收尾（如播放至末尾最后几秒），大幅超出视为客户端垃圾值封顶。
const stoppedOffsetTolerance = 0.02

// handlePlexPlaybackNotification 处理一条实时播放通知：
// 更新会话观察行，按状态跃迁上报 scrobble（start/pause 不计次），
// stopped 事件以最终进度上报 stop（计次与去重由 Trakt 服务端裁定），
// 随后清理会话观察行（轮询不与 SSE 并行运行，收尾由 stopped 事件承担）。
func handlePlexPlaybackNotification(plexClient *plexserver.Client, traktClient *trakt.Client, syncStore *store.TraktSyncStore, notif plexserver.PlaybackNotification) {
	defer func() {
		if r := recover(); r != nil {
			appLog("trakt", "sse_event_panic", "ERROR", fmt.Sprintf("实时通知处理异常：%v", r), nil)
		}
	}()
	if notif.RatingKey == "" {
		return
	}
	// 会话观察行与切集处理。抓包证实（2026-10-25，config/data/sse-capture/）：
	// 播放/暂停态事件随 ~10s 周期重发，正常播放的会话必有观察行；播放队列
	// 切集时新 sessionKey 先以「旧条目 + 旧最终 offset」出现（paused），
	// ratingKey 随后切换而 viewOffset 仍为旧值（buffering），直到 playing
	// 才复位——若不重置，上一集的 ~99% 会被并进下一集的收尾。
	tracked := syncStore.GetSyncSession(notif.SessionKey)
	if tracked != nil && tracked.PlexRef != "" && tracked.PlexRef != notif.RatingKey {
		finishSwitchedSession(syncStore, traktClient, tracked, "sse", notif.SessionKey)
		scrobbleStateFor(notif.SessionKey).awaitPlay = true
		tracked = nil
	}
	st := scrobbleStateFor(notif.SessionKey)
	if notif.State != "stopped" {
		st.lastObserved = notif.State
	}
	if notif.State == "playing" {
		st.awaitPlay = false // 开播即恢复信任：切集后 playing 的 offset 已复位（抓包）
	}

	// 播放结束：以观察行身份 + 上下文内最大进度上报 stop，随后清理观察行
	// 与上报状态（计次与去重由 Trakt 服务端裁定）。未跟踪过（无观察行）或
	// 切集后尚未开播（awaitPlay，offset 是上一集遗留值）的 stopped 不据此
	// 上报，观看状态由定时全量同步兜底。
	if notif.State == "stopped" {
		if tracked != nil && tracked.TmdbID > 0 && !st.awaitPlay {
			duration := notif.DurationMS
			if duration == 0 {
				duration = tracked.DurationMS
			}
			finalProgress := 0.0
			if duration > 0 {
				finalProgress = float64(notif.ViewOffsetMS) / float64(duration)
			}
			// 收尾进度封顶（仅暂停态会话）：实测（2026-10-26 ATV 暂停后休眠）
			// 播放队列客户端的会话被终止时报出的 offset 是上一集的时长
			// （观察全程 1%、收尾报 92%=上集时长/本集时长）。暂停中的播放器
			// 位置不可能前进——最后观察到暂停（或未知状态）时，stopped 报文
			// 大幅超出观察轨迹即垃圾值，按观察值收尾；playing 态与 stopped
			// 之间的自然推进不封顶（不足额的真实观看由全量同步兜底）。
			if st.lastObserved != "playing" && finalProgress > tracked.MaxProgress+stoppedOffsetTolerance {
				finalProgress = tracked.MaxProgress
			}
			if tracked.MaxProgress > finalProgress {
				finalProgress = tracked.MaxProgress
			}
			finishSession(syncStore, traktClient, syncStore, tracked.Key(), derefString(tracked.Title), "sse", finalProgress, notif.SessionKey)
		}
		dropScrobbleState(notif.SessionKey)
		syncStore.DeleteSyncSession(notif.SessionKey)
		return
	}

	// duration/标题缺失时先取会话观察行，再退回详情接口；标题随观察行缓存，
	// 每个会话至多拉一次详情
	duration := notif.DurationMS
	title := ""
	if tracked != nil {
		if duration == 0 {
			duration = tracked.DurationMS
		}
		title = derefString(tracked.Title)
	}
	if duration == 0 || title == "" {
		if md, err := plexClient.Metadata(notif.RatingKey); err == nil {
			if duration == 0 {
				duration = int64(md.Duration)
			}
			if title == "" {
				title = sessionDisplayTitle(md)
			}
		}
	}
	source := plexserver.NewSource(plexClient, buildTitleIndex(), syncStore)
	sess := &plexserver.Metadata{
		SessionKey: notif.SessionKey, RatingKey: notif.RatingKey,
		ViewOffset: int(notif.ViewOffsetMS), Duration: int(duration),
	}
	key, keyOK := resolveSessionKey(syncStore, source, sess)

	progress := 0.0
	if duration > 0 {
		progress = float64(notif.ViewOffsetMS) / float64(duration)
	}
	// 切集后尚未开播（awaitPlay）的 paused/buffering 事件携带上一集遗留
	// offset（抓包），进度按 0 记；UpsertSyncSession 取历史最大，归零可保
	// 新条目从零跟踪
	maxProgress := progress
	if st.awaitPlay {
		maxProgress = 0
	}
	row := store.TraktSyncSession{
		SessionKey: notif.SessionKey, PlexRef: notif.RatingKey,
		ViewOffsetMS: notif.ViewOffsetMS, DurationMS: duration,
		MaxProgress: maxProgress, Title: &title,
	}
	if keyOK {
		row.MediaType = key.Type
		if key.TVDBID > 0 {
			row.Anchor = "tvdb"
			row.TmdbID = key.TVDBID
		} else {
			row.TmdbID = key.TMDBID
		}
		row.Season = key.Season
		row.Episode = key.Episode
	}
	syncStore.UpsertSyncSession(row)

	if !keyOK || duration <= 0 {
		// 身份解析失败此前完全静默（每 10s 事件重复丢弃），事故排查只能靠
		// 逐层抓包；这里每会话告警一次，给出可核对的条目定位
		if !keyOK && !st.unresolvedLogged {
			st.unresolvedLogged = true
			appLog("trakt", "session_unresolved", "WARNING",
				fmt.Sprintf("无法识别正在播放的条目对应哪部媒体，本次不推送播放记录（Plex 条目 %s）。常见原因：Plex 未提供外部 ID，或文件路径与本地媒体库不一致", notif.RatingKey), nil)
		}
		return
	}
	pct, _ := scrobbleProgress(notif.ViewOffsetMS, duration)
	if sessionScrobbleGate(syncStore, key) {
		scrobbleTransition("sse", syncStore, traktClient, syncStore, key, title, notif.State, pct, notif.SessionKey)
	}
}

// ensureTraktSSELoop 启动 Plex 实时通知订阅（幂等）：
// 常驻一条 SSE 连接，断线指数退避重连；事件经有界队列异步分发。
func ensureTraktSSELoop() {
	traktSSEOnce.Do(func() {
		go func() {
			appLog("trakt", "sse_started", "INFO", "Plex 实时通知线程已启动", nil)
			backoff := 5 * time.Second
			lastGateOK := true
			for {
				// 门控只看 sse 开关、配对就绪与 →Trakt 方向，不受「启用定时同步」影响：
				// 实时推送是事件驱动的观看同步，与定时全量是两个独立开关
				gate := func() (plexClient *plexserver.Client, traktClient *trakt.Client, syncStore *store.TraktSyncStore, ok bool) {
					cfg := RT().GetConfig()
					if !cfg.Trakt.SSEEnabled || !cfg.Trakt.SyncToTrakt || !cfg.IsPlexReady() || !cfg.IsTraktReady() {
						return nil, nil, nil, false
					}
					plexClient = RT().GetPlexClient()
					traktClient = RT().GetTraktClient()
					syncStore = RT().GetTraktSyncStore()
					if plexClient == nil || traktClient == nil {
						return nil, nil, nil, false
					}
					return plexClient, traktClient, syncStore, true
				}
				plexClient, traktClient, syncStore, ok := gate()
				if !ok {
					setSSEConnected(false)
					if lastGateOK {
						appLog("trakt", "sse_gate_pending", "INFO", "实时通知未开启：需开启实时通知开关并完成 Trakt 与 Plex 授权，30 秒后重新检查", nil)
						lastGateOK = false
					}
					if !sleepWatcher(traktSSEWatcher, 30*time.Second) {
						return
					}
					continue
				}
				lastGateOK = true

				// 有界异步分发：读循环永不被 Trakt 写入阻塞
				events := make(chan plexserver.PlaybackNotification, sseEventQueueSize)
				go func() {
					defer func() {
						if r := recover(); r != nil {
							appLog("trakt", "sse_worker_panic", "ERROR", fmt.Sprintf("实时事件分发异常：%v", r), nil)
						}
					}()
					for notif := range events {
						handlePlexPlaybackNotification(plexClient, traktClient, syncStore, notif)
					}
				}()

				// 「已连接」只在真正订阅成功（HTTP 200）后报告，
				// 订阅被拒/断线不算连接成功，退避继续累加
				var openedAt time.Time
				ctx, cancel := context.WithCancel(context.Background())
				go func() {
					select {
					case <-traktSSEWatcher.stopCh:
						cancel()
					case <-ctx.Done():
					}
				}()
				err := plexClient.StreamNotifications(ctx, func() {
					openedAt = time.Now()
					setSSEConnected(true)
					NotifyServiceHealth("Plex", true, "")
					appLog("trakt", "sse_connected", "INFO",
						fmt.Sprintf("Plex 实时通知已连接（%s）", plexClient.BaseURL), nil)
				}, func(n plexserver.PlaybackNotification) {
					select {
					case events <- n:
					default:
						// 队列满：丢最旧，保最新
						select {
						case <-events:
						default:
						}
						select {
						case events <- n:
						default:
						}
					}
				})
				cancel()
				close(events)
				setSSEConnected(false)
				if errors.Is(err, plexserver.ErrUnauthorized) {
					NotifyServiceHealth("Plex", false, err.Error())
				}
				msg := "连接结束"
				if err != nil {
					msg = err.Error()
				}
				appLog("trakt", "sse_disconnected", "WARNING",
					fmt.Sprintf("Plex 实时通知已断开（%s），将于 %s 后重新连接，期间由会话轮询同步", msg, backoff), nil)
				// 退避策略：稳定连接（存活 ≥60s）后重置；秒断（订阅被拒/服务不可达）
				// 则逐次加倍直至 60s，避免日志刷屏；端点不存在（旧版 PMS）降到 10 分钟一探
				switch {
				case errors.Is(err, plexserver.ErrNotificationsUnsupported):
					backoff = 10 * time.Minute
				case !openedAt.IsZero() && time.Since(openedAt) >= 60*time.Second:
					backoff = 5 * time.Second
				case backoff < 60*time.Second:
					backoff *= 2
				}
				if !sleepWatcher(traktSSEWatcher, backoff) {
					return
				}
			}
		}()
	})
}

// sleepWatcher 可中断的睡眠：stopCh 关闭时返回 false（线程退出）。
func sleepWatcher(w *watcherState, d time.Duration) bool {
	select {
	case <-w.stopCh:
		return false
	case <-time.After(d):
		return true
	}
}

// resolveSessionKey 会话身份解析：状态记录快路径 → 会话行缓存 → Plex 详情补解析。
func resolveSessionKey(store *store.TraktSyncStore, source *plexserver.Source, sess *plexserver.Metadata) (watched.MediaKey, bool) {
	if shadow, ok, _ := store.GetShadowByPlexRef(sess.RatingKey); ok && shadow.Key.TMDBID > 0 {
		return shadow.Key, true
	}
	if row := store.GetSyncSession(sess.SessionKey); row != nil && row.TmdbID > 0 {
		return row.Key(), true
	}
	return source.ResolveMetadataRef(sess.RatingKey)
}

// ── 后台轮询循环 ───────────────────────────────────────────

var (
	traktWatcher        = &watcherState{stopCh: make(chan struct{})}
	traktSessionWatcher = &watcherState{stopCh: make(chan struct{})}
	traktSSEWatcher     = &watcherState{stopCh: make(chan struct{})}
	traktSessionOnce    sync.Once
	traktSSEOnce        sync.Once
)

// plexReachable 轻探测 Plex 可达性（/identity，失败含 DNS/连接错误）。
func plexReachable() bool {
	plexClient := RT().GetPlexClient()
	if plexClient == nil {
		return true // 未完成 Plex 配对时不等待，由同步自身报出明确的配置错误
	}
	_, err := plexClient.Identity()
	return err == nil
}

// waitPlexReachable 等待 Plex 恢复可达：30 秒一探，上限 10 分钟。
// Plex 常与本服务同 compose 部署、随之同时重启——启动首轮同步若在
// 对端未就绪时触发，会在 DNS/连接层立刻失败并留下错误记录。探测
// 不可达时短周期等待，恢复后立即开始本轮；超时放弃，交由下一轮定时。
// halted=true 表示调度已被停止（随 stop 退出）。
func waitPlexReachable(stop <-chan struct{}) (ready, halted bool) {
	if plexReachable() {
		return true, false
	}
	appLog("trakt", "plex_unreachable", "WARNING", "Plex 暂不可达，等待就绪后开始本轮同步", nil)
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-stop:
			return false, true
		case <-time.After(30 * time.Second):
		}
		if plexReachable() {
			return true, false
		}
	}
	appLog("trakt", "plex_unreachable_timeout", "WARNING", "等待 Plex 就绪超时，本轮定时同步跳过", nil)
	return false, false
}

// ensureTraktSessionLoop 启动 Plex 会话轮询（正在看 → Trakt）。
// 纯兜底：每轮先查 SSE 是否在线，在线则跳过实际轮询（见 pollPlexSessionsOnce）。
func ensureTraktSessionLoop() {
	traktSessionOnce.Do(func() {
		go func() {
			appLog("trakt", "session_poll_started", "INFO", "Plex 会话轮询线程已启动", nil)
			for {
				interval := RT().GetConfig().Trakt.SessionPollSeconds
				if interval < 15 {
					interval = 15
				}
				func() {
					defer func() {
						if r := recover(); r != nil {
							msg := fmt.Sprintf("%v", r)
							traktSessionWatcher.mu.Lock()
							traktSessionWatcher.lastPollError = &msg
							traktSessionWatcher.mu.Unlock()
						}
					}()
					pollPlexSessionsOnce()
				}()
				select {
				case <-traktSessionWatcher.stopCh:
					appLog("trakt", "session_poll_stopped", "INFO", "Plex 会话轮询线程已停止", nil)
					return
				case <-time.After(time.Duration(interval) * time.Second):
				}
			}
		}()
	})
}

// TraktSyncStatusPayload 供外部（如状态聚合）调用。
