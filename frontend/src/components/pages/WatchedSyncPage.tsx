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
  Ban,
  Undo2,
} from 'lucide-react'
import {
  getWatchedSyncStatus,
  triggerWatchedSync,
  listWatchedSyncRuns,
  getWatchedSyncRunItems,
  getWatchedSyncIgnores,
  setWatchedSyncIgnore,
} from '@/api'
import { StatePanel } from '@/components/StatePanel'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import PageHeader from '@/components/shared/PageHeader'
import { relativeTime } from '@/lib/time'
import { usePolling } from '@/hooks/usePolling'
import type {
  WatchedSyncRun,
  WatchedSyncRunItem,
  WatchedSyncStatus,
  WatchedSyncIgnore,
  WatchedFailingMedia,
} from '@/types/api'

/** 实时推送（sse/poll）相邻同媒体同结果的运行合并为一组：
 * 推送失败按退避每小时重试，同一媒体会连续产生多条同结果记录，折叠后只占一行 */
interface RunGroup {
  key: string
  runs: WatchedSyncRun[]
}

const SESSION_TRIGGERS = new Set(['sse', 'poll'])

function groupConsecutiveRuns(runs: WatchedSyncRun[]): RunGroup[] {
  const groups: RunGroup[] = []
  for (const run of runs) {
    // 进行中的运行永不合并；全量同步（manual/schedule）一轮就是一条，也不合并
    const key = SESSION_TRIGGERS.has(run.trigger) && run.title && run.finished_at != null
      ? `session|${run.title}|${run.error ? 'fail' : 'ok'}`
      : `run|${run.id}`
    const last = groups[groups.length - 1]
    if (last && last.key === key) {
      last.runs.push(run)
    } else {
      groups.push({ key, runs: [run] })
    }
  }
  return groups
}

/** 单条运行的「状态」列内容（普通行与折叠组行共用） */
function RunStatusCell({ run }: { run: WatchedSyncRun }) {
  if (run.finished_at == null) {
    return (
      <span className="inline-flex items-center gap-1 text-xs text-primary">
        <Loader2 className="size-3 animate-spin" />进行中
      </span>
    )
  }
  if (run.error) {
    return (
      <span className="inline-flex max-w-[200px] items-center gap-1 text-xs text-destructive" title={run.error || ''}>
        <AlertCircle className="size-3 shrink-0" />
        <span className="truncate">失败：{run.error}</span>
      </span>
    )
  }
  return (
    <span className="inline-flex items-center gap-1 text-xs text-success">
      <CheckCircle className="size-3" />完成
    </span>
  )
}

const TRIGGER_LABELS: Record<string, string> = {
  manual: '手动',
  schedule: '定时',
  activity: 'Trakt 活动',
  sse: '实时推送',
  poll: '会话轮询',
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

// 明细失败行上的快捷忽略按钮（媒体级，加完即生效，下轮同步起排除）。
function IgnoreItemButton({ mediaType, tmdbId, reason, onToast }: {
  mediaType: string
  tmdbId: number
  reason: string
  onToast?: (type: 'success' | 'error', title: string, message?: string) => void
}) {
  const [busy, setBusy] = useState(false)
  return (
    <Button
      variant="ghost"
      size="sm"
      disabled={busy}
      onClick={async () => {
        setBusy(true)
        try {
          const res = await setWatchedSyncIgnore({ action: 'add', media_type: mediaType, tmdb_id: tmdbId, reason })
          onToast?.('success', '已忽略', res.data?.message || '该媒体将不再参与同步')
        } catch (e) {
          const detail = (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail
          onToast?.('error', '忽略失败', detail || (e as Error).message)
        } finally {
          setBusy(false)
        }
      }}
      className="h-6 shrink-0 gap-1 rounded-full px-2 text-[11px] text-muted-foreground"
      title="忽略这部媒体（两侧不再同步，可在「忽略列表」恢复）"
    >
      {busy ? <Loader2 className="size-3 animate-spin" /> : <Ban className="size-3" />}
      忽略
    </Button>
  )
}

function RunItems({ runId, running, onToast }: {
  runId: number
  running: boolean
  onToast?: (type: 'success' | 'error', title: string, message?: string) => void
}) {
  const [items, setItems] = useState<WatchedSyncRunItem[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  // 序列守卫：runId 切换后丢弃过期响应（轮询与补拉并存时防乱序覆盖）
  const seq = useRef(0)
  const load = useCallback(async () => {
    const id = ++seq.current
    try {
      const res = await getWatchedSyncRunItems(runId, { limit: 1000 })
      if (id !== seq.current) return
      setItems(res.data?.items || [])
    } catch (e) {
      if (id === seq.current) setError((e as Error).message)
    }
  }, [runId])

  useEffect(() => {
    load()
  }, [load, running])

  // 同步进行中每 2 秒刷新，实现「同步一条显示一条」；结束后上面的补拉覆盖完整明细
  usePolling(load, 2000, { enabled: running })

  if (error) return <p className="px-3 py-2 text-xs text-destructive">明细加载失败：{error}</p>
  if (!items) return <p className="px-3 py-2 text-xs text-muted-foreground">加载中…</p>
  if (items.length === 0) {
    return (
      <p className="px-3 py-2 text-xs text-muted-foreground">
        {running ? '正在比对两侧观看状态，明细将逐条显示…' : '无逐条明细（本轮无变更）'}
      </p>
    )
  }

  return (
    <div className="overflow-hidden rounded-lg border border-border/50">
      {running && (
        <div className="flex items-center gap-2 border-b border-border/40 bg-muted/30 px-3 py-1.5 text-xs text-muted-foreground">
          <Loader2 className="size-3 animate-spin text-primary" />
          同步进行中，已处理 {items.length} 条；Trakt 按批次提交，明细按批显示
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
                    <span className="flex items-center gap-2">
                      <span className="inline-flex min-w-0 items-center gap-1 text-destructive" title={it.error || ''}>
                        <AlertCircle className="size-3 shrink-0" />
                        <span className="truncate">{it.error || '失败'}</span>
                      </span>
                      {it.media_type && it.tmdb_id && (
                        <IgnoreItemButton mediaType={it.media_type} tmdbId={it.tmdb_id} reason={it.error || ''} onToast={onToast} />
                      )}
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

// 忽略列表 tab：反复失败的候选（一键忽略）+ 已忽略媒体（可恢复）。
// 忽略是媒体级的——整部电影/整部剧两侧均不再参与同步，恢复后下轮自动纳入。
function IgnoreTab({ onToast }: { onToast?: (type: 'success' | 'error', title: string, message?: string) => void }) {
  const [data, setData] = useState<{ ignores: WatchedSyncIgnore[]; candidates: WatchedFailingMedia[] } | null>(null)
  const [busy, setBusy] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      const res = await getWatchedSyncIgnores()
      setData({ ignores: res.data?.ignores || [], candidates: res.data?.candidates || [] })
    } catch { /* 静默，下次操作后刷新 */ }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const act = async (action: 'add' | 'remove', mediaType: string, tmdbId: number, reason?: string) => {
    const key = `${mediaType}:${tmdbId}`
    setBusy(key)
    try {
      const res = await setWatchedSyncIgnore({ action, media_type: mediaType, tmdb_id: tmdbId, reason })
      onToast?.('success', action === 'add' ? '已忽略' : '已恢复', res.data?.message)
      await load()
    } catch (e) {
      const detail = (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail
      onToast?.('error', '操作失败', detail || (e as Error).message)
    } finally {
      setBusy(null)
    }
  }

  if (!data) return <p className="py-6 text-center text-xs text-muted-foreground">加载中…</p>

  return (
    <div className="space-y-6">
      {/* 候选区：近 7 天失败 ≥2 次且未忽略的媒体 */}
      <div>
        <div className="mb-2 flex items-center gap-2 text-[13px] font-medium">
          <AlertCircle className="size-3.5 text-warning" />
          反复同步失败的媒体
          <span className="text-[11px] font-normal text-muted-foreground">近 7 天失败 ≥2 次；持续失败通常为 Trakt 缺少该媒体的 TMDB 映射，可选择忽略</span>
        </div>
        {data.candidates.length === 0 ? (
          <p className="rounded-lg border border-border/50 px-4 py-5 text-center text-xs text-muted-foreground">
            没有反复失败的媒体
          </p>
        ) : (
          <div className="overflow-hidden rounded-xl border border-border/60">
            {data.candidates.map((c) => {
              const key = `${c.media_type}:${c.tmdb_id}`
              return (
                <div key={key} className="flex items-center gap-3 border-b border-border/40 px-4 py-2.5 last:border-b-0">
                  <DirectionIcon mediaType={c.media_type} />
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm">{c.title || `TMDB ${c.tmdb_id}`}</div>
                    <div className="truncate text-[11px] text-muted-foreground" title={c.last_error || ''}>
                      失败 {c.fails} 次 · {c.last_error || '未知原因'} · 最近 {relativeTime(c.last_at)}
                    </div>
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={busy === key}
                    onClick={() => act('add', c.media_type, c.tmdb_id, c.last_error || '')}
                    className="shrink-0 rounded-full"
                  >
                    {busy === key ? <Loader2 className="size-3.5 animate-spin" /> : <Ban className="size-3.5" />}
                    忽略
                  </Button>
                </div>
              )
            })}
          </div>
        )}
      </div>

      {/* 已忽略区 */}
      <div>
        <div className="mb-2 flex items-center gap-2 text-[13px] font-medium">
          <Ban className="size-3.5 text-muted-foreground" />
          已忽略的媒体
          <span className="text-[11px] font-normal text-muted-foreground">两侧均不参与同步；恢复后下轮同步自动纳入</span>
        </div>
        {data.ignores.length === 0 ? (
          <p className="rounded-lg border border-border/50 px-4 py-5 text-center text-xs text-muted-foreground">
            暂无忽略的媒体
          </p>
        ) : (
          <div className="overflow-hidden rounded-xl border border-border/60">
            {data.ignores.map((it) => {
              const key = `${it.media_type}:${it.tmdb_id}`
              return (
                <div key={key} className="flex items-center gap-3 border-b border-border/40 px-4 py-2.5 last:border-b-0">
                  <DirectionIcon mediaType={it.media_type} />
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm">{it.title || `TMDB ${it.tmdb_id}`}</div>
                    <div className="truncate text-[11px] text-muted-foreground" title={it.reason || ''}>
                      {it.reason ? `${it.reason} · ` : ''}忽略于 {relativeTime(it.created_at)}
                    </div>
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={busy === key}
                    onClick={() => act('remove', it.media_type, it.tmdb_id)}
                    className="shrink-0 rounded-full"
                  >
                    {busy === key ? <Loader2 className="size-3.5 animate-spin" /> : <Undo2 className="size-3.5" />}
                    恢复
                  </Button>
                </div>
              )
            })}
          </div>
        )}
      </div>
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
  const runGroups = groupConsecutiveRuns(runs)

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
      <div className="overflow-hidden rounded-xl border border-border/60 bg-card/50">
        <table className="w-full text-left text-sm">
          <thead className="bg-muted/40 text-xs text-muted-foreground">
            <tr>
              <th className="w-[130px] px-4 py-2.5 font-medium">时间</th>
              <th className="w-[128px] px-4 py-2.5 font-medium">触发</th>
              <th className="hidden max-w-[380px] px-3 py-2.5 font-medium md:table-cell">内容</th>
              <th className="w-[68px] px-3 py-2.5 text-right font-medium">→Trakt</th>
              <th className="w-[68px] px-3 py-2.5 text-right font-medium">→Plex</th>
              <th className="w-[68px] px-3 py-2.5 text-right font-medium">冲突</th>
              <th className="w-[72px] px-3 py-2.5 text-right font-medium">未识别</th>
              <th className="w-[220px] px-4 py-2.5 font-medium">状态</th>
              <th className="w-8 px-2 py-2.5" />
            </tr>
          </thead>
          <tbody>
            {runs.length === 0 && (
              <tr>
                <td colSpan={9} className="px-4 py-8 text-center text-xs text-muted-foreground">
                  暂无同步记录——在「配置 → 观看同步」完成 Trakt 与 Plex 配对后，点击「立即同步」即可开始双向同步观看状态
                </td>
              </tr>
            )}
            {runGroups.map((group) => {
              const latest = group.runs[0] // 接口按时间倒序，首条即最近一次
              const n = group.runs.length
              const isGroup = n > 1
              // 以组内最旧一条的 id 作为展开状态键：轮询追加新记录不影响已展开状态
              const groupKey = `g${group.runs[group.runs.length - 1].id}`
              const open = isGroup && !!expandedGroups[groupKey]
              const singleExpanded = !isGroup && expandedRun === latest.id
              return (
                <Fragment key={groupKey}>
                  <tr
                    className={`border-t border-border/40 transition-colors ${
                      open
                        ? 'cursor-pointer bg-primary/[0.06] hover:bg-primary/[0.08]'
                        : 'cursor-pointer hover:bg-muted/30'
                    }`}
                    onClick={
                      isGroup
                        ? () => setExpandedGroups((prev) => ({ ...prev, [groupKey]: !prev[groupKey] }))
                        : () => setExpandedRun(singleExpanded ? null : latest.id)
                    }
                  >
                    <td className="whitespace-nowrap px-4 py-2.5 text-xs">
                      <span className="inline-flex items-center gap-1">
                        {isGroup && (
                          <ChevronDown
                            className={`size-3.5 shrink-0 text-primary transition-transform ${open ? '' : '-rotate-90'}`}
                          />
                        )}
                        {relativeTime(latest.started_at)}
                      </span>
                    </td>
                    <td className="px-4 py-2.5">
                      <Badge variant="outline" className="whitespace-nowrap border-border/70 bg-muted/50 text-xs">
                        {TRIGGER_LABELS[latest.trigger] || latest.trigger}
                      </Badge>
                    </td>
                    <td className="hidden max-w-[380px] px-3 py-2.5 text-xs md:table-cell" title={latest.title ?? ''}>
                      <span className="flex items-center gap-2">
                        <span className="truncate">
                          {latest.title ? `${latest.title} · ${Math.round((latest.progress ?? 0) * 100)}%` : '—'}
                        </span>
                        {isGroup && (
                          <Badge variant="outline" className="shrink-0 border-primary/30 bg-primary/10 text-[10px] text-primary">
                            ×{n}
                          </Badge>
                        )}
                      </span>
                    </td>
                    <td className="px-4 py-2.5 text-right tabular-nums">{latest.to_trakt || '-'}</td>
                    <td className="px-4 py-2.5 text-right tabular-nums">{latest.to_plex || '-'}</td>
                    <td className="px-4 py-2.5 text-right tabular-nums">{latest.conflicts || '-'}</td>
                    <td className="px-4 py-2.5 text-right tabular-nums text-muted-foreground">{latest.unresolved || '-'}</td>
                    <td className="px-4 py-2.5">
                      <RunStatusCell run={latest} />
                    </td>
                    <td className="w-8 px-2 py-2.5" />
                  </tr>
                  {open && group.runs.slice(1).map((r) => (
                    <tr key={r.id} className="border-t border-border/40 hover:bg-muted/30">
                      <td className="px-4 py-2.5 text-xs text-muted-foreground">{relativeTime(r.started_at)}</td>
                      <td className="px-4 py-2.5">
                        <Badge variant="outline" className="whitespace-nowrap border-border/70 bg-muted/50 text-xs">
                          {TRIGGER_LABELS[r.trigger] || r.trigger}
                        </Badge>
                      </td>
                      <td className="hidden max-w-[380px] truncate px-3 py-2.5 text-xs text-muted-foreground md:table-cell">
                        {r.title ? `${r.title} · ${Math.round((r.progress ?? 0) * 100)}%` : '—'}
                      </td>
                      <td className="px-4 py-2.5 text-right tabular-nums text-muted-foreground">{r.to_trakt || '-'}</td>
                      <td className="px-4 py-2.5 text-right tabular-nums text-muted-foreground">{r.to_plex || '-'}</td>
                      <td className="px-4 py-2.5 text-right tabular-nums text-muted-foreground">{r.conflicts || '-'}</td>
                      <td className="px-4 py-2.5 text-right tabular-nums text-muted-foreground">{r.unresolved || '-'}</td>
                      <td className="px-4 py-2.5">
                        <RunStatusCell run={r} />
                      </td>
                      <td className="w-8 px-2 py-2.5" />
                    </tr>
                  ))}
                  {singleExpanded && (
                    <tr className="border-t border-border/40 bg-muted/20">
                      <td colSpan={9} className="px-4 py-3">
                        <RunItems runId={latest.id} running={latest.finished_at == null} onToast={props.onToast} />
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
        </TabsContent>
        <TabsContent value="ignore" className="mt-4">
          <IgnoreTab onToast={props.onToast} />
        </TabsContent>
      </Tabs>
    </div>
  )
}
