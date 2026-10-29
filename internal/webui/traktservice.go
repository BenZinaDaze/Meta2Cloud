// traktservice.go —— 观看同步域服务：scrobble 会话、SSE、会话轮询、双向同步
// 与 Trakt/Plex 授权状态负载（方法分布于 scrobble.go / plexsse.go /
// traktsvc.go / traktsyncsvc.go，接收器统一为 s）。
package webui

import (
	"sync"
	"time"
)

// traktService 观看同步域服务，持有该域全部运行态。
type traktService struct {
	rt *Runtime

	scrobbleMu   sync.Mutex
	scrobbleByID map[string]*sessionScrobble

	sse struct {
		mu          sync.Mutex
		connected   bool
		lastEventAt time.Time
	}

	run struct {
		mu      sync.Mutex
		running bool
	}

	syncWatcher    *watcherState // 定时全量同步心跳
	sessionWatcher *watcherState // Plex 会话轮询心跳
	sseWatcher     *watcherState // SSE 连接生命周期

	sessionOnce sync.Once
	sseOnce     sync.Once

	// 异常提醒节流：持续异常每小时至多一条
	anomalyMu   sync.Mutex
	anomalyLast time.Time
}

// scrobbleStateFor 取（或创建）会话的推送观察状态。
func (s *traktService) scrobbleStateFor(sessionKey string) *sessionScrobble {
	s.scrobbleMu.Lock()
	defer s.scrobbleMu.Unlock()
	st := s.scrobbleByID[sessionKey]
	if st == nil {
		st = &sessionScrobble{}
		s.scrobbleByID[sessionKey] = st
	}
	return st
}

func (s *traktService) dropScrobbleState(sessionKey string) {
	s.scrobbleMu.Lock()
	delete(s.scrobbleByID, sessionKey)
	s.scrobbleMu.Unlock()
}
