// U115Section.tsx —— 115 网盘配置区块（自 ConfigPage 抽离）。
// 自包含：OAuth 状态加载、扫码轮询、连接/Cookie 测试均在本组件内完成；
// 仅通过 cfg/set 与配置表单联动（保存仍由 ConfigPage 统一处理）。
import { useCallback, useEffect, useRef, useState } from 'react'
import {
  createU115OauthSession,
  fetchU115QrCode,
  getU115OauthStatus,
  pollU115OauthStatus,
  testU115Connection,
  testU115Cookie,
} from '@/api'
import { Field, FieldGroup, NoticeMessage, NumberInput, TextInput, Toggle } from '@/components/config/fields'
import FolderSelectInput from '@/components/config/FolderSelectInput'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import type { U115OauthStatus } from '@/types/api'
import { Separator } from '@/components/ui/separator'
import { apiDetail, useAsyncAction } from '@/hooks/useAsyncAction'
import { cn } from '@/lib/utils'

interface U115SectionProps {
  cfg: Record<string, unknown> | null
  set: (section: string, key: string, val: unknown) => void
  original: Record<string, unknown> | null
}

function formatBytes(bytes: number): string {
  const value = Number(bytes)
  if (!Number.isFinite(value) || value < 0) return '-'
  const GB = 1024 ** 3
  const TB = 1024 ** 4
  if (value < 5 * TB) return `${(value / GB).toFixed(2)} GB`
  return `${(value / TB).toFixed(2)} TB`
}

export default function U115Section({ cfg, set, original }: U115SectionProps) {
  const [u115Oauth, setU115Oauth] = useState<U115OauthStatus | null>(null)
  const u115OauthAction = useAsyncAction()
  const u115CookieAction = useAsyncAction()
  const [u115QrUrl, setU115QrUrl] = useState('')
  const [u115QrPreviewUrl, setU115QrPreviewUrl] = useState('')
  const [u115Polling, setU115Polling] = useState(false)
  const u115PollAbortRef = useRef<AbortController | null>(null)
  // 轮询 effect 内使用的稳定 setter 别名（useState setter 恒稳定，可安全列入依赖）
  const { setMessage: setU115OauthMsg } = u115OauthAction

  const loadU115OauthStatus = useCallback(async () => {
    try {
      const res = await getU115OauthStatus()
      setU115Oauth(res.data)
    } catch {
      setU115Oauth(null)
    }
  }, [])

  useEffect(() => {
    loadU115OauthStatus()
  }, [loadU115OauthStatus])

  // 115 扫码轮询
  useEffect(() => {
    if (!u115Polling || !u115QrUrl) return

    let cancelled = false
    let timer: ReturnType<typeof setTimeout> | null = null
    const startedAt = Date.now()
    const maxPollingMs = 5 * 60 * 1000
    const pollIntervalMs = 2000

    const finishPolling = () => {
      setU115QrUrl('')
      setU115QrPreviewUrl('')
      setU115Polling(false)
    }

    const scheduleNextPoll = () => {
      if (cancelled) return
      if (Date.now() - startedAt >= maxPollingMs) {
        setU115OauthMsg({ type: 'error', text: '115 扫码授权已超时，请重新发起扫码。' })
        finishPolling()
        return
      }
      timer = setTimeout(pollOnce, pollIntervalMs)
    }

    const pollOnce = async () => {
      try {
        const controller = new AbortController()
        u115PollAbortRef.current = controller
        const res = await pollU115OauthStatus({ signal: controller.signal })
        if (cancelled) return
        const data = res?.data || {}
        if (data.confirmed) {
          if (data.ok) {
            setU115OauthMsg({ type: 'success', text: '115 授权成功，token 已写入本地。' })
            finishPolling()
            loadU115OauthStatus()
          } else {
            setU115OauthMsg({ type: 'error', text: data.message || '115 换取 token 失败' })
            finishPolling()
          }
          return
        }
        const statusCode = Number(data.status)
        const msg = String(data.message || '')
        if (Number.isFinite(statusCode) && statusCode < 0) {
          setU115OauthMsg({ type: 'error', text: msg || `115 扫码授权已失效：${data.status}` })
          finishPolling()
          return
        }
        // 检查 message 是否包含错误关键词
        if (msg && (msg.includes('失败') || msg.includes('错误') || msg.includes('拒绝') || msg.includes('取消'))) {
          setU115OauthMsg({ type: 'error', text: `115 扫码失败：${msg}` })
          finishPolling()
          return
        }
        setU115OauthMsg({
          type: 'success',
          text: statusCode >= 1 ? '已扫码，等待手机确认…' : `等待扫码…`,
        })
        scheduleNextPoll()
      } catch (e) {
        if (cancelled) return
        if ((e as { code?: string; name?: string })?.code === 'ERR_CANCELED' || (e as { name?: string })?.name === 'CanceledError') return
        const status = (e as { response?: { status?: number } })?.response?.status
        if (status === 502 || status === 503 || status === 504 || !status) {
          setU115OauthMsg({ type: 'error', text: '115 扫码状态查询暂时超时，系统会继续自动重试…' })
          scheduleNextPoll()
          return
        }
        setU115Polling(false)
        setU115OauthMsg({ type: 'error', text: apiDetail(e) || (e as Error).message || '查询 115 扫码状态失败' })
      } finally {
        u115PollAbortRef.current = null
      }
    }

    scheduleNextPoll()

    return () => {
      cancelled = true
      if (timer) clearTimeout(timer)
      if (u115PollAbortRef.current) {
        u115PollAbortRef.current.abort()
        u115PollAbortRef.current = null
      }
    }
  }, [u115Polling, u115QrUrl, loadU115OauthStatus, setU115OauthMsg])

  async function handleU115CreateQr() {
    const savedU115Config = (original?.u115 as Record<string, unknown>) || {}
    const currentU115Config = (cfg?.u115 as Record<string, unknown>) || {}
    if (
      currentU115Config.client_id !== savedU115Config.client_id ||
      currentU115Config.token_json !== savedU115Config.token_json
    ) {
      u115OauthAction.setMessage({ type: 'error', text: '请先保存 115 Client ID 和 Token 路径后再扫码授权。' })
      return
    }

    setU115Polling(false)
    await u115OauthAction.run(async () => {
      const res = await createU115OauthSession({
        client_id: savedU115Config.client_id,
        token_json: savedU115Config.token_json,
      })
      setU115QrUrl(res?.data?.qrcode || '')
      try {
        const qrRes = await fetchU115QrCode()
        const objectUrl = URL.createObjectURL(qrRes.data)
        setU115QrPreviewUrl(objectUrl)
      } catch {
        setU115QrPreviewUrl('')
      }
      setU115Polling(true)
      loadU115OauthStatus()
      return '115 扫码会话已创建，请使用 App 扫码并确认，授权将自动完成。'
    }, '创建 115 扫码会话失败')
  }

  const handleU115ConnectionTest = () =>
    u115OauthAction.run(async () => {
      const res = await testU115Connection()
      const data = res?.data || {}
      loadU115OauthStatus()
      return `连接成功：剩余空间 ${formatBytes(data.remain_space)} / 总空间 ${formatBytes(data.total_space)}`
    }, '115 连接测试失败')

  const handleU115CookieTest = () =>
    u115CookieAction.run(async () => {
      const res = await testU115Cookie()
      const data = res?.data || {}
      const remainSpace = data?.space_info?.all_remain?.size
      const totalSpace = data?.space_info?.all_total?.size
      const spaceInfo = remainSpace != null && totalSpace != null
        ? `，剩余空间 ${formatBytes(remainSpace)} / 总空间 ${formatBytes(totalSpace)}`
        : ''
      return `Cookie 可用${spaceInfo}`
    }, 'Cookie 测试失败')

  return (
    <Card className="border-border/70 bg-card/40">
      <CardHeader>
        <CardTitle className="text-base">115 网盘</CardTitle>
        <CardDescription>OAuth、Cookie 与目录配置</CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        <div>
          <div className="mb-3 text-[13px] font-medium">授权状态</div>
          <div className="flex flex-wrap items-center gap-3">
            <Badge variant={u115Oauth?.authorized ? 'success' : 'secondary'}>
              {u115Oauth?.authorized ? '已授权' : '未授权'}
            </Badge>
            <Button
              variant="outline"
              size="sm"
              onClick={handleU115CreateQr}
              disabled={u115OauthAction.busy || !(cfg?.u115 as Record<string, unknown>)?.client_id}
            >
              {u115OauthAction.busy ? '处理中…' : (u115Polling ? '等待扫码…' : '扫码授权')}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={handleU115ConnectionTest}
              disabled={u115Oauth?.token_exists === false}
            >
              测试连接
            </Button>
          </div>
          {u115QrUrl && u115QrPreviewUrl && (
            <div className="mt-3 inline-block animate-stagger-in rounded-xl border border-border/70 bg-muted/40 p-3">
              <div className="text-xs text-muted-foreground">使用 115 App 扫码</div>
              <img src={u115QrPreviewUrl} alt="QR" className="mt-2 size-36 rounded-lg bg-white p-1" />
            </div>
          )}
          <NoticeMessage message={u115OauthAction.message} />
        </div>

        <Separator className="opacity-60" />

        <FieldGroup>
          <Field label="Client ID">
            <TextInput
              value={(cfg?.u115 as Record<string, unknown>)?.client_id as string | undefined}
              onChange={(v) => set('u115', 'client_id', v)}
              placeholder="your-client-id"
              mono
            />
          </Field>
          <Field label="Token 路径">
            <TextInput
              value={(cfg?.u115 as Record<string, unknown>)?.token_json as string | undefined}
              onChange={(v) => set('u115', 'token_json', v)}
              placeholder="config/115-token.json"
              mono
            />
          </Field>
          <Field label="云下载目录" description="离线保存位置（点击选择）">
            <FolderSelectInput
              value={(cfg?.u115 as Record<string, unknown>)?.download_folder_id as string | undefined}
              onChange={(v: string) => set('u115', 'download_folder_id', v)}
              placeholder="选择云下载目录"
            />
          </Field>
          <Field label="媒体库根目录" description="分类目录位于此目录下（点击选择）">
            <FolderSelectInput
              value={(cfg?.u115 as Record<string, unknown>)?.root_folder_id as string | undefined}
              onChange={(v: string) => set('u115', 'root_folder_id', v)}
              placeholder="选择媒体库根目录"
            />
          </Field>
          <Field label="共享请求 QPS" description="整理/播放/下载全项目共用，调低可降低风控风险">
            <NumberInput
              value={(cfg?.u115 as Record<string, unknown>)?.qps as number | undefined}
              onChange={(v) => set('u115', 'qps', v)}
              min={1}
              max={10}
            />
          </Field>
        </FieldGroup>

        <Separator className="opacity-60" />

        <div>
          <div className="mb-3 text-[13px] font-medium">自动整理</div>
          <FieldGroup>
            <Field label="启用监听">
              <Toggle
                value={(cfg?.u115 as Record<string, unknown>)?.auto_organize_enabled as boolean | undefined}
                onChange={(v) => set('u115', 'auto_organize_enabled', v)}
              />
            </Field>
            <Field label="定时规则" description="cron 表达式：分 时 日 月 周（服务器时区），支持 @every 30m 简写；留空表示不定时">
              <TextInput
                value={(cfg?.u115 as Record<string, unknown>)?.auto_organize_cron as string | undefined}
                onChange={(v) => set('u115', 'auto_organize_cron', v)}
                placeholder="*/5 * * * * 或 0 8 * * *"
                mono
              />
            </Field>
            <Field label="稳定等待" description="秒">
              <NumberInput
                value={(cfg?.u115 as Record<string, unknown>)?.auto_organize_stable_seconds as number | undefined ?? 30}
                onChange={(v) => set('u115', 'auto_organize_stable_seconds', v)}
                min={0}
                max={600}
              />
            </Field>
          </FieldGroup>
        </div>

        <Separator className="opacity-60" />

        <div>
          <div className="mb-3 flex items-center gap-3">
            <span className="text-[13px] font-medium">Cookie</span>
            <Button
              variant="outline"
              size="sm"
              onClick={handleU115CookieTest}
              disabled={u115CookieAction.busy || !(cfg?.u115 as Record<string, unknown>)?.cookie}
            >
              {u115CookieAction.busy ? '测试中…' : '测试'}
            </Button>
            {u115CookieAction.message && (
              <span className={cn('text-xs', u115CookieAction.message.type === 'success' ? 'text-success' : 'text-destructive')}>
                {u115CookieAction.message.text}
              </span>
            )}
          </div>
          <TextInput
            value={(cfg?.u115 as Record<string, unknown>)?.cookie as string | undefined}
            onChange={(v) => set('u115', 'cookie', v)}
            placeholder="用于分享转存等功能"
            mono
          />
        </div>
      </CardContent>
    </Card>
  )
}
