// WatchedCard.tsx —— 仪表盘的观看同步卡：双向连接状态 + 同步方向 + 最近运行摘要。
import { Activity } from 'lucide-react'
import { SectionCard, RunBadge, LastRunChips, ConnRow } from '@/components/dashboard/primitives'
import { relativeTime } from '@/lib/time'
import type { WatchedSyncStatus } from '@/types/api'

const watchedTriggerLabels: Record<string, string> = {
  manual: '手动', schedule: '定时', activity: 'Trakt 活动', sse: '实时推送', poll: '会话轮询',
}

export function WatchedCard({ watched, className = '' }: { watched: WatchedSyncStatus | null; className?: string }) {
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
