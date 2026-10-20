import { useCallback, useEffect, useMemo, useState } from 'react'
import { RefreshCw, Trash2, Search, Link, Cloud } from 'lucide-react'
import {
  addU115OfflineUrls,
  clearU115OfflineTasks,
  deleteU115OfflineTasks,
  getU115OfflineOverview,
  getU115OfflineQuota,
  testU115Connection,
} from '@/api'
import BackToTopButton from '@/components/BackToTopButton'
import { StatePanel } from '@/components/StatePanel'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Input } from '@/components/ui/input'
import PageHeader from '@/components/shared/PageHeader'
import { formatBytes } from '@/lib/format'
import { formatDateTime } from '@/lib/time'
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
import { cn } from '@/lib/utils'
import { toast } from 'sonner'

interface U115Task {
  info_hash: string
  name: string
  url: string
  status: number
  size: string
  percent_done: number
  add_time: string
  last_update: string
  wp_path_id?: string
}

interface U115Overview {
  tasks: U115Task[]
  pagination?: {
    page: number
    total_pages: number
    total: number
    has_prev: boolean
    has_next: boolean
  }
}

interface U115Quota {
  count: number
  surplus: number
}

function statusMeta(status: number): { label: string; className: string } {
  if (status === 2) return { label: '已完成', className: 'bg-success/12 text-success' }
  if (status === 1) return { label: '下载中', className: 'bg-info/12 text-info' }
  if (status === -1) return { label: '失败', className: 'bg-destructive/12 text-destructive' }
  return { label: '等待中', className: 'bg-muted text-muted-foreground' }
}

function SummaryCard({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="card-lift relative flex min-h-[120px] flex-col justify-between gap-3 overflow-hidden rounded-2xl border border-border/70 bg-card/50 p-5">
      <div className="pointer-events-none absolute inset-0 bg-ambient opacity-50" />
      <div className="relative space-y-2">
        <span className="text-[11px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">{label}</span>
        <span className="block text-3xl font-bold tabular-nums tracking-tight">{value}</span>
        {sub && <span className="block text-xs leading-5 text-muted-foreground">{sub}</span>}
      </div>
    </div>
  )
}

interface U115OfflinePageProps {
  onToast?: (type: 'success' | 'error' | 'warning', title: string, message?: string) => void
}

export default function U115OfflinePage({ onToast }: U115OfflinePageProps) {
  const [overview, setOverview] = useState<U115Overview | null>(null)
  const [quota, setQuota] = useState<U115Quota | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [pageVisible, setPageVisible] = useState(true)
  const [urls, setUrls] = useState('')
  const [wpPathId, setWpPathId] = useState('')
  const [driveSpace, setDriveSpace] = useState<{ remain_space: number; total_space: number } | null>(null)
  const [parseTestFile, setParseTestFile] = useState<string | null>(null)
  const [deleteConfirmTask, setDeleteConfirmTask] = useState<U115Task | null>(null)
  const [page, setPage] = useState(1)

  const loadOverview = useCallback(async () => {
    try {
      const res = await getU115OfflineOverview({ page })
      setOverview(res.data)
    } catch (e) {
      const detail = (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail || '读取 115 云下载概览失败'
      if (onToast) {
        onToast('error', '云下载加载失败', detail)
      } else {
        toast.error('云下载加载失败', { description: detail })
      }
    } finally {
      setLoading(false)
    }
  }, [onToast, page])

  const loadQuota = useCallback(async () => {
    try {
      const res = await getU115OfflineQuota()
      setQuota(res?.data?.quota || null)
    } catch {
      setQuota(null)
    }
  }, [])

  const loadDriveSpace = useCallback(async () => {
    try {
      const res = await testU115Connection()
      setDriveSpace(res?.data || null)
    } catch {
      setDriveSpace(null)
    }
  }, [])

  useEffect(() => {
    loadOverview()
    loadQuota()
    loadDriveSpace()
  }, [loadOverview, loadQuota, loadDriveSpace])

  useEffect(() => {
    const handleVisibilityChange = () => {
      setPageVisible(document.visibilityState === 'visible')
    }
    document.addEventListener('visibilitychange', handleVisibilityChange)
    return () => document.removeEventListener('visibilitychange', handleVisibilityChange)
  }, [])

  useEffect(() => {
    if (!pageVisible || busy) return
    loadOverview()
  }, [busy, loadOverview, pageVisible])

  useEffect(() => {
    if (!pageVisible) return undefined
    const timer = setInterval(() => {
      if (!busy) loadOverview()
    }, 15000)
    return () => clearInterval(timer)
  }, [busy, loadOverview, pageVisible])

  const tasks = useMemo(() => overview?.tasks || [], [overview])
  const pagination = overview?.pagination

  async function handleAddUrls() {
    if (!urls.trim()) {
      toast.warning('缺少链接', { description: '请至少输入一个下载链接' })
      return
    }
    setBusy(true)
    try {
      const res = await addU115OfflineUrls({
        urls,
        wp_path_id: wpPathId.trim() || undefined,
      })
      const results = res?.data?.results || []
      const successCount = results.filter((item: { state: boolean }) => item.state).length
      const failedCount = results.length - successCount
      toast[failedCount > 0 ? 'warning' : 'success']('云下载任务已提交', {
        description: failedCount > 0 ? `成功 ${successCount} 条，失败 ${failedCount} 条` : `成功提交 ${successCount} 条链接`,
      })
      setUrls('')
      await loadOverview()
    } catch (e) {
      toast.error('添加任务失败', { description: (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail || '115 云下载添加链接失败' })
    } finally {
      setBusy(false)
    }
  }

  async function handleClear(flag: string) {
    setBusy(true)
    try {
      await clearU115OfflineTasks({ flag })
      toast.success('任务已清空', { description: flag === '2' ? '已清空失败任务' : flag === '0' ? '已清空完成任务' : '已清空全部任务' })
      await loadOverview()
    } catch (e) {
      toast.error('清空任务失败', { description: (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail || '115 云下载清空任务失败' })
    } finally {
      setBusy(false)
    }
  }

  async function handleDeleteOne(infoHash: string, delSourceFile = 0) {
    if (!infoHash) return
    setBusy(true)
    try {
      await deleteU115OfflineTasks({
        info_hashes: [infoHash],
        del_source_file: delSourceFile,
      })
      toast.success('任务已删除', { description: delSourceFile ? '已删除任务并删除源文件' : '已删除任务' })
      setDeleteConfirmTask(null)
      await loadOverview()
    } catch (e) {
      toast.error('删除任务失败', { description: (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail || '115 云下载删除任务失败' })
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex flex-1 flex-col gap-5">
      <PageHeader
        kicker="Offline Download Center"
        title="115 云下载"
        description="提交离线下载链接，并管理当前 115 云下载任务"
        actions={
          <>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                loadOverview()
                loadQuota()
                loadDriveSpace()
              }}
              disabled={busy}
              className="rounded-full"
            >
              <RefreshCw className="size-4" />
              刷新
            </Button>
            <Button variant="outline" size="sm" onClick={() => handleClear('0')} disabled={busy} className="rounded-full">
              清空已完成任务
            </Button>
          </>
        }
      />

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        <SummaryCard
          label="剩余配额"
          value={quota ? `${quota.surplus} / ${quota.count}` : '-'}
          sub="剩余次数 / 总配额"
        />
        <SummaryCard label="任务总数" value={`${pagination?.total ?? tasks.length}`} sub="当前云下载任务列表" />
        <SummaryCard
          label="115 网盘容量"
          value={driveSpace ? `${formatBytes(driveSpace.remain_space)} / ${formatBytes(driveSpace.total_space)}` : '无授权'}
          sub={driveSpace ? '剩余空间 / 总空间' : '请先到配置页完成 115 授权'}
        />
      </div>

      <div className="relative overflow-hidden rounded-2xl border border-border/70 bg-muted/25 p-5">
        <div className="pointer-events-none absolute inset-0 bg-ambient opacity-60" />
        <div className="relative">
          <div className="mb-3 flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.14em] text-primary/90">
            <span className="h-px w-5 bg-gradient-to-r from-tide to-transparent" />
            新建云下载
          </div>
          <Textarea
            value={urls}
            onChange={(e) => setUrls(e.target.value)}
            placeholder="每行一个链接，支持 HTTP、HTTPS、磁力链"
            className="h-28 resize-none font-mono text-[13px]"
          />
          <div className="mt-3">
            <Input
              value={wpPathId}
              onChange={(e) => setWpPathId(e.target.value)}
              placeholder="保存目录 ID（可选，留空则使用配置页中的云下载目录 ID）"
              className="rounded-full font-mono text-xs"
            />
            <div className="mt-3 flex flex-wrap items-center gap-2.5">
              <Button onClick={handleAddUrls} disabled={busy} className="rounded-full px-4">
                <Link className="size-4" />
                {busy ? '提交中…' : '提交链接'}
              </Button>
            </div>
          </div>
          <div className="mt-2 text-xs text-muted-foreground">提交后任务会出现在下方列表中</div>
        </div>
      </div>

      <section className="rounded-2xl border border-border/70 bg-card/40 p-5 sm:p-7">
        <div className="mb-6 flex flex-wrap items-center gap-3">
          <div>
            <div className="mb-1 flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.14em] text-primary/90">
              <span className="h-px w-5 bg-gradient-to-r from-tide to-transparent" />
              Offline Queue
            </div>
            <div className="text-base font-semibold tracking-tight">任务列表</div>
          </div>
          <div className="ml-auto flex flex-wrap items-center gap-2">
            <Button variant="outline" size="sm" onClick={() => handleClear('2')} disabled={busy} className="rounded-full">
              清空失败任务
            </Button>
          </div>
        </div>

        {loading ? (
          <StatePanel title="正在加载云下载任务" description="任务列表和空间状态正在同步中。" compact />
        ) : tasks.length === 0 ? (
          <StatePanel icon={<Cloud className="size-6" />} title="当前没有云下载任务" description="在上方提交链接后，任务会出现在这里。" />
        ) : (
          <div className="grid gap-3">
            {tasks.map((task) => {
              const status = statusMeta(task.status)
              const pct = Math.max(Math.min(Number(task.percent_done) || 0, 100), task.status === 2 ? 100 : 0)
              const isActive = task.status === 1
              return (
                <div
                  key={task.info_hash}
                  className="rounded-2xl border border-border/70 bg-card/50 p-5 transition-all duration-300 hover:border-primary/25 hover:bg-card/80"
                >
                  <div className="flex flex-wrap items-start gap-3">
                    <div className="min-w-0 flex-1">
                      <div className="mb-2 flex flex-wrap items-center gap-2">
                        <span className={cn('flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-[11px] font-semibold uppercase tracking-wider', status.className)}>
                          {isActive && <span className="relative flex size-1.5">
                            <span className="absolute inline-flex size-1.5 rounded-full bg-current opacity-60 animate-tide-ping" />
                            <span className="relative inline-flex size-1.5 rounded-full bg-current" />
                          </span>}
                          {status.label}
                        </span>
                        <span className="text-xs tabular-nums text-muted-foreground">
                          {Math.round(Number(task.percent_done) || 0)}%
                        </span>
                        <div className="ml-auto">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="size-8 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                            onClick={() => setDeleteConfirmTask(task)}
                            disabled={busy}
                            aria-label="删除任务"
                          >
                            <Trash2 className="size-4" />
                          </Button>
                        </div>
                      </div>
                      <div className="mb-3">
                        <div className="mb-1.5 flex justify-between text-xs tabular-nums text-muted-foreground">
                          <span>{formatBytes(task.size)}</span>
                          <span className="font-semibold text-foreground/80">{pct}%</span>
                        </div>
                        <div className="h-1.5 overflow-hidden rounded-full bg-muted/80">
                          <div
                            className={cn(
                              'h-full rounded-full transition-all duration-500',
                              task.status === -1 ? 'bg-destructive' : task.status === 2 ? 'bg-success' : 'bg-lime',
                              task.status === 1 && 'progress-flow glow-lime'
                            )}
                            style={{ width: `${pct}%` }}
                          />
                        </div>
                      </div>
                      <div className="flex items-start gap-2">
                        <div className="min-w-0 flex-1 break-all text-sm font-medium">
                          {task.name || task.url || task.info_hash}
                        </div>
                        <Button
                          variant="outline"
                          size="icon-xs"
                          onClick={() => setParseTestFile(task.name || task.url || task.info_hash || '')}
                          title="解析此任务"
                        >
                          <Search className="size-3" />
                        </Button>
                      </div>
                      <div className="mt-3 grid gap-1 text-xs text-muted-foreground">
                        <div>添加时间：{formatDateTime(task.add_time)}</div>
                        <div>更新时间：{formatDateTime(task.last_update)}</div>
                        <div className="break-all font-mono">Info Hash：{task.info_hash || '-'}</div>
                        {task.wp_path_id && <div className="break-all font-mono">保存目录：{task.wp_path_id}</div>}
                        {task.url && <div className="break-all font-mono">链接：{task.url}</div>}
                      </div>
                    </div>
                  </div>
                </div>
              )
            })}
          </div>
        )}

        {pagination && pagination.total_pages > 1 && (
          <div className="mt-5 flex flex-wrap items-center justify-between gap-3">
            <div className="text-xs text-muted-foreground">
              第 {pagination.page} / {pagination.total_pages} 页，共 {pagination.total} 条
            </div>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setPage(pagination.page - 1)}
                disabled={busy || !pagination.has_prev}
              >
                上一页
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setPage(pagination.page + 1)}
                disabled={busy || !pagination.has_next}
              >
                下一页
              </Button>
            </div>
          </div>
        )}
      </section>

      <ParseTestModal
        open={!!parseTestFile}
        onOpenChange={(open) => !open && setParseTestFile(null)}
        initialFilename={parseTestFile || ''}
      />

      <AlertDialog open={!!deleteConfirmTask} onOpenChange={(open) => !open && setDeleteConfirmTask(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除云下载任务</AlertDialogTitle>
            <AlertDialogDescription>
              请选择要执行的删除方式。删除源文件后，115 网盘中的对应文件也会一并删除。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="mb-4 break-all rounded-xl border border-border/60 bg-muted/40 p-4 text-sm">
            {deleteConfirmTask?.name || deleteConfirmTask?.url || deleteConfirmTask?.info_hash}
          </div>
          <AlertDialogFooter className="flex-col gap-2 sm:flex-row">
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => deleteConfirmTask && handleDeleteOne(deleteConfirmTask.info_hash, 0)}
              className="bg-muted text-foreground"
            >
              不删除源文件
            </AlertDialogAction>
            <AlertDialogAction
              onClick={() => deleteConfirmTask && handleDeleteOne(deleteConfirmTask.info_hash, 1)}
              className="bg-destructive text-destructive-foreground"
            >
              删除源文件
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <BackToTopButton />
    </div>
  )
}
