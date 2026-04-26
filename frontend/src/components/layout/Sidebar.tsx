import { useState, useEffect } from 'react'
import {
  BookOpen,
  Search,
  Rss,
  Film,
  Tv,
  Download,
  Cloud,
  Zap,
  List,
  Archive,
  Activity,
  History,
  Calendar,
  ChevronDown,
  Settings,
  LogOut,
  Circle,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { Aria2Overview } from '@/types/api'

// GitHub SVG 图标
function GithubIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="currentColor">
      <path fillRule="evenodd" clipRule="evenodd" d="M12 2C6.477 2 2 6.477 2 12c0 4.42 2.87 8.17 6.84 9.5.5.08.66-.23.66-.5v-1.69c-2.77.6-3.36-1.34-3.36-1.34-.45-1.15-1.11-1.46-1.11-1.46-.91-.62.07-.6.07-.6 1 .07 1.53 1.03 1.53 1.03.87 1.52 2.34 1.07 2.91.83.09-.65.35-1.09.63-1.34-2.22-.25-4.55-1.11-4.55-4.92 0-1.11.38-2 1.03-2.71-.1-.25-.45-1.29.1-2.64 0 0 .84-.27 2.75 1.02.79-.22 1.65-.33 2.5-.33.85 0 1.71.11 2.5.33 1.91-1.29 2.75-1.02 2.75-1.02.55 1.35.2 2.39.1 2.64.65.71 1.03 1.6 1.03 2.71 0 3.82-2.34 4.66-4.57 4.91.36.31.69.92.69 1.85V21c0 .27.16.59.67.5C19.14 20.16 22 16.42 22 12A10 10 0 0 0 12 2z" />
    </svg>
  )
}

// 版本比较函数：支持 v4.04 vs v4.1，以及 v4.1.1 vs v4.1.2
function compareVer(v1: string, v2: string): number {
  const parse = (v: string) => {
    const parts = (v || '').replace(/[^\d.]/g, '').split('.')
    const major = Number.parseInt(parts[0] || '0', 10) || 0
    const minor = Number.parseInt(parts[1] || '0', 10) || 0
    const patches = parts.slice(2).map(Number)
    return { major, minor, patches }
  }

  const p1 = parse(v1)
  const p2 = parse(v2)

  if (p1.major !== p2.major) return p1.major - p2.major
  if (p1.minor !== p2.minor) return p1.minor - p2.minor

  const len = Math.max(p1.patches.length, p2.patches.length)
  for (let i = 0; i < len; i++) {
    const n1 = p1.patches[i] || 0
    const n2 = p2.patches[i] || 0
    if (n1 !== n2) return n1 - n2
  }
  return 0
}

interface NavItemProps {
  icon: React.ReactNode
  label: string
  active?: boolean
  onClick?: () => void
  indent?: boolean
  right?: React.ReactNode
  bold?: boolean
  meta?: number | null
}

function NavItem({ icon, label, active, onClick, indent, right, bold, meta }: NavItemProps) {
  return (
    <button
      onClick={onClick}
      className={cn(
        "group relative flex w-full items-center gap-3 overflow-hidden text-left font-medium transition-colors",
        indent ? "px-4 py-2 pl-10" : "px-4 py-2.5",
        active
          ? "bg-accent text-accent-foreground"
          : "text-muted-foreground hover:bg-muted/50 hover:text-foreground"
      )}
    >
      <span className={cn(
        "flex-shrink-0",
        active ? "text-accent-foreground/70" : "text-muted-foreground group-hover:text-foreground"
      )}>
        {icon}
      </span>
      <span
        className={cn("flex-1 text-sm", indent && "text-[13px]")}
        style={{ fontWeight: bold || active ? 600 : 500 }}
      >
        {label}
      </span>
      <span className="flex min-w-[40px] items-center justify-end gap-2">
        {meta !== null && meta !== undefined && (
          <span className={cn(
            "flex-shrink-0 rounded px-1.5 py-0.5 text-[10px] font-semibold tabular-nums",
            active ? "bg-primary/10 text-primary" : "bg-muted text-muted-foreground"
          )}>
            {meta}
          </span>
        )}
        {right && <span className="flex items-center justify-center">{right}</span>}
      </span>
    </button>
  )
}

interface SidebarProps {
  active: string
  onSelect: (key: string) => void
  aria2Overview?: Aria2Overview | null
  aria2ConnectionStatus?: 'connecting' | 'connected' | 'error' | 'disabled'
  aria2Enabled?: boolean | null
  u115Authorized?: boolean
  mobileOpen?: boolean
  onMobileClose?: () => void
  onLogout?: () => void
}

export default function Sidebar({
  active,
  onSelect,
  aria2Overview = null,
  aria2ConnectionStatus = 'connecting',
  aria2Enabled = null,
  u115Authorized = false,
  mobileOpen = false,
  onLogout,
}: SidebarProps) {
  const [libraryExpanded, setLibraryExpanded] = useState(true)
  const [downloadsExpanded, setDownloadsExpanded] = useState(false)
  const [configExpanded, setConfigExpanded] = useState(false)
  const [confirmingLogout, setConfirmingLogout] = useState(false)
  const [latestVersion, setLatestVersion] = useState<string | null>(null)

  const currentVersion = import.meta.env.VITE_APP_VERSION || 'v0.0.0'

  // 检测最新版本
  useEffect(() => {
    fetch('https://api.github.com/repos/BenZinaDaze/Meta2Cloud/tags')
      .then(res => res.json())
      .then((tags: Array<{ name: string }>) => {
        if (tags && tags.length > 0) {
          const sortedTags = tags.sort((a, b) => compareVer(b.name, a.name))
          setLatestVersion(sortedTags[0].name)
        }
      })
      .catch(() => { })
  }, [])

  useEffect(() => {
    if (!confirmingLogout) return
    const timer = window.setTimeout(() => setConfirmingLogout(false), 3000)
    return () => window.clearTimeout(timer)
  }, [confirmingLogout])

  const hasUpdate = latestVersion && currentVersion !== 'dev' && compareVer(latestVersion, currentVersion) > 0

  const isLibraryExpanded = libraryExpanded || active === 'movies' || active === 'tv'
  const isDownloadsExpanded = downloadsExpanded || ['downloads-active', 'downloads-waiting', 'downloads-stopped'].includes(active)

  const activeCount = aria2Overview?.summary?.activeCount ?? 0
  const waitingCount = aria2Overview?.summary?.waitingCount ?? 0
  const stoppedCount = aria2Overview?.summary?.stoppedCount ?? 0
  const totalDownloadCount = activeCount + waitingCount + stoppedCount
  const aria2Connected = !!aria2Overview
  const showDownloads = aria2Enabled !== false

  const connectionState = {
    connected: { label: '已连接', color: 'text-success' },
    disabled: { label: '已禁用', color: 'text-muted-foreground' },
    connecting: { label: '连接中', color: 'text-warning' },
    error: { label: '未连接', color: 'text-destructive' },
  }[aria2ConnectionStatus] || { label: '未连接', color: 'text-destructive' }

  function handleLogoutClick() {
    if (!confirmingLogout) {
      setConfirmingLogout(true)
      return
    }
    setConfirmingLogout(false)
    onLogout?.()
  }

  const chevronIcon = (expanded: boolean) => (
    <ChevronDown
      className={cn(
        "size-4 text-muted-foreground transition-transform duration-200",
        expanded && "rotate-180"
      )}
    />
  )

  return (
    <aside
      className={cn(
        "fixed left-0 top-14 bottom-0 flex w-[260px] flex-col border-r bg-background z-40",
        "transition-transform duration-300 ease-out",
        mobileOpen ? "translate-x-0" : "-translate-x-full lg:translate-x-0"
      )}
    >
      {/* Navigation */}
      <div className="min-h-0 flex-1 overflow-y-auto py-2">
        <NavItem
          icon={<BookOpen className="size-4" />}
          label="媒体库"
          active={active === 'all'}
          onClick={() => { setLibraryExpanded(!libraryExpanded); onSelect('all') }}
          bold
          right={chevronIcon(isLibraryExpanded)}
        />

        <div
          className={cn(
            "overflow-hidden transition-all duration-200",
            isLibraryExpanded ? "max-h-[100px] opacity-100" : "max-h-0 opacity-0"
          )}
        >
          <div className="relative">
            <div className="absolute bottom-0 top-0 left-5 w-px bg-border" />
            <NavItem
              icon={<Film className="size-4" />}
              label="电影"
              active={active === 'movies'}
              onClick={() => onSelect('movies')}
              indent
            />
            <NavItem
              icon={<Tv className="size-4" />}
              label="电视剧"
              active={active === 'tv'}
              onClick={() => onSelect('tv')}
              indent
            />
          </div>
        </div>

        <div className="hidden lg:block">
          <NavItem
            icon={<Calendar className="size-4" />}
            label="新番列表"
            active={active === 'calendar'}
            onClick={() => onSelect('calendar')}
          />
        </div>

        <div className="hidden lg:block">
          <NavItem
            icon={<Search className="size-4" />}
            label="资源检索"
            active={active === 'scraper-search'}
            onClick={() => onSelect('scraper-search')}
          />
        </div>

        <NavItem
          icon={<Rss className="size-4" />}
          label="订阅列表"
          active={active === 'subscriptions'}
          onClick={() => onSelect('subscriptions')}
        />

        {showDownloads && (
          <>
            <NavItem
              icon={<Download className="size-4" />}
              label="下载管理"
              active={active === 'downloads'}
              onClick={() => { setDownloadsExpanded(!downloadsExpanded); onSelect('downloads') }}
              meta={aria2Connected ? totalDownloadCount : null}
              right={
                <span className="flex items-center gap-1.5">
                  <Circle className={cn("size-2 fill-current", connectionState.color)} />
                  {chevronIcon(isDownloadsExpanded)}
                </span>
              }
            />

            <div
              className={cn(
                "overflow-hidden transition-all duration-200",
                isDownloadsExpanded ? "max-h-[150px] opacity-100" : "max-h-0 opacity-0"
              )}
            >
              <div className="relative">
                <div className="absolute bottom-0 top-0 left-5 w-px bg-border" />
                <NavItem
                  icon={<Zap className="size-4" />}
                  label="下载中"
                  active={active === 'downloads-active'}
                  onClick={() => onSelect('downloads-active')}
                  indent
                  meta={activeCount}
                />
                <NavItem
                  icon={<List className="size-4" />}
                  label="等待中"
                  active={active === 'downloads-waiting'}
                  onClick={() => onSelect('downloads-waiting')}
                  indent
                  meta={waitingCount}
                />
                <NavItem
                  icon={<Archive className="size-4" />}
                  label="已停止"
                  active={active === 'downloads-stopped'}
                  onClick={() => onSelect('downloads-stopped')}
                  indent
                  meta={stoppedCount}
                />
              </div>
            </div>
          </>
        )}

        {u115Authorized && (
          <div className="hidden lg:block">
            <NavItem
              icon={<Cloud className="size-4" />}
              label="云下载"
              active={active === 'u115-offline'}
              onClick={() => onSelect('u115-offline')}
            />
          </div>
        )}

        <NavItem
          icon={<History className="size-4" />}
          label="入库记录"
          active={active === 'ingest-history'}
          onClick={() => onSelect('ingest-history')}
        />

        <NavItem
          icon={<Activity className="size-4" />}
          label="日志"
          active={active === 'logs'}
          onClick={() => onSelect('logs')}
        />

        <NavItem
          icon={<Settings className="size-4" />}
          label="配置"
          active={false}
          onClick={() => setConfigExpanded(!configExpanded)}
          right={chevronIcon(configExpanded)}
        />

        <div
          className={cn(
            "overflow-hidden transition-all duration-200",
            configExpanded ? "max-h-[100px] opacity-100" : "max-h-0 opacity-0"
          )}
        >
          <div className="relative">
            <div className="absolute bottom-0 top-0 left-5 w-px bg-border" />
            <NavItem
              icon={<Settings className="size-4" />}
              label="基础配置"
              active={active === 'config'}
              onClick={() => onSelect('config')}
              indent
            />
            <NavItem
              icon={<Search className="size-4" />}
              label="识别规则"
              active={active === 'config-filename-rules'}
              onClick={() => onSelect('config-filename-rules')}
              indent
            />
          </div>
        </div>
      </div>

      {/* Footer */}
      <div className="mt-auto border-t">
        {onLogout && (
          <div className="p-3">
            <Button
              variant={confirmingLogout ? "destructive" : "ghost"}
              className="w-full justify-start gap-2 text-muted-foreground hover:text-foreground"
              onClick={handleLogoutClick}
            >
              <LogOut className="size-4" />
              <span className="text-sm">{confirmingLogout ? '再次点击确认退出' : '退出登录'}</span>
            </Button>
          </div>
        )}

        {/* Version Info */}
        <div className="px-4 pb-3 pt-2">
          <div className="flex items-center gap-2 text-[11px] text-muted-foreground/60">
            <span className="font-medium">{currentVersion}</span>
            {hasUpdate && (
              <>
                <span className="text-muted-foreground/40">·</span>
                <span className="text-primary/80">
                  可更新到 {latestVersion}
                </span>
              </>
            )}
            <a
              href="https://github.com/BenZinaDaze/Meta2Cloud"
              target="_blank"
              rel="noopener noreferrer"
              className="ml-auto inline-flex size-5 items-center justify-center rounded hover:bg-muted/50 transition-colors"
              title="GitHub"
            >
              <GithubIcon className="size-3 opacity-50 hover:opacity-80" />
            </a>
          </div>
        </div>
      </div>
    </aside>
  )
}
