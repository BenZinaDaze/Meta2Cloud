export function formatBytes(bytes?: number | null): string {
  if (!bytes) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  const size = bytes / 1024 ** index
  return `${size >= 100 ? size.toFixed(0) : size.toFixed(1)} ${units[index]}`
}
