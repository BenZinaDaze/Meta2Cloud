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
