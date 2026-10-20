import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { RefreshCw, Search } from 'lucide-react'
import { getLogs, getPipelineStatus } from '@/api'
import BackToTopButton from '@/components/BackToTopButton'
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
import PageHeader from '@/components/shared/PageHeader'
import { formatDateTime } from '@/lib/time'
import { useWebSocket, type WsMessage } from '@/hooks/useWebSocket'
import { cn } from '@/lib/utils'

interface LogItem {
  ts: string
  level: string
  message: string
  event?: string
  details?: Record<string, unknown>
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
    case 'pipeline_output':
      return ''
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
  return `[${formatDateTime(item.ts)}] [${item.level}] ${item.message}${detailSuffix}`
}

function wsToLogItem(msg: WsMessage): LogItem {
  return {
    ts: msg.ts || new Date().toISOString(),
    level: msg.level,
    message: msg.message,
    event: 'pipeline_output',
    details: msg.runId ? { runId: msg.runId } : {},
  }
}

function levelColor(line: string): string {
  if (line.includes('[ERROR]')) return 'text-destructive'
  if (line.includes('[WARNING]')) return 'text-warning'
  if (line.includes('[SUCCESS]')) return 'text-success'
  return 'text-foreground/80'
}

const STATUS_POLL_MS = 5000
const MAX_ITEMS = 1000

export default function LogsPage() {
  const [items, setItems] = useState<LogItem[]>([])
  const [level, setLevel] = useState('all')
  const [keyword, setKeyword] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [isLive, setIsLive] = useState(false)
  const [refreshing, setRefreshing] = useState(false)

  const mountedRef = useRef(true)
  const statusTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

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

  const handleWsMessage = useCallback((msg: WsMessage) => {
    if (!mountedRef.current) return
    const item = wsToLogItem(msg)
    setItems((prev) => {
      const next = [item, ...prev]
      return next.length > MAX_ITEMS ? next.slice(0, MAX_ITEMS) : next
    })
  }, [])

  const { connected } = useWebSocket({ onMessage: handleWsMessage })

  // Poll pipeline status to show LIVE badge
  useEffect(() => {
    let active = true
    const tick = async () => {
      if (!active) return
      try {
        const res = await getPipelineStatus()
        if (!active) return
        setIsLive(res?.data?.running || res?.data?.debounce || false)
      } catch {
        if (active) setIsLive(false)
      }
      if (active) {
        statusTimerRef.current = setTimeout(tick, STATUS_POLL_MS)
      }
    }
    tick()
    return () => {
      active = false
      if (statusTimerRef.current) clearTimeout(statusTimerRef.current)
    }
  }, [])

  // Initial load and re-fetch on level change
  useEffect(() => {
    mountedRef.current = true
    setLoading(true)
    fetchLogs(false).then(() => {
      if (mountedRef.current) setLoading(false)
    })

    return () => {
      mountedRef.current = false
    }
  }, [fetchLogs])

  const handleRefresh = useCallback(async () => {
    setRefreshing(true)
    await fetchLogs(false)
    setRefreshing(false)
  }, [fetchLogs])

  const filteredLines = useMemo(() => {
    const needle = keyword.trim().toLowerCase()
    const lines = items.map(formatLogLine)
    if (!needle) return lines
    return lines.filter((line) => line.toLowerCase().includes(needle))
  }, [items, keyword])

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        kicker="System Activity"
        title={
          <span className="flex items-center gap-3">
            日志
            {!isLive && (
              <span className="relative flex size-2.5">
                <span
                  className={cn(
                    'absolute inline-flex size-2.5 rounded-full opacity-60',
                    connected ? 'bg-success animate-tide-ping' : ''
                  )}
                />
                <span className={cn('relative inline-flex size-2.5 rounded-full', connected ? 'bg-success' : 'bg-destructive')} />
              </span>
            )}
          </span>
        }
        description="实时跟踪整理流水线、下载任务与系统事件。"
        actions={
          <>
            {isLive && (
              <span className="flex items-center gap-1.5 rounded-full bg-success/12 px-3 py-1.5 text-xs font-semibold text-success">
                <span className="relative flex size-2">
                  <span className="absolute inline-flex size-2 animate-tide-ping rounded-full bg-success opacity-60" />
                  <span className="relative inline-flex size-2 rounded-full bg-success" />
                </span>
                整理中
              </span>
            )}
            <div className="relative w-60">
              <Search className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                type="text"
                value={keyword}
                onChange={(e) => setKeyword(e.target.value)}
                placeholder="搜索日志"
                className="h-10 rounded-full pl-10"
              />
            </div>
            <Select value={level} onValueChange={setLevel}>
              <SelectTrigger className="h-10 w-36 rounded-full border-border/70 bg-card/50 focus:ring-0 focus:ring-offset-0">
                <SelectValue placeholder="级别：全部" />
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
              onClick={handleRefresh}
              disabled={refreshing || loading}
              className="h-10 gap-2 rounded-full px-4"
            >
              <RefreshCw className={cn('size-4', refreshing && 'animate-spin')} />
              刷新
            </Button>
          </>
        }
      />

      <section className="rounded-2xl border border-border/70 bg-card/40 px-4 py-5 sm:px-6 sm:py-6">
        <div className="mb-4 flex items-center justify-between gap-3">
          <div>
            <h2 className="text-lg font-semibold tracking-tight">日志内容</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              最新 500 条日志，当前显示 {filteredLines.length} 条
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
          <div data-scroll-container="logs-panel" className="h-[70vh] overflow-auto rounded-xl border border-border/60 bg-background/60 px-5 py-4">
            <div className="space-y-0.5 font-mono text-[13px] leading-6">
              {filteredLines.map((line, index) => (
                <div key={`${index}-${line}`} className={cn('break-all transition-colors hover:text-foreground', levelColor(line))}>
                  {line}
                </div>
              ))}
            </div>
          </div>
        )}
      </section>
      <BackToTopButton containerSelector='[data-scroll-container="logs-panel"]' className="fixed right-6 bottom-24 z-50 rounded-full shadow-lg lg:bottom-6" />
    </div>
  )
}
