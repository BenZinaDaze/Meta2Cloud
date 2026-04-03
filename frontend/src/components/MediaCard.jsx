// 圆形进度条
function RingProgress({ value, max, size = 34, stroke = 3.5 }) {
  const pct = max > 0 ? value / max : 0
  const r = (size - stroke) / 2
  const circ = 2 * Math.PI * r
  const dash = pct * circ
  const color = pct >= 1 ? 'var(--color-success)' : pct > 0.5 ? 'var(--color-accent)' : 'var(--color-warning)'
  return (
    <svg width={size} height={size} style={{ transform: 'rotate(-90deg)' }}>
      <circle cx={size/2} cy={size/2} r={r} fill="none" stroke="var(--color-border)" strokeWidth={stroke} />
      <circle cx={size/2} cy={size/2} r={r} fill="none" stroke={color} strokeWidth={stroke}
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
  const pct  = isTV && item.total_episodes > 0
    ? Math.round(item.in_library_episodes / item.total_episodes * 100)
    : null

  return (
    <div
      onClick={() => onClick?.(item)}
      className="group relative flex flex-col rounded-xl overflow-hidden cursor-pointer transition-all duration-200 hover:scale-[1.03] hover:shadow-2xl"
      style={{ background: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
    >
      {/* 海报区 */}
      <div className="relative aspect-[2/3] overflow-hidden bg-gray-800">
        {item.poster_url ? (
          <img
            src={item.poster_url}
            alt={item.title}
            className="w-full h-full object-cover transition-transform duration-300 group-hover:scale-105"
            loading="lazy"
          />
        ) : (
          <div className="w-full h-full flex items-center justify-center text-4xl"
            style={{ background: 'var(--color-surface)' }}>
            {isTV ? '📺' : '🎬'}
          </div>
        )}

        {/* 评分徽章 */}
        {item.rating > 0 && (
          <div className="absolute top-2 right-2 text-xs font-bold px-1.5 py-0.5 rounded"
            style={{ background: 'rgba(0,0,0,0.75)', color: 'var(--color-warning)' }}>
            ★ {item.rating}
          </div>
        )}

        {/* 类型徽章（左上） */}
        <div className="absolute top-2 left-2 text-xs px-1.5 py-0.5 rounded font-medium"
          style={{ background: 'rgba(0,0,0,0.65)', color: 'var(--color-muted)' }}>
          {isTV ? 'TV' : '电影'}
        </div>

        {/* TV 进度环 */}
        {isTV && pct !== null && (
          <div className="absolute bottom-2 right-2 flex items-center gap-1"
            style={{ background: 'rgba(0,0,0,0.75)', borderRadius: 20, padding: '2px 5px' }}>
            <RingProgress value={item.in_library_episodes} max={item.total_episodes} />
            <span className="text-xs font-semibold" style={{ color: 'var(--color-text)' }}>{pct}%</span>
          </div>
        )}

        {/* hover 渐变遮罩 */}
        <div className="absolute inset-0 opacity-0 group-hover:opacity-100 transition-opacity duration-200"
          style={{ background: 'linear-gradient(to top, rgba(0,0,0,0.5) 0%, transparent 50%)' }} />
      </div>

      {/* 信息区 ── 固定高度，保证网格对齐 */}
      <div
        className="px-3 pt-2.5 pb-2.5 flex flex-col"
        style={{ height: compact ? 58 : 80, overflow: 'hidden' }}
      >
        <p
          className={`text-sm font-semibold leading-snug ${compact ? 'line-clamp-1' : 'line-clamp-2'}`}
          style={{ color: 'var(--color-text)' }}
        >
          {item.title}
        </p>
        <div className="flex items-center gap-1.5 mt-auto flex-nowrap">
          {item.year && (
            <span className="text-xs shrink-0" style={{ color: 'var(--color-muted)' }}>{item.year}</span>
          )}
          {item.year && !compact && isTV && (
            <span className="text-xs shrink-0" style={{ color: 'var(--color-border)' }}>·</span>
          )}
          {!compact && isTV && (
            <span className="text-xs truncate" style={{ color: 'var(--color-muted)' }}>
              已入库 {item.in_library_episodes}/{item.total_episodes} 集
            </span>
          )}
        </div>
      </div>
    </div>
  )
}
