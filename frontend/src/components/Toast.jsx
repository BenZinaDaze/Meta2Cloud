import { useEffect, useRef } from 'react'

/**
 * Toast 通知组件
 * Props:
 *   toasts: Array<{ id, type, title, message }>
 *   onRemove: (id) => void
 */
export default function ToastContainer({ toasts, onRemove }) {
  return (
    <div
      className="fixed top-4 right-4 flex flex-col gap-2 z-[9999]"
      style={{ minWidth: 280, maxWidth: 380 }}
    >
      {toasts.map(t => (
        <ToastItem key={t.id} toast={t} onRemove={onRemove} />
      ))}
    </div>
  )
}

function ToastItem({ toast, onRemove }) {
  const timerRef = useRef(null)

  useEffect(() => {
    timerRef.current = setTimeout(() => onRemove(toast.id), 4000)
    return () => clearTimeout(timerRef.current)
  }, [toast.id, onRemove])

  const colors = {
    success: { bg: '#1a3a2a', border: '#2d6a4f', icon: '✅' },
    error:   { bg: '#3a1a1a', border: '#7a2828', icon: '❌' },
    info:    { bg: '#1a2a3a', border: '#2d4a6f', icon: 'ℹ️' },
    loading: { bg: '#2a2a1a', border: '#6a6a28', icon: '⏳' },
  }
  const c = colors[toast.type] || colors.info

  return (
    <div
      className="flex items-start gap-3 px-4 py-3 rounded-xl shadow-2xl"
      style={{
        background: c.bg,
        border: `1px solid ${c.border}`,
        animation: 'slideInRight 0.2s ease-out',
        backdropFilter: 'blur(8px)',
      }}
    >
      <span style={{ fontSize: 18, flexShrink: 0, marginTop: 1 }}>{c.icon}</span>
      <div className="flex-1 min-w-0">
        {toast.title && (
          <p className="text-sm font-semibold mb-0.5" style={{ color: '#e8e8e8' }}>
            {toast.title}
          </p>
        )}
        <p className="text-sm" style={{ color: '#a8b4c0', wordBreak: 'break-word' }}>
          {toast.message}
        </p>
      </div>
      <button
        onClick={() => onRemove(toast.id)}
        className="text-xs flex-shrink-0 opacity-50 hover:opacity-100 transition-opacity"
        style={{ color: '#a8b4c0', marginTop: 2 }}
      >
        ✕
      </button>
    </div>
  )
}
