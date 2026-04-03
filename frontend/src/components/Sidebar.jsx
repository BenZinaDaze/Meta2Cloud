import { useState, useEffect } from 'react'

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
function NavItem({ icon, label, active, onClick, indent = false, right, bold = false }) {
  return (
    <button
      onClick={onClick}
      className="flex items-center gap-3 font-medium transition-all duration-150 text-left relative overflow-hidden group"
      style={{
        padding: indent ? '11px 18px 11px 52px' : '12px 18px',
        margin: '2px 10px',
        width: 'calc(100% - 20px)',
        borderRadius: 28,
        background: active ? 'color-mix(in srgb, var(--color-accent) 18%, transparent)' : 'transparent',
        color: active ? 'var(--color-accent)' : (bold ? 'var(--color-text)' : 'var(--color-muted)'),
      }}
    >
      {/* Hover 涟漪层 */}
      <span
        className="absolute inset-0 opacity-0 group-hover:opacity-100 transition-opacity duration-150 rounded-[28px]"
        style={{ background: active ? 'color-mix(in srgb, var(--color-accent) 8%, transparent)' : 'rgba(255,255,255,0.04)' }}
      />
      {/* 图标 */}
      <span className="relative flex-shrink-0" style={{ color: active ? 'var(--color-accent)' : 'inherit' }}>
        {icon}
      </span>
      {/* 文字 */}
      <span className="relative flex-1" style={{ fontSize: indent ? 15 : 16, fontWeight: bold ? 600 : 500 }}>
        {label}
      </span>
      {/* 右侧插槽（折叠箭头等） */}
      {right && <span className="relative flex-shrink-0">{right}</span>}
    </button>
  )
}

export default function Sidebar({ active, onSelect }) {
  const [expanded, setExpanded] = useState(true)

  useEffect(() => {
    if (active === 'movies' || active === 'tv') setExpanded(true)
  }, [active])

  function handleLibraryClick() {
    setExpanded(prev => !prev)
    onSelect('all')
  }

  const chevron = (
    <span
      className="transition-transform duration-200"
      style={{
        color: 'var(--color-muted)',
        display: 'flex',
        transform: expanded ? 'rotate(180deg)' : 'rotate(0deg)',
      }}
    >
      {Icons.chevron}
    </span>
  )

  return (
    <aside
      className="fixed top-14 left-0 bottom-0 w-72 flex flex-col pt-4 pb-4"
      style={{ background: 'var(--color-surface)', borderRight: '1px solid var(--color-border)' }}
    >
      {/* 媒体库（父项）—— 与 NavItem 完全相同的 margin/padding */}
      <NavItem
        icon={Icons.library}
        label="媒体库"
        active={active === 'all'}
        onClick={handleLibraryClick}
        bold
        right={chevron}
      />

      {/* 子项：电影 / 电视剧 */}
      <div
        className="overflow-hidden transition-all duration-200 ease-in-out"
        style={{ maxHeight: expanded ? '120px' : '0px', opacity: expanded ? 1 : 0 }}
      >
        <div className="relative">
          {/* 连接线 */}
          <div className="absolute top-0 bottom-0" style={{ left: 34, width: 1, background: 'var(--color-border)' }} />
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

      {/* 配置文件（与媒体库同级）—— 完全相同的 NavItem 样式 */}
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
