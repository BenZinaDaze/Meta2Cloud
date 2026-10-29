import { useState, useEffect, useCallback, useRef, Fragment } from 'react'
import {
  RefreshCw,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  CheckCircle,
  AlertCircle,
  Clock,
  FileVideo,
  FolderSync,
  Loader2,
} from 'lucide-react'
import { getStrmSyncStatus, triggerStrmSync, listStrmSyncRuns, getStrmSyncRunFiles } from '@/api'
import { StatePanel } from '@/components/StatePanel'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import PageHeader from '@/components/shared/PageHeader'
import { usePolling } from '@/hooks/usePolling'
import { relativeTime } from '@/lib/time'
import type { StrmSyncRun, StrmSyncFile, StrmSyncStatus } from '@/types/api'

const TRIGGER_LABELS: Record<string, string> = {
  manual: '手动',
  schedule: '定时',
  ingest: '入库',
  reclassify: '重新归类',
  'title-pin-adopt': '更新译名',
  reidentify: '重新识别',
  'after-delete': '删除后同步',
}

const ACTION_LABELS: Record<string, { label: string; className: string }> = {
  created: { label: '新增', className: 'border-success/30 bg-success/10 text-success' },
  updated: { label: '更新', className: 'border-warning/30 bg-warning/10 text-warning' },
  removed: { label: '删除', className: 'border-destructive/30 bg-destructive/10 text-destructive' },
  skipped: { label: '跳过', className: 'border-border bg-muted text-muted-foreground' },
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

function RunFiles({ runId }: { runId: number }) {
  const [files, setFiles] = useState<StrmSyncFile[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    getStrmSyncRunFiles(runId)
      .then((res) => { if (!cancelled) setFiles(res.data?.files || []) })
      .catch((e) => { if (!cancelled) setError((e as Error).message) })
    return () => { cancelled = true }
  }, [runId])

  if (error) return <p className="px-3 py-2 text-xs text-destructive">明细加载失败：{error}</p>
  if (!files) return <p className="px-3 py-2 text-xs text-muted-foreground">加载中…</p>
  if (files.length === 0) return <p className="px-3 py-2 text-xs text-muted-foreground">无文件明细（全部跳过）</p>

  return (
    <div className="overflow-hidden rounded-lg border border-border/50">
      <table className="w-full text-left text-xs">
        <thead className="bg-muted/50 text-muted-foreground">
          <tr>
            <th className="px-3 py-2 font-medium">文件</th>
            <th className="hidden px-3 py-2 font-medium sm:table-cell">本地 STRM 路径</th>
            <th className="px-3 py-2 font-medium">动作</th>
            <th className="px-3 py-2 font-medium">结果</th>
          </tr>
        </thead>
        <tbody>
          {files.map((f) => {
            const action = ACTION_LABELS[f.action] || { label: f.action, className: 'border-border bg-muted text-muted-foreground' }
            return (
              <tr key={f.id} className="border-t border-border/40">
                <td className="max-w-[200px] truncate px-3 py-2 font-mono">{f.file_name || '-'}</td>
                <td className="hidden max-w-[320px] truncate px-3 py-2 font-mono text-muted-foreground sm:table-cell">{f.local_path || '-'}</td>
                <td className="px-3 py-2">
                  <Badge variant="outline" className={`gap-1 ${action.className}`}>{action.label}</Badge>
                </td>
                <td className="max-w-[220px] truncate px-3 py-2">
                  {f.success ? (
                    <span className="inline-flex items-center gap-1 text-success"><CheckCircle className="size-3" />成功</span>
                  ) : (
                    <span className="inline-flex items-center gap-1 text-destructive" title={f.error || ''}>
                      <AlertCircle className="size-3" />{f.error || '失败'}
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

interface StrmSyncPageProps {
  onToast?: (type: 'success' | 'error' | 'warning', title: string, message?: string) => void
}

export default function StrmSyncPage(props: StrmSyncPageProps) {
  const [status, setStatus] = useState<StrmSyncStatus | null>(null)
  const [runs, setRuns] = useState<StrmSyncRun[]>([])
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
        getStrmSyncStatus(),
        listStrmSyncRuns({ limit: PAGE_SIZE, offset: page * PAGE_SIZE }),
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

  // 同步进行中加密轮询，空闲时低速轮询
  usePolling(() => fetchData(), status?.running ? 3000 : 30000)

  const handleSync = useCallback(async () => {
    setSyncing(true)
    try {
      await triggerStrmSync()
      props.onToast?.('success', '同步已启动', 'STRM 同步正在后台执行')
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

  if (loading && !status) {
    return (
      <div className="flex flex-col gap-5">
        <h1 className="text-2xl font-bold leading-tight sm:text-[34px]">STRM 同步</h1>
        <StatePanel title="正在加载" description="STRM 同步状态加载中…" />
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        kicker="STRM Sync"
        title="STRM 同步"
        description="115 网盘媒体库 → 本地 STRM 文件的生成记录，播放时通过 302 直链实时串流。"
        actions={
          <Button onClick={handleSync} disabled={syncing || running || !status?.enabled} className="rounded-full">
            {running ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
            {running ? '同步中…' : '立即同步'}
          </Button>
        }
      />

      {!status?.enabled && (
        <div className="rounded-lg border border-warning/25 bg-warning/10 px-4 py-3 text-sm text-warning">
          STRM 功能未启用，请先在「配置 → STRM」中开启并配置路径映射。
        </div>
      )}

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatCard
          label="状态"
          icon={<FolderSync className="size-3" />}
          value={
            <span className={running ? 'text-primary' : ''}>
              {running ? '同步中' : status?.enabled ? '空闲' : '未启用'}
            </span>
          }
        />
        <StatCard label="STRM 总数" icon={<FileVideo className="size-3" />} value={status?.total_files ?? 0} />
        <StatCard label="映射目录" icon={<FolderSync className="size-3" />} value={status?.mappings ?? 0} />
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
          <span>新增 {status.last_run.created}</span>
          <span>更新 {status.last_run.updated}</span>
          <span>删除 {status.last_run.removed}</span>
          <span>跳过 {status.last_run.skipped}</span>
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
              <th className="px-4 py-2.5 text-right font-medium">新增</th>
              <th className="px-4 py-2.5 text-right font-medium">更新</th>
              <th className="px-4 py-2.5 text-right font-medium">删除</th>
              <th className="px-4 py-2.5 text-right font-medium">跳过</th>
              <th className="px-4 py-2.5 font-medium">状态</th>
              <th className="w-8 px-2 py-2.5" />
            </tr>
          </thead>
          <tbody>
            {runs.length === 0 && (
              <tr>
                <td colSpan={8} className="px-4 py-8 text-center text-xs text-muted-foreground">
                  暂无同步记录，点击「立即同步」开始生成 STRM
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
                    <td className="px-4 py-2.5 text-right tabular-nums text-success">{run.created || '-'}</td>
                    <td className="px-4 py-2.5 text-right tabular-nums">{run.updated || '-'}</td>
                    <td className="px-4 py-2.5 text-right tabular-nums text-destructive">{run.removed || '-'}</td>
                    <td className="px-4 py-2.5 text-right tabular-nums text-muted-foreground">{run.skipped || '-'}</td>
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
                        <RunFiles runId={run.id} />
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
