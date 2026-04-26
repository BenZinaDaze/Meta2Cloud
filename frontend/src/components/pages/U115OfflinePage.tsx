import { useCallback, useEffect, useMemo, useState } from 'react'
import { RefreshCw, Trash2, Search, Link } from 'lucide-react'
import {
  addU115OfflineUrls,
  clearU115OfflineTasks,
  deleteU115OfflineTasks,
  getU115OfflineOverview,
  getU115OfflineQuota,
  testU115Connection,
} from '@/api'
import { StatePanel } from '@/components/StatePanel'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
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
import ParseTestModal from '@/components/modals/ParseTestModal'
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

function formatBytes(bytes: string | number): string {
  const value = Number(bytes)
  if (!Number.isFinite(value) || value <= 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = value
  let index = 0
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index += 1
  }
  return `${size.toFixed(index === 0 ? 0 : 2)} ${units[index]}`
}

function formatTime(ts: string | number | undefined): string {
  if (!ts) return '-'
  const asNumber = Number(ts)
  let ms: number
  if (!Number.isFinite(asNumber)) {
    const parsed = new Date(ts)
    if (Number.isNaN(parsed.getTime())) return '-'
    ms = parsed.getTime()
  } else {
    ms = asNumber < 1e12 ? asNumber * 1000 : asNumber
  }
  return new Date(ms).toLocaleString('zh-CN', { hour12: false })
}

function statusMeta(status: number): { label: string; className: string } {
  if (status === 2) return { label: '已完成', className: 'bg-success/20 text-success' }
  if (status === 1) return { label: '下载中', className: 'bg-info/20 text-info' }
  if (status === -1) return { label: '失败', className: 'bg-destructive/20 text-destructive' }
  return { label: '等待中', className: 'bg-muted text-muted-foreground' }
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
      <div className="mb-2 flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
        <div className="max-w-2xl">
          <div className="mb-2 text-[11px] font-semibold uppercase tracking-widest text-brand">
            Offline Download Center
          </div>
          <h1 className="text-[34px] font-bold leading-tight">115 云下载</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            提交离线下载链接，并管理当前 115 云下载任务
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              loadOverview()
              loadQuota()
              loadDriveSpace()
            }}
            disabled={busy}
          >
            <RefreshCw className="size-4" />
            刷新
          </Button>
          <Button variant="outline" size="sm" onClick={() => handleClear('0')} disabled={busy}>
            清空已完成
          </Button>
        </div>
      </div>

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

      <div className="rounded-2xl border bg-muted/30 p-5">
        <div className="mb-3 text-[11px] font-semibold uppercase tracking-widest text-brand">
          新建云下载
        </div>
        <Textarea
          value={urls}
          onChange={(e) => setUrls(e.target.value)}
          placeholder="每行一个链接，支持 HTTP、HTTPS、磁力链"
          className="h-28 resize-none"
        />
        <div className="mt-3">
          <Input
            value={wpPathId}
            onChange={(e) => setWpPathId(e.target.value)}
            placeholder="保存目录 ID（可选，留空则使用配置页中的云下载目录 ID）"
            className="rounded-full"
          />
          <div className="mt-3 flex flex-wrap items-center gap-2.5">
            <Button onClick={handleAddUrls} disabled={busy}>
              <Link className="size-4" />
              {busy ? '提交中…' : '提交链接'}
            </Button>
          </div>
        </div>
        <div className="mt-2 text-xs text-muted-foreground">提交后任务会出现在下方列表中</div>
      </div>

      <section className="rounded-3xl border bg-card p-7">
        <div className="mb-6 flex flex-wrap items-center gap-3">
          <div>
            <div className="mb-1 text-[11px] font-semibold uppercase tracking-widest text-brand">
              Offline Queue
            </div>
            <div className="text-base font-semibold">任务列表</div>
          </div>
          <div className="ml-auto flex flex-wrap items-center gap-2">
            <Button variant="outline" size="sm" onClick={() => handleClear('2')} disabled={busy}>
              清空失败任务
            </Button>
          </div>
        </div>

        {loading ? (
          <StatePanel title="正在加载云下载任务" description="任务列表和空间状态正在同步中。" compact />
        ) : tasks.length === 0 ? (
          <StatePanel icon="☁" title="当前没有云下载任务" description="在上方提交链接后，任务会出现在这里。" />
        ) : (
          <div className="grid gap-3">
            {tasks.map((task) => {
              const status = statusMeta(task.status)
              return (
                <div
                  key={task.info_hash}
                  className="rounded-2xl border bg-card p-5 transition-all hover:bg-muted/50"
                >
                  <div className="flex flex-wrap items-start gap-3">
                    <div className="min-w-0 flex-1">
                      <div className="mb-2 flex flex-wrap items-center gap-2">
                        <span className={`rounded-full px-2.5 py-0.5 text-[11px] font-semibold uppercase tracking-wider ${status.className}`}>
                          {status.label}
                        </span>
                        <span className="text-xs text-muted-foreground">
                          {Math.round(Number(task.percent_done) || 0)}%
                        </span>
                        <div className="ml-auto">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="size-8 text-muted-foreground hover:text-destructive hover:bg-destructive/10"
                            onClick={() => setDeleteConfirmTask(task)}
                            disabled={busy}
                            aria-label="删除任务"
                          >
                            <Trash2 className="size-4" />
                          </Button>
                        </div>
                      </div>
                      <div className="mb-3">
                        <div className="mb-1.5 flex justify-between text-xs text-muted-foreground">
                          <span>{formatBytes(task.size)}</span>
                          <span>{Math.round(Number(task.percent_done) || 0)}%</span>
                        </div>
                        <div className="h-1.5 overflow-hidden rounded-full bg-muted">
                          <div
                            className={`h-full rounded-full transition-all ${
                              task.status === -1 ? 'bg-destructive' : 'bg-brand'
                            }`}
                            style={{
                              width: `${Math.max(Math.min(Number(task.percent_done) || 0, 100), task.status === 2 ? 100 : 0)}%`,
                            }}
                          />
                        </div>
                      </div>
                      <div className="flex items-start gap-2">
                        <div className="min-w-0 flex-1 text-sm font-medium break-all">
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
                        <div>添加时间：{formatTime(task.add_time)}</div>
                        <div>更新时间：{formatTime(task.last_update)}</div>
                        <div className="break-all">Info Hash：{task.info_hash || '-'}</div>
                        {task.wp_path_id && <div className="break-all">保存目录：{task.wp_path_id}</div>}
                        {task.url && <div className="break-all">链接：{task.url}</div>}
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
          <div className="mb-4 rounded-xl border bg-muted/50 p-4 text-sm break-all">
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
    </div>
  )
}
