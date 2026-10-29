import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import {
  Activity, CheckCircle, Cloud, FileVideo, Film, HelpCircle, Inbox, Play,
  RefreshCw, Tv, XCircle,
} from 'lucide-react'
import {
  getIngestDailyStats, getIngestRecent, getIngestStats, getPipelineStatus,
  getStats, getStrmPlaybackHistory, getStrmSyncStatus, getU115Space,
  getWatchedSyncStatus, listStrmSyncRuns,
} from '@/api'
import { StatePanel } from '@/components/StatePanel'
import { Button } from '@/components/ui/button'
import PageHeader from '@/components/shared/PageHeader'
import { relativeTime } from '@/lib/time'
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

interface StrmRunBrief {
  id: number
  trigger: string
  created: number
  updated: number
  removed: number
  finished_at: string | null
}

const statusBadge: Record<string, { label: string; className: string }> = {
  success: { label: '成功', className: 'border-success/30 bg-success/10 text-success' },
  failed: { label: '失败', className: 'border-destructive/30 bg-destructive/10 text-destructive' },
  no_tmdb: { label: '无 TMDB', className: 'border-warning/30 bg-warning/10 text-warning' },
}

const strmTriggerLabels: Record<string, string> = {
  manual: '手动', schedule: '定时', ingest: '入库',
  reclassify: '重新归类', 'title-pin-adopt': '更新译名', reidentify: '重新识别',
}

const watchedTriggerLabels: Record<string, string> = {
  manual: '手动', schedule: '定时', activity: 'Trakt 活动', sse: '实时推送', poll: '会话轮询',
}

/** 识别常见播放器 User-Agent，返回友好名称（与播放记录页同规则） */
function parseClient(ua: string): string {
  const u = ua.toLowerCase()
  const rules: Array<[RegExp, string]> = [
    [/infuse/, 'Infuse'], [/emby\/|emby /, 'Emby'], [/jellyfin/, 'Jellyfin'],
    [/plex|plexmediaplayer/, 'Plex'], [/vlc/, 'VLC'], [/kodi|libreelec/, 'Kodi'],
    [/mpv/, 'mpv'], [/iina/, 'IINA'], [/nplayer/, 'nPlayer'], [/vidhub/, 'VidHub'],
    [/senplayer/, 'Senplayer'], [/dlna|dms|bubbleupnp/, 'DLNA'],
    [/ LG-|web0s|netcast/, 'LG TV'], [/bravia|sony/, 'Sony TV'],
    [/tizen|samsung/, 'Samsung TV'], [/androidtv|atv_|google tv/, 'Android TV'],
    [/applewebkit.*mobile.*safari|iphone|ipad/, 'iOS Safari'],
    [/chrome\/|crios\//, 'Chrome'], [/firefox/, 'Firefox'], [/safari\//, 'Safari'],
    [/curl/, 'curl'],
  ]
  for (const [re, name] of rules) {
    if (re.test(u)) return name
  }
  const first = ua.split(/[\s/]/)[0] || ''
  return first.slice(0, 24) || '未知客户端'
}

function compact(n: number): string {
  if (n >= 1000) return `${(n / 1000).toFixed(n >= 10000 ? 0 : 1)}K`
  return String(n)
}

function formatBytes(n: number): string {
  if (!n) return '0'
  const tb = n / 1024 ** 4
  if (tb >= 1) return `${tb.toFixed(tb >= 100 ? 0 : 1)} TB`
  const gb = n / 1024 ** 3
  if (gb >= 1) return `${gb.toFixed(gb >= 100 ? 0 : 1)} GB`
  const mb = n / 1024 ** 2
  return `${mb.toFixed(0)} MB`
}

// ── 趋势图（手写 SVG 柱状图） ────────────────────────────────

function useContainerSize<T extends HTMLElement>() {
  const ref = useRef<T | null>(null)
  const [size, setSize] = useState({ w: 0, h: 0 })
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const ro = new ResizeObserver((entries) => {
      for (const e of entries) setSize({ w: e.contentRect.width, h: e.contentRect.height })
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  return [ref, size] as const
}

// 补齐近 days 天（后端只返回有记录的日期，UTC 对齐）
// 旧版后端对未知 API 会兜底返回 HTML（200），必须校验数组形状
function fillDays(raw: unknown, days: number): DayStat[] {
  if (!Array.isArray(raw)) return []
  const map = new Map(
    raw
      .filter((d): d is { date: string; total: number; movies: number } => !!d && typeof d === 'object' && typeof (d as { date?: unknown }).date === 'string')
      .map((d) => [d.date, { total: Number(d.total) || 0, movies: Number(d.movies) || 0 } as const]),
  )
  const out: DayStat[] = []
  const now = new Date()
  for (let i = days - 1; i >= 0; i--) {
    const d = new Date(now)
    d.setUTCDate(d.getUTCDate() - i)
    const key = d.toISOString().slice(0, 10)
    const v = map.get(key)
    out.push({ date: key, total: v?.total ?? 0, movies: v?.movies ?? 0 })
  }
  return out
}

function niceCeil(v: number): number {
  if (v <= 4) return 4
  const pow = 10 ** Math.floor(Math.log10(v))
  for (const m of [1, 2, 2.5, 5, 10]) {
    if (m * pow >= v) return m * pow
  }
  return 10 * pow
}

// 归一化同步 runs 响应（旧后端未知路由会返回 HTML，须校验）
function normalizeRuns(raw: unknown): StrmRunBrief[] {
  let items: unknown[] = []
  if (Array.isArray(raw)) items = raw
  else if (raw && typeof raw === 'object' && Array.isArray((raw as { runs?: unknown }).runs)) {
    items = (raw as { runs: unknown[] }).runs
  }
  return items
    .filter((d): d is Record<string, unknown> => !!d && typeof d === 'object')
    .map((d) => ({
      id: Number(d.id) || 0,
      trigger: typeof d.trigger === 'string' ? d.trigger : '',
      created: Number(d.created) || 0,
      updated: Number(d.updated) || 0,
      removed: Number(d.removed) || 0,
      finished_at: typeof d.finished_at === 'string' ? d.finished_at : null,
    }))
}

// 顶部圆角、基线平头的柱形
function barPath(x: number, y: number, w: number, h: number): string {
  if (h <= 0.5) return ''
  const r = Math.min(4, w / 2, h)
  return (
    `M${x},${y + h}` +
    `L${x},${y + r} Q${x},${y} ${x + r},${y}` +
    `L${x + w - r},${y} Q${x + w},${y} ${x + w},${y + r}` +
    `L${x + w},${y + h} Z`
  )
}

function IngestTrendChart({ data }: { data: DayStat[] }) {
  const [wrapRef, size] = useContainerSize<HTMLDivElement>()
  const [hover, setHover] = useState(-1)
  const width = size.w
  const H = Math.max(200, size.h || 216)
  const pad = { top: 10, right: 6, bottom: 24, left: 34 }

  const total = data.reduce((s, d) => s + d.total, 0)
  const maxVal = Math.max(...data.map((d) => d.total), 1)
  const top = niceCeil(maxVal)
  const innerW = Math.max(0, width - pad.left - pad.right)
  const innerH = H - pad.top - pad.bottom
  const band = data.length > 0 ? innerW / data.length : 0
  const barW = Math.min(24, Math.max(3, band * 0.55))
  const ticks = [0, top / 2, top]
  const labelEvery = Math.max(1, Math.ceil(data.length / 6))
  const hoveredBar = hover >= 0 && hover < data.length ? data[hover] : null

  return (
    <div
      ref={wrapRef}
      className="relative min-h-[216px] flex-1"
      onMouseLeave={() => setHover(-1)}
    >
      {width > 0 && (
        <svg width={width} height={H} role="img" aria-label="近 30 天入库趋势柱状图">
          {ticks.map((t) => {
            const y = pad.top + innerH - (t / top) * innerH
            return (
              <g key={t}>
                <line x1={pad.left} x2={width - pad.right} y1={y} y2={y} stroke="var(--border)" strokeWidth={1} />
                <text x={pad.left - 8} y={y + 3.5} textAnchor="end" fontSize={10} fill="var(--muted-foreground)">
                  {compact(t)}
                </text>
              </g>
            )
          })}
          {data.map((d, i) => {
            const h = (d.total / top) * innerH
            const x = pad.left + i * band + (band - barW) / 2
            const y = pad.top + innerH - h
            return (
              <path
                key={d.date}
                d={barPath(x, y, barW, h)}
                fill="var(--primary)"
                style={hover === i ? { filter: 'brightness(1.25)' } : undefined}
              />
            )
          })}
          {data.map((d, i) =>
            i % labelEvery === 0 ? (
              <text
                key={d.date}
                x={pad.left + i * band + band / 2}
                y={H - 6}
                textAnchor="middle"
                fontSize={10}
                fill="var(--muted-foreground)"
              >
                {d.date.slice(5)}
              </text>
            ) : null,
          )}
          {data.map((d, i) => (
            <rect
              key={d.date}
              x={pad.left + i * band}
              y={0}
              width={band}
              height={H - pad.bottom}
              fill="transparent"
              onPointerEnter={() => setHover(i)}
              onFocus={() => setHover(i)}
              tabIndex={-1}
            />
          ))}
        </svg>
      )}
      {hoveredBar && width > 0 && (
        <div
          className="pointer-events-none absolute z-10 rounded-lg border border-border/70 bg-card/95 px-2.5 py-1.5 text-xs shadow-lg backdrop-blur"
          style={{
            left: Math.min(Math.max(pad.left + hover * band + band / 2 - 44, 4), Math.max(width - 92, 4)),
            top: Math.max(pad.top + innerH - (hoveredBar.total / top) * innerH - 52, 2),
          }}
        >
          <span className="font-semibold text-foreground">{hoveredBar.total} 项</span>
          <span className="ml-1.5 text-muted-foreground">{hoveredBar.date.slice(5)}</span>
        </div>
      )}
      <div className="absolute right-0 top-0 text-xs text-muted-foreground">
        合计 <span className="font-semibold text-foreground">{total}</span> 项
      </div>
    </div>
  )
}

// ── 通用小件 ────────────────────────────────────────────────

function KpiCard({
  icon, label, value, suffix, sub, meter,
}: {
  icon: React.ReactNode
  label: string
  value: React.ReactNode
  suffix?: string
  sub?: React.ReactNode
  meter?: number
}) {
  return (
    <div className="card-lift relative overflow-hidden rounded-2xl border border-border/70 bg-card/50 p-4">
      <div className="pointer-events-none absolute inset-0 bg-ambient opacity-40" />
      <div className="relative flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">
        {icon}
        {label}
      </div>
      <p className="relative mt-1 text-2xl font-bold tracking-tight">
        {value}
        {suffix && <span className="ml-0.5 text-sm font-semibold text-muted-foreground">{suffix}</span>}
      </p>
      {meter !== undefined && (
        <div className="relative mt-2 h-1.5 overflow-hidden rounded-full bg-muted">
          <div
            className="h-full rounded-full bg-primary transition-all duration-500"
            style={{ width: `${Math.min(100, Math.max(0, meter))}%` }}
          />
        </div>
      )}
      {sub && <p className="relative mt-1.5 text-xs text-muted-foreground">{sub}</p>}
    </div>
  )
}

function SectionCard({
  title, extra, children, className = '',
}: {
  title: React.ReactNode
  extra?: React.ReactNode
  children: React.ReactNode
  className?: string
}) {
  return (
    <section className={`flex min-w-0 flex-col overflow-hidden rounded-2xl border border-border/70 bg-card/40 p-4 sm:p-5 ${className}`}>
      <div className="mb-3 flex shrink-0 items-center justify-between gap-2">
        <h2 className="text-sm font-semibold tracking-tight">{title}</h2>
        {extra}
      </div>
      {/* 弹性列 + 均匀分布：剩余空间平均分到各内容块之间，不留整块空白 */}
      <div className="flex min-h-0 flex-1 flex-col justify-between gap-2">
        {children}
      </div>
    </section>
  )
}

// 运行状态徽标：dot + 文案
function RunBadge({ running, idleLabel = '空闲' }: { running: boolean; idleLabel?: string }) {
  return (
    <span className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
      {running ? (
      <span className="size-1.5 animate-tide-breathe rounded-full bg-primary" />
      ) : (
      <span className="size-1.5 rounded-full bg-muted-foreground/40" />
      )}
      {running ? '同步中' : idleLabel}
    </span>
  )
}

// 上次同步的统计 chip 行
function LastRunChips({ items }: { items: { label: string; value: number; className?: string }[] }) {
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {items.map(
        (c) =>
          c.value > 0 && (
            <span
              key={c.label}
              className={`rounded-md border px-1.5 py-0.5 font-mono text-[10px] ${c.className || 'border-border/60 bg-muted/60 text-muted-foreground'}`}
            >
              {c.label} {c.value}
            </span>
          ),
      )}
    </div>
  )
}

// ── STRM 同步卡 ─────────────────────────────────────────────

function StrmCard({ strm, runs, className = '' }: { strm: StrmSyncStatus | null; runs: StrmRunBrief[]; className?: string }) {
  const olderRuns = runs.filter((r) => r.id !== strm?.last_run?.id).slice(0, 2)
  return (
    <SectionCard
      className={className}
      title={
        <span className="flex items-center gap-2">
          <FileVideo className="size-4 text-muted-foreground" />
          STRM 同步
        </span>
      }
      extra={strm?.enabled ? <RunBadge running={strm.running} /> : <span className="text-xs text-muted-foreground">未启用</span>}
    >
      {!strm?.enabled ? (
        <p className="py-4 text-sm text-muted-foreground">
          在「配置 → STRM」开启后，自动生成 INCLUDE STRM 文件，播放器可 302 直连网盘播放。
        </p>
      ) : (
        <>
          <div className="flex items-end gap-6">
            <div>
              <p className="text-2xl font-bold tracking-tight">{strm.total_files}</p>
              <p className="text-xs text-muted-foreground">STRM 文件</p>
            </div>
            <div>
              <p className="text-2xl font-bold tracking-tight">{strm.mappings}</p>
              <p className="text-xs text-muted-foreground">映射目录</p>
            </div>
          </div>
          {strm.last_run ? (
            <>
              <p className="text-xs text-muted-foreground">
                上次同步 · {strmTriggerLabels[strm.last_run.trigger] || strm.last_run.trigger}
                {strm.last_run.finished_at && ` · ${relativeTime(strm.last_run.finished_at)}`}
              </p>
              <LastRunChips
                items={[
                  { label: '新增', value: strm.last_run.created, className: 'border-success/30 bg-success/10 text-success' },
                  { label: '更新', value: strm.last_run.updated, className: 'border-warning/30 bg-warning/10 text-warning' },
                  { label: '删除', value: strm.last_run.removed, className: 'border-destructive/30 bg-destructive/10 text-destructive' },
                  { label: '跳过', value: strm.last_run.skipped },
                ]}
              />
              {strm.last_run.error && (
                <p className="truncate text-xs text-destructive" title={strm.last_run.error}>
                  {strm.last_run.error}
                </p>
              )}
              {olderRuns.length > 0 &&
                olderRuns.map((r) => (
                  <div key={r.id} className="flex items-center gap-2 text-[11px] text-muted-foreground">
                    <span className="w-14 shrink-0">{strmTriggerLabels[r.trigger] || r.trigger}</span>
                    <span className="min-w-0 flex-1 truncate font-mono">
                      +{r.created} ~{r.updated} -{r.removed}
                    </span>
                    <span className="shrink-0">{r.finished_at ? relativeTime(r.finished_at) : ''}</span>
                  </div>
                ))}
            </>
          ) : (
            <p className="text-xs text-muted-foreground">尚未执行过同步</p>
          )}
        </>
      )}
    </SectionCard>
  )
}

// ── 观看同步卡 ──────────────────────────────────────────────

function ConnRow({ label, ok, okText, badText }: { label: string; ok: boolean; okText: string; badText: string }) {
  return (
    <div className="flex items-center justify-between gap-2 py-1 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className={`flex items-center gap-1.5 text-xs font-medium ${ok ? 'text-success' : 'text-muted-foreground'}`}>
        {ok ? <CheckCircle className="size-3.5" /> : <XCircle className="size-3.5" />}
        {ok ? okText : badText}
      </span>
    </div>
  )
}

function WatchedCard({ watched, className = '' }: { watched: WatchedSyncStatus | null; className?: string }) {
  return (
    <SectionCard
      className={className}
      title={
        <span className="flex items-center gap-2">
          <Activity className="size-4 text-muted-foreground" />
          观看同步
        </span>
      }
      extra={watched?.enabled ? <RunBadge running={watched.running} /> : <span className="text-xs text-muted-foreground">未启用</span>}
    >
      {!watched?.enabled ? (
        <p className="py-4 text-sm text-muted-foreground">
          在「配置 → 观看同步」完成 Trakt 与 Plex 配对后，双向同步观看状态。
        </p>
      ) : (
        <>
          <ConnRow label="Trakt" ok={watched.trakt_authorized} okText="已授权" badText="未授权" />
          <ConnRow label="Plex" ok={watched.plex_connected} okText="已连接" badText="未连接" />
          {(watched.sync_to_trakt || watched.sync_to_plex) && (
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="text-xs text-muted-foreground">同步方向</span>
              {watched.sync_to_trakt && (
                <span className="rounded-md border border-primary/30 bg-primary/10 px-1.5 py-0.5 font-mono text-[10px] text-primary">→ Trakt</span>
              )}
              {watched.sync_to_plex && (
                <span className="rounded-md border border-info/30 bg-info/10 px-1.5 py-0.5 font-mono text-[10px] text-info">→ Plex</span>
              )}
            </div>
          )}
          {watched.last_run ? (
            <>
              <p className="text-xs text-muted-foreground">
                上次同步 · {watchedTriggerLabels[watched.last_run.trigger] || watched.last_run.trigger}
                {watched.last_run.finished_at && ` · ${relativeTime(watched.last_run.finished_at)}`}
              </p>
              <LastRunChips
                items={[
                  { label: '→ Trakt', value: watched.last_run.to_trakt, className: 'border-primary/30 bg-primary/10 text-primary' },
                  { label: '→ Plex', value: watched.last_run.to_plex, className: 'border-info/30 bg-info/10 text-info' },
                  { label: '冲突', value: watched.last_run.conflicts, className: 'border-warning/30 bg-warning/10 text-warning' },
                  { label: '跳过', value: watched.last_run.skipped },
                ]}
              />
              {watched.last_run.error && (
                <p className="truncate text-xs text-destructive" title={watched.last_run.error}>
                  {watched.last_run.error}
                </p>
              )}
            </>
          ) : (
            <p className="text-xs text-muted-foreground">尚未执行过同步</p>
          )}
        </>
      )}
    </SectionCard>
  )
}

// ── 播放记录卡 ──────────────────────────────────────────────

function PlaybackCard({
  pb, strmEnabled, className = '',
}: {
  pb: { stats: StrmPlaybackStats | null; items: StrmPlaybackLog[] } | null
  strmEnabled: boolean
  className?: string
}) {
  const stats = pb?.stats
  return (
    <SectionCard
      className={className}
      title={
        <span className="flex items-center gap-2">
          <Play className="size-4 text-muted-foreground" />
          播放记录
        </span>
      }
      extra={stats ? <span className="text-xs text-muted-foreground">累计 {stats.total} 次</span> : null}
    >
      {!stats || stats.total === 0 ? (
        <p className="py-4 text-sm text-muted-foreground">
          {!strmEnabled
            ? 'STRM 未启用，启用并播放后此处展示直链播放动态'
            : '暂无播放记录——播放器直连 302 后会记录每次播放'}
        </p>
      ) : (
        <>
          <div className="grid grid-cols-3 gap-2">
            <div>
              <p className="text-lg font-bold tracking-tight">{stats.today}</p>
              <p className="text-xs text-muted-foreground">今日播放</p>
            </div>
            <div>
              <p className="text-lg font-bold tracking-tight">
                {stats.avg_latency_ms >= 1000 ? `${(stats.avg_latency_ms / 1000).toFixed(1)}s` : `${Math.round(stats.avg_latency_ms)}ms`}
              </p>
              <p className="text-xs text-muted-foreground">直链均耗</p>
            </div>
            <div>
              <p className="text-lg font-bold tracking-tight">{(stats.cache_hit_rate * 100).toFixed(0)}%</p>
              <p className="text-xs text-muted-foreground">缓存命中</p>
            </div>
          </div>
          {pb!.items.slice(0, 8).map((it, i) => {
            const client = parseClient(it.user_agent || '')
            const failed = it.status !== 302
            return (
              <div key={`${it.played_at}-${i}`} className="flex items-center gap-2 text-xs">
                <span className="w-20 shrink-0 truncate rounded-md bg-muted px-1.5 py-0.5 text-center font-medium text-muted-foreground" title={it.user_agent || ''}>
                  {client}
                </span>
                <span className="min-w-0 flex-1 truncate text-foreground" title={it.file_name || ''}>
                  {it.file_name || '-'}
                </span>
                {it.cache_hit ? (
                  <span className="shrink-0 rounded-full border border-info/30 bg-info/10 px-1.5 py-0.5 text-[10px] text-info">缓存</span>
                ) : failed ? (
                  <span
                    className="shrink-0 rounded-full border border-destructive/30 bg-destructive/10 px-1.5 py-0.5 text-[10px] text-destructive"
                    title={it.error || `status ${it.status}`}
                  >
                    失败
                  </span>
                ) : (
                  <span className="shrink-0 text-[10px] text-muted-foreground">
                    {it.duration_ms >= 1000 ? `${(it.duration_ms / 1000).toFixed(1)}s` : `${it.duration_ms}ms`}
                  </span>
                )}
                <span className="w-14 shrink-0 text-right text-[10px] text-muted-foreground">{relativeTime(it.played_at)}</span>
              </div>
            )
          })}
        </>
      )}
    </SectionCard>
  )
}

// ── 页面 ────────────────────────────────────────────────────

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
      if (statsR.status === 'fulfilled' && statsR.value.data) setLibStats(statsR.value.data as LibraryStats)
      if (ingestR.status === 'fulfilled' && ingestR.value.data) setIngest7(ingestR.value.data as IngestStats)
      if (ingest14R.status === 'fulfilled' && ingest14R.value.data) {
        setIngest14Total((ingest14R.value.data as { total?: number }).total ?? null)
      }
      if (dailyR.status === 'fulfilled') setDaily(fillDays(dailyR.value.data, 30))
      if (recentR.status === 'fulfilled' && Array.isArray(recentR.value.data)) {
        setRecent(recentR.value.data as RecentIngest[])
      }
      if (strmR.status === 'fulfilled' && strmR.value.data) setStrm(strmR.value.data)
      if (runsR.status === 'fulfilled') setStrmRuns(normalizeRuns(runsR.value.data))
      if (playbackR.status === 'fulfilled' && playbackR.value.data) {
        const d = playbackR.value.data as { stats?: StrmPlaybackStats; items?: StrmPlaybackLog[] }
        setPlayback({ stats: d.stats ?? null, items: Array.isArray(d.items) ? d.items : [] })
      }
      // 115 空间可能因限流/冷却较慢，不阻塞首屏
      getU115Space()
        .then((r) => {
          const d = r.data as { total_space?: number; remain_space?: number }
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
  useEffect(() => {
    const timer = window.setInterval(async () => {
      try {
        const r = await getPipelineStatus()
        const running = !!r.data?.running
        setPipeline(r.data)
        if (pipelineRef.current && !running) load()
        pipelineRef.current = running
      } catch { /* ignore */ }
    }, 5000)
    return () => window.clearInterval(timer)
  }, [load])

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
