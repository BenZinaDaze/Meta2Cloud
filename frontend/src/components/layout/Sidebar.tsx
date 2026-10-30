import { useState, useEffect } from 'react'
import { NavLink, useLocation } from 'react-router'
import {
  ArrowLeftRight,
  BookOpen,
  Search,
  Rss,
  Film,
  Tv,
  Cloud,
  Activity,
  History,
  Calendar,
  ChevronDown,
  Settings,
  LogOut,
  FolderOpen,
  FileVideo,
  LayoutDashboard,
  Play,
  Tag,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import Logo from '@/components/layout/Logo'

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

interface NavGroupLabelProps {
  label: string
}

function NavGroupLabel({ label }: NavGroupLabelProps) {
  return (
    <div className="px-4 pb-1.5 pt-5 text-[10px] font-semibold uppercase tracking-[0.14em] text-foreground/40">
      {label}
    </div>
  )
}

interface NavItemProps {
  icon: React.ReactNode
  label: string
  /** 路由目标；缺省时渲染为普通按钮（折叠开关等） */
  to?: string
  active?: boolean
  onClick?: () => void
  indent?: boolean
  right?: React.ReactNode
  meta?: number | null
}

function NavItem({ icon, label, to, active, onClick, indent, right, meta }: NavItemProps) {
  const className = cn(
    "group relative flex w-full items-center gap-2.5 overflow-hidden rounded-lg text-left font-medium transition-all duration-300 ease-[cubic-bezier(0.22,1,0.36,1)]",
    indent
      ? "mx-2 w-[calc(100%-1rem)] py-1.5 pl-9 pr-2.5"
      : "mx-2 w-[calc(100%-1rem)] px-2.5 py-2",
    active
      ? "bg-foreground text-[oklch(18%_0.01_140)] shadow-[0_4px_14px_-4px_oklch(0%_0_0/0.6)]"
      : "text-muted-foreground hover:bg-muted/70 hover:text-foreground"
  )
  const content = (
    <>
      <span className={cn("flex-shrink-0 transition-colors", active ? "opacity-80" : "text-muted-foreground/80 group-hover:text-foreground")}>
        {icon}
      </span>
      <span className={cn("flex-1 truncate", indent ? "text-[12.5px]" : "text-[13px]", active && "font-semibold")}>
        {label}
      </span>
      <span className="flex min-w-[24px] items-center justify-end gap-1.5">
        {meta !== null && meta !== undefined && (
          <span className={cn(
            "flex-shrink-0 rounded-full px-1.5 py-0.5 text-[10px] font-semibold tabular-nums transition-colors",
            active ? "bg-[oklch(18%_0.01_140)]/10 text-[oklch(18%_0.01_140)]/75" : "bg-muted text-muted-foreground"
          )}>
            {meta}
          </span>
        )}
        {right}
      </span>
    </>
  )
  if (to) {
    return (
      <NavLink to={to} onClick={onClick} className={className}>
        {content}
      </NavLink>
    )
  }
  return (
    <button onClick={onClick} className={className}>
      {content}
    </button>
  )
}

function Chevron({ expanded }: { expanded: boolean }) {
  return (
    <ChevronDown
      className={cn(
        "size-3.5 text-muted-foreground/70 transition-transform duration-300 ease-[cubic-bezier(0.22,1,0.36,1)]",
        expanded && "rotate-180"
      )}
    />
  )
}

function NavCollapse({ expanded, children }: { expanded: boolean; children: React.ReactNode }) {
  return (
    <div
      className={cn(
        "grid transition-[grid-template-rows,opacity] duration-300 ease-[cubic-bezier(0.22,1,0.36,1)]",
        expanded ? "grid-rows-[1fr] opacity-100" : "grid-rows-[0fr] opacity-0"
      )}
    >
      <div className="overflow-hidden">
        <div className="relative py-0.5">
          {/* 树状层级线：挂在一级图标中心正下方 */}
          <div className="absolute bottom-1 top-1 left-[25px] w-[2px] rounded-full bg-gradient-to-b from-foreground/15 via-foreground/[0.07] to-transparent" />
          {children}
        </div>
      </div>
    </div>
  )
}

interface SidebarProps {
  u115Authorized?: boolean
  mobileOpen?: boolean
  onMobileClose?: () => void
  onLogout?: () => void
}

export default function Sidebar({
  u115Authorized = false,
  mobileOpen = false,
  onMobileClose,
  onLogout,
}: SidebarProps) {
  const [libraryExpanded, setLibraryExpanded] = useState(true)
  const [configExpanded, setConfigExpanded] = useState(false)
  const [confirmingLogout, setConfirmingLogout] = useState(false)
  const [latestVersion, setLatestVersion] = useState<string | null>(null)

  const { pathname } = useLocation()

  const currentVersion = import.meta.env.VITE_APP_VERSION || 'v0.0.0'

  // 检测最新版本
  useEffect(() => {
    fetch('https://api.github.com/repos/BenZinaDaze/Panoply/tags')
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

  const isLibraryExpanded = libraryExpanded || pathname === '/movies' || pathname === '/tv'
  const isConfigExpanded = configExpanded || pathname.startsWith('/config')
  const closeMobile = () => onMobileClose?.()

  function handleLogoutClick() {
    if (!confirmingLogout) {
      setConfirmingLogout(true)
      return
    }
    setConfirmingLogout(false)
    onLogout?.()
  }

  return (
    <aside
      className={cn(
        "fixed left-0 top-0 bottom-0 z-40 flex w-[248px] flex-col border-r border-border/70 glass transition-transform duration-300 ease-[cubic-bezier(0.22,1,0.36,1)]",
        mobileOpen ? "translate-x-0 shadow-[24px_0_80px_-24px_oklch(5%_0.01_140/0.8)]" : "-translate-x-full lg:translate-x-0"
      )}
    >
      {/* Brand */}
      <div className="flex h-14 flex-shrink-0 items-center gap-2.5 border-b border-border/60 px-4">
        <Logo className="size-8 drop-shadow-[0_2px_8px_oklch(76%_0.19_150/0.45)]" />
        <div className="min-w-0">
          <div className="text-sm font-bold leading-tight tracking-tight text-foreground">
            Meta<span className="text-gradient-tide">2</span>Cloud
          </div>
          <div className="text-[9px] font-semibold uppercase tracking-[0.18em] text-foreground/45">
            Media Archive
          </div>
        </div>
      </div>

      {/* Navigation */}
      <nav className="min-h-0 flex-1 overflow-y-auto py-1">
        <NavGroupLabel label="媒体库" />
        <NavItem
          icon={<LayoutDashboard className="size-4" />}
          label="仪表盘"
          to="/dashboard"
          active={pathname === '/dashboard' || pathname === '/'}
          onClick={closeMobile}
        />
        <NavItem
          icon={<BookOpen className="size-4" />}
          label="媒体库总览"
          to="/library"
          active={pathname === '/library'}
          onClick={() => { setLibraryExpanded(!libraryExpanded); closeMobile() }}
          right={<Chevron expanded={isLibraryExpanded} />}
        />
        <NavCollapse expanded={isLibraryExpanded}>
          <NavItem
            icon={<Film className="size-4" />}
            label="电影"
            to="/movies"
            active={pathname === '/movies' || pathname.startsWith('/movies/')}
            onClick={closeMobile}
            indent
          />
          <NavItem
            icon={<Tv className="size-4" />}
            label="剧集"
            to="/tv"
            active={pathname === '/tv' || pathname.startsWith('/tv/')}
            onClick={closeMobile}
            indent
          />
        </NavCollapse>

        <div className="hidden lg:block">
          <NavItem
            icon={<Calendar className="size-4" />}
            label="新番列表"
            to="/calendar"
            active={pathname === '/calendar'}
            onClick={closeMobile}
          />
          <NavItem
            icon={<Search className="size-4" />}
            label="资源检索"
            to="/search"
            active={pathname === '/search'}
            onClick={closeMobile}
          />
        </div>

        <NavItem
          icon={<Rss className="size-4" />}
          label="订阅列表"
          to="/subscriptions"
          active={pathname === '/subscriptions'}
          onClick={closeMobile}
        />

        {u115Authorized && (
          <div className="hidden lg:block">
            <NavItem
              icon={<Cloud className="size-4" />}
              label="云下载"
              to="/u115-offline"
              active={pathname === '/u115-offline'}
              onClick={closeMobile}
            />
          </div>
        )}

        <NavGroupLabel label="系统" />
        {u115Authorized && (
          <NavItem
            icon={<FolderOpen className="size-4" />}
            label="文件管理"
            to="/files"
            active={pathname === '/files'}
            onClick={closeMobile}
          />
        )}
        <NavItem
          icon={<History className="size-4" />}
          label="入库记录"
          to="/ingest-history"
          active={pathname === '/ingest-history'}
          onClick={closeMobile}
        />
        <NavItem
          icon={<Tag className="size-4" />}
          label="整理名称"
          to="/title-pins"
          active={pathname === '/title-pins'}
          onClick={closeMobile}
        />
        <NavItem
          icon={<FileVideo className="size-4" />}
          label="STRM 同步"
          to="/strm-sync"
          active={pathname === '/strm-sync'}
          onClick={closeMobile}
        />
        <NavItem
          icon={<Play className="size-4" />}
          label="播放记录"
          to="/strm-playback"
          active={pathname === '/strm-playback'}
          onClick={closeMobile}
        />
        <NavItem
          icon={<ArrowLeftRight className="size-4" />}
          label="Trakt 同步"
          to="/watched-sync"
          active={pathname === '/watched-sync'}
          onClick={closeMobile}
        />
        <NavItem
          icon={<Activity className="size-4" />}
          label="日志"
          to="/logs"
          active={pathname === '/logs'}
          onClick={closeMobile}
        />
        <NavItem
          icon={<Settings className="size-4" />}
          label="配置"
          active={false}
          onClick={() => setConfigExpanded(!configExpanded)}
          right={<Chevron expanded={isConfigExpanded} />}
        />
        <NavCollapse expanded={isConfigExpanded}>
          <NavItem
            icon={<Settings className="size-4" />}
            label="基础配置"
            to="/config"
            active={pathname === '/config'}
            onClick={closeMobile}
            indent
          />
          <NavItem
            icon={<Search className="size-4" />}
            label="识别规则"
            to="/config/filename-rules"
            active={pathname === '/config/filename-rules'}
            onClick={closeMobile}
            indent
          />
          <NavItem
            icon={<Tag className="size-4" />}
            label="分类配置"
            to="/config/categories"
            active={pathname === '/config/categories'}
            onClick={closeMobile}
            indent
          />
        </NavCollapse>
      </nav>

      {/* Footer */}
      <div className="mt-auto flex-shrink-0 border-t border-border/60 pb-16 lg:pb-0">
        {onLogout && (
          <div className="p-2.5">
            <button
              onClick={handleLogoutClick}
              className={cn(
                "flex w-full items-center justify-start gap-2.5 rounded-lg px-3 py-2 text-sm font-medium transition-all duration-300",
                confirmingLogout
                  ? "bg-destructive/15 text-destructive shadow-[inset_0_0_0_1px_oklch(70%_0.19_22/0.3)] animate-tide-breathe"
                  : "text-muted-foreground hover:bg-muted/60 hover:text-foreground"
              )}
            >
              <LogOut className="size-4" />
              <span>{confirmingLogout ? '再次点击确认退出' : '退出登录'}</span>
            </button>
          </div>
        )}

        {/* Version Info */}
        <div className="px-3 pb-3 pt-1.5">
          <div className="flex items-center gap-2">
            <span className="inline-flex h-5 shrink-0 items-center rounded-full border border-border/80 bg-muted/70 px-2 font-mono text-[10px] font-semibold text-foreground/75">
              {currentVersion}
            </span>
            {hasUpdate && (
              <span className="animate-tide-breathe truncate text-[10px] font-medium text-primary">
                可更新到 {latestVersion}
              </span>
            )}
            <a
              href="https://github.com/BenZinaDaze/Panoply"
              target="_blank"
              rel="noopener noreferrer"
              className="ml-auto inline-flex size-6 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted/70 hover:text-foreground"
              title="GitHub"
            >
              <GithubIcon className="size-3.5" />
            </a>
          </div>
        </div>
      </div>
    </aside>
  )
}
