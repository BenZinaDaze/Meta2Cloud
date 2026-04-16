import { useState, useEffect, useMemo } from 'react'
import { getLibrary } from '../api'
import MediaCard from './MediaCard'
import ScraperDetailModal from './ScraperDetailModal'
import { StatePanel } from './StatePanel'

/** ISO 时间 → 「x 分钟前」 */
function relativeTime(isoStr) {
  if (!isoStr) return '从未刷新'
  const diff = Math.floor((Date.now() - new Date(isoStr).getTime()) / 1000)
  if (diff < 60)         return '刚刚'
  if (diff < 3600)       return `${Math.floor(diff / 60)} 分钟前`
  if (diff < 86400)      return `${Math.floor(diff / 3600)} 小时前`
  if (diff < 86400 * 30) return `${Math.floor(diff / 86400)} 天前`
  return new Date(isoStr).toLocaleDateString('zh-CN')
}

function StatCard({ label, value, sub, action }) {
  return (
    <div
      className="flex items-center justify-between gap-2 rounded-[20px] px-4 py-3 sm:rounded-[24px] sm:px-5 sm:py-5"
      style={{
        background: 'linear-gradient(180deg, rgba(20, 37, 59, 0.96) 0%, rgba(14, 28, 46, 0.98) 100%)',
        border: '1px solid var(--color-border)',
        boxShadow: 'var(--shadow-soft)',
      }}
    >
      <div className="min-w-0 flex-1 space-y-0.5">
        <span className="text-[10px] font-semibold uppercase tracking-[0.18em] sm:text-[11px]" style={{ color: 'var(--color-muted-soft)' }}>{label}</span>
        <span
          className={`block font-bold tabular-nums leading-tight ${
            action ? 'text-base sm:text-2xl' : 'text-xl sm:text-3xl'
          }`}
          style={{ color: 'var(--color-text)' }}
        >{value}</span>
        {sub && <span className="block text-[10px] leading-4 opacity-60" style={{ color: 'var(--color-muted)' }}>{sub}</span>}
      </div>
      {action && (
        <div className="ml-2 flex-shrink-0">{action}</div>
      )}
    </div>
  )
}

/** 横向滚动区块 */
function SectionRow({ title, count, items, onSelect, onViewAll }) {
  return (
    <section className="mb-10">
      <div className="mb-5 flex items-center gap-3">
        <h2 className="shrink-0 text-[22px] font-bold" style={{ color: 'var(--color-text)' }}>{title}</h2>
        <span className="shrink-0 rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.14em]"
          style={{ background: 'rgba(255,255,255,0.03)', color: 'var(--color-muted)' }}>
          {count}
        </span>
        <div className="h-px flex-1" style={{ background: 'linear-gradient(90deg, rgba(144, 178, 221, 0.2) 0%, rgba(144, 178, 221, 0.02) 100%)' }} />
        <button
          onClick={onViewAll}
          className="flex shrink-0 items-center gap-1 rounded-full px-3 py-1.5 text-xs font-semibold transition-all duration-150 hover:opacity-80"
          style={{ background: 'rgba(255,255,255,0.03)', color: 'var(--color-accent-hover)', border: '1px solid var(--color-border)' }}
        >
          查看全部
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
            <polyline points="9 18 15 12 9 6"/>
          </svg>
        </button>
      </div>

      <div
        className="flex gap-4 overflow-x-auto pb-3"
        style={{ scrollbarWidth: 'thin', scrollbarColor: 'var(--color-border) transparent' }}
      >
        {items.map(item => (
          <div
            key={`${item.media_type}-${item.tmdb_id || item.title}`}
            style={{ flex: '0 0 auto', width: 148 }}
          >
            <MediaCard item={item} onClick={onSelect} compact />
          </div>
        ))}
      </div>
    </section>
  )
}

/** 普通网格（单类型过滤时使用） */
function MediaGrid({ items, onSelect }) {
  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-8 gap-3">
      {items.map(item => (
        <MediaCard
          key={`${item.media_type}-${item.tmdb_id || item.title}`}
          item={item}
          onClick={onSelect}
        />
      ))}
    </div>
  )
}

function LoadingRow() {
  return (
    <div className="flex gap-3 overflow-hidden">
      {Array.from({ length: 8 }).map((_, i) => (
        <div key={i} style={{ flex: '0 0 auto', width: 148 }}
          className="rounded-xl overflow-hidden animate-pulse"
          style2={{ background: 'var(--color-surface-2)' }}>
          <div className="aspect-[2/3] rounded-xl" style={{ background: 'var(--color-surface-2)' }} />
          <div className="pt-2 space-y-1.5 px-1">
            <div className="h-3 rounded" style={{ background: 'var(--color-border)', width: '85%' }} />
            <div className="h-2.5 rounded" style={{ background: 'var(--color-border)', width: '50%' }} />
          </div>
        </div>
      ))}
    </div>
  )
}


export default function LibraryPage({ filter, onChangeFilter, onRefresh, refreshing, onToast, onGlobalSearch }) {
  const [data, setData]         = useState(null)
  const [loading, setLoading]   = useState(true)
  const [error, setError]       = useState(null)
  const [selected, setSelected] = useState(null)
  const [search, setSearch]     = useState('')
  const [, setTick]             = useState(0)

  useEffect(() => {
    getLibrary()
      .then(res => setData(res.data))
      .catch(err => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  // 每60秒更新「x分钟前」
  useEffect(() => {
    const id = setInterval(() => setTick(t => t + 1), 60_000)
    return () => clearInterval(id)
  }, [])

  const stats = useMemo(() => {
    if (!data) return null
    return { movies: data.total_movies, tv: data.total_tv }
  }, [data])

  // 搜索时合并所有结果
  const filteredItems = useMemo(() => {
    if (!data || !search.trim()) return null
    const q = search.trim().toLowerCase()
    return [...data.movies, ...data.tv_shows].filter(i =>
      i.title.toLowerCase().includes(q) ||
      (i.original_title || '').toLowerCase().includes(q)
    )
  }, [data, search])

  // 单类型过滤（非 all）时的列表
  const singleList = useMemo(() => {
    if (!data) return []
    // 用严格 media_type 过滤，防止旧快照数据混入
    if (filter === 'movies') return [...data.movies, ...data.tv_shows].filter(m => m.media_type === 'movie')
    if (filter === 'tv')     return [...data.movies, ...data.tv_shows].filter(t => t.media_type === 'tv')
    return []
  }, [data, filter])

  const pageTitle = { all: '全部媒体', movies: '电影', tv: '电视剧' }[filter]

  return (
    <div className="flex-1">
      <section
        className="page-panel panel-surface flex flex-1 flex-col rounded-[22px] px-3 py-4 sm:rounded-[32px] sm:px-7 sm:py-7 relative"
      >
        <div className="mb-4 flex flex-col gap-3 sm:mb-7 sm:gap-5 lg:flex-row lg:items-end lg:justify-between">
          <div className="max-w-2xl">
            <div className="mb-1 text-[10px] font-semibold uppercase tracking-[0.24em] sm:mb-2 sm:text-[11px]" style={{ color: 'var(--color-accent-hover)' }}>
              Cinematic catalog
            </div>
            <h1 className="text-2xl font-bold leading-tight sm:text-[34px]" style={{ color: 'var(--color-text)' }}>{pageTitle}</h1>
            <p className="mt-2 hidden text-sm leading-7 sm:block" style={{ color: 'var(--color-muted)' }}>
              以更清晰的方式浏览你在 Drive 中维护的电影与剧集元数据，快速检索、刷新并查看季集完整度。
            </p>
          </div>

          <div className="relative w-full sm:w-auto">
          <span className="absolute left-3 top-1/2 -translate-y-1/2" style={{ color: 'var(--color-muted)' }}>
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/>
            </svg>
          </span>
          <input
            type="text"
            placeholder="搜索标题..."
            value={search}
            onChange={e => setSearch(e.target.value)}
            className="min-h-11 w-full rounded-full py-3 pl-10 pr-4 text-sm outline-none transition-all sm:w-72"
            style={{
              background: 'rgba(255,255,255,0.03)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-text)',
            }}
          />
        </div>
        </div>

        {stats && (() => {
          const showMovie = filter !== 'tv'
          const showTv    = filter !== 'movies'
          // 窄屏：两个计数卡各占 1 列，刷新卡在 'all' 时占满 2 列，否则占 1 列
          const refreshColSpan = (showMovie && showTv) ? 'col-span-2 sm:col-span-1' : 'col-span-1'

          const refreshBtn = (
            <button
              onClick={onRefresh}
              disabled={refreshing}
              title="刷新媒体库"
              className="flex items-center justify-center rounded-full transition-all duration-150"
              style={{
                width: 38,
                height: 38,
                background: refreshing
                  ? 'rgba(200, 146, 77, 0.16)'
                  : 'linear-gradient(135deg, var(--color-accent) 0%, #b37533 100%)',
                color: '#fff',
                cursor: refreshing ? 'not-allowed' : 'pointer',
                border: 'none',
                boxShadow: refreshing ? 'none' : '0 6px 16px rgba(200, 146, 77, 0.3)',
                flexShrink: 0,
              }}
            >
              <svg
                width="17" height="17" viewBox="0 0 24 24" fill="none"
                stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"
                style={{ animation: refreshing ? 'spin 1s linear infinite' : 'none' }}
              >
                <polyline points="23 4 23 10 17 10"/>
                <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/>
              </svg>
            </button>
          )

          return (
            <div className="mb-5 grid grid-cols-2 gap-2 sm:mb-10 sm:grid-cols-3 sm:gap-4">
              {showMovie && <StatCard label="电影总数" value={stats.movies} />}
              {showTv    && <StatCard label="电视剧总数" value={stats.tv} />}
              <div className={refreshColSpan}>
                <StatCard
                  label="最后刷新"
                  value={relativeTime(data?.scanned_at)}
                  sub={data?.scanned_at
                    ? new Date(data.scanned_at).toLocaleString('zh-CN', { hour12: false })
                    : undefined}
                  action={refreshBtn}
                />
              </div>
            </div>
          )
        })()}


        {loading && (
          <div className="flex flex-col gap-8">
          <div>
            <div className="h-5 w-24 rounded mb-4" style={{ background: 'var(--color-surface-2)' }} />
            <LoadingRow />
          </div>
          <div>
            <div className="h-5 w-24 rounded mb-4" style={{ background: 'var(--color-surface-2)' }} />
            <LoadingRow />
          </div>
        </div>
        )}

        {error && (
          <StatePanel
            icon="!"
            title={`加载失败：${error}`}
            description="请确认 FastAPI 后端已启动，或稍后重试。"
            tone="danger"
          />
        )}

        {!loading && !error && search.trim() && (
        filteredItems?.length > 0 ? (
          <>
            <p className="text-sm mb-3" style={{ color: 'var(--color-muted)' }}>
              找到 {filteredItems.length} 个结果
            </p>
            <MediaGrid items={filteredItems} onSelect={setSelected} />
          </>
        ) : (
          <StatePanel
            icon="🔎"
            title="没有匹配的结果"
            description="换一个标题关键词，或者试试原始标题。"
          />
        )
        )}

        {!loading && !error && !search.trim() && filter === 'all' && data && (
        <>
          {data.movies.length === 0 && data.tv_shows.length === 0 ? (
            <StatePanel
              icon="📭"
              title="媒体库为空"
              description="点击上方刷新媒体库，重新扫描 Drive 内容。"
            />
          ) : (
            <>
              {data.movies.length > 0 && (
                <SectionRow
                  title="电影" count={data.movies.length}
                  items={data.movies} onSelect={setSelected}
                  onViewAll={() => onChangeFilter?.('movies')}
                />
              )}
              {data.tv_shows.length > 0 && (
                <SectionRow
                  title="电视剧" count={data.tv_shows.length}
                  items={data.tv_shows} onSelect={setSelected}
                  onViewAll={() => onChangeFilter?.('tv')}
                />
              )}
            </>
          )}
        </>
        )}

        {!loading && !error && !search.trim() && filter !== 'all' && (
        singleList.length > 0 ? (
          <MediaGrid items={singleList} onSelect={setSelected} />
        ) : (
          <StatePanel
            icon="📭"
            title={`暂无${pageTitle}`}
            description="这个分类下暂时没有可展示的媒体项。"
          />
        )
        )}
      </section>

      {selected && (
        <ScraperDetailModal 
          item={selected} 
          onClose={() => setSelected(null)} 
          onToast={onToast} 
          onSearchResources={(item) => {
            onGlobalSearch?.(item)
            setSelected(null)
          }}
        />
      )}
    </div>
  )
}
