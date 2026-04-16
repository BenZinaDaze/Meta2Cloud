export function StatePanel({
  icon,
  title,
  description,
  tone = 'neutral',
  compact = false,
  action = null,
}) {
  const tones = {
    neutral: {
      background: 'rgba(255,255,255,0.03)',
      border: '1px solid var(--color-border)',
      title: 'var(--color-text)',
      description: 'var(--color-muted)',
      icon: 'var(--color-muted-soft)',
    },
    danger: {
      background: 'rgba(239,125,117,0.08)',
      border: '1px solid rgba(239,125,117,0.2)',
      title: 'var(--color-danger)',
      description: 'var(--color-muted)',
      icon: 'var(--color-danger)',
    },
  }

  const palette = tones[tone] || tones.neutral

  return (
    <div
      className={`flex flex-col items-center justify-center rounded-[24px] px-5 text-center ${compact ? 'py-10' : 'py-16 sm:py-24'}`}
      style={{ background: palette.background, border: palette.border }}
    >
      {icon ? (
        <div
          className={`mb-4 flex items-center justify-center rounded-full ${compact ? 'size-14 text-3xl' : 'size-20 text-5xl'}`}
          style={{ background: 'rgba(255,255,255,0.03)', color: palette.icon }}
        >
          {icon}
        </div>
      ) : null}
      <p className={`${compact ? 'text-lg' : 'text-xl'} font-semibold`} style={{ color: palette.title }}>
        {title}
      </p>
      {description ? (
        <p className="mt-2 max-w-md text-sm leading-6" style={{ color: palette.description }}>
          {description}
        </p>
      ) : null}
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  )
}

export function SkeletonPanel({ rows = 3, compact = false }) {
  return (
    <div
      className={`rounded-[24px] border px-5 ${compact ? 'py-8' : 'py-10'}`}
      style={{ background: 'rgba(255,255,255,0.03)', borderColor: 'var(--color-border)' }}
    >
      <div className="animate-pulse space-y-4">
        {Array.from({ length: rows }).map((_, index) => (
          <div
            key={index}
            className="h-5 rounded-full"
            style={{ width: `${72 - index * 10}%`, background: 'rgba(255,255,255,0.08)' }}
          />
        ))}
      </div>
    </div>
  )
}
