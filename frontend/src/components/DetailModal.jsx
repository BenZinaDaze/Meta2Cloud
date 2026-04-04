import { useCallback, useEffect, useState } from 'react'

function EpisodePill({ ep }) {
  const color = ep.in_library
    ? 'var(--color-success)'
    : ep.air_date && new Date(ep.air_date) > new Date()
    ? 'var(--color-muted)'
    : 'var(--color-danger)'

  return (
    <div
      title={`E${String(ep.episode_number).padStart(2, '0')} ${ep.episode_title}${ep.air_date ? ' · ' + ep.air_date : ''}`}
      className="w-8 h-8 rounded flex items-center justify-center text-xs font-bold cursor-default transition-transform hover:scale-110"
      style={{
        background: ep.in_library ? 'rgba(74,222,128,0.15)' : 'var(--color-surface)',
        border: `1.5px solid ${color}`,
        color,
      }}
    >
      {ep.episode_number}
    </div>
  )
}

function SeasonBlock({ season }) {
  const pct = season.episode_count > 0
    ? Math.round(season.in_library_count / season.episode_count * 100) : 0
  const barColor = pct >= 100 ? 'var(--color-success)' : pct > 50 ? 'var(--color-accent)' : 'var(--color-warning)'

  return (
    <div className="mb-6">
      <div className="flex items-center gap-3 mb-3">
        {season.poster_url && (
          <img src={season.poster_url} alt={season.season_name}
            className="rounded object-cover flex-shrink-0" style={{ width: 40, height: 60 }} />
        )}
        <div className="flex-1 min-w-0">
          <div className="flex items-center justify-between mb-1">
            <span className="text-sm font-semibold" style={{ color: 'var(--color-text)' }}>
              {season.season_name}
            </span>
            <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
              {season.in_library_count} / {season.episode_count} 集 · {pct}%
            </span>
          </div>
          <div className="h-1.5 rounded-full overflow-hidden" style={{ background: 'var(--color-border)' }}>
            <div className="h-full rounded-full transition-all duration-500"
              style={{ width: `${pct}%`, background: barColor }} />
          </div>
        </div>
      </div>
      <div className="flex flex-wrap gap-1.5">
        {season.episodes.map(ep => (
          <EpisodePill key={ep.episode_number} ep={ep} />
        ))}
      </div>
    </div>
  )
}

export default function DetailModal({ item, onClose }) {
  const [show, setShow] = useState(false)

  const handleClose = useCallback(() => {
    setShow(false)
    setTimeout(onClose, 200)
  }, [onClose])

  useEffect(() => {
    requestAnimationFrame(() => setShow(true))
    const onKey = (e) => e.key === 'Escape' && handleClose()
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [handleClose])

  if (!item) return null
  const isTV = item.media_type === 'tv'

  // 海报高度估算（2:3 比例，宽度 w-24 = 96px）
  const POSTER_W = 96
  const POSTER_H = 144  // 2:3 ratio
  const BACKDROP_H = 160 // h-40

  return (
    <div
      className="fixed inset-0 z-[100] flex items-start justify-center overflow-y-auto p-4 pt-20"
      style={{
        background: 'rgba(2, 8, 18, 0.78)',
        backdropFilter: 'blur(10px)',
        opacity: show ? 1 : 0,
        transition: 'opacity 0.2s',
      }}
      onClick={(e) => e.target === e.currentTarget && handleClose()}
    >
      <div
        className="relative w-full max-w-3xl overflow-hidden rounded-[30px]"
        style={{
          background: 'linear-gradient(180deg, rgba(15, 27, 45, 0.98) 0%, rgba(11, 22, 37, 0.98) 100%)',
          border: '1px solid var(--color-border)',
          transform: show ? 'translateY(0)' : 'translateY(20px)',
          transition: 'transform 0.2s',
          boxShadow: 'var(--shadow-strong)',
        }}
      >
        <div className="relative overflow-hidden" style={{ height: BACKDROP_H }}>
          {item.backdrop_url
            ? <img src={item.backdrop_url} alt="" className="absolute inset-0 w-full h-full object-cover" />
            : <div className="absolute inset-0" style={{ background: 'var(--color-surface-2)' }} />
          }
          <div className="absolute inset-0"
            style={{ background: 'linear-gradient(to bottom, rgba(0,0,0,0.08) 10%, rgba(7, 17, 31, 0.88) 100%)' }} />
        </div>

        <div className="flex gap-4 px-6 relative items-end" style={{ zIndex: 10 }}>
          <div className="flex-shrink-0" style={{ width: POSTER_W }}>
            {item.poster_url && (
              <img
                src={item.poster_url}
                alt={item.title}
                className="rounded-lg shadow-xl w-full"
                style={{
                  marginTop: -(POSTER_H / 2 + 16),
                  border: '2px solid rgba(255,255,255,0.08)',
                  display: 'block',
                }}
              />
            )}
          </div>

          <div className="flex-1 min-w-0 pb-3 pr-10">
            <div className="mb-2 text-[11px] font-semibold uppercase tracking-[0.22em]" style={{ color: 'var(--color-accent-hover)' }}>
              Metadata detail
            </div>
            <h2 className="mb-1 text-[28px] font-bold leading-snug" style={{ color: 'var(--color-text)' }}>
              {item.title}
            </h2>
            {item.original_title && item.original_title !== item.title && (
              <p className="mb-3 text-sm leading-snug" style={{ color: 'var(--color-muted)' }}>
                {item.original_title}
              </p>
            )}
            <div className="flex flex-wrap gap-1.5">
              {item.year && (
                <span className="text-xs px-2 py-0.5 rounded-full"
                  style={{ background: 'rgba(255,255,255,0.04)', color: 'var(--color-muted)', border: '1px solid var(--color-border)' }}>
                  {item.year}
                </span>
              )}
              {item.rating > 0 && (
                <span className="text-xs px-2 py-0.5 rounded-full"
                  style={{ background: 'rgba(255,255,255,0.04)', color: 'var(--color-warning)', border: '1px solid var(--color-border)' }}>
                  ★ {item.rating}
                </span>
              )}
              {item.status && (
                <span className="text-xs px-2 py-0.5 rounded-full"
                  style={{ background: 'rgba(255,255,255,0.04)', color: 'var(--color-accent-hover)', border: '1px solid var(--color-border)' }}>
                  {item.status}
                </span>
              )}
              {isTV && (
                <span className="text-xs px-2 py-0.5 rounded-full"
                  style={{ background: 'rgba(255,255,255,0.04)', color: 'var(--color-text)', border: '1px solid var(--color-border)' }}>
                  {item.in_library_episodes}/{item.total_episodes} 集已入库
                </span>
              )}
            </div>
          </div>
        </div>

        <div className="px-6 pb-6 mt-2">
          {item.overview && (
            <p className="mb-5 text-sm leading-7" style={{ color: 'var(--color-muted)' }}>
              {item.overview}
            </p>
          )}

          {isTV && item.seasons && item.seasons.length > 0 && (
            <div>
              <h3 className="text-sm font-semibold mb-3 pb-2 border-b"
                style={{ color: 'var(--color-text)', borderColor: 'var(--color-border)' }}>
                季集入库状态
              </h3>
              <div className="flex gap-4 mb-4 text-xs" style={{ color: 'var(--color-muted)' }}>
                {[
                  { style: { border: '1.5px solid var(--color-success)', background: 'rgba(74,222,128,0.15)' }, label: '已入库' },
                  { style: { border: '1.5px solid var(--color-danger)', background: 'var(--color-surface)' }, label: '未入库' },
                  { style: { border: '1.5px solid var(--color-muted)', background: 'var(--color-surface)' }, label: '未播出' },
                ].map(({ style, label }) => (
                  <span key={label} className="flex items-center gap-1">
                    <span className="w-3 h-3 rounded-sm inline-block" style={style} /> {label}
                  </span>
                ))}
              </div>
              {item.seasons.map(season => (
                <SeasonBlock key={season.season_number} season={season} />
              ))}
            </div>
          )}
        </div>

        <button
          onClick={handleClose}
          className="absolute right-4 top-4 flex size-9 items-center justify-center rounded-full text-sm transition-colors hover:bg-black/40"
          style={{ background: 'rgba(0,0,0,0.45)', color: 'var(--color-text)', border: '1px solid rgba(255,255,255,0.08)' }}
        >
          ✕
        </button>
      </div>
    </div>
  )
}
