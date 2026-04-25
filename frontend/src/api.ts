import axios from 'axios'

const api = axios.create({ baseURL: '/api' })

// 未授权回调
let _onUnauthorized: (() => void) | null = null
export function setUnauthorizedHandler(fn: () => void) { _onUnauthorized = fn }

// 请求拦截：自动附加 Bearer token
api.interceptors.request.use(config => {
  const token = localStorage.getItem('auth_token')
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

// 响应拦截：401 时触发登出
api.interceptors.response.use(
  res => res,
  err => {
    if (err.response?.status === 401 && _onUnauthorized) {
      _onUnauthorized()
    }
    return Promise.reject(err)
  }
)

// ── 认证 ──
export const login = (username: string, password: string) =>
  axios.post('/api/auth/login', { username, password })

export const getMe = () => api.get('/auth/me')
export const logout = () => api.post('/auth/logout')

// ── 媒体库 ──
export const getLibrary = () => api.get('/library')
export const getMovies = () => api.get('/library/movies')
export const getTvShows = () => api.get('/library/tv')
export const getStats = () => api.get('/stats')
export const getTvDetail = (tmdbId: number) => api.get(`/tv/${tmdbId}`)
export const refreshLibrary = () => api.post('/library/refresh')
export const refreshMediaItem = (
  tmdb_id: number,
  media_type: string,
  drive_folder_id: string,
  title?: string,
  year?: string
) => api.post('/library/refresh-item', { tmdb_id, media_type, drive_folder_id, title, year })

// ── TMDB/刮削 ──
export const tmdbSearchMulti = (keyword: string, config?: { signal?: AbortSignal }) =>
  api.get('/tmdb/search_multi', { params: { keyword }, ...config })
export const tmdbGetDetail = (media_type: string, tmdb_id: number) =>
  api.get('/tmdb/detail', { params: { media_type, tmdb_id } })
export const tmdbGetAlternativeNames = (media_type: string, tmdb_id: number, config?: { signal?: AbortSignal }) =>
  api.get('/tmdb/alternative_names', { params: { media_type, tmdb_id }, ...config })
export const tmdbGetEpisodes = (id: number, season: number) =>
  api.get(`/tmdb/tv/${id}/season/${season}`)
export const searchMedia = (keyword: string, config?: { signal?: AbortSignal }) =>
  api.get('/scraper/search_media', { params: { keyword }, ...config })
export const getEpisodes = (site: string, media_id: string, subgroup_id?: string, config?: { signal?: AbortSignal }) =>
  api.get('/scraper/get_episodes', { params: { site, media_id, subgroup_id }, ...config })

// ── 订阅 ──
export const listSubscriptions = () => api.get('/subscriptions')
export const getSubscription = (id: number) => api.get(`/subscriptions/${id}`)
export const createSubscription = (data: Record<string, unknown>) =>
  api.post('/subscriptions', data)
export const updateSubscription = (id: number, data: Record<string, unknown>) =>
  api.put(`/subscriptions/${id}`, data)
export const deleteSubscription = (id: number) => api.delete(`/subscriptions/${id}`)
export const testSubscription = (data: Record<string, unknown>) =>
  api.post('/subscriptions/test', data)
export const checkSubscription = (id: number) =>
  api.post(`/subscriptions/${id}/check`)

// ── 配置 ──
export const getConfig = () => api.get('/config')
export const saveConfig = (data: Record<string, unknown>) =>
  api.put('/config', { data })
export const getMainConfig = () => api.get('/config/main')
export const saveMainConfig = (data: Record<string, unknown>) =>
  api.put('/config/main', { data })
export const getParserRulesConfig = () => api.get('/config/parser-rules')
export const saveParserRulesConfig = (data: Record<string, unknown>) =>
  api.put('/config/parser-rules', { data })
export const testParse = (filename: string, skipTmdb: boolean = false) =>
  api.post('/parser/test', { filename }, { params: { skip_tmdb: skipTmdb } })

// ── Aria2 ──
export const getAria2Overview = (params?: { queue?: string; page?: number; page_size?: number; search?: string }) =>
  api.get('/aria2/overview', { params })
export const getAria2Options = () => api.get('/aria2/options')
export const saveAria2Options = (data: Record<string, unknown>) =>
  api.put('/aria2/options', data)
export const addAria2Uri = (data: Record<string, unknown>) =>
  api.post('/aria2/add-uri', data)
export const addAria2Torrent = (data: Record<string, unknown>) =>
  api.post('/aria2/add-torrent', data)
export const pauseAria2Tasks = (gids: string[]) =>
  api.post('/aria2/tasks/pause', { gids })
export const unpauseAria2Tasks = (gids: string[]) =>
  api.post('/aria2/tasks/unpause', { gids })
export const removeAria2Tasks = (gids: string[]) =>
  api.post('/aria2/tasks/remove', { gids })
export const retryAria2Tasks = (gids: string[]) =>
  api.post('/aria2/tasks/retry', { gids })
export const purgeAria2Tasks = () => api.post('/aria2/tasks/purge')

// ── 115 网盘 ──
export const getU115OauthStatus = () => api.get('/u115/oauth/status')
export const createU115OauthSession = (data: Record<string, unknown>, config?: { signal?: AbortSignal }) =>
  api.post('/u115/oauth/create', data, config)
export const fetchU115QrCode = (config?: { signal?: AbortSignal }) =>
  api.get('/u115/oauth/qrcode', { responseType: 'blob', ...config })
export const pollU115OauthStatus = (config?: { signal?: AbortSignal }) =>
  api.get('/u115/oauth/poll', { params: { ts: Date.now() }, ...config })
export const exchangeU115OauthToken = (data: Record<string, unknown>, config?: { signal?: AbortSignal }) =>
  api.post('/u115/oauth/exchange', data, config)
export const testU115Connection = () => api.post('/u115/test')
export const testU115Cookie = () => api.post('/u115/test-cookie')
export const getU115OfflineOverview = (params?: Record<string, unknown>) =>
  api.get('/u115/offline/overview', { params })
export const getU115OfflineQuota = () => api.get('/u115/offline/quota')
export const getU115AutoOrganizeStatus = () => api.get('/u115/offline/auto-organize-status')
export const addU115OfflineUrls = (data: { urls: string; wp_path_id?: string }) =>
  api.post('/u115/offline/add-urls', data)
export const deleteU115OfflineTasks = (data: { info_hashes: string[]; del_source_file?: number }) =>
  api.post('/u115/offline/tasks/delete', data)
export const clearU115OfflineTasks = (data: { flag: string }) =>
  api.post('/u115/offline/tasks/clear', data)

// ── 其他 ──
export const getLogs = (params?: { limit?: number; offset?: number }) =>
  api.get('/logs', { params })
export const getPipelineStatus = () => api.get('/pipeline/status')
export const triggerPipeline = () => api.post('/pipeline/trigger')

// ── Google Drive ──
export const getDriveOauthStatus = () => api.get('/drive/oauth/status')
export const testDriveConnection = () => api.post('/drive/test')
