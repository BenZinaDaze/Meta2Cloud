// 圆形进度条
function RingProgress({ value, max, size = 18, stroke = 2.4 }) {
  const pct = max > 0 ? value / max : 0
  const r = (size - stroke) / 2
  const circ = 2 * Math.PI * r
  const dash = pct * circ
  const color = pct >= 1 ? 'var(--color-success)' : pct > 0.5 ? 'var(--color-accent)' : 'var(--color-warning)'
  return (
    <svg width={size} height={size} style={{ transform: 'rotate(-90deg)' }}>
      <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="var(--color-border)" strokeWidth={stroke} />
      <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke={color} strokeWidth={stroke}
        strokeDasharray={`${dash} ${circ}`} strokeLinecap="round"
        style={{ transition: 'stroke-dasharray 0.4s ease' }} />
    </svg>
  )
}

/**
 * MediaCard
 *
 * compact=false (默认) — 网格视图：line-clamp-2 标题，显示集数
 * compact=true          — 横向滚动行：固定高度信息区，line-clamp-1 标题，高低一致
 */
export default function MediaCard({ item, onClick, compact = false }) {
  const isTV = item.media_type === 'tv'
  const pct = isTV && item.total_episodes > 0 && item.in_library_episodes !== undefined
    ? Math.round(item.in_library_episodes / item.total_episodes * 100)
    : null

  return (
    <div
      onClick={() => onClick?.(item)}
      className="group relative flex cursor-pointer flex-col overflow-hidden rounded-[20px] transition-all duration-200 hover:-translate-y-1 sm:rounded-[22px]"
      style={{
        background: 'linear-gradient(180deg, rgba(20, 37, 59, 0.96) 0%, rgba(13, 26, 44, 0.98) 100%)',
        border: '1px solid var(--color-border)',
        boxShadow: '0 18px 38px rgba(3, 11, 22, 0.22)',
      }}
    >
      <div className="relative aspect-[2/3] overflow-hidden bg-gray-800">
        {item.poster_url ? (
          <img
            src={item.poster_url}
            alt={item.title}
            className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
            loading="lazy"
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-4xl"
            style={{ background: 'var(--color-surface)' }}>
            {isTV ? '📺' : '🎬'}
          </div>
        )}

        {item.rating > 0 && (
          <div className="absolute right-2.5 top-2.5 rounded-full px-2 py-1 text-[10px] font-bold sm:right-3 sm:top-3 sm:text-[11px]"
            style={{ background: 'rgba(4, 11, 21, 0.84)', color: 'var(--color-warning)', border: '1px solid rgba(255,255,255,0.08)' }}>
            ★ {item.rating}
          </div>
        )}

        <div className="absolute left-2.5 top-2.5 rounded-full px-2 py-1 text-[10px] font-semibold uppercase tracking-[0.14em] sm:left-3 sm:top-3 sm:text-[11px]"
          style={{ background: 'rgba(4, 11, 21, 0.84)', color: 'var(--color-muted)', border: '1px solid rgba(255,255,255,0.08)' }}>
          {isTV ? 'TV' : '电影'}
        </div>

        {isTV && pct !== null && (
          <div className="absolute bottom-2 right-2 flex items-center gap-1 rounded-full sm:bottom-2.5 sm:right-2.5"
            style={{ background: 'rgba(4, 11, 21, 0.78)', padding: '2px 4px', border: '1px solid rgba(255,255,255,0.06)' }}>
            <RingProgress value={item.in_library_episodes} max={item.total_episodes} />
            <span className="text-[10px] font-semibold tabular-nums" style={{ color: 'var(--color-text)' }}>{pct}%</span>
          </div>
        )}

        <div className="absolute inset-0 opacity-0 group-hover:opacity-100 transition-opacity duration-200"
          style={{ background: 'linear-gradient(to top, rgba(3, 9, 17, 0.72) 0%, transparent 56%)' }} />
      </div>

      <div
        className="flex flex-col px-3 pt-3 pb-3 sm:px-3.5 sm:pt-3.5 sm:pb-3.5"
        style={{ height: compact ? 64 : 88, overflow: 'hidden' }}
      >
        <p
          className={`font-semibold leading-snug ${compact ? 'line-clamp-1 text-sm' : 'line-clamp-2 text-[13px] sm:text-sm'}`}
          style={{ color: 'var(--color-text)' }}
        >
          {item.title}
        </p>
        <div className="mt-auto flex min-w-0 flex-nowrap items-center gap-1.5">
          {item.year && (
            <span className="shrink-0 text-xs tabular-nums" style={{ color: 'var(--color-muted)' }}>{item.year}</span>
          )}
          {item.year && !compact && isTV && item.in_library_episodes !== undefined && (
            <span className="text-xs shrink-0" style={{ color: 'var(--color-border)' }}>·</span>
          )}
          {!compact && isTV && item.in_library_episodes !== undefined && (
            <span className="truncate text-[11px] sm:text-xs" style={{ color: 'var(--color-muted)' }}>
              已入库 {item.in_library_episodes}/{item.total_episodes} 集
            </span>
          )}
        </div>
      </div>
    </div>
  )
}
