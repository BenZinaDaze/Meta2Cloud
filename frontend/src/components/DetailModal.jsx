import { useCallback, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'

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
  const episodeCount = season.episode_count || season.episodes?.length || 0
  const inLibraryCount = season.in_library_count ?? 0
  const pct = episodeCount > 0
    ? Math.round(inLibraryCount / episodeCount * 100) : 0
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
              {inLibraryCount} / {episodeCount} 集 · {pct}%
            </span>
          </div>
          <div className="h-1.5 rounded-full overflow-hidden" style={{ background: 'var(--color-border)' }}>
            <div className="h-full rounded-full transition-all duration-500"
              style={{ width: `${pct}%`, background: barColor }} />
          </div>
        </div>
      </div>
      <div className="flex flex-wrap gap-1.5">
        {(season.episodes || []).map(ep => (
          <EpisodePill key={ep.episode_number} ep={ep} />
        ))}
      </div>
    </div>
  )
}

const STATUS_MAP = {
  'Returning Series': '连载中',
  'Ended': '已完结',
  'Canceled': '已取消',
  'In Production': '制作中',
  'Planned': '计划中',
  'Pilot': '试播',
  'In Limbo': '播出未定',
}

function formatStatus(status) {
  if (!status) return status
  return STATUS_MAP[status] ?? status
}

export default function DetailModal({ item, onClose, contentSlot, footerSlot, loadingSlot, headerRightSlot, titleActionSlot }) {
  const [show, setShow] = useState(false)
  const [showOverview, setShowOverview] = useState(false)
  const hasFooter = !!footerSlot
  const headerRef = useRef(null)
  const [contentMaxHeight, setContentMaxHeight] = useState(null)

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

  useEffect(() => {
    const { body, documentElement } = document
    const prevBodyOverflow = body.style.overflow
    const prevHtmlOverflow = documentElement.style.overflow

    body.style.overflow = 'hidden'
    documentElement.style.overflow = 'hidden'

    return () => {
      body.style.overflow = prevBodyOverflow
      documentElement.style.overflow = prevHtmlOverflow
    }
  }, [])

  useEffect(() => {
    if (hasFooter || !headerRef.current) return

    const headerEl = headerRef.current

    const updateContentMaxHeight = () => {
      const headerHeight = headerEl.getBoundingClientRect().height
      const desktopTopGap = 120
      const mobileBottomGap = 12
      const extraSpacing = 24
      const viewportAllowance = window.innerWidth >= 640
        ? `100dvh - ${desktopTopGap}px`
        : `100dvh - env(safe-area-inset-top) - ${mobileBottomGap}px`

      setContentMaxHeight(`calc(${viewportAllowance} - ${Math.ceil(headerHeight + extraSpacing)}px)`)
    }

    updateContentMaxHeight()

    const resizeObserver = new ResizeObserver(updateContentMaxHeight)
    resizeObserver.observe(headerEl)
    window.addEventListener('resize', updateContentMaxHeight)

    return () => {
      resizeObserver.disconnect()
      window.removeEventListener('resize', updateContentMaxHeight)
    }
  }, [hasFooter, item?.title, item?.overview, item?.poster_url, item?.backdrop_url, headerRightSlot, titleActionSlot, loadingSlot])

  if (!item) return null
  const isTV = item.media_type === 'tv'
  const summaryPairs = [
    item.year ? ['年份', item.year] : null,
    item.genre_names?.length ? ['类型', item.genre_names.slice(0, 2).join(' / ')] : null,
    item.original_language ? ['语言', String(item.original_language).toUpperCase()] : null,
    isTV && item.total_episodes ? ['总集数', `${item.total_episodes} 集`] : null,
    !isTV && item.runtime ? ['时长', `${item.runtime} 分钟`] : null,
  ].filter(Boolean)

  return createPortal(
    <div
      className="fixed inset-0 z-[100] flex items-end justify-center overflow-hidden p-0 sm:items-start sm:p-4 sm:pt-28"
      style={{
        background: 'rgba(2, 8, 18, 0.78)',
        backdropFilter: 'blur(10px)',
        opacity: show ? 1 : 0,
        transition: 'opacity 0.2s',
      }}
      onClick={(e) => e.target === e.currentTarget && handleClose()}
    >
      <div
        className={`relative w-full max-w-3xl overflow-hidden rounded-t-[28px] sm:rounded-[30px] ${
          hasFooter
            ? 'grid h-[calc(100dvh-env(safe-area-inset-top)-0.5rem)] grid-rows-[auto_minmax(0,1fr)_auto] sm:h-[calc(100dvh-7.5rem)]'
            : 'flex max-h-[calc(100dvh-env(safe-area-inset-top)-0.75rem)] flex-col sm:max-h-[calc(100dvh-7.5rem)]'
        }`}
        style={{
          background: 'linear-gradient(180deg, rgba(15, 27, 45, 0.98) 0%, rgba(11, 22, 37, 0.98) 100%)',
          border: '1px solid var(--color-border)',
          transform: show ? 'translateY(0)' : 'translateY(20px)',
          transition: 'transform 0.2s',
          boxShadow: 'var(--shadow-strong)',
        }}
        onClick={e => e.stopPropagation()}
      >
        <div ref={headerRef} className="min-h-0">
          <div
            className="sticky top-0 z-20 border-b px-4 pb-3 pt-[calc(env(safe-area-inset-top)+0.75rem)] sm:hidden"
            style={{
              borderColor: 'rgba(144, 178, 221, 0.12)',
              background: 'linear-gradient(180deg, rgba(15, 27, 45, 0.99) 0%, rgba(11, 22, 37, 0.98) 100%)',
              backdropFilter: 'blur(12px)',
              WebkitBackdropFilter: 'blur(12px)',
            }}
          >
            <div className="mx-auto mb-3 h-1.5 w-12 rounded-full" style={{ background: 'rgba(255,255,255,0.14)' }} />
            <div className="flex items-center justify-between gap-3">
              <div className="min-w-0">
                <div className="text-[11px] font-semibold uppercase tracking-[0.2em]" style={{ color: 'var(--color-accent-hover)' }}>
                  Metadata detail
                </div>
                <div className="mt-1 truncate text-base font-semibold" style={{ color: 'var(--color-text)' }}>
                  {item.title}
                </div>
              </div>
              <button
                onClick={handleClose}
                className="flex size-10 flex-shrink-0 items-center justify-center rounded-full text-sm transition-colors hover:bg-black/40"
                style={{ background: 'rgba(0,0,0,0.45)', color: 'var(--color-text)', border: '1px solid rgba(255,255,255,0.08)' }}
                aria-label="关闭详情"
              >
                ✕
              </button>
            </div>
          </div>

          <div className="relative h-[108px] overflow-hidden sm:h-40">
            {item.backdrop_url
              ? <img src={item.backdrop_url} alt="" className="absolute inset-0 h-full w-full object-cover transition-opacity duration-300" style={{ opacity: loadingSlot ? 0.3 : 1 }} />
              : <div className="absolute inset-0" style={{ background: 'var(--color-surface-2)' }} />
            }
            <div className="absolute inset-0"
              style={{ background: 'linear-gradient(to bottom, rgba(0,0,0,0.08) 10%, rgba(7, 17, 31, 0.88) 100%)' }} />
          </div>

          <div
            className="flex flex-col gap-3 px-4 pt-3 transition-opacity duration-300 sm:flex-row sm:items-end sm:gap-4 sm:px-6 sm:pt-4"
            style={{ position: 'relative', zIndex: 10, opacity: loadingSlot ? 0.5 : 1 }}
          >
            {item.poster_url && (
              <img
                src={item.poster_url}
                alt={item.title}
                className="h-[112px] w-[76px] flex-shrink-0 rounded-lg shadow-xl sm:h-[132px] sm:w-[88px]"
                style={{
                  objectFit: 'cover',
                  border: '2px solid rgba(255,255,255,0.08)',
                  display: 'block',
                }}
              />
            )}

            <div className="min-w-0 flex-1 pb-2 sm:pb-3 sm:pr-4">
              <div className="mb-2 hidden text-[11px] font-semibold uppercase tracking-[0.22em] sm:block" style={{ color: 'var(--color-accent-hover)' }}>
                Metadata detail
              </div>
              <div className="mb-1">
                <h2 className="text-[18px] font-bold leading-snug sm:text-[28px]" style={{ color: 'var(--color-text)' }}>
                  {item.title}
                </h2>
              </div>
              {item.original_title && item.original_title !== item.title && (
                <p className="mb-2 text-[13px] leading-snug sm:mb-3 sm:text-sm" style={{ color: 'var(--color-muted)' }}>
                  {item.original_title}
                </p>
              )}
              <div className="mt-2 flex flex-wrap gap-1.5">
                {item.year && (
                  <span className="rounded-full px-2 py-0.5 text-xs"
                    style={{ background: 'rgba(255,255,255,0.04)', color: 'var(--color-muted)', border: '1px solid var(--color-border)' }}>
                    {item.year}
                  </span>
                )}
                {item.rating > 0 && (
                  <span className="rounded-full px-2 py-0.5 text-xs"
                    style={{ background: 'rgba(255,255,255,0.04)', color: 'var(--color-warning)', border: '1px solid var(--color-border)' }}>
                    ★ {item.rating}
                  </span>
                )}
                {item.status && (
                  <span className="rounded-full px-2 py-0.5 text-xs"
                    style={{ background: 'rgba(255,255,255,0.04)', color: 'var(--color-accent-hover)', border: '1px solid var(--color-border)' }}>
                    {formatStatus(item.status)}
                  </span>
                )}
                {isTV && item.in_library_episodes !== undefined && item.in_library && (
                  <span className="rounded-full px-2 py-0.5 text-xs"
                    style={{ background: 'rgba(74,222,128,0.15)', color: 'var(--color-success)', border: '1px solid var(--color-success)' }}>
                    {item.in_library_episodes}/{item.total_episodes || '?'} 集已入库
                  </span>
                )}
                {!isTV && item.in_library && (
                  <span className="rounded-full px-2 py-0.5 text-xs"
                    style={{ background: 'rgba(255,255,255,0.04)', color: 'var(--color-success)', border: '1px solid var(--color-border)' }}>
                    已入库
                  </span>
                )}
                {item.tmdb_id && (
                  <div className="flex items-center gap-1.5">
                    <a
                      href={`https://www.themoviedb.org/${isTV ? 'tv' : 'movie'}/${item.tmdb_id}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="rounded-full px-2 py-0.5 text-xs transition-all duration-150"
                      style={{
                        background: 'rgba(1, 180, 228, 0.08)',
                        color: '#01b4e4',
                        border: '1px solid rgba(1, 180, 228, 0.35)',
                        textDecoration: 'none',
                        cursor: 'pointer',
                      }}
                      onMouseEnter={e => {
                        e.currentTarget.style.background = 'rgba(1, 180, 228, 0.18)'
                        e.currentTarget.style.borderColor = 'rgba(1, 180, 228, 0.65)'
                      }}
                      onMouseLeave={e => {
                        e.currentTarget.style.background = 'rgba(1, 180, 228, 0.08)'
                        e.currentTarget.style.borderColor = 'rgba(1, 180, 228, 0.35)'
                      }}
                    >
                      TMDB {item.tmdb_id}
                    </a>
                    {titleActionSlot}
                  </div>
                )}
              </div>
              {headerRightSlot && (
                <div className="mt-2 flex items-center gap-2 sm:hidden">
                  {headerRightSlot}
                </div>
              )}
            </div>

            {headerRightSlot && (
              <div className="hidden flex-shrink-0 self-start pb-3 sm:block sm:self-center sm:pr-2">
                {headerRightSlot}
              </div>
            )}
          </div>
        </div>

        <div
          className={`relative mt-2 px-4 pb-4 sm:mt-4 sm:px-6 sm:pb-6 ${
            hasFooter ? 'min-h-0 flex-1 overflow-y-auto' : 'overflow-y-auto'
          }`}
          style={{
            paddingBottom: 'calc(env(safe-area-inset-bottom) + 1rem)',
            overscrollBehavior: 'contain',
            WebkitOverflowScrolling: 'touch',
            maxHeight: hasFooter ? undefined : contentMaxHeight ?? 'calc(100dvh - env(safe-area-inset-top) - 18rem)',
          }}
        >
          {loadingSlot && (
            <div className="absolute inset-0 z-20 flex items-center justify-center bg-[#0B1625]/50 backdrop-blur-sm">
               <span className="text-white/40 text-sm animate-pulse flex items-center gap-2">
                 <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="animate-spin"><circle cx="12" cy="12" r="10"/><path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83"/></svg>
                 加载详情中...
               </span>
            </div>
          )}

          {summaryPairs.length > 0 && (
            <div className="mb-3 grid grid-cols-2 gap-2 sm:hidden">
              {summaryPairs.map(([label, value]) => (
                <div
                  key={label}
                  className="rounded-2xl px-3 py-2.5"
                  style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.05)' }}
                >
                  <div className="text-[11px] font-semibold uppercase tracking-[0.18em]" style={{ color: 'var(--color-muted)' }}>
                    {label}
                  </div>
                  <div className="mt-1 text-[13px] font-medium leading-5" style={{ color: 'var(--color-text)' }}>
                    {value}
                  </div>
                </div>
              ))}
            </div>
          )}

          {item.overview && (
            <>
              <div
                className="mb-4 rounded-[22px] px-4 py-3.5 sm:hidden"
                style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.05)' }}
              >
                <div className="text-[11px] font-semibold uppercase tracking-[0.18em]" style={{ color: 'var(--color-muted)' }}>
                  内容简介
                </div>
                <p
                  className="mt-2 text-[13px] leading-6"
                  style={{
                    color: 'var(--color-muted)',
                    display: '-webkit-box',
                    WebkitLineClamp: showOverview ? 'unset' : 4,
                    WebkitBoxOrient: 'vertical',
                    overflow: 'hidden',
                  }}
                >
                  {item.overview}
                </p>
                {item.overview.length > 120 && (
                  <button
                    type="button"
                    onClick={() => setShowOverview((value) => !value)}
                    className="mt-2 text-xs font-semibold"
                    style={{ color: 'var(--color-accent-hover)' }}
                  >
                    {showOverview ? '收起简介' : '展开简介'}
                  </button>
                )}
              </div>
              <p className="mb-5 hidden text-sm leading-7 sm:block" style={{ color: 'var(--color-muted)' }}>
                {item.overview}
              </p>
            </>
          )}

          {isTV && item.seasons && item.seasons.length > 0 ? (
            <div>
              <h3 className="text-sm font-semibold mb-3 pb-2 border-b"
                style={{ color: 'var(--color-text)', borderColor: 'var(--color-border)' }}>
                季集入库状态
              </h3>
              <div className="mb-3 flex flex-wrap gap-3 text-xs" style={{ color: 'var(--color-muted)' }}>
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
              {(item.seasons || []).map(season => (
                <SeasonBlock key={season.season_number} season={season} />
              ))}
            </div>
          ) : isTV ? (
            <div className="text-sm" style={{ color: 'var(--color-muted)' }}>
              暂无季集信息
            </div>
          ) : null}

          {contentSlot ? (
            <div className="mt-5">
              {contentSlot}
            </div>
          ) : null}

        </div>

        {hasFooter && (
          <div
            className="border-t px-4 pb-4 pt-3 sm:px-6 sm:pb-6"
            style={{
              borderColor: 'rgba(144, 178, 221, 0.12)',
              background: 'linear-gradient(180deg, rgba(15, 27, 45, 0.96) 0%, rgba(11, 22, 37, 0.99) 100%)',
            }}
          >
            {footerSlot}
          </div>
        )}

        <button
          onClick={handleClose}
          className="absolute right-4 top-4 hidden size-9 items-center justify-center rounded-full text-sm transition-colors hover:bg-black/40 z-50 sm:flex"
          style={{ background: 'rgba(0,0,0,0.45)', color: 'var(--color-text)', border: '1px solid rgba(255,255,255,0.08)' }}
        >
          ✕
        </button>
      </div>
    </div>,
    document.body
  )
}
