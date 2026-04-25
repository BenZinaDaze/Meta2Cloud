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
      className="fixed right-5 top-24 z-[9999] flex flex-col gap-3"
      style={{ minWidth: 300, maxWidth: 380 }}
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

  const icons = {
    success: (
      <div className="flex items-center justify-center size-8 rounded-full text-emerald-400 shrink-0"
        style={{ background: 'var(--success-bg)' }}>
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
          <polyline points="20 6 9 17 4 12"></polyline>
        </svg>
      </div>
    ),
    error: (
      <div className="flex items-center justify-center size-8 rounded-full text-red-500 shrink-0"
        style={{ background: 'var(--danger-soft)' }}>
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
          <circle cx="12" cy="12" r="10"></circle>
          <line x1="15" y1="9" x2="9" y2="15"></line>
          <line x1="9" y1="9" x2="15" y2="15"></line>
        </svg>
      </div>
    ),
    warning: (
      <div className="flex items-center justify-center size-8 rounded-full text-amber-400 shrink-0"
        style={{ background: 'var(--warning-bg)' }}>
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
          <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path>
          <line x1="12" y1="9" x2="12" y2="13"></line>
          <line x1="12" y1="17" x2="12.01" y2="17"></line>
        </svg>
      </div>
    ),
    info: (
      <div className="flex items-center justify-center size-8 rounded-full text-blue-400 shrink-0"
        style={{ background: 'var(--info-bg)' }}>
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
          <circle cx="12" cy="12" r="10"></circle>
          <line x1="12" y1="16" x2="12" y2="12"></line>
          <line x1="12" y1="8" x2="12.01" y2="8"></line>
        </svg>
      </div>
    ),
    loading: (
      <div className="flex items-center justify-center size-8 rounded-full shrink-0"
        style={{ background: 'var(--accent-soft)', color: 'var(--color-accent)' }}>
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" className="animate-spin">
          <path d="M21 12a9 9 0 1 1-6.219-8.56"></path>
        </svg>
      </div>
    )
  }

  const icon = icons[toast.type] || icons.info

  return (
    <div
      className="flex items-start gap-3.5 rounded-2xl p-4 w-full relative overflow-hidden group shadow-[var(--toast-shadow)]"
      style={{
        background: 'var(--toast-bg)',
        border: '1px solid var(--border-default)',
        animation: 'slideInRight 0.3s cubic-bezier(0.16, 1, 0.3, 1)',
        backdropFilter: 'blur(16px)',
      }}
    >
      {icon}
      
      <div className="flex-1 min-w-0 pt-0.5">
        {toast.title && (
          <h4 className="text-[15px] font-bold text-white/95 mb-1 tracking-wide">
            {toast.title}
          </h4>
        )}
        <p className="text-sm font-medium text-white/60 leading-snug break-words">
          {toast.message}
        </p>
      </div>
      
      <button
        onClick={() => onRemove(toast.id)}
        className="flex items-center justify-center size-6 rounded-full text-white/40 hover:text-white hover:bg-white/10 transition-colors shrink-0 -mr-1 -mt-1 outline-none"
        aria-label="Close"
      >
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
          <line x1="18" y1="6" x2="6" y2="18"></line>
          <line x1="6" y1="6" x2="18" y2="18"></line>
        </svg>
      </button>
    </div>
  )
}
