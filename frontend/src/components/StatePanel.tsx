import { CloudFog } from 'lucide-react'

interface StatePanelProps {
  icon?: React.ReactNode
  title: string
  description?: string
  tone?: 'neutral' | 'danger'
  compact?: boolean
  action?: React.ReactNode
}

export function StatePanel({
  icon,
  title,
  description,
  tone = 'neutral',
  compact = false,
  action,
}: StatePanelProps) {
  return (
    <div
      className={`relative flex flex-col items-center justify-center overflow-hidden rounded-2xl border border-border/60 px-5 text-center ${
        compact ? 'py-10' : 'py-16 sm:py-20'
      } ${tone === 'danger' ? 'bg-destructive/[0.06] border-destructive/20' : 'bg-card/40'}`}
    >
      <div className="pointer-events-none absolute inset-0 bg-grid-fade opacity-60" />
      <div
        className={`relative mb-4 flex items-center justify-center rounded-2xl border ${
          compact ? 'size-14' : 'size-16'
        } ${
          tone === 'danger'
            ? 'border-destructive/25 bg-destructive/10 text-destructive'
            : 'border-border/80 bg-muted/60 text-muted-foreground'
        }`}
      >
        {icon ?? <CloudFog className={compact ? 'size-6' : 'size-7'} />}
      </div>
      <p className={`relative font-semibold ${compact ? 'text-base' : 'text-lg'}`}>{title}</p>
      {description && (
        <p className="relative mt-2 max-w-md text-sm leading-6 text-muted-foreground">{description}</p>
      )}
      {action && <div className="relative mt-5">{action}</div>}
    </div>
  )
}

export function SkeletonPanel({ rows = 3, compact = false }: { rows?: number; compact?: boolean }) {
  return (
    <div className={`rounded-2xl border border-border/60 bg-card/40 p-5 ${compact ? 'py-8' : 'py-10'}`}>
      <div className="space-y-4">
        {Array.from({ length: rows }).map((_, index) => (
          <div
            key={index}
            className="h-5 animate-pulse rounded-full bg-muted/80"
            style={{ width: `${72 - index * 10}%` }}
          />
        ))}
      </div>
    </div>
  )
}
