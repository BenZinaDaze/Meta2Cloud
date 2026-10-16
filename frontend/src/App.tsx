import { Suspense, lazy, useState, useEffect, useCallback } from 'react'
import { setUnauthorizedHandler, getMe, getU115OauthStatus, refreshLibrary } from './api'
import type { MediaItem } from '@/types/api'
import { Toaster } from '@/components/ui/sonner'
import Sidebar from '@/components/layout/Sidebar'
import Topbar from '@/components/layout/Topbar'
import MobileNav from '@/components/layout/MobileNav'
import { toast } from 'sonner'

const LoginPage = lazy(() => import('@/components/pages/LoginPage'))
const LibraryPage = lazy(() => import('@/components/pages/LibraryPage'))
const ConfigPage = lazy(() => import('@/components/pages/ConfigPage'))
const FileManagerPage = lazy(() => import('@/components/pages/FileManagerPage'))
const CalendarPage = lazy(() => import('@/components/pages/CalendarPage'))
const SubscriptionsPage = lazy(() => import('@/components/pages/SubscriptionsPage'))
const LogsPage = lazy(() => import('@/components/pages/LogsPage'))
const IngestHistoryPage = lazy(() => import('@/components/pages/IngestHistoryPage'))
const U115OfflinePage = lazy(() => import('@/components/pages/U115OfflinePage'))
const ScraperSearch = lazy(() => import('@/components/pages/ScraperSearch'))
const ParseTestModal = lazy(() => import('@/components/modals/ParseTestModal'))

type NavKey = 'all' | 'movies' | 'tv' | 'config' | 'config-filename-rules' | 'calendar' | 'scraper-search' | 'subscriptions' | 'files' | 'ingest-history' | 'logs' | 'u115-offline'

function PageFallback() {
  return (
    <div className="flex min-h-[240px] items-center justify-center">
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <span className="size-1.5 animate-tide-breathe rounded-full bg-primary" />
        页面加载中...
      </div>
    </div>
  )
}

export default function App() {
  const [token, setToken] = useState(() => localStorage.getItem('auth_token'))
  const [checkingAuth, setChecking] = useState(true)

  const [activeNav, setActiveNav] = useState<NavKey>('all')
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false)
  const [u115Authorized, setU115Authorized] = useState(false)
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

  useEffect(() => {
    setUnauthorizedHandler(() => {
      localStorage.removeItem('auth_token')
      setToken(null)
    })
  }, [])

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
      <div className="relative flex min-h-screen items-center justify-center">
        <div className="pointer-events-none absolute inset-0 bg-ambient" />
        <div className="relative flex items-center gap-2.5 text-muted-foreground">
          <span className="size-1.5 animate-tide-breathe rounded-full bg-primary" />
          正在验证身份...
        </div>
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

  return (
    <div className="relative h-screen overflow-hidden bg-background">
      {/* 环境光晕装饰层 */}
      <div className="ambient-layer" />

      <Topbar
        onToggleSidebar={() => setMobileSidebarOpen((o) => !o)}
        onOpenParseTest={() => setShowParseTest(true)}
      />
      <Sidebar
        active={activeNav}
        onSelect={(key) => { setActiveNav(key as NavKey); setMobileSidebarOpen(false) }}
        u115Authorized={u115Authorized}
        mobileOpen={mobileSidebarOpen}
        onMobileClose={() => setMobileSidebarOpen(false)}
        onLogout={handleLogout}
      />

      {mobileSidebarOpen && (
        <div
          className="fixed inset-0 z-[35] bg-black/50 backdrop-blur-sm lg:hidden"
          onClick={() => setMobileSidebarOpen(false)}
        />
      )}

      <main className="relative z-10 h-full pb-20 pt-14 lg:ml-[248px] lg:pb-0">
        <div
          key={activeNav}
          data-main-scroll-container="true"
          className="h-full overflow-y-auto p-4 animate-page-enter lg:p-8"
        >
          <Suspense fallback={<PageFallback />}>
            {activeNav === 'config' ? (
              <ConfigPage page="general" />
            ) : activeNav === 'config-filename-rules' ? (
              <ConfigPage page="filenameRules" />
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
              />
            ) : activeNav === 'subscriptions' ? (
              <SubscriptionsPage
                onToast={handleToast}
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
