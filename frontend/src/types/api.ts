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
  in_library?: boolean
  genre_names?: string[]
  original_language?: string
  runtime?: number
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

// Aria2 任务
export interface Aria2Task {
  gid: string
  status: 'active' | 'waiting' | 'paused' | 'error' | 'complete' | 'removed'
  totalLength: string
  completedLength: string
  downloadSpeed: string
  uploadSpeed?: string
  connections?: number
  filename?: string
  title?: string
  error?: string
  progress?: number
}

// Aria2 分页信息
export interface Aria2Pagination {
  page: number
  page_size: number
  total: number
  total_pages: number
  queue?: string
  search?: string
}

// Aria2 概览
export interface Aria2Overview {
  items: Aria2Task[]
  pagination: Aria2Pagination
  summary?: {
    activeCount: number
    waitingCount: number
    stoppedCount: number
    downloadSpeed: number
    uploadSpeed: number
  }
  version?: string
}

// 统计
export interface StatsResponse {
  total_movies: number
  total_tv_shows: number
  total_episodes_in_library: number
  total_episodes_on_tmdb: number
  completion_rate: number
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
  status: 'success' | 'failed' | 'no_tmdb'
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
}
