// primitives.tsx —— 仪表盘卡片族的共享小件：KPI 卡、分区卡、运行徽标、统计 chips。
import type { ReactNode } from 'react'
import { CheckCircle, XCircle } from 'lucide-react'

/** 归一化后的 STRM 运行摘要（防御性解析，见页面 normalizeRuns） */
export interface StrmRunBrief {
  id: number
  trigger: string
  created: number
  updated: number
  removed: number
  finished_at: string | null
}

// 归一化同步 runs 响应（旧后端未知路由会返回 HTML，须校验）
export function normalizeRuns(raw: unknown): StrmRunBrief[] {
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

export function KpiCard({
  icon, label, value, suffix, sub, meter,
}: {
  icon: ReactNode
  label: string
  value: ReactNode
  suffix?: string
  sub?: ReactNode
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

export function SectionCard({
  title, extra, children, className = '',
}: {
  title: ReactNode
  extra?: ReactNode
  children: ReactNode
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
export function RunBadge({ running, idleLabel = '空闲' }: { running: boolean; idleLabel?: string }) {
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
export function LastRunChips({ items }: { items: { label: string; value: number; className?: string }[] }) {
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

// 连接状态行（观看同步卡用）
export function ConnRow({ label, ok, okText, badText }: { label: string; ok: boolean; okText: string; badText: string }) {
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
