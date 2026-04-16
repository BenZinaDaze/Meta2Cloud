import { useState, useEffect } from 'react'

// SVG 图标（MD 风格线条图标）
const Icons = {
  library: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z" />
      <path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z" />
    </svg>
  ),
  search: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="11" cy="11" r="8" />
      <line x1="21" y1="21" x2="16.65" y2="16.65" />
    </svg>
  ),
  rss: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <path d="M4 11a9 9 0 0 1 9 9" />
      <path d="M4 4a16 16 0 0 1 16 16" />
      <circle cx="5" cy="19" r="1.5" fill="currentColor" stroke="none" />
    </svg>
  ),
  movie: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <rect x="2" y="2" width="20" height="20" rx="2.18" ry="2.18" />
      <line x1="7" y1="2" x2="7" y2="22" /><line x1="17" y1="2" x2="17" y2="22" />
      <line x1="2" y1="12" x2="22" y2="12" /><line x1="2" y1="7" x2="7" y2="7" />
      <line x1="2" y1="17" x2="7" y2="17" /><line x1="17" y1="17" x2="22" y2="17" />
      <line x1="17" y1="7" x2="22" y2="7" />
    </svg>
  ),
  tv: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <rect x="2" y="7" width="20" height="15" rx="2" ry="2" />
      <polyline points="17 2 12 7 7 2" />
    </svg>
  ),
  download: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
      <polyline points="7 10 12 15 17 10" />
      <line x1="12" y1="15" x2="12" y2="3" />
    </svg>
  ),
  cloudDownload: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <path d="M20 17.58A5 5 0 0 0 18 8h-1.26A8 8 0 1 0 4 16.25" />
      <path d="M12 12v9" />
      <path d="m8 17 4 4 4-4" />
    </svg>
  ),
  bolt: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2" />
    </svg>
  ),
  queue: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <line x1="8" y1="6" x2="21" y2="6" />
      <line x1="8" y1="12" x2="21" y2="12" />
      <line x1="8" y1="18" x2="21" y2="18" />
      <line x1="3" y1="6" x2="3.01" y2="6" />
      <line x1="3" y1="12" x2="3.01" y2="12" />
      <line x1="3" y1="18" x2="3.01" y2="18" />
    </svg>
  ),
  archive: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="21 8 21 21 3 21 3 8" />
      <rect x="1" y="3" width="22" height="5" />
      <line x1="10" y1="12" x2="14" y2="12" />
    </svg>
  ),
  activity: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" />
    </svg>
  ),
  calendar: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="4" width="18" height="18" rx="2" ry="2" />
      <line x1="16" y1="2" x2="16" y2="6" />
      <line x1="8" y1="2" x2="8" y2="6" />
      <line x1="3" y1="10" x2="21" y2="10" />
      <line x1="8" y1="14" x2="8.01" y2="14" />
      <line x1="12" y1="14" x2="12.01" y2="14" />
      <line x1="16" y1="14" x2="16.01" y2="14" />
      <line x1="8" y1="18" x2="8.01" y2="18" />
      <line x1="12" y1="18" x2="12.01" y2="18" />
    </svg>
  ),
  chevron: (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="6 9 12 15 18 9" />
    </svg>
  ),
  settings: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
    </svg>
  ),
  logout: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
      <polyline points="16 17 21 12 16 7" />
      <line x1="21" y1="12" x2="9" y2="12" />
    </svg>
  ),
}

/** 统一导航项——所有顶层和子项共用同一套 margin/padding，保证图标像素级对齐 */
function NavItem({ icon, label, active, onClick, indent = false, right, bold = false, meta = null }) {
  return (
    <button
      onClick={onClick}
      className="group relative flex items-center gap-3 overflow-hidden text-left font-medium transition-all duration-150"
      style={{
        padding: indent ? '12px 18px 12px 54px' : '13px 18px',
        margin: '4px 12px',
        width: 'calc(100% - 24px)', // Fix: 100% minus 12px*2 margin
        borderRadius: 20,
        background: active ? 'linear-gradient(90deg, rgba(200, 146, 77, 0.16) 0%, rgba(200, 146, 77, 0.04) 100%)' : 'transparent',
        border: active ? '1px solid rgba(200, 146, 77, 0.22)' : '1px solid transparent',
        color: active ? 'var(--color-accent-hover)' : (bold ? 'var(--color-text)' : 'var(--color-muted)'),
      }}
    >
      <span
        className="absolute inset-0 rounded-[20px] opacity-0 transition-opacity duration-150 group-hover:opacity-100"
        style={{ background: active ? 'rgba(200, 146, 77, 0.06)' : 'rgba(255,255,255,0.035)' }}
      />
      <span className="relative flex-shrink-0" style={{ color: active ? 'var(--color-accent)' : 'inherit' }}>
        {icon}
      </span>
      <span className="relative flex-1" style={{ fontSize: indent ? 15 : 16, fontWeight: bold ? 600 : 500 }}>
        {label}
      </span>
      <span className="relative flex min-w-[44px] items-center justify-end gap-2">
        {meta !== null && (
          <span
            className="flex-shrink-0 rounded-full px-2 py-0.5 text-[11px] font-semibold"
            style={{
              background: active ? 'rgba(200, 146, 77, 0.18)' : 'rgba(255,255,255,0.05)',
              color: active ? 'var(--color-accent-hover)' : 'var(--color-muted)',
            }}
          >
            {meta}
          </span>
        )}
        {right && <span className="flex items-center justify-center">{right}</span>}
      </span>
    </button>
  )
}

export default function Sidebar({ active, onSelect, aria2Overview = null, aria2ConnectionStatus = 'connecting', mobileOpen = false, onMobileClose, onLogout }) {
  const [libraryExpanded, setLibraryExpanded] = useState(true)
  const [downloadsExpanded, setDownloadsExpanded] = useState(false)
  const [configExpanded, setConfigExpanded] = useState(false)
  const [latestVersion, setLatestVersion] = useState(null)
  const [confirmingLogout, setConfirmingLogout] = useState(false)

  const currentVersion = import.meta.env.VITE_APP_VERSION || 'v4.03'

  // Compare format conceptually: supports v4.04 vs v4.1, AND v4.1.1 vs v4.1.2
  // We treat the "Major.Minor" as a float, and any subsequent ".Patch" as integers
  const compareVer = (v1, v2) => {
    const parse = v => {
      const parts = (v || '').replace(/[^\d.]/g, '').split('.');
      const major = parts[0] || '0';
      const minor = parts.length > 1 ? parts[1] : '';
      const floatPart = parseFloat(`${major}${minor ? '.' + minor : ''}`);
      const patches = parts.slice(2).map(Number);
      return { floatPart, patches };
    };

    const p1 = parse(v1);
    const p2 = parse(v2);

    if (p1.floatPart !== p2.floatPart) {
      return p1.floatPart - p2.floatPart;
    }

    const len = Math.max(p1.patches.length, p2.patches.length);
    for (let i = 0; i < len; i++) {
      const n1 = p1.patches[i] || 0;
      const n2 = p2.patches[i] || 0;
      if (n1 !== n2) return n1 - n2;
    }
    return 0;
  };

  useEffect(() => {
    fetch('https://api.github.com/repos/BenZinaDaze/Meta2Cloud/tags')
      .then(res => res.json())
      .then(tags => {
        if (tags && tags.length > 0) {
          // Sort tags by version descending, so the highest version is first
          const sortedTags = tags.sort((a, b) => compareVer(b.name, a.name))
          setLatestVersion(sortedTags[0].name)
        }
      })
      .catch(() => { })
  }, [])

  useEffect(() => {
    if (!confirmingLogout) return undefined
    const timer = window.setTimeout(() => setConfirmingLogout(false), 3000)
    return () => window.clearTimeout(timer)
  }, [confirmingLogout])

  const hasUpdate = latestVersion && currentVersion !== 'dev' && compareVer(latestVersion, currentVersion) > 0

  const isLibraryExpanded = libraryExpanded || active === 'movies' || active === 'tv'
  const isDownloadsExpanded = downloadsExpanded || ['downloads-active', 'downloads-waiting', 'downloads-stopped'].includes(active)
  const isConfigExpanded = configExpanded
  const activeCount = aria2Overview?.summary?.activeCount ?? 0
  const waitingCount = aria2Overview?.summary?.waitingCount ?? 0
  const stoppedCount = aria2Overview?.summary?.stoppedCount ?? 0
  const totalDownloadCount = activeCount + waitingCount + stoppedCount
  const aria2Connected = !!aria2Overview
  const connectionState = {
    connected: {
      label: '已连接',
      color: 'var(--color-success)',
    },
    disabled: {
      label: '已禁用',
      color: 'var(--color-muted)',
    },
    connecting: {
      label: '连接中',
      color: '#f5c451',
    },
    error: {
      label: '未连接',
      color: 'var(--color-danger)',
    },
  }[aria2ConnectionStatus] || {
    label: '未连接',
    color: 'var(--color-danger)',
  }

  function handleLibraryClick() {
    setLibraryExpanded(prev => !prev)
    onSelect('all')
  }

  function handleDownloadsClick() {
    setDownloadsExpanded(prev => !prev)
    onSelect('downloads')
  }

  function handleConfigClick() {
    setConfigExpanded(prev => !prev)
  }

  function handleLogoutClick() {
    if (!confirmingLogout) {
      setConfirmingLogout(true)
      return
    }
    setConfirmingLogout(false)
    onLogout?.()
  }

  const libraryChevron = (
    <span
      className="transition-transform duration-200"
      style={{
        color: 'var(--color-muted)',
        display: 'flex',
        transform: isLibraryExpanded ? 'rotate(180deg)' : 'rotate(0deg)',
      }}
    >
      {Icons.chevron}
    </span>
  )

  const downloadsChevron = (
    <span
      className="transition-transform duration-200"
      style={{
        color: 'var(--color-muted)',
        display: 'flex',
        transform: isDownloadsExpanded ? 'rotate(180deg)' : 'rotate(0deg)',
      }}
    >
      {Icons.chevron}
    </span>
  )

  const downloadsStatus = (
    <span className="flex items-center gap-2">
      <span
        className="inline-block h-2.5 w-2.5 rounded-full"
        style={{ background: connectionState.color }}
      />
      {downloadsChevron}
    </span>
  )

  return (
    <aside
      className="fixed bottom-0 left-0 top-0 flex w-[17rem] flex-col overflow-hidden pt-5 pb-5 lg:bottom-5 lg:left-5 lg:top-[96px] lg:w-72 lg:rounded-[30px]"
      style={{
        background: 'linear-gradient(180deg, rgba(15, 27, 45, 0.95) 0%, rgba(10, 19, 32, 0.98) 100%)',
        border: '1px solid var(--color-border)',
        boxShadow: 'var(--shadow-soft)',
        backdropFilter: 'blur(18px)',
        zIndex: 45,
        width: 'min(20rem, calc(100vw - 2rem))',
        top: 'calc(var(--mobile-topbar-offset) + var(--mobile-topbar-height) + 0.5rem)',
        bottom: 'calc(var(--mobile-nav-height) + 0.5rem)',
        left: '0.75rem',
        borderRadius: '28px 28px 22px 22px',
        // 移动端：根据 mobileOpen 状态滑入/滑出
        transform: mobileOpen ? 'translateX(0)' : 'translateX(-110%)',
        transition: 'transform 0.28s cubic-bezier(0.32, 0, 0.12, 1)',
      }}
    // 桌面端始终可见（lg+，通过 CSS 覆盖 transform）
    >
      <div className="sticky top-0 z-10 px-5 pb-4 pt-5 sm:px-6"
        style={{
          background: 'linear-gradient(180deg, rgba(15, 27, 45, 0.98) 0%, rgba(15, 27, 45, 0.92) 75%, rgba(15, 27, 45, 0) 100%)',
          backdropFilter: 'blur(12px)',
          WebkitBackdropFilter: 'blur(12px)',
        }}
      >
        <div className="flex items-start justify-between gap-4">
          <div>
            <div className="text-[10px] font-semibold uppercase tracking-[0.24em]" style={{ color: 'var(--color-muted-soft)' }}>
              Navigation
            </div>
            <div className="mt-2 text-xl font-semibold" style={{ color: 'var(--color-text)' }}>
              影视资料馆
            </div>
            <div className="mt-1 text-xs lg:hidden" style={{ color: 'var(--color-muted)' }}>
              从这里访问次级页面与配置项
            </div>
          </div>
          <button
            type="button"
            onClick={() => onMobileClose?.()}
            className="flex size-11 items-center justify-center rounded-2xl transition-all duration-150 lg:hidden"
            style={{
              background: 'rgba(255,255,255,0.04)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-text)',
            }}
            aria-label="关闭导航抽屉"
          >
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round">
              <path d="M6 6l12 12" />
              <path d="M18 6L6 18" />
            </svg>
          </button>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto pb-3">
        <NavItem
          icon={Icons.library}
          label="媒体库"
          active={active === 'all'}
          onClick={handleLibraryClick}
          bold
          right={libraryChevron}
        />

        <div
          className="overflow-hidden transition-all duration-200 ease-in-out"
          style={{ maxHeight: isLibraryExpanded ? '120px' : '0px', opacity: isLibraryExpanded ? 1 : 0 }}
        >
          <div className="relative">
            <div className="absolute bottom-0 top-0" style={{ left: 38, width: 1, background: 'rgba(144, 178, 221, 0.16)' }} />
            {[
              { key: 'movies', label: '电影', icon: Icons.movie },
              { key: 'tv', label: '电视剧', icon: Icons.tv },
            ].map(({ key, label, icon }) => (
              <NavItem
                key={key}
                icon={icon}
                label={label}
                active={active === key}
                onClick={() => onSelect(key)}
                indent
              />
            ))}
          </div>
        </div>

        <div className="hidden lg:block">
          <NavItem
            icon={Icons.calendar}
            label="新番列表"
            active={active === 'calendar'}
            onClick={() => onSelect('calendar')}
            bold
          />
        </div>

        <div className="hidden lg:block">
          <NavItem
            icon={Icons.search}
            label="资源检索"
            active={active === 'scraper-search'}
            onClick={() => onSelect('scraper-search')}
            bold
          />
        </div>

        <div className="hidden lg:block">
          <NavItem
            icon={Icons.rss}
            label="订阅列表"
            active={active === 'subscriptions'}
            onClick={() => onSelect('subscriptions')}
            bold
          />
        </div>

        <NavItem
          icon={Icons.download}
          label="下载管理"
          active={active === 'downloads'}
          onClick={handleDownloadsClick}
          bold
          meta={aria2Connected ? totalDownloadCount : null}
          right={downloadsStatus}
        />

        <div
          className="overflow-hidden transition-all duration-200 ease-in-out"
          style={{ maxHeight: isDownloadsExpanded ? '180px' : '0px', opacity: isDownloadsExpanded ? 1 : 0 }}
        >
          <div className="relative">
            <div className="absolute bottom-0 top-0" style={{ left: 38, width: 1, background: 'rgba(144, 178, 221, 0.16)' }} />
            {[
              { key: 'downloads-active', label: '下载中', icon: Icons.bolt },
              { key: 'downloads-waiting', label: '等待中', icon: Icons.queue },
              { key: 'downloads-stopped', label: '已停止', icon: Icons.archive },
            ].map(({ key, label, icon }) => (
              <NavItem
                key={key}
                icon={icon}
                label={label}
                active={active === key}
                onClick={() => onSelect(key)}
                indent
                meta={
                  key === 'downloads-active'
                    ? activeCount
                    : key === 'downloads-waiting'
                      ? waitingCount
                      : stoppedCount
                }
              />
            ))}
          </div>
        </div>

        <div className="hidden lg:block">
          <NavItem
            icon={Icons.cloudDownload}
            label="云下载"
            active={active === 'u115-offline'}
            onClick={() => onSelect('u115-offline')}
            bold
          />
        </div>

        <NavItem
          icon={Icons.activity}
          label="日志"
          active={active === 'logs'}
          onClick={() => onSelect('logs')}
          bold
        />

        <NavItem
          icon={Icons.settings}
          label="配置文件"
          active={false}
          onClick={handleConfigClick}
          bold
          right={
            <span
              className="transition-transform duration-200"
              style={{
                color: 'var(--color-muted)',
                display: 'flex',
                transform: isConfigExpanded ? 'rotate(180deg)' : 'rotate(0deg)',
              }}
            >
              {Icons.chevron}
            </span>
          }
        />

        <div
          className="overflow-hidden transition-all duration-200 ease-in-out"
          style={{ maxHeight: isConfigExpanded ? '120px' : '0px', opacity: isConfigExpanded ? 1 : 0 }}
        >
          <div className="relative">
            <div className="absolute bottom-0 top-0" style={{ left: 38, width: 1, background: 'rgba(144, 178, 221, 0.16)' }} />
            <NavItem
              icon={Icons.settings}
              label="基础配置"
              active={active === 'config'}
              onClick={() => onSelect('config')}
              indent
            />
            <NavItem
              icon={Icons.search}
              label="识别规则"
              active={active === 'config-filename-rules'}
              onClick={() => onSelect('config-filename-rules')}
              indent
            />
          </div>
        </div>
      </div>

      <div
        className="sticky bottom-0 z-10 border-t px-4 pb-2 pt-3 sm:px-5"
        style={{
          borderColor: 'rgba(144, 178, 221, 0.12)',
          background: 'linear-gradient(180deg, rgba(15, 27, 45, 0) 0%, rgba(15, 27, 45, 0.94) 18%, rgba(10, 19, 32, 0.98) 100%)',
          backdropFilter: 'blur(12px)',
          WebkitBackdropFilter: 'blur(12px)',
        }}
      >
        {onLogout ? (
          <div>
            <div className="mb-2 px-1 text-[11px] font-semibold uppercase tracking-[0.18em]" style={{ color: 'var(--color-muted-soft)' }}>
              账户操作
            </div>
            <button
              type="button"
              onClick={handleLogoutClick}
              className="flex min-h-12 w-full items-center gap-3 rounded-[18px] px-4 py-3 text-left font-semibold transition-all duration-150"
              style={{
                background: confirmingLogout ? 'rgba(239, 125, 117, 0.14)' : 'rgba(239, 125, 117, 0.08)',
                border: confirmingLogout ? '1px solid rgba(239, 125, 117, 0.34)' : '1px solid rgba(239, 125, 117, 0.22)',
                color: 'var(--color-danger)',
              }}
            >
              <span className="flex-shrink-0">{Icons.logout}</span>
              <span className="flex-1 text-[15px]">{confirmingLogout ? '再次点击确认退出' : '退出登录'}</span>
            </button>
          </div>
        ) : null}

        <a
          href="https://github.com/BenZinaDaze/Meta2Cloud"
          target="_blank"
          rel="noopener noreferrer"
          className="mx-2 mb-2 mt-3 flex items-center justify-between rounded-[16px] px-3 py-2.5 transition-colors duration-150 hover:bg-white/5"
          style={{ border: '1px solid rgba(144, 178, 221, 0.12)', textDecoration: 'none' }}
          title="View Repository on GitHub"
        >
          <div className="flex items-center gap-2">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" style={{ color: 'var(--color-muted-soft)' }}>
              <path fillRule="evenodd" clipRule="evenodd" d="M12 2C6.477 2 2 6.477 2 12c0 4.42 2.87 8.17 6.84 9.5.5.08.66-.23.66-.5v-1.69c-2.77.6-3.36-1.34-3.36-1.34-.45-1.15-1.11-1.46-1.11-1.46-.91-.62.07-.6.07-.6 1 .07 1.53 1.03 1.53 1.03.87 1.52 2.34 1.07 2.91.83.09-.65.35-1.09.63-1.34-2.22-.25-4.55-1.11-4.55-4.92 0-1.11.38-2 1.03-2.71-.1-.25-.45-1.29.1-2.64 0 0 .84-.27 2.75 1.02.79-.22 1.65-.33 2.5-.33.85 0 1.71.11 2.5.33 1.91-1.29 2.75-1.02 2.75-1.02.55 1.35.2 2.39.1 2.64.65.71 1.03 1.6 1.03 2.71 0 3.82-2.34 4.66-4.57 4.91.36.31.69.92.69 1.85V21c0 .27.16.59.67.5C19.14 20.16 22 16.42 22 12A10 10 0 0 0 12 2z" />
            </svg>
            <span className="text-[12px] font-medium" style={{ color: 'var(--color-muted)' }}>GitHub</span>
          </div>
          <div className="flex items-center gap-2">
            {hasUpdate && (
              <span className="animate-pulse flex items-center justify-center rounded-md px-1.5 py-0.5 text-[10px] font-bold shadow-sm" style={{ background: 'var(--color-accent)', color: 'var(--color-primary-text)' }}>
                可更新 {latestVersion}
              </span>
            )}
            <span className="min-w-[64px] rounded-md px-2.5 py-0.5 text-center text-[11px] font-semibold tracking-[0.01em]" style={{ background: 'rgba(255,255,255,0.05)', border: '1px solid var(--color-border)', color: 'var(--color-text)' }}>
              {currentVersion}
            </span>
          </div>
        </a>
      </div>
    </aside>
  )
}
