import { useState } from 'react'
import { logout } from '../api'

export default function Topbar({ onLogout }) {
  const [confirming, setConfirming] = useState(false)

  async function handleLogout() {
    if (!confirming) { setConfirming(true); setTimeout(() => setConfirming(false), 3000); return }
    try { await logout() } catch (_) {}
    onLogout?.()
  }

  return (
    <header
      className="fixed top-0 left-0 right-0 z-50 h-14 flex items-center justify-between px-6 border-b"
      style={{ background: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
    >
      {/* Logo */}
      <div className="flex items-center gap-3">
        <div
          className="w-8 h-8 rounded-lg flex items-center justify-center text-white font-bold text-sm"
          style={{ background: 'var(--color-accent)' }}
        >
          M
        </div>
        <span className="font-semibold text-base tracking-tight" style={{ color: 'var(--color-text)' }}>
          Metadata<span style={{ color: 'var(--color-accent)' }}>2GD</span>
        </span>
      </div>

      {/* 右侧：登出按钮 */}
      {onLogout && (
        <button
          onClick={handleLogout}
          className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded-lg transition-all"
          style={{
            color: confirming ? '#ef4444' : 'var(--color-muted)',
            background: confirming ? 'rgba(239,68,68,0.1)' : 'transparent',
            border: confirming ? '1px solid rgba(239,68,68,0.3)' : '1px solid transparent',
          }}
          title="退出登录"
        >
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/>
            <polyline points="16 17 21 12 16 7"/>
            <line x1="21" y1="12" x2="9" y2="12"/>
          </svg>
          {confirming ? '再次点击确认' : '退出'}
        </button>
      )}
    </header>
  )
}
