import { useState, useCallback, useEffect } from 'react'
import { setUnauthorizedHandler, getMe, refreshLibrary, getAria2Overview } from './api'
import './index.css'
import LoginPage from './components/LoginPage'
import Topbar from './components/Topbar'
import Sidebar from './components/Sidebar'
import LibraryPage from './components/LibraryPage'
import ConfigPage from './components/ConfigPage'
import DownloadsPage from './components/DownloadsPage'
import LogsPage from './components/LogsPage'
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
  const [aria2Overview, setAria2Overview] = useState(null)
  const [aria2ConnectionStatus, setAria2ConnectionStatus] = useState('connecting')

  const downloadQueue = {
    downloads: 'all',
    'downloads-active': 'active',
    'downloads-waiting': 'waiting',
    'downloads-stopped': 'stopped',
  }[activeNav]

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

  // ── 全局维持 aria2 连接状态 ─────────────────────────
  useEffect(() => {
    if (!token) {
      setAria2Overview(null)
      setAria2ConnectionStatus('connecting')
      return
    }

    let cancelled = false

    async function loadAria2Overview() {
      try {
        const res = await getAria2Overview()
        if (!cancelled) {
          setAria2Overview(res.data)
          setAria2ConnectionStatus('connected')
        }
      } catch {
        if (!cancelled) {
          setAria2Overview(null)
          setAria2ConnectionStatus('error')
        }
      }
    }

    setAria2ConnectionStatus('connecting')
    loadAria2Overview()
    const id = setInterval(loadAria2Overview, 5000)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [token])

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
      <div className="app-shell flex min-h-dvh items-center justify-center px-6">
        <div
          className="panel-surface rounded-[28px] px-8 py-6 text-sm"
          style={{ color: 'var(--color-muted)' }}
        >
          正在验证身份信息…
        </div>
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
    <div className="app-shell">
      <Topbar onLogout={handleLogout} />
      <Sidebar
        active={activeNav}
        onSelect={setActiveNav}
        aria2Overview={aria2Overview}
        aria2ConnectionStatus={aria2ConnectionStatus}
      />
      <main
        className="fixed right-0 bottom-0 overflow-y-auto pr-5 pb-5"
        style={{
          left: 'calc(18rem + 2.5rem)',
          top: '96px',
        }}
      >
        <div className="flex">
          {activeNav === 'config' ? (
            <ConfigPage />
          ) : activeNav === 'logs' ? (
            <LogsPage />
          ) : downloadQueue ? (
            <DownloadsPage
              queue={downloadQueue}
              initialOverview={aria2Overview}
              onChangeQueue={(queue) => {
                const nextNav = {
                  all: 'downloads',
                  active: 'downloads-active',
                  waiting: 'downloads-waiting',
                  stopped: 'downloads-stopped',
                }[queue]
                setActiveNav(nextNav || 'downloads')
              }}
              onToast={addToast}
            />
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
