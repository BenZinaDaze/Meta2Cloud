import { useState, useEffect, useCallback, useRef } from 'react'
import { useNavigate } from 'react-router'
import { Film, Tv, Search, RefreshCw, ChevronLeft, ChevronRight, AlertCircle, CheckCircle, HelpCircle, Clock, Trash2 } from 'lucide-react'
import { getIngestHistory, getIngestStats } from '@/api'
import BackToTopButton from '@/components/BackToTopButton'
import { StatePanel } from '@/components/StatePanel'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Badge } from '@/components/ui/badge'
import PageHeader from '@/components/shared/PageHeader'
import { relativeTime } from '@/lib/time'
import type { IngestRecord, IngestHistoryResponse, IngestStatsResponse } from '@/types/api'

function StatusBadge({ status }: { status: string }) {
  const config = {
    success: { label: '成功', icon: CheckCircle, className: 'border-success/30 bg-success/10 text-success' },
    failed: { label: '失败', icon: AlertCircle, className: 'border-destructive/30 bg-destructive/10 text-destructive' },
    no_tmdb: { label: '无元数据', icon: HelpCircle, className: 'border-warning/30 bg-warning/10 text-warning' },
    deleted: { label: '删除', icon: Trash2, className: 'border-destructive/30 bg-destructive/10 text-destructive' },
  }[status] || { label: status, icon: AlertCircle, className: 'border-border bg-muted text-muted-foreground' }

  const Icon = config.icon
  return (
    <Badge variant="outline" className={`gap-1 ${config.className}`}>
      <Icon className="size-3" />
      {config.label}
    </Badge>
  )
}

function IngestCard({ item }: { item: IngestRecord }) {
  const isTV = item.media_type === 'tv'
  const navigate = useNavigate()
  const linkable = item.tmdb_id > 0

  return (
    <div className="flex gap-3 overflow-hidden rounded-xl border border-border/60 bg-card/50 p-3 transition-colors duration-200 hover:border-primary/20 hover:bg-card/80 sm:gap-4 sm:p-4">
      <div className="h-20 w-14 flex-shrink-0 overflow-hidden rounded-lg border border-border/60 bg-muted sm:h-24 sm:w-16">
        {item.poster_url ? (
          <img
            src={item.poster_url}
            alt={item.title}
            className="h-full w-full object-cover"
            loading="lazy"
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-muted-foreground">
            {isTV ? <Tv className="size-6" /> : <Film className="size-6" />}
          </div>
        )}
      </div>

      <div className="min-w-0 flex-1">
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0 flex-1">
            {linkable ? (
              <button
                onClick={() => navigate(isTV ? `/tv/${item.tmdb_id}` : `/movies/${item.tmdb_id}`)}
                className="block w-full truncate text-left font-semibold transition-colors hover:text-primary"
                title="查看媒体详情"
              >
                {item.title}
              </button>
            ) : (
              <h3 className="truncate font-semibold">{item.title}</h3>
            )}
            {item.original_title && item.original_title !== item.title && (
              <p className="truncate text-xs text-muted-foreground">{item.original_title}</p>
            )}
          </div>
          <StatusBadge status={item.status} />
        </div>

        <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          {isTV && item.season !== null && (
            <span className="rounded-md bg-muted px-1.5 py-0.5 font-mono">
              S{String(item.season).padStart(2, '0')}E{String(item.episode || 0).padStart(2, '0')}
            </span>
          )}
          {item.year && <span>{item.year}</span>}
          <span className="text-border">·</span>
          <Clock className="size-3" />
          <span>{relativeTime(item.ingested_at)}</span>
        </div>

        {item.status === 'failed' && item.error_message && (
          <p className="mt-1.5 text-xs text-destructive">{item.error_message}</p>
        )}

        {isTV && item.episode_title && (
          <p className="mt-1 text-xs text-muted-foreground">单集：{item.episode_title}</p>
        )}
      </div>
    </div>
  )
}

function StatCard({ label, value, icon }: { label: string; value: number; icon?: React.ReactNode }) {
  return (
    <div className="card-lift relative overflow-hidden rounded-2xl border border-border/70 bg-card/50 p-4">
      <div className="pointer-events-none absolute inset-0 bg-ambient opacity-40" />
      <div className="relative flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">
        {icon}
        {label}
      </div>
      <p className="relative mt-1 text-2xl font-bold tabular-nums tracking-tight">{value}</p>
    </div>
  )
}

interface IngestHistoryPageProps {
  onToast?: (type: 'success' | 'error' | 'warning', title: string, message?: string) => void
}

export default function IngestHistoryPage(props: IngestHistoryPageProps) {
  void props
  const [data, setData] = useState<IngestHistoryResponse | null>(null)
  const [stats, setStats] = useState<IngestStatsResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [page, setPage] = useState(1)
  const [mediaType, setMediaType] = useState('all')
  const [status, setStatus] = useState('all')
  const [keyword, setKeyword] = useState('')
  const [fetchKey, setFetchKey] = useState(0)
  const mountedRef = useRef(true)

  useEffect(() => {
    mountedRef.current = true
    return () => { mountedRef.current = false }
  }, [])

  const fetchHistory = useCallback(async (showLoading = false) => {
    if (showLoading) setLoading(true)
    try {
      const res = await getIngestHistory({
        page,
        page_size: 20,
        ...(mediaType !== 'all' ? { media_type: mediaType } : {}),
        ...(status !== 'all' ? { status } : {}),
        ...(keyword ? { keyword } : {}),
      })
      if (mountedRef.current) {
        setData(res.data)
        setError(null)
      }
    } catch (err) {
      if (mountedRef.current) setError((err as Error).message)
    } finally {
      if (mountedRef.current) setLoading(false)
    }
  }, [page, mediaType, status, keyword])

  const fetchStats = useCallback(async () => {
    try {
      const res = await getIngestStats(7)
      if (mountedRef.current) setStats(res.data)
    } catch { /* stats are non-critical */ }
  }, [])

  useEffect(() => {
    fetchHistory(true)
  }, [fetchHistory, fetchKey])

  useEffect(() => {
    fetchStats()
  }, [fetchStats])

  const handleRefresh = useCallback(() => {
    setPage(1)
    setFetchKey(k => k + 1)
    fetchStats()
  }, [fetchStats])

  if (loading && !data) {
    return (
      <div className="flex flex-col gap-5">
        <h1 className="text-2xl font-bold leading-tight sm:text-[34px]">入库记录</h1>
        <StatePanel title="正在加载" description="入库记录正在加载中…" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex flex-col gap-5">
        <h1 className="text-2xl font-bold leading-tight sm:text-[34px]">入库记录</h1>
        <StatePanel icon="!" title={`加载失败：${error}`} description="请检查后端服务状态" tone="danger" />
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        kicker="Import Log"
        title="入库记录"
        description="查看所有媒体的入库记录，包括电影和剧集的时间、状态及元数据信息。"
        actions={
          <>
            <div className="relative w-full min-w-0 sm:w-56">
              <Search className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                type="text"
                value={keyword}
                onChange={(e) => setKeyword(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && handleRefresh()}
                placeholder="搜索标题…"
                className="h-10 rounded-full pl-10"
              />
            </div>
            <Select value={mediaType} onValueChange={(v) => { setMediaType(v); setPage(1) }}>
              <SelectTrigger className="h-10 w-28 rounded-full border-border/70 bg-card/50 focus:ring-0 focus:ring-offset-0">
                <SelectValue placeholder="类型" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部</SelectItem>
                <SelectItem value="movie">电影</SelectItem>
                <SelectItem value="tv">剧集</SelectItem>
              </SelectContent>
            </Select>
            <Select value={status} onValueChange={(v) => { setStatus(v); setPage(1) }}>
              <SelectTrigger className="h-10 w-28 rounded-full border-border/70 bg-card/50 focus:ring-0 focus:ring-offset-0">
                <SelectValue placeholder="状态" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部</SelectItem>
                <SelectItem value="success">成功</SelectItem>
                <SelectItem value="failed">失败</SelectItem>
                <SelectItem value="no_tmdb">无元数据</SelectItem>
                <SelectItem value="deleted">删除</SelectItem>
              </SelectContent>
            </Select>
            <Button
              variant="outline"
              onClick={handleRefresh}
              className="h-10 gap-2 rounded-full px-4"
            >
              <RefreshCw className="size-4" />
              刷新
            </Button>
          </>
        }
      />

      {stats && (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-7">
          <StatCard label="近 7 天入库" value={stats.total} icon={<Clock className="size-3" />} />
          <StatCard label="电影" value={stats.movies} icon={<Film className="size-3" />} />
          <StatCard label="剧集集数" value={stats.tv_episodes} icon={<Tv className="size-3" />} />
          <StatCard label="成功" value={stats.success} icon={<CheckCircle className="size-3" />} />
          <StatCard label="失败" value={stats.failed} icon={<AlertCircle className="size-3" />} />
          <StatCard label="无元数据" value={stats.no_tmdb} icon={<HelpCircle className="size-3" />} />
          <StatCard label="删除" value={stats.deleted} icon={<Trash2 className="size-3" />} />
        </div>
      )}

      <section className="min-w-0 overflow-hidden rounded-2xl border border-border/70 bg-card/40 px-4 py-4 sm:px-6 sm:py-6">
        <div className="mb-4 flex items-center justify-between">
          <div>
            <h2 className="text-lg font-semibold tracking-tight">入库记录</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              共 {data?.pagination.total || 0} 条记录
            </p>
          </div>
        </div>

        {data?.items.length === 0 ? (
          <StatePanel
            icon={<Clock className="size-6" />}
            title="暂无入库记录"
            description="当有媒体文件完成整理后，入库记录将显示在这里。"
          />
        ) : (
          <>
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-1">
              {data?.items.map((item, idx) => (
                <div key={item.id} className="animate-stagger-in" style={{ animationDelay: `${Math.min(idx * 20, 240)}ms` }}>
                  <IngestCard item={item} />
                </div>
              ))}
            </div>

            {data && data.pagination.total_pages > 1 && (
              <div className="mt-6 flex items-center justify-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page <= 1}
                  onClick={() => setPage((p) => p - 1)}
                >
                  <ChevronLeft className="size-4" />
                  上一页
                </Button>
                <span className="px-4 text-sm tabular-nums text-muted-foreground">
                  第 {page} / {data.pagination.total_pages} 页
                </span>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page >= data.pagination.total_pages}
                  onClick={() => setPage((p) => p + 1)}
                >
                  下一页
                  <ChevronRight className="size-4" />
                </Button>
              </div>
            )}
          </>
        )}
      </section>
      <BackToTopButton />
    </div>
  )
}
