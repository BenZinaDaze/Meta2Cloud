import { Film, Tv, Star } from 'lucide-react'
import type { MediaItem } from '@/types/api'

interface MediaCardProps {
  item: MediaItem
  onClick?: (item: MediaItem) => void
  compact?: boolean
  style?: React.CSSProperties
}

export default function MediaCard({ item, onClick, compact = false, style }: MediaCardProps) {
  const isTV = item.media_type === 'tv'
  const pct = isTV && item.total_episodes && item.total_episodes > 0 && item.in_library_episodes !== undefined
    ? Math.round(item.in_library_episodes / item.total_episodes * 100)
    : null

  return (
    <div
      onClick={() => onClick?.(item)}
      style={style}
      className="group relative flex cursor-pointer flex-col overflow-hidden rounded-xl border border-border/70 bg-card transition-all duration-300 ease-[cubic-bezier(0.22,1,0.36,1)] hover:-translate-y-1.5 hover:border-primary/40 hover:shadow-[0_16px_40px_-12px_oklch(8%_0.01_140/0.9),0_0_24px_-8px_oklch(76%_0.19_150/0.35)]"
    >
      <div className="relative aspect-[2/3] overflow-hidden bg-muted">
        {item.poster_url ? (
          <img
            src={item.poster_url}
            alt={item.title}
            className="h-full w-full object-cover transition-transform duration-500 ease-[cubic-bezier(0.22,1,0.36,1)] group-hover:scale-[1.06]"
            loading="lazy"
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center bg-gradient-to-b from-muted to-background text-muted-foreground">
            {isTV ? <Tv className="size-10" /> : <Film className="size-10" />}
          </div>
        )}

        {/* 悬停渐变遮罩 */}
        <div className="pointer-events-none absolute inset-0 bg-gradient-to-t from-black/50 via-transparent to-transparent opacity-0 transition-opacity duration-300 group-hover:opacity-100" />

        {item.rating > 0 && (
          <div className="absolute right-2 top-2 flex items-center gap-0.5 rounded-full bg-black/55 px-2 py-0.5 text-[10px] font-bold text-warning backdrop-blur-md">
            <Star className="size-2.5 fill-current" />
            {item.rating.toFixed(1)}
          </div>
        )}

        <div className="absolute left-2 top-2 rounded-full bg-black/55 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-white/90 backdrop-blur-md">
          {isTV ? 'TV' : '电影'}
        </div>

        {isTV && pct !== null && pct > 0 && (
          <div className="absolute bottom-0 left-0 right-0 h-1 bg-black/40">
            <div
              className={`h-full transition-all duration-500 ${pct >= 100 ? 'bg-success' : 'bg-lime glow-lime'}`}
              style={{ width: `${Math.min(pct, 100)}%` }}
            />
          </div>
        )}
      </div>

      <div className="flex flex-col gap-1 bg-card p-2.5" style={{ height: compact ? 56 : 76 }}>
        <p className={`font-semibold leading-snug ${compact ? 'line-clamp-1 text-[13px]' : 'line-clamp-2 text-[13px]'}`}>
          {item.title}
        </p>
        <div className="mt-auto flex min-w-0 flex-nowrap items-center gap-1.5">
          {item.year && (
            <span className="shrink-0 text-xs tabular-nums text-muted-foreground">{item.year}</span>
          )}
          {item.year && !compact && isTV && item.in_library_episodes !== undefined && (
            <span className="text-xs text-border">·</span>
          )}
          {!compact && isTV && item.in_library_episodes !== undefined && (
            <span className="truncate text-xs text-muted-foreground">
              已入库 {item.in_library_episodes}/{item.total_episodes} 集
            </span>
          )}
        </div>
      </div>
    </div>
  )
}
