import { Suspense, lazy, useState, useEffect, useCallback } from 'react'
import { BrowserRouter, Routes, Route, Navigate, useNavigate, useLocation, useSearchParams } from 'react-router'
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
const StrmSyncPage = lazy(() => import('@/components/pages/StrmSyncPage'))
const StrmPlaybackPage = lazy(() => import('@/components/pages/StrmPlaybackPage'))
const TitlePinsPage = lazy(() => import('@/components/pages/TitlePinsPage'))
const ParseTestModal = lazy(() => import('@/components/modals/ParseTestModal'))

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

// 跨页跳转的交接数据（location.state 携带）
interface SearchHandoffState {
  initialItem?: MediaItem
}

export default function App() {
  const [token, setToken] = useState(() => localStorage.getItem('auth_token'))
  const [checkingAuth, setChecking] = useState(true)
  const [u115Authorized, setU115Authorized] = useState(false)

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
    <BrowserRouter>
      <AppShell u115Authorized={u115Authorized} onLogout={() => {
        localStorage.removeItem('auth_token')
        setToken(null)
      }} />
      <Toaster />
    </BrowserRouter>
  )
}

// AppShell 登录后的应用骨架（路由在此层消费）
function AppShell({ u115Authorized, onLogout }: { u115Authorized: boolean; onLogout: () => void }) {
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [refreshKey, setRefreshKey] = useState(0)
  const [showParseTest, setShowParseTest] = useState(false)

  const navigate = useNavigate()
  const location = useLocation()
  const [searchParams, setSearchParams] = useSearchParams()

  const handoff = (location.state ?? null) as SearchHandoffState | null

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

  const renderLibrary = (filter: string) => (
    <LibraryPage
      filter={filter}
      onChangeFilter={(f) => navigate(`/${f}`)}
      onRefresh={handleRefresh}
      refreshing={refreshing}
      refreshKey={refreshKey}
      onToast={handleToast}
      onGlobalSearch={(item) => navigate('/search', { state: { initialItem: item } })}
    />
  )

  return (
    <div className="relative h-screen overflow-hidden bg-background">
      {/* 环境光晕装饰层 */}
      <div className="ambient-layer" />

      <Topbar
        onToggleSidebar={() => setMobileSidebarOpen((o) => !o)}
        onOpenParseTest={() => setShowParseTest(true)}
      />
      <Sidebar
        u115Authorized={u115Authorized}
        mobileOpen={mobileSidebarOpen}
        onMobileClose={() => setMobileSidebarOpen(false)}
        onLogout={onLogout}
      />

      {mobileSidebarOpen && (
        <div
          className="fixed inset-0 z-[35] bg-black/50 backdrop-blur-sm lg:hidden"
          onClick={() => setMobileSidebarOpen(false)}
        />
      )}

      <main className="relative z-10 h-full pb-20 pt-14 lg:ml-[248px] lg:pb-0">
        <div
          key={location.pathname}
          data-main-scroll-container="true"
          className="h-full overflow-y-auto p-4 animate-page-enter lg:p-8"
        >
          <Suspense fallback={<PageFallback />}>
            <Routes>
              <Route path="/" element={<Navigate to="/library" replace />} />
              <Route path="/library" element={renderLibrary('all')} />
              <Route path="/movies" element={renderLibrary('movies')} />
              <Route path="/tv" element={renderLibrary('tv')} />
              <Route
                path="/calendar"
                element={<CalendarPage onSearch={(title) => navigate(`/search?q=${encodeURIComponent(title)}`)} />}
              />
              <Route
                path="/search"
                element={
                  <ScraperSearch
                    initialSearchItem={handoff?.initialItem ?? null}
                    onClearInitialSearchItem={() => navigate('/search', { replace: true, state: null })}
                    initialQuery={searchParams.get('q') ?? undefined}
                    onClearInitialQuery={() => setSearchParams({}, { replace: true })}
                  />
                }
              />
              <Route path="/subscriptions" element={<SubscriptionsPage onToast={handleToast} u115Authorized={u115Authorized} />} />
              <Route path="/files" element={<FileManagerPage />} />
              <Route path="/ingest-history" element={<IngestHistoryPage onToast={handleToast} />} />
              <Route path="/title-pins" element={<TitlePinsPage onToast={handleToast} />} />
              <Route path="/strm-sync" element={<StrmSyncPage onToast={handleToast} />} />
              <Route path="/strm-playback" element={<StrmPlaybackPage onToast={handleToast} />} />
              <Route path="/logs" element={<LogsPage />} />
              <Route path="/u115-offline" element={<U115OfflinePage onToast={handleToast} />} />
              <Route path="/config" element={<ConfigPage page="general" />} />
              <Route path="/config/filename-rules" element={<ConfigPage page="filenameRules" />} />
              <Route path="*" element={<Navigate to="/library" replace />} />
            </Routes>
          </Suspense>
        </div>
      </main>

      <MobileNav
        onToggleSidebar={() => setMobileSidebarOpen((o) => !o)}
        u115Authorized={u115Authorized}
      />

      <Suspense fallback={null}>
        <ParseTestModal
          open={showParseTest}
          onOpenChange={setShowParseTest}
        />
      </Suspense>
    </div>
  )
}
