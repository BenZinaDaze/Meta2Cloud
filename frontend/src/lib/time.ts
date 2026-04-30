export function relativeTime(
  isoStr: string | undefined,
  opts?: { fallback?: string; dayThreshold?: number }
): string {
  const fallback = opts?.fallback ?? '未知'
  const dayThreshold = opts?.dayThreshold ?? 7
  if (!isoStr) return fallback
  const diff = Math.floor((Date.now() - new Date(isoStr).getTime()) / 1000)
  if (diff < 60) return '刚刚'
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`
  if (diff < 86400 * dayThreshold) return `${Math.floor(diff / 86400)} 天前`
  return new Date(isoStr).toLocaleDateString('zh-CN')
}

export function formatDateTime(
  dateStr?: string,
  opts?: { locale?: string; fallback?: string }
): string {
  const fallback = opts?.fallback ?? '-'
  if (!dateStr) return fallback
  try {
    return new Date(dateStr).toLocaleString(opts?.locale ?? 'zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
  } catch {
    return dateStr
  }
}
