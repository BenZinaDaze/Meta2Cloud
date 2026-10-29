import type { ReactNode } from 'react'

/** 统计卡：页面顶部的小卡（label+icon 行 + 大数字）。hint 悬停补充说明。
 * 此前 5 个页面各自定义同构实现，收敛于此。 */
export default function StatCard({ label, value, icon, hint }: {
  label: string
  value: ReactNode
  icon?: ReactNode
  hint?: string
}) {
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
