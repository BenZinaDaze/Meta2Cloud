package syncengine

import (
	"context"
	"testing"

	"meta2cloud/internal/watched"
)

// ── 测试替身 ────────────────────────────────────────────────

type fakeProvider struct {
	name       string
	state      map[watched.MediaKey]watched.ItemState
	watchLog   []watched.WatchOp
	unwatchLog []watched.MediaKey
	failKeys   map[watched.MediaKey]bool // 这些键的写入返回失败
	// WatchedStampProvider 能力（Trakt 侧测试用）：模拟数据时间戳
	hasStamp                bool
	stampMovies, stampShows int64
}

func newFakeProvider(name string) *fakeProvider {
	return &fakeProvider{
		name:     name,
		state:    map[watched.MediaKey]watched.ItemState{},
		failKeys: map[watched.MediaKey]bool{},
	}
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) WatchedStamp(_ context.Context) (int64, int64, error) {
	return f.stampMovies, f.stampShows, nil
}

func (f *fakeProvider) ListAll(_ context.Context) (map[watched.MediaKey]watched.ItemState, error) {
	out := map[watched.MediaKey]watched.ItemState{}
	for k, v := range f.state {
		out[k] = v
	}
	return out, nil
}

func (f *fakeProvider) MarkWatched(_ context.Context, ops []watched.WatchOp) []watched.OpResult {
	results := make([]watched.OpResult, 0, len(ops))
	for _, op := range ops {
		f.watchLog = append(f.watchLog, op)
		if f.failKeys[op.Key] {
			results = append(results, watched.OpResult{Key: op.Key, OK: false, Err: "注入失败"})
			continue
		}
		cur := f.state[op.Key]
		cur.Present = true
		cur.Watched = true
		if op.At > 0 {
			cur.LastAt = op.At
		}
		cur.Plays++
		f.state[op.Key] = cur
		results = append(results, watched.OpResult{Key: op.Key, OK: true})
	}
	return results
}

func (f *fakeProvider) MarkUnwatched(_ context.Context, keys []watched.MediaKey) []watched.OpResult {
	results := make([]watched.OpResult, 0, len(keys))
	for _, key := range keys {
		f.unwatchLog = append(f.unwatchLog, key)
		if f.failKeys[key] {
			results = append(results, watched.OpResult{Key: key, OK: false, Err: "注入失败"})
			continue
		}
		cur := f.state[key]
		cur.Present = true
		cur.Watched = false
		cur.LastAt = 0
		f.state[key] = cur
		results = append(results, watched.OpResult{Key: key, OK: true})
	}
	return results
}

func (f *fakeProvider) setWatched(key watched.MediaKey, lastAt int64, plays int) {
	f.state[key] = watched.ItemState{Present: true, Watched: true, LastAt: lastAt, Plays: plays, Ref: "ref-" + key.String()}
}

func (f *fakeProvider) setUnwatched(key watched.MediaKey) {
	f.state[key] = watched.ItemState{Present: true, Watched: false, Ref: "ref-" + key.String()}
}

type fakeStore struct {
	items       map[watched.MediaKey]Shadow
	runs        []RunSummary
	runItems    []RunItem
	traktPushed int64
}

func newFakeStore() *fakeStore { return &fakeStore{items: map[watched.MediaKey]Shadow{}} }

func (s *fakeStore) AllShadow() (map[watched.MediaKey]Shadow, error) {
	out := map[watched.MediaKey]Shadow{}
	for k, v := range s.items {
		out[k] = v
	}
	return out, nil
}

func (s *fakeStore) SaveShadow(items []Shadow) error {
	for _, it := range items {
		s.items[it.Key] = it
	}
	return nil
}

func (s *fakeStore) StartSyncRun(_ string) (int64, error) { return 1, nil }

func (s *fakeStore) FinishSyncRun(_ int64, summary RunSummary) error {
	s.runs = append(s.runs, summary)
	return nil
}

func (s *fakeStore) InsertRunItem(_ int64, item RunItem) error {
	s.runItems = append(s.runItems, item)
	return nil
}

func (s *fakeStore) SetTraktPushedAt(at int64) error {
	s.traktPushed = at
	return nil
}

func (s *fakeStore) TraktPushedAt() (int64, error) { return s.traktPushed, nil }

func movie(id int64) watched.MediaKey { return watched.MediaKey{Type: "movie", TMDBID: id} }
func episode(id, s, e int) watched.MediaKey {
	return watched.MediaKey{Type: "tv", TMDBID: int64(id), Season: s, Episode: e}
}

func newEngine(plex, trakt *fakeProvider, store *fakeStore, toPlex, toTrakt bool) *Engine {
	return &Engine{
		Playback: plex, Trakt: trakt, Store: store,
		ToPlex: toPlex, ToTrakt: toTrakt,
	}
}

// ── 用例 ────────────────────────────────────────────────────

func TestFirstRunUnionTraktToPlex(t *testing.T) {
	plex, trakt := newFakeProvider("plex"), newFakeProvider("trakt")
	store := newFakeStore()
	key := movie(100)
	// 条目需在 Plex 枚举中存在（ratingKey 只在枚举中产生）但未观看
	plex.setUnwatched(key)
	trakt.setWatched(key, 1700000000, 1)

	e := newEngine(plex, trakt, store, true, true)
	summary, err := e.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run 出错：%v", err)
	}
	if summary.ToPlex != 1 {
		t.Fatalf("期望 →Plex 1 条，实际 %d", summary.ToPlex)
	}
	if len(plex.watchLog) != 1 || plex.watchLog[0].Key != key {
		t.Fatalf("期望 Plex 收到 watch，实际 %+v", plex.watchLog)
	}
	sh := store.items[key]
	if sh.PlexWatched == nil || !*sh.PlexWatched || sh.TraktWatched == nil || !*sh.TraktWatched {
		t.Fatalf("状态应为双侧已看：%+v", sh)
	}
	// 重跑幂等：零写入
	plex.watchLog = nil
	trakt.watchLog = nil
	summary, err = e.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("第二次 Run 出错：%v", err)
	}
	if summary.ToPlex != 0 || summary.ToTrakt != 0 || len(plex.watchLog) != 0 || len(trakt.watchLog) != 0 {
		t.Fatalf("重跑应零写入：summary=%+v plex=%v trakt=%v", summary, plex.watchLog, trakt.watchLog)
	}
}

func TestFirstRunUnionPlexToTraktCarriesTimestamp(t *testing.T) {
	plex, trakt := newFakeProvider("plex"), newFakeProvider("trakt")
	store := newFakeStore()
	key := episode(200, 1, 3)
	plex.setWatched(key, 1700000123, 2)

	e := newEngine(plex, trakt, store, true, true)
	summary, err := e.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run 出错：%v", err)
	}
	if summary.ToTrakt != 1 {
		t.Fatalf("期望 →Trakt 1 条，实际 %d", summary.ToTrakt)
	}
	if len(trakt.watchLog) != 1 || trakt.watchLog[0].At != 1700000123 {
		t.Fatalf("期望 Trakt watch 携带 Plex 时间戳，实际 %+v", trakt.watchLog)
	}
}

func TestFirstRunBothWatchedNoWrites(t *testing.T) {
	plex, trakt := newFakeProvider("plex"), newFakeProvider("trakt")
	store := newFakeStore()
	key := movie(300)
	plex.setWatched(key, 1700000000, 1)
	trakt.setWatched(key, 1700000000, 1)

	e := newEngine(plex, trakt, store, true, true)
	summary, err := e.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run 出错：%v", err)
	}
	if summary.ToPlex != 0 || summary.ToTrakt != 0 {
		t.Fatalf("双侧已看不应写入：%+v", summary)
	}
}

func TestPlexUnwatchPropagatesToTrakt(t *testing.T) {
	plex, trakt := newFakeProvider("plex"), newFakeProvider("trakt")
	store := newFakeStore()
	key := movie(400)
	plex.setWatched(key, 1700000000, 1)
	trakt.setWatched(key, 1700000000, 1)
	e := newEngine(plex, trakt, store, true, true)
	if _, err := e.Run(context.Background(), "test"); err != nil {
		t.Fatalf("基线 Run 出错：%v", err)
	}

	// Plex 取消观看
	plex.setUnwatched(key)
	summary, err := e.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run 出错：%v", err)
	}
	if summary.ToTrakt != 1 {
		t.Fatalf("期望 →Trakt 1 条取消，实际 %+v", summary)
	}
	if len(trakt.unwatchLog) != 1 || trakt.unwatchLog[0] != key {
		t.Fatalf("期望 Trakt 收到 unwatch，实际 %+v", trakt.unwatchLog)
	}
	sh := store.items[key]
	if sh.TraktWatched == nil || *sh.TraktWatched {
		t.Fatalf("状态 trakt 侧应已取消：%+v", sh)
	}
}

func TestTraktResetPropagatesToPlex(t *testing.T) {
	plex, trakt := newFakeProvider("plex"), newFakeProvider("trakt")
	store := newFakeStore()
	key := movie(500)
	plex.setWatched(key, 1700000000, 1)
	trakt.setWatched(key, 1700000000, 1)
	e := newEngine(plex, trakt, store, true, true)
	if _, err := e.Run(context.Background(), "test"); err != nil {
		t.Fatalf("基线 Run 出错：%v", err)
	}

	// Trakt reset：条目从已看记录中消失
	delete(trakt.state, key)
	summary, err := e.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run 出错：%v", err)
	}
	if summary.ToPlex != 1 {
		t.Fatalf("期望 →Plex 1 条取消，实际 %+v", summary)
	}
	if len(plex.unwatchLog) != 1 || plex.unwatchLog[0] != key {
		t.Fatalf("期望 Plex 收到 unscrobble，实际 %+v", plex.unwatchLog)
	}
}

func TestPlexAbsentNotReset(t *testing.T) {
	plex, trakt := newFakeProvider("plex"), newFakeProvider("trakt")
	store := newFakeStore()
	key := movie(600)
	plex.setWatched(key, 1700000000, 1)
	trakt.setWatched(key, 1700000000, 1)
	e := newEngine(plex, trakt, store, true, true)
	if _, err := e.Run(context.Background(), "test"); err != nil {
		t.Fatalf("基线 Run 出错：%v", err)
	}

	// Plex 枚举缺席（文件被移走/重扫中）→ 不应触发任何写入
	delete(plex.state, key)
	summary, err := e.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run 出错：%v", err)
	}
	if summary.ToPlex != 0 || summary.ToTrakt != 0 || len(plex.unwatchLog) != 0 || len(trakt.unwatchLog) != 0 {
		t.Fatalf("缺席不应触发写入：summary=%+v", summary)
	}
}

func TestConflictTraktPriorityWins(t *testing.T) {
	plex, trakt := newFakeProvider("plex"), newFakeProvider("trakt")
	store := newFakeStore()
	key := movie(700)
	plex.setWatched(key, 1700000000, 1)
	trakt.setWatched(key, 1700000000, 1)
	e := newEngine(plex, trakt, store, true, true)
	if _, err := e.Run(context.Background(), "test"); err != nil {
		t.Fatalf("基线 Run 出错：%v", err)
	}

	// 冲突：Trakt reset + Plex 重看（时间戳更新）→ 一律以 Trakt 为准 → Plex 被取消
	delete(trakt.state, key)
	plex.setWatched(key, 1700010000, 2)
	summary, err := e.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run 出错：%v", err)
	}
	if summary.Conflicts != 1 {
		t.Fatalf("期望 1 次冲突，实际 %+v", summary)
	}
	if summary.ToPlex != 1 || len(plex.unwatchLog) != 1 {
		t.Fatalf("冲突应以 Trakt 为准（取消 Plex 已看）：summary=%+v plex=%v", summary, plex.unwatchLog)
	}
	if summary.ToTrakt != 0 {
		t.Fatalf("冲突中不应写 Trakt：summary=%+v", summary)
	}
}

func TestConflictTraktPriorityWatchedWins(t *testing.T) {
	plex, trakt := newFakeProvider("plex"), newFakeProvider("trakt")
	store := newFakeStore()
	key := movie(710)
	plex.setWatched(key, 1700000000, 1)
	trakt.setWatched(key, 1700000000, 1)
	e := newEngine(plex, trakt, store, true, true)
	if _, err := e.Run(context.Background(), "test"); err != nil {
		t.Fatalf("基线 Run 出错：%v", err)
	}

	// 冲突：Plex 取消观看 + Trakt 重看（时间戳更新）→ 以 Trakt 为准 → Plex 重新标记
	plex.setUnwatched(key)
	trakt.setWatched(key, 1700010000, 2)
	summary, err := e.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run 出错：%v", err)
	}
	if summary.Conflicts != 1 {
		t.Fatalf("期望 1 次冲突，实际 %+v", summary)
	}
	if summary.ToPlex != 1 || len(plex.watchLog) != 1 {
		t.Fatalf("冲突应以 Trakt 为准（重新标记 Plex）：summary=%+v plex=%v", summary, plex.watchLog)
	}
}

// TestStaleTraktReadNoReset 过期读取防护：Trakt 数据时间戳早于最近一次写入时，
// 本轮 Trakt 侧 diff 跳过——不得把「还没读到」当成 reset 去取消 Plex。
func TestStaleTraktReadNoReset(t *testing.T) {
	plex, trakt := newFakeProvider("plex"), newFakeProvider("trakt")
	store := newFakeStore()
	key := movie(720)
	plex.setWatched(key, 1700000000, 1)
	trakt.setWatched(key, 1700000000, 1)
	e := newEngine(plex, trakt, store, true, true)
	if _, err := e.Run(context.Background(), "test"); err != nil {
		t.Fatalf("基线 Run 出错：%v", err)
	}
	// 基线轮已写入 Trakt → lastPush 有值；模拟 Trakt 数据时间戳停在写入之前
	if err := store.SetTraktPushedAt(1700010000); err != nil {
		t.Fatal(err)
	}
	trakt.stampMovies, trakt.stampShows = 1700000000, 1700000000

	// 下一轮 Trakt 枚举里这 9… N 条"还没出现"（陈旧读取）
	delete(trakt.state, key)
	summary, err := e.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run 出错：%v", err)
	}
	if summary.ToPlex != 0 || len(plex.unwatchLog) != 0 {
		t.Fatalf("陈旧读取不得触发 Plex 取消：summary=%+v plex=%v", summary, plex.unwatchLog)
	}
	sh := store.items[key]
	if sh.TraktWatched == nil || !*sh.TraktWatched {
		t.Fatalf("陈旧轮状态 trakt 侧应保持旧值：%+v", sh)
	}

	// 数据新鲜后（时间戳超过写入时刻），缺席才被识别为 reset 并传播
	trakt.stampMovies, trakt.stampShows = 1700020000, 1700020000
	summary, err = e.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run 出错：%v", err)
	}
	if summary.ToPlex != 1 || len(plex.unwatchLog) != 1 {
		t.Fatalf("新鲜数据后 reset 应传播：summary=%+v plex=%v", summary, plex.unwatchLog)
	}
}

func TestSameDirectionConvergenceNoWrites(t *testing.T) {
	plex, trakt := newFakeProvider("plex"), newFakeProvider("trakt")
	store := newFakeStore()
	key := movie(800)
	plex.setWatched(key, 1700000000, 1)
	trakt.setWatched(key, 1700000000, 1)
	e := newEngine(plex, trakt, store, true, true)
	if _, err := e.Run(context.Background(), "test"); err != nil {
		t.Fatalf("基线 Run 出错：%v", err)
	}

	// 双侧同时取消（Plex unwatch at=0 + Trakt reset at=0）：同向收敛，
	// 状态已一致，不应再产生任何写入
	plex.setUnwatched(key)
	delete(trakt.state, key)
	summary, err := e.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run 出错：%v", err)
	}
	if summary.ToPlex != 0 || summary.ToTrakt != 0 {
		t.Fatalf("同向收敛不应写入：summary=%+v", summary)
	}
	if len(plex.unwatchLog) != 0 || len(trakt.unwatchLog) != 0 {
		t.Fatalf("同向收敛不应写入：plex=%v trakt=%v", plex.unwatchLog, trakt.unwatchLog)
	}
	sh := store.items[key]
	if sh.PlexWatched == nil || *sh.PlexWatched || sh.TraktWatched == nil || *sh.TraktWatched {
		t.Fatalf("状态应记录双侧未看：%+v", sh)
	}
}

func TestEchoGuardSessionPush(t *testing.T) {
	plex, trakt := newFakeProvider("plex"), newFakeProvider("trakt")
	store := newFakeStore()
	key := movie(900)
	plex.setUnwatched(key)

	e := newEngine(plex, trakt, store, true, true)
	// 基线：Plex 未看 + Trakt 未看
	if _, err := e.Run(context.Background(), "test"); err != nil {
		t.Fatalf("基线 Run 出错：%v", err)
	}

	// 模拟会话轮询已推送（trakt 刚在 2 分钟前记录）而状态 plex 侧未更新
	pushAt := int64(1700000400)
	trakt.setWatched(key, pushAt, 1)
	sh := store.items[key]
	sh.SessionPushedAt = pushAt
	if err := store.SaveShadow([]Shadow{sh}); err != nil {
		t.Fatalf("SaveShadow 出错：%v", err)
	}
	// Plex 随后标记已看（Plex 侧自动 scrobble）
	plex.setWatched(key, pushAt+120, 1)

	summary, err := e.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run 出错：%v", err)
	}
	if summary.ToTrakt != 0 || len(trakt.watchLog) != 0 {
		t.Fatalf("echo 窗口内不应重复推送 Trakt：summary=%+v trakt=%v", summary, trakt.watchLog)
	}
	sh = store.items[key]
	if sh.PlexWatched == nil || !*sh.PlexWatched {
		t.Fatalf("状态 plex 侧应更新为已看：%+v", sh)
	}
}

func TestFailedWriteShadowUntouchedAndRetried(t *testing.T) {
	plex, trakt := newFakeProvider("plex"), newFakeProvider("trakt")
	store := newFakeStore()
	key := movie(1000)
	plex.setWatched(key, 1700000000, 1)

	e := newEngine(plex, trakt, store, true, true)
	// 首跑建立基线（trakt 侧未看）
	if _, err := e.Run(context.Background(), "test"); err != nil {
		t.Fatalf("基线 Run 出错：%v", err)
	}

	// Plex 侧重看 + Trakt 写入注入失败
	plex.setWatched(key, 1700010000, 2)
	trakt.failKeys[key] = true
	summary, err := e.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run 出错：%v", err)
	}
	if summary.ToTrakt != 0 {
		t.Fatalf("失败写入不应计数：%+v", summary)
	}
	sh := store.items[key]
	if sh.TraktWatched != nil && *sh.TraktWatched && sh.TraktLastAt == 1700010000 {
		t.Fatalf("失败键状态不应落盘：%+v", sh)
	}
	// 下一轮恢复后重试成功
	trakt.failKeys = nil
	summary, err = e.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("重试 Run 出错：%v", err)
	}
	if summary.ToTrakt != 1 {
		t.Fatalf("重试应成功：%+v", summary)
	}
}

func TestDirectionToggleOff(t *testing.T) {
	plex, trakt := newFakeProvider("plex"), newFakeProvider("trakt")
	store := newFakeStore()
	key := movie(1100)
	plex.setWatched(key, 1700000000, 1)

	// ToTrakt=false：Plex 变化不推 Trakt
	e := newEngine(plex, trakt, store, true, false)
	summary, err := e.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run 出错：%v", err)
	}
	if summary.ToTrakt != 0 || len(trakt.watchLog) != 0 {
		t.Fatalf("方向关闭时不应写入 Trakt：%+v", summary)
	}
	// 反向仍工作：Trakt 变化推 Plex（条目需在 Plex 枚举中存在）
	key2 := movie(1101)
	plex.setUnwatched(key2)
	trakt.setWatched(key2, 1700000000, 1)
	summary, err = e.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run 出错：%v", err)
	}
	if summary.ToPlex != 1 || len(plex.watchLog) == 0 {
		t.Fatalf("ToPlex 方向应正常：%+v plex=%v", summary, plex.watchLog)
	}
}

func TestUnresolvedCounted(t *testing.T) {
	plex, trakt := newFakeProvider("plex"), newFakeProvider("trakt")
	store := newFakeStore()
	// fakeProvider 不实现 UnresolvedReporter —— 直接构造一个实现的包装
	playback := &unresolvedPlex{fakeProvider: plex}
	e := &Engine{
		Playback: playback, Trakt: trakt, Store: store, ToPlex: true, ToTrakt: true,
	}
	summary, err := e.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run 出错：%v", err)
	}
	if summary.Unresolved != 2 {
		t.Fatalf("期望 2 条未识别，实际 %d", summary.Unresolved)
	}
}

type unresolvedPlex struct {
	*fakeProvider
	drained bool
}

func (u *unresolvedPlex) DrainUnresolved() []watched.UnresolvedItem {
	if u.drained {
		return nil
	}
	u.drained = true
	return []watched.UnresolvedItem{
		{Ref: "1", Title: "神秘影片", Reason: "无法解析 TMDB 身份"},
		{Ref: "2", Title: "另一部", Reason: "无法解析 TMDB 身份"},
	}
}

func TestRewatchPropagates(t *testing.T) {
	plex, trakt := newFakeProvider("plex"), newFakeProvider("trakt")
	store := newFakeStore()
	key := movie(1200)
	plex.setWatched(key, 1700000000, 1)
	trakt.setWatched(key, 1700000000, 1)
	e := newEngine(plex, trakt, store, true, true)
	if _, err := e.Run(context.Background(), "test"); err != nil {
		t.Fatalf("基线 Run 出错：%v", err)
	}

	// Plex 重看：时间戳前进 + 播放次数增加 → Trakt 追加一条播放
	plex.setWatched(key, 1700010000, 2)
	summary, err := e.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run 出错：%v", err)
	}
	if summary.ToTrakt != 1 || len(trakt.watchLog) != 1 || trakt.watchLog[0].At != 1700010000 {
		t.Fatalf("重看应推送 Trakt：summary=%+v trakt=%v", summary, trakt.watchLog)
	}
}

func TestSummaryCountsLogged(t *testing.T) {
	plex, trakt := newFakeProvider("plex"), newFakeProvider("trakt")
	store := newFakeStore()
	key := movie(1300)
	plex.setWatched(key, 1700000000, 1)

	e := newEngine(plex, trakt, store, true, true)
	if _, err := e.Run(context.Background(), "manual"); err != nil {
		t.Fatalf("Run 出错：%v", err)
	}
	if len(store.runs) != 1 || store.runs[0].ToTrakt != 1 {
		t.Fatalf("run 统计缺失或不符：%+v", store.runs)
	}
	if len(store.runItems) != 1 || store.runItems[0].Direction != "to_trakt" {
		t.Fatalf("run 明细缺失或不符：%+v", store.runItems)
	}
}
