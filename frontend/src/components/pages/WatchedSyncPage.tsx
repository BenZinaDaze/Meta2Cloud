import { useState, useEffect, useCallback, useRef } from 'react'
import {
  RefreshCw,
  ChevronLeft,
  ChevronRight,
  Loader2,
  Clock,
  ArrowLeftRight,
} from 'lucide-react'
import {
  getWatchedSyncStatus,
  triggerWatchedSync,
  listWatchedSyncRuns,
} from '@/api'
import { StatePanel } from '@/components/StatePanel'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import PageHeader from '@/components/shared/PageHeader'
import StatCard from '@/components/shared/StatCard'
import { relativeTime } from '@/lib/time'
import { usePolling } from '@/hooks/usePolling'
import { TRIGGER_LABELS, WatchedRunsTable } from '@/components/watched/WatchedRunsTable'
import { WatchedIgnoreTab } from '@/components/watched/WatchedIgnoreTab'
import type { WatchedSyncStatus, WatchedSyncRun } from '@/types/api'

const PAGE_SIZE = 20

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
  // 折叠组的展开状态（可多组同开）：以组内最旧一条的 id 为键，轮询追加新记录不影响已展开状态
  const [expandedGroups, setExpandedGroups] = useState<Record<string, boolean>>({})
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
  usePolling(() => fetchData(), status?.running ? 3000 : 30000)

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
        <StatePanel title="正在加载" description="Trakt 同步状态加载中…" />
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        kicker="Trakt Sync"
        title="Trakt 同步"
        description="观看状态双向同步的运行记录与逐条明细，含时间、方向与结果。配对与同步设置在「配置 → 观看同步」。"
        actions={
          <Button onClick={handleSync} disabled={syncing || running || !ready} className="rounded-full">
            {running ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
            {running ? '同步中…' : '立即同步'}
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
              {running ? '同步中' : '空闲'}
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

      <Tabs defaultValue="runs">
        <TabsList className="h-auto gap-1 bg-card/40 p-1">
          <TabsTrigger value="runs" className="rounded-full px-4">同步记录</TabsTrigger>
          <TabsTrigger value="ignore" className="rounded-full px-4">忽略列表</TabsTrigger>
        </TabsList>
        <TabsContent value="runs" className="mt-4">
          <WatchedRunsTable
            runs={runs}
            expandedRun={expandedRun}
            expandedGroups={expandedGroups}
            onToggleRun={(id) => setExpandedRun(expandedRun === id ? null : id)}
            onToggleGroup={(key) => setExpandedGroups((prev) => ({ ...prev, [key]: !prev[key] }))}
            onToast={props.onToast}
          />

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
        </TabsContent>
        <TabsContent value="ignore" className="mt-4">
          <WatchedIgnoreTab onToast={props.onToast} />
        </TabsContent>
      </Tabs>
    </div>
  )
}
