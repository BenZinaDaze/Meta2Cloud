import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  addU115OfflineUrls,
  clearU115OfflineTasks,
  deleteU115OfflineTasks,
  getU115OfflineOverview,
  testU115Connection,
} from '../api'
import ParseTestModal from './ParseTestModal'

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
  const value = Number(ts)
  if (!Number.isFinite(value)) return '-'
  const ms = value < 1e12 ? value * 1000 : value
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

function TaskCheckbox({ checked, onChange, label }) {
  return (
    <button
      type="button"
      role="checkbox"
      aria-checked={checked}
      aria-label={label}
      onClick={onChange}
      className="flex items-center justify-center transition-all"
      style={{
        width: 22,
        height: 22,
        marginTop: 2,
        borderRadius: 7,
        border: checked ? '1px solid var(--color-accent)' : '1px solid var(--color-border)',
        background: checked ? 'var(--color-accent)' : 'var(--color-surface)',
        boxShadow: checked ? '0 8px 20px rgba(180, 117, 51, 0.22)' : 'none',
        color: '#fff',
        flexShrink: 0,
      }}
    >
      <svg
        width="13"
        height="13"
        viewBox="0 0 16 16"
        fill="none"
        style={{ opacity: checked ? 1 : 0, transform: checked ? 'scale(1)' : 'scale(0.7)', transition: 'all 0.16s ease' }}
      >
        <path
          d="M3.5 8.5L6.5 11.5L12.5 4.5"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
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

export default function U115OfflinePage({ onToast }) {
  const [overview, setOverview] = useState(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [urls, setUrls] = useState('')
  const [wpPathId, setWpPathId] = useState('')
  const [selected, setSelected] = useState([])
  const [driveSpace, setDriveSpace] = useState(null)
  const [parseTestFile, setParseTestFile] = useState('')
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

  useEffect(() => {
    loadOverview()
  }, [loadOverview])

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
    const timer = setInterval(() => {
      if (!busy) {
        loadOverview()
        loadDriveSpace()
      }
    }, 5000)

    return () => {
      clearInterval(timer)
    }
  }, [busy, loadDriveSpace, loadOverview])

  const tasks = useMemo(() => overview?.tasks || [], [overview])
  const quota = overview?.quota
  const pagination = overview?.pagination

  const selectedSet = useMemo(() => new Set(selected), [selected])

  useEffect(() => {
    setSelected(prev => prev.filter(infoHash => tasks.some(task => task.info_hash === infoHash)))
  }, [tasks])

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
    } catch (e) {
      onToast?.('error', '添加任务失败', e?.response?.data?.detail || e.message || '115 云下载添加链接失败')
    } finally {
      setBusy(false)
    }
  }

  async function handleDelete(infoHashes, delSourceFile = 0) {
    if (!infoHashes.length) return
    setBusy(true)
    try {
      await deleteU115OfflineTasks({
        info_hashes: infoHashes,
        del_source_file: delSourceFile,
      })
      onToast?.('success', '任务已删除', `已删除 ${infoHashes.length} 个云下载任务`)
      setSelected(prev => prev.filter(item => !infoHashes.includes(item)))
      await loadOverview()
    } catch (e) {
      onToast?.('error', '删除任务失败', e?.response?.data?.detail || e.message || '115 云下载删除任务失败')
    } finally {
      setBusy(false)
    }
  }

  async function handleClear(flag) {
    setBusy(true)
    try {
      await clearU115OfflineTasks({ flag })
      onToast?.('success', '任务已清空', flag === 1 ? '已清空失败任务' : flag === 2 ? '已清空完成任务' : '已清空全部任务')
      setSelected([])
      await loadOverview()
    } catch (e) {
      onToast?.('error', '清空任务失败', e?.response?.data?.detail || e.message || '115 云下载清空任务失败')
    } finally {
      setBusy(false)
    }
  }

  function toggleTask(infoHash) {
    setSelected(prev => (
      prev.includes(infoHash)
        ? prev.filter(item => item !== infoHash)
        : [...prev, infoHash]
    ))
  }

  return (
    <div className="flex-1 flex flex-col gap-5 min-w-0" style={{ maxWidth: 1440 }}>
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
          <ToolButton onClick={loadOverview} disabled={busy}>刷新</ToolButton>
          <ToolButton onClick={() => handleClear(2)} disabled={busy}>清空已完成</ToolButton>
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
            <ToolButton onClick={() => handleDelete(selected, 0)} disabled={busy || selected.length === 0}>删除选中</ToolButton>
            <ToolButton danger onClick={() => handleDelete(selected, 1)} disabled={busy || selected.length === 0}>删除并删源文件</ToolButton>
            <ToolButton onClick={() => handleClear(1)} disabled={busy}>清空失败任务</ToolButton>
          </div>
        </div>

        {loading ? (
          <div className="py-20 text-center text-sm" style={{ color: 'var(--color-muted)' }}>正在加载云下载任务…</div>
        ) : tasks.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-4 py-24">
            <span style={{ fontSize: 96, lineHeight: 1 }}>☁</span>
            <p className="text-xl font-medium" style={{ color: 'var(--color-muted)' }}>当前没有云下载任务</p>
          </div>
        ) : (
          <div className="grid gap-3">
            {tasks.map((task) => {
              const status = statusMeta(task.status)
              const checked = selectedSet.has(task.info_hash)
              return (
                <div
                  key={task.info_hash}
                  className="rounded-[24px] px-5 py-5 transition-all duration-150"
                  style={{
                    background: checked
                      ? 'linear-gradient(180deg, rgba(41, 56, 75, 0.94) 0%, rgba(22, 33, 48, 0.98) 100%)'
                      : 'linear-gradient(180deg, rgba(20, 37, 59, 0.82) 0%, rgba(13, 24, 39, 0.96) 100%)',
                    border: checked
                      ? '1px solid rgba(200, 146, 77, 0.28)'
                      : '1px solid var(--color-border)',
                    boxShadow: checked ? '0 0 0 1px rgba(200,146,77,0.12) inset' : 'none',
                  }}
                >
                  <div className="flex flex-wrap items-start gap-3">
                    <TaskCheckbox
                      checked={checked}
                      onChange={() => toggleTask(task.info_hash)}
                      label={`选择任务 ${task.name || task.url || task.info_hash}`}
                    />
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
                    <ToolButton danger onClick={() => handleDelete([task.info_hash], 0)} disabled={busy}>删除</ToolButton>
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
            setSelected([])
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
    </div>
  )
}
