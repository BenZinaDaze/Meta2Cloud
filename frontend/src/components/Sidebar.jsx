import { useState } from 'react'

// SVG 图标（MD 风格线条图标）
const Icons = {
  library: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z"/>
      <path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z"/>
    </svg>
  ),
  movie: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <rect x="2" y="2" width="20" height="20" rx="2.18" ry="2.18"/>
      <line x1="7" y1="2" x2="7" y2="22"/><line x1="17" y1="2" x2="17" y2="22"/>
      <line x1="2" y1="12" x2="22" y2="12"/><line x1="2" y1="7" x2="7" y2="7"/>
      <line x1="2" y1="17" x2="7" y2="17"/><line x1="17" y1="17" x2="22" y2="17"/>
      <line x1="17" y1="7" x2="22" y2="7"/>
    </svg>
  ),
  tv: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <rect x="2" y="7" width="20" height="15" rx="2" ry="2"/>
      <polyline points="17 2 12 7 7 2"/>
    </svg>
  ),
  download: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
      <polyline points="7 10 12 15 17 10"/>
      <line x1="12" y1="15" x2="12" y2="3"/>
    </svg>
  ),
  bolt: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/>
    </svg>
  ),
  queue: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <line x1="8" y1="6" x2="21" y2="6"/>
      <line x1="8" y1="12" x2="21" y2="12"/>
      <line x1="8" y1="18" x2="21" y2="18"/>
      <line x1="3" y1="6" x2="3.01" y2="6"/>
      <line x1="3" y1="12" x2="3.01" y2="12"/>
      <line x1="3" y1="18" x2="3.01" y2="18"/>
    </svg>
  ),
  archive: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="21 8 21 21 3 21 3 8"/>
      <rect x="1" y="3" width="22" height="5"/>
      <line x1="10" y1="12" x2="14" y2="12"/>
    </svg>
  ),
  activity: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>
    </svg>
  ),
  chevron: (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="6 9 12 15 18 9"/>
    </svg>
  ),
  settings: (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="3"/>
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>
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
        width: 'calc(100% - 20px)',
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

export default function Sidebar({ active, onSelect, aria2Overview = null, aria2ConnectionStatus = 'connecting' }) {
  const [libraryExpanded, setLibraryExpanded] = useState(true)
  const [downloadsExpanded, setDownloadsExpanded] = useState(true)
  const isLibraryExpanded = libraryExpanded || active === 'movies' || active === 'tv'
  const isDownloadsExpanded = downloadsExpanded || ['downloads-active', 'downloads-waiting', 'downloads-stopped'].includes(active)
  const activeCount = aria2Overview?.tasks?.active?.length ?? 0
  const waitingCount = aria2Overview?.tasks?.waiting?.length ?? 0
  const stoppedCount = aria2Overview?.tasks?.stopped?.length ?? 0
  const totalDownloadCount = activeCount + waitingCount + stoppedCount
  const aria2Connected = !!aria2Overview
  const connectionState = {
    connected: {
      label: '已连接',
      color: 'var(--color-success)',
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
      className="fixed bottom-5 left-5 top-[96px] flex w-72 flex-col rounded-[30px] pt-5 pb-5"
      style={{
        background: 'linear-gradient(180deg, rgba(15, 27, 45, 0.95) 0%, rgba(10, 19, 32, 0.98) 100%)',
        border: '1px solid var(--color-border)',
        boxShadow: 'var(--shadow-soft)',
        backdropFilter: 'blur(18px)',
      }}
    >
      <div className="px-6 pb-4">
        <div className="text-[10px] font-semibold uppercase tracking-[0.24em]" style={{ color: 'var(--color-muted-soft)' }}>
          Navigation
        </div>
        <div className="mt-2 text-xl font-semibold" style={{ color: 'var(--color-text)' }}>
          影视资料馆
        </div>
      </div>

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
          <div className="absolute top-0 bottom-0" style={{ left: 38, width: 1, background: 'rgba(144, 178, 221, 0.16)' }} />
          {[
            { key: 'movies', label: '电影',   icon: Icons.movie },
            { key: 'tv',     label: '电视剧', icon: Icons.tv },
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
          <div className="absolute top-0 bottom-0" style={{ left: 38, width: 1, background: 'rgba(144, 178, 221, 0.16)' }} />
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
        active={active === 'config'}
        onClick={() => onSelect('config')}
        bold
      />
    </aside>
  )
}
