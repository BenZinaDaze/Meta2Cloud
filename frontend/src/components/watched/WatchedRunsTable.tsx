// WatchedRunsTable.tsx —— 观看同步运行记录表：
// 实时推送（sse/poll）相邻同媒体同结果的运行折叠为一组，单条运行可展开逐条明细。
import { Fragment } from 'react'
import { ChevronDown, Loader2, AlertCircle, CheckCircle } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { relativeTime } from '@/lib/time'
import { RunItems } from '@/components/watched/WatchedRunItems'
import type { WatchedSyncRun } from '@/types/api'

export const TRIGGER_LABELS: Record<string, string> = {
  manual: '手动',
  schedule: '定时',
  activity: 'Trakt 活动',
  sse: '实时推送',
  poll: '会话轮询',
}

/** 实时推送（sse/poll）相邻同媒体同结果的运行合并为一组：
 * 推送失败按退避每小时重试，同一媒体会连续产生多条同结果记录，折叠后只占一行 */
interface RunGroup {
  key: string
  runs: WatchedSyncRun[]
}

const SESSION_TRIGGERS = new Set(['sse', 'poll'])

function groupConsecutiveRuns(runs: WatchedSyncRun[]): RunGroup[] {
  const groups: RunGroup[] = []
  for (const run of runs) {
    // 进行中的运行永不合并；全量同步（manual/schedule）一轮就是一条，也不合并
    const key = SESSION_TRIGGERS.has(run.trigger) && run.title && run.finished_at != null
      ? `session|${run.title}|${run.error ? 'fail' : 'ok'}`
      : `run|${run.id}`
    const last = groups[groups.length - 1]
    if (last && last.key === key) {
      last.runs.push(run)
    } else {
      groups.push({ key, runs: [run] })
    }
  }
  return groups
}

/** 单条运行的「状态」列内容（普通行与折叠组行共用） */
function RunStatusCell({ run }: { run: WatchedSyncRun }) {
  if (run.finished_at == null) {
    return (
      <span className="inline-flex items-center gap-1 text-xs text-primary">
        <Loader2 className="size-3 animate-spin" />进行中
      </span>
    )
  }
  if (run.error) {
    return (
      <span className="inline-flex max-w-[200px] items-center gap-1 text-xs text-destructive" title={run.error || ''}>
        <AlertCircle className="size-3 shrink-0" />
        <span className="truncate">失败：{run.error}</span>
      </span>
    )
  }
  return (
    <span className="inline-flex items-center gap-1 text-xs text-success">
      <CheckCircle className="size-3" />完成
    </span>
  )
}

interface WatchedRunsTableProps {
  runs: WatchedSyncRun[]
  expandedRun: number | null
  expandedGroups: Record<string, boolean>
  onToggleRun: (id: number) => void
  onToggleGroup: (key: string) => void
  onToast?: (type: 'success' | 'error', title: string, message?: string) => void
}

export function WatchedRunsTable({ runs, expandedRun, expandedGroups, onToggleRun, onToggleGroup, onToast }: WatchedRunsTableProps) {
  const runGroups = groupConsecutiveRuns(runs)

  return (
    <div className="overflow-hidden rounded-xl border border-border/60 bg-card/50">
      <table className="w-full text-left text-sm">
        <thead className="bg-muted/40 text-xs text-muted-foreground">
          <tr>
            <th className="w-[130px] px-4 py-2.5 font-medium">时间</th>
            <th className="w-[128px] px-4 py-2.5 font-medium">触发</th>
            <th className="hidden max-w-[380px] px-3 py-2.5 font-medium md:table-cell">内容</th>
            <th className="w-[68px] px-3 py-2.5 text-right font-medium">→Trakt</th>
            <th className="w-[68px] px-3 py-2.5 text-right font-medium">→Plex</th>
            <th className="w-[68px] px-3 py-2.5 text-right font-medium">冲突</th>
            <th className="w-[72px] px-3 py-2.5 text-right font-medium">未识别</th>
            <th className="w-[220px] px-4 py-2.5 font-medium">状态</th>
            <th className="w-8 px-2 py-2.5" />
          </tr>
        </thead>
        <tbody>
          {runs.length === 0 && (
            <tr>
              <td colSpan={9} className="px-4 py-8 text-center text-xs text-muted-foreground">
                暂无同步记录——在「配置 → 观看同步」完成 Trakt 与 Plex 配对后，点击「立即同步」即可开始双向同步观看状态
              </td>
            </tr>
          )}
          {runGroups.map((group) => {
            const latest = group.runs[0] // 接口按时间倒序，首条即最近一次
            const n = group.runs.length
            const isGroup = n > 1
            // 以组内最旧一条的 id 作为展开状态键：轮询追加新记录不影响已展开状态
            const groupKey = `g${group.runs[group.runs.length - 1].id}`
            const open = isGroup && !!expandedGroups[groupKey]
            const singleExpanded = !isGroup && expandedRun === latest.id
            return (
              <Fragment key={groupKey}>
                <tr
                  className={`border-t border-border/40 transition-colors ${
                    open
                      ? 'cursor-pointer bg-primary/[0.06] hover:bg-primary/[0.08]'
                      : 'cursor-pointer hover:bg-muted/30'
                  }`}
                  onClick={
                    isGroup
                      ? () => onToggleGroup(groupKey)
                      : () => onToggleRun(latest.id)
                  }
                >
                  <td className="whitespace-nowrap px-4 py-2.5 text-xs">
                    <span className="inline-flex items-center gap-1">
                      {isGroup && (
                        <ChevronDown
                          className={`size-3.5 shrink-0 text-primary transition-transform ${open ? '' : '-rotate-90'}`}
                        />
                      )}
                      {relativeTime(latest.started_at)}
                    </span>
                  </td>
                  <td className="px-4 py-2.5">
                    <Badge variant="outline" className="whitespace-nowrap border-border/70 bg-muted/50 text-xs">
                      {TRIGGER_LABELS[latest.trigger] || latest.trigger}
                    </Badge>
                  </td>
                  <td className="hidden max-w-[380px] px-3 py-2.5 text-xs md:table-cell" title={latest.title ?? ''}>
                    <span className="flex items-center gap-2">
                      <span className="truncate">
                        {latest.title ? `${latest.title} · ${Math.round((latest.progress ?? 0) * 100)}%` : '—'}
                      </span>
                      {isGroup && (
                        <Badge variant="outline" className="shrink-0 border-primary/30 bg-primary/10 text-[10px] text-primary">
                          ×{n}
                        </Badge>
                      )}
                    </span>
                  </td>
                  <td className="px-4 py-2.5 text-right tabular-nums">{latest.to_trakt || '-'}</td>
                  <td className="px-4 py-2.5 text-right tabular-nums">{latest.to_plex || '-'}</td>
                  <td className="px-4 py-2.5 text-right tabular-nums">{latest.conflicts || '-'}</td>
                  <td className="px-4 py-2.5 text-right tabular-nums text-muted-foreground">{latest.unresolved || '-'}</td>
                  <td className="px-4 py-2.5">
                    <RunStatusCell run={latest} />
                  </td>
                  <td className="w-8 px-2 py-2.5" />
                </tr>
                {open && group.runs.slice(1).map((r) => (
                  <tr key={r.id} className="border-t border-border/40 hover:bg-muted/30">
                    <td className="px-4 py-2.5 text-xs text-muted-foreground">{relativeTime(r.started_at)}</td>
                    <td className="px-4 py-2.5">
                      <Badge variant="outline" className="whitespace-nowrap border-border/70 bg-muted/50 text-xs">
                        {TRIGGER_LABELS[r.trigger] || r.trigger}
                      </Badge>
                    </td>
                    <td className="hidden max-w-[380px] truncate px-3 py-2.5 text-xs text-muted-foreground md:table-cell">
                      {r.title ? `${r.title} · ${Math.round((r.progress ?? 0) * 100)}%` : '—'}
                    </td>
                    <td className="px-4 py-2.5 text-right tabular-nums text-muted-foreground">{r.to_trakt || '-'}</td>
                    <td className="px-4 py-2.5 text-right tabular-nums text-muted-foreground">{r.to_plex || '-'}</td>
                    <td className="px-4 py-2.5 text-right tabular-nums text-muted-foreground">{r.conflicts || '-'}</td>
                    <td className="px-4 py-2.5 text-right tabular-nums text-muted-foreground">{r.unresolved || '-'}</td>
                    <td className="px-4 py-2.5">
                      <RunStatusCell run={r} />
                    </td>
                    <td className="w-8 px-2 py-2.5" />
                  </tr>
                ))}
                {singleExpanded && (
                  <tr className="border-t border-border/40 bg-muted/20">
                    <td colSpan={9} className="px-4 py-3">
                      <RunItems runId={latest.id} running={latest.finished_at == null} onToast={onToast} />
                    </td>
                  </tr>
                )}
              </Fragment>
              )
            })}
          </tbody>
        </table>
      </div>
  )
}
