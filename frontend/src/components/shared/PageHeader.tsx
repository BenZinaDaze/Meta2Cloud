interface PageHeaderProps {
  kicker?: string
  title: React.ReactNode
  description?: string
  actions?: React.ReactNode
  className?: string
}

/** 统一页面头部：潮汐 kicker + 大标题 + 描述 + 右侧操作区 */
export default function PageHeader({ kicker, title, description, actions, className }: PageHeaderProps) {
  return (
    <div className={`mb-6 flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between ${className || ''}`}>
      <div className="min-w-0 max-w-2xl">
        {kicker && (
          <div className="mb-1.5 flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.16em] text-primary/90">
            <span className="h-px w-5 bg-gradient-to-r from-tide to-transparent" />
            {kicker}
          </div>
        )}
        <h1 className="text-2xl font-bold leading-tight tracking-tight sm:text-[32px]">
          {title}
        </h1>
        {description && (
          <p className="mt-2 hidden text-sm leading-7 text-muted-foreground sm:block">
            {description}
          </p>
        )}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  )
}
