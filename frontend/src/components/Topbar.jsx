import { useState } from 'react'
import { logout } from '../api'
import BrandMark from './BrandMark'

export default function Topbar({ onLogout, onOpenParseTest, onToggleSidebar }) {
  const [confirming, setConfirming] = useState(false)

  async function handleLogout() {
    if (!confirming) { setConfirming(true); setTimeout(() => setConfirming(false), 3000); return }
    try { await logout() } catch { void 0 }
    onLogout?.()
  }

  return (
    <header
      className="fixed left-3 right-3 top-3 z-50 flex h-14 items-center justify-between rounded-[22px] px-3 sm:left-5 sm:right-5 sm:top-5 sm:h-16 sm:rounded-[26px] sm:px-5"
      style={{
        background: 'linear-gradient(180deg, rgba(15, 27, 45, 0.995) 0%, rgba(10, 19, 32, 1) 100%)',
        border: '1px solid var(--color-border-strong)',
        boxShadow: '0 20px 48px rgba(2, 8, 18, 0.58)',
        backdropFilter: 'blur(24px)',
        WebkitBackdropFilter: 'blur(24px)',
      }}
    >
      <div className="flex items-center gap-3">
        {/* 移动端汉堡菜单按钮，打开侧边栏 */}
        <button
          onClick={onToggleSidebar}
          className="flex h-9 w-9 items-center justify-center rounded-xl transition-all duration-150 lg:hidden"
          style={{
            background: 'rgba(255,255,255,0.04)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-muted)',
          }}
          title="打开导航"
          aria-label="打开侧边栏"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
            <line x1="3" y1="6" x2="21" y2="6"/>
            <line x1="3" y1="12" x2="21" y2="12"/>
            <line x1="3" y1="18" x2="21" y2="18"/>
          </svg>
        </button>

        <div
          className="flex size-9 items-center justify-center rounded-xl sm:size-10 sm:rounded-2xl"
          style={{ boxShadow: '0 10px 24px rgba(200, 146, 77, 0.22)' }}
        >
          <BrandMark className="size-9 sm:size-10" compact />
        </div>
        <div>
          <div className="hidden text-[10px] font-semibold uppercase tracking-[0.24em] sm:block" style={{ color: 'var(--color-muted)' }}>
            Media Archive
          </div>
          <span className="block text-sm font-semibold sm:text-base" style={{ color: 'var(--color-text)' }}>
            Metadata<span style={{ color: 'var(--color-accent)' }}>2GD</span>
          </span>
        </div>
      </div>

      <div className="flex items-center gap-2 sm:gap-3">
        <div
          className="hidden rounded-full px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.18em] md:block"
          style={{
            background: 'rgba(255,255,255,0.03)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-muted)',
          }}
        >
          Curated Library
        </div>
        <button
          onClick={onOpenParseTest}
          className="flex items-center gap-1.5 rounded-full px-2.5 py-2 text-xs font-semibold transition-all duration-150 sm:px-4"
          style={{
            color: 'var(--color-text)',
            background: 'rgba(255,255,255,0.03)',
            border: '1px solid var(--color-border)',
          }}
          title="解析测试"
        >
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <path d="M4 7h16"/>
            <path d="M7 12h10"/>
            <path d="M10 17h4"/>
          </svg>
          <span className="hidden sm:inline">解析测试</span>
        </button>
        {onLogout && (
          <button
            onClick={handleLogout}
            className="flex items-center gap-1.5 rounded-full px-2.5 py-2 text-xs font-semibold transition-all duration-150 sm:px-4"
            style={{
              color: confirming ? 'var(--color-danger)' : 'var(--color-text)',
              background: confirming ? 'rgba(239, 125, 117, 0.12)' : 'rgba(255,255,255,0.03)',
              border: confirming ? '1px solid rgba(239, 125, 117, 0.28)' : '1px solid var(--color-border)',
            }}
            title="退出登录"
          >
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/>
              <polyline points="16 17 21 12 16 7"/>
              <line x1="21" y1="12" x2="9" y2="12"/>
            </svg>
            <span className="hidden sm:inline">{confirming ? '再次点击确认' : '退出登录'}</span>
          </button>
        )}
      </div>
    </header>
  )
}
