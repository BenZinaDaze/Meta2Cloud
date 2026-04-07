import axios from 'axios'

const api = axios.create({ baseURL: '/api' })

// ── 未授权回调（由 App.jsx 注册）──
let _onUnauthorized = null
export function setUnauthorizedHandler(fn) { _onUnauthorized = fn }

// ── 请求拦截：自动附加 Bearer token ──
api.interceptors.request.use(config => {
  const token = localStorage.getItem('auth_token')
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

// ── 响应拦截：401 时触发登出 ──
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
export const login   = (username, password) =>
  axios.post('/api/auth/login', { username, password })

export const getMe   = () => api.get('/auth/me')
export const logout  = () => api.post('/auth/logout')

// ── 媒体库 ──
export const getLibrary  = () => api.get('/library')
export const getMovies   = () => api.get('/library/movies')
export const getTvShows  = () => api.get('/library/tv')
export const tmdbGetEpisodes = (id, season) => api.get(`/tmdb/tv/${id}/season/${season}`)

// ── Scraper ──
export const tmdbSearchMulti = (keyword) => api.get('/tmdb/search_multi', { params: { keyword } })
export const tmdbGetDetail = (media_type, tmdb_id) => api.get('/tmdb/detail', { params: { media_type, tmdb_id } })
export const tmdbGetAlternativeNames = (media_type, tmdb_id) => api.get('/tmdb/alternative_names', { params: { media_type, tmdb_id } })
export const searchMedia = (keyword) => api.get('/scraper/search_media', { params: { keyword } })
export const getEpisodes = (site, media_id, subgroup_id) => api.get('/scraper/get_episodes', { params: { site, media_id, subgroup_id } })

export const getStats    = () => api.get('/stats')
export const getTvDetail = (tmdbId) => api.get(`/tv/${tmdbId}`)

// ── 配置 ──
export const getConfig  = () => api.get('/config')
export const saveConfig = (data) => api.put('/config', { data })
export const testParse = (filename) => api.post('/parser/test', { filename })

// ── 媒体库刷新 ──
export const refreshLibrary = () => api.post('/library/refresh')
export const getLogs = (params) => api.get('/logs', { params })
export const getPipelineStatus = () => api.get('/pipeline/status')
export const triggerPipeline = () => api.post('/pipeline/trigger')

// ── Aria2 下载管理 ──
export const getAria2Overview = () => api.get('/aria2/overview')
export const getAria2Options = () => api.get('/aria2/options')
export const saveAria2Options = (data) => api.put('/aria2/options', data)
export const addAria2Uri = (data) => api.post('/aria2/add-uri', data)
export const addAria2Torrent = (data) => api.post('/aria2/add-torrent', data)
export const pauseAria2Tasks = (gids) => api.post('/aria2/tasks/pause', { gids })
export const unpauseAria2Tasks = (gids) => api.post('/aria2/tasks/unpause', { gids })
export const removeAria2Tasks = (gids) => api.post('/aria2/tasks/remove', { gids })
export const retryAria2Tasks = (gids) => api.post('/aria2/tasks/retry', { gids })
export const purgeAria2Tasks = () => api.post('/aria2/tasks/purge')
