// configNormalize.ts —— 配置对象规范化（自 ConfigPage 抽离的纯函数）。
// 保存前把缺失/空值的配置项补为后端默认值，避免保存后把定时规则、
// 开关等写成空串；与后端 config.Default() 的默认值保持一致。
export function normalizeConfig(data: Record<string, unknown>): Record<string, unknown> {
  const next = structuredClone(data || {})
  const webui = { ...(next.webui as Record<string, unknown> || {}) }
  const tmdb = { ...(next.tmdb as Record<string, unknown> || {}) }
  const u115 = { ...(next.u115 as Record<string, unknown> || {}) }
  const rss = { ...(next.rss as Record<string, unknown> || {}) }
  const telegram = { ...(next.telegram as Record<string, unknown> || {}) }
  const mediavault = { ...(next.mediavault as Record<string, unknown> || {}) }
  const pipeline = { ...(next.pipeline as Record<string, unknown> || {}) }
  const strm = { ...(next.strm as Record<string, unknown> || {}) }
  const trakt = { ...(next.trakt as Record<string, unknown> || {}) }
  const plex = { ...(next.plex as Record<string, unknown> || {}) }

  if (webui.token_expire_hours === undefined || webui.token_expire_hours === null || webui.token_expire_hours === '') {
    webui.token_expire_hours = 24
  }
  if (webui.log_retention_days === undefined || webui.log_retention_days === null || webui.log_retention_days === '') {
    webui.log_retention_days = 7
  }
  if (tmdb.timeout === undefined || tmdb.timeout === null || tmdb.timeout === '') {
    tmdb.timeout = 10
  }
  if (tmdb.image_base_url === undefined || tmdb.image_base_url === null) {
    tmdb.image_base_url = ''
  }
  if (u115.auto_organize_cron === undefined || u115.auto_organize_cron === null) {
    // 与后端默认值一致（等价旧默认 45 秒轮询），避免保存后把定时规则写成空串（不定时）
    u115.auto_organize_cron = '* * * * *'
  }
  if (u115.auto_organize_stable_seconds === undefined || u115.auto_organize_stable_seconds === null || u115.auto_organize_stable_seconds === '') {
    u115.auto_organize_stable_seconds = 30
  }
  if (u115.qps === undefined || u115.qps === null || u115.qps === '') {
    u115.qps = 3
  }
  if (rss.poll_cron === undefined || rss.poll_cron === null) {
    // 与后端默认值一致（等价旧默认 300 秒），避免保存后把轮询写成空串（不定时）
    rss.poll_cron = '*/5 * * * *'
  }
  if (telegram.debounce_seconds === undefined || telegram.debounce_seconds === null || telegram.debounce_seconds === '') {
    telegram.debounce_seconds = 60
  }
  if (mediavault.scheme === undefined || mediavault.scheme === null || mediavault.scheme === '') {
    mediavault.scheme = 'http'
  }
  if (mediavault.timeout === undefined || mediavault.timeout === null || mediavault.timeout === '') {
    mediavault.timeout = 10
  }
  if (strm.enabled === undefined || strm.enabled === null) {
    strm.enabled = false
  }
  if (strm.sync_cron === undefined || strm.sync_cron === null) {
    strm.sync_cron = ''
  }
  if (!Array.isArray(strm.mappings)) {
    strm.mappings = []
  }
  if (!Array.isArray(strm.media_ext) || strm.media_ext.length === 0) {
    strm.media_ext = ['mp4', 'mkv', 'avi', 'mov', 'wmv', 'flv', 'ts', 'm2ts', 'webm', 'rmvb', 'rm', 'mpg', 'mpeg', 'vob', 'iso', '3gp']
  }
  if (strm.fetch_sidecar === undefined || strm.fetch_sidecar === null) {
    strm.fetch_sidecar = true
  }
  if (!Array.isArray(strm.sidecar_ext) || strm.sidecar_ext.length === 0) {
    strm.sidecar_ext = ['srt', 'ass', 'ssa']
  }
  if (strm.refresh_plex === undefined || strm.refresh_plex === null) {
    strm.refresh_plex = false
  }
  if (trakt.enabled === undefined || trakt.enabled === null) {
    trakt.enabled = false
  }
  if (trakt.sync_cron === undefined || trakt.sync_cron === null) {
    // 与后端默认值一致（等价旧默认 900 秒），避免保存后把定时写成空串（不定时）
    trakt.sync_cron = '*/15 * * * *'
  }
  if (trakt.session_poll_seconds === undefined || trakt.session_poll_seconds === null || trakt.session_poll_seconds === '') {
    trakt.session_poll_seconds = 45
  }
  if (trakt.sse_enabled === undefined || trakt.sse_enabled === null) {
    trakt.sse_enabled = true
  }
  if (trakt.sync_to_plex === undefined || trakt.sync_to_plex === null) {
    trakt.sync_to_plex = true
  }
  if (trakt.sync_to_trakt === undefined || trakt.sync_to_trakt === null) {
    trakt.sync_to_trakt = true
  }
  if (plex.timeout === undefined || plex.timeout === null || plex.timeout === '') {
    plex.timeout = 10
  }

  next.webui = webui
  next.tmdb = tmdb
  next.u115 = u115
  next.trakt = trakt
  next.plex = plex
  next.rss = rss
  next.telegram = telegram
  next.mediavault = mediavault
  next.pipeline = pipeline
  next.strm = strm
  return next
}
