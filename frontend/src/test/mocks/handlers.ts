// handlers.ts —— MSW 默认请求处理器：覆盖页面冒烟测试涉及的只读端点，
// 返回最小合法负载（字段与 types/api.ts 对应）。单个测试用 server.use 覆盖。
import { http, HttpResponse } from 'msw'

export const handlers = [
  http.get('/api/auth/me', () => HttpResponse.json({ username: 'tester' })),

  // 仪表盘
  http.get('/api/stats', () =>
    HttpResponse.json({
      total_movies: 3, total_tv_shows: 2,
      total_episodes_in_library: 20, total_episodes_on_tmdb: 40,
      completion_rate: 50,
    }),
  ),
  http.get('/api/ingest/stats', () =>
    HttpResponse.json({ days: 7, total: 5, movies: 2, tv_episodes: 3, success: 4, failed: 1, no_tmdb: 0, deleted: 0 }),
  ),
  http.get('/api/ingest/stats/daily', () =>
    HttpResponse.json([{ date: '2026-10-29', total: 2, movies: 1 }]),
  ),
  http.get('/api/ingest/recent', () => HttpResponse.json([])),
  http.get('/api/pipeline/status', () => HttpResponse.json({ running: false, debounce: false })),
  http.get('/api/strm/sync/status', () =>
    HttpResponse.json({
      enabled: true, url_base: 'http://m2c:38765', mappings: 1, running: false,
      total_files: 12, last_run: null,
    }),
  ),
  http.get('/api/strm/sync/runs', () => HttpResponse.json({ total: 0, runs: [] })),
  http.get('/api/strm/playback/history', () =>
    HttpResponse.json({
      total: 0, items: [],
      stats: { total: 0, today: 0, avg_latency_ms: 0, cache_hit_rate: 0, distinct_clients: 0 },
    }),
  ),
  http.get('/api/trakt/sync/status', () =>
    HttpResponse.json({
      enabled: false, trakt_ready: false, trakt_authorized: false, plex_ready: false,
      plex_connected: false, plex_reachable: false, sync_to_plex: true, sync_to_trakt: true,
      sync_cron: '*/15 * * * *', session_poll_seconds: 45, running: false,
      sync_watcher_alive: true, session_watcher_alive: true, sse_enabled: true, sse_connected: false,
      last_polled_at: null, last_poll_error: null, last_session_polled_at: null, last_run: null,
    }),
  ),
  http.get('/api/u115/space', () => HttpResponse.json({ total_space: 1_000_000, remain_space: 500_000 })),
  http.get('/api/u115/oauth/status', () =>
    HttpResponse.json({
      client_id: '100197847', token_path: 'config/115-token.json', session_path: 'config/115-device-session.json',
      cookie_configured: false, token_exists: false, session_exists: false,
      authorized: false, token_valid: false, token_expired: false,
      refresh_time: null, expires_at: null, refreshed: false, refresh_error: '',
    }),
  ),
  http.get('/api/u115/offline/overview', () =>
    HttpResponse.json({ ok: true, tasks: [], pagination: { page: 1, page_size: 0, total: 0, total_pages: 1, has_prev: false, has_next: false } }),
  ),

  // 媒体库 / 分类 / 配置 / 日志
  http.get('/api/library', () =>
    HttpResponse.json({ movies: [], tv_shows: [], total_movies: 0, total_tv: 0, scanned_at: null, page: 1, page_size: 50 }),
  ),
  http.get('/api/categories', () =>
    HttpResponse.json({
      dirs: { tv: '电视剧', movie: '电影', other: '其他' },
      tv: [{ name: '日番' }],
      movie: [{ name: '动画电影' }],
    }),
  ),
  http.get('/api/config', () => HttpResponse.json({ webui: { username: 'admin' } })),
  http.get('/api/logs', () => HttpResponse.json({ items: [], summary: {}, maxId: 0 })),
]
