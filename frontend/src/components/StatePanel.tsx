import { AlertCircle } from 'lucide-react'

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
      className={`flex flex-col items-center justify-center rounded-2xl px-5 text-center ${
        compact ? 'py-10' : 'py-16 sm:py-24'
      } ${tone === 'danger' ? 'bg-destructive/10' : 'bg-muted/50'}`}
    >
      {icon ? (
        <div
          className={`mb-4 flex items-center justify-center rounded-full bg-muted ${
            compact ? 'size-14 text-3xl' : 'size-20 text-5xl'
          } ${tone === 'danger' ? 'text-destructive' : 'text-muted-foreground'}`}
        >
          {icon}
        </div>
      ) : (
        <div
          className={`mb-4 flex items-center justify-center rounded-full bg-muted ${
            compact ? 'size-14' : 'size-20'
          }`}
        >
          <AlertCircle className={`text-muted-foreground ${compact ? 'size-6' : 'size-8'}`} />
        </div>
      )}
      <p className={`font-semibold ${compact ? 'text-lg' : 'text-xl'}`}>{title}</p>
      {description && (
        <p className="mt-2 max-w-md text-sm leading-6 text-muted-foreground">{description}</p>
      )}
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
}

export function SkeletonPanel({ rows = 3, compact = false }: { rows?: number; compact?: boolean }) {
  return (
    <div className={`rounded-2xl border bg-card p-5 ${compact ? 'py-8' : 'py-10'}`}>
      <div className="space-y-4">
        {Array.from({ length: rows }).map((_, index) => (
          <div
            key={index}
            className="h-5 animate-pulse rounded-full bg-muted"
            style={{ width: `${72 - index * 10}%` }}
          />
        ))}
      </div>
    </div>
  )
}
