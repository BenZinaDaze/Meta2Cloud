import { useCallback, useEffect, useMemo, useState } from 'react'
import { createPortal } from 'react-dom'
import {
  addU115OfflineUrls,
  clearU115OfflineTasks,
  deleteU115OfflineTasks,
  getU115AutoOrganizeStatus,
  getU115OfflineOverview,
  getU115OfflineQuota,
  testU115Connection,
} from '../api'
import ParseTestModal from './ParseTestModal'
import { StatePanel } from './StatePanel'

function formatBytes(bytes) {
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

function formatTime(ts) {
  if (!ts) return '-'
  const asNumber = Number(ts)
  let ms = asNumber
  if (!Number.isFinite(asNumber)) {
    const parsed = new Date(ts)
    if (Number.isNaN(parsed.getTime())) return '-'
    ms = parsed.getTime()
  } else {
    ms = asNumber < 1e12 ? asNumber * 1000 : asNumber
  }
  return new Date(ms).toLocaleString('zh-CN', { hour12: false })
}

function statusMeta(status) {
  if (status === 2) return { label: '已完成', color: '#22c55e', bg: 'rgba(34,197,94,0.12)' }
  if (status === 1) return { label: '下载中', color: '#3b82f6', bg: 'rgba(59,130,246,0.12)' }
  if (status === -1) return { label: '失败', color: '#ef4444', bg: 'rgba(239,68,68,0.12)' }
  return { label: '等待中', color: '#94a3b8', bg: 'rgba(148,163,184,0.12)' }
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
        {sub ? <span className="block text-xs leading-5" style={{ color: 'var(--color-muted)' }}>{sub}</span> : null}
      </div>
    </div>
  )
}

function ToolButton({ children, onClick, danger = false, disabled = false }) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className="rounded-full px-3 py-2 text-xs font-semibold transition-all disabled:cursor-not-allowed disabled:opacity-40"
      style={{
        background: danger ? 'rgba(239, 125, 117, 0.12)' : 'rgba(255,255,255,0.03)',
        border: danger ? '1px solid rgba(239, 125, 117, 0.28)' : '1px solid var(--color-border)',
        color: danger ? 'var(--color-danger)' : 'var(--color-text)',
      }}
    >
      {children}
    </button>
  )
}

function PaginationBar({ pagination, onChange, busy = false }) {
  if (!pagination || pagination.total_pages <= 1) return null

  return (
    <div className="mt-5 flex flex-wrap items-center justify-between gap-3">
      <div className="text-xs" style={{ color: 'var(--color-muted)' }}>
        第 {pagination.page} / {pagination.total_pages} 页，共 {pagination.total} 条
      </div>
      <div className="flex items-center gap-2">
        <ToolButton onClick={() => onChange(pagination.page - 1)} disabled={busy || !pagination.has_prev}>上一页</ToolButton>
        <ToolButton onClick={() => onChange(pagination.page + 1)} disabled={busy || !pagination.has_next}>下一页</ToolButton>
      </div>
    </div>
  )
}

function DeleteConfirmModal({ task, busy = false, onDeleteKeepSource, onDeleteSource, onClose }) {
  const [show, setShow] = useState(false)

  useEffect(() => {
    requestAnimationFrame(() => setShow(true))
    const onKey = (event) => {
      if (event.key === 'Escape' && !busy) onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [busy, onClose])

  if (!task) return null

  return createPortal(
    <div
      className="fixed inset-0 z-[140] flex items-center justify-center p-4"
      style={{
        background: 'rgba(2, 8, 18, 0.78)',
        backdropFilter: 'blur(10px)',
        opacity: show ? 1 : 0,
        transition: 'opacity 0.2s',
      }}
    >
      <div
        className="relative w-full max-w-lg overflow-hidden rounded-[30px]"
        style={{
          background: 'linear-gradient(180deg, rgba(15, 27, 45, 0.98) 0%, rgba(11, 22, 37, 0.98) 100%)',
          border: '1px solid var(--color-border)',
          transform: show ? 'translateY(0)' : 'translateY(20px)',
          transition: 'transform 0.2s',
          boxShadow: 'var(--shadow-strong)',
        }}
      >
        <div className="border-b px-6 py-5" style={{ borderColor: 'var(--color-border)' }}>
          <div className="text-[11px] font-semibold uppercase tracking-[0.22em]" style={{ color: 'var(--color-accent-hover)' }}>
            Delete Task
          </div>
          <h2 className="mt-2 text-[24px] font-bold leading-snug" style={{ color: 'var(--color-text)' }}>
            删除云下载任务
          </h2>
          <p className="mt-2 text-sm leading-6" style={{ color: 'var(--color-muted)' }}>
            请选择要执行的删除方式。删除源文件后，115 网盘中的对应文件也会一并删除。
          </p>
        </div>

        <div className="px-6 py-5">
          <div
            className="rounded-[22px] px-4 py-4 text-sm break-all"
            style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)', color: 'var(--color-text)' }}
          >
            {task.name || task.url || task.info_hash}
          </div>

          <div className="mt-5 flex flex-wrap gap-3">
            <button
              onClick={onDeleteKeepSource}
              disabled={busy}
              className="rounded-full px-4 py-2.5 text-sm font-semibold transition-all disabled:opacity-40"
              style={{
                background: 'rgba(255,255,255,0.04)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-text)',
              }}
            >
              {busy ? '处理中…' : '不删除源文件'}
            </button>
            <button
              onClick={onDeleteSource}
              disabled={busy}
              className="rounded-full px-4 py-2.5 text-sm font-semibold transition-all disabled:opacity-40"
              style={{
                background: 'rgba(239, 125, 117, 0.12)',
                border: '1px solid rgba(239, 125, 117, 0.28)',
                color: 'var(--color-danger)',
              }}
            >
              {busy ? '处理中…' : '删除源文件'}
            </button>
            <button
              onClick={onClose}
              disabled={busy}
              className="rounded-full px-4 py-2.5 text-sm font-semibold transition-all disabled:opacity-40"
              style={{
                background: 'transparent',
                border: '1px solid rgba(255,255,255,0.08)',
                color: 'var(--color-muted)',
              }}
            >
              取消
            </button>
          </div>
        </div>
      </div>
    </div>,
    document.body
  )
}

export default function U115OfflinePage({ onToast }) {
  const [overview, setOverview] = useState(null)
  const [quota, setQuota] = useState(null)
  const [autoOrganizeStatus, setAutoOrganizeStatus] = useState(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [pageVisible, setPageVisible] = useState(() => (
    typeof document === 'undefined' ? true : document.visibilityState === 'visible'
  ))
  const [urls, setUrls] = useState('')
  const [wpPathId, setWpPathId] = useState('')
  const [driveSpace, setDriveSpace] = useState(null)
  const [parseTestFile, setParseTestFile] = useState('')
  const [deleteConfirmTask, setDeleteConfirmTask] = useState(null)
  const [page, setPage] = useState(1)

  const loadOverview = useCallback(async () => {
    try {
      const res = await getU115OfflineOverview({ page })
      setOverview(res.data)
    } catch (e) {
      onToast?.('error', '云下载加载失败', e?.response?.data?.detail || e.message || '读取 115 云下载概览失败')
    } finally {
      setLoading(false)
    }
  }, [onToast, page])

  const loadAutoOrganizeStatus = useCallback(async () => {
    try {
      const res = await getU115AutoOrganizeStatus()
      setAutoOrganizeStatus(res.data)
    } catch {
      setAutoOrganizeStatus(null)
    }
  }, [])

  const loadQuota = useCallback(async () => {
    try {
      const res = await getU115OfflineQuota()
      setQuota(res?.data?.quota || null)
    } catch {
      setQuota(null)
    }
  }, [])

  useEffect(() => {
    loadOverview()
  }, [loadOverview])

  useEffect(() => {
    loadQuota()
  }, [loadQuota])

  const loadDriveSpace = useCallback(async () => {
    try {
      const res = await testU115Connection()
      setDriveSpace(res?.data || null)
    } catch {
      setDriveSpace(null)
    }
  }, [])

  useEffect(() => {
    loadDriveSpace()
  }, [loadDriveSpace])

  useEffect(() => {
    loadAutoOrganizeStatus()
  }, [loadAutoOrganizeStatus])

  useEffect(() => {
    const handleVisibilityChange = () => {
      setPageVisible(document.visibilityState === 'visible')
    }
    document.addEventListener('visibilitychange', handleVisibilityChange)
    return () => {
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
  }, [])

  useEffect(() => {
    if (!pageVisible || busy) return
    loadOverview()
    loadAutoOrganizeStatus()
  }, [busy, loadAutoOrganizeStatus, loadOverview, pageVisible])

  useEffect(() => {
    if (!pageVisible) return undefined
    const timer = setInterval(() => {
      if (!busy) {
        loadOverview()
        loadAutoOrganizeStatus()
      }
    }, 15000)

    return () => {
      clearInterval(timer)
    }
  }, [busy, loadAutoOrganizeStatus, loadOverview, pageVisible])

  const tasks = useMemo(() => overview?.tasks || [], [overview])
  const pagination = overview?.pagination
  const latestTrigger = autoOrganizeStatus?.last_triggered
  const latestTriggerTask = latestTrigger?.tasks?.[0] || null
  const autoOrganizeState = !autoOrganizeStatus?.enabled
    ? {
        label: '已关闭',
        color: '#94a3b8',
        bg: 'rgba(148,163,184,0.12)',
        desc: '自动整理未启用',
      }
    : !autoOrganizeStatus?.authorized
      ? {
          label: '未授权',
          color: '#f59e0b',
          bg: 'rgba(245,158,11,0.14)',
          desc: '115 尚未授权，自动整理监听不会开始轮询',
        }
      : autoOrganizeStatus?.last_poll_error
        ? {
            label: '异常',
            color: '#ef4444',
            bg: 'rgba(239,68,68,0.12)',
            desc: '自动整理已启用，但最近一次轮询失败',
          }
        : {
            label: '运行中',
            color: '#3b82f6',
            bg: 'rgba(59,130,246,0.12)',
            desc: `每 ${autoOrganizeStatus?.poll_seconds ?? '-'} 秒检查一次，完成后等待 ${autoOrganizeStatus?.stable_seconds ?? '-'} 秒触发整理`,
          }

  async function handleAddUrls() {
    if (!urls.trim()) {
      onToast?.('warning', '缺少链接', '请至少输入一个下载链接')
      return
    }
    setBusy(true)
    try {
      const res = await addU115OfflineUrls({
        urls,
        wp_path_id: wpPathId.trim() || null,
      })
      const results = res?.data?.results || []
      const successCount = results.filter(item => item.state).length
      const failedCount = results.length - successCount
      onToast?.(
        failedCount > 0 ? 'warning' : 'success',
        '云下载任务已提交',
        failedCount > 0
          ? `成功 ${successCount} 条，失败 ${failedCount} 条`
          : `成功提交 ${successCount} 条链接`
      )
      setUrls('')
      await loadOverview()
      await loadAutoOrganizeStatus()
    } catch (e) {
      onToast?.('error', '添加任务失败', e?.response?.data?.detail || e.message || '115 云下载添加链接失败')
    } finally {
      setBusy(false)
    }
  }

  async function handleClear(flag) {
    setBusy(true)
    try {
      await clearU115OfflineTasks({ flag })
      onToast?.('success', '任务已清空', flag === 2 ? '已清空失败任务' : flag === 0 ? '已清空完成任务' : '已清空全部任务')
      await loadOverview()
      await loadAutoOrganizeStatus()
    } catch (e) {
      onToast?.('error', '清空任务失败', e?.response?.data?.detail || e.message || '115 云下载清空任务失败')
    } finally {
      setBusy(false)
    }
  }

  async function handleDeleteOne(infoHash, delSourceFile = 0) {
    if (!infoHash) return
    setBusy(true)
    try {
      await deleteU115OfflineTasks({
        info_hashes: [infoHash],
        del_source_file: delSourceFile,
      })
      onToast?.('success', '任务已删除', delSourceFile ? '已删除任务并删除源文件' : '已删除任务')
      setDeleteConfirmTask(null)
      await loadOverview()
      await loadAutoOrganizeStatus()
    } catch (e) {
      onToast?.('error', '删除任务失败', e?.response?.data?.detail || e.message || '115 云下载删除任务失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex-1 flex flex-col gap-5 min-w-0">
      <div className="mb-2 flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
        <div className="max-w-2xl">
          <div className="mb-2 text-[11px] font-semibold uppercase tracking-[0.24em]" style={{ color: 'var(--color-accent-hover)' }}>
            Offline Download Center
          </div>
          <h1 className="text-[34px] font-bold leading-tight" style={{ color: 'var(--color-text)' }}>
            115 云下载
          </h1>
          <p className="mt-2 text-sm" style={{ color: 'var(--color-muted)' }}>
            提交离线下载链接，并管理当前 115 云下载任务
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <ToolButton
            onClick={() => {
              loadOverview()
              loadQuota()
              loadDriveSpace()
              loadAutoOrganizeStatus()
            }}
            disabled={busy}
          >
            刷新
          </ToolButton>
          <ToolButton onClick={() => handleClear(0)} disabled={busy}>清空已完成</ToolButton>
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

      <div
        className="rounded-[24px] px-5 py-5"
        style={{
          background: 'linear-gradient(180deg, rgba(20, 37, 59, 0.88) 0%, rgba(13, 24, 39, 0.96) 100%)',
          border: '1px solid var(--color-border)',
          boxShadow: 'var(--shadow-soft)',
        }}
      >
        <div className="mb-3 flex flex-wrap items-center gap-2">
          <div className="text-[11px] font-semibold uppercase tracking-[0.2em]" style={{ color: 'var(--color-accent-hover)' }}>
            Auto Organize Watcher
          </div>
          <span
            className="rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.14em]"
            style={{
              background: autoOrganizeState.bg,
              color: autoOrganizeState.color,
            }}
          >
            {autoOrganizeState.label}
          </span>
        </div>
        <div className="mb-4 text-sm" style={{ color: 'var(--color-muted)' }}>
          {autoOrganizeState.desc}
        </div>
        <div className="grid gap-2 text-sm md:grid-cols-3" style={{ color: 'var(--color-text)' }}>
          <div>
            <div className="text-xs" style={{ color: 'var(--color-muted)' }}>上次轮询</div>
            <div>{formatTime(autoOrganizeStatus?.last_polled_at)}</div>
          </div>
          <div>
            <div className="text-xs" style={{ color: 'var(--color-muted)' }}>上次触发</div>
            <div>{formatTime(latestTrigger?.triggered_at)}</div>
          </div>
          <div>
            <div className="text-xs" style={{ color: 'var(--color-muted)' }}>轮询配置</div>
            <div>
              {autoOrganizeStatus
                ? `${autoOrganizeStatus.poll_seconds}s / 稳定等待 ${autoOrganizeStatus.stable_seconds}s`
                : '-'}
            </div>
          </div>
        </div>
        <div className="mt-3 grid gap-2 text-sm md:grid-cols-2">
          <div>
            <div className="text-xs" style={{ color: 'var(--color-muted)' }}>上次触发任务</div>
            <div className="break-all" style={{ color: 'var(--color-text)' }}>
              {latestTriggerTask?.name || '-'}
            </div>
          </div>
          <div>
            <div className="text-xs" style={{ color: 'var(--color-muted)' }}>监听目录</div>
            <div className="break-all" style={{ color: 'var(--color-text)' }}>
              {autoOrganizeStatus?.download_folder_id || '-'}
            </div>
          </div>
        </div>
        {autoOrganizeStatus?.last_poll_error ? (
          <div className="mt-3 rounded-[18px] px-4 py-3 text-xs break-all"
            style={{ background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.24)', color: '#ef4444' }}>
            最近一次轮询错误：{autoOrganizeStatus.last_poll_error}
          </div>
        ) : null}
      </div>

      <div
        className="rounded-[28px] px-5 py-5"
        style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)' }}
      >
        <div className="mb-3 text-[11px] font-semibold uppercase tracking-[0.2em]" style={{ color: 'var(--color-accent-hover)' }}>
          新建云下载
        </div>
        <textarea
          value={urls}
          onChange={(e) => setUrls(e.target.value)}
          placeholder={'每行一个链接，支持 HTTP、HTTPS、磁力链'}
          className="h-28 w-full rounded-[18px] px-4 py-3 text-sm outline-none"
          style={{
            background: 'var(--color-surface)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-text)',
            resize: 'vertical',
            overflowY: 'auto',
          }}
        />
        <div className="mt-3">
          <input
            value={wpPathId}
            onChange={(e) => setWpPathId(e.target.value)}
            placeholder="保存目录 ID（可选，留空则使用配置页中的云下载目录 ID）"
            className="w-full rounded-full px-4 py-2 text-sm outline-none"
            style={{
              background: 'rgba(255,255,255,0.03)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-text)',
            }}
          />
          <div className="mt-3 flex flex-wrap items-center gap-2.5">
            <button
              onClick={handleAddUrls}
              disabled={busy}
              className="rounded-full px-4 py-2 text-sm font-semibold transition-all disabled:opacity-40"
              style={{ background: 'linear-gradient(135deg, var(--color-accent) 0%, #b37533 100%)', color: '#fff', border: 'none' }}
            >
              {busy ? '提交中…' : '提交链接'}
            </button>
          </div>
        </div>
        <div className="mt-2 text-xs" style={{ color: 'var(--color-muted)' }}>
          提交后任务会出现在下方列表中
        </div>
      </div>

      <section className="panel-surface rounded-[32px] px-7 py-7">
        <div className="mb-6 flex flex-wrap items-center gap-3">
          <div>
            <div className="mb-1 text-[11px] font-semibold uppercase tracking-[0.2em]" style={{ color: 'var(--color-accent-hover)' }}>
              Offline Queue
            </div>
            <div className="text-base font-semibold" style={{ color: 'var(--color-text)' }}>任务列表</div>
          </div>
          <div className="ml-auto flex flex-wrap items-center gap-2">
            <ToolButton onClick={() => handleClear(2)} disabled={busy}>清空失败任务</ToolButton>
          </div>
        </div>

        {loading ? (
          <StatePanel
            title="正在加载云下载任务"
            description="任务列表和空间状态正在同步中。"
            compact
          />
        ) : tasks.length === 0 ? (
          <StatePanel
            icon="☁"
            title="当前没有云下载任务"
            description="在上方提交链接后，任务会出现在这里。"
          />
        ) : (
          <div className="grid gap-3">
            {tasks.map((task) => {
              const status = statusMeta(task.status)
              return (
                <div
                  key={task.info_hash}
                  className="rounded-[24px] px-5 py-5 transition-all duration-150"
                  style={{
                    background: 'linear-gradient(180deg, rgba(20, 37, 59, 0.82) 0%, rgba(13, 24, 39, 0.96) 100%)',
                    border: '1px solid var(--color-border)',
                    boxShadow: 'none',
                  }}
                >
                  <div className="flex flex-wrap items-start gap-3">
                    <div className="flex-1 min-w-0">
                      <div className="flex flex-wrap items-center gap-2 mb-2">
                        <span
                          className="rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.14em]"
                          style={{ background: status.bg, color: status.color }}
                        >
                          {status.label}
                        </span>
                        <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
                          {Math.round(Number(task.percent_done) || 0)}%
                        </span>
                      </div>
                      <div className="mb-3">
                        <div className="mb-1.5 flex justify-between text-xs" style={{ color: 'var(--color-muted)' }}>
                          <span>{formatBytes(task.size)}</span>
                          <span>{Math.round(Number(task.percent_done) || 0)}%</span>
                        </div>
                        <div className="h-1.5 overflow-hidden rounded-full" style={{ background: 'rgba(255,255,255,0.06)' }}>
                          <div
                            className="h-full rounded-full"
                            style={{
                              width: `${Math.max(Math.min(Number(task.percent_done) || 0, 100), task.status === 2 ? 100 : 0)}%`,
                              background: task.status === -1
                                ? 'linear-gradient(90deg,rgba(239,125,117,.9),rgba(239,125,117,.65))'
                                : 'linear-gradient(90deg,var(--color-accent),#dfb36f)',
                            }}
                          />
                        </div>
                      </div>
                      <div className="flex items-start gap-2">
                        <div className="flex-1 min-w-0 text-sm font-medium break-all" style={{ color: 'var(--color-text)' }}>
                          {task.name || task.url || task.info_hash}
                        </div>
                        <button
                          onClick={() => setParseTestFile(task.name || task.url || task.info_hash || '')}
                          className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full transition-all hover:opacity-80"
                          style={{
                            background: 'linear-gradient(135deg, #e3b778, #c8924d)',
                            border: '1px solid rgba(227,183,120,0.35)',
                            color: '#0A1320',
                            boxShadow: '0 4px 16px rgba(200,146,77,0.22)',
                          }}
                          title="解析此任务"
                        >
                          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                            <circle cx="11" cy="11" r="7" /><line x1="21" y1="21" x2="16.65" y2="16.65" />
                          </svg>
                        </button>
                      </div>
                      <div className="grid gap-1 mt-3 text-xs" style={{ color: 'var(--color-muted)' }}>
                        <div>添加时间：{formatTime(task.add_time)}</div>
                        <div>更新时间：{formatTime(task.last_update)}</div>
                        <div className="break-all">Info Hash：{task.info_hash || '-'}</div>
                        {task.wp_path_id ? <div className="break-all">保存目录：{task.wp_path_id}</div> : null}
                        {task.url ? <div className="break-all">链接：{task.url}</div> : null}
                      </div>
                    </div>
                    <ToolButton danger onClick={() => setDeleteConfirmTask(task)} disabled={busy}>删除</ToolButton>
                  </div>
                </div>
              )
            })}
          </div>
        )}

        <PaginationBar
          pagination={pagination}
          busy={busy}
          onChange={(nextPage) => {
            setPage(nextPage)
          }}
        />
      </section>

      {parseTestFile ? (
        <ParseTestModal
          initialFilename={parseTestFile}
          onClose={() => setParseTestFile('')}
        />
      ) : null}
      {deleteConfirmTask ? (
        <DeleteConfirmModal
          task={deleteConfirmTask}
          busy={busy}
          onDeleteKeepSource={() => handleDeleteOne(deleteConfirmTask.info_hash, 0)}
          onDeleteSource={() => handleDeleteOne(deleteConfirmTask.info_hash, 1)}
          onClose={() => !busy && setDeleteConfirmTask(null)}
        />
      ) : null}
    </div>
  )
}
