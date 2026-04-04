import { useState } from 'react'
import { logout } from '../api'

export default function Topbar({ onLogout }) {
  const [confirming, setConfirming] = useState(false)

  async function handleLogout() {
    if (!confirming) { setConfirming(true); setTimeout(() => setConfirming(false), 3000); return }
    try { await logout() } catch { void 0 }
    onLogout?.()
  }

  return (
    <header
      className="fixed left-5 right-5 top-5 z-50 flex h-16 items-center justify-between rounded-[26px] px-5"
      style={{
        background: 'linear-gradient(180deg, rgba(15, 27, 45, 0.92) 0%, rgba(11, 21, 35, 0.96) 100%)',
        border: '1px solid var(--color-border)',
        boxShadow: 'var(--shadow-soft)',
        backdropFilter: 'blur(18px)',
      }}
    >
      <div className="flex items-center gap-4">
        <div
          className="flex size-10 items-center justify-center rounded-2xl text-sm font-bold text-white"
          style={{
            background: 'linear-gradient(135deg, var(--color-accent) 0%, #a56d2c 100%)',
            boxShadow: '0 10px 24px rgba(200, 146, 77, 0.28)',
          }}
        >
          M
        </div>
        <div>
          <div className="text-[10px] font-semibold uppercase tracking-[0.24em]" style={{ color: 'var(--color-muted)' }}>
            Media Archive
          </div>
          <span className="block text-base font-semibold" style={{ color: 'var(--color-text)' }}>
            Metadata<span style={{ color: 'var(--color-accent)' }}>2GD</span>
          </span>
        </div>
      </div>

      <div className="flex items-center gap-3">
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
        {onLogout && (
          <button
            onClick={handleLogout}
            className="flex items-center gap-2 rounded-full px-4 py-2 text-xs font-semibold transition-all duration-150"
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
            {confirming ? '再次点击确认' : '退出登录'}
          </button>
        )}
      </div>
    </header>
  )
}
