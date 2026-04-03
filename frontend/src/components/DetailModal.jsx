import { useEffect, useState } from 'react'

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

  useEffect(() => {
    requestAnimationFrame(() => setShow(true))
    const onKey = (e) => e.key === 'Escape' && handleClose()
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  function handleClose() {
    setShow(false)
    setTimeout(onClose, 200)
  }

  if (!item) return null
  const isTV = item.media_type === 'tv'

  // 海报高度估算（2:3 比例，宽度 w-24 = 96px）
  const POSTER_W = 96
  const POSTER_H = 144  // 2:3 ratio
  const BACKDROP_H = 160 // h-40

  return (
    <div
      className="fixed inset-0 z-[100] flex items-start justify-center p-4 pt-16 overflow-y-auto"
      style={{
        background: 'rgba(0,0,0,0.8)',
        backdropFilter: 'blur(4px)',
        opacity: show ? 1 : 0,
        transition: 'opacity 0.2s',
      }}
      onClick={(e) => e.target === e.currentTarget && handleClose()}
    >
      <div
        className="relative w-full max-w-3xl rounded-2xl shadow-2xl"
        style={{
          background: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
          transform: show ? 'translateY(0)' : 'translateY(20px)',
          transition: 'transform 0.2s',
          overflow: 'hidden',  /* keep rounded corners */
        }}
      >
        {/* ── 1: Backdrop ── */}
        <div className="relative overflow-hidden" style={{ height: BACKDROP_H }}>
          {item.backdrop_url
            ? <img src={item.backdrop_url} alt="" className="absolute inset-0 w-full h-full object-cover" />
            : <div className="absolute inset-0" style={{ background: 'var(--color-surface-2)' }} />
          }
          {/* 渐变 */}
          <div className="absolute inset-0"
            style={{ background: 'linear-gradient(to bottom, rgba(0,0,0,0.1) 40%, var(--color-surface) 100%)' }} />
        </div>

        {/* ── 2: 海报 + 标题行 ──
              flex 行加 relative z-10，使其作为定位元素绘制在 backdrop（position:relative）之上
              左栏 = 海报（负 margin 上穿 backdrop）
              右栏 = 标题（始终在 backdrop 下方，不会被裁）                              */}
        <div className="flex gap-4 px-6 relative items-end" style={{ zIndex: 10 }}>
          {/* 左栏：海报 */}
          <div className="flex-shrink-0" style={{ width: POSTER_W }}>
            {item.poster_url && (
              <img
                src={item.poster_url}
                alt={item.title}
                className="rounded-lg shadow-xl w-full"
                style={{
                  marginTop: -(POSTER_H / 2 + 16),
                  border: '2px solid var(--color-border)',
                  display: 'block',
                }}
              />
            )}
          </div>

          {/* 右栏：标题区 — pb-3 底部留白与海报底部对齐，pr-10 给关闭按钮留空间 */}
          <div className="flex-1 min-w-0 pb-3 pr-10">
            <h2 className="text-lg font-bold leading-snug mb-1" style={{ color: 'var(--color-text)' }}>
              {item.title}
            </h2>
            {item.original_title && item.original_title !== item.title && (
              <p className="text-sm mb-2 leading-snug" style={{ color: 'var(--color-muted)' }}>
                {item.original_title}
              </p>
            )}
            <div className="flex flex-wrap gap-1.5">
              {item.year && (
                <span className="text-xs px-2 py-0.5 rounded-full"
                  style={{ background: 'var(--color-surface-2)', color: 'var(--color-muted)' }}>
                  {item.year}
                </span>
              )}
              {item.rating > 0 && (
                <span className="text-xs px-2 py-0.5 rounded-full"
                  style={{ background: 'var(--color-surface-2)', color: 'var(--color-warning)' }}>
                  ★ {item.rating}
                </span>
              )}
              {item.status && (
                <span className="text-xs px-2 py-0.5 rounded-full"
                  style={{ background: 'var(--color-surface-2)', color: 'var(--color-accent)' }}>
                  {item.status}
                </span>
              )}
              {isTV && (
                <span className="text-xs px-2 py-0.5 rounded-full"
                  style={{ background: 'var(--color-surface-2)', color: 'var(--color-text)' }}>
                  {item.in_library_episodes}/{item.total_episodes} 集已入库
                </span>
              )}
            </div>
          </div>
        </div>

        {/* ── 3: 主体内容 ── */}
        <div className="px-6 pb-6 mt-2">
          {item.overview && (
            <p className="text-sm leading-relaxed mb-4" style={{ color: 'var(--color-muted)' }}>
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

        {/* 关闭按钮 */}
        <button
          onClick={handleClose}
          className="absolute top-3 right-3 w-8 h-8 rounded-full flex items-center justify-center text-sm transition-colors hover:bg-black/40"
          style={{ background: 'rgba(0,0,0,0.45)', color: 'var(--color-text)' }}
        >
          ✕
        </button>
      </div>
    </div>
  )
}
