import { useState, useCallback, useEffect } from 'react'
import { setUnauthorizedHandler, getMe, refreshLibrary } from './api'
import './index.css'
import LoginPage from './components/LoginPage'
import Topbar from './components/Topbar'
import Sidebar from './components/Sidebar'
import LibraryPage from './components/LibraryPage'
import ConfigPage from './components/ConfigPage'
import ToastContainer from './components/Toast'

let _toastId = 0

export default function App() {
  // ── 认证状态 ──────────────────────────────────────────
  const [token, setToken]           = useState(() => localStorage.getItem('auth_token'))
  const [checkingAuth, setChecking] = useState(true)

  // ── UI 状态 ──────────────────────────────────────────
  const [activeNav, setActiveNav]   = useState('all')
  const [toasts, setToasts]         = useState([])
  const [libraryKey, setLibraryKey] = useState(0)
  const [refreshing, setRefreshing] = useState(false)

  // ── 验证 token ──────────────────────────────────────
  useEffect(() => {
    if (!token) { setChecking(false); return }
    getMe()
      .then(() => setChecking(false))
      .catch(() => {
        localStorage.removeItem('auth_token')
        setToken(null)
        setChecking(false)
      })
  }, [token])

  // ── 注册 401 全局处理 ────────────────────────────────
  useEffect(() => {
    setUnauthorizedHandler(() => {
      localStorage.removeItem('auth_token')
      setToken(null)
    })
  }, [])

  // ── 登录 / 登出 ────────────────────────────────────
  const handleLogin = useCallback((newToken) => {
    localStorage.setItem('auth_token', newToken)
    setToken(newToken)
  }, [])

  const handleLogout = useCallback(() => {
    localStorage.removeItem('auth_token')
    setToken(null)
  }, [])

  // ── Toast ──────────────────────────────────────────
  const addToast = useCallback((type, title, message) => {
    const id = ++_toastId
    setToasts(prev => [...prev, { id, type, title, message }])
  }, [])

  const removeToast = useCallback((id) => {
    setToasts(prev => prev.filter(t => t.id !== id))
  }, [])

  // ── 刷新媒体库 ────────────────────────────────────
  async function handleRefresh() {
    if (refreshing) return
    setRefreshing(true)
    try {
      const res = await refreshLibrary()
      const { new_movies, new_tv, total_movies, total_tv } = res.data
      const parts = []
      if (new_movies > 0) parts.push(`新增 ${new_movies} 部电影`)
      if (new_tv > 0)     parts.push(`新增 ${new_tv} 部剧集`)
      const body = parts.length > 0
        ? parts.join('，')
        : `共 ${total_movies} 部电影，${total_tv} 部剧集，无变化`
      addToast('success', '媒体库已更新', body)
      setLibraryKey(k => k + 1)
    } catch (e) {
      addToast('error', '刷新失败', e?.response?.data?.detail || e.message)
    } finally {
      setRefreshing(false)
    }
  }

  // ── 鉴权检查中 ──────────────────────────────────
  if (checkingAuth) {
    return (
      <div className="min-h-screen flex items-center justify-center" style={{ background: 'var(--color-bg)' }}>
        <div style={{ color: 'var(--color-muted)', fontSize: 14 }}>正在验证…</div>
      </div>
    )
  }

  // ── 未登录 → 显示登录页 ──────────────────────────
  if (!token) {
    return (
      <>
        <LoginPage onLogin={handleLogin} />
        <ToastContainer toasts={toasts} onRemove={removeToast} />
      </>
    )
  }

  // ── 已登录 → 主界面 ──────────────────────────────
  return (
    <div className="min-h-screen" style={{ background: 'var(--color-bg)' }}>
      <Topbar onLogout={handleLogout} />
      <Sidebar active={activeNav} onSelect={setActiveNav} />
      <main className="pl-72 pt-14 min-h-screen" style={{ background: 'var(--color-bg)' }}>
        <div className="px-10 py-8 flex">
          {activeNav === 'config' ? (
            <ConfigPage />
          ) : (
            <LibraryPage
              key={libraryKey}
              filter={activeNav}
              onChangeFilter={setActiveNav}
              onRefresh={handleRefresh}
              refreshing={refreshing}
            />
          )}
        </div>
      </main>
      <ToastContainer toasts={toasts} onRemove={removeToast} />
    </div>
  )
}
