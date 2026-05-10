import { Suspense, lazy, useState, useEffect, useCallback } from 'react'
import { setUnauthorizedHandler, getMe, getAria2Overview, getConfig, getU115OauthStatus, refreshLibrary } from './api'
import type { Aria2Overview, MediaItem } from '@/types/api'
import { Toaster } from '@/components/ui/sonner'
import Sidebar from '@/components/layout/Sidebar'
import Topbar from '@/components/layout/Topbar'
import MobileNav from '@/components/layout/MobileNav'
import { toast } from 'sonner'

const LoginPage = lazy(() => import('@/components/pages/LoginPage'))
const LibraryPage = lazy(() => import('@/components/pages/LibraryPage'))
const ConfigPage = lazy(() => import('@/components/pages/ConfigPage'))
const FileManagerPage = lazy(() => import('@/components/pages/FileManagerPage'))
const DownloadsPage = lazy(() => import('@/components/pages/DownloadsPage'))
const CalendarPage = lazy(() => import('@/components/pages/CalendarPage'))
const SubscriptionsPage = lazy(() => import('@/components/pages/SubscriptionsPage'))
const LogsPage = lazy(() => import('@/components/pages/LogsPage'))
const IngestHistoryPage = lazy(() => import('@/components/pages/IngestHistoryPage'))
const U115OfflinePage = lazy(() => import('@/components/pages/U115OfflinePage'))
const ScraperSearch = lazy(() => import('@/components/pages/ScraperSearch'))
const ParseTestModal = lazy(() => import('@/components/modals/ParseTestModal'))

type NavKey = 'all' | 'movies' | 'tv' | 'config' | 'config-filename-rules' | 'calendar' | 'scraper-search' | 'subscriptions' | 'files' | 'ingest-history' | 'logs' | 'u115-offline' | 'downloads' | 'downloads-active' | 'downloads-waiting' | 'downloads-stopped'

const DOWNLOAD_QUEUE_MAP: Record<string, string | undefined> = {
  downloads: 'all',
  'downloads-active': 'active',
  'downloads-waiting': 'waiting',
  'downloads-stopped': 'stopped',
}

function PageFallback() {
  return (
    <div className="flex min-h-[240px] items-center justify-center">
      <div className="text-sm text-muted-foreground">页面加载中...</div>
    </div>
  )
}

export default function App() {
  const [token, setToken] = useState(() => localStorage.getItem('auth_token'))
  const [checkingAuth, setChecking] = useState(true)

  const [activeNav, setActiveNav] = useState<NavKey>('all')
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false)
  const [aria2Overview, setAria2Overview] = useState<Aria2Overview | null>(null)
  const [aria2Enabled, setAria2Enabled] = useState<boolean | null>(null)
  const [u115Authorized, setU115Authorized] = useState(false)
  const [aria2ConnectionStatus, setAria2ConnectionStatus] = useState<'connecting' | 'connected' | 'error' | 'disabled'>('connecting')
  const [refreshing, setRefreshing] = useState(false)
  const [refreshKey, setRefreshKey] = useState(0)
  const [globalSearchItem, setGlobalSearchItem] = useState<MediaItem | null>(null)
  const [globalSearchQuery, setGlobalSearchQuery] = useState('')
  const [showParseTest, setShowParseTest] = useState(false)

  useEffect(() => {
    if (!token) { setChecking(false); return }
    let cancelled = false
    getMe().then(() => { if (!cancelled) setChecking(false) }).catch(() => {
      if (!cancelled) {
        localStorage.removeItem('auth_token')
        setToken(null)
        setChecking(false)
      }
    })
    return () => { cancelled = true }
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
        if (!cancelled) setAria2Enabled(false)
      })

    getU115OauthStatus()
      .then((res) => {
        if (!cancelled) setU115Authorized(!!res?.data?.authorized)
      })
      .catch(() => {
        if (!cancelled) setU115Authorized(false)
      })

    return () => { cancelled = true }
  }, [token])

  useEffect(() => {
    setUnauthorizedHandler(() => {
      localStorage.removeItem('auth_token')
      setToken(null)
    })
  }, [])

  useEffect(() => {
    if (!token || aria2Enabled === null) {
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
    let timer: ReturnType<typeof setTimeout>

    async function poll() {
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
      } finally {
        if (!cancelled) {
          timer = setTimeout(poll, 5000)
        }
      }
    }

    poll()
    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [token, aria2Enabled])

  const handleLogin = useCallback((newToken: string) => {
    localStorage.setItem('auth_token', newToken)
    setToken(newToken)
  }, [])

  const handleLogout = useCallback(() => {
    localStorage.removeItem('auth_token')
    setToken(null)
  }, [])

  const handleToast = useCallback((type: 'success' | 'error' | 'warning', title: string, message?: string) => {
    toast[type](title, { description: message })
  }, [])

  const handleRefresh = useCallback(async () => {
    setRefreshing(true)
    try {
      await refreshLibrary()
      setRefreshKey((k) => k + 1)
    } finally {
      setRefreshing(false)
    }
  }, [])

  if (checkingAuth) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="text-muted-foreground">正在验证身份...</div>
      </div>
    )
  }

  if (!token) {
    return (
      <>
        <Suspense fallback={<PageFallback />}>
          <LoginPage onLogin={handleLogin} />
        </Suspense>
        <Toaster />
      </>
    )
  }

  const downloadQueue = DOWNLOAD_QUEUE_MAP[activeNav]

  return (
    <div className="h-screen overflow-hidden bg-background">
      <Topbar
        onToggleSidebar={() => setMobileSidebarOpen((o) => !o)}
        onOpenParseTest={() => setShowParseTest(true)}
      />
      <Sidebar
        active={activeNav}
        onSelect={(key) => { setActiveNav(key as NavKey); setMobileSidebarOpen(false) }}
        aria2Overview={aria2Overview}
        aria2Enabled={aria2Enabled}
        u115Authorized={u115Authorized}
        mobileOpen={mobileSidebarOpen}
        onMobileClose={() => setMobileSidebarOpen(false)}
        onLogout={handleLogout}
        aria2ConnectionStatus={aria2ConnectionStatus}
      />

      {mobileSidebarOpen && (
        <div
          className="fixed inset-0 z-[35] bg-black/50 backdrop-blur-sm lg:hidden"
          onClick={() => setMobileSidebarOpen(false)}
        />
      )}

      <main className="h-full pt-14 pb-20 lg:pb-0 lg:ml-[260px]">
        <div data-main-scroll-container="true" className="h-full overflow-y-auto p-4 lg:p-8">
          <Suspense fallback={<PageFallback />}>
            {activeNav === 'config' ? (
              <ConfigPage onAria2EnabledChange={setAria2Enabled} page="general" />
            ) : activeNav === 'config-filename-rules' ? (
              <ConfigPage onAria2EnabledChange={setAria2Enabled} page="filenameRules" />
            ) : activeNav === 'calendar' ? (
              <CalendarPage
                onSearch={(title) => {
                  setGlobalSearchQuery(title)
                  setActiveNav('scraper-search')
                }}
              />
            ) : activeNav === 'scraper-search' ? (
              <ScraperSearch
                initialSearchItem={globalSearchItem}
                onClearInitialSearchItem={() => setGlobalSearchItem(null)}
                initialQuery={globalSearchQuery}
                onClearInitialQuery={() => setGlobalSearchQuery('')}
                aria2Enabled={aria2Enabled ?? false}
              />
            ) : activeNav === 'subscriptions' ? (
              <SubscriptionsPage
                onToast={handleToast}
                aria2Enabled={aria2Enabled ?? false}
                u115Authorized={u115Authorized}
              />
            ) : activeNav === 'files' ? (
              <FileManagerPage />
            ) : activeNav === 'ingest-history' ? (
              <IngestHistoryPage onToast={handleToast} />
            ) : activeNav === 'logs' ? (
              <LogsPage />
            ) : activeNav === 'u115-offline' ? (
              <U115OfflinePage onToast={handleToast} />
            ) : downloadQueue ? (
              <DownloadsPage
                queue={downloadQueue}
                onChangeQueue={(q) => setActiveNav(q === 'all' ? 'downloads' : `downloads-${q}` as NavKey)}
                onToast={handleToast}
                aria2Enabled={aria2Enabled}
              />
            ) : (
              <LibraryPage
                filter={activeNav}
                onChangeFilter={(f) => setActiveNav(f as NavKey)}
                onRefresh={handleRefresh}
                refreshing={refreshing}
                refreshKey={refreshKey}
                onToast={handleToast}
                onGlobalSearch={(item) => {
                  setGlobalSearchItem(item)
                  setActiveNav('scraper-search')
                }}
              />
            )}
          </Suspense>
        </div>
      </main>

      <MobileNav
        active={activeNav}
        onSelect={(key) => setActiveNav(key as NavKey)}
        onToggleSidebar={() => setMobileSidebarOpen((o) => !o)}
        u115Authorized={u115Authorized}
      />

      <Suspense fallback={null}>
        <ParseTestModal
          open={showParseTest}
          onOpenChange={setShowParseTest}
        />
      </Suspense>

      <Toaster />
    </div>
  )
}
