import { useState, useEffect, useCallback, useRef } from 'react'
import { useNavigate } from 'react-router'
import {
  RefreshCw,
  Film,
  Tv,
  Inbox,
  Cloud,
  CheckCircle,
  HelpCircle,
  XCircle,
} from 'lucide-react'
import {
  getStats,
  getIngestStats,
  getIngestDailyStats,
  getIngestRecent,
  getPipelineStatus,
  getStrmPlaybackHistory,
  getStrmSyncStatus,
  getU115Space,
  getWatchedSyncStatus,
  listStrmSyncRuns,
} from '@/api'
import { StatePanel } from '@/components/StatePanel'
import { Button } from '@/components/ui/button'
import PageHeader from '@/components/shared/PageHeader'
import { relativeTime } from '@/lib/time'
import { usePolling } from '@/hooks/usePolling'
import { IngestTrendChart, fillDays } from '@/components/dashboard/IngestTrendChart'
import {
  KpiCard,
  SectionCard,
  normalizeRuns,
} from '@/components/dashboard/primitives'
import { StrmCard } from '@/components/dashboard/StrmCard'
import { WatchedCard } from '@/components/dashboard/WatchedCard'
import { PlaybackCard } from '@/components/dashboard/PlaybackCard'
import { formatBytes, statusBadge } from '@/components/dashboard/format'
import type {
  LibraryStats,
  IngestStats,
  DayStat,
  RecentIngest,
  StrmSyncStatus,
  WatchedSyncStatus,
  StrmPlaybackStats,
  StrmPlaybackLog,
} from '@/types/api'
import type { StrmRunBrief } from '@/components/dashboard/primitives'

export default function DashboardPage() {
  const navigate = useNavigate()
  const [loading, setLoading] = useState(true)
  const [libStats, setLibStats] = useState<LibraryStats | null>(null)
  const [ingest7, setIngest7] = useState<IngestStats | null>(null)
  const [ingest14Total, setIngest14Total] = useState<number | null>(null)
  const [daily, setDaily] = useState<DayStat[]>([])
  const [recent, setRecent] = useState<RecentIngest[]>([])
  const [strm, setStrm] = useState<StrmSyncStatus | null>(null)
  const [strmRuns, setStrmRuns] = useState<StrmRunBrief[]>([])
  const [u115Space, setU115Space] = useState<{ total: number; remain: number } | null>(null)
  const [playback, setPlayback] = useState<{ stats: StrmPlaybackStats | null; items: StrmPlaybackLog[] } | null>(null)
  const [pipeline, setPipeline] = useState<{ running: boolean; debounce: boolean } | null>(null)
  const [watched, setWatched] = useState<WatchedSyncStatus | null>(null)
  const pipelineRef = useRef(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [
        statsR, ingestR, ingest14R, dailyR, recentR, strmR, runsR, playbackR,
      ] = await Promise.allSettled([
        getStats(),
        getIngestStats(7),
        getIngestStats(14),
        getIngestDailyStats(30),
        getIngestRecent(8),
        getStrmSyncStatus(),
        listStrmSyncRuns({ limit: 3 }),
        getStrmPlaybackHistory({ limit: 8 }),
      ])
      if (statsR.status === 'fulfilled' && statsR.value.data) setLibStats(statsR.value.data)
      if (ingestR.status === 'fulfilled' && ingestR.value.data) setIngest7(ingestR.value.data)
      if (ingest14R.status === 'fulfilled' && ingest14R.value.data) {
        setIngest14Total(ingest14R.value.data.total ?? null)
      }
      if (dailyR.status === 'fulfilled') setDaily(fillDays(dailyR.value.data, 30))
      if (recentR.status === 'fulfilled' && Array.isArray(recentR.value.data)) {
        setRecent(recentR.value.data)
      }
      if (strmR.status === 'fulfilled' && strmR.value.data) setStrm(strmR.value.data)
      if (runsR.status === 'fulfilled') setStrmRuns(normalizeRuns(runsR.value.data))
      if (playbackR.status === 'fulfilled' && playbackR.value.data) {
        const d = playbackR.value.data
        setPlayback({ stats: d.stats ?? null, items: Array.isArray(d.items) ? d.items : [] })
      }
      // 115 空间可能因限流/冷却较慢，不阻塞首屏
      getU115Space()
        .then((r) => {
          const d = r.data
          if (typeof d?.total_space === 'number' && typeof d?.remain_space === 'number') {
            setU115Space({ total: d.total_space, remain: d.remain_space })
          }
        })
        .catch(() => setU115Space(null))
      try {
        const watchedR = await getWatchedSyncStatus()
        setWatched(watchedR.data)
      } catch { /* 观看同步未配置时静默 */ }
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  // 整理状态轮询：运行结束（true→false）时自动刷新仪表盘
  usePolling(async () => {
    try {
      const r = await getPipelineStatus()
      const running = !!r.data?.running
      setPipeline(r.data)
      if (pipelineRef.current && !running) load()
      pipelineRef.current = running
    } catch { /* ignore */ }
  }, 5000)

  const dist = ingest7
    ? [
        { key: 'success', label: '成功', count: ingest7.success, className: 'bg-success/70', icon: <CheckCircle className="size-3.5 text-success" /> },
        { key: 'no_tmdb', label: '无 TMDB', count: ingest7.no_tmdb, className: 'bg-warning/70', icon: <HelpCircle className="size-3.5 text-warning" /> },
        { key: 'failed', label: '失败', count: ingest7.failed, className: 'bg-destructive/70', icon: <XCircle className="size-3.5 text-destructive" /> },
      ]
    : []
  const distTotal = dist.reduce((s, d) => s + d.count, 0)

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        kicker="Dashboard"
        title="仪表盘"
        description="媒体库规模、入库动态与同步服务运行状态一览。"
        actions={
          <div className="flex items-center gap-2">
            {pipeline && (
              <span className="flex h-10 items-center gap-2 rounded-full border border-border/60 bg-card/50 px-4 text-xs font-medium text-muted-foreground">
                {pipeline.running ? (
                  <>
                    <span className="size-1.5 animate-tide-breathe rounded-full bg-primary" />
                    <span className="text-foreground">整理中</span>
                  </>
                ) : pipeline.debounce ? (
                  <>
                    <span className="size-1.5 animate-tide-breathe rounded-full bg-warning" />
                    <span className="text-foreground">待触发</span>
                  </>
                ) : (
                  <>
                    <span className="size-1.5 rounded-full bg-muted-foreground/40" />
                    整理空闲
                  </>
                )}
              </span>
            )}
            <Button
              variant="outline"
              size="icon"
              onClick={load}
              disabled={loading}
              title="刷新"
              className="size-10 shrink-0 rounded-full"
            >
              <RefreshCw className={`size-4 ${loading ? 'animate-spin' : ''}`} />
            </Button>
          </div>
        }
      />

      {loading ? (
        <StatePanel title="正在加载" description="仪表盘数据加载中…" />
      ) : (
        <>
          {/* 库存规模 */}
          <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
            <KpiCard
              icon={<Film className="size-3.5" />}
              label="电影"
              value={libStats?.total_movies ?? 0}
              sub="媒体库在库电影"
            />
            <KpiCard
              icon={<Tv className="size-3.5" />}
              label="剧集"
              value={libStats?.total_tv_shows ?? 0}
              sub={<>已入库 {libStats?.total_episodes_in_library ?? 0} 集</>}
            />
            <KpiCard
              icon={<Inbox className="size-3.5" />}
              label="近 7 天入库"
              value={ingest7?.total ?? 0}
              sub={<>电影 {ingest7?.movies ?? 0} · 集 {ingest7?.tv_episodes ?? 0}</>}
            />
            <KpiCard
              icon={<Cloud className="size-3.5" />}
              label="115 剩余空间"
              value={u115Space ? formatBytes(u115Space.remain) : '—'}
              meter={
                u115Space && u115Space.total > 0
                  ? ((u115Space.total - u115Space.remain) / u115Space.total) * 100
                  : undefined
              }
              sub={
                u115Space
                  ? <>共 {formatBytes(u115Space.total)} · 已用 {(((u115Space.total - u115Space.remain) / u115Space.total) * 100).toFixed(0)}%</>
                  : '115 未授权或暂不可用'
              }
            />
          </div>

          {/* 入库动态：趋势 + 结果分布 */}
          <div className="grid gap-3 xl:grid-cols-3">
            <SectionCard title="入库趋势" extra={<span className="text-xs text-muted-foreground">近 30 天</span>} className="xl:col-span-2">
              <IngestTrendChart data={daily} />
            </SectionCard>

            <SectionCard title="近 7 天入库结果">
              {distTotal === 0 ? (
                <p className="py-6 text-center text-sm text-muted-foreground">近 7 天暂无入库记录</p>
              ) : (
                <>
                  <div className="flex h-3 gap-[2px] overflow-hidden rounded-full">
                    {dist.map(
                      (d) =>
                        d.count > 0 && (
                          <div
                            key={d.key}
                            className={d.className}
                            style={{ flexGrow: d.count, flexBasis: 0 }}
                            title={`${d.label} ${d.count}`}
                          />
                        ),
                    )}
                  </div>
                  {dist.map((d) => (
                    <div key={d.key} className="flex items-center justify-between gap-2 text-sm">
                      <span className="flex items-center gap-2 text-muted-foreground">
                        {d.icon}
                        {d.label}
                      </span>
                      <span className="font-medium tabular-nums text-foreground">
                        {d.count}
                        <span className="ml-1.5 text-xs text-muted-foreground">
                          {((d.count / distTotal) * 100).toFixed(0)}%
                        </span>
                      </span>
                    </div>
                  ))}
                  {ingest14Total !== null && (
                    <div className="flex items-center justify-between border-t border-border/40 pt-2 text-xs text-muted-foreground">
                      <span>日均 {(ingest7 ? ingest7.total / 7 : 0).toFixed(1)} 项</span>
                      {(() => {
                        const delta = ingest7 ? ingest14Total - ingest7.total : 0
                        const text = delta > 0 ? `+${delta}` : delta === 0 ? '持平' : String(delta)
                        return (
                          <span>
                            较上周 <span className={delta > 0 ? 'font-medium text-primary' : 'font-medium text-foreground'}>{text}</span>
                          </span>
                        )
                      })()}
                    </div>
                  )}
                </>
              )}
            </SectionCard>
          </div>

          {/* 同步服务：左列 STRM/观看同步堆叠，右列播放记录贯通（高度跟随内容，不再拉稀疏卡） */}
          <div className="grid gap-3 md:grid-cols-2">
            <StrmCard strm={strm} runs={strmRuns} className="xl:col-start-1 xl:row-start-1" />
            <WatchedCard watched={watched} className="xl:col-start-1 xl:row-start-2" />
            <PlaybackCard
              pb={playback}
              strmEnabled={!!strm?.enabled}
              className="md:col-span-2 xl:col-span-1 xl:col-start-2 xl:row-start-1 xl:row-end-3"
            />
          </div>

          {/* 最近入库 */}
          <SectionCard
            title="最近入库"
            extra={
              <button
                onClick={() => navigate('/ingest-history')}
                className="text-xs text-muted-foreground transition-colors hover:text-foreground"
              >
                查看全部 →
              </button>
            }
          >
            {recent.length === 0 ? (
              <p className="py-6 text-center text-sm text-muted-foreground">暂无入库记录</p>
            ) : (
              <div className="flex flex-col gap-1">
                {recent.map((item, i) => {
                  const isTV = item.media_type === 'tv'
                  const badge = statusBadge[item.status]
                  const clickable = item.tmdb_id > 0
                  return (
                    <div
                      key={`${item.ingested_at}-${i}`}
                      className={`flex items-center gap-3 rounded-xl border border-transparent p-2 transition-colors ${
                        clickable ? 'cursor-pointer hover:border-border/60 hover:bg-card/60' : ''
                      }`}
                      onClick={
                        clickable
                          ? () => navigate(`${isTV ? '/tv' : '/movies'}/${item.tmdb_id}`)
                          : undefined
                      }
                    >
                      <div className="h-16 w-11 shrink-0 overflow-hidden rounded-md border border-border/60 bg-muted">
                        {item.poster_url ? (
                          <img src={item.poster_url} alt="" className="h-full w-full object-cover" loading="lazy" />
                        ) : (
                          <div className="flex h-full w-full items-center justify-center text-muted-foreground">
                            {isTV ? <Tv className="size-4" /> : <Film className="size-4" />}
                          </div>
                        )}
                      </div>
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-2">
                          <span className="min-w-0 truncate text-sm font-medium">{item.title}</span>
                          {item.year && <span className="shrink-0 text-xs text-muted-foreground">{item.year}</span>}
                          {isTV && item.season !== null && (
                            <span className="shrink-0 rounded-md bg-muted px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
                              S{String(item.season).padStart(2, '0')}
                              {item.episode !== null ? `E${String(item.episode).padStart(2, '0')}` : ''}
                            </span>
                          )}
                        </div>
                        {item.episode_title && (
                          <p className="mt-0.5 truncate text-xs text-muted-foreground">{item.episode_title}</p>
                        )}
                      </div>
                      <span className="shrink-0 text-xs text-muted-foreground">{relativeTime(item.ingested_at)}</span>
                      {badge && (
                        <span className={`shrink-0 rounded-full border px-2 py-0.5 text-[10px] ${badge.className}`}>
                          {badge.label}
                        </span>
                      )}
                    </div>
                  )
                })}
              </div>
            )}
          </SectionCard>
        </>
      )}
    </div>
  )
}
