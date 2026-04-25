import { Film, Tv } from 'lucide-react'
import type { MediaItem } from '@/types/api'

interface MediaCardProps {
  item: MediaItem
  onClick?: (item: MediaItem) => void
  compact?: boolean
}

export default function MediaCard({ item, onClick, compact = false }: MediaCardProps) {
  const isTV = item.media_type === 'tv'
  const pct = isTV && item.total_episodes && item.total_episodes > 0 && item.in_library_episodes !== undefined
    ? Math.round(item.in_library_episodes / item.total_episodes * 100)
    : null

  return (
    <div
      onClick={() => onClick?.(item)}
      className="group relative flex cursor-pointer flex-col overflow-hidden rounded-xl transition-all duration-200 hover:-translate-y-1"
    >
      <div className="relative aspect-[2/3] overflow-hidden bg-muted">
        {item.poster_url ? (
          <img
            src={item.poster_url}
            alt={item.title}
            className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
            loading="lazy"
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-muted-foreground">
            {isTV ? <Tv className="size-10" /> : <Film className="size-10" />}
          </div>
        )}

        {item.rating > 0 && (
          <div className="absolute right-2 top-2 rounded-full bg-muted/80 px-2 py-0.5 text-[10px] font-bold text-warning backdrop-blur-sm">
            ★ {item.rating.toFixed(1)}
          </div>
        )}

        <div className="absolute left-2 top-2 rounded-full bg-muted/80 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider backdrop-blur-sm">
          {isTV ? 'TV' : '电影'}
        </div>

        <div className="pointer-events-none absolute inset-0 bg-gradient-to-t from-black/40 to-transparent opacity-0 transition-opacity duration-200 group-hover:opacity-100" />

        {isTV && pct !== null && pct > 0 && (
          <div className="absolute bottom-0 left-0 right-0 h-1 bg-black/30">
            <div
              className="h-full bg-brand shadow-brand-glow transition-all duration-300"
              style={{ width: `${Math.min(pct, 100)}%` }}
            />
          </div>
        )}
      </div>

      <div className="flex flex-col gap-1 rounded-b-xl bg-muted p-2.5" style={{ height: compact ? 56 : 76 }}>
        <p className={`font-semibold leading-snug ${compact ? 'line-clamp-1 text-sm' : 'line-clamp-2 text-sm'}`}>
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
