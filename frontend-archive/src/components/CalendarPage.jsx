import { useState, useEffect, useRef } from 'react'
import { StatePanel } from './StatePanel'

const WEEKDAY_ORDER = [1, 2, 3, 4, 5, 6, 7] // Mon → Sun

function normalizeBangumiUrl(url) {
  if (typeof url !== 'string') return url
  if (url.startsWith('http://')) {
    return `https://${url.slice('http://'.length)}`
  }
  return url
}

function normalizeCalendarPayload(payload) {
  if (!Array.isArray(payload)) return []
  return payload.map(day => ({
    ...day,
    items: Array.isArray(day?.items)
      ? day.items.map(item => ({
          ...item,
          images: item?.images
            ? {
                ...item.images,
                large: normalizeBangumiUrl(item.images.large),
                common: normalizeBangumiUrl(item.images.common),
                medium: normalizeBangumiUrl(item.images.medium),
                small: normalizeBangumiUrl(item.images.small),
                grid: normalizeBangumiUrl(item.images.grid),
              }
            : item?.images,
        }))
      : day?.items,
  }))
}

function normalizeAndSortCalendarData(json) {
  const normalized = normalizeCalendarPayload(json)
  return [...normalized].sort((a, b) => a.weekday.id - b.weekday.id)
}

async function fetchCalendar(options = {}) {
  const res = await fetch('https://api.bgm.tv/calendar', {
    headers: { 'User-Agent': 'Meta2Cloud/1.0 (https://github.com/BenZinaDaze/Meta2Cloud)' },
    ...options,
  })
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  const json = await res.json()
  return normalizeAndSortCalendarData(json)
}

function StarIcon({ fill = false }) {
  return (
    <svg width="12" height="12" viewBox="0 0 24 24" fill={fill ? 'currentColor' : 'none'} stroke="currentColor" strokeWidth="2">
      <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
    </svg>
  )
}

// ── 动画卡片 ─────────────────────────────────────────────────
function AnimeCard({ item, onSearch }) {
  const cover = item.images?.large || item.images?.common || item.images?.medium
  const title = item.name_cn || item.name
  const score = item.rating?.score

  return (
    <div
      onClick={() => onSearch(item)}
      className="group relative flex flex-col overflow-hidden rounded-[16px] transition-all duration-200"
      style={{
        background: 'rgba(15, 27, 45, 0.7)',
        border: '1px solid rgba(144, 178, 221, 0.10)',
        cursor: 'pointer',
      }}
      title={title}
    >
      {/* Hover overlay */}
      <span
        className="absolute inset-0 z-10 rounded-[16px] opacity-0 transition-opacity duration-200 group-hover:opacity-100"
        style={{ background: 'rgba(200, 146, 77, 0.06)', boxShadow: 'inset 0 0 0 1px rgba(200, 146, 77, 0.25)' }}
      />

      {/* Cover */}
      <div className="relative overflow-hidden" style={{ aspectRatio: '2/3', background: 'rgba(255,255,255,0.03)' }}>
        {cover ? (
          <img
            src={cover}
            alt={title}
            loading="lazy"
            className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
            style={{ display: 'block' }}
            onError={e => { e.currentTarget.style.display = 'none' }}
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center" style={{ color: 'rgba(255,255,255,0.15)' }}>
            <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
              <rect x="2" y="7" width="20" height="15" rx="2" /><polyline points="17 2 12 7 7 2" />
            </svg>
          </div>
        )}
        {/* Score badge */}
        {score > 0 && (
          <div
            className="absolute bottom-2 left-2 flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-bold"
            style={{ background: 'rgba(7,14,24,0.82)', backdropFilter: 'blur(6px)', color: score >= 8 ? '#fbbf24' : score >= 7 ? 'var(--color-accent-hover)' : 'var(--color-muted)' }}
          >
            <span><StarIcon fill /></span>
            {score.toFixed(1)}
          </div>
        )}
      </div>

      {/* Info */}
      <div className="flex flex-col gap-1 px-3 py-2.5">
        <p
          className="line-clamp-2 text-[13px] font-semibold leading-snug"
          style={{ color: 'var(--color-text)' }}
        >
          {title}
        </p>
      </div>
    </div>
  )
}

function WeekdaySection({ weekday, items, isToday, onSearch }) {
  const [expanded, setExpanded] = useState(true)
  const sectionRef = useRef(null)

  useEffect(() => {
    if (isToday && sectionRef.current) {
      setTimeout(() => {
        sectionRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
      }, 300)
    }
  }, [isToday])

  if (!items || items.length === 0) return null

  return (
    <section ref={sectionRef} className="mb-8">
      {/* Section header */}
      <div
        onClick={() => setExpanded(v => !v)}
        className="mb-4 flex items-center gap-3 select-none"
        style={{ cursor: 'pointer' }}
      >
        {isToday && (
          <span
            className="flex h-5 items-center rounded-full px-2 text-[10px] font-bold uppercase tracking-wide flex-shrink-0"
            style={{ background: 'var(--color-accent)', color: 'var(--color-primary-text)' }}
          >
            今日
          </span>
        )}
        <span
          className="text-[22px] font-bold flex-shrink-0"
          style={{ color: isToday ? 'var(--color-accent-hover)' : 'var(--color-text)' }}
        >
          {weekday.cn}
        </span>
        <span className="text-[15px] flex-shrink-0" style={{ color: 'var(--color-muted)' }}>
          {weekday.en}
        </span>
        <span
          className="rounded-full px-2 py-0.5 text-[13px] font-semibold flex-shrink-0"
          style={{
            background: isToday ? 'rgba(200, 146, 77, 0.15)' : 'rgba(255,255,255,0.05)',
            color: isToday ? 'var(--color-accent-hover)' : 'var(--color-muted)',
          }}
        >
          {items.length}
        </span>
        {/* Divider line */}
        <span className="flex-1" style={{ height: 1, background: 'rgba(144, 178, 221, 0.10)' }} />
        <span
          style={{
            color: 'var(--color-muted)',
            transform: expanded ? 'rotate(180deg)' : 'rotate(0deg)',
            transition: 'transform 0.2s',
            display: 'flex',
          }}
        >
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <polyline points="6 9 12 15 18 9" />
          </svg>
        </span>
      </div>

      {/* Grid */}
      <div
        className="overflow-hidden transition-all duration-300 ease-in-out"
        style={{ maxHeight: expanded ? '9999px' : '0px', opacity: expanded ? 1 : 0 }}
      >
        <div
          className="grid gap-3"
          style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(160px, 1fr))' }}
        >
          {items.map(item => (
            <AnimeCard key={item.id} item={item} onSearch={onSearch} />
          ))}
        </div>
      </div>
    </section>
  )
}

function LazyWeekdaySection({ weekday, items, isToday, onSearch, eager = false }) {
  const [revealed, setRevealed] = useState(eager)
  const anchorRef = useRef(null)

  useEffect(() => {
    if (eager) return
    const node = anchorRef.current
    if (!node || revealed) return

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some(entry => entry.isIntersecting)) {
          setRevealed(true)
          observer.disconnect()
        }
      },
      { rootMargin: '900px 0px' }
    )
    observer.observe(node)
    return () => observer.disconnect()
  }, [eager, revealed])

  if (!items || items.length === 0) return null

  return (
    <div ref={anchorRef}>
      {revealed ? (
        <WeekdaySection weekday={weekday} items={items} isToday={isToday} onSearch={onSearch} />
      ) : (
        <section className="mb-8">
          <div className="mb-4 flex items-center gap-3">
            {isToday && (
              <span
                className="flex h-5 items-center rounded-full px-2 text-[10px] font-bold uppercase tracking-wide flex-shrink-0"
                style={{ background: 'var(--color-accent)', color: 'var(--color-primary-text)' }}
              >
                今日
              </span>
            )}
            <span className="text-[22px] font-bold flex-shrink-0" style={{ color: isToday ? 'var(--color-accent-hover)' : 'var(--color-text)' }}>
              {weekday.cn}
            </span>
            <span className="text-[15px] flex-shrink-0" style={{ color: 'var(--color-muted)' }}>
              {weekday.en}
            </span>
            <span
              className="rounded-full px-2 py-0.5 text-[13px] font-semibold flex-shrink-0"
              style={{
                background: isToday ? 'rgba(200, 146, 77, 0.15)' : 'rgba(255,255,255,0.05)',
                color: isToday ? 'var(--color-accent-hover)' : 'var(--color-muted)',
              }}
            >
              {items.length}
            </span>
            <span className="flex-1" style={{ height: 1, background: 'rgba(144, 178, 221, 0.10)' }} />
          </div>
          <div className="grid gap-3" style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(160px, 1fr))' }}>
            {Array.from({ length: Math.min(items.length, 6) }, (_, index) => (
              <div
                key={`${weekday.id}-placeholder-${index}`}
                className="animate-pulse rounded-[16px] overflow-hidden"
                style={{ background: 'rgba(255,255,255,0.04)', aspectRatio: '2/3' }}
              />
            ))}
          </div>
        </section>
      )}
    </div>
  )
}

export default function CalendarPage({ onSearch }) {
  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [lastUpdated, setLastUpdated] = useState(null)

  // Bangumi weekday id: 1=Mon … 7=Sun
  // JS getDay(): 0=Sun, 1=Mon … 6=Sat
  // Map JS day → bgm id
  const jsDayToBgm = d => d === 0 ? 7 : d
  const todayBgmId = jsDayToBgm(new Date().getDay())
  const eagerWeekdays = new Set([
    todayBgmId,
    todayBgmId === 1 ? 7 : todayBgmId - 1,
    todayBgmId === 7 ? 1 : todayBgmId + 1,
  ])

  useEffect(() => {
    fetchCalendar()
      .then(sorted => {
        setData(sorted)
        setError(null)
        setLastUpdated(new Date())
        setLoading(false)
      })
      .catch(err => {
        setError(err.message || '加载失败')
        setLoading(false)
      })
  }, [])

  function handleRefresh() {
    setLoading(true)
    setError(null)
    fetchCalendar({ cache: 'no-store' })
      .then(sorted => {
        setData(sorted)
        setLastUpdated(new Date())
        setLoading(false)
      })
      .catch(err => {
        setError(err.message || '加载失败')
        setLoading(false)
      })
  }

  return (
    <div className="py-6">
      {/* Page header */}
      <div className="mb-8 flex items-start justify-between gap-4">
        <div>
          <div className="text-[10px] font-semibold uppercase tracking-[0.2em]" style={{ color: 'var(--color-muted-soft)' }}>
            Bangumi · 新番放送
          </div>
          <h1 className="mt-1 text-3xl font-bold" style={{ color: 'var(--color-text)' }}>
            新番列表
          </h1>
          {lastUpdated && (
            <p className="mt-1 text-[12px]" style={{ color: 'var(--color-muted)' }}>
              更新于 {lastUpdated.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })}
            </p>
          )}
        </div>
        <button
          onClick={handleRefresh}
          disabled={loading}
          className="flex items-center gap-2 rounded-[14px] px-4 py-2.5 text-[13px] font-semibold transition-all duration-150"
          style={{
            background: loading ? 'rgba(255,255,255,0.04)' : 'rgba(200, 146, 77, 0.14)',
            border: '1px solid rgba(200, 146, 77, 0.20)',
            color: loading ? 'var(--color-muted)' : 'var(--color-accent-hover)',
            cursor: loading ? 'not-allowed' : 'pointer',
          }}
        >
          <svg
            width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"
            className={loading ? 'animate-spin' : ''}
          >
            <polyline points="23 4 23 10 17 10" />
            <polyline points="1 20 1 14 7 14" />
            <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
          </svg>
          {loading ? '加载中…' : '刷新'}
        </button>
      </div>

      {/* Error state */}
      {error && !loading && (
        <div className="mb-6">
          <StatePanel
            icon="!"
            title={`加载失败：${error}`}
            description="请检查网络连接，或稍后刷新重试。"
            tone="danger"
            compact
          />
        </div>
      )}

      {/* Loading skeleton */}
      {loading && (
        <div className="space-y-8">
          {[1, 2, 3].map(i => (
            <div key={i}>
              <div className="mb-4 h-10 w-48 animate-pulse rounded-[12px]" style={{ background: 'rgba(255,255,255,0.05)' }} />
              <div className="grid gap-3" style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(160px, 1fr))' }}>
                {Array.from({ length: 8 }, (_, j) => (
                  <div key={j} className="animate-pulse rounded-[16px] overflow-hidden" style={{ background: 'rgba(255,255,255,0.04)', aspectRatio: '2/3' }} />
                ))}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Content */}
      {!loading && data && (
        <div>
          {data.map(({ weekday, items }) => (
            <LazyWeekdaySection
              key={weekday.id}
              weekday={weekday}
              items={items || []}
              isToday={weekday.id === todayBgmId}
              onSearch={onSearch}
              eager={eagerWeekdays.has(weekday.id)}
            />
          ))}
        </div>
      )}

      {/* Source attribution */}
      {!loading && data && (
        <div className="mt-4 text-center text-[11px]" style={{ color: 'var(--color-muted-soft)' }}>
          数据来源：
          <a href="https://bgm.tv" target="_blank" rel="noopener noreferrer" style={{ color: 'var(--color-accent)', textDecoration: 'none' }}>
            Bangumi 番组计划
          </a>
        </div>
      )}
    </div>
  )
}
