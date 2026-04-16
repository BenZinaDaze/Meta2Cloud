import { useState, useCallback, useEffect, useRef } from 'react'
import { setUnauthorizedHandler, getMe, refreshLibrary, getAria2Overview, getConfig } from './api'
import './index.css'
import LoginPage from './components/LoginPage'
import Topbar from './components/Topbar'
import Sidebar from './components/Sidebar'
import LibraryPage from './components/LibraryPage'
import ConfigPage from './components/ConfigPage'
import DownloadsPage from './components/DownloadsPage'
import LogsPage from './components/LogsPage'
import ScraperSearch from './components/ScraperSearch'
import { clearResultsCache } from './utils/resultsCache'
import ParseTestModal from './components/ParseTestModal'
import CalendarPage from './components/CalendarPage'
import ToastContainer from './components/Toast'
import U115OfflinePage from './components/U115OfflinePage'
import SubscriptionsPage from './components/SubscriptionsPage'
import { getU115OauthStatus } from './api'

let _toastId = 0

// 移动端底部导航图标
const MobileNavIcons = {
  library: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z"/>
      <path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z"/>
    </svg>
  ),
  download: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
      <polyline points="7 10 12 15 17 10"/>
      <line x1="12" y1="15" x2="12" y2="3"/>
    </svg>
  ),
  cloudDownload: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <path d="M20 17.58A5 5 0 0 0 18 8h-1.26A8 8 0 1 0 4 16.25" />
      <path d="M12 12v9" />
      <path d="m8 17 4 4 4-4" />
    </svg>
  ),
  search: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="11" cy="11" r="8"/>
      <line x1="21" y1="21" x2="16.65" y2="16.65"/>
    </svg>
  ),
  calendar: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="4" width="18" height="18" rx="2" ry="2"/>
      <line x1="16" y1="2" x2="16" y2="6"/>
      <line x1="8" y1="2" x2="8" y2="6"/>
      <line x1="3" y1="10" x2="21" y2="10"/>
      <line x1="8" y1="14" x2="8.01" y2="14"/>
      <line x1="12" y1="14" x2="12.01" y2="14"/>
      <line x1="16" y1="14" x2="16.01" y2="14"/>
    </svg>
  ),
  settings: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="3"/>
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>
    </svg>
  ),
  menu: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <line x1="3" y1="6" x2="21" y2="6"/>
      <line x1="3" y1="12" x2="21" y2="12"/>
      <line x1="3" y1="18" x2="21" y2="18"/>
    </svg>
  ),
}

function MobileNav({ active, onSelect, onToggleSidebar }) {
  const isLibrary = ['all', 'movies', 'tv'].includes(active)
  const isMore = ['downloads', 'downloads-active', 'downloads-waiting', 'downloads-stopped', 'logs', 'config', 'config-filename-rules'].includes(active)

  const tabs = [
    { key: 'all', label: '媒体库', icon: MobileNavIcons.library, isActive: isLibrary },
    { key: 'scraper-search', label: '检索', icon: MobileNavIcons.search, isActive: active === 'scraper-search' },
    { key: 'calendar', label: '新番', icon: MobileNavIcons.calendar, isActive: active === 'calendar' },
    { key: 'u115-offline', label: '云下载', icon: MobileNavIcons.cloudDownload, isActive: active === 'u115-offline' },
  ]

  return (
    <nav
      className="fixed bottom-0 left-0 right-0 z-40 lg:hidden"
      style={{
        background: 'linear-gradient(180deg, rgba(10, 19, 32, 0.97) 0%, rgba(7, 14, 24, 1) 100%)',
        borderTop: '1px solid rgba(144, 178, 221, 0.18)',
        backdropFilter: 'blur(20px)',
        WebkitBackdropFilter: 'blur(20px)',
        minHeight: 'var(--mobile-nav-height)',
        paddingBottom: 'env(safe-area-inset-bottom)',
      }}
    >
      <div className="flex">
        {tabs.map(tab => (
          <button
            key={tab.key}
            onClick={() => onSelect(tab.key)}
            className="flex min-h-16 flex-1 flex-col items-center justify-center gap-1 py-2.5 transition-all duration-150"
            style={{
              color: tab.isActive ? 'var(--color-accent-hover)' : 'var(--color-muted)',
              background: 'none',
              border: 'none',
              cursor: 'pointer',
            }}
          >
            <span style={{ color: tab.isActive ? 'var(--color-accent)' : 'inherit' }}>
              {tab.icon}
            </span>
            <span style={{ fontSize: 10, fontWeight: tab.isActive ? 600 : 500 }}>
              {tab.label}
            </span>
          </button>
        ))}
        {/* 汉堡菜单——展开侧边栏 */}
        <button
          onClick={onToggleSidebar}
          className="flex min-h-16 flex-1 flex-col items-center justify-center gap-1 py-2.5 transition-all duration-150"
          style={{
            color: isMore ? 'var(--color-accent-hover)' : 'var(--color-muted)',
            background: 'none',
            border: 'none',
            cursor: 'pointer',
          }}
        >
          <span style={{ color: isMore ? 'var(--color-accent)' : 'inherit' }}>
            {MobileNavIcons.menu}
          </span>
          <span style={{ fontSize: 10, fontWeight: isMore ? 600 : 500 }}>更多</span>
        </button>
      </div>
    </nav>
  )
}

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
  const [aria2Enabled, setAria2Enabled] = useState(null)
  const [showParseTest, setShowParseTest] = useState(false)
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false)
  const [initialSearchItem, setInitialSearchItem] = useState(null)
  const [initialQuery, setInitialQuery] = useState(null)
  const [u115Authorized, setU115Authorized] = useState(false)

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

  useEffect(() => {
    if (!token) {
      setAria2Enabled(null)
      setU115Authorized(false)
      return
    }

    let cancelled = false

    getConfig()
      .then((res) => {
        if (!cancelled) {
          setAria2Enabled(res?.data?.aria2?.enabled !== false && res?.data?.aria2?.auto_connect !== false)
        }
      })
      .catch(() => {
        if (!cancelled) {
          setAria2Enabled(false)
        }
      })

    return () => {
      cancelled = true
    }
  }, [token])

  useEffect(() => {
    if (!token) {
      setU115Authorized(false)
      return
    }

    let cancelled = false
    getU115OauthStatus()
      .then((res) => {
        if (!cancelled) setU115Authorized(!!res?.data?.authorized)
      })
      .catch(() => {
        if (!cancelled) setU115Authorized(false)
      })
    return () => { cancelled = true }
  }, [token])

  // ── 注册 401 全局处理 ────────────────────────────────
  useEffect(() => {
    setUnauthorizedHandler(() => {
      localStorage.removeItem('auth_token')
      setToken(null)
    })
  }, [])

  // ── 全局维持 aria2 连接状态 ─────────────────────────
  const aria2ErrorCount = useRef(0)
  const aria2PollTimer = useRef(null)

  useEffect(() => {
    if (!token) {
      setAria2Overview(null)
      setAria2ConnectionStatus('connecting')
      return
    }

    if (aria2Enabled === null) {
      setAria2Overview(null)
      setAria2ConnectionStatus('connecting')
      return
    }

    if (!aria2Enabled) {
      setAria2Overview(null)
      setAria2ConnectionStatus('disabled')
      return
    }

    let cancelled = false

    async function loadAria2Overview() {
      try {
        const res = await getAria2Overview()
        if (!cancelled) {
          setAria2Overview(res.data)
          setAria2ConnectionStatus('connected')
          aria2ErrorCount.current = 0
        }
      } catch {
        if (!cancelled) {
          setAria2Overview(null)
          setAria2ConnectionStatus('error')
          aria2ErrorCount.current += 1
        }
      } finally {
        if (!cancelled) {
          const delays = [5000, 10000, 20000, 30000]
          const delay = delays[Math.min(aria2ErrorCount.current, delays.length - 1)]
          aria2PollTimer.current = setTimeout(loadAria2Overview, delay)
        }
      }
    }

    setAria2ConnectionStatus('connecting')
    loadAria2Overview()
    return () => {
      cancelled = true
      if (aria2PollTimer.current) clearTimeout(aria2PollTimer.current)
    }
  }, [token, aria2Enabled])

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
      <Topbar
        onLogout={handleLogout}
        onOpenParseTest={() => setShowParseTest(true)}
        onToggleSidebar={() => setMobileSidebarOpen(o => !o)}
        onToast={addToast}
        mobileSidebarOpen={mobileSidebarOpen}
      />
      <Sidebar
        active={activeNav}
        onSelect={(key) => { setActiveNav(key); setMobileSidebarOpen(false) }}
        aria2Overview={aria2Overview}
        aria2ConnectionStatus={aria2ConnectionStatus}
        mobileOpen={mobileSidebarOpen}
        onMobileClose={() => setMobileSidebarOpen(false)}
        onLogout={handleLogout}
      />

      {/* 移动端侧边栏遮罩 */}
      {mobileSidebarOpen && (
        <div
          className="fixed inset-0 z-[35] lg:hidden"
          style={{ background: 'rgba(3,10,19,0.65)', backdropFilter: 'blur(4px)', WebkitBackdropFilter: 'blur(4px)' }}
          onClick={() => setMobileSidebarOpen(false)}
        />
      )}

      {/* 主内容区 */}
      <main
        className="main-scrollable fixed inset-x-0 bottom-0 overflow-y-auto"
      >
        {/* 
          桌面端(lg+)：左边让给侧边栏(18rem + 2.5rem padding)
          移动端:      满宽，底部为移动导航栏留空间
        */}
        <div
          className="main-pad flex min-h-full flex-col px-3 pt-3 sm:px-4 lg:pl-[calc(18rem+2.5rem)] lg:pr-5"
          style={{ paddingBottom: 'calc(var(--mobile-nav-height) + 0.75rem)' }}
        >
          {activeNav === 'config' ? (
            <ConfigPage onAria2EnabledChange={setAria2Enabled} page="general" />
          ) : activeNav === 'config-filename-rules' ? (
            <ConfigPage onAria2EnabledChange={setAria2Enabled} page="filenameRules" />
          ) : activeNav === 'calendar' ? (
            <CalendarPage
              onSearch={(anime) => {
                const name = anime.name_cn || anime.name
                setInitialQuery(name)
                setActiveNav('scraper-search')
              }}
            />
          ) : activeNav === 'scraper-search' ? (
            <ScraperSearch 
              onToast={addToast} 
              aria2Enabled={aria2Enabled}
              initialSearchItem={initialSearchItem}
              onClearInitialSearchItem={() => setInitialSearchItem(null)}
              initialQuery={initialQuery}
              onClearInitialQuery={() => setInitialQuery(null)}
            />
          ) : activeNav === 'subscriptions' ? (
            <SubscriptionsPage onToast={addToast} aria2Enabled={aria2Enabled} u115Authorized={u115Authorized} />
          ) : activeNav === 'logs' ? (
            <LogsPage />
          ) : activeNav === 'u115-offline' ? (
            <U115OfflinePage onToast={addToast} />
          ) : downloadQueue ? (
            <DownloadsPage
              queue={downloadQueue}
              initialOverview={aria2Overview}
              aria2Enabled={aria2Enabled}
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
              onToast={addToast}
              onGlobalSearch={(item) => {
                clearResultsCache(item.title || item.original_title || item.name)
                setInitialSearchItem(item)
                setActiveNav('scraper-search')
              }}
            />
          )}
        </div>
      </main>

      {/* 移动端底部导航栏 */}
      <MobileNav
        active={activeNav}
        onSelect={setActiveNav}
        onToggleSidebar={() => setMobileSidebarOpen(o => !o)}
      />

      {showParseTest ? <ParseTestModal onClose={() => setShowParseTest(false)} /> : null}
      <ToastContainer toasts={toasts} onRemove={removeToast} />
    </div>
  )
}
