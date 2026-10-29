// 媒体项
export interface MediaItem {
  tmdb_id: number
  title: string
  original_title: string
  year: string
  media_type: 'movie' | 'tv'
  poster_url?: string
  backdrop_url?: string
  overview: string
  rating: number
  seasons?: SeasonStatus[]
  total_episodes?: number
  in_library_episodes?: number
  status?: string
  drive_folder_id?: string
  updated_at?: string
  folder_modified_time?: number
  in_library?: boolean
  genre_names?: string[]
  original_language?: string
  runtime?: number
}

export interface ReidentifyResponse {
  ok: boolean
  partial?: boolean
  tmdb_id: number
  uploaded: string[]
  errors: string[]
  rename_errors?: string[]
  renamed?: boolean
  folder_name?: string
  files_renamed?: number
  reclassified?: boolean
  category?: string
  reclassify_errors?: string[]
  history_rewritten?: number
  item?: MediaItem
}

// 季状态
export interface SeasonStatus {
  season_number: number
  season_name: string
  poster_url?: string
  episode_count: number
  in_library_count: number
  episodes: EpisodeStatus[]
}

// 剧集状态
export interface EpisodeStatus {
  episode_number: number
  episode_title: string
  air_date: string
  in_library: boolean
}

// 媒体库响应
export interface LibraryResponse {
  movies: MediaItem[]
  tv_shows: MediaItem[]
  total_movies: number
  total_tv: number
  scanned_at?: string
  hint?: string
  page?: number
  page_size?: number
}

// 订阅命中记录
export interface SubscriptionHit {
  id: number
  episode_title: string
  season_number?: number
  episode_number?: number
  push_status?: string
  created_at?: string
}

// 订阅库状态
export interface SubscriptionLibrary {
  in_library?: boolean
  in_library_episodes?: number
  total_episodes?: number
  poster_url?: string
  year?: string
}

// 订阅 TMDB 信息
export interface SubscriptionTmdb {
  title?: string
  original_title?: string
  overview?: string
  rating?: number
  status?: string
  release_date?: string
  poster_path?: string
  poster_url?: string
  backdrop_path?: string
  backdrop_url?: string
}

// 订阅
export interface Subscription {
  id: number
  name: string
  media_title: string
  media_type: 'movie' | 'tv'
  tmdb_id?: number
  poster_url?: string
  site: string
  rss_url: string
  subgroup_name?: string
  season_number: number
  start_episode: number
  keyword_all: string[]
  push_target: string
  enabled: boolean
  created_at?: string
  updated_at?: string
  hit_count?: number
  library?: SubscriptionLibrary
  tmdb?: SubscriptionTmdb
  recent_hits?: SubscriptionHit[]
}

// 用户
export interface User {
  username: string
}

// 入库记录
export interface IngestRecord {
  id: number
  media_type: 'movie' | 'tv'
  tmdb_id: number
  title: string
  original_title: string
  year: string
  season?: number
  episode?: number
  episode_title: string
  poster_path: string
  poster_url?: string
  drive_folder_id: string
  original_name: string
  status: 'success' | 'failed' | 'no_tmdb' | 'deleted'
  error_message: string
  ingested_at: string
}

export interface IngestPagination {
  page: number
  page_size: number
  total: number
  total_pages: number
}

export interface IngestHistoryResponse {
  items: IngestRecord[]
  pagination: IngestPagination
}

export interface IngestStatsResponse {
  days: number
  total: number
  movies: number
  tv_episodes: number
  success: number
  failed: number
  no_tmdb: number
  deleted: number
}

// 文件管理
export interface CloudFile {
  id: string
  name: string
  file_type: 'file' | 'folder'
  size?: number
  modified_time?: string
  parent_id?: string
  mime_type?: string
  is_video: boolean
  is_subtitle: boolean
  extension: string
  extra: Record<string, unknown>
}

export interface PathBreadcrumbItem {
  id: string
  name: string
}

export interface ListFilesResponse {
  items: CloudFile[]
  folder_id: string
  folder_name?: string
  parent_id?: string
  provider: string
  root_id: string
  path: PathBreadcrumbItem[]
}

// 整理名称（标题钉扎）
export interface TitlePin {
  media_type: string
  tmdb_id: number
  pinned_title: string
  pinned_year: string
  source: string
  created_at: string
  updated_at: string
  current_title: string
  current_year: string
  drifted: boolean
  folder_ids: string[]
}

export interface TitlePinsResponse {
  items: TitlePin[]
}

// 媒体分类规则
export interface CategoryRule {
  name: string
  genre_ids?: number[]
  languages?: string[]
}

export interface CategoryDirs {
  tv: string
  movie: string
  other: string
}

export interface CategoriesConfig {
  dirs: CategoryDirs
  tv: CategoryRule[]
  movie: CategoryRule[]
}

// ── 观看同步（Trakt ↔ Plex）──
// 与后端 payload struct 字段一一对应（internal/webui/traktsvc.go / traktsyncsvc.go）

/** 进行中的 Trakt 设备码配对会话 */
export interface TraktPairing {
  user_code: string
  verification_url: string
  expires_at: number
}

/** GET /api/trakt/oauth/status（TraktOAuthStatusPayload） */
export interface TraktOauthStatus {
  ready: boolean
  client_id: string
  token_path: string
  token_exists: boolean
  authorized: boolean
  token_valid: boolean
  token_expired: boolean
  refresh_time: number | null
  expires_at: number | null
  refreshed: boolean
  pairing: TraktPairing | null
}

/** 同步运行记录（status.last_run 与 runs 列表共用形状） */
export interface WatchedSyncRun {
  id: number
  started_at: string
  finished_at: string | null
  trigger: string
  to_trakt: number
  to_plex: number
  conflicts: number
  unresolved: number
  skipped: number
  error: string | null
  /** 实时推送（sse/poll）记录的媒体标题与进度；全量同步为 null */
  title: string | null
  progress: number | null
}

/** GET /api/trakt/sync/status（TraktSyncStatusPayload） */
export interface WatchedSyncStatus {
  enabled: boolean
  trakt_ready: boolean
  trakt_authorized: boolean
  plex_ready: boolean
  plex_connected: boolean
  plex_reachable: boolean
  sync_to_plex: boolean
  sync_to_trakt: boolean
  sync_cron: string
  session_poll_seconds: number
  running: boolean
  sync_watcher_alive: boolean
  session_watcher_alive: boolean
  sse_enabled: boolean
  sse_connected: boolean
  last_polled_at: string | null
  last_poll_error: string | null
  last_session_polled_at: string | null
  last_run: WatchedSyncRun | null
}

/** 同步明细单条 */
export interface WatchedSyncRunItem {
  id: number
  media_type: string | null
  tmdb_id: number | null
  season: number | null
  episode: number | null
  title: string | null
  direction: string
  action: string
  success: boolean
  error: string | null
}

/** 忽略清单条目 */
export interface WatchedSyncIgnore {
  media_type: string
  anchor: string
  tmdb_id: number
  title: string | null
  reason: string | null
  created_at: string
}

/** 反复失败候选（建议忽略） */
export interface WatchedFailingMedia {
  media_type: string
  anchor: string
  tmdb_id: number
  title: string
  fails: number
  last_error: string | null
  last_at: string
}

/** GET /api/trakt/sync/ignores */
export interface WatchedSyncIgnoresResponse {
  ignores: WatchedSyncIgnore[]
  candidates: WatchedFailingMedia[]
}
