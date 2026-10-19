import { useState, useEffect, useCallback, useRef, Fragment } from 'react'
import {
  RefreshCw,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  CheckCircle,
  AlertCircle,
  Clock,
  ArrowLeftRight,
  Loader2,
  Tv,
  Film,
} from 'lucide-react'
import {
  getWatchedSyncStatus,
  triggerWatchedSync,
  listWatchedSyncRuns,
  getWatchedSyncRunItems,
} from '@/api'
import { StatePanel } from '@/components/StatePanel'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import PageHeader from '@/components/shared/PageHeader'
import { relativeTime } from '@/lib/time'

interface WatchedSyncRun {
  id: number
  started_at: string
  finished_at: string | null
  trigger: string
  to_trakt: number
  to_plex: number
  conflicts: number
  unresolved: number
  skipped: number
  error: string | null
}

interface WatchedSyncRunItem {
  id: number
  media_type: string | null
  tmdb_id: number | null
  season: number | null
  episode: number | null
  title: string | null
  direction: string
  action: string
  success: boolean
  error: string | null
}

interface WatchedSyncStatus {
  enabled: boolean
  trakt_ready: boolean
  trakt_authorized: boolean
  plex_ready: boolean
  plex_connected: boolean
  plex_reachable: boolean
  sync_to_plex: boolean
  sync_to_trakt: boolean
  sync_interval_seconds: number
  session_poll_seconds: number
  running: boolean
  sse_enabled?: boolean
  sse_connected?: boolean
  last_run: WatchedSyncRun | null
}

const TRIGGER_LABELS: Record<string, string> = {
  manual: '手动',
  schedule: '定时',
  activity: 'Trakt 活动',
  plex_activity: 'Plex 变动',
}

const DIRECTION_LABELS: Record<string, { label: string; className: string }> = {
  to_trakt: { label: '→ Trakt', className: 'border-primary/30 bg-primary/10 text-primary' },
  to_plex: { label: '→ Plex', className: 'border-info/30 bg-info/10 text-info' },
  unresolved: { label: '未识别', className: 'border-warning/30 bg-warning/10 text-warning' },
}

const ACTION_LABELS: Record<string, string> = {
  watched: '标记已看',
  unwatched: '取消已看',
  unresolved: '—',
}

const PAGE_SIZE = 20

function StatCard({ label, value, icon }: { label: string; value: React.ReactNode; icon?: React.ReactNode }) {
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

// 侧向图标：→ Trakt 用剧集，→ Plex 用电影，仅作视觉区分
function DirectionIcon({ mediaType }: { mediaType: string | null }) {
  if (mediaType === 'movie') return <Film className="size-3" />
  if (mediaType === 'tv') return <Tv className="size-3" />
  return null
}

function itemDisplayName(item: WatchedSyncRunItem): string {
  const title = item.title || (item.tmdb_id ? `TMDB ${item.tmdb_id}` : '未知条目')
  if (item.media_type === 'tv' && item.season != null && item.episode != null) {
    return `${title} S${String(item.season).padStart(2, '0')}E${String(item.episode).padStart(2, '0')}`
  }
  return title
}

function RunItems({ runId, running }: { runId: number; running: boolean }) {
  const [items, setItems] = useState<WatchedSyncRunItem[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      try {
        const res = await getWatchedSyncRunItems(runId, { limit: 1000 })
        if (!cancelled) setItems(res.data?.items || [])
      } catch (e) {
        if (!cancelled) setError((e as Error).message)
      }
    }
    load()
    // 同步进行中每 2 秒刷新，实现「同步一条显示一条」；结束后补拉一次完整明细
    let timer: ReturnType<typeof setInterval> | null = null
    if (running) {
      timer = setInterval(load, 2000)
    }
    return () => {
      cancelled = true
      if (timer) clearInterval(timer)
    }
  }, [runId, running])

  if (error) return <p className="px-3 py-2 text-xs text-destructive">明细加载失败：{error}</p>
  if (!items) return <p className="px-3 py-2 text-xs text-muted-foreground">加载中...</p>
  if (items.length === 0) {
    return (
      <p className="px-3 py-2 text-xs text-muted-foreground">
        {running ? '正在枚举两侧观看状态，稍后这里会逐条显示同步内容…' : '无逐条明细（本轮无变更，或仅统计未记录）'}
      </p>
    )
  }

  return (
    <div className="overflow-hidden rounded-lg border border-border/50">
      {running && (
        <div className="flex items-center gap-2 border-b border-border/40 bg-muted/30 px-3 py-1.5 text-xs text-muted-foreground">
          <Loader2 className="size-3 animate-spin text-primary" />
          同步进行中，已处理 {items.length} 条（Trakt 侧按批提交，明细会一批约 100 条地出现）
        </div>
      )}
      <table className="w-full text-left text-xs">
        <thead className="bg-muted/50 text-muted-foreground">
          <tr>
            <th className="px-3 py-2 font-medium">方向</th>
            <th className="px-3 py-2 font-medium">内容</th>
            <th className="px-3 py-2 font-medium">动作</th>
            <th className="px-3 py-2 font-medium">结果</th>
          </tr>
        </thead>
        <tbody>
          {items.map((it) => {
            const dir = DIRECTION_LABELS[it.direction] || { label: it.direction, className: 'border-border bg-muted text-muted-foreground' }
            return (
              <tr key={it.id} className="border-t border-border/40">
                <td className="px-3 py-2">
                  <Badge variant="outline" className={`gap-1 ${dir.className}`}>
                    <DirectionIcon mediaType={it.media_type} />
                    {dir.label}
                  </Badge>
                </td>
                <td className="max-w-[280px] truncate px-3 py-2" title={it.tmdb_id ? `${itemDisplayName(it)}（TMDB ${it.tmdb_id}）` : itemDisplayName(it)}>
                  {itemDisplayName(it)}
                </td>
                <td className="px-3 py-2 text-muted-foreground">{ACTION_LABELS[it.action] || it.action}</td>
                <td className="max-w-[240px] truncate px-3 py-2">
                  {it.success ? (
                    <span className="inline-flex items-center gap-1 text-success"><CheckCircle className="size-3" />成功</span>
                  ) : (
                    <span className="inline-flex items-center gap-1 text-destructive" title={it.error || ''}>
                      <AlertCircle className="size-3" />{it.error || '失败'}
                    </span>
                  )}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

interface WatchedSyncPageProps {
  onToast?: (type: 'success' | 'error' | 'warning', title: string, message?: string) => void
}

export default function WatchedSyncPage(props: WatchedSyncPageProps) {
  const [status, setStatus] = useState<WatchedSyncStatus | null>(null)
  const [runs, setRuns] = useState<WatchedSyncRun[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(0)
  const [loading, setLoading] = useState(true)
  const [syncing, setSyncing] = useState(false)
  const [expandedRun, setExpandedRun] = useState<number | null>(null)
  const mountedRef = useRef(true)

  useEffect(() => {
    mountedRef.current = true
    return () => { mountedRef.current = false }
  }, [])

  const fetchData = useCallback(async (showLoading = false) => {
    if (showLoading) setLoading(true)
    try {
      const [statusRes, runsRes] = await Promise.all([
        getWatchedSyncStatus(),
        listWatchedSyncRuns({ limit: PAGE_SIZE, offset: page * PAGE_SIZE }),
      ])
      if (mountedRef.current) {
        setStatus(statusRes.data)
        setRuns(runsRes.data?.runs || [])
        setTotal(runsRes.data?.total || 0)
      }
    } catch { /* 轮询失败静默，下轮重试 */ }
    finally {
      if (mountedRef.current) setLoading(false)
    }
  }, [page])

  useEffect(() => {
    fetchData(true)
  }, [fetchData])

  // 同步进行中加密轮询，空闲时低速轮询
  useEffect(() => {
    const running = status?.running
    const interval = running ? 3000 : 30000
    const timer = setInterval(() => fetchData(), interval)
    return () => clearInterval(timer)
  }, [fetchData, status?.running])

  // 有运行中的同步时自动展开它，实时逐条显示明细
  useEffect(() => {
    if (expandedRun != null) return
    const runningRun = runs.find((r) => r.finished_at == null)
    if (runningRun) setExpandedRun(runningRun.id)
  }, [runs, expandedRun])

  const handleSync = useCallback(async () => {
    setSyncing(true)
    try {
      await triggerWatchedSync()
      props.onToast?.('success', '同步已启动', '观看同步正在后台执行')
      fetchData()
    } catch (e) {
      const detail = (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail
      props.onToast?.('error', '触发同步失败', detail || (e as Error).message)
    } finally {
      setSyncing(false)
    }
  }, [props, fetchData])

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const running = !!status?.running
  const ready = !!status?.trakt_authorized && !!status?.plex_connected

  if (loading && !status) {
    return (
      <div className="flex flex-col gap-5">
        <h1 className="text-2xl font-bold leading-tight sm:text-[34px]">Trakt 同步</h1>
        <StatePanel title="正在加载" description="Trakt 同步状态加载中..." />
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        kicker="Trakt Sync"
        title="Trakt 同步"
        description="Trakt 与 Plex 观看状态双向同步的运行历史与逐条明细：同步了什么内容、往哪个方向、结果如何。配对与同步选项在「配置 → 观看同步」设置。"
        actions={
          <Button onClick={handleSync} disabled={syncing || running || !ready} className="rounded-full">
            {running ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
            {running ? '同步中...' : '立即同步'}
          </Button>
        }
      />

      {!status?.trakt_authorized && (
        <div className="rounded-lg border border-warning/25 bg-warning/10 px-4 py-3 text-sm text-warning">
          Trakt 未授权，请先在「配置 → 观看同步」完成设备码配对。
        </div>
      )}
      {!!status?.trakt_authorized && !status?.plex_connected && (
        <div className="rounded-lg border border-warning/25 bg-warning/10 px-4 py-3 text-sm text-warning">
          Plex 未连接，请先在「配置 → 观看同步」完成 PIN 授权。
        </div>
      )}

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
        <StatCard
          label="状态"
          icon={<ArrowLeftRight className="size-3" />}
          value={
            <span className={running ? 'text-primary' : ''}>
              {running ? '同步中' : status?.enabled ? '空闲（定时已开启）' : '空闲'}
            </span>
          }
        />
        <StatCard
          label="Trakt"
          icon={<ArrowLeftRight className="size-3" />}
          value={
            <span className={status?.trakt_authorized ? '' : 'text-muted-foreground'}>
              {status?.trakt_authorized ? '已授权' : '未授权'}
            </span>
          }
        />
        <StatCard
          label="Plex"
          icon={<ArrowLeftRight className="size-3" />}
          value={
            <span className={status?.plex_connected ? (status?.plex_reachable ? '' : 'text-warning') : 'text-muted-foreground'}>
              {status?.plex_connected ? (status?.plex_reachable ? '已连接' : '不可达') : '未连接'}
            </span>
          }
        />
        <StatCard
          label="实时推送"
          icon={<ArrowLeftRight className="size-3" />}
          value={
            status?.sse_enabled === false ? (
              <span className="text-muted-foreground">未启用</span>
            ) : status?.sse_connected ? (
              <span className="text-success">已连接</span>
            ) : (
              <span className="text-muted-foreground">轮询模式</span>
            )
          }
        />
        <StatCard
          label="上次同步"
          icon={<Clock className="size-3" />}
          value={
            status?.last_run
              ? <span className="text-base">{relativeTime(status.last_run.started_at)}</span>
              : '-'
          }
        />
      </div>

      {status?.last_run && (
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <Badge variant="outline" className="border-border/70 bg-muted/50">
            {TRIGGER_LABELS[status.last_run.trigger] || status.last_run.trigger}
          </Badge>
          {status?.sync_to_trakt && <span>→Trakt {status.last_run.to_trakt}</span>}
          {status?.sync_to_plex && <span>→Plex {status.last_run.to_plex}</span>}
          <span>冲突 {status.last_run.conflicts}</span>
          <span>未识别 {status.last_run.unresolved}</span>
          {status.last_run.error && (
            <span className="text-destructive" title={status.last_run.error}>出错：{status.last_run.error}</span>
          )}
        </div>
      )}

      <div className="overflow-hidden rounded-xl border border-border/60 bg-card/50">
        <table className="w-full text-left text-sm">
          <thead className="bg-muted/40 text-xs text-muted-foreground">
            <tr>
              <th className="px-4 py-2.5 font-medium">时间</th>
              <th className="px-4 py-2.5 font-medium">触发</th>
              <th className="px-4 py-2.5 text-right font-medium">→Trakt</th>
              <th className="px-4 py-2.5 text-right font-medium">→Plex</th>
              <th className="px-4 py-2.5 text-right font-medium">冲突</th>
              <th className="px-4 py-2.5 text-right font-medium">未识别</th>
              <th className="px-4 py-2.5 font-medium">状态</th>
              <th className="w-8 px-2 py-2.5" />
            </tr>
          </thead>
          <tbody>
            {runs.length === 0 && (
              <tr>
                <td colSpan={8} className="px-4 py-8 text-center text-xs text-muted-foreground">
                  暂无同步记录——在「配置 → 观看同步」完成 Trakt 与 Plex 配对后，点击「立即同步」即可开始双向同步观看状态
                </td>
              </tr>
            )}
            {runs.map((run) => {
              const failed = !!run.error
              const expanded = expandedRun === run.id
              return (
                <Fragment key={run.id}>
                  <tr
                    className="cursor-pointer border-t border-border/40 transition-colors hover:bg-muted/40"
                    onClick={() => setExpandedRun(expanded ? null : run.id)}
                  >
                    <td className="px-4 py-2.5 text-xs">{relativeTime(run.started_at)}</td>
                    <td className="px-4 py-2.5">
                      <Badge variant="outline" className="border-border/70 bg-muted/50 text-xs">
                        {TRIGGER_LABELS[run.trigger] || run.trigger}
                      </Badge>
                    </td>
                    <td className="px-4 py-2.5 text-right tabular-nums">{run.to_trakt || '-'}</td>
                    <td className="px-4 py-2.5 text-right tabular-nums">{run.to_plex || '-'}</td>
                    <td className="px-4 py-2.5 text-right tabular-nums">{run.conflicts || '-'}</td>
                    <td className="px-4 py-2.5 text-right tabular-nums text-muted-foreground">{run.unresolved || '-'}</td>
                    <td className="px-4 py-2.5">
                      {run.finished_at == null ? (
                        <span className="inline-flex items-center gap-1 text-xs text-primary">
                          <Loader2 className="size-3 animate-spin" />进行中
                        </span>
                      ) : failed ? (
                        <span className="inline-flex items-center gap-1 text-xs text-destructive" title={run.error || ''}>
                          <AlertCircle className="size-3" />失败
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1 text-xs text-success">
                          <CheckCircle className="size-3" />完成
                        </span>
                      )}
                    </td>
                    <td className="px-2 py-2.5">
                      <ChevronDown className={`size-4 text-muted-foreground transition-transform ${expanded ? 'rotate-180' : ''}`} />
                    </td>
                  </tr>
                  {expanded && (
                    <tr className="border-t border-border/40 bg-muted/20">
                      <td colSpan={8} className="px-4 py-3">
                        <RunItems runId={run.id} running={run.finished_at == null} />
                      </td>
                    </tr>
                  )}
                </Fragment>
              )
            })}
          </tbody>
        </table>
      </div>

      {totalPages > 1 && (
        <div className="flex items-center justify-center gap-3 text-xs text-muted-foreground">
          <Button
            variant="outline"
            size="icon-sm"
            className="rounded-full"
            disabled={page === 0}
            onClick={() => setPage((p) => Math.max(0, p - 1))}
          >
            <ChevronLeft className="size-4" />
          </Button>
          <span className="tabular-nums">{page + 1} / {totalPages}</span>
          <Button
            variant="outline"
            size="icon-sm"
            className="rounded-full"
            disabled={page >= totalPages - 1}
            onClick={() => setPage((p) => p + 1)}
          >
            <ChevronRight className="size-4" />
          </Button>
        </div>
      )}
    </div>
  )
}
