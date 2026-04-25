import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { RefreshCw, Search } from 'lucide-react'
import { getLogs, getPipelineStatus } from '@/api'
import { StatePanel } from '@/components/StatePanel'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

interface LogItem {
  ts: string
  level: string
  message: string
  event?: string
  details?: Record<string, unknown>
}

function formatTime(value: string | undefined): string {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

function formatEventDetails(item: LogItem): string {
  const details = item.details || {}

  switch (item.event) {
    case 'refresh_scan_finish': {
      const parts: string[] = []
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
      const parts: string[] = []
      if (details.name) parts.push(`任务名：${details.name}`)
      if (details.uriCount !== undefined) parts.push(`链接数：${details.uriCount}`)
      if (details.gid) parts.push(`任务 ID：${details.gid}`)
      return parts.join('，')
    }
    case 'task_added_torrent': {
      const parts: string[] = []
      if (details.name) parts.push(`任务名：${details.name}`)
      if (details.gid) parts.push(`任务 ID：${details.gid}`)
      return parts.join('，')
    }
    case 'tasks_paused':
    case 'tasks_unpaused':
    case 'tasks_removed': {
      const parts: string[] = []
      if (details.count !== undefined) parts.push(`任务数：${details.count}`)
      if (Array.isArray(details.gids) && details.gids.length) parts.push(`任务 ID：${details.gids.join(', ')}`)
      return parts.join('，')
    }
    case 'tasks_retried': {
      const parts: string[] = []
      if (Array.isArray(details.sourceGids) && details.sourceGids.length) parts.push(`原任务：${details.sourceGids.join(', ')}`)
      if (Array.isArray(details.newGids) && details.newGids.length) parts.push(`新任务：${details.newGids.join(', ')}`)
      return parts.join('，')
    }
    case 'options_updated':
      return Array.isArray(details.keys) && details.keys.length ? `更新项：${details.keys.join('、')}` : ''
    case 'aria2_rpc_error':
      return '请检查 aria2 服务是否正在运行'
    case 'aria2_rpc_invalid_json':
      return 'aria2 返回了意外的响应格式'
    case 'aria2_rpc_api_error': {
      const parts: string[] = []
      if (details.message) parts.push(String(details.message))
      if (details.code !== undefined) parts.push(`错误码 ${details.code}`)
      return parts.join('，')
    }
    case 'webhook_trigger': {
      const parts: string[] = []
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

function formatLogLine(item: LogItem): string {
  const detail = formatEventDetails(item)
  const detailSuffix = detail ? ` | ${detail}` : ''
  return `[${formatTime(item.ts)}] [${item.level}] ${item.message}${detailSuffix}`
}

const POLL_INTERVAL_MS = 2000

export default function LogsPage() {
  const [items, setItems] = useState<LogItem[]>([])
  const [level, setLevel] = useState('all')
  const [keyword, setKeyword] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [isLive, setIsLive] = useState(false)
  const [refreshing, setRefreshing] = useState(false)

  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const mountedRef = useRef(true)

  const fetchLogs = useCallback(async (showLoading = false) => {
    if (showLoading) setLoading(true)
    try {
      const res = await getLogs({ limit: 500, ...(level !== 'all' ? { level } : {}) })
      if (!mountedRef.current) return
      setItems(res.data.items || [])
      setError('')
    } catch (err) {
      if (!mountedRef.current) return
      setError((err as { response?: { data?: { detail?: string } } })?.response?.data?.detail || '加载日志失败')
    } finally {
      if (mountedRef.current && showLoading) setLoading(false)
    }
  }, [level])

  const tick = useCallback(async () => {
    if (!mountedRef.current) return
    try {
      const res = await getPipelineStatus()
      const running = res?.data?.running || res?.data?.debounce || false
      if (!mountedRef.current) return
      setIsLive(running)
      await fetchLogs(false)
      if (running && mountedRef.current) {
        timerRef.current = setTimeout(tick, POLL_INTERVAL_MS)
      } else {
        timerRef.current = null
      }
    } catch {
      if (mountedRef.current) setIsLive(false)
      timerRef.current = null
    }
  }, [fetchLogs])

  useEffect(() => {
    mountedRef.current = true
    setLoading(true)
    fetchLogs(false).then(() => {
      if (mountedRef.current) setLoading(false)
    })
    timerRef.current = setTimeout(tick, POLL_INTERVAL_MS)

    return () => {
      mountedRef.current = false
      if (timerRef.current) {
        clearTimeout(timerRef.current)
        timerRef.current = null
      }
    }
  }, [fetchLogs, tick])

  const handleRefresh = useCallback(async () => {
    setRefreshing(true)
    await fetchLogs(false)
    setRefreshing(false)
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = setTimeout(tick, POLL_INTERVAL_MS)
  }, [fetchLogs, tick])

  const filteredLines = useMemo(() => {
    const needle = keyword.trim().toLowerCase()
    const lines = items.map(formatLogLine)
    if (!needle) return lines
    return lines.filter((line) => line.toLowerCase().includes(needle))
  }, [items, keyword])

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-col gap-5 lg:flex-row lg:items-center lg:justify-between">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-bold leading-tight sm:text-[34px]">日志</h1>
          {isLive && (
            <span className="flex items-center gap-1.5 rounded-full bg-success/20 px-3 py-1 text-xs font-semibold text-success">
              <span className="size-2 animate-pulse rounded-full bg-success" />
              LIVE
            </span>
          )}
        </div>

        <div className="flex items-center gap-3">
          <div className="relative w-60">
            <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              type="text"
              value={keyword}
              onChange={(e) => setKeyword(e.target.value)}
              placeholder="搜索日志"
              className="h-11 rounded-full pl-10"
            />
          </div>
          <Select value={level} onValueChange={setLevel}>
            <SelectTrigger className="h-11 w-36 rounded-full">
              <SelectValue placeholder="级别: 全部" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部</SelectItem>
              <SelectItem value="INFO">INFO</SelectItem>
              <SelectItem value="WARNING">WARNING</SelectItem>
              <SelectItem value="ERROR">ERROR</SelectItem>
              <SelectItem value="SUCCESS">SUCCESS</SelectItem>
            </SelectContent>
          </Select>
          <Button
            variant="outline"
            size="sm"
            onClick={handleRefresh}
            disabled={refreshing || loading}
            className="gap-2"
          >
            <RefreshCw className={`size-4 ${refreshing ? 'animate-spin' : ''}`} />
            刷新
          </Button>
        </div>
      </div>

      <section className="rounded-3xl border bg-card px-6 py-6">
        <div className="mb-4 flex items-center justify-between gap-3">
          <div>
            <h2 className="text-lg font-semibold">日志内容</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              最新 500 条日志，当前显示 {filteredLines.length} 条
              {isLive && <span className="text-success"> · 整理中，每 2 秒自动刷新</span>}
            </p>
          </div>
        </div>

        {loading ? (
          <StatePanel title="正在加载日志" description="最近日志内容正在准备中。" compact />
        ) : error ? (
          <StatePanel icon="!" title={error} description="请检查日志服务状态，或稍后重试。" tone="danger" compact />
        ) : filteredLines.length === 0 ? (
          <StatePanel icon="≡" title="当前筛选条件下没有日志" description="调整级别或关键词后再试一次。" compact />
        ) : (
          <div className="h-[70vh] overflow-auto rounded-2xl border bg-muted/30 px-5 py-4">
            <div className="space-y-1 font-mono text-sm leading-7">
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
