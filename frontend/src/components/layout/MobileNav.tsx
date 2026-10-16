import { BookOpen, Search, Calendar, Cloud, Menu } from 'lucide-react'
import { cn } from '@/lib/utils'

interface MobileNavProps {
  active: string
  onSelect: (key: string) => void
  onToggleSidebar?: () => void
  u115Authorized?: boolean
}

export default function MobileNav({
  active,
  onSelect,
  onToggleSidebar,
  u115Authorized = false,
}: MobileNavProps) {
  const isLibrary = ['all', 'movies', 'tv'].includes(active)
  const isMore = ['subscriptions', 'ingest-history', 'strm-sync', 'strm-playback', 'logs', 'config', 'config-filename-rules'].includes(active)

  const tabs = [
    { key: 'all', label: '媒体库', icon: BookOpen, isActive: isLibrary },
    { key: 'scraper-search', label: '检索', icon: Search, isActive: active === 'scraper-search' },
    { key: 'calendar', label: '新番', icon: Calendar, isActive: active === 'calendar' },
    ...(u115Authorized ? [{ key: 'u115-offline', label: '云下载', icon: Cloud, isActive: active === 'u115-offline' }] : []),
  ]

  return (
    <nav className="fixed bottom-0 left-0 right-0 z-40 border-t border-border/60 glass-strong lg:hidden pb-[env(safe-area-inset-bottom)]">
      <div className="flex">
        {tabs.map((tab) => {
          const Icon = tab.icon
          return (
            <button
              key={tab.key}
              onClick={() => onSelect(tab.key)}
              className={cn(
                "relative flex min-h-16 flex-1 flex-col items-center justify-center gap-1 py-2.5 transition-colors duration-300",
                tab.isActive ? "text-primary" : "text-muted-foreground"
              )}
            >
              {tab.isActive && (
                <span className="absolute top-0 h-[2px] w-8 rounded-full bg-gradient-to-r from-tide to-tide-2 glow-tide-sm" />
              )}
              <Icon className={cn("size-[22px] transition-transform duration-300", tab.isActive && "scale-110")} />
              <span className={cn("text-[10px]", tab.isActive ? "font-semibold" : "font-medium")}>
                {tab.label}
              </span>
            </button>
          )
        })}
        {/* More menu button */}
        <button
          onClick={onToggleSidebar}
          className={cn(
            "relative flex min-h-16 flex-1 flex-col items-center justify-center gap-1 py-2.5 transition-colors duration-300",
            isMore ? "text-primary" : "text-muted-foreground"
          )}
        >
          {isMore && (
            <span className="absolute top-0 h-[2px] w-8 rounded-full bg-gradient-to-r from-tide to-tide-2 glow-tide-sm" />
          )}
          <Menu className={cn("size-[22px] transition-transform duration-300", isMore && "scale-110")} />
          <span className={cn("text-[10px]", isMore ? "font-semibold" : "font-medium")}>更多</span>
        </button>
      </div>
    </nav>
  )
}
