// Package syncengine 实现 Plex ↔ Trakt 的双向观看状态同步引擎。
//
// 引擎只依赖 watched.Provider 与 ShadowStore 接口，对具体播放端零依赖：
// 新增 Emby/Jellyfin 等播放端只需提供各自的 watched.Provider 实现。
//
// 一轮同步（Run）：
//  1. 全量拉取两侧观看状态 + 读取影子表
//  2. 逐键三方比对（当前两侧状态 vs 最后确认态）判定变化方向
//  3. 依据方向开关与 echo 防重生成写入计划，双侧反向冲突按「新者胜」裁决
//  4. 批量执行写入，成功后才落影子（幂等：影子=写后状态，重跑零 delta）
package syncengine

import (
	"context"
	"fmt"
	"time"

	"meta2cloud/internal/watched"
)

const (
	// echoSkew 回声防重窗口：目标端已存在时间戳落在写入时刻之前该窗口内则视为自己写入的回声。
	echoSkew = 10 * time.Minute
)

// Shadow 影子行：两侧「最后确认态」。语义规则（与 store/traktsync.go DDL 注释一致）：
//   - 影子仅在干净读取或远端写成功后更新
//   - Trakt reset = 行存在且 trakt 侧已看但本轮枚举缺席；从未看过 = 无行或 watched 未知
//   - 播放端缺席本轮枚举（seen=false）≠ 取消观看，本轮跳过该侧 diff
type Shadow struct {
	Key watched.MediaKey

	PlexRef  string
	PlexSeen bool
	// PlexWatched nil=未知；引擎与存储都以指针表达
	PlexWatched *bool
	PlexPlays   int
	PlexLastAt  int64

	TraktSeen    bool
	TraktWatched *bool
	TraktPlays   int
	TraktLastAt  int64

	SessionPushedAt int64 // 会话轮询最近一次推送时刻（防重）
}

// RunSummary 一次同步的统计。
type RunSummary struct {
	ToTrakt    int
	ToPlex     int
	Conflicts  int
	Unresolved int
	Skipped    int
	Err        string
}

// RunItem 逐条操作明细（方向 to_plex/to_trakt/unresolved；动作 watched/unwatched/unresolved）。
type RunItem struct {
	Key       watched.MediaKey
	Title     string
	Direction string
	Action    string
	Success   bool
	Err       string
}

// ShadowStore 影子存储接口（internal/webui/store 的 TraktSyncStore 实现；
// 独立 traktsync.db，不与其他库混用）。
type ShadowStore interface {
	AllShadow() (map[watched.MediaKey]Shadow, error)
	SaveShadow(items []Shadow) error
	StartSyncRun(trigger string) (int64, error)
	FinishSyncRun(runID int64, summary RunSummary) error
	InsertRunItem(runID int64, item RunItem) error
	// SetTraktPushedAt / TraktPushedAt 记录最近一次成功向 Trakt 写入的时刻，
	// 配合 Trakt 侧的 last_activities 时间戳做过期读取防护。
	SetTraktPushedAt(at int64) error
	TraktPushedAt() (int64, error)
}

// traktStaleMargin Trakt 数据时间戳与本端写入时刻的比较容差（秒），
// 吸收两侧时钟偏差。
const traktStaleMargin = 120

// Logger 可选日志回调（level: INFO/WARNING/SUCCESS/ERROR）。
type Logger func(level, message string, details map[string]any)

// Engine 双向同步引擎。
type Engine struct {
	Playback watched.Provider // 播放端（Plex；未来 Emby/Jellyfin 亦实现此接口）
	Trakt    watched.Provider // Trakt
	Store    ShadowStore
	ToPlex   bool // trakt → 播放端
	ToTrakt  bool // 播放端 → trakt
	Log      Logger
	Now      func() time.Time // 测试注入；nil 用 time.Now
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e *Engine) log(level, message string, details map[string]any) {
	if e.Log != nil {
		e.Log(level, message, details)
	}
}

// sideDelta 单侧相对影子的变化。
type sideDelta struct {
	to    bool  // true=新看/重看，false=取消观看
	at    int64 // 事件时间
	plays int
}

// detectDelta 与影子比对单侧变化。seen=false（基线未建立）返回 nil，本轮仅记录观察。
// 缺席语义按侧区分：absentMeansReset=true（Trakt——已看记录缺席即 reset）；
// false（播放端——文件移走/重扫中常见，缺席不等于取消）。
func detectDelta(seen, present, absentMeansReset bool, prevWatched *bool, prevAt int64, prevPlays int, cur watched.ItemState) *sideDelta {
	if !seen {
		return nil
	}
	if !present {
		if absentMeansReset {
			prevW := prevWatched != nil && *prevWatched
			if prevW {
				return &sideDelta{to: false, at: 0}
			}
		}
		return nil
	}
	prevW := prevWatched != nil && *prevWatched
	switch {
	case !cur.Watched && prevW:
		return &sideDelta{to: false, at: cur.LastAt, plays: cur.Plays}
	case cur.Watched && !prevW:
		return &sideDelta{to: true, at: cur.LastAt, plays: cur.Plays}
	case cur.Watched && prevW && (cur.LastAt > prevAt || cur.Plays > prevPlays):
		// 重看：时间戳前进或播放次数增加
		return &sideDelta{to: true, at: max64(cur.LastAt, prevAt), plays: cur.Plays}
	}
	return nil
}

// writePlan 一个键的写入计划。
type writePlan struct {
	key      watched.MediaKey
	side     string // "plex" | "trakt"
	desired  bool
	at       int64
	plays    int // 写入成功后目标端的期望播放数（scrobble 会 +1），防止下轮把追平误判为重看
	conflict bool
}

// Run 执行一轮同步。
func (e *Engine) Run(ctx context.Context, trigger string) (RunSummary, error) {
	summary := RunSummary{}
	runID, err := e.Store.StartSyncRun(trigger)
	if err != nil {
		return summary, fmt.Errorf("启动同步 run 失败：%w", err)
	}
	defer func() { _ = e.Store.FinishSyncRun(runID, summary) }()

	playbackMap, err := e.Playback.ListAll(ctx)
	if err != nil {
		summary.Err = err.Error()
		return summary, fmt.Errorf("枚举播放端观看状态失败：%w", err)
	}
	traktMap, err := e.Trakt.ListAll(ctx)
	if err != nil {
		summary.Err = err.Error()
		return summary, fmt.Errorf("枚举 Trakt 观看状态失败：%w", err)
	}

	// 过期读取防护（最终一致性）：Trakt 的已看列表在写入后存在延迟。
	// 若本端最近一次写入晚于 Trakt 数据时间戳（留时钟容差），本轮 Trakt 侧
	// 视图不可信——跳过 Trakt 侧 diff 与观察态落影子，避免把「还没读到」
	// 误判成「被重置」而对播放端发起取消。
	traktStaleMovies, traktStaleShows := false, false
	if sp, ok := e.Trakt.(watched.WatchedStampProvider); ok {
		if moviesStamp, showsStamp, err := sp.WatchedStamp(ctx); err == nil {
			lastPush, _ := e.Store.TraktPushedAt()
			if lastPush > 0 {
				traktStaleMovies = moviesStamp+traktStaleMargin < lastPush
				traktStaleShows = showsStamp+traktStaleMargin < lastPush
				if traktStaleMovies || traktStaleShows {
					e.log("WARNING", "Trakt 已看数据尚未反映最近一次写入，本轮跳过 Trakt 侧比对", map[string]any{
						"movies_stamp": moviesStamp, "shows_stamp": showsStamp, "last_push": lastPush,
					})
				}
			}
		}
	}
	traktStale := func(key watched.MediaKey) bool {
		if key.Type == "movie" {
			return traktStaleMovies
		}
		return traktStaleShows
	}

	// 播放端身份解析失败条目（计入 run 明细，跳过）
	if rep, ok := e.Playback.(watched.UnresolvedReporter); ok {
		for _, u := range rep.DrainUnresolved() {
			summary.Unresolved++
			_ = e.Store.InsertRunItem(runID, RunItem{
				Title:     fmt.Sprintf("%s（ref=%s）", u.Title, u.Ref),
				Direction: "unresolved", Action: "unresolved",
				Success: false, Err: u.Reason,
			})
		}
	}

	shadow, err := e.Store.AllShadow()
	if err != nil {
		summary.Err = err.Error()
		return summary, fmt.Errorf("读取同步状态失败：%w", err)
	}

	// ── Phase A：逐键比对，生成写入计划 ─────────────────────
	keys := map[watched.MediaKey]struct{}{}
	for k := range playbackMap {
		keys[k] = struct{}{}
	}
	for k := range traktMap {
		keys[k] = struct{}{}
	}
	for k := range shadow {
		keys[k] = struct{}{}
	}

	var plans []writePlan
	for key := range keys {
		p := playbackMap[key]
		t := traktMap[key]
		sh, hasRow := shadow[key]

		if !hasRow {
			// 首次观察（首跑或新条目）：并集规则，向缺失侧补齐。
			// 陈旧轮的 Trakt 视图不可信：整体延后到数据新鲜的一轮再播种。
			if traktStale(key) {
				continue
			}
			// 播放端目标要求条目在场（ratingKey 只在枚举中产生）；Trakt 目标无此限制。
			pWatched := p.Present && p.Watched
			tWatched := t.Present && t.Watched
			desired := pWatched || tWatched
			at := max64(p.LastAt, t.LastAt)
			if e.ToPlex && p.Present && pWatched != desired {
				plans = append(plans, writePlan{
					key: key, side: "plex", desired: desired, at: at,
					plays: expectedPlays(p, desired),
				})
			}
			if e.ToTrakt && tWatched != desired {
				plans = append(plans, writePlan{
					key: key, side: "trakt", desired: desired, at: at,
					plays: expectedPlays(t, desired),
				})
			}
			continue
		}

		pDelta := detectDelta(sh.PlexSeen, p.Present, false, sh.PlexWatched, sh.PlexLastAt, sh.PlexPlays, p)
		tDelta := (*sideDelta)(nil)
		if !traktStale(key) {
			tDelta = detectDelta(sh.TraktSeen, t.Present, true, sh.TraktWatched, sh.TraktLastAt, sh.TraktPlays, t)
		}

		switch {
		case pDelta == nil && tDelta == nil:
			// 无变化（一侧缺席本轮枚举时保持观察）
		case pDelta != nil && tDelta == nil:
			if plan, ok := e.planEcho(key, "trakt", *pDelta, t, sh); ok {
				plans = append(plans, plan)
			} else {
				summary.Skipped++
			}
		case pDelta == nil && tDelta != nil:
			if plan, ok := e.planEcho(key, "plex", *tDelta, p, sh); ok {
				plans = append(plans, plan)
			} else {
				summary.Skipped++
			}
		default:
			// 双侧都变：同向=状态已自行收敛，仅更新影子；反向=冲突，
			// 一律以 Trakt 为准（用户指定：Trakt 记录更全更权威），
			// 把 Trakt 当前状态刷向播放端。
			if pDelta.to == tDelta.to {
				continue
			}
			summary.Conflicts++
			if plan, ok := e.planEcho(key, "plex", *tDelta, p, sh); ok {
				plan.conflict = true
				plans = append(plans, plan)
			}
		}
	}

	// ── Phase B：执行写入 ───────────────────────────────────
	type sideOutcome struct {
		side    string
		ok      bool
		desired bool
		at      int64
	}
	outcomes := map[watched.MediaKey][]sideOutcome{}
	// 写入失败的键：本轮影子整行不推进（观测态也不覆盖），
	// 使下一轮能重新检测到同一变化并重试。
	failedKeys := map[watched.MediaKey]bool{}
	traktWriteOK := false
	apply := func(provider watched.Provider, side string, items []writePlan) {
		if len(items) == 0 {
			return
		}
		var watchOps []watched.WatchOp
		var unwatchKeys []watched.MediaKey
		for _, p := range items {
			if p.desired {
				watchOps = append(watchOps, watched.WatchOp{Key: p.key, At: p.at})
			} else {
				unwatchKeys = append(unwatchKeys, p.key)
			}
		}
		results := provider.MarkWatched(ctx, watchOps)
		results = append(results, provider.MarkUnwatched(ctx, unwatchKeys)...)
		byKey := map[watched.MediaKey]watched.OpResult{}
		for _, res := range results {
			byKey[res.Key] = res
		}
		for _, p := range items {
			res := byKey[p.key]
			outcome := sideOutcome{side: side, ok: res.OK, desired: p.desired, at: p.at}
			outcomes[p.key] = append(outcomes[p.key], outcome)
			direction := "to_plex"
			if side == "trakt" {
				direction = "to_trakt"
			}
			action := "watched"
			if !p.desired {
				action = "unwatched"
			}
			_ = e.Store.InsertRunItem(runID, RunItem{
				Key: p.key, Direction: direction, Action: action,
				Success: res.OK, Err: res.Err,
			})
			if !res.OK {
				failedKeys[p.key] = true
				e.log("WARNING", fmt.Sprintf("观看同步写入 %s 失败：%s", p.key, res.Err), map[string]any{
					"key": p.key.String(), "side": side, "action": action,
				})
				continue
			}
			if side == "trakt" {
				summary.ToTrakt++
				traktWriteOK = true
			} else {
				summary.ToPlex++
			}
		}
	}
	var traktPlans, plexPlans []writePlan
	for _, p := range plans {
		if p.side == "trakt" {
			traktPlans = append(traktPlans, p)
		} else {
			plexPlans = append(plexPlans, p)
		}
	}
	traktWriteOK = false
	apply(e.Trakt, "trakt", traktPlans)
	apply(e.Playback, "plex", plexPlans)
	if traktWriteOK {
		// 记录写入时刻，供下一轮的过期读取防护使用
		_ = e.Store.SetTraktPushedAt(e.now().Unix())
	}

	// ── Phase C：构建并保存影子（仅落盘有变化或新建的行） ──────
	var toSave []Shadow
	for key := range keys {
		if failedKeys[key] {
			continue // 影子整行不推进，下轮重试
		}
		p := playbackMap[key]
		t := traktMap[key]
		sh, hasRow := shadow[key]
		row := Shadow{Key: key}
		if hasRow {
			row = sh
		}
		// 观察态覆盖。两侧缺席语义不同：
		// Plex 缺席（文件移走/重扫中）≠ 取消 → 保留旧值、seen 置 false；
		// Trakt /sync/watched 是权威全量 → 缺席即未看，落未看态。
		// 但本轮为过期读取（traktStale）时，Trakt 观察态整体不落影子，
		// 保持上一轮确认值，待数据新鲜后再 diff。
		if p.Present {
			row.PlexSeen = true
			row.PlexWatched = boolPtr(p.Watched)
			row.PlexPlays = p.Plays
			row.PlexLastAt = p.LastAt
			row.PlexRef = p.Ref
		} else {
			row.PlexSeen = false
		}
		if traktStale(key) {
			// 保持旧值
		} else if t.Present {
			row.TraktSeen = true
			row.TraktWatched = boolPtr(true)
			row.TraktPlays = t.Plays
			row.TraktLastAt = t.LastAt
		} else {
			row.TraktSeen = false
			row.TraktWatched = boolPtr(false)
			row.TraktPlays = 0
			row.TraktLastAt = 0
		}
		// 成功写入覆盖目标侧（Plex 服务器时间不可知，scrobble 后以 now 近似）：
		// 按计划重放，仅当该计划的结果为 OK 时落影子；失败键影子不动，下轮重试。
		for _, plan := range plans {
			if plan.key != key {
				continue
			}
			ok := false
			for _, res := range outcomes[key] {
				if res.side == plan.side && res.ok && res.desired == plan.desired && res.at == plan.at {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
			if plan.side == "trakt" {
				row.TraktSeen = true
				row.TraktWatched = boolPtr(plan.desired)
				if plan.desired {
					row.TraktLastAt = max64(row.TraktLastAt, plan.at)
					row.TraktPlays = maxInt(row.TraktPlays, plan.plays)
				} else {
					row.TraktLastAt = 0
					row.TraktPlays = 0
				}
			} else {
				row.PlexSeen = true
				row.PlexWatched = boolPtr(plan.desired)
				if plan.desired {
					row.PlexLastAt = max64(row.PlexLastAt, e.now().Unix())
					row.PlexPlays = maxInt(row.PlexPlays, plan.plays)
				} else {
					row.PlexLastAt = 0
					row.PlexPlays = 0
				}
			}
		}
		if !hasRow || shadowChanged(sh, row) {
			toSave = append(toSave, row)
		}
	}
	if len(toSave) > 0 {
		if err := e.Store.SaveShadow(toSave); err != nil {
			summary.Err = err.Error()
			return summary, fmt.Errorf("保存同步状态失败：%w", err)
		}
	}
	return summary, nil
}

// planEcho 生成「把 delta 写到 targetSide」的计划；返回 false 表示因方向开关或
// echo 防重跳过（防重复 play：会话轮询推送、双端近乎同时观看等）。
// target 为目标侧当前观测态，sh 为影子。
func (e *Engine) planEcho(key watched.MediaKey, targetSide string, delta sideDelta, target watched.ItemState, sh Shadow) (writePlan, bool) {
	if targetSide == "trakt" && !e.ToTrakt {
		return writePlan{}, false
	}
	if targetSide == "plex" && !e.ToPlex {
		return writePlan{}, false
	}
	if delta.to {
		if targetSide == "trakt" {
			// trakt 已看且时间戳落在写入时刻前一个窗口内 → 视为回声
			if target.Present && target.Watched {
				if target.LastAt >= delta.at-int64(echoSkew/time.Second) || sh.SessionPushedAt >= delta.at-int64(echoSkew/time.Second) {
					return writePlan{}, false
				}
			}
		} else {
			// 播放端目标：条目不在枚举中（无法 scrobble）或已看（会虚增播放次数）→ 跳过
			if !target.Present || target.Watched {
				return writePlan{}, false
			}
		}
	} else {
		// 取消观看：目标端本就未看则无事可做
		if targetSide == "trakt" {
			if !(target.Present && target.Watched) {
				return writePlan{}, false
			}
		} else {
			if target.Present && !target.Watched {
				return writePlan{}, false
			}
			if !target.Present {
				return writePlan{}, false
			}
		}
	}
	return writePlan{key: key, side: targetSide, desired: delta.to, at: delta.at, plays: expectedPlays(target, delta.to)}, true
}

// expectedPlays 写入后目标端的期望播放数：scrobble/历史 add 使播放数 +1（缺席视为 0）。
func expectedPlays(target watched.ItemState, desired bool) int {
	if !desired {
		return 0
	}
	if !target.Present {
		return 1
	}
	return target.Plays + 1
}

func boolPtr(v bool) *bool { return &v }

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// shadowChanged 比较影子行是否变化（SessionPushedAt 由会话轮询维护，不参与）。
func shadowChanged(old, new Shadow) bool {
	return old.PlexRef != new.PlexRef ||
		old.PlexSeen != new.PlexSeen ||
		!boolEqual(old.PlexWatched, new.PlexWatched) ||
		old.PlexPlays != new.PlexPlays ||
		old.PlexLastAt != new.PlexLastAt ||
		old.TraktSeen != new.TraktSeen ||
		!boolEqual(old.TraktWatched, new.TraktWatched) ||
		old.TraktPlays != new.TraktPlays ||
		old.TraktLastAt != new.TraktLastAt
}

func boolEqual(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
