import { useEffect, useMemo, useRef, useState } from 'react'
import {
  RefreshCw,
  Pause,
  Play,
  Trash2,
  RotateCcw,
  Search,
  Upload,
  Link,
  ChevronLeft,
  ChevronRight,
} from 'lucide-react'
import {
  addAria2Torrent,
  addAria2Uri,
  getAria2Overview,
  pauseAria2Tasks,
  removeAria2Tasks,
  retryAria2Tasks,
  unpauseAria2Tasks,
} from '@/api'
import { StatePanel } from '@/components/StatePanel'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
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
import ParseTestModal from '@/components/modals/ParseTestModal'
import type { Aria2Task, Aria2Overview } from '@/types/api'
import { toast } from 'sonner'

function formatBytes(bytes: string | number): string {
  const value = Number(bytes) || 0
  if (value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  const size = value / 1024 ** index
  return `${size >= 100 ? size.toFixed(0) : size.toFixed(1)} ${units[index]}`
}

function formatSpeed(bytes: string | number): string {
  return `${formatBytes(bytes)}/s`
}

function statusLabel(status: string): string {
  return {
    active: '下载中',
    waiting: '队列中',
    paused: '已暂停',
    complete: '已完成',
    error: '失败',
    removed: '已移除',
    stopped: '已停止',
  }[status] || status
}

function SummaryCard({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="flex min-h-[132px] flex-col justify-between gap-3 rounded-2xl border bg-card p-5">
      <div className="space-y-2">
        <span className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">{label}</span>
        <span className="block text-3xl font-bold tabular-nums">{value}</span>
        {sub && <span className="block text-xs leading-5 text-muted-foreground">{sub}</span>}
      </div>
    </div>
  )
}

type QueueItemTask = Aria2Task & {
  name?: string
  dir?: string
  errorMessage?: string
}

type Aria2OverviewWithVersionObject = Omit<Aria2Overview, 'version'> & {
  version?: string | {
    version?: string
    enabledFeatures?: string[]
  }
}

const PAGE_SIZE = 20

function getVersionLabel(overview: Aria2Overview | null): string {
  const version = (overview as Aria2OverviewWithVersionObject | null)?.version
  if (!version) return '--'
  return typeof version === 'string' ? version : version.version || '--'
}

function TaskCard({
  task,
  selected,
  onToggleSelect,
  onOpen,
  pendingAction,
  onPause,
  onResume,
  onRemove,
  onRetry,
}: {
  task: Aria2Task
  selected: boolean
  onToggleSelect: (gid: string) => void
  onOpen: (task: Aria2Task) => void
  pendingAction: string | null
  onPause: (gid: string) => void
  onResume: (gid: string) => void
  onRemove: (gid: string) => void
  onRetry: (gid: string) => void
}) {
  const errored = task.status === 'error'
  const pausable = task.status === 'active' || task.status === 'waiting'
  const resumable = task.status === 'paused'
  const queueItem = task as QueueItemTask
  const completedLength = Number(task.completedLength) || 0
  const totalLength = Math.max(Number(task.totalLength) || 0, 1)
  const progress = (completedLength / totalLength) * 100
  const displayName = queueItem.name || task.filename || task.title || '未知任务'
  const directory = queueItem.dir || task.title || task.filename || '未指定目录'
  const errorMessage = task.error || queueItem.errorMessage

  return (
    <div
      onClick={() => onOpen(task)}
      className="cursor-pointer rounded-2xl border bg-card p-4 transition-all hover:bg-muted/50 sm:p-5"
    >
      <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div className="min-w-0 flex-1 flex items-start gap-2 sm:gap-4">
          <div
            className="flex-shrink-0 cursor-pointer p-3 -ml-3 -mt-2.5 transition-opacity hover:opacity-80"
            onClick={(e) => { e.stopPropagation(); onToggleSelect(task.gid) }}
          >
            <div
              className={`flex size-5 items-center justify-center rounded transition-colors ${
                selected ? 'bg-brand text-primary-foreground' : 'border bg-muted'
              }`}
            >
              {selected && (
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3" className="size-3">
                  <polyline points="20 6 9 17 4 12" />
                </svg>
              )}
            </div>
          </div>

          <div className="min-w-0 flex-1">
            <div className="mb-2 flex flex-wrap items-center gap-2">
              <span
                className={`rounded-full px-2.5 py-0.5 text-[11px] font-semibold uppercase tracking-wider ${
                  errored ? 'bg-destructive/10 text-destructive' : 'bg-brand/10 text-brand'
                }`}
              >
                {statusLabel(task.status)}
              </span>
              <span className="text-xs font-mono text-muted-foreground">{task.gid}</span>
              <div className="ml-auto">
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-8 text-muted-foreground hover:text-destructive hover:bg-destructive/10"
                  onClick={(e) => { e.stopPropagation(); onRemove(task.gid) }}
                  disabled={pendingAction === 'remove'}
                  aria-label="移除任务"
                >
                  <Trash2 className="size-4" />
                </Button>
              </div>
            </div>

            <div className="line-clamp-2 text-base font-semibold leading-snug sm:text-lg">
              {displayName}
            </div>
            <div className="mt-1.5 line-clamp-2 text-xs leading-5 text-muted-foreground sm:mt-2 sm:text-sm">
              {directory}
            </div>

            <div className="mt-3 sm:mt-4">
              <div className="mb-2 flex items-center justify-between text-xs text-muted-foreground">
                <span>{formatBytes(task.completedLength)} / {formatBytes(task.totalLength)}</span>
                <span>{progress.toFixed(1)}%</span>
              </div>
              <div className="h-2 overflow-hidden rounded-full bg-muted">
                <div
                  className={`h-full rounded-full transition-all ${
                    errored ? 'bg-destructive' : 'bg-brand'
                  }`}
                  style={{
                    width: `${Math.min(progress, 100)}%`,
                  }}
                />
              </div>
            </div>

            <div className="mt-3 grid grid-cols-2 gap-2 text-sm sm:mt-4 sm:gap-3 md:grid-cols-4">
              <div className="rounded-2xl border bg-muted/50 p-2.5">
                <div className="text-[11px] text-muted-foreground">下载速度</div>
                <div className="mt-1 text-sm font-semibold">{formatSpeed(task.downloadSpeed)}</div>
              </div>
              <div className="hidden rounded-2xl border bg-muted/50 p-2.5 md:block">
                <div className="text-[11px] text-muted-foreground">上传速度</div>
                <div className="mt-1 text-sm font-semibold">{formatSpeed(task.uploadSpeed || 0)}</div>
              </div>
              <div className="rounded-2xl border bg-muted/50 p-2.5">
                <div className="text-[11px] text-muted-foreground">文件数</div>
                <div className="mt-1 text-sm font-semibold">{task.totalLength}</div>
              </div>
              <div className="hidden rounded-2xl border bg-muted/50 p-2.5 md:block">
                <div className="text-[11px] text-muted-foreground">连接数</div>
                <div className="mt-1 text-sm font-semibold">{task.connections || '-'}</div>
              </div>
            </div>

            {errorMessage && (
              <div className="mt-4 rounded-2xl border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
                {errorMessage}
              </div>
            )}
          </div>
        </div>

        <div className="flex flex-wrap gap-2 xl:ml-auto xl:max-w-[180px] xl:justify-end">
          {pausable && (
            <Button
              variant="outline"
              size="sm"
              onClick={(e) => { e.stopPropagation(); onPause(task.gid) }}
              disabled={pendingAction === 'pause'}
            >
              {pendingAction === 'pause' ? '暂停中…' : '暂停'}
            </Button>
          )}
          {resumable && (
            <Button
              variant="outline"
              size="sm"
              onClick={(e) => { e.stopPropagation(); onResume(task.gid) }}
              disabled={pendingAction === 'resume'}
            >
              {pendingAction === 'resume' ? '继续中…' : '继续'}
            </Button>
          )}
          {errored && (
            <Button
              variant="outline"
              size="sm"
              onClick={(e) => { e.stopPropagation(); onRetry(task.gid) }}
              disabled={pendingAction === 'retry'}
            >
              {pendingAction === 'retry' ? '重试中…' : '重试'}
            </Button>
          )}
        </div>
      </div>
    </div>
  )
}

function ActionPanel({
  uriInput,
  setUriInput,
  onSubmitUri,
  onTorrentChange,
  torrentName,
}: {
  uriInput: string
  setUriInput: (v: string) => void
  onSubmitUri: () => void
  onTorrentChange: (e: React.ChangeEvent<HTMLInputElement>) => void
  torrentName: string
}) {
  return (
    <div className="rounded-2xl border bg-muted/30 p-4 sm:p-5">
      <div className="mb-3 text-[11px] font-semibold uppercase tracking-widest text-brand">
        新建下载
      </div>
      <Textarea
        value={uriInput}
        onChange={(e) => setUriInput(e.target.value)}
        placeholder="每行一个链接，支持 HTTP、HTTPS、FTP、磁力链"
        className="h-24 resize-none"
      />
      <div className="mt-3 flex flex-col gap-2.5 sm:flex-row sm:flex-wrap sm:items-center">
        <Button onClick={onSubmitUri} className="gap-2">
          <Link className="size-4" />
          添加链接下载
        </Button>
        <label className="inline-flex cursor-pointer items-center justify-center gap-2 rounded-lg border bg-muted px-4 py-2 text-sm font-medium transition-colors hover:bg-muted/80">
          <Upload className="size-4" />
          上传 Torrent
          <input type="file" accept=".torrent,application/x-bittorrent" hidden onChange={onTorrentChange} />
        </label>
        {torrentName && <span className="text-xs text-muted-foreground">{torrentName}</span>}
      </div>
    </div>
  )
}

interface DownloadsPageProps {
  queue?: string
  onChangeQueue?: (queue: string) => void
  onToast?: (type: 'success' | 'error' | 'warning', title: string, message?: string) => void
  aria2Enabled?: boolean | null
}

export default function DownloadsPage({
  queue = 'all',
  onChangeQueue,
  aria2Enabled = null,
}: DownloadsPageProps) {
  const [overview, setOverview] = useState<Aria2Overview | null>(null)
  const [uriInput, setUriInput] = useState('')
  const [torrentName, setTorrentName] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [selectedTask, setSelectedTask] = useState<Aria2Task | null>(null)
  const [parseTestFile, setParseTestFile] = useState<string | null>(null)
  const [pendingActions, setPendingActions] = useState<Record<string, string>>({})
  const [confirmRemoveGids, setConfirmRemoveGids] = useState<string[] | null>(null)
  const [selectedGids, setSelectedGids] = useState<Set<string>>(new Set())
  const [searchQuery, setSearchQuery] = useState('')
  const [debouncedSearchQuery, setDebouncedSearchQuery] = useState('')
  const [page, setPage] = useState(1)
  const showDashboard = queue === 'all'

  useEffect(() => {
    const timer = setTimeout(() => setDebouncedSearchQuery(searchQuery), 250)
    return () => clearTimeout(timer)
  }, [searchQuery])

  const pollTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const errorCount = useRef(0)

  useEffect(() => {
    if (aria2Enabled === null) {
      setLoading(true)
      return
    }

    if (!aria2Enabled) {
      setLoading(false)
      return
    }

    let cancelled = false

    async function poll(silent = false) {
      try {
        if (!silent) setLoading(true)
        const overviewRes = await getAria2Overview({ queue, page, page_size: PAGE_SIZE, search: debouncedSearchQuery })
        if (cancelled) return
        setOverview(overviewRes.data)
        setError(null)
        errorCount.current = 0
      } catch (e) {
        if (cancelled) return
        const detail = (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail
        setError(detail || '下载中心加载失败')
        errorCount.current += 1
      } finally {
        if (!cancelled) {
          if (!silent) setLoading(false)
          const delays = [5000, 10000, 20000, 30000]
          const delay = delays[Math.min(errorCount.current, delays.length - 1)]
          pollTimer.current = setTimeout(() => poll(true), delay)
        }
      }
    }

    poll()
    return () => {
      cancelled = true
      if (pollTimer.current) clearTimeout(pollTimer.current)
    }
  }, [aria2Enabled, page, queue, debouncedSearchQuery])

  const tasks = useMemo(() => overview?.items || [], [overview])

  useEffect(() => {
    setPage(1)
    setSelectedGids(new Set())
  }, [queue, debouncedSearchQuery])

  async function withAction(
    action: () => Promise<unknown>,
    successMessage: string,
    gid: string | null = null,
    actionName: string | null = null
  ) {
    try {
      setBusy(true)
      if (gid && actionName) {
        setPendingActions((prev) => ({ ...prev, [gid]: actionName }))
      }
      await action()
      const overviewRes = await getAria2Overview({ queue, page, page_size: PAGE_SIZE, search: debouncedSearchQuery })
      setOverview(overviewRes.data)
      toast.success('下载管理', { description: successMessage })
    } catch (e) {
      const detail = (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail
      toast.error('下载管理', { description: detail || '操作失败' })
    } finally {
      if (gid) {
        setPendingActions((prev) => {
          const next = { ...prev }
          delete next[gid]
          return next
        })
      }
      setBusy(false)
    }
  }

  async function handleRefreshOverview() {
    try {
      setBusy(true)
      const overviewRes = await getAria2Overview({ queue, page, page_size: PAGE_SIZE, search: debouncedSearchQuery })
      setOverview(overviewRes.data)
      setError(null)
      errorCount.current = 0
    } catch (e) {
      const detail = (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail
      setError(detail || '下载中心加载失败')
    } finally {
      setBusy(false)
    }
  }

  function handleToggleSelect(gid: string) {
    setSelectedGids((prev) => {
      const next = new Set(prev)
      if (next.has(gid)) next.delete(gid)
      else next.add(gid)
      return next
    })
  }

  function handleRemoveTask(gid: string) {
    setConfirmRemoveGids([gid])
  }

  async function confirmRemoveTask() {
    if (!confirmRemoveGids?.length) return
    const isSingle = confirmRemoveGids.length === 1
    const gid = isSingle ? confirmRemoveGids[0] : null
    await withAction(
      () => removeAria2Tasks(confirmRemoveGids),
      isSingle ? '任务已移除' : `已移除 ${confirmRemoveGids.length} 个任务`,
      gid,
      'remove'
    )
    setConfirmRemoveGids(null)
    setSelectedGids(new Set())
  }

  async function handleSubmitUri() {
    const uris = uriInput.split('\n').map((v) => v.trim()).filter(Boolean)
    if (!uris.length) {
      toast.warning('下载管理', { description: '请先输入至少一个下载链接' })
      return
    }
    await withAction(async () => {
      await Promise.all(uris.map((uri) => addAria2Uri({ uris: [uri] })))
      setUriInput('')
    }, '已添加下载任务')
  }

  async function handleTorrentChange(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = async () => {
      const result = String(reader.result || '')
      const base64 = result.includes(',') ? result.split(',')[1] : result
      await withAction(async () => {
        await addAria2Torrent({ torrent: base64 })
        setTorrentName(file.name)
      }, '已添加 Torrent 任务')
    }
    reader.readAsDataURL(file)
  }

  const summary = overview?.summary
  const pagination = overview?.pagination
  const totalPages = Math.max(pagination?.total_pages ?? 1, 1)
  const totalItems = pagination?.total ?? tasks.length
  const canPageBack = page > 1 && !busy && !loading
  const canPageForward = page < totalPages && !busy && !loading

  function handlePageChange(nextPage: number) {
    const clampedPage = Math.min(Math.max(nextPage, 1), totalPages)
    if (clampedPage === page) return
    setPage(clampedPage)
    setSelectedGids(new Set())
  }

  return (
    <div className="flex flex-1 flex-col gap-5">
      <div className="mb-2 flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
        <div className="max-w-2xl">
          <div className="mb-2 text-[11px] font-semibold uppercase tracking-widest text-brand">
            Native Download Center
          </div>
          <h1 className="text-[28px] font-bold leading-tight sm:text-[34px]">
            {showDashboard ? '下载管理' : statusLabel(queue)}
          </h1>
        </div>

        <div className="flex flex-nowrap items-center gap-2 overflow-x-auto px-1 sm:flex-wrap sm:overflow-visible sm:px-0">
          <Button variant="outline" size="sm" onClick={handleRefreshOverview} disabled={busy || !aria2Enabled}>
            <RefreshCw className="size-4" />
            刷新
          </Button>
        </div>
      </div>

      {showDashboard && summary && (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <SummaryCard label="下载速度" value={formatSpeed(summary.downloadSpeed || 0)} sub="当前全局吞吐" />
          <SummaryCard label="上传速度" value={formatSpeed(summary.uploadSpeed || 0)} sub="适用于 BT 做种场景" />
          <SummaryCard
            label="活跃任务"
            value={String(summary.activeCount ?? '--')}
            sub={`等待 ${summary.waitingCount ?? '--'} · 已停止 ${summary.stoppedCount ?? '--'}`}
          />
          <SummaryCard label="Aria2 版本" value={getVersionLabel(overview)} sub="等待连接" />
        </div>
      )}

      <div className="flex flex-col gap-4">
        <div className="flex flex-nowrap gap-2 overflow-x-auto sm:flex-wrap">
          {['all', 'active', 'waiting', 'stopped'].map((q) => {
            const count = q === 'all'
              ? (summary?.activeCount || 0) + (summary?.waitingCount || 0) + (summary?.stoppedCount || 0)
              : q === 'active'
                ? (summary?.activeCount || 0)
                : q === 'waiting'
                  ? (summary?.waitingCount || 0)
                  : (summary?.stoppedCount || 0)
            return (
              <Button
                key={q}
                variant={queue === q ? 'default' : 'outline'}
                size="sm"
                onClick={() => onChangeQueue?.(q)}
                className="rounded-full"
              >
                {statusLabel(q)} · {count}
              </Button>
            )
          })}
        </div>

        <div className="w-full sm:max-w-xs">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              type="text"
              placeholder="搜索任务名称 或 GID..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="h-11 rounded-full pl-10"
            />
          </div>
        </div>
      </div>

      {showDashboard && (
        <ActionPanel
          uriInput={uriInput}
          setUriInput={setUriInput}
          onSubmitUri={handleSubmitUri}
          onTorrentChange={handleTorrentChange}
          torrentName={torrentName}
        />
      )}

      {loading && !overview && (
        <StatePanel title="正在连接 aria2" description="下载中心正在初始化任务状态和运行信息。" compact />
      )}

      {!loading && !error && !overview && !aria2Enabled && (
        <StatePanel
          icon="⤓"
          title="Aria2 集成已关闭"
          description="请先在配置页启用 Aria2，再使用下载中心。"
          compact
        />
      )}

      {error && (
        <StatePanel icon="!" title={error} description="请检查 aria2 连接状态，或稍后重试。" tone="danger" compact />
      )}

      {!loading && !error && (
        <div className="space-y-4">
          {tasks.length === 0 ? (
            <StatePanel
              icon="⤓"
              title="当前分组没有任务"
              description="换一个任务分组，或者添加新的下载任务。"
            />
          ) : (
            tasks.map((task) => (
              <TaskCard
                key={task.gid}
                task={task}
                selected={selectedGids.has(task.gid)}
                onToggleSelect={handleToggleSelect}
                onOpen={setSelectedTask}
                pendingAction={pendingActions[task.gid] || null}
                onPause={(gid) => withAction(() => pauseAria2Tasks([gid]), '任务已暂停', gid, 'pause')}
                onResume={(gid) => withAction(() => unpauseAria2Tasks([gid]), '任务已继续', gid, 'resume')}
                onRemove={handleRemoveTask}
                onRetry={(gid) => withAction(() => retryAria2Tasks([gid]), '任务已重新加入队列', gid, 'retry')}
              />
            ))
          )}

          {pagination && totalPages > 1 && (
            <div className="flex flex-col gap-3 rounded-2xl border bg-card p-4 sm:flex-row sm:items-center sm:justify-between">
              <div className="text-sm text-muted-foreground">
                第 <span className="font-semibold text-foreground">{page}</span> / {totalPages} 页，共 {totalItems} 个任务
              </div>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => handlePageChange(page - 1)}
                  disabled={!canPageBack}
                >
                  <ChevronLeft className="size-4" />
                  上一页
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => handlePageChange(page + 1)}
                  disabled={!canPageForward}
                >
                  下一页
                  <ChevronRight className="size-4" />
                </Button>
              </div>
            </div>
          )}
        </div>
      )}

      <Dialog open={!!selectedTask} onOpenChange={(open) => !open && setSelectedTask(null)}>
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>{selectedTask?.filename || selectedTask?.title || '任务详情'}</DialogTitle>
            <DialogDescription className="sr-only">查看下载任务详情</DialogDescription>
          </DialogHeader>
          {selectedTask && (
            <div className="space-y-4">
              <div className="flex flex-wrap gap-2">
                <span
                  className={`rounded-full px-2.5 py-0.5 text-[11px] font-semibold uppercase tracking-wider ${
                    selectedTask.status === 'error' ? 'bg-destructive/10 text-destructive' : 'bg-brand/10 text-brand'
                  }`}
                >
                  {statusLabel(selectedTask.status)}
                </span>
                <span className="text-xs font-mono text-muted-foreground">{selectedTask.gid}</span>
              </div>

              <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
                <div className="rounded-xl border bg-muted/50 p-3">
                  <div className="text-[11px] uppercase tracking-wider text-muted-foreground">下载速度</div>
                  <div className="mt-1 text-sm font-semibold">{formatSpeed(selectedTask.downloadSpeed)}</div>
                </div>
                <div className="rounded-xl border bg-muted/50 p-3">
                  <div className="text-[11px] uppercase tracking-wider text-muted-foreground">上传速度</div>
                  <div className="mt-1 text-sm font-semibold">{formatSpeed(selectedTask.uploadSpeed || 0)}</div>
                </div>
                <div className="rounded-xl border bg-muted/50 p-3">
                  <div className="text-[11px] uppercase tracking-wider text-muted-foreground">连接数</div>
                  <div className="mt-1 text-sm font-semibold">{selectedTask.connections || '-'}</div>
                </div>
              </div>

              <div className="flex flex-wrap gap-2">
                {(selectedTask.status === 'active' || selectedTask.status === 'waiting') && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      pauseAria2Tasks([selectedTask.gid])
                      setSelectedTask(null)
                    }}
                  >
                    <Pause className="size-4" />
                    暂停
                  </Button>
                )}
                {selectedTask.status === 'paused' && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      unpauseAria2Tasks([selectedTask.gid])
                      setSelectedTask(null)
                    }}
                  >
                    <Play className="size-4" />
                    继续
                  </Button>
                )}
                {selectedTask.status === 'error' && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      retryAria2Tasks([selectedTask.gid])
                      setSelectedTask(null)
                    }}
                  >
                    <RotateCcw className="size-4" />
                    重试
                  </Button>
                )}
                <Button
                  variant="destructive"
                  size="sm"
                  onClick={() => {
                    handleRemoveTask(selectedTask.gid)
                    setSelectedTask(null)
                  }}
                >
                  <Trash2 className="size-4" />
                  移除
                </Button>
              </div>
            </div>
          )}
        </DialogContent>
      </Dialog>

      <AlertDialog open={!!confirmRemoveGids} onOpenChange={(open) => !open && setConfirmRemoveGids(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirmRemoveGids && confirmRemoveGids.length > 1
                ? `移除 ${confirmRemoveGids.length} 个下载任务`
                : '移除下载任务'}
            </AlertDialogTitle>
            <AlertDialogDescription>
              已完成任务会从列表清除，未完成任务会停止下载。确认后将立即执行。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction onClick={confirmRemoveTask} className="bg-destructive text-destructive-foreground">
              确认移除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <ParseTestModal
        open={!!parseTestFile}
        onOpenChange={(open) => !open && setParseTestFile(null)}
        initialFilename={parseTestFile || ''}
      />
    </div>
  )
}
