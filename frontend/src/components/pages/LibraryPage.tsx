import { useState, useEffect, useMemo } from 'react'
import { Search, RefreshCw, ChevronRight } from 'lucide-react'
import { getLibrary } from '@/api'
import MediaCard from '@/components/MediaCard'
import { StatePanel } from '@/components/StatePanel'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import ScraperDetailModal from '@/components/modals/ScraperDetailModal'
import type { MediaItem, LibraryResponse } from '@/types/api'

function relativeTime(isoStr: string | undefined): string {
  if (!isoStr) return '从未刷新'
  const diff = Math.floor((Date.now() - new Date(isoStr).getTime()) / 1000)
  if (diff < 60) return '刚刚'
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`
  if (diff < 86400 * 30) return `${Math.floor(diff / 86400)} 天前`
  return new Date(isoStr).toLocaleDateString('zh-CN')
}

function StatCard({ label, value, sub, action }: {
  label: string
  value: string | number
  sub?: string
  action?: React.ReactNode
}) {
  return (
    <div className="flex items-center justify-between gap-2 rounded-2xl border bg-card p-4 sm:p-5">
      <div className="min-w-0 flex-1 space-y-0.5">
        <span className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground sm:text-[11px]">
          {label}
        </span>
        <span className={`block font-bold tabular-nums leading-tight ${action ? 'text-base sm:text-2xl' : 'text-xl sm:text-3xl'}`}>
          {value}
        </span>
        {sub && <span className="block text-[10px] leading-4 text-muted-foreground/60">{sub}</span>}
      </div>
      {action && <div className="ml-2 flex-shrink-0">{action}</div>}
    </div>
  )
}

function SectionRow({
  title,
  count,
  items,
  onSelect,
  onViewAll,
}: {
  title: string
  count: number
  items: MediaItem[]
  onSelect: (item: MediaItem) => void
  onViewAll: () => void
}) {
  return (
    <section className="mb-10">
      <div className="mb-5 flex items-center gap-3">
        <h2 className="shrink-0 text-xl font-bold sm:text-[22px]">{title}</h2>
        <span className="shrink-0 rounded-full bg-muted px-2.5 py-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
          {count}
        </span>
        <div className="h-px flex-1 bg-gradient-to-r from-border to-transparent" />
        <Button variant="outline" size="sm" onClick={onViewAll} className="gap-1 text-xs">
          查看全部
          <ChevronRight className="size-3" />
        </Button>
      </div>

      <div className="flex gap-4 overflow-x-auto pb-3">
        {items.map((item) => (
          <div key={`${item.media_type}-${item.tmdb_id || item.title}`} className="w-[148px] flex-shrink-0">
            <MediaCard item={item} onClick={onSelect} compact />
          </div>
        ))}
      </div>
    </section>
  )
}

function MediaGrid({ items, onSelect }: { items: MediaItem[]; onSelect: (item: MediaItem) => void }) {
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-10">
      {items.map((item) => (
        <MediaCard key={`${item.media_type}-${item.tmdb_id || item.title}`} item={item} onClick={onSelect} />
      ))}
    </div>
  )
}

function LoadingRow() {
  return (
    <div className="flex gap-3 overflow-hidden">
      {Array.from({ length: 8 }).map((_, i) => (
        <div key={i} className="w-[148px] flex-shrink-0 animate-pulse overflow-hidden rounded-xl">
          <div className="aspect-[2/3] rounded-xl bg-muted" />
          <div className="space-y-1.5 px-1 pt-2">
            <div className="h-3 w-[85%] rounded bg-muted" />
            <div className="h-2.5 w-[50%] rounded bg-muted" />
          </div>
        </div>
      ))}
    </div>
  )
}

interface LibraryPageProps {
  filter: string
  onChangeFilter?: (filter: string) => void
  onRefresh?: () => void
  refreshing?: boolean
  refreshKey?: number
  onToast?: (type: 'success' | 'error' | 'warning', title: string, message?: string) => void
  onGlobalSearch?: (item: MediaItem) => void
}

export default function LibraryPage({
  filter,
  onChangeFilter,
  onRefresh,
  refreshing = false,
  refreshKey = 0,
  onGlobalSearch,
}: LibraryPageProps) {
  const [data, setData] = useState<LibraryResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selected, setSelected] = useState<MediaItem | null>(null)
  const [search, setSearch] = useState('')
  const [, setTick] = useState(0)

  useEffect(() => {
    getLibrary()
      .then((res) => setData(res.data))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false))
  }, [refreshKey])

  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), 60_000)
    return () => clearInterval(id)
  }, [])

  const stats = useMemo(() => {
    if (!data) return null
    return { movies: data.total_movies, tv: data.total_tv }
  }, [data])

  const filteredItems = useMemo(() => {
    if (!data || !search.trim()) return null
    const q = search.trim().toLowerCase()
    return [...data.movies, ...data.tv_shows].filter(
      (i) =>
        i.title.toLowerCase().includes(q) ||
        (i.original_title || '').toLowerCase().includes(q)
    )
  }, [data, search])

  const singleList = useMemo(() => {
    if (!data) return []
    if (filter === 'movies') return [...data.movies, ...data.tv_shows].filter((m) => m.media_type === 'movie')
    if (filter === 'tv') return [...data.movies, ...data.tv_shows].filter((t) => t.media_type === 'tv')
    return []
  }, [data, filter])

  const pageTitle = { all: '全部媒体', movies: '电影', tv: '电视剧' }[filter] || '全部媒体'

  const refreshBtn = (
    <Button
      size="icon"
      onClick={onRefresh}
      disabled={refreshing}
      title="刷新媒体库"
      className="size-9 rounded-full"
    >
      <RefreshCw className={`size-4 ${refreshing ? 'animate-spin' : ''}`} />
    </Button>
  )

  return (
    <div className="flex flex-1 flex-col gap-5">
      <div className="mb-2 flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
        <div className="max-w-2xl">
          <div className="mb-1 text-[10px] font-semibold uppercase tracking-widest text-brand sm:mb-2 sm:text-[11px]">
            Cinematic catalog
          </div>
          <h1 className="text-2xl font-bold leading-tight sm:text-[34px]">{pageTitle}</h1>
          <p className="mt-2 hidden text-sm leading-7 text-muted-foreground sm:block">
            以更清晰的方式浏览你在 Drive 中维护的电影与剧集元数据，快速检索、刷新并查看季集完整度。
          </p>
        </div>

        <div className="relative w-full sm:w-auto">
          <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            type="text"
            placeholder="搜索标题..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="h-11 w-full rounded-full pl-10 pr-4 sm:w-72"
          />
        </div>
      </div>

      {stats && (
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 sm:gap-4">
          {filter !== 'tv' && <StatCard label="电影总数" value={stats.movies} />}
          {filter !== 'movies' && <StatCard label="电视剧总数" value={stats.tv} />}
          <div className={filter === 'all' ? 'col-span-2 sm:col-span-1' : 'col-span-1'}>
            <StatCard
              label="最后刷新"
              value={relativeTime(data?.scanned_at)}
              sub={
                data?.scanned_at
                  ? new Date(data.scanned_at).toLocaleString('zh-CN', { hour12: false })
                  : undefined
              }
              action={refreshBtn}
            />
          </div>
        </div>
      )}

      {loading && (
        <div className="flex flex-col gap-8">
          <div>
            <div className="mb-4 h-5 w-24 rounded bg-muted" />
            <LoadingRow />
          </div>
          <div>
            <div className="mb-4 h-5 w-24 rounded bg-muted" />
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
        filteredItems && filteredItems.length > 0 ? (
          <>
            <p className="mb-3 text-sm text-muted-foreground">找到 {filteredItems.length} 个结果</p>
            <MediaGrid items={filteredItems} onSelect={setSelected} />
          </>
        ) : (
          <StatePanel
            icon={<Search className="size-6" />}
            title="没有匹配的结果"
            description="换一个标题关键词，或者试试原始标题。"
          />
        )
      )}

      {!loading && !error && !search.trim() && filter === 'all' && data && (
        data.movies.length === 0 && data.tv_shows.length === 0 ? (
          <StatePanel
            icon="📭"
            title="媒体库为空"
            description="点击上方刷新媒体库，重新扫描 Drive 内容。"
          />
        ) : (
          <>
            {data.movies.length > 0 && (
              <SectionRow
                title="电影"
                count={data.movies.length}
                items={data.movies}
                onSelect={setSelected}
                onViewAll={() => onChangeFilter?.('movies')}
              />
            )}
            {data.tv_shows.length > 0 && (
              <SectionRow
                title="电视剧"
                count={data.tv_shows.length}
                items={data.tv_shows}
                onSelect={setSelected}
                onViewAll={() => onChangeFilter?.('tv')}
              />
            )}
          </>
        )
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

      <ScraperDetailModal
        item={selected}
        open={!!selected}
        onOpenChange={(open) => !open && setSelected(null)}
        onSearchResources={(item) => {
          onGlobalSearch?.(item)
          setSelected(null)
        }}
      />
    </div>
  )
}
