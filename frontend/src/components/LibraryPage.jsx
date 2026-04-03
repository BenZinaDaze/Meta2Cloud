import { useState, useEffect, useMemo } from 'react'
import { getLibrary } from '../api'
import MediaCard from './MediaCard'
import DetailModal from './DetailModal'

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
    <div className="relative flex flex-col justify-center gap-1 px-5 rounded-xl"
      style={{
        background: 'var(--color-surface-2)',
        border: '1px solid var(--color-border)',
        minHeight: 88,
        paddingTop: 16,
        paddingBottom: 16,
      }}>
      <span className="text-xs" style={{ color: 'var(--color-muted)' }}>{label}</span>
      <span className="text-2xl font-bold" style={{ color: 'var(--color-text)' }}>{value}</span>
      {sub && <span className="text-xs" style={{ color: 'var(--color-muted)' }}>{sub}</span>}
      {action && (
        <div className="absolute right-4 top-1/2 -translate-y-1/2">{action}</div>
      )}
    </div>
  )
}

/** 横向滚动区块（MoviePilot 风格） */
function SectionRow({ title, count, items, onSelect, onViewAll }) {
  return (
    <section className="mb-8">
      {/* 区块标题栏 */}
      <div className="flex items-center gap-3 mb-4">
        <h2 className="text-base font-bold flex-shrink-0" style={{ color: 'var(--color-text)' }}>{title}</h2>
        <span className="text-xs px-2 py-0.5 rounded-full flex-shrink-0"
          style={{ background: 'var(--color-surface-2)', color: 'var(--color-muted)' }}>
          {count}
        </span>
        {/* 延伸分割线 */}
        <div className="flex-1 h-px" style={{ background: 'var(--color-border)' }} />
        <button
          onClick={onViewAll}
          className="flex-shrink-0 flex items-center gap-1 text-xs px-3 py-1 rounded-full transition-all duration-150 hover:opacity-80"
          style={{ background: 'var(--color-surface-2)', color: 'var(--color-accent)', border: '1px solid var(--color-border)' }}
        >
          查看全部
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
            <polyline points="9 18 15 12 9 6"/>
          </svg>
        </button>
      </div>

      {/* 横向滚动行 */}
      <div
        className="flex gap-3 overflow-x-auto pb-3"
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

export default function LibraryPage({ filter, onChangeFilter, onRefresh, refreshing }) {
  const [data, setData]         = useState(null)
  const [loading, setLoading]   = useState(true)
  const [error, setError]       = useState(null)
  const [selected, setSelected] = useState(null)
  const [search, setSearch]     = useState('')
  const [, setTick]             = useState(0)

  useEffect(() => {
    setLoading(true)
    setError(null)
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
      {/* 顶栏：标题 + 搜索 */}
      <div className="flex items-center gap-4 mb-5">
        <h1 className="text-xl font-bold" style={{ color: 'var(--color-text)' }}>{pageTitle}</h1>
        <div className="ml-auto relative">
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
            className="pl-9 pr-4 py-2 text-sm rounded-full outline-none w-56 transition-all"
            style={{
              background: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-text)',
            }}
          />
        </div>
      </div>

      {/* 统计卡片 */}
      {stats && (
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-3 gap-3 mb-8">
          {/* 全部 / 电影视图：显示电影数 */}
          {filter !== 'tv' && (
            <StatCard label="电影总数" value={stats.movies} />
          )}
          {/* 全部 / 电视剧视图：显示剧集数 */}
          {filter !== 'movies' && (
            <StatCard label="电视剧总数" value={stats.tv} />
          )}
          <StatCard
            label="媒体库最后刷新"
            value={relativeTime(data?.scanned_at)}
            sub={data?.scanned_at
              ? new Date(data.scanned_at).toLocaleString('zh-CN', { hour12: false })
              : undefined}
            action={
              <button
                onClick={onRefresh}
                disabled={refreshing}
                title="刷新媒体库"
                className="flex items-center justify-center rounded-full transition-all duration-150"
                style={{
                  width: 70,
                  height: 70,
                  background: refreshing ? 'rgba(180,40,40,0.3)' : 'rgba(220,38,38,0.85)',
                  color: '#fff',
                  cursor: refreshing ? 'not-allowed' : 'pointer',
                  border: 'none',
                  boxShadow: refreshing ? 'none' : '0 2px 12px rgba(220,38,38,0.45)',
                }}
              >
                <svg
                  width="32" height="32" viewBox="0 0 24 24" fill="none"
                  stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"
                  style={{ animation: refreshing ? 'spin 1s linear infinite' : 'none' }}
                >
                  <polyline points="23 4 23 10 17 10"/>
                  <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/>
                </svg>
              </button>
            }
          />
        </div>
      )}

      {/* 加载中 */}
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

      {/* 错误 */}
      {error && (
        <div className="flex flex-col items-center justify-center py-32 gap-4">
          <span style={{ fontSize: 120, lineHeight: 1 }}>⚠️</span>
          <p className="text-xl font-medium" style={{ color: 'var(--color-danger)' }}>加载失败：{error}</p>
          <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
            请确认 FastAPI 后端已在 localhost:8765 运行
          </p>
        </div>
      )}

      {/* 搜索结果（搜索时覆盖所有其他视图） */}
      {!loading && !error && search.trim() && (
        filteredItems?.length > 0 ? (
          <>
            <p className="text-sm mb-3" style={{ color: 'var(--color-muted)' }}>
              找到 {filteredItems.length} 个结果
            </p>
            <MediaGrid items={filteredItems} onSelect={setSelected} />
          </>
        ) : (
          <div className="flex flex-col items-center justify-center py-32 gap-4">
            <span style={{ fontSize: 120, lineHeight: 1 }}>📭</span>
            <p className="text-xl font-medium" style={{ color: 'var(--color-muted)' }}>没有匹配的结果</p>
            <p className="text-sm" style={{ color: 'var(--color-muted)', opacity: 0.6 }}>试试其他关键词</p>
          </div>
        )
      )}

      {/* 全部视图：分区横向滚动（MoviePilot 风格） */}
      {!loading && !error && !search.trim() && filter === 'all' && data && (
        <>
          {data.movies.length === 0 && data.tv_shows.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-32 gap-4">
              <span style={{ fontSize: 120, lineHeight: 1 }}>📭</span>
              <p className="text-xl font-medium" style={{ color: 'var(--color-muted)' }}>媒体库为空</p>
              <p className="text-sm" style={{ color: 'var(--color-muted)', opacity: 0.6 }}>点击右上角「刷新媒体库」扫描 Drive</p>
            </div>
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

      {/* 单类型视图：网格 */}
      {!loading && !error && !search.trim() && filter !== 'all' && (
        singleList.length > 0 ? (
          <MediaGrid items={singleList} onSelect={setSelected} />
        ) : (
          <div className="flex flex-col items-center justify-center py-32 gap-4">
            <span style={{ fontSize: 120, lineHeight: 1 }}>📭</span>
            <p className="text-xl font-medium" style={{ color: 'var(--color-muted)' }}>暂无{pageTitle}</p>
          </div>
        )
      )}

      {selected && (
        <DetailModal item={selected} onClose={() => setSelected(null)} />
      )}
    </div>
  )
}
