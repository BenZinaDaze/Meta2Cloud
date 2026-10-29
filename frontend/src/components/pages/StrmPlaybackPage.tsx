import { Fragment, useState, useEffect, useCallback, useRef } from 'react'
import {
  ChevronLeft,
  ChevronRight,
  ChevronDown,
  CheckCircle,
  AlertCircle,
  CornerDownRight,
  Play,
  Zap,
  Gauge,
  Users,
  Clock,
} from 'lucide-react'
import { getStrmPlaybackHistory } from '@/api'
import { StatePanel } from '@/components/StatePanel'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import PageHeader from '@/components/shared/PageHeader'
import { usePolling } from '@/hooks/usePolling'
import { relativeTime } from '@/lib/time'
import type { StrmPlaybackLog, StrmPlaybackStats } from '@/types/api'

const PAGE_SIZE = 50

interface PlaybackGroup {
  /** 分组键：同一键的相邻记录合并（同一文件+客户端+结果） */
  key: string
  items: StrmPlaybackLog[]
}

/** 相邻同键记录合并为一组：播放器拖动进度条会对同一文件连续产生多条 302 记录 */
function groupConsecutive(items: StrmPlaybackLog[]): PlaybackGroup[] {
  const groups: PlaybackGroup[] = []
  for (const it of items) {
    const key = `${it.pick_code}|${it.user_agent || ''}|${it.status}`
    const last = groups[groups.length - 1]
    if (last && last.key === key) {
      last.items.push(it)
    } else {
      groups.push({ key, items: [it] })
    }
  }
  return groups
}

/** 识别常见播放器 User-Agent，返回友好名称 */
function parseClient(ua: string): { name: string; short: string } {
  const u = ua.toLowerCase()
  const rules: Array<[RegExp, string]> = [
    [/infuse/, 'Infuse'],
    [/emby\/|emby /, 'Emby'],
    [/jellyfin/, 'Jellyfin'],
    [/plex|plexmediaplayer/, 'Plex'],
    [/vlc/, 'VLC'],
    [/kodi|libreelec/, 'Kodi'],
    [/mpv/, 'mpv'],
    [/iina/, 'IINA'],
    [/nplayer/, 'nPlayer'],
    [/vidhub/, 'VidHub'],
    [/senplayer/, 'Senplayer'],
    [/dlna|dms|bubbleupnp/, 'DLNA'],
    [/ LG-|web0s|netcast/, 'LG TV'],
    [/bravia|sony/, 'Sony TV'],
    [/tizen|samsung/, 'Samsung TV'],
    [/androidtv|atv_|google tv/, 'Android TV'],
    [/applewebkit.*mobile.*safari|iphone|ipad/, 'iOS Safari'],
    [/chrome\/|crios\//, 'Chrome'],
    [/firefox/, 'Firefox'],
    [/safari\//, 'Safari'],
    [/curl/, 'curl'],
  ]
  for (const [re, name] of rules) {
    if (re.test(u)) return { name, short: name }
  }
  // 无匹配时截取首个 token 展示
  const first = ua.split(/[\s/]/)[0] || ''
  return { name: ua, short: first.slice(0, 24) || '未知客户端' }
}

function StatCard({ label, value, icon, hint }: { label: string; value: React.ReactNode; icon?: React.ReactNode; hint?: string }) {
  return (
    <div className="card-lift relative overflow-hidden rounded-2xl border border-border/70 bg-card/50 p-4" title={hint}>
      <div className="pointer-events-none absolute inset-0 bg-ambient opacity-40" />
      <div className="relative flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">
        {icon}
        {label}
      </div>
      <p className="relative mt-1 text-2xl font-bold tabular-nums tracking-tight">{value}</p>
    </div>
  )
}

interface StrmPlaybackPageProps {
  onToast?: (type: 'success' | 'error' | 'warning', title: string, message?: string) => void
}

export default function StrmPlaybackPage(props: StrmPlaybackPageProps) {
  void props
  const [items, setItems] = useState<StrmPlaybackLog[]>([])
  const [stats, setStats] = useState<StrmPlaybackStats | null>(null)
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(0)
  const [loading, setLoading] = useState(true)
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})
  const mountedRef = useRef(true)

  useEffect(() => {
    mountedRef.current = true
    return () => { mountedRef.current = false }
  }, [])

  const fetchData = useCallback(async (showLoading = false) => {
    if (showLoading) setLoading(true)
    try {
      const res = await getStrmPlaybackHistory({ limit: PAGE_SIZE, offset: page * PAGE_SIZE })
      if (mountedRef.current) {
        setItems(res.data?.items || [])
        setStats(res.data?.stats || null)
        setTotal(res.data?.total || 0)
      }
    } catch { /* 轮询失败静默 */ }
    finally {
      if (mountedRef.current) setLoading(false)
    }
  }, [page])

  useEffect(() => {
    fetchData(true)
  }, [fetchData])

  // 低速轮询，播放中新记录自动出现
  usePolling(() => fetchData(), 30000)

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const groups = groupConsecutive(items)

  const renderRow = (l: StrmPlaybackLog) => {
    const client = parseClient(l.user_agent || '')
    const ok = l.status === 302
    return (
      <tr key={l.id} className="border-t border-border/40 bg-muted/25 transition-colors hover:bg-muted/40">
        <td className="whitespace-nowrap px-4 py-2 text-xs text-muted-foreground">
          <span className="ml-7 inline-flex items-center gap-1">
            <CornerDownRight className="size-3 shrink-0 text-primary/50" />
            {relativeTime(l.played_at)}
          </span>
        </td>
        <td className="max-w-[260px] px-4 py-2.5">
          <p className="truncate text-xs font-medium" title={l.pan_path || l.file_name || l.pick_code}>
            {l.file_name || l.pan_path?.split('/').pop() || l.pick_code}
          </p>
          {(l.pan_path || l.local_path) && (
            <p className="truncate text-[11px] text-muted-foreground" title={l.pan_path || ''}>
              {l.pan_path || l.local_path}
            </p>
          )}
        </td>
        <td className="max-w-[180px] px-4 py-2.5">
          <Badge variant="outline" className="border-border/70 bg-muted/50 text-xs">
            <Users className="size-3" />
            {client.short}
          </Badge>
          <p className="mt-0.5 max-w-[180px] truncate text-[10px] text-muted-foreground" title={l.user_agent || ''}>
            {l.user_agent || '-'}
          </p>
        </td>
        <td className="whitespace-nowrap px-4 py-2.5 text-xs tabular-nums">
          {ok ? (
            l.cache_hit ? (
              <span className="inline-flex items-center gap-1 text-muted-foreground" title="直链缓存命中，无 API 调用">
                <Zap className="size-3 text-warning" />缓存
              </span>
            ) : (
              <span className="inline-flex items-center gap-1" title="实时向 115 获取直链耗时">
                <Gauge className="size-3 text-muted-foreground" />
                {l.duration_ms} ms
              </span>
            )
          ) : '-'}
        </td>
        <td className="px-4 py-2.5">
          {ok ? (
            <span className="inline-flex items-center gap-1 text-xs text-success">
              <CheckCircle className="size-3" />302
            </span>
          ) : (
            <span className="inline-flex items-center gap-1 text-xs text-destructive" title={l.error || ''}>
              <AlertCircle className="size-3" />{l.status}{l.error ? ` · ${l.error}` : ''}
            </span>
          )}
        </td>
      </tr>
    )
  }

  if (loading && !stats) {
    return (
      <div className="flex flex-col gap-5">
        <h1 className="text-2xl font-bold leading-tight sm:text-[34px]">播放记录</h1>
        <StatePanel title="正在加载" description="播放记录加载中…" />
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        kicker="Playback Log"
        title="播放记录"
        description="STRM 302 跳转记录：播放内容、客户端、延迟与结果。"
      />

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatCard label="今日播放" icon={<Play className="size-3" />} value={stats?.today ?? 0} />
        <StatCard label="累计播放" icon={<Play className="size-3" />} value={stats?.total ?? 0} />
        <StatCard
          label="直链延迟"
          icon={<Gauge className="size-3" />}
          value={<span className="text-xl">{stats ? `${Math.round(stats.avg_latency_ms)} ms` : '-'}</span>}
          hint="实时向 115 获取直链的平均耗时（缓存命中不计）"
        />
        <StatCard
          label="缓存命中率"
          icon={<Zap className="size-3" />}
          value={stats ? `${Math.round(stats.cache_hit_rate * 100)}%` : '-'}
          hint={`不同客户端 UA 数：${stats?.distinct_clients ?? 0}`}
        />
      </div>

      <div className="overflow-hidden rounded-xl border border-border/60 bg-card/50">
        <table className="w-full text-left text-sm">
          <thead className="bg-muted/40 text-xs text-muted-foreground">
            <tr>
              <th className="px-4 py-2.5 font-medium">时间</th>
              <th className="px-4 py-2.5 font-medium">播放内容</th>
              <th className="px-4 py-2.5 font-medium">客户端</th>
              <th className="px-4 py-2.5 font-medium">延迟</th>
              <th className="px-4 py-2.5 font-medium">结果</th>
            </tr>
          </thead>
          <tbody>
            {items.length === 0 && (
              <tr>
                <td colSpan={5} className="px-4 py-8 text-center text-xs text-muted-foreground">
                  暂无播放记录，播放 STRM 媒体后将在此显示
                </td>
              </tr>
            )}
            {groups.map((g) => {
              const latest = g.items[0]
              const n = g.items.length
              const isGroup = n > 1
              // 以组内最旧一条的 id 作为展开状态键：轮询追加新记录不影响已展开状态
              const groupKey = `g${g.items[g.items.length - 1].id}`
              const open = isGroup && !!expanded[groupKey]
              const client = parseClient(latest.user_agent || '')
              const ok = latest.status === 302
              return (
                <Fragment key={groupKey}>
                  <tr
                    className={`border-t border-border/40 transition-colors ${
                      open
                        ? 'cursor-pointer bg-primary/[0.06] hover:bg-primary/[0.08]'
                        : isGroup
                          ? 'cursor-pointer hover:bg-muted/30'
                          : 'hover:bg-muted/30'
                    }`}
                    onClick={isGroup ? () => setExpanded((prev) => ({ ...prev, [groupKey]: !prev[groupKey] })) : undefined}
                  >
                    <td className="whitespace-nowrap px-4 py-2.5 text-xs text-muted-foreground">
                      <span className="inline-flex items-center gap-1">
                        {isGroup ? (
                          <ChevronDown
                            className={`size-3.5 shrink-0 text-primary transition-transform ${open ? '' : '-rotate-90'}`}
                          />
                        ) : (
                          <span className="size-3.5 shrink-0" />
                        )}
                        <Clock className="size-3" />
                        {relativeTime(latest.played_at)}
                      </span>
                    </td>
                    <td className="max-w-[260px] px-4 py-2.5">
                      <p className="truncate text-xs font-medium" title={latest.pan_path || latest.file_name || latest.pick_code}>
                        {latest.file_name || latest.pan_path?.split('/').pop() || latest.pick_code}
                        {isGroup && (
                          <Badge variant="outline" className="ml-2 border-primary/30 bg-primary/10 text-[10px] text-primary">
                            ×{n}
                          </Badge>
                        )}
                      </p>
                      {(latest.pan_path || latest.local_path) && (
                        <p className="truncate text-[11px] text-muted-foreground" title={latest.pan_path || ''}>
                          {latest.pan_path || latest.local_path}
                        </p>
                      )}
                    </td>
                    <td className="max-w-[180px] px-4 py-2.5">
                      <Badge variant="outline" className="border-border/70 bg-muted/50 text-xs">
                        <Users className="size-3" />
                        {client.short}
                      </Badge>
                      <p className="mt-0.5 max-w-[180px] truncate text-[10px] text-muted-foreground" title={latest.user_agent || ''}>
                        {latest.user_agent || '-'}
                      </p>
                    </td>
                    <td className="whitespace-nowrap px-4 py-2.5 text-xs tabular-nums">
                      {ok ? (
                        (() => {
                          const cacheCount = g.items.filter((i) => i.cache_hit).length
                          const realCount = n - cacheCount
                          if (isGroup && cacheCount > 0 && realCount > 0) {
                            return (
                              <span className="inline-flex items-center gap-1 text-muted-foreground">
                                <Zap className="size-3 text-warning" />
                                缓存 {cacheCount} · 实时 {realCount}
                              </span>
                            )
                          }
                          return latest.cache_hit ? (
                            <span className="inline-flex items-center gap-1 text-muted-foreground" title="直链缓存命中，无 API 调用">
                              <Zap className="size-3 text-warning" />缓存
                            </span>
                          ) : (
                            <span className="inline-flex items-center gap-1" title="实时向 115 获取直链耗时">
                              <Gauge className="size-3 text-muted-foreground" />
                              {latest.duration_ms} ms
                            </span>
                          )
                        })()
                      ) : '-'}
                    </td>
                    <td className="px-4 py-2.5">
                      {ok ? (
                        <span className="inline-flex items-center gap-1 text-xs text-success">
                          <CheckCircle className="size-3" />302
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1 text-xs text-destructive" title={latest.error || ''}>
                          <AlertCircle className="size-3" />{latest.status}{latest.error ? ` · ${latest.error}` : ''}
                        </span>
                      )}
                    </td>
                  </tr>
                  {open && g.items.slice(1).map((l) => renderRow(l))}
                </Fragment>
              )
            })}
          </tbody>
        </table>
      </div>

      {totalPages > 1 && (
        <div className="flex items-center justify-center gap-3 text-xs text-muted-foreground">
          <Button
            variant="outline"
            size="icon-sm"
            className="rounded-full"
            disabled={page === 0}
            onClick={() => setPage((p) => Math.max(0, p - 1))}
          >
            <ChevronLeft className="size-4" />
          </Button>
          <span className="tabular-nums">{page + 1} / {totalPages}</span>
          <Button
            variant="outline"
            size="icon-sm"
            className="rounded-full"
            disabled={page >= totalPages - 1}
            onClick={() => setPage((p) => p + 1)}
          >
            <ChevronRight className="size-4" />
          </Button>
        </div>
      )}
    </div>
  )
}
