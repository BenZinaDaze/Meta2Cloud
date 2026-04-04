import { useEffect, useMemo, useState } from 'react'
import { getLogs } from '../api'

function Dropdown({ value, onChange, options, label = '' }) {
  const [open, setOpen] = useState(false)
  const selected = options.find((option) => option.value === value) || options[0]

  return (
    <div className="relative z-20">
      <button
        type="button"
        onClick={() => setOpen((prev) => !prev)}
        className="flex min-w-[148px] items-center justify-between gap-3 rounded-full px-4 py-2 text-sm outline-none transition-all duration-150"
        style={{
          background: 'rgba(255,255,255,0.03)',
          border: '1px solid var(--color-border)',
          color: 'var(--color-text)',
        }}
      >
        <span>{label ? `${label}: ${selected.label}` : selected.label}</span>
        <span
          className="flex h-4 w-4 items-center justify-center transition-transform duration-150"
          style={{ color: 'var(--color-muted)', transform: open ? 'rotate(180deg)' : 'rotate(0deg)' }}
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <polyline points="6 9 12 15 18 9"/>
          </svg>
        </span>
      </button>

      {open ? (
        <>
          <div
            className="absolute right-0 z-20 mt-2 min-w-full overflow-hidden rounded-[20px] p-2"
            style={{
              background: 'linear-gradient(180deg, rgba(20, 37, 59, 0.98) 0%, rgba(14, 28, 46, 0.99) 100%)',
              border: '1px solid var(--color-border)',
              boxShadow: 'var(--shadow-soft)',
              backdropFilter: 'blur(18px)',
            }}
          >
            {options.map((option) => {
              const active = value === option.value
              return (
                <button
                  key={option.value}
                  type="button"
                  onClick={() => {
                    onChange(option.value)
                    setOpen(false)
                  }}
                  className="flex w-full items-center rounded-2xl px-3 py-2.5 text-left text-sm font-medium transition-all duration-150"
                  style={{
                    color: active ? 'var(--color-accent-hover)' : 'var(--color-text)',
                    background: active ? 'rgba(200, 146, 77, 0.14)' : 'transparent',
                    border: active ? '1px solid rgba(200, 146, 77, 0.22)' : '1px solid transparent',
                  }}
                >
                  {option.label}
                </button>
              )
            })}
          </div>
          <button
            type="button"
            aria-label="关闭菜单"
            onClick={() => setOpen(false)}
            className="fixed inset-0 z-10 cursor-default"
            style={{ background: 'transparent' }}
          />
        </>
      ) : null}
    </div>
  )
}

function SearchField({ value, onChange }) {
  return (
    <input
      type="text"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder="搜索日志"
      className="min-w-[280px] rounded-full px-4 py-2 text-sm outline-none"
      style={{
        background: 'rgba(255,255,255,0.03)',
        border: '1px solid var(--color-border)',
        color: 'var(--color-text)',
      }}
    />
  )
}

function formatTime(value) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

function formatEventDetails(item) {
  const details = item.details || {}

  switch (item.event) {
    case 'refresh_scan_finish': {
      const parts = []
      if (details.added !== undefined) parts.push(`新增 ${details.added} 项`)
      if (details.removed !== undefined) parts.push(`移除 ${details.removed} 项`)
      if (details.new_movies !== undefined) parts.push(`新增电影 ${details.new_movies} 部`)
      if (details.new_tv !== undefined) parts.push(`新增剧集 ${details.new_tv} 部`)
      if (details.total_movies !== undefined) parts.push(`电影总数 ${details.total_movies}`)
      if (details.total_tv !== undefined) parts.push(`剧集总数 ${details.total_tv}`)
      return parts.join('，')
    }
    case 'refresh_failed':
    case 'pipeline_refresh_failed':
    case 'pipeline_exception':
      return details.error ? `原因：${details.error}` : ''
    case 'pipeline_finish':
      return details.returncode !== undefined ? `退出码 ${details.returncode}` : ''
    case 'task_added_uri': {
      const parts = []
      if (details.name) parts.push(`任务名：${details.name}`)
      if (details.uriCount !== undefined) parts.push(`链接数：${details.uriCount}`)
      if (details.gid) parts.push(`任务 ID：${details.gid}`)
      return parts.join('，')
    }
    case 'task_added_torrent': {
      const parts = []
      if (details.name) parts.push(`任务名：${details.name}`)
      if (details.gid) parts.push(`任务 ID：${details.gid}`)
      return parts.join('，')
    }
    case 'tasks_paused':
    case 'tasks_unpaused':
    case 'tasks_removed': {
      const parts = []
      if (details.count !== undefined) parts.push(`任务数：${details.count}`)
      if (Array.isArray(details.gids) && details.gids.length) parts.push(`任务 ID：${details.gids.join(', ')}`)
      return parts.join('，')
    }
    case 'tasks_retried': {
      const parts = []
      if (Array.isArray(details.sourceGids) && details.sourceGids.length) parts.push(`原任务：${details.sourceGids.join(', ')}`)
      if (Array.isArray(details.newGids) && details.newGids.length) parts.push(`新任务：${details.newGids.join(', ')}`)
      return parts.join('，')
    }
    case 'options_updated':
      return Array.isArray(details.keys) && details.keys.length ? `更新项：${details.keys.join('、')}` : ''
    case 'aria2_rpc_error':
    case 'aria2_rpc_invalid_json':
    case 'aria2_rpc_api_error': {
      const parts = []
      if (details.method) parts.push(`方法：${details.method}`)
      if (details.message) parts.push(`信息：${details.message}`)
      if (details.code !== undefined) parts.push(`代码：${details.code}`)
      if (details.error) parts.push(`原因：${details.error}`)
      return parts.join('，')
    }
    case 'webhook_trigger': {
      const parts = []
      if (details.path) parts.push(`来源路径：${details.path}`)
      if (details.debounceSeconds !== undefined) parts.push(`防抖：${details.debounceSeconds} 秒`)
      return parts.join('，')
    }
    case 'pipeline_schedule':
    case 'pipeline_schedule_reset':
      return details.debounceSeconds !== undefined ? `防抖：${details.debounceSeconds} 秒` : ''
    default:
      return ''
  }
}

function formatLogLine(item) {
  const detail = formatEventDetails(item)
  const detailSuffix = detail ? ` | ${detail}` : ''
  return `[${formatTime(item.ts)}] [${item.level}] ${item.message}${detailSuffix}`
}

export default function LogsPage() {
  const [items, setItems] = useState([])
  const [level, setLevel] = useState('')
  const [keyword, setKeyword] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false

    async function loadLogs() {
      setLoading(true)
      try {
        const res = await getLogs({
          limit: 200,
          ...(level ? { level } : {}),
        })
        if (cancelled) return
        setItems(res.data.items || [])
        setError('')
      } catch (err) {
        if (cancelled) return
        setError(err?.response?.data?.detail || err.message || '加载日志失败')
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    loadLogs()
    return () => {
      cancelled = true
    }
  }, [level])

  const filteredLines = useMemo(() => {
    const needle = keyword.trim().toLowerCase()
    const lines = items.map(formatLogLine)
    if (!needle) return lines
    return lines.filter((line) => line.toLowerCase().includes(needle))
  }, [items, keyword])

  return (
    <div className="flex w-full flex-col gap-6 px-3 pb-8">
      <section className="panel-surface rounded-[32px] px-8 py-8">
        <div className="flex flex-col gap-5 lg:flex-row lg:items-center lg:justify-between">
          <h1 className="text-[36px] leading-[1.05]" style={{ color: 'var(--color-text)' }}>
            日志
          </h1>

          <div className="flex flex-wrap items-center gap-3">
            <SearchField value={keyword} onChange={setKeyword} />
            <Dropdown
              value={level}
              onChange={setLevel}
              label="级别"
              options={[
                { value: '', label: '全部' },
                { value: 'INFO', label: 'INFO' },
                { value: 'WARNING', label: 'WARNING' },
                { value: 'ERROR', label: 'ERROR' },
                { value: 'SUCCESS', label: 'SUCCESS' },
              ]}
            />
          </div>
        </div>
      </section>

      <section
        className="rounded-[28px] px-6 py-6"
        style={{
          background: 'linear-gradient(180deg, rgba(15, 27, 45, 0.96) 0%, rgba(10, 19, 32, 0.98) 100%)',
          border: '1px solid var(--color-border)',
          boxShadow: 'var(--shadow-soft)',
        }}
      >
        <div className="mb-4 flex items-center justify-between gap-3">
          <div>
            <h2 className="text-lg font-semibold" style={{ color: 'var(--color-text)' }}>日志内容</h2>
            <p className="mt-1 text-sm" style={{ color: 'var(--color-muted)' }}>最新 200 条日志，当前显示 {filteredLines.length} 条</p>
          </div>
        </div>

        {loading ? (
          <div className="rounded-[22px] px-5 py-10 text-center text-sm" style={{ color: 'var(--color-muted)', background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)' }}>
            正在加载日志…
          </div>
        ) : error ? (
          <div className="rounded-[22px] px-5 py-10 text-center text-sm" style={{ color: 'var(--color-danger)', background: 'rgba(239,125,117,0.08)', border: '1px solid rgba(239,125,117,0.2)' }}>
            {error}
          </div>
        ) : filteredLines.length === 0 ? (
          <div className="rounded-[22px] px-5 py-10 text-center text-sm" style={{ color: 'var(--color-muted)', background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)' }}>
            当前筛选条件下没有日志
          </div>
        ) : (
          <div
            className="overflow-x-auto rounded-[22px] px-5 py-4"
            style={{
              background: 'rgba(6, 13, 24, 0.72)',
              border: '1px solid var(--color-border)',
            }}
          >
            <div className="space-y-1 font-mono text-sm leading-7" style={{ color: 'var(--color-text)' }}>
              {filteredLines.map((line, index) => (
                <div key={`${index}-${line}`} className="break-all">
                  {line}
                </div>
              ))}
            </div>
          </div>
        )}
      </section>
    </div>
  )
}
