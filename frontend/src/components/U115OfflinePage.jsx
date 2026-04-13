import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  addU115OfflineUrls,
  clearU115OfflineTasks,
  deleteU115OfflineTasks,
  getU115OfflineOverview,
  testU115Connection,
} from '../api'

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

function QuotaCard({ label, value, hint }) {
  return (
    <div
      className="rounded-2xl p-4"
      style={{ background: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <div className="text-xs mb-1" style={{ color: 'var(--color-muted)' }}>{label}</div>
      <div className="text-lg font-semibold" style={{ color: 'var(--color-text)' }}>{value}</div>
      {hint ? <div className="text-xs mt-1" style={{ color: 'var(--color-muted)' }}>{hint}</div> : null}
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

  const loadOverview = useCallback(async () => {
    try {
      const res = await getU115OfflineOverview()
      setOverview(res.data)
    } catch (e) {
      onToast?.('error', '云下载加载失败', e?.response?.data?.detail || e.message || '读取 115 云下载概览失败')
    } finally {
      setLoading(false)
    }
  }, [onToast])

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
      <div className="flex flex-wrap items-center gap-3">
        <div>
          <h1 className="text-xl font-bold" style={{ color: 'var(--color-text)' }}>115 云下载</h1>
          <p className="text-sm mt-0.5" style={{ color: 'var(--color-muted)' }}>
            提交离线下载链接，并管理当前 115 云下载任务
          </p>
        </div>
        <div className="ml-auto flex items-center gap-2">
          <button
            onClick={loadOverview}
            disabled={busy}
            className="text-sm px-4 py-2 rounded-lg transition-all disabled:opacity-40"
            style={{ background: 'var(--color-surface)', border: '1px solid var(--color-border)', color: 'var(--color-text)' }}
          >
            刷新
          </button>
          <button
            onClick={() => handleClear(2)}
            disabled={busy}
            className="text-sm px-4 py-2 rounded-lg transition-all disabled:opacity-40"
            style={{ background: 'var(--color-surface)', border: '1px solid var(--color-border)', color: 'var(--color-text)' }}
          >
            清空已完成
          </button>
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        <QuotaCard
          label="剩余配额"
          value={quota ? `${quota.surplus} / ${quota.count}` : '-'}
          hint="剩余次数 / 总配额"
        />
        <QuotaCard label="任务总数" value={`${tasks.length}`} hint="当前云下载任务列表" />
        <QuotaCard
          label="115 网盘容量"
          value={driveSpace ? `${formatBytes(driveSpace.remain_space)} / ${formatBytes(driveSpace.total_space)}` : '无授权'}
          hint={driveSpace ? '剩余空间 / 总空间' : '请先到配置页完成 115 授权'}
        />
      </div>

      <div
        className="rounded-3xl p-5"
        style={{ background: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
      >
        <div className="text-base font-semibold mb-3" style={{ color: 'var(--color-text)' }}>新增云下载任务</div>
        <div className="grid gap-3">
          <textarea
            value={urls}
            onChange={(e) => setUrls(e.target.value)}
            placeholder={'每行一个链接\n支持 http / https / magnet 等离线下载地址'}
            className="w-full rounded-2xl px-4 py-3 text-sm outline-none"
            rows={6}
            style={{
              background: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-text)',
              resize: 'vertical',
            }}
          />
          <input
            value={wpPathId}
            onChange={(e) => setWpPathId(e.target.value)}
            placeholder="保存目录 ID（可选）"
            className="w-full rounded-2xl px-4 py-3 text-sm outline-none"
            style={{
              background: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-text)',
            }}
          />
          <div className="flex flex-wrap items-center gap-2">
            <button
              onClick={handleAddUrls}
              disabled={busy}
              className="text-sm px-5 py-2 rounded-lg font-medium transition-all disabled:opacity-40"
              style={{ background: 'var(--color-accent)', color: '#fff', border: 'none' }}
            >
              {busy ? '提交中…' : '提交链接'}
            </button>
            <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
              提交后任务会出现在下方列表中
            </span>
          </div>
        </div>
      </div>

      <div
        className="rounded-3xl p-5"
        style={{ background: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
      >
        <div className="flex flex-wrap items-center gap-3 mb-4">
          <div className="text-base font-semibold" style={{ color: 'var(--color-text)' }}>任务列表</div>
          <div className="ml-auto flex flex-wrap items-center gap-2">
            <button
              onClick={() => handleDelete(selected, 0)}
              disabled={busy || selected.length === 0}
              className="text-xs px-3 py-1.5 rounded-full transition-all disabled:opacity-40"
              style={{ background: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-text)' }}
            >
              删除选中
            </button>
            <button
              onClick={() => handleDelete(selected, 1)}
              disabled={busy || selected.length === 0}
              className="text-xs px-3 py-1.5 rounded-full transition-all disabled:opacity-40"
              style={{ background: 'rgba(239,68,68,0.12)', border: '1px solid rgba(239,68,68,0.22)', color: '#ef4444' }}
            >
              删除并删源文件
            </button>
            <button
              onClick={() => handleClear(1)}
              disabled={busy}
              className="text-xs px-3 py-1.5 rounded-full transition-all disabled:opacity-40"
              style={{ background: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-text)' }}
            >
              清空失败任务
            </button>
          </div>
        </div>

        {loading ? (
          <div className="text-sm" style={{ color: 'var(--color-muted)' }}>正在加载云下载任务…</div>
        ) : tasks.length === 0 ? (
          <div className="text-sm" style={{ color: 'var(--color-muted)' }}>当前没有云下载任务</div>
        ) : (
          <div className="grid gap-3">
            {tasks.map((task) => {
              const status = statusMeta(task.status)
              const checked = selectedSet.has(task.info_hash)
              return (
                <div
                  key={task.info_hash}
                  className="rounded-2xl p-4"
                  style={{ background: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
                >
                  <div className="flex flex-wrap items-start gap-3">
                    <input
                      type="checkbox"
                      checked={checked}
                      onChange={() => toggleTask(task.info_hash)}
                      style={{ marginTop: 4 }}
                    />
                    <div className="flex-1 min-w-0">
                      <div className="flex flex-wrap items-center gap-2 mb-2">
                        <span
                          className="text-xs px-2 py-1 rounded-full"
                          style={{ background: status.bg, color: status.color }}
                        >
                          {status.label}
                        </span>
                        <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
                          {Math.round(Number(task.percent_done) || 0)}%
                        </span>
                      </div>
                      <div className="text-sm font-medium break-all" style={{ color: 'var(--color-text)' }}>
                        {task.name || task.url || task.info_hash}
                      </div>
                      <div className="grid gap-1 mt-3 text-xs" style={{ color: 'var(--color-muted)' }}>
                        <div>大小：{formatBytes(task.size)}</div>
                        <div>添加时间：{formatTime(task.add_time)}</div>
                        <div>更新时间：{formatTime(task.last_update)}</div>
                        <div className="break-all">Info Hash：{task.info_hash || '-'}</div>
                        {task.url ? <div className="break-all">链接：{task.url}</div> : null}
                      </div>
                    </div>
                    <button
                      onClick={() => handleDelete([task.info_hash], 0)}
                      disabled={busy}
                      className="text-xs px-3 py-1.5 rounded-full transition-all disabled:opacity-40"
                      style={{ background: 'var(--color-surface)', border: '1px solid var(--color-border)', color: 'var(--color-text)' }}
                    >
                      删除
                    </button>
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}
