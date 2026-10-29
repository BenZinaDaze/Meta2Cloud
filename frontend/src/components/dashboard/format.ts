// format.ts —— 仪表盘展示格式化：数字紧凑、字节、入库结果徽标映射。

/** KPI/坐标轴的紧凑数字（1.2K / 34.5K） */
export function compact(n: number): string {
  if (n >= 1000) return `${(n / 1000).toFixed(n >= 10000 ? 0 : 1)}K`
  return String(n)
}

/** 字节数 → MB/GB/TB 展示（115 空间用，与 lib/format 的通用版精度语义不同） */
export function formatBytes(n: number): string {
  if (!n) return '0'
  const tb = n / 1024 ** 4
  if (tb >= 1) return `${tb.toFixed(tb >= 100 ? 0 : 1)} TB`
  const gb = n / 1024 ** 3
  if (gb >= 1) return `${gb.toFixed(gb >= 100 ? 0 : 1)} GB`
  const mb = n / 1024 ** 2
  return `${mb.toFixed(0)} MB`
}

/** 最近入库的状态徽标映射 */
export const statusBadge: Record<string, { label: string; className: string }> = {
  success: { label: '成功', className: 'border-success/30 bg-success/10 text-success' },
  failed: { label: '失败', className: 'border-destructive/30 bg-destructive/10 text-destructive' },
  no_tmdb: { label: '无 TMDB', className: 'border-warning/30 bg-warning/10 text-warning' },
}
