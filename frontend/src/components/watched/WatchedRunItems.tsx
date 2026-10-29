// WatchedRunItems.tsx —— 观看同步的单次运行明细表：
// 展开行内自持加载与 2s 轮询（同步进行中「同步一条显示一条」），失败行带快捷忽略。
import { useState, useEffect, useCallback, useRef } from 'react'
import { Loader2, AlertCircle, CheckCircle, Film, Tv, Ban } from 'lucide-react'
import { toast } from 'sonner'
import { getWatchedSyncRunItems, setWatchedSyncIgnore } from '@/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { usePolling } from '@/hooks/usePolling'
import type { WatchedSyncRunItem } from '@/types/api'

export const DIRECTION_LABELS: Record<string, { label: string; className: string }> = {
  to_trakt: { label: '→ Trakt', className: 'border-primary/30 bg-primary/10 text-primary' },
  to_plex: { label: '→ Plex', className: 'border-info/30 bg-info/10 text-info' },
  unresolved: { label: '未识别', className: 'border-warning/30 bg-warning/10 text-warning' },
}

const ACTION_LABELS: Record<string, string> = {
  watched: '标记已看',
  unwatched: '取消已看',
  unresolved: '—',
}

// 侧向图标：→ Trakt 用剧集，→ Plex 用电影，仅作视觉区分
export function DirectionIcon({ mediaType }: { mediaType: string | null }) {
  if (mediaType === 'movie') return <Film className="size-3" />
  if (mediaType === 'tv') return <Tv className="size-3" />
  return null
}

export function itemDisplayName(item: WatchedSyncRunItem): string {
  const title = item.title || (item.tmdb_id ? `TMDB ${item.tmdb_id}` : '未知条目')
  if (item.media_type === 'tv' && item.season != null && item.episode != null) {
    return `${title} S${String(item.season).padStart(2, '0')}E${String(item.episode).padStart(2, '0')}`
  }
  return title
}

// 明细失败行上的快捷忽略按钮（媒体级，加完即生效，下轮同步起排除）。
function IgnoreItemButton({ mediaType, tmdbId, reason }: {
  mediaType: string
  tmdbId: number
  reason: string
}) {
  const [busy, setBusy] = useState(false)
  return (
    <Button
      variant="ghost"
      size="sm"
      disabled={busy}
      onClick={async () => {
        setBusy(true)
        try {
          const res = await setWatchedSyncIgnore({ action: 'add', media_type: mediaType, tmdb_id: tmdbId, reason })
          toast.success('已忽略', { description: res.data?.message || '该媒体将不再参与同步' })
        } catch (e) {
          const detail = (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail
          toast.error('忽略失败', { description: detail || (e as Error).message })
        } finally {
          setBusy(false)
        }
      }}
      className="h-6 shrink-0 gap-1 rounded-full px-2 text-[11px] text-muted-foreground"
      title="忽略这部媒体（两侧不再同步，可在「忽略列表」恢复）"
    >
      {busy ? <Loader2 className="size-3 animate-spin" /> : <Ban className="size-3" />}
      忽略
    </Button>
  )
}

export function RunItems({ runId, running }: {
  runId: number
  running: boolean
}) {
  const [items, setItems] = useState<WatchedSyncRunItem[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  // 序列守卫：runId 切换后丢弃过期响应（轮询与补拉并存时防乱序覆盖）
  const seq = useRef(0)
  const load = useCallback(async () => {
    const id = ++seq.current
    try {
      const res = await getWatchedSyncRunItems(runId, { limit: 1000 })
      if (id !== seq.current) return
      setItems(res.data?.items || [])
    } catch (e) {
      if (id === seq.current) setError((e as Error).message)
    }
  }, [runId])

  useEffect(() => {
    load()
  }, [load, running])

  // 同步进行中每 2 秒刷新，实现「同步一条显示一条」；结束后上面的补拉覆盖完整明细
  usePolling(load, 2000, { enabled: running })

  if (error) return <p className="px-3 py-2 text-xs text-destructive">明细加载失败：{error}</p>
  if (!items) return <p className="px-3 py-2 text-xs text-muted-foreground">加载中…</p>
  if (items.length === 0) {
    return (
      <p className="px-3 py-2 text-xs text-muted-foreground">
        {running ? '正在比对两侧观看状态，明细将逐条显示…' : '无逐条明细（本轮无变更）'}
      </p>
    )
  }

  return (
    <div className="overflow-hidden rounded-lg border border-border/50">
      {running && (
        <div className="flex items-center gap-2 border-b border-border/40 bg-muted/30 px-3 py-1.5 text-xs text-muted-foreground">
          <Loader2 className="size-3 animate-spin text-primary" />
          同步进行中，已处理 {items.length} 条；Trakt 按批次提交，明细按批显示
        </div>
      )}
      <table className="w-full text-left text-xs">
        <thead className="bg-muted/50 text-muted-foreground">
          <tr>
            <th className="px-3 py-2 font-medium">方向</th>
            <th className="px-3 py-2 font-medium">内容</th>
            <th className="px-3 py-2 font-medium">动作</th>
            <th className="px-3 py-2 font-medium">结果</th>
          </tr>
        </thead>
        <tbody>
          {items.map((it) => {
            const dir = DIRECTION_LABELS[it.direction] || { label: it.direction, className: 'border-border bg-muted text-muted-foreground' }
            return (
              <tr key={it.id} className="border-t border-border/40">
                <td className="px-3 py-2">
                  <Badge variant="outline" className={`gap-1 ${dir.className}`}>
                    <DirectionIcon mediaType={it.media_type} />
                    {dir.label}
                  </Badge>
                </td>
                <td className="max-w-[280px] truncate px-3 py-2" title={it.tmdb_id ? `${itemDisplayName(it)}（TMDB ${it.tmdb_id}）` : itemDisplayName(it)}>
                  {itemDisplayName(it)}
                </td>
                <td className="px-3 py-2 text-muted-foreground">{ACTION_LABELS[it.action] || it.action}</td>
                <td className="max-w-[240px] truncate px-3 py-2">
                  {it.success ? (
                    <span className="inline-flex items-center gap-1 text-success"><CheckCircle className="size-3" />成功</span>
                  ) : (
                    <span className="flex items-center gap-2">
                      <span className="inline-flex min-w-0 items-center gap-1 text-destructive" title={it.error || ''}>
                        <AlertCircle className="size-3 shrink-0" />
                        <span className="truncate">{it.error || '失败'}</span>
                      </span>
                      {it.media_type && it.tmdb_id && (
                        <IgnoreItemButton mediaType={it.media_type} tmdbId={it.tmdb_id} reason={it.error || ''} />
                      )}
                    </span>
                  )}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
