// WatchedIgnoreTab.tsx —— 忽略列表 tab：反复失败的候选（一键忽略）+ 已忽略媒体（可恢复）。
// 忽略是媒体级的——整部电影/整部剧两侧均不再参与同步，恢复后下轮自动纳入。
import { useState, useEffect, useCallback } from 'react'
import { Loader2, AlertCircle, Ban, Undo2 } from 'lucide-react'
import { toast } from 'sonner'
import { getWatchedSyncIgnores, setWatchedSyncIgnore } from '@/api'
import { Button } from '@/components/ui/button'
import { relativeTime } from '@/lib/time'
import { DirectionIcon } from '@/components/watched/WatchedRunItems'
import type { WatchedSyncIgnore, WatchedFailingMedia } from '@/types/api'

export function WatchedIgnoreTab() {
  const [data, setData] = useState<{ ignores: WatchedSyncIgnore[]; candidates: WatchedFailingMedia[] } | null>(null)
  const [busy, setBusy] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      const res = await getWatchedSyncIgnores()
      setData({ ignores: res.data?.ignores || [], candidates: res.data?.candidates || [] })
    } catch { /* 静默，下次操作后刷新 */ }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const act = async (action: 'add' | 'remove', mediaType: string, tmdbId: number, reason?: string) => {
    const key = `${mediaType}:${tmdbId}`
    setBusy(key)
    try {
      const res = await setWatchedSyncIgnore({ action, media_type: mediaType, tmdb_id: tmdbId, reason })
      toast.success(action === 'add' ? '已忽略' : '已恢复', { description: res.data?.message })
      await load()
    } catch (e) {
      const detail = (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail
      toast.error('操作失败', { description: detail || (e as Error).message })
    } finally {
      setBusy(null)
    }
  }

  if (!data) return <p className="py-6 text-center text-xs text-muted-foreground">加载中…</p>

  return (
    <div className="space-y-6">
      {/* 候选区：近 7 天失败 ≥2 次且未忽略的媒体 */}
      <div>
        <div className="mb-2 flex items-center gap-2 text-[13px] font-medium">
          <AlertCircle className="size-3.5 text-warning" />
          反复同步失败的媒体
          <span className="text-[11px] font-normal text-muted-foreground">近 7 天失败 ≥2 次；持续失败通常为 Trakt 缺少该媒体的 TMDB 映射，可选择忽略</span>
        </div>
        {data.candidates.length === 0 ? (
          <p className="rounded-lg border border-border/50 px-4 py-5 text-center text-xs text-muted-foreground">
            没有反复失败的媒体
          </p>
        ) : (
          <div className="overflow-hidden rounded-xl border border-border/60">
            {data.candidates.map((c) => {
              const key = `${c.media_type}:${c.tmdb_id}`
              return (
                <div key={key} className="flex items-center gap-3 border-b border-border/40 px-4 py-2.5 last:border-b-0">
                  <DirectionIcon mediaType={c.media_type} />
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm">{c.title || `TMDB ${c.tmdb_id}`}</div>
                    <div className="truncate text-[11px] text-muted-foreground" title={c.last_error || ''}>
                      失败 {c.fails} 次 · {c.last_error || '未知原因'} · 最近 {relativeTime(c.last_at)}
                    </div>
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={busy === key}
                    onClick={() => act('add', c.media_type, c.tmdb_id, c.last_error || '')}
                    className="shrink-0 rounded-full"
                  >
                    {busy === key ? <Loader2 className="size-3.5 animate-spin" /> : <Ban className="size-3.5" />}
                    忽略
                  </Button>
                </div>
              )
            })}
          </div>
        )}
      </div>

      {/* 已忽略区 */}
      <div>
        <div className="mb-2 flex items-center gap-2 text-[13px] font-medium">
          <Ban className="size-3.5 text-muted-foreground" />
          已忽略的媒体
          <span className="text-[11px] font-normal text-muted-foreground">两侧均不参与同步；恢复后下轮同步自动纳入</span>
        </div>
        {data.ignores.length === 0 ? (
          <p className="rounded-lg border border-border/50 px-4 py-5 text-center text-xs text-muted-foreground">
            暂无忽略的媒体
          </p>
        ) : (
          <div className="overflow-hidden rounded-xl border border-border/60">
            {data.ignores.map((it) => {
              const key = `${it.media_type}:${it.tmdb_id}`
              return (
                <div key={key} className="flex items-center gap-3 border-b border-border/40 px-4 py-2.5 last:border-b-0">
                  <DirectionIcon mediaType={it.media_type} />
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm">{it.title || `TMDB ${it.tmdb_id}`}</div>
                    <div className="truncate text-[11px] text-muted-foreground" title={it.reason || ''}>
                      {it.reason ? `${it.reason} · ` : ''}忽略于 {relativeTime(it.created_at)}
                    </div>
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={busy === key}
                    onClick={() => act('remove', it.media_type, it.tmdb_id)}
                    className="shrink-0 rounded-full"
                  >
                    {busy === key ? <Loader2 className="size-3.5 animate-spin" /> : <Undo2 className="size-3.5" />}
                    恢复
                  </Button>
                </div>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}
