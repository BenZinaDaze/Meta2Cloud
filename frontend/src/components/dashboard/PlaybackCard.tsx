// PlaybackCard.tsx —— 仪表盘的播放记录卡：今日/均耗/命中统计 + 最近播放列表。
import { Play } from 'lucide-react'
import { SectionCard } from '@/components/dashboard/primitives'
import { relativeTime } from '@/lib/time'
import type { StrmPlaybackStats, StrmPlaybackLog } from '@/types/api'

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

export function PlaybackCard({
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
