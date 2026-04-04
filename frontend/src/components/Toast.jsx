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
      className="fixed right-5 top-24 z-[9999] flex flex-col gap-2"
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
    success: { bg: 'rgba(17, 48, 36, 0.94)', border: 'rgba(94, 211, 154, 0.24)', icon: '✅' },
    error:   { bg: 'rgba(61, 24, 22, 0.95)', border: 'rgba(239, 125, 117, 0.28)', icon: '❌' },
    info:    { bg: 'rgba(16, 34, 54, 0.95)', border: 'rgba(122, 165, 219, 0.28)', icon: 'ℹ️' },
    loading: { bg: 'rgba(55, 42, 15, 0.95)', border: 'rgba(240, 196, 107, 0.28)', icon: '⏳' },
  }
  const c = colors[toast.type] || colors.info

  return (
    <div
      className="flex items-start gap-3 rounded-2xl px-4 py-3"
      style={{
        background: c.bg,
        border: `1px solid ${c.border}`,
        animation: 'slideInRight 0.2s ease-out',
        backdropFilter: 'blur(14px)',
        boxShadow: 'var(--shadow-soft)',
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
