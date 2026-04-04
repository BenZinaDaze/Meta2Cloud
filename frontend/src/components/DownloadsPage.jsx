import { useEffect, useMemo, useState } from 'react'
import {
  addAria2Torrent,
  addAria2Uri,
  getAria2Overview,
  pauseAria2Tasks,
  purgeAria2Tasks,
  removeAria2Tasks,
  retryAria2Tasks,
  unpauseAria2Tasks,
} from '../api'

function formatBytes(bytes) {
  const value = Number(bytes) || 0
  if (value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  const size = value / 1024 ** index
  return `${size >= 100 ? size.toFixed(0) : size.toFixed(1)} ${units[index]}`
}

function formatSpeed(bytes) {
  return `${formatBytes(bytes)}/s`
}

function statusLabel(status) {
  return {
    active: '下载中',
    waiting: '队列中',
    paused: '已暂停',
    complete: '已完成',
    error: '失败',
    removed: '已移除',
  }[status] || status
}

function SummaryCard({ label, value, sub }) {
  return (
    <div
      className="relative flex min-h-[132px] flex-col justify-between gap-3 rounded-[24px] px-5 py-5"
      style={{
        background: 'linear-gradient(180deg, rgba(20, 37, 59, 0.96) 0%, rgba(14, 28, 46, 0.98) 100%)',
        border: '1px solid var(--color-border)',
        boxShadow: 'var(--shadow-soft)',
      }}
    >
      <div className="space-y-2">
        <span className="text-[11px] font-semibold uppercase tracking-[0.18em]" style={{ color: 'var(--color-muted-soft)' }}>{label}</span>
        <span className="block text-3xl font-bold tabular-nums" style={{ color: 'var(--color-text)' }}>{value}</span>
        {sub && <span className="block text-xs leading-5" style={{ color: 'var(--color-muted)' }}>{sub}</span>}
      </div>
    </div>
  )
}

function ToolButton({ children, onClick, danger = false, disabled = false, loading = false }) {
  return (
    <button
      onClick={onClick}
      disabled={disabled || loading}
      className="rounded-full px-3 py-2 text-xs font-semibold transition-all disabled:cursor-not-allowed disabled:opacity-40"
      style={{
        background: danger ? 'rgba(239, 125, 117, 0.12)' : 'rgba(255,255,255,0.03)',
        border: danger ? '1px solid rgba(239, 125, 117, 0.28)' : '1px solid var(--color-border)',
        color: danger ? 'var(--color-danger)' : 'var(--color-text)',
      }}
    >
      <span className="inline-flex items-center gap-1.5">
        {loading ? (
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" style={{ animation: 'spin 1s linear infinite' }}>
            <path d="M21 12a9 9 0 1 1-6.219-8.56"/>
          </svg>
        ) : null}
        {children}
      </span>
    </button>
  )
}

function QueueTab({ active, label, count, onClick }) {
  return (
    <button
      onClick={onClick}
      className="rounded-full px-4 py-2 text-sm font-medium transition-all duration-150"
      style={{
        color: active ? 'var(--color-accent-hover)' : 'var(--color-muted)',
        background: active ? 'rgba(200, 146, 77, 0.14)' : 'rgba(255,255,255,0.03)',
        border: active ? '1px solid rgba(200, 146, 77, 0.22)' : '1px solid var(--color-border)',
      }}
    >
      {label} · {count}
    </button>
  )
}

function DetailRow({ label, value }) {
  return (
    <div className="rounded-2xl px-4 py-3" style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)' }}>
      <div className="text-[11px] uppercase tracking-[0.16em]" style={{ color: 'var(--color-muted-soft)' }}>{label}</div>
      <div className="mt-1 text-sm break-all" style={{ color: 'var(--color-text)' }}>{value || '-'}</div>
    </div>
  )
}

function ConfirmModal({ open, title, message, confirmLabel, onConfirm, onCancel, loading = false }) {
  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-[130] flex items-center justify-center bg-[rgba(3,10,19,0.66)] px-5"
      onClick={loading ? undefined : onCancel}
    >
      <div
        className="panel-surface w-full max-w-[520px] rounded-[28px] p-6"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="text-[11px] font-semibold uppercase tracking-[0.22em]" style={{ color: 'var(--color-danger)' }}>
          Confirm Action
        </div>
        <h3 className="mt-3 text-[28px] leading-tight" style={{ color: 'var(--color-text)' }}>{title}</h3>
        <p className="mt-3 text-sm leading-7" style={{ color: 'var(--color-muted)' }}>{message}</p>

        <div className="mt-6 flex items-center justify-end gap-3">
          <button
            onClick={onCancel}
            disabled={loading}
            className="rounded-full px-4 py-2 text-sm font-semibold disabled:opacity-40"
            style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)', color: 'var(--color-text)' }}
          >
            取消
          </button>
          <ToolButton danger onClick={onConfirm} loading={loading}>
            {loading ? '移除中…' : confirmLabel}
          </ToolButton>
        </div>
      </div>
    </div>
  )
}

function TaskDetailModal({ task, onClose, onPause, onResume, onRemove, onRetry, pendingAction }) {
  if (!task) return null

  const errored = task.status === 'error'
  const pausable = task.status === 'active' || task.status === 'waiting'
  const resumable = task.status === 'paused'
  const showPause = pendingAction ? pendingAction === 'pause' : pausable
  const showResume = pendingAction ? pendingAction === 'resume' : resumable
  const showRetry = pendingAction ? pendingAction === 'retry' : errored

  return (
    <div
      className="fixed inset-0 z-[120] flex items-center justify-center bg-[rgba(3,10,19,0.62)] px-5 pb-5 pt-24"
      onClick={onClose}
    >
      <div
        className="panel-surface flex max-h-[calc(100dvh-116px)] w-full max-w-[1180px] flex-col rounded-[32px] p-6"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-5 flex items-start justify-between gap-4">
          <div className="min-w-0">
            <div className="mb-2 flex items-center gap-2">
              <span className="rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.14em]"
                style={{
                  background: errored ? 'rgba(239, 125, 117, 0.12)' : 'rgba(200, 146, 77, 0.12)',
                  color: errored ? 'var(--color-danger)' : 'var(--color-accent-hover)',
                }}>
                {statusLabel(task.status)}
              </span>
              <span className="text-xs font-mono" style={{ color: 'var(--color-muted-soft)' }}>{task.gid}</span>
            </div>
            <h2 className="text-[28px] leading-tight" style={{ color: 'var(--color-text)' }}>{task.name}</h2>
            <p className="mt-2 text-sm" style={{ color: 'var(--color-muted)' }}>{task.dir || '未指定目录'}</p>
          </div>

          <button
            onClick={onClose}
            className="flex h-10 w-10 items-center justify-center rounded-full text-lg"
            style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)', color: 'var(--color-muted)' }}
          >
            ×
          </button>
        </div>

        <div className="mb-5 flex flex-wrap gap-2">
          {showPause && <ToolButton onClick={() => onPause(task.gid)} loading={pendingAction === 'pause'}>{pendingAction === 'pause' ? '暂停中…' : '暂停'}</ToolButton>}
          {showResume && <ToolButton onClick={() => onResume(task.gid)} loading={pendingAction === 'resume'}>{pendingAction === 'resume' ? '继续中…' : '继续'}</ToolButton>}
          {showRetry && <ToolButton onClick={() => onRetry(task.gid)} loading={pendingAction === 'retry'}>{pendingAction === 'retry' ? '重试中…' : '重试'}</ToolButton>}
          <ToolButton danger onClick={() => onRemove(task.gid)} loading={pendingAction === 'remove'}>{pendingAction === 'remove' ? '移除中…' : '移除'}</ToolButton>
        </div>

        <div className="mb-5">
          <div className="mb-2 flex items-center justify-between text-xs" style={{ color: 'var(--color-muted)' }}>
            <span>{formatBytes(task.completedLength)} / {formatBytes(task.totalLength)}</span>
            <span>{task.progress.toFixed(1)}%</span>
          </div>
          <div className="h-2 overflow-hidden rounded-full" style={{ background: 'rgba(255,255,255,0.06)' }}>
            <div
              className="h-full rounded-full"
              style={{
                width: `${Math.max(task.progress, task.status === 'complete' ? 100 : 0)}%`,
                background: errored
                  ? 'linear-gradient(90deg, rgba(239,125,117,0.9) 0%, rgba(239,125,117,0.65) 100%)'
                  : 'linear-gradient(90deg, var(--color-accent) 0%, #dfb36f 100%)',
              }}
            />
          </div>
        </div>

        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          <DetailRow label="下载速度" value={formatSpeed(task.downloadSpeed)} />
          <DetailRow label="上传速度" value={formatSpeed(task.uploadSpeed)} />
          <DetailRow label="连接数" value={String(task.connections)} />
          <DetailRow label="文件数" value={String(task.fileCount)} />
          <DetailRow label="做种数" value={String(task.numSeeders)} />
          <DetailRow label="已上传" value={formatBytes(task.uploadLength)} />
          {task.bittorrent?.mode ? <DetailRow label="种子模式" value={task.bittorrent.mode} /> : null}
          {task.bittorrent?.comment ? <DetailRow label="种子备注" value={task.bittorrent.comment} /> : null}
        </div>

        {task.errorMessage && (
          <div className="mt-5 rounded-2xl px-4 py-3 text-sm"
            style={{ background: 'rgba(239,125,117,0.08)', border: '1px solid rgba(239,125,117,0.2)', color: 'var(--color-danger)' }}>
            {task.errorMessage}
          </div>
        )}

        <div className="mt-5 min-h-0 flex-1 overflow-y-auto pr-1">
          {task.files?.length ? (
            <section className="mb-5">
              <div className="mb-3 text-[11px] font-semibold uppercase tracking-[0.2em]" style={{ color: 'var(--color-accent-hover)' }}>
                文件列表
              </div>
              <div className="space-y-2">
                {task.files.map((file, index) => (
                  <div key={`${file.path}-${index}`} className="rounded-2xl px-4 py-3"
                    style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)' }}>
                    <div className="break-all text-sm" style={{ color: 'var(--color-text)' }}>{file.path || `文件 ${index + 1}`}</div>
                    <div className="mt-2 flex flex-wrap gap-4 text-xs" style={{ color: 'var(--color-muted)' }}>
                      <span>{formatBytes(file.completedLength)} / {formatBytes(file.length)}</span>
                      <span>{file.selected ? '已选择' : '未选择'}</span>
                    </div>
                  </div>
                ))}
              </div>
            </section>
          ) : null}

          {task.uris?.length ? (
            <section>
              <div className="mb-3 text-[11px] font-semibold uppercase tracking-[0.2em]" style={{ color: 'var(--color-accent-hover)' }}>
                来源链接
              </div>
              <div className="space-y-2">
                {task.uris.map((uri) => (
                  <div key={uri} className="rounded-2xl px-4 py-3 text-sm break-all"
                    style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)', color: 'var(--color-text)' }}>
                    {uri}
                  </div>
                ))}
              </div>
            </section>
          ) : null}
        </div>
      </div>
    </div>
  )
}

function TaskCard({ task, onPause, onResume, onRemove, onRetry, onOpen, pendingAction }) {
  const errored = task.status === 'error'
  const pausable = task.status === 'active' || task.status === 'waiting'
  const resumable = task.status === 'paused'
  const showPause = pendingAction ? pendingAction === 'pause' : pausable
  const showResume = pendingAction ? pendingAction === 'resume' : resumable
  const showRetry = pendingAction ? pendingAction === 'retry' : errored

  return (
    <div
      className="rounded-[24px] px-5 py-5 transition-all duration-150"
      onClick={() => onOpen(task)}
      style={{
        background: 'linear-gradient(180deg, rgba(20, 37, 59, 0.82) 0%, rgba(13, 24, 39, 0.96) 100%)',
        border: '1px solid var(--color-border)',
        cursor: 'pointer',
      }}
    >
      <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div className="min-w-0 flex-1">
          <div className="mb-2 flex flex-wrap items-center gap-2">
            <span className="rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.14em]"
              style={{
                background: errored ? 'rgba(239, 125, 117, 0.12)' : 'rgba(200, 146, 77, 0.12)',
                color: errored ? 'var(--color-danger)' : 'var(--color-accent-hover)',
              }}>
              {statusLabel(task.status)}
            </span>
            <span className="text-xs font-mono" style={{ color: 'var(--color-muted-soft)' }}>{task.gid}</span>
          </div>

          <div className="truncate text-lg font-semibold" style={{ color: 'var(--color-text)' }}>{task.name}</div>
          <div className="mt-2 text-sm leading-6" style={{ color: 'var(--color-muted)' }}>
            {task.dir || '未指定目录'}
          </div>

          <div className="mt-4">
            <div className="mb-2 flex items-center justify-between text-xs" style={{ color: 'var(--color-muted)' }}>
              <span>{formatBytes(task.completedLength)} / {formatBytes(task.totalLength)}</span>
              <span>{task.progress.toFixed(1)}%</span>
            </div>
            <div className="h-2 overflow-hidden rounded-full" style={{ background: 'rgba(255,255,255,0.06)' }}>
              <div
                className="h-full rounded-full"
                style={{
                  width: `${Math.max(task.progress, task.status === 'complete' ? 100 : 0)}%`,
                  background: errored
                    ? 'linear-gradient(90deg, rgba(239,125,117,0.9) 0%, rgba(239,125,117,0.65) 100%)'
                    : 'linear-gradient(90deg, var(--color-accent) 0%, #dfb36f 100%)',
                }}
              />
            </div>
          </div>

          <div className="mt-4 grid grid-cols-2 gap-3 text-sm md:grid-cols-4">
            <div><div style={{ color: 'var(--color-muted-soft)' }}>下载速度</div><div style={{ color: 'var(--color-text)' }}>{formatSpeed(task.downloadSpeed)}</div></div>
            <div><div style={{ color: 'var(--color-muted-soft)' }}>上传速度</div><div style={{ color: 'var(--color-text)' }}>{formatSpeed(task.uploadSpeed)}</div></div>
            <div><div style={{ color: 'var(--color-muted-soft)' }}>连接数</div><div style={{ color: 'var(--color-text)' }}>{task.connections}</div></div>
            <div><div style={{ color: 'var(--color-muted-soft)' }}>文件数</div><div style={{ color: 'var(--color-text)' }}>{task.fileCount}</div></div>
          </div>

          {task.errorMessage && (
            <div className="mt-4 rounded-2xl px-4 py-3 text-sm"
              style={{ background: 'rgba(239,125,117,0.08)', border: '1px solid rgba(239,125,117,0.2)', color: 'var(--color-danger)' }}>
              {task.errorMessage}
            </div>
          )}
        </div>

        <div className="flex flex-wrap gap-2 xl:max-w-[220px] xl:justify-end">
          {showPause && <ToolButton onClick={(e) => { e.stopPropagation(); onPause(task.gid) }} loading={pendingAction === 'pause'}>{pendingAction === 'pause' ? '暂停中…' : '暂停'}</ToolButton>}
          {showResume && <ToolButton onClick={(e) => { e.stopPropagation(); onResume(task.gid) }} loading={pendingAction === 'resume'}>{pendingAction === 'resume' ? '继续中…' : '继续'}</ToolButton>}
          {showRetry && <ToolButton onClick={(e) => { e.stopPropagation(); onRetry(task.gid) }} loading={pendingAction === 'retry'}>{pendingAction === 'retry' ? '重试中…' : '重试'}</ToolButton>}
          <ToolButton danger onClick={(e) => { e.stopPropagation(); onRemove(task.gid) }} loading={pendingAction === 'remove'}>{pendingAction === 'remove' ? '移除中…' : '移除'}</ToolButton>
        </div>
      </div>
    </div>
  )
}

function ActionPanel({ uriInput, setUriInput, onSubmitUri, onTorrentChange, torrentName }) {
  return (
    <div className="rounded-[28px] px-5 py-5" style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)' }}>
      <div className="mb-3 text-[11px] font-semibold uppercase tracking-[0.2em]" style={{ color: 'var(--color-accent-hover)' }}>
        新建下载
      </div>
      <textarea
        value={uriInput}
        onChange={(e) => setUriInput(e.target.value)}
        placeholder="每行一个链接，支持 HTTP、HTTPS、FTP、磁力链"
        className="h-24 w-full rounded-[18px] px-4 py-3 text-sm outline-none"
        style={{
          background: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
          color: 'var(--color-text)',
          resize: 'none',
          overflowY: 'auto',
        }}
      />
      <div className="mt-3 flex flex-wrap items-center gap-2.5">
        <button
          onClick={onSubmitUri}
          className="rounded-full px-4 py-2 text-sm font-semibold"
          style={{ background: 'linear-gradient(135deg, var(--color-accent) 0%, #b37533 100%)', color: '#fff', border: 'none' }}
        >
          添加链接下载
        </button>

        <label
          className="cursor-pointer rounded-full px-4 py-2 text-sm font-semibold"
          style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)', color: 'var(--color-text)' }}
        >
          上传 Torrent
          <input type="file" accept=".torrent,application/x-bittorrent" hidden onChange={onTorrentChange} />
        </label>

        {torrentName && <span className="text-xs" style={{ color: 'var(--color-muted)' }}>{torrentName}</span>}
      </div>
    </div>
  )
}

export default function DownloadsPage({ queue = 'all', onChangeQueue, onToast, initialOverview = null }) {
  const [overview, setOverview] = useState(initialOverview)
  const [uriInput, setUriInput] = useState('')
  const [torrentName, setTorrentName] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState(null)
  const [selectedTask, setSelectedTask] = useState(null)
  const [pendingActions, setPendingActions] = useState({})
  const [confirmRemoveGid, setConfirmRemoveGid] = useState(null)
  const showDashboard = queue === 'all'

  useEffect(() => {
    if (initialOverview) {
      setOverview(initialOverview)
      setError(null)
      setLoading(false)
    }
  }, [initialOverview])

  function applyOptimisticTaskUpdate(gid, patch, targetBucket = null) {
    setOverview((prev) => {
      if (!prev?.tasks) return prev
      const buckets = ['active', 'waiting', 'stopped']
      let movedTask = null
      const nextTasks = {}

      for (const bucket of buckets) {
        const currentList = prev.tasks[bucket] || []
        nextTasks[bucket] = currentList.filter((item) => {
          if (item.gid !== gid) return true
          movedTask = { ...item, ...patch }
          return false
        })
      }

      if (!movedTask) {
        return prev
      }

      const destination = targetBucket || buckets.find((bucket) =>
        (prev.tasks[bucket] || []).some((item) => item.gid === gid)
      ) || 'waiting'

      nextTasks[destination] = [movedTask, ...(nextTasks[destination] || [])]

      return {
        ...prev,
        summary: {
          ...(prev.summary || {}),
          activeCount: nextTasks.active.length,
          waitingCount: nextTasks.waiting.length,
          stoppedCount: nextTasks.stopped.length,
        },
        tasks: nextTasks,
      }
    })
  }

  async function loadAll(silent = false) {
    try {
      if (!silent) setLoading(true)
      const overviewRes = await getAria2Overview()
      setOverview(overviewRes.data)
      setError(null)
    } catch (e) {
      setError(e?.response?.data?.detail || e.message)
    } finally {
      if (!silent) setLoading(false)
    }
  }

  useEffect(() => {
    loadAll()
    const id = setInterval(() => loadAll(true), 5000)
    return () => clearInterval(id)
  }, [])

  const tasks = useMemo(() => {
    if (!overview?.tasks) return []
    if (queue === 'active') return overview.tasks.active
    if (queue === 'waiting') return overview.tasks.waiting
    if (queue === 'stopped') return overview.tasks.stopped
    return [...overview.tasks.active, ...overview.tasks.waiting, ...overview.tasks.stopped]
  }, [overview, queue])

  useEffect(() => {
    if (!selectedTask || !overview?.tasks) return
    const latest = [...overview.tasks.active, ...overview.tasks.waiting, ...overview.tasks.stopped]
      .find((item) => item.gid === selectedTask.gid)
    if (latest) {
      setSelectedTask(latest)
    } else {
      setSelectedTask(null)
    }
  }, [overview, selectedTask])

  async function withAction(action, successMessage, gid = null, actionName = null, optimisticPatch = null, optimisticBucket = null) {
    try {
      setBusy(true)
      if (gid && actionName) {
        setPendingActions((prev) => ({ ...prev, [gid]: actionName }))
      }
      if (gid && optimisticPatch) {
        applyOptimisticTaskUpdate(gid, optimisticPatch, optimisticBucket)
      }
      await action()
      await loadAll(true)
      onToast?.('success', '下载管理', successMessage)
    } catch (e) {
      onToast?.('error', '下载管理', e?.response?.data?.detail || e.message)
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

  function handleRemoveTask(gid) {
    setConfirmRemoveGid(gid)
  }

  async function confirmRemoveTask() {
    if (!confirmRemoveGid) return
    await withAction(() => removeAria2Tasks([confirmRemoveGid]), '任务已移除', confirmRemoveGid, 'remove')
    setConfirmRemoveGid(null)
  }

  async function handleSubmitUri() {
    const uris = uriInput.split('\n').map(v => v.trim()).filter(Boolean)
    if (!uris.length) {
      onToast?.('warning', '下载管理', '请先输入至少一个下载链接')
      return
    }
    await withAction(async () => {
      await addAria2Uri({ uris })
      setUriInput('')
    }, '已添加下载任务')
  }

  async function handleTorrentChange(event) {
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

  return (
    <div className="flex-1">
      <section className="panel-surface rounded-[32px] px-7 py-7" style={{ minHeight: 'calc(100dvh - 128px)' }}>
        <div className="mb-7 flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
          <div className="max-w-2xl">
            <div className="mb-2 text-[11px] font-semibold uppercase tracking-[0.24em]" style={{ color: 'var(--color-accent-hover)' }}>
              Native Download Center
            </div>
            <h1 className="text-[34px] font-bold leading-tight" style={{ color: 'var(--color-text)' }}>
              {showDashboard ? '下载管理' : statusLabel(queue)}
            </h1>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <ToolButton onClick={() => loadAll()} disabled={busy}>刷新</ToolButton>
            {showDashboard ? (
              <ToolButton danger onClick={() => withAction(() => purgeAria2Tasks(), '已清理已完成任务')} disabled={busy}>
                清理已完成
              </ToolButton>
            ) : null}
          </div>
        </div>

        {showDashboard ? (
          <div className="mb-8 grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <SummaryCard label="下载速度" value={summary ? formatSpeed(summary.downloadSpeed) : '--'} sub="当前全局吞吐" />
            <SummaryCard label="上传速度" value={summary ? formatSpeed(summary.uploadSpeed) : '--'} sub="适用于 BT 做种场景" />
            <SummaryCard label="活跃任务" value={summary?.activeCount ?? '--'} sub={`等待 ${summary?.waitingCount ?? '--'} · 已停止 ${summary?.stoppedCount ?? '--'}`} />
            <SummaryCard label="Aria2 版本" value={overview?.version?.version || '--'} sub={(overview?.version?.enabledFeatures || []).slice(0, 3).join(' · ') || '等待连接'} />
          </div>
        ) : null}

        <div className="mb-6 flex flex-wrap gap-2">
          <QueueTab active={queue === 'all'} label="全部" count={(overview?.tasks?.active?.length || 0) + (overview?.tasks?.waiting?.length || 0) + (overview?.tasks?.stopped?.length || 0)} onClick={() => onChangeQueue?.('all')} />
          <QueueTab active={queue === 'active'} label="下载中" count={overview?.tasks?.active?.length || 0} onClick={() => onChangeQueue?.('active')} />
          <QueueTab active={queue === 'waiting'} label="等待中" count={overview?.tasks?.waiting?.length || 0} onClick={() => onChangeQueue?.('waiting')} />
          <QueueTab active={queue === 'stopped'} label="已停止" count={overview?.tasks?.stopped?.length || 0} onClick={() => onChangeQueue?.('stopped')} />
        </div>

        {showDashboard ? (
          <ActionPanel
            uriInput={uriInput}
            setUriInput={setUriInput}
            onSubmitUri={handleSubmitUri}
            onTorrentChange={handleTorrentChange}
            torrentName={torrentName}
          />
        ) : null}

        {loading && !overview && (
          <div className="py-20 text-center text-sm" style={{ color: 'var(--color-muted)' }}>
            正在连接 aria2…
          </div>
        )}

        {error && (
          <div className="mt-6 rounded-[24px] px-5 py-5 text-sm"
            style={{ background: 'rgba(239,125,117,0.08)', border: '1px solid rgba(239,125,117,0.2)', color: 'var(--color-danger)' }}>
            {error}
          </div>
        )}

        {!loading && !error && (
          <div className={`${showDashboard ? 'mt-6' : 'mt-2'} space-y-4`}>
            {tasks.length === 0 ? (
              <div className="flex flex-col items-center justify-center gap-4 py-24">
                <span style={{ fontSize: 96, lineHeight: 1 }}>⤓</span>
                <p className="text-xl font-medium" style={{ color: 'var(--color-muted)' }}>当前分组没有任务</p>
              </div>
            ) : (
              tasks.map(task => (
                <TaskCard
                  key={task.gid}
                  task={task}
                  onOpen={setSelectedTask}
                  pendingAction={pendingActions[task.gid]}
                  onPause={(gid) => withAction(() => pauseAria2Tasks([gid]), '任务已暂停', gid, 'pause', { status: 'paused' }, 'waiting')}
                  onResume={(gid) => withAction(() => unpauseAria2Tasks([gid]), '任务已继续', gid, 'resume', { status: 'active' }, 'active')}
                  onRemove={handleRemoveTask}
                  onRetry={(gid) => withAction(() => retryAria2Tasks([gid]), '任务已重新加入队列', gid, 'retry')}
                />
              ))
            )}
          </div>
        )}
      </section>

      <TaskDetailModal
        task={selectedTask}
        onClose={() => setSelectedTask(null)}
        pendingAction={selectedTask ? pendingActions[selectedTask.gid] : null}
        onPause={(gid) => withAction(() => pauseAria2Tasks([gid]), '任务已暂停', gid, 'pause', { status: 'paused' }, 'waiting')}
        onResume={(gid) => withAction(() => unpauseAria2Tasks([gid]), '任务已继续', gid, 'resume', { status: 'active' }, 'active')}
        onRemove={handleRemoveTask}
        onRetry={(gid) => withAction(() => retryAria2Tasks([gid]), '任务已重新加入队列', gid, 'retry')}
      />

      <ConfirmModal
        open={!!confirmRemoveGid}
        title="移除下载任务"
        message="已完成任务会从列表清除，未完成任务会停止下载。确认后将立即执行。"
        confirmLabel="确认移除"
        loading={confirmRemoveGid ? pendingActions[confirmRemoveGid] === 'remove' : false}
        onCancel={() => setConfirmRemoveGid(null)}
        onConfirm={confirmRemoveTask}
      />
    </div>
  )
}
