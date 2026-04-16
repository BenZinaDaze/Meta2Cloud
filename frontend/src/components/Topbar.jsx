import { triggerPipeline } from '../api'
import BrandMark from './BrandMark'

export default function Topbar({ onOpenParseTest, onToggleSidebar, onToast }) {
  return (
    <header
      className="app-topbar fixed left-3 right-3 top-3 z-50 flex h-14 items-center justify-between rounded-[22px] px-3 sm:left-5 sm:right-5 sm:top-5 sm:h-16 sm:rounded-[26px] sm:px-5"
      style={{
        top: 'var(--mobile-topbar-offset)',
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
          className="hidden h-11 w-11 items-center justify-center rounded-2xl transition-all duration-150 sm:flex lg:hidden"
          style={{
            background: 'rgba(255,255,255,0.04)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-muted)',
          }}
          title="打开导航"
          aria-label="打开侧边栏"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
            <line x1="3" y1="6" x2="21" y2="6" />
            <line x1="3" y1="12" x2="21" y2="12" />
            <line x1="3" y1="18" x2="21" y2="18" />
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
            Meta<span style={{ color: 'var(--color-accent)' }}>2Cloud</span>
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
          onClick={async () => {
            try {
              await triggerPipeline()
              onToast?.('success', '已发送整理指令', '后台整理将立即启动')
            } catch (e) {
              onToast?.('error', '触发失败', e?.response?.data?.detail || e.message)
            }
          }}
          className="flex min-h-11 items-center gap-1.5 rounded-full px-3 py-2 text-xs font-semibold transition-all duration-150 sm:px-4"
          style={{
            color: 'var(--color-accent-hover)',
            background: 'linear-gradient(135deg, rgba(200, 146, 77, 0.16) 0%, rgba(200, 146, 77, 0.05) 100%)',
            border: '1px solid rgba(200, 146, 77, 0.3)',
          }}
          title="触发整理"
        >
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" />
          </svg>
          <span className="hidden sm:inline">一键整理</span>
        </button>

        <button
          onClick={onOpenParseTest}
          className="hidden min-h-11 items-center gap-1.5 rounded-full px-3 py-2 text-xs font-semibold transition-all duration-150 min-[430px]:flex sm:px-4"
          style={{
            color: 'var(--color-text)',
            background: 'rgba(255,255,255,0.03)',
            border: '1px solid var(--color-border)',
          }}
          title="解析测试"
        >
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <path d="M4 7h16" />
            <path d="M7 12h10" />
            <path d="M10 17h4" />
          </svg>
          <span className="hidden sm:inline">解析测试</span>
        </button>
      </div>
    </header>
  )
}
