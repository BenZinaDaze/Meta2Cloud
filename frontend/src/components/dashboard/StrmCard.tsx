// StrmCard.tsx —— 仪表盘的 STRM 同步卡：文件/映射计数 + 最近运行摘要。
import { FileVideo } from 'lucide-react'
import { SectionCard, RunBadge, LastRunChips } from '@/components/dashboard/primitives'
import { relativeTime } from '@/lib/time'
import type { StrmSyncStatus } from '@/types/api'
import type { StrmRunBrief } from '@/components/dashboard/primitives'

const strmTriggerLabels: Record<string, string> = {
  manual: '手动',
  schedule: '定时',
  ingest: '入库',
  reclassify: '重新归类',
  'title-pin-adopt': '更新译名',
  reidentify: '重新识别',
}

export function StrmCard({ strm, runs, className = '' }: { strm: StrmSyncStatus | null; runs: StrmRunBrief[]; className?: string }) {
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
