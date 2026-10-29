import { useState, useEffect, useMemo } from 'react'
import { Search, RefreshCw, ChevronRight, ChevronLeft, ArrowUpDown, RefreshCw as RefreshIcon } from 'lucide-react'
import { getLibrary, getMovieDetail, getTVShowDetail, previewLibraryDelete, deleteLibraryMedia } from '@/api'
import { toast } from 'sonner'
import BackToTopButton from '@/components/BackToTopButton'
import MediaCard from '@/components/MediaCard'
import { StatePanel } from '@/components/StatePanel'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import PageHeader from '@/components/shared/PageHeader'
import ScraperDetailModal from '@/components/modals/ScraperDetailModal'
import type { MediaItem, LibraryResponse } from '@/types/api'
import { relativeTime } from '@/lib/time'

function StatCard({ label, value, sub, action }: {
  label: string
  value: string | number
  sub?: string
  action?: React.ReactNode
}) {
  return (
    <div className="card-lift relative flex items-center justify-between gap-2 overflow-hidden rounded-2xl border border-border/70 bg-card/50 p-4 sm:p-5">
      <div className="pointer-events-none absolute inset-0 bg-ambient opacity-50" />
      <div className="relative min-w-0 flex-1 space-y-0.5">
        <span className="text-[10px] font-semibold uppercase tracking-[0.14em] text-muted-foreground sm:text-[11px]">
          {label}
        </span>
        <span className={`relative block font-bold tabular-nums leading-tight ${action ? 'text-base sm:text-2xl' : 'text-xl sm:text-3xl'}`}>
          {value}
        </span>
        {sub && <span className="block text-[10px] leading-4 text-muted-foreground/60">{sub}</span>}
      </div>
      {action && <div className="relative ml-2 flex-shrink-0">{action}</div>}
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
        <h2 className="shrink-0 text-xl font-bold tracking-tight sm:text-[22px]">{title}</h2>
        <span className="shrink-0 rounded-full bg-muted/80 px-2.5 py-1 text-[11px] font-semibold tabular-nums text-muted-foreground">
          {count}
        </span>
        <div className="h-px flex-1 bg-gradient-to-r from-border via-border/50 to-transparent" />
        <Button variant="outline" size="sm" onClick={onViewAll} className="gap-1 rounded-full text-xs">
          查看全部
          <ChevronRight className="size-3" />
        </Button>
      </div>

      <div className="flex gap-4 overflow-x-auto pb-3 [scrollbar-width:thin]">
        {items.map((item, idx) => (
          <div
            key={`${item.media_type}-${item.tmdb_id || item.title}`}
            className="w-[148px] flex-shrink-0 animate-stagger-in"
            style={{ animationDelay: `${Math.min(idx * 40, 400)}ms` }}
          >
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
      {items.map((item, idx) => (
        <div
          key={`${item.media_type}-${item.tmdb_id || item.title}`}
          className="animate-stagger-in"
          style={{ animationDelay: `${Math.min(idx * 25, 400)}ms` }}
        >
          <MediaCard item={item} onClick={onSelect} />
        </div>
      ))}
    </div>
  )
}

function LoadingRow() {
  return (
    <div className="flex gap-3 overflow-hidden">
      {Array.from({ length: 8 }).map((_, i) => (
        <div key={i} className="w-[148px] flex-shrink-0 animate-pulse overflow-hidden rounded-xl">
          <div className="aspect-[2/3] rounded-xl bg-muted/70" />
          <div className="space-y-1.5 px-1 pt-2">
            <div className="h-3 w-[85%] rounded bg-muted/70" />
            <div className="h-2.5 w-[50%] rounded bg-muted/50" />
          </div>
        </div>
      ))}
    </div>
  )
}

// 联动删除预览（GET /library/media/{id}）
interface LibraryDeletePreviewSubscription {
  id: number
  name: string
  media_title?: string
  media_type?: string
  tmdb_id?: number
  site?: string
  subgroup_name?: string
  season_number?: number
}

interface LibraryDeletePreview {
  drive_folder_id: string
  title: string
  media_type: string
  tmdb_id: number
  /** Provider 不可用或目录无法遍历（含已被外部删除）时省略——此时无法准确统计 */
  strm_count?: number
  subscriptions: LibraryDeletePreviewSubscription[]
}

function apiErrorMessage(e: unknown): string {
  const err = e as { response?: { data?: { detail?: string; message?: string } }; message?: string }
  return err?.response?.data?.detail || err?.response?.data?.message || err?.message || '未知错误'
}

interface LibraryPageProps {
  filter: string
  onChangeFilter?: (filter: string) => void
  onRefresh?: () => void
  refreshing?: boolean
  refreshKey?: number
  onGlobalSearch?: (item: MediaItem) => void
  /** 直链打开详情：URL 中的 tmdb_id（/movies/:tmdbId、/tv/:tmdbId） */
  openTmdbId?: string
  /** 直链打开的详情弹窗关闭时回调（回退到列表路由） */
  onDetailClose?: () => void
}

export default function LibraryPage({
  filter,
  onChangeFilter,
  onRefresh,
  refreshing = false,
  refreshKey = 0,
  onGlobalSearch,
  openTmdbId,
  onDetailClose,
}: LibraryPageProps) {
  const [data, setData] = useState<LibraryResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selected, setSelected] = useState<MediaItem | null>(null)
  const [searchInput, setSearchInput] = useState('')
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const [sortBy, setSortBy] = useState('folder_modified_time')
  const [sortOrder, setSortOrder] = useState('desc')
  const [, setTick] = useState(0)

  // 联动删除：条目 → 预览 → 确认删除（网盘文件 + 本地记录）
  const [deleteTarget, setDeleteTarget] = useState<MediaItem | null>(null)
  const [deletePreview, setDeletePreview] = useState<LibraryDeletePreview | null>(null)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [cancelSubscription, setCancelSubscription] = useState(true)

  const PAGE_SIZE = 60

  // 直链打开详情：按 tmdb_id 异步加载（/movies/:tmdbId、/tv/:tmdbId）
  useEffect(() => {
    const tmdbId = Number(openTmdbId)
    if (!Number.isFinite(tmdbId) || tmdbId <= 0) return
    let cancelled = false
    const request = filter === 'movies' ? getMovieDetail(tmdbId) : getTVShowDetail(tmdbId)
    request
      .then((res) => {
        if (!cancelled && res.data) setSelected(res.data)
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [openTmdbId, filter])

  // 搜索防抖：输入停顿 300ms 后才触发服务端检索
  useEffect(() => {
    const id = setTimeout(() => {
      setSearch(searchInput.trim())
      setPage(1)
    }, 300)
    return () => clearTimeout(id)
  }, [searchInput])

  // 类型/排序变化时回到第一页
  useEffect(() => {
    setPage(1)
  }, [filter, sortBy, sortOrder])

  useEffect(() => {
    setLoading(true)
    getLibrary({
      media_type: filter === 'all' ? undefined : filter,
      keyword: search || undefined,
      page,
      page_size: PAGE_SIZE,
      sort_by: sortBy,
      sort_order: sortOrder,
    })
      .then((res) => setData(res.data))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false))
  }, [refreshKey, sortBy, sortOrder, filter, search, page])

  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), 60_000)
    return () => clearInterval(id)
  }, [])

  const stats = useMemo(() => {
    if (!data) return null
    return { movies: data.total_movies, tv: data.total_tv }
  }, [data])

  // 搜索结果（服务端检索）：all 视图合并两类，其余只看当前类型
  const searchResults = useMemo(() => {
    if (!data || !search) return null
    return filter === 'all' ? [...data.movies, ...data.tv_shows] : filter === 'movies' ? data.movies : data.tv_shows
  }, [data, search, filter])

  const searchTotal = data ? data.total_movies + data.total_tv : 0
  const searchTotalPages = Math.max(1, Math.ceil(searchTotal / PAGE_SIZE))

  // 当前类型页（movies/tv 标签页；all 视图走 SectionRow 横滑，不需要页脚）
  const singleList = useMemo(() => {
    if (!data) return []
    if (filter === 'movies') return data.movies
    if (filter === 'tv') return data.tv_shows
    return []
  }, [data, filter])

  const singleTotal = filter === 'movies' ? data?.total_movies ?? 0 : data?.total_tv ?? 0
  const singleTotalPages = Math.max(1, Math.ceil(singleTotal / PAGE_SIZE))

  const pagination = (totalPages: number) => (
    <div className="flex items-center justify-center gap-3 text-xs text-muted-foreground">
      <Button
        variant="outline"
        size="icon-sm"
        className="rounded-full"
        disabled={page <= 1}
        onClick={() => setPage((p) => Math.max(1, p - 1))}
      >
        <ChevronLeft className="size-4" />
      </Button>
      <span className="tabular-nums">
        {page} / {totalPages}
      </span>
      <Button
        variant="outline"
        size="icon-sm"
        className="rounded-full"
        disabled={page >= totalPages}
        onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
      >
        <ChevronRight className="size-4" />
      </Button>
    </div>
  )

  const pageTitle = { all: '全部媒体', movies: '电影', tv: '剧集' }[filter] || '全部媒体'

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

  function handleRemoved() {
    // 分页模式下直接重取当前页（总数由服务端统计）
    setSelected(null)
    setLoading(true)
    getLibrary({
      media_type: filter === 'all' ? undefined : filter,
      keyword: search || undefined,
      page,
      page_size: PAGE_SIZE,
      sort_by: sortBy,
      sort_order: sortOrder,
    })
      .then((res) => setData(res.data))
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  function handleUpdated(item: MediaItem) {
    setData((current) => {
      if (!current) return current
      const updateList = (entries: MediaItem[]) =>
        entries.map((entry) => (entry.drive_folder_id === item.drive_folder_id ? { ...entry, ...item } : entry))
      return {
        ...current,
        movies: updateList(current.movies),
        tv_shows: updateList(current.tv_shows),
      }
    })
    setSelected(item)
  }

  // 联动删除入口：先取预览（受影响 strm 数 + 匹配订阅），再弹确认框
  function handleDeleteStart(item: MediaItem) {
    if (!item.drive_folder_id) {
      toast.error('该媒体项缺少网盘文件夹 ID，无法删除')
      return
    }
    setDeleteTarget(item)
    setDeletePreview(null)
    setCancelSubscription(true)
    setPreviewLoading(true)
    previewLibraryDelete(item.drive_folder_id)
      .then((res) => setDeletePreview(res.data))
      .catch((err) => {
        toast.error('获取删除预览失败', { description: apiErrorMessage(err) })
        setDeleteTarget(null)
      })
      .finally(() => setPreviewLoading(false))
  }

  async function handleDeleteConfirm() {
    if (!deleteTarget?.drive_folder_id) return
    setDeleting(true)
    try {
      const res = await deleteLibraryMedia(deleteTarget.drive_folder_id, cancelSubscription)
      const removedStrm = res.data?.removed_strm ?? 0
      const cancelledSubs = res.data?.cancelled_subscriptions ?? 0
      toast.success(`已删除「${deleteTarget.title}」`, {
        description: `已删除 ${removedStrm} 个 STRM 文件${
          cancelledSubs > 0 ? `，已取消 ${cancelledSubs} 个对应订阅` : ''
        }`,
      })
      setDeleteTarget(null)
      setDeletePreview(null)
      handleRemoved()
    } catch (err) {
      toast.error('删除失败', { description: apiErrorMessage(err) })
    } finally {
      setDeleting(false)
    }
  }

  return (
    <div className="flex flex-1 flex-col gap-5">
      <PageHeader
        kicker="Cinematic catalog"
        title={pageTitle}
        description="浏览网盘媒体库中的电影与剧集元数据，支持检索、刷新与季集完整度查看。"
        actions={
          <>
            <Select
              value={`${sortBy}:${sortOrder}`}
              onValueChange={(v) => {
                const [by, order] = v.split(':')
                setSortBy(by)
                setSortOrder(order)
              }}
            >
              <SelectTrigger className="h-10 w-36 rounded-full border-border/70 bg-card/50 focus:ring-0 focus:ring-offset-0">
                <ArrowUpDown className="size-3.5 text-muted-foreground" />
                <SelectValue />
              </SelectTrigger>
              <SelectContent className="rounded-xl">
                <SelectItem value="folder_modified_time:desc">最近更新</SelectItem>
                <SelectItem value="folder_modified_time:asc">最早更新</SelectItem>
                <SelectItem value="updated_at:desc">最近入库</SelectItem>
                <SelectItem value="updated_at:asc">最早入库</SelectItem>
                <SelectItem value="year:desc">最新发行</SelectItem>
                <SelectItem value="year:asc">最早发行</SelectItem>
                <SelectItem value="title:asc">标题 A-Z</SelectItem>
                <SelectItem value="title:desc">标题 Z-A</SelectItem>
              </SelectContent>
            </Select>
            <div className="relative w-full sm:w-auto">
              <Search className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                type="text"
                placeholder="搜索标题…"
                value={searchInput}
                onChange={(e) => setSearchInput(e.target.value)}
                className="h-10 w-full rounded-full pl-10 pr-4 sm:w-72"
              />
            </div>
          </>
        }
      />

      {stats && (
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 sm:gap-4">
          {filter !== 'tv' && <StatCard label="电影总数" value={stats.movies} />}
          {filter !== 'movies' && <StatCard label="剧集总数" value={stats.tv} />}
          <div className={filter === 'all' ? 'col-span-2 sm:col-span-1' : 'col-span-1'}>
            <StatCard
              label="最后刷新"
              value={relativeTime(data?.scanned_at, { fallback: '从未刷新', dayThreshold: 30 })}
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
            <div className="mb-4 h-5 w-24 rounded bg-muted/70" />
            <LoadingRow />
          </div>
          <div>
            <div className="mb-4 h-5 w-24 rounded bg-muted/70" />
            <LoadingRow />
          </div>
        </div>
      )}

      {error && (
        <StatePanel
          icon="!"
          title={`加载失败：${error}`}
          description="请确认后端服务已启动，或稍后重试。"
          tone="danger"
        />
      )}

      {!loading && !error && search.trim() && (
        searchResults && searchResults.length > 0 ? (
          <>
            <p className="mb-3 text-sm text-muted-foreground">找到 {searchTotal} 个结果</p>
            <MediaGrid items={searchResults} onSelect={setSelected} />
            {searchTotalPages > 1 && pagination(searchTotalPages)}
          </>
        ) : (
          <StatePanel
            icon={<Search className="size-6" />}
            title="未找到匹配结果"
            description="请尝试其他标题关键词或原始标题。"
          />
        )
      )}

      {!loading && !error && !search.trim() && filter === 'all' && data && (
        data.movies.length === 0 && data.tv_shows.length === 0 ? (
          <StatePanel
            icon={<RefreshIcon className="size-6" />}
            title="媒体库为空"
            description="点击上方刷新媒体库，重新扫描网盘内容。"
          />
        ) : (
          <>
            {data.movies.length > 0 && (
              <SectionRow
                title="电影"
                count={data.total_movies}
                items={data.movies}
                onSelect={setSelected}
                onViewAll={() => onChangeFilter?.('movies')}
              />
            )}
            {data.tv_shows.length > 0 && (
              <SectionRow
                title="剧集"
                count={data.total_tv}
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
          <>
            <MediaGrid items={singleList} onSelect={setSelected} />
            {singleTotalPages > 1 && pagination(singleTotalPages)}
          </>
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
        onOpenChange={(open) => {
          if (!open) {
            setSelected(null)
            onDetailClose?.()
          }
        }}
        onUpdated={handleUpdated}
        onDeleteRequest={handleDeleteStart}
        onSearchResources={(item) => {
          onGlobalSearch?.(item)
          setSelected(null)
        }}
      />

      <AlertDialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteTarget(null)
            setDeletePreview(null)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认删除该媒体？</AlertDialogTitle>
            <AlertDialogDescription>
              将删除网盘目录下的全部文件，并移除本地 STRM 与媒体库记录，操作不可恢复。
            </AlertDialogDescription>
          </AlertDialogHeader>
          {previewLoading && (
            <p className="text-sm text-muted-foreground">正在获取删除预览…</p>
          )}
          {deletePreview && (
            <>
              <div className="space-y-1.5 rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm">
                <p className="font-semibold text-destructive">{deletePreview.title}</p>
                <p className="text-muted-foreground">
                  网盘目录：<span className="break-all font-mono text-xs">{deletePreview.drive_folder_id}</span>
                </p>
                <p className="text-muted-foreground">
                  受影响 STRM 文件：
                  <span className="font-semibold text-destructive">{deletePreview.strm_count ?? '未知'}</span> 个
                </p>
              </div>
              {deletePreview.subscriptions.length > 0 && (
                <label className="flex cursor-pointer items-start gap-2.5 text-sm text-muted-foreground">
                  <input
                    type="checkbox"
                    checked={cancelSubscription}
                    onChange={(e) => setCancelSubscription(e.target.checked)}
                    className="mt-0.5 size-4 shrink-0 rounded border-input accent-[oklch(76%_0.19_150)]"
                  />
                  <span>
                    同时取消对应订阅（{deletePreview.subscriptions.length} 个）：
                    {deletePreview.subscriptions.map((s) => `「${s.name}」`).join('、')}
                  </span>
                </label>
              )}
            </>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault()
                handleDeleteConfirm()
              }}
              disabled={previewLoading || deleting || !deletePreview}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleting ? '删除中…' : '确认删除'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <BackToTopButton />
    </div>
  )
}
