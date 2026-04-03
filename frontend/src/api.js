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
export const getStats    = () => api.get('/stats')
export const getTvDetail = (tmdbId) => api.get(`/tv/${tmdbId}`)

// ── 配置 ──
export const getConfig  = () => api.get('/config')
export const saveConfig = (data) => api.put('/config', { data })

// ── 媒体库刷新 ──
export const refreshLibrary = () => api.post('/library/refresh')
