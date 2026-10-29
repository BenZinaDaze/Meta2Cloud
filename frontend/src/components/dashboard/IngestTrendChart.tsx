// IngestTrendChart.tsx —— 近 30 天入库趋势手写 SVG 柱状图（容器尺寸自适应 + 悬停提示）。
import { useState, useEffect, useRef } from 'react'
import type { DayStat } from '@/types/api'
import { compact } from '@/components/dashboard/format'

function useContainerSize<T extends HTMLElement>() {
  const ref = useRef<T | null>(null)
  const [size, setSize] = useState({ w: 0, h: 0 })
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const ro = new ResizeObserver((entries) => {
      for (const e of entries) setSize({ w: e.contentRect.width, h: e.contentRect.height })
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  return [ref, size] as const
}

// 补齐近 days 天（后端只返回有记录的日期，UTC 对齐）
// 旧版后端对未知 API 会兜底返回 HTML（200），必须校验数组形状
export function fillDays(raw: unknown, days: number): DayStat[] {
  if (!Array.isArray(raw)) return []
  const map = new Map(
    raw
      .filter((d): d is { date: string; total: number; movies: number } => !!d && typeof d === 'object' && typeof (d as { date?: unknown }).date === 'string')
      .map((d) => [d.date, { total: Number(d.total) || 0, movies: Number(d.movies) || 0 } as const]),
  )
  const out: DayStat[] = []
  const now = new Date()
  for (let i = days - 1; i >= 0; i--) {
    const d = new Date(now)
    d.setUTCDate(d.getUTCDate() - i)
    const key = d.toISOString().slice(0, 10)
    const v = map.get(key)
    out.push({ date: key, total: v?.total ?? 0, movies: v?.movies ?? 0 })
  }
  return out
}

function niceCeil(v: number): number {
  if (v <= 4) return 4
  const pow = 10 ** Math.floor(Math.log10(v))
  for (const m of [1, 2, 2.5, 5, 10]) {
    if (m * pow >= v) return m * pow
  }
  return 10 * pow
}

// 顶部圆角、基线平头的柱形
function barPath(x: number, y: number, w: number, h: number): string {
  if (h <= 0.5) return ''
  const r = Math.min(4, w / 2, h)
  return (
    `M${x},${y + h}` +
    `L${x},${y + r} Q${x},${y} ${x + r},${y}` +
    `L${x + w - r},${y} Q${x + w},${y} ${x + w},${y + r}` +
    `L${x + w},${y + h} Z`
  )
}

export function IngestTrendChart({ data }: { data: DayStat[] }) {
  const [wrapRef, size] = useContainerSize<HTMLDivElement>()
  const [hover, setHover] = useState(-1)
  const width = size.w
  const H = Math.max(200, size.h || 216)
  const pad = { top: 10, right: 6, bottom: 24, left: 34 }

  const total = data.reduce((s, d) => s + d.total, 0)
  const maxVal = Math.max(...data.map((d) => d.total), 1)
  const top = niceCeil(maxVal)
  const innerW = Math.max(0, width - pad.left - pad.right)
  const innerH = H - pad.top - pad.bottom
  const band = data.length > 0 ? innerW / data.length : 0
  const barW = Math.min(24, Math.max(3, band * 0.55))
  const ticks = [0, top / 2, top]
  const labelEvery = Math.max(1, Math.ceil(data.length / 6))
  const hoveredBar = hover >= 0 && hover < data.length ? data[hover] : null

  return (
    <div
      ref={wrapRef}
      className="relative min-h-[216px] flex-1"
      onMouseLeave={() => setHover(-1)}
    >
      {width > 0 && (
        <svg width={width} height={H} role="img" aria-label="近 30 天入库趋势柱状图">
          {ticks.map((t) => {
            const y = pad.top + innerH - (t / top) * innerH
            return (
              <g key={t}>
                <line x1={pad.left} x2={width - pad.right} y1={y} y2={y} stroke="var(--border)" strokeWidth={1} />
                <text x={pad.left - 8} y={y + 3.5} textAnchor="end" fontSize={10} fill="var(--muted-foreground)">
                  {compact(t)}
                </text>
              </g>
            )
          })}
          {data.map((d, i) => {
            const h = (d.total / top) * innerH
            const x = pad.left + i * band + (band - barW) / 2
            const y = pad.top + innerH - h
            return (
              <path
                key={d.date}
                d={barPath(x, y, barW, h)}
                fill="var(--primary)"
                style={hover === i ? { filter: 'brightness(1.25)' } : undefined}
              />
            )
          })}
          {data.map((d, i) =>
            i % labelEvery === 0 ? (
              <text
                key={d.date}
                x={pad.left + i * band + band / 2}
                y={H - 6}
                textAnchor="middle"
                fontSize={10}
                fill="var(--muted-foreground)"
              >
                {d.date.slice(5)}
              </text>
            ) : null,
          )}
          {data.map((d, i) => (
            <rect
              key={d.date}
              x={pad.left + i * band}
              y={0}
              width={band}
              height={H - pad.bottom}
              fill="transparent"
              onPointerEnter={() => setHover(i)}
              onFocus={() => setHover(i)}
              tabIndex={-1}
            />
          ))}
        </svg>
      )}
      {hoveredBar && width > 0 && (
        <div
          className="pointer-events-none absolute z-10 rounded-lg border border-border/70 bg-card/95 px-2.5 py-1.5 text-xs shadow-lg backdrop-blur"
          style={{
            left: Math.min(Math.max(pad.left + hover * band + band / 2 - 44, 4), Math.max(width - 92, 4)),
            top: Math.max(pad.top + innerH - (hoveredBar.total / top) * innerH - 52, 2),
          }}
        >
          <span className="font-semibold text-foreground">{hoveredBar.total} 项</span>
          <span className="ml-1.5 text-muted-foreground">{hoveredBar.date.slice(5)}</span>
        </div>
      )}
      <div className="absolute right-0 top-0 text-xs text-muted-foreground">
        合计 <span className="font-semibold text-foreground">{total}</span> 项
      </div>
    </div>
  )
}
