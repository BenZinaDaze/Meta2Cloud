// WatchedSyncSection.tsx —— 观看同步（Trakt ↔ Plex）配置区块（自 ConfigPage 抽离）。
// 自包含：授权状态加载、设备码/PIN 轮询、同步状态自适应轮询与触发均在本组件内完成；
// 仅通过 cfg/set 与配置表单联动（保存仍由 ConfigPage 统一处理）。
import { useCallback, useEffect, useState } from 'react'
import { ExternalLink } from 'lucide-react'
import {
  createPlexPin,
  createTraktDeviceCode,
  disconnectTrakt,
  getTraktOauthStatus,
  getWatchedSyncStatus,
  pollPlexPin,
  pollTraktOauth,
  testPlexConnection,
  triggerWatchedSync,
} from '@/api'
import { Field, FieldGroup, NoticeMessage, NumberInput, TextInput, Toggle } from '@/components/config/fields'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { apiDetail, useAsyncAction } from '@/hooks/useAsyncAction'

interface WatchedSyncSectionProps {
  cfg: Record<string, unknown> | null
  set: (section: string, key: string, val: unknown) => void
}

export default function WatchedSyncSection({ cfg, set }: WatchedSyncSectionProps) {
  const [traktOauth, setTraktOauth] = useState<Record<string, unknown> | null>(null)
  const [traktPairing, setTraktPairing] = useState<{ user_code: string; verification_url: string } | null>(null)
  const [traktPolling, setTraktPolling] = useState(false)
  const traktAction = useAsyncAction()
  const [plexPairing, setPlexPairing] = useState<{ code: string; auth_url: string } | null>(null)
  const [plexPolling, setPlexPolling] = useState(false)
  const plexAction = useAsyncAction()
  const [syncStatus, setSyncStatus] = useState<Record<string, unknown> | null>(null)
  const syncTriggerAction = useAsyncAction()
  // 轮询 effect 内使用的稳定 setter 别名（useState setter 恒稳定，可安全列入依赖）
  const { setMessage: setTraktMsg } = traktAction
  const { setMessage: setPlexMsg } = plexAction

  const loadWatchedSyncStatus = useCallback(async () => {
    try {
      const res = await getWatchedSyncStatus()
      setSyncStatus(res.data)
    } catch {
      /* 状态加载失败不阻塞配置页 */
    }
  }, [])

  const loadTraktOauthStatus = useCallback(async () => {
    try {
      const res = await getTraktOauthStatus()
      setTraktOauth(res.data)
    } catch {
      setTraktOauth(null)
    }
  }, [])

  useEffect(() => {
    loadTraktOauthStatus()
    loadWatchedSyncStatus()
  }, [loadTraktOauthStatus, loadWatchedSyncStatus])

  // 同步进行中自适应轮询（running ? 3s : 30s）
  const syncRunning = Boolean(syncStatus?.running)
  useEffect(() => {
    let stopped = false
    let timer: ReturnType<typeof setTimeout> | null = null
    const tick = async () => {
      try {
        const res = await getWatchedSyncStatus()
        if (stopped) return
        setSyncStatus(res.data)
        if (res.data?.running) {
          timer = setTimeout(tick, 3000)
        } else {
          timer = setTimeout(tick, 30000)
          loadWatchedSyncStatus()
        }
      } catch {
        if (!stopped) timer = setTimeout(tick, 30000)
      }
    }
    timer = setTimeout(tick, syncRunning ? 3000 : 30000)
    return () => {
      stopped = true
      if (timer) clearTimeout(timer)
    }
  }, [syncRunning, loadWatchedSyncStatus])

  // Trakt 设备码轮询（前端驱动，与 115 扫码同模式）
  useEffect(() => {
    if (!traktPolling) return
    let cancelled = false
    let timer: ReturnType<typeof setTimeout> | null = null
    const startedAt = Date.now()
    const maxPollingMs = 5 * 60 * 1000
    const pollIntervalMs = 2000

    const finishPolling = () => setTraktPolling(false)

    const pollOnce = async () => {
      try {
        const res = await pollTraktOauth()
        if (cancelled) return
        const data = res?.data || {}
        if (data.confirmed) {
          setTraktMsg({ type: 'success', text: data.user ? `Trakt 授权成功（用户 ${data.user}）` : 'Trakt 授权成功' })
          setTraktPairing(null)
          finishPolling()
          loadTraktOauthStatus()
          loadWatchedSyncStatus()
          return
        }
        if (data.expired) {
          setTraktMsg({ type: 'error', text: data.message || '设备码已过期，请重新发起配对' })
          setTraktPairing(null)
          finishPolling()
          return
        }
        if (Date.now() - startedAt >= maxPollingMs) {
          setTraktMsg({ type: 'error', text: 'Trakt 配对等待超时，请重新发起。' })
          setTraktPairing(null)
          finishPolling()
          return
        }
        timer = setTimeout(pollOnce, pollIntervalMs)
      } catch (e) {
        if (cancelled) return
        if ((e as { code?: string })?.code === 'ERR_CANCELED') return
        const status = (e as { response?: { status?: number } })?.response?.status
        if (status === 404) {
          // 会话不存在（已消费或未发起）→ 停止
          setTraktPairing(null)
          finishPolling()
          return
        }
        timer = setTimeout(pollOnce, pollIntervalMs)
      }
    }
    timer = setTimeout(pollOnce, pollIntervalMs)
    return () => {
      cancelled = true
      if (timer) clearTimeout(timer)
    }
  }, [traktPolling, loadTraktOauthStatus, loadWatchedSyncStatus, setTraktMsg])

  // Plex PIN 轮询
  useEffect(() => {
    if (!plexPolling) return
    let cancelled = false
    let timer: ReturnType<typeof setTimeout> | null = null
    const startedAt = Date.now()
    const maxPollingMs = 5 * 60 * 1000
    const pollIntervalMs = 2500

    const pollOnce = async () => {
      try {
        const res = await pollPlexPin()
        if (cancelled) return
        const data = res?.data || {}
        if (data.confirmed) {
          if (data.reachable === false) {
            setPlexMsg({ type: 'error', text: 'Plex 授权完成，但服务器不可达，请检查 plex.base_url 与网络。' })
          } else {
            setPlexMsg({ type: 'success', text: data.message || 'Plex 授权成功' })
          }
          setPlexPairing(null)
          setPlexPolling(false)
          loadWatchedSyncStatus()
          return
        }
        if (Date.now() - startedAt >= maxPollingMs) {
          setPlexMsg({ type: 'error', text: 'Plex 配对等待超时，请重新发起。' })
          setPlexPairing(null)
          setPlexPolling(false)
          return
        }
        timer = setTimeout(pollOnce, pollIntervalMs)
      } catch (e) {
        if (cancelled) return
        if ((e as { code?: string })?.code === 'ERR_CANCELED') return
        const status = (e as { response?: { status?: number } })?.response?.status
        if (status === 404) {
          setPlexPairing(null)
          setPlexPolling(false)
          return
        }
        timer = setTimeout(pollOnce, pollIntervalMs)
      }
    }
    timer = setTimeout(pollOnce, pollIntervalMs)
    return () => {
      cancelled = true
      if (timer) clearTimeout(timer)
    }
  }, [plexPolling, loadWatchedSyncStatus, setPlexMsg])

  async function handleTraktPair() {
    await traktAction.run(async () => {
      const res = await createTraktDeviceCode()
      const data = res.data || {}
      setTraktPairing({ user_code: data.user_code, verification_url: data.verification_url })
      setTraktPolling(true)
    }, (e) => apiDetail(e) || (e as Error).message || '创建设备码失败')
  }

  const handleTraktDisconnect = () =>
    traktAction.run(async () => {
      await disconnectTrakt()
      setTraktPairing(null)
      setTraktPolling(false)
      loadTraktOauthStatus()
      loadWatchedSyncStatus()
      return '已断开 Trakt 授权'
    }, '断开失败')

  async function handlePlexPair() {
    await plexAction.run(async () => {
      const res = await createPlexPin()
      const data = res.data || {}
      setPlexPairing({ code: data.code, auth_url: data.auth_url })
      setPlexPolling(true)
    }, (e) => apiDetail(e) || (e as Error).message || '创建 PIN 失败')
  }

  const handlePlexTest = () =>
    plexAction.run(async () => {
      const res = await testPlexConnection()
      return `连接成功（服务器 ${res.data?.server_name || ''} v${res.data?.server_version || ''}）`
    }, '连接失败')

  const handleWatchedSyncTrigger = () =>
    syncTriggerAction.run(async () => {
      const res = await triggerWatchedSync()
      loadWatchedSyncStatus()
      return res.data?.message || '观看同步已触发'
    }, '触发失败')

  return (
    <Card className="border-border/70 bg-card/40">
      <CardHeader>
        <CardTitle className="text-base">观看同步（Trakt ↔ Plex）</CardTitle>
        <CardDescription>
          双向同步两侧观看状态；冲突时以 Trakt 为准。同步历史与明细见「Trakt 同步」页。
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {/* 总览状态 */}
        <div>
          <div className="mb-3 flex flex-wrap items-center gap-3">
            <Badge variant={(cfg?.trakt as Record<string, unknown>)?.enabled ? 'success' : 'secondary'}>
              定时同步{(cfg?.trakt as Record<string, unknown>)?.enabled ? '已开启' : '未开启'}
            </Badge>
            <Badge variant={traktOauth?.authorized ? 'success' : 'secondary'}>
              Trakt {traktOauth?.authorized ? '已授权' : '未授权'}
            </Badge>
            <Badge variant={syncStatus?.plex_connected ? 'success' : 'secondary'}>
              Plex {syncStatus?.plex_connected ? (syncStatus?.plex_reachable ? '已连接' : '已授权（服务器不可达）') : '未连接'}
            </Badge>
            <Button
              variant="outline"
              size="sm"
              onClick={handleWatchedSyncTrigger}
              disabled={syncTriggerAction.busy || !traktOauth?.authorized || !syncStatus?.plex_connected}
            >
              {syncStatus?.running ? '同步中…' : '立即同步'}
            </Button>
          </div>
          <NoticeMessage message={syncTriggerAction.message} />
          {syncStatus?.last_run != null && (
            <div className="mt-1 text-xs text-muted-foreground">
              上次同步：{String((syncStatus.last_run as Record<string, unknown>).started_at || '')}
              {' · '}→Trakt {String((syncStatus.last_run as Record<string, unknown>).to_trakt ?? 0)}
              {' · '}→Plex {String((syncStatus.last_run as Record<string, unknown>).to_plex ?? 0)}
              {(syncStatus.last_run as Record<string, unknown>).error
                ? ` · 错误：${String((syncStatus.last_run as Record<string, unknown>).error)}`
                : ''}
            </div>
          )}
        </div>

        {/* ── Trakt 侧（独立面板）── */}
        <div className="space-y-4 rounded-xl border border-border/60 bg-card/40 p-4">
          <div className="text-sm font-medium">Trakt 授权</div>
          <div className="space-y-3">
            <div className="flex flex-wrap items-center gap-3">
              <Badge variant={traktOauth?.token_expired ? 'warning' : traktOauth?.authorized ? 'success' : 'secondary'}>
                {traktOauth?.token_expired ? 'Token 已过期' : traktOauth?.authorized ? '已授权' : '未授权'}
              </Badge>
              {Boolean(traktOauth?.authorized) && (
                <Button variant="outline" size="sm" onClick={handleTraktDisconnect}>
                  断开
                </Button>
              )}
            </div>
            <p className="text-xs text-muted-foreground">
              先在 trakt.tv/oauth/applications 创建应用（类型任选，Redirect URI 填 http://localhost），
              把 Client ID 填入下方并保存，再点击「设备码配对」。授权后 token 有效期 90 天，会自动续期。
            </p>
            <FieldGroup>
              <Field label="Client ID" description="trakt.tv/oauth/applications 创建" fullWidth>
                <div className="flex flex-wrap items-center gap-3">
                  <div className="min-w-0 flex-1 sm:max-w-xl">
                    <TextInput
                      value={(cfg?.trakt as Record<string, unknown>)?.client_id as string | undefined}
                      onChange={(v) => set('trakt', 'client_id', v)}
                      placeholder="your-trakt-client-id"
                      mono
                    />
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleTraktPair}
                    disabled={traktAction.busy || !(cfg?.trakt as Record<string, unknown>)?.client_id}
                    className="shrink-0"
                  >
                    {traktAction.busy ? '处理中…' : traktPolling ? '等待确认…' : '设备码配对'}
                  </Button>
                  {traktPairing && (
                    <a
                      href={traktPairing.verification_url}
                      target="_blank"
                      rel="noreferrer"
                      className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
                    >
                      打开 {traktPairing.verification_url.replace(/^https?:\/\//, '')}
                      <ExternalLink className="size-3" />
                    </a>
                  )}
                </div>
              </Field>
            </FieldGroup>
            {traktPairing && (
              <div className="inline-flex flex-col gap-1 animate-stagger-in rounded-xl border border-border/70 bg-muted/40 p-3">
                <div className="text-xs text-muted-foreground">在 Trakt 页面输入以下代码：</div>
                <div className="font-mono text-2xl font-semibold tracking-[0.3em]">{traktPairing.user_code}</div>
              </div>
            )}
            <NoticeMessage message={traktAction.message} />
          </div>
        </div>

        {/* ── Plex 侧（独立面板）── */}
        <div className="space-y-4 rounded-xl border border-border/60 bg-card/40 p-4">
          <div className="text-sm font-medium">Plex 连接</div>
          <div className="space-y-3">
            <div className="flex flex-wrap items-center gap-3">
              <Badge variant={syncStatus?.plex_connected ? 'success' : 'secondary'}>
                {syncStatus?.plex_connected ? '已授权' : '未授权'}
              </Badge>
              <Button
                variant="outline"
                size="sm"
                onClick={handlePlexPair}
                disabled={plexAction.busy || !(cfg?.plex as Record<string, unknown>)?.base_url}
              >
                {plexAction.busy ? '处理中…' : plexPolling ? '等待授权…' : 'PIN 授权'}
              </Button>
              <Button variant="outline" size="sm" onClick={handlePlexTest} disabled={plexAction.busy || !syncStatus?.plex_connected}>
                {plexAction.busy ? '测试中…' : '测试连接'}
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              填写 Plex 服务器地址（如 http://192.168.1.10:32400）并保存后，点击「PIN 授权」，
              在弹出的 Plex 授权页登录并确认。授权凭证保存在本地 plex-auth.json。
            </p>
            <FieldGroup>
              <Field label="Plex 服务器地址" description="Meta2Cloud 需能直连该地址" fullWidth>
                <div className="max-w-xl">
                  <TextInput
                    value={(cfg?.plex as Record<string, unknown>)?.base_url as string | undefined}
                    onChange={(v) => set('plex', 'base_url', v)}
                    placeholder="http://192.168.1.10:32400"
                    mono
                  />
                </div>
              </Field>
              <Field label="请求超时" description="秒">
                <NumberInput
                  value={(cfg?.plex as Record<string, unknown>)?.timeout as number | undefined ?? 10}
                  onChange={(v) => set('plex', 'timeout', v)}
                  min={1}
                  max={120}
                />
              </Field>
            </FieldGroup>
            {plexPairing && (
              <div className="inline-flex flex-col gap-1 animate-stagger-in rounded-xl border border-border/70 bg-muted/40 p-3">
                <div className="text-xs text-muted-foreground">1. 在 Plex 授权页确认（无需输入代码，页面会自动关联）</div>
                <a
                  href={plexPairing.auth_url}
                  target="_blank"
                  rel="noreferrer"
                  className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
                >
                  打开 Plex 授权页 <ExternalLink className="size-3" />
                </a>
                <div className="text-xs text-muted-foreground">备用代码：{plexPairing.code}</div>
              </div>
            )}
            <NoticeMessage message={plexAction.message} />
          </div>
        </div>

        {/* ── 同步选项（两侧共用）── */}
        <div className="space-y-4 rounded-xl border border-border/60 bg-card/40 p-4">
          <div className="text-sm font-medium">同步设置</div>
          {/* 开关区：集中放一起 */}
          <FieldGroup>
            <Field label="启用定时同步" description="按下方定时规则自动全量同步">
              <Toggle
                value={(cfg?.trakt as Record<string, unknown>)?.enabled as boolean | undefined}
                onChange={(v) => set('trakt', 'enabled', v)}
              />
            </Field>
            <Field label="实时推送（SSE）" description="与 Plex 保持长连接，播放进度实时同步到 Trakt；关闭后由会话轮询同步">
              <Toggle
                value={(cfg?.trakt as Record<string, unknown>)?.sse_enabled as boolean | undefined}
                onChange={(v) => set('trakt', 'sse_enabled', v)}
              />
            </Field>
            <Field label="同步方向 → Plex" description="将 Trakt 观看状态同步至 Plex">
              <Toggle
                value={(cfg?.trakt as Record<string, unknown>)?.sync_to_plex as boolean | undefined}
                onChange={(v) => set('trakt', 'sync_to_plex', v)}
              />
            </Field>
            <Field label="同步方向 → Trakt" description="将 Plex 观看记录同步至 Trakt">
              <Toggle
                value={(cfg?.trakt as Record<string, unknown>)?.sync_to_trakt as boolean | undefined}
                onChange={(v) => set('trakt', 'sync_to_trakt', v)}
              />
            </Field>
          </FieldGroup>
          {/* 同段内子分组分隔：淡虚线（与跨侧实线区分） */}
          <div className="border-t border-dashed border-border/50" />
          {/* 参数区：集中放一起 */}
          <FieldGroup>
            <Field label="定时规则" description="cron 表达式：分 时 日 月 周（服务器时区），支持 @every 简写（如 @every 15m，间隔不得小于 1 分钟）；留空仅 SSE 实时/会话轮询/手动触发">
              <TextInput
                value={(cfg?.trakt as Record<string, unknown>)?.sync_cron as string | undefined}
                onChange={(v) => set('trakt', 'sync_cron', v)}
                placeholder="*/15 * * * *"
                mono
              />
            </Field>
            <Field label="会话轮询间隔" description="秒；SSE 未连接时生效，用于检测播放进度">
              <NumberInput
                value={(cfg?.trakt as Record<string, unknown>)?.session_poll_seconds as number | undefined ?? 45}
                onChange={(v) => set('trakt', 'session_poll_seconds', v)}
                min={15}
                max={600}
              />
            </Field>
          </FieldGroup>
        </div>
      </CardContent>
    </Card>
  )
}
