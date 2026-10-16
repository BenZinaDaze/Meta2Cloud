import { useState, useEffect, useCallback, useRef } from 'react'
import {
  Save,
  RotateCcw,
  Eye,
  EyeOff,
  Plus,
  X,
  RefreshCw,
  Cloud,
  Shield,
  Database,
  HardDrive,
  Rss,
  Bell,
  FolderSync,
  FileText,
  FileVideo,
  Users,
  BookOpen,
  Captions,
  RadioTower,
  AlertTriangle,
  Dices,
} from 'lucide-react'
import {
  getMainConfig,
  saveMainConfig,
  getParserRulesConfig,
  saveParserRulesConfig,
  testMediaVault,
  getU115OauthStatus,
  createU115OauthSession,
  testU115Connection,
  testU115Cookie,
  fetchU115QrCode,
  pollU115OauthStatus,
  refreshLibrary,
} from '@/api'
import CustomWordsHelp from '@/components/config/CustomWordsHelp'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { SkeletonPanel } from '@/components/StatePanel'
import PageHeader from '@/components/shared/PageHeader'
import { cn } from '@/lib/utils'

const PAGE_VARIANTS = {
  general: {
    title: '系统配置',
    description: '管理存储、认证、下载与通知等核心设置',
  },
  filenameRules: {
    title: '识别规则',
    description: '文件名解析、自定义识别词与字幕组配置',
  },
}

const PARSER_TABS = [
  { key: 'words', label: '识别词', icon: FileText },
  { key: 'groups', label: '字幕组', icon: Users },
  { key: 'help', label: '说明', icon: BookOpen },
]

const CONFIG_TABS = [
  { key: 'storage', label: '存储', icon: Cloud },
  { key: 'auth', label: '认证', icon: Shield },
  { key: 'tmdb', label: 'TMDB', icon: Database },
  { key: 'u115', label: '115', icon: HardDrive },
  { key: 'rss', label: 'RSS', icon: Rss },
  { key: 'telegram', label: '通知', icon: Bell },
  { key: 'mediavault', label: 'MediaVault', icon: RadioTower },
  { key: 'pipeline', label: '策略', icon: FolderSync },
  { key: 'subtitle', label: '字幕', icon: Captions },
  { key: 'strm', label: 'STRM', icon: FileVideo },
]

function normalizeConfig(data: Record<string, unknown>): Record<string, unknown> {
  const next = structuredClone(data || {})
  const webui = { ...(next.webui as Record<string, unknown> || {}) }
  const tmdb = { ...(next.tmdb as Record<string, unknown> || {}) }
  const u115 = { ...(next.u115 as Record<string, unknown> || {}) }
  const rss = { ...(next.rss as Record<string, unknown> || {}) }
  const telegram = { ...(next.telegram as Record<string, unknown> || {}) }
  const mediavault = { ...(next.mediavault as Record<string, unknown> || {}) }
  const pipeline = { ...(next.pipeline as Record<string, unknown> || {}) }
  const strm = { ...(next.strm as Record<string, unknown> || {}) }

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
  if (u115.auto_organize_poll_seconds === undefined || u115.auto_organize_poll_seconds === null || u115.auto_organize_poll_seconds === '') {
    u115.auto_organize_poll_seconds = 45
  }
  if (u115.auto_organize_stable_seconds === undefined || u115.auto_organize_stable_seconds === null || u115.auto_organize_stable_seconds === '') {
    u115.auto_organize_stable_seconds = 30
  }
  if (u115.qps === undefined || u115.qps === null || u115.qps === '') {
    u115.qps = 3
  }
  if (rss.poll_seconds === undefined || rss.poll_seconds === null || rss.poll_seconds === '') {
    rss.poll_seconds = 300
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
  if (pipeline.skip_metadata_upload === undefined || pipeline.skip_metadata_upload === null) {
    pipeline.skip_metadata_upload = false
  }
  if (strm.enabled === undefined || strm.enabled === null) {
    strm.enabled = false
  }
  if (strm.sync_interval_seconds === undefined || strm.sync_interval_seconds === null || strm.sync_interval_seconds === '') {
    strm.sync_interval_seconds = 0
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

  next.webui = webui
  next.tmdb = tmdb
  next.u115 = u115
  next.rss = rss
  next.telegram = telegram
  next.mediavault = mediavault
  next.pipeline = pipeline
  next.strm = strm
  return next
}

function FieldGroup({ children }: { children: React.ReactNode }) {
  return <div className="grid gap-x-8 gap-y-4 sm:grid-cols-2">{children}</div>
}

function Field({
  label,
  description,
  children,
  fullWidth = false,
}: {
  label: string
  description?: string
  children: React.ReactNode
  fullWidth?: boolean
}) {
  return (
    <div className={fullWidth ? 'sm:col-span-2' : ''}>
      <div className="mb-1.5 flex items-baseline gap-2">
        <label className="text-[13px] font-medium">{label}</label>
        {description && (
          <span className="text-[11px] text-muted-foreground/80">{description}</span>
        )}
      </div>
      {children}
    </div>
  )
}

function TextInput({ value, onChange, placeholder, type = 'text', mono = false }: {
  value: string | undefined
  onChange: (v: string) => void
  placeholder?: string
  type?: string
  mono?: boolean
}) {
  const [show, setShow] = useState(false)
  const isPassword = type === 'password'
  const inputType = isPassword ? (show ? 'text' : 'password') : type

  return (
    <div className="relative flex items-center">
      <Input
        type={inputType}
        value={value ?? ''}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className={mono ? 'font-mono text-[13px]' : ''}
      />
      {isPassword && (
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          onClick={() => setShow((s) => !s)}
          className="absolute right-2"
        >
          {show ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
        </Button>
      )}
    </div>
  )
}

function NumberInput({ value, onChange, min, max }: {
  value: number | undefined
  onChange: (v: number) => void
  min?: number
  max?: number
}) {
  return (
    <Input
      type="number"
      value={value ?? ''}
      onChange={(e) => {
        const v = Number(e.target.value)
        if (!Number.isNaN(v)) {
          const clamped = Math.min(max ?? v, Math.max(min ?? v, v))
          onChange(clamped)
        }
      }}
      min={min}
      max={max}
      className="w-24 tabular-nums"
    />
  )
}

function Toggle({ value, onChange }: { value: boolean | undefined; onChange: (v: boolean) => void }) {
  return (
    <div className="flex items-center gap-3">
      <Switch checked={!!value} onCheckedChange={onChange} />
      <span className="text-xs text-muted-foreground">{value ? '已开启' : '已关闭'}</span>
    </div>
  )
}

function ListField({ value = [], onChange }: { value: string[]; onChange: (v: string[]) => void }) {
  const [draft, setDraft] = useState('')

  function add() {
    const v = draft.trim()
    if (!v || value.includes(v)) {
      setDraft('')
      return
    }
    onChange([...value, v])
    setDraft('')
  }

  return (
    <div className="space-y-2">
      {value.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {value.map((item, i) => (
            <div
              key={i}
              className="inline-flex max-w-full items-center gap-2 rounded-full border border-border/70 bg-muted/50 py-1.5 pl-3 pr-1.5"
            >
              <span className="truncate font-mono text-xs">{item}</span>
              <Button
                variant="ghost"
                size="icon-xs"
                onClick={() => onChange(value.filter((_, j) => j !== i))}
                className="text-muted-foreground hover:bg-destructive/15 hover:text-destructive"
              >
                <X className="size-3" />
              </Button>
            </div>
          ))}
        </div>
      )}
      <div className="flex gap-2">
        <Input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), add())}
          placeholder="输入后按 Enter 或点击添加"
          className="font-mono text-xs"
        />
        <Button variant="outline" size="sm" onClick={add} disabled={!draft.trim()}>
          <Plus className="size-4" />
          添加
        </Button>
      </div>
    </div>
  )
}

function MultilineRulesField({ value = [], onChange, placeholder = '' }: {
  value: string[]
  onChange: (v: string[]) => void
  placeholder?: string
}) {
  const serialize = useCallback((lines: string[]) => (Array.isArray(lines) ? lines.join('\n') : ''), [])
  const parse = useCallback(
    (nextText: string) => nextText.split('\n').map((line) => line.trim()).filter(Boolean),
    [],
  )
  const [draftText, setDraftText] = useState(() => serialize(value))
  const lineCount = draftText === '' ? 1 : draftText.split('\n').length

  useEffect(() => {
    const incomingText = serialize(value)
    const draftMatchesValue =
      JSON.stringify(parse(draftText)) === JSON.stringify(Array.isArray(value) ? value : [])
    if (!draftMatchesValue && incomingText !== draftText) {
      setDraftText(incomingText)
    }
  }, [draftText, parse, serialize, value])

  return (
    <div className="space-y-2">
      <Textarea
        value={draftText}
        onChange={(e) => {
          const nextText = e.target.value
          setDraftText(nextText)
          onChange(parse(nextText))
        }}
        placeholder={placeholder}
        spellCheck={false}
        className="resize-none font-mono text-[13px] leading-6"
        rows={Math.max(8, Math.min(18, lineCount + 2))}
      />
      <div className="text-xs text-muted-foreground">
        一行一个规则，空行会自动忽略。当前 {value.length} 条。
      </div>
    </div>
  )
}

/** 生成 128 位熵的随机签名密钥（浏览器密码学安全随机源）。 */
function randomSecret(): string {
  const bytes = new Uint8Array(16)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
}

function NoticeMessage({ message }: { message: { type: 'success' | 'error'; text: string } | null }) {
  if (!message) return null
  return (
    <div
      className={cn(
        'mt-3 animate-stagger-in rounded-lg border px-3 py-2 text-xs',
        message.type === 'success'
          ? 'border-success/25 bg-success/10 text-success'
          : 'border-destructive/25 bg-destructive/10 text-destructive'
      )}
    >
      {message.text}
    </div>
  )
}

interface ConfigPageProps {
  page?: string
}

export default function ConfigPage({ page = 'general' }: ConfigPageProps) {
  const [cfg, setCfg] = useState<Record<string, unknown> | null>(null)
  const [original, setOriginal] = useState<Record<string, unknown> | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)
  const [u115Oauth, setU115Oauth] = useState<{ authorized: boolean; token_exists: boolean } | null>(null)
  const [u115OauthMessage, setU115OauthMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [u115CookieMessage, setU115CookieMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [u115QrUrl, setU115QrUrl] = useState('')
  const [u115QrPreviewUrl, setU115QrPreviewUrl] = useState('')
  const [u115AuthBusy, setU115AuthBusy] = useState(false)
  const [u115CookieTestBusy, setU115CookieTestBusy] = useState(false)
  const [u115Polling, setU115Polling] = useState(false)
  const u115PollAbortRef = useRef<AbortController | null>(null)
  const [fullRefreshing, setFullRefreshing] = useState(false)
  const [fullRefreshMessage, setFullRefreshMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [mediavaultTestBusy, setMediavaultTestBusy] = useState(false)
  const [mediavaultMessage, setMediavaultMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const loadU115OauthStatus = useCallback(async () => {
    try {
      const res = await getU115OauthStatus()
      setU115Oauth(res.data)
    } catch {
      setU115Oauth(null)
    }
  }, [])

  useEffect(() => {
    setLoading(true)
    const loader = page === 'filenameRules' ? getParserRulesConfig : getMainConfig
    loader()
      .then((r) => {
        const source = page === 'filenameRules' ? { parser: r.data || {} } : r.data
        const normalized = normalizeConfig(source as Record<string, unknown>)
        setCfg(normalized)
        setOriginal(normalized)
      })
      .catch((e) => setError((e as { response?: { data?: { detail?: string } } })?.response?.data?.detail || e.message))
      .finally(() => setLoading(false))
    if (page !== 'filenameRules') {
      loadU115OauthStatus()
    }
  }, [loadU115OauthStatus, page])

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
        setU115OauthMessage({ type: 'error', text: '115 扫码授权已超时，请重新发起扫码。' })
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
            setU115OauthMessage({ type: 'success', text: '115 授权成功，token 已写入本地。' })
            finishPolling()
            loadU115OauthStatus()
          } else {
            setU115OauthMessage({ type: 'error', text: data.message || '115 换取 token 失败' })
            finishPolling()
          }
          return
        }
        const statusCode = Number(data.status)
        const msg = String(data.message || '')
        if (Number.isFinite(statusCode) && statusCode < 0) {
          setU115OauthMessage({ type: 'error', text: msg || `115 扫码授权已失效：${data.status}` })
          finishPolling()
          return
        }
        // 检查 message 是否包含错误关键词
        if (msg && (msg.includes('失败') || msg.includes('错误') || msg.includes('拒绝') || msg.includes('取消'))) {
          setU115OauthMessage({ type: 'error', text: `115 扫码失败：${msg}` })
          finishPolling()
          return
        }
        setU115OauthMessage({
          type: 'success',
          text: statusCode >= 1 ? '已扫码，等待手机确认…' : `等待扫码…`,
        })
        scheduleNextPoll()
      } catch (e) {
        if (cancelled) return
        if ((e as { code?: string; name?: string })?.code === 'ERR_CANCELED' || (e as { name?: string })?.name === 'CanceledError') return
        const status = (e as { response?: { status?: number } })?.response?.status
        if (status === 502 || status === 503 || status === 504 || !status) {
          setU115OauthMessage({ type: 'error', text: '115 扫码状态查询暂时超时，系统会继续自动重试…' })
          scheduleNextPoll()
          return
        }
        setU115Polling(false)
        setU115OauthMessage({ type: 'error', text: (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail || (e as Error).message || '查询 115 扫码状态失败' })
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
  }, [u115Polling, u115QrUrl, loadU115OauthStatus])

  const isDirty = JSON.stringify(cfg) !== JSON.stringify(original)
  const pageMeta = PAGE_VARIANTS[page as keyof typeof PAGE_VARIANTS] || PAGE_VARIANTS.general
  const isFilenameRulesPage = page === 'filenameRules'

  const set = useCallback((section: string, key: string, val: unknown) => {
    setCfg((prev) => ({
      ...prev,
      [section]: { ...(prev?.[section] as Record<string, unknown> || {}), [key]: val },
    }))
  }, [])

  async function handleSave() {
    setSaving(true)
    setError(null)
    setSaved(false)
    try {
      const normalized = normalizeConfig(cfg || {})
      if (page === 'filenameRules') {
        await saveParserRulesConfig(normalized?.parser as Record<string, unknown> || normalized)
      } else {
        await saveMainConfig(normalized)
      }
      setCfg(normalized)
      setOriginal(normalized)
      setSaved(true)
      setTimeout(() => setSaved(false), 3000)
    } catch (e) {
      setError((e as { response?: { data?: { detail?: string } } })?.response?.data?.detail || '保存失败')
    } finally {
      setSaving(false)
    }
  }

  async function handleU115CreateQr() {
    const savedU115Config = (original?.u115 as Record<string, unknown>) || {}
    const currentU115Config = (cfg?.u115 as Record<string, unknown>) || {}
    if (
      currentU115Config.client_id !== savedU115Config.client_id ||
      currentU115Config.token_json !== savedU115Config.token_json
    ) {
      setU115OauthMessage({ type: 'error', text: '请先保存 115 Client ID 和 Token 路径后再扫码授权。' })
      return
    }

    setU115AuthBusy(true)
    setU115Polling(false)
    setU115OauthMessage(null)
    try {
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
      setU115OauthMessage({ type: 'success', text: '115 扫码会话已创建，请使用 App 扫码并确认，系统会自动轮询。' })
      loadU115OauthStatus()
    } catch (e) {
      setU115Polling(false)
      setU115OauthMessage({ type: 'error', text: (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail || '创建 115 扫码会话失败' })
    } finally {
      setU115AuthBusy(false)
    }
  }

  async function handleU115ConnectionTest() {
    try {
      const res = await testU115Connection()
      const data = res?.data || {}
      setU115OauthMessage({ type: 'success', text: `连接成功：剩余空间 ${formatBytes(data.remain_space)} / 总空间 ${formatBytes(data.total_space)}` })
      loadU115OauthStatus()
    } catch (e) {
      setU115OauthMessage({ type: 'error', text: (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail || '115 连接测试失败' })
    }
  }

  async function handleU115CookieTest() {
    setU115CookieTestBusy(true)
    setU115CookieMessage(null)
    try {
      const res = await testU115Cookie()
      const data = res?.data || {}
      const remainSpace = data?.space_info?.all_remain?.size
      const totalSpace = data?.space_info?.all_total?.size
      const spaceInfo = remainSpace != null && totalSpace != null
        ? `，剩余空间 ${formatBytes(remainSpace)} / 总空间 ${formatBytes(totalSpace)}`
        : ''
      setU115CookieMessage({ type: 'success', text: `Cookie 可用${spaceInfo}` })
    } catch (e) {
      setU115CookieMessage({ type: 'error', text: (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail || 'Cookie 测试失败' })
    } finally {
      setU115CookieTestBusy(false)
    }
  }

  async function handleFullRefresh() {
    setFullRefreshing(true)
    setFullRefreshMessage(null)
    try {
      const res = await refreshLibrary(true)
      const data = res?.data || {}
      setFullRefreshMessage({
        type: 'success',
        text: `全量刷新完成：电影 ${data.total_movies ?? '-'} 部，剧集 ${data.total_tv ?? '-'} 部`,
      })
    } catch (e) {
      setFullRefreshMessage({
        type: 'error',
        text: (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail || '全量刷新失败',
      })
    } finally {
      setFullRefreshing(false)
    }
  }

  async function handleMediaVaultTest() {
    const mediavault = (cfg?.mediavault as Record<string, unknown>) || {}
    setMediavaultTestBusy(true)
    setMediavaultMessage(null)
    try {
      const res = await testMediaVault({
        enabled: mediavault.enabled,
        scheme: mediavault.scheme,
        host: mediavault.host,
        api_key: mediavault.api_key,
        timeout: mediavault.timeout,
      })
      const data = res?.data || {}
      setMediavaultMessage({ type: 'success', text: data.message || 'MediaVault 测试成功' })
    } catch (e) {
      setMediavaultMessage({ type: 'error', text: (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail || 'MediaVault 测试失败' })
    } finally {
      setMediavaultTestBusy(false)
    }
  }

  function formatBytes(bytes: number): string {
    const value = Number(bytes)
    if (!Number.isFinite(value) || value < 0) return '-'
    const GB = 1024 ** 3
    const TB = 1024 ** 4
    if (value < 5 * TB) return `${(value / GB).toFixed(2)} GB`
    return `${(value / TB).toFixed(2)} TB`
  }

  if (loading) {
    return (
      <div className="flex-1 space-y-4">
        {[1, 2, 3].map((i) => <SkeletonPanel key={i} compact rows={3} />)}
      </div>
    )
  }

  return (
    <div className="flex min-w-0 flex-1 flex-col">
      <PageHeader
        title={pageMeta.title}
        description={pageMeta.description}
        actions={
          <>
            {isDirty && !saving && (
              <span className="flex items-center gap-1.5 rounded-full bg-warning/12 px-2.5 py-1 text-xs text-warning">
                <AlertTriangle className="size-3" />
                未保存
              </span>
            )}
            {saved && (
              <span className="animate-stagger-in rounded-full bg-success/12 px-2.5 py-1 text-xs text-success">✓ 已保存</span>
            )}
            <Button
              variant="outline"
              size="sm"
              onClick={() => { setCfg(original); setError(null) }}
              disabled={!isDirty || saving}
              className="rounded-full"
            >
              <RotateCcw className="size-4" />
              重置
            </Button>
            <Button size="sm" onClick={handleSave} disabled={!isDirty || saving} className="rounded-full px-4">
              {saving ? <RefreshCw className="size-4 animate-spin" /> : <Save className="size-4" />}
              {saving ? '保存中…' : '保存'}
            </Button>
          </>
        }
      />

      {error && (
        <div className="mb-5 rounded-xl border border-destructive/25 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {isFilenameRulesPage ? (
        <Tabs defaultValue="words" className="w-full">
          <div className="mb-6 overflow-x-auto">
            <TabsList className="h-auto gap-1 bg-card/40 p-1">
              {PARSER_TABS.map((tab) => {
                const Icon = tab.icon
                const wordsCount = ((cfg?.parser as Record<string, unknown>)?.custom_words as string[] | undefined)?.length ?? 0
                const groupsCount = ((cfg?.parser as Record<string, unknown>)?.custom_release_groups as string[] | undefined)?.length ?? 0
                const count = tab.key === 'words' ? wordsCount : tab.key === 'groups' ? groupsCount : null
                return (
                  <TabsTrigger
                    key={tab.key}
                    value={tab.key}
                    className="gap-1.5 px-3 py-2 text-[13px]"
                  >
                    <Icon className="size-4" />
                    {tab.label}
                    {count !== null && (
                      <Badge variant="secondary" className="ml-1 h-5 px-1.5 text-[11px]">
                        {count}
                      </Badge>
                    )}
                  </TabsTrigger>
                )
              })}
            </TabsList>
          </div>

          <TabsContent value="words" className="mt-0">
            <Card className="border-border/70 bg-card/40">
              <CardHeader>
                <CardTitle className="text-base">自定义识别词</CardTitle>
                <CardDescription>
                  一行一个规则，保存后写入 parser-rules.yaml
                </CardDescription>
              </CardHeader>
              <CardContent>
                <MultilineRulesField
                  value={((cfg?.parser as Record<string, unknown>)?.custom_words as string[] | undefined) ?? []}
                  onChange={(v) => set('parser', 'custom_words', v)}
                  placeholder={'国语配音\nOVA => SP\n第 <> 集 >> EP-1'}
                />
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="groups" className="mt-0">
            <Card className="border-border/70 bg-card/40">
              <CardHeader>
                <CardTitle className="text-base">自定义字幕组</CardTitle>
                <CardDescription>
                  补充内置列表没有的字幕组名称，每个字幕组显示为独立胶囊
                </CardDescription>
              </CardHeader>
              <CardContent>
                <ListField
                  value={((cfg?.parser as Record<string, unknown>)?.custom_release_groups as string[] | undefined) ?? []}
                  onChange={(v) => set('parser', 'custom_release_groups', v)}
                />
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="help" className="mt-0">
            <Card className="border-border/70 bg-card/40">
              <CardHeader>
                <CardTitle className="text-base">规则格式说明</CardTitle>
                <CardDescription>
                  支持屏蔽、替换、偏移和组合四种规则类型
                </CardDescription>
              </CardHeader>
              <CardContent>
                <CustomWordsHelp defaultOpen />
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      ) : (
        <Tabs defaultValue="storage" className="w-full">
          <div className="mb-6 overflow-x-auto">
            <TabsList className="h-auto gap-1 bg-card/40 p-1">
              {CONFIG_TABS.map((tab) => {
                const Icon = tab.icon
                return (
                  <TabsTrigger
                    key={tab.key}
                    value={tab.key}
                    className="gap-1.5 px-3 py-2 text-[13px]"
                  >
                    <Icon className="size-4" />
                    {tab.label}
                  </TabsTrigger>
                )
              })}
            </TabsList>
          </div>

          <TabsContent value="storage" className="mt-0">
            <Card className="border-border/70 bg-card/40">
              <CardHeader>
                <CardTitle className="text-base">云存储选择</CardTitle>
                <CardDescription>选择媒体文件存放在哪个网盘</CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                <FieldGroup>
                  <Field label="主存储">
                    <Select
                      value={((cfg?.storage as Record<string, unknown>)?.primary as string) || 'pan115'}
                      onValueChange={(v) => set('storage', 'primary', v)}
                    >
                      <SelectTrigger className="w-full sm:w-48 focus:ring-0 focus:ring-offset-0">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="pan115">115 网盘</SelectItem>
                      </SelectContent>
                    </Select>
                  </Field>
                </FieldGroup>

                <Separator className="opacity-60" />

                <div>
                  <div className="mb-3 text-[13px] font-medium">媒体库全量刷新</div>
                  <p className="mb-3 text-xs leading-5 text-muted-foreground">
                    全量扫描所有文件并重建媒体库。日常使用无需操作，仅在增量刷新出现数据不一致时使用。
                  </p>
                  <Button
                    variant="destructive"
                    size="lg"
                    onClick={handleFullRefresh}
                    disabled={fullRefreshing}
                  >
                    <RefreshCw className={cn('size-5', fullRefreshing && 'animate-spin')} />
                    {fullRefreshing ? '刷新中…' : '全量刷新媒体库'}
                  </Button>
                  <NoticeMessage message={fullRefreshMessage} />
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="auth" className="mt-0">
            <Card className="border-border/70 bg-card/40">
              <CardHeader>
                <CardTitle className="text-base">WebUI 认证</CardTitle>
                <CardDescription>登录凭据与会话设置</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field label="用户名" description="默认 admin">
                    <TextInput
                      value={(cfg?.webui as Record<string, unknown>)?.username as string | undefined}
                      onChange={(v) => set('webui', 'username', v)}
                      placeholder="输入登录用户名"
                    />
                  </Field>
                  <Field label="密码" description="留空则无认证">
                    <TextInput
                      value={(cfg?.webui as Record<string, unknown>)?.password as string | undefined}
                      onChange={(v) => set('webui', 'password', v)}
                      type="password"
                      placeholder="设置强密码"
                    />
                  </Field>
                  <Field label="Token 有效期" description="小时">
                    <NumberInput
                      value={(cfg?.webui as Record<string, unknown>)?.token_expire_hours as number | undefined}
                      onChange={(v) => set('webui', 'token_expire_hours', v)}
                      min={1}
                      max={8760}
                    />
                  </Field>
                  <Field label="日志保留" description="天">
                    <NumberInput
                      value={(cfg?.webui as Record<string, unknown>)?.log_retention_days as number | undefined}
                      onChange={(v) => set('webui', 'log_retention_days', v)}
                      min={1}
                      max={365}
                    />
                  </Field>
                  <Field label="Webhook 密钥" description="/trigger 端点校验" fullWidth>
                    <TextInput
                      value={(cfg?.webui as Record<string, unknown>)?.webhook_secret as string | undefined}
                      onChange={(v) => set('webui', 'webhook_secret', v)}
                      type="password"
                      placeholder="留空则不校验"
                      mono
                    />
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="tmdb" className="mt-0">
            <Card className="border-border/70 bg-card/40">
              <CardHeader>
                <CardTitle className="text-base">TMDB 设置</CardTitle>
                <CardDescription>元数据源配置</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field label="API Key" description="TMDB v3 必填" fullWidth>
                    <TextInput
                      value={(cfg?.tmdb as Record<string, unknown>)?.api_key as string | undefined}
                      onChange={(v) => set('tmdb', 'api_key', v)}
                      type="password"
                      placeholder="your-tmdb-api-key"
                      mono
                    />
                  </Field>
                  <Field label="返回语言" description="如 zh-CN">
                    <TextInput
                      value={(cfg?.tmdb as Record<string, unknown>)?.language as string | undefined}
                      onChange={(v) => set('tmdb', 'language', v)}
                      placeholder="zh-CN"
                    />
                  </Field>
                  <Field label="请求超时" description="秒">
                    <NumberInput
                      value={(cfg?.tmdb as Record<string, unknown>)?.timeout as number | undefined}
                      onChange={(v) => set('tmdb', 'timeout', v)}
                      min={1}
                      max={120}
                    />
                  </Field>
                  <Field label="HTTP 代理" description="可选" fullWidth>
                    <TextInput
                      value={(cfg?.tmdb as Record<string, unknown>)?.proxy as string | undefined}
                      onChange={(v) => set('tmdb', 'proxy', v)}
                      placeholder="http://proxy-host:port"
                      mono
                    />
                  </Field>
                  <Field label="TMDB 图片代理域名" description="可选，仅填根域名" fullWidth>
                    <TextInput
                      value={(cfg?.tmdb as Record<string, unknown>)?.image_base_url as string | undefined}
                      onChange={(v) => set('tmdb', 'image_base_url', v)}
                      placeholder="https://image-proxy.example.com"
                      mono
                    />
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="u115" className="mt-0">
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
                      disabled={u115AuthBusy || !(cfg?.u115 as Record<string, unknown>)?.client_id}
                    >
                      {u115AuthBusy ? '处理中…' : (u115Polling ? '等待扫码…' : '扫码授权')}
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
                  <NoticeMessage message={u115OauthMessage} />
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
                  <Field label="共享请求 QPS" description="整理/播放/下载全项目共用，调低更防风控">
                    <NumberInput
                      value={(cfg?.u115 as Record<string, unknown>)?.qps as number | undefined}
                      onChange={(v) => set('u115', 'qps', v)}
                      min={1}
                      max={10}
                    />
                  </Field>
                  <Field label="媒体库根目录 ID">
                    <TextInput
                      value={(cfg?.u115 as Record<string, unknown>)?.root_folder_id as string | undefined}
                      onChange={(v) => set('u115', 'root_folder_id', v)}
                      placeholder="115 目录 ID"
                      mono
                    />
                  </Field>
                  <Field label="云下载目录 ID" description="离线保存位置">
                    <TextInput
                      value={(cfg?.u115 as Record<string, unknown>)?.download_folder_id as string | undefined}
                      onChange={(v) => set('u115', 'download_folder_id', v)}
                      placeholder="115 云下载目录 ID"
                      mono
                    />
                  </Field>
                  <Field label="电影归档目录" description="留空同根目录">
                    <TextInput
                      value={(cfg?.u115 as Record<string, unknown>)?.movie_root_id as string | undefined}
                      onChange={(v) => set('u115', 'movie_root_id', v)}
                      placeholder="留空同根目录"
                      mono
                    />
                  </Field>
                  <Field label="剧集归档目录" description="留空同根目录">
                    <TextInput
                      value={(cfg?.u115 as Record<string, unknown>)?.tv_root_id as string | undefined}
                      onChange={(v) => set('u115', 'tv_root_id', v)}
                      placeholder="留空同根目录"
                      mono
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
                    <Field label="轮询间隔" description="秒">
                      <NumberInput
                        value={(cfg?.u115 as Record<string, unknown>)?.auto_organize_poll_seconds as number | undefined ?? 45}
                        onChange={(v) => set('u115', 'auto_organize_poll_seconds', v)}
                        min={10}
                        max={3600}
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
                      disabled={u115CookieTestBusy || !(cfg?.u115 as Record<string, unknown>)?.cookie}
                    >
                      {u115CookieTestBusy ? '测试中…' : '测试'}
                    </Button>
                    {u115CookieMessage && (
                      <span className={cn('text-xs', u115CookieMessage.type === 'success' ? 'text-success' : 'text-destructive')}>
                        {u115CookieMessage.text}
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
          </TabsContent>

          <TabsContent value="rss" className="mt-0">
            <Card className="border-border/70 bg-card/40">
              <CardHeader>
                <CardTitle className="text-base">RSS 订阅</CardTitle>
                <CardDescription>后台轮询设置</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field label="轮询间隔" description="检查订阅频率（秒）">
                    <NumberInput
                      value={(cfg?.rss as Record<string, unknown>)?.poll_seconds as number | undefined ?? 300}
                      onChange={(v) => set('rss', 'poll_seconds', v)}
                      min={10}
                      max={3600}
                    />
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="telegram" className="mt-0">
            <Card className="border-border/70 bg-card/40">
              <CardHeader>
                <CardTitle className="text-base">Telegram 通知</CardTitle>
                <CardDescription>入库完成推送</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field label="Bot Token" description="@BotFather 获取" fullWidth>
                    <TextInput
                      value={(cfg?.telegram as Record<string, unknown>)?.bot_token as string | undefined}
                      onChange={(v) => set('telegram', 'bot_token', v)}
                      type="password"
                      placeholder="your-bot-token"
                      mono
                    />
                  </Field>
                  <Field label="Chat ID" description="接收通知的账号">
                    <TextInput
                      value={(cfg?.telegram as Record<string, unknown>)?.chat_id as string | undefined}
                      onChange={(v) => set('telegram', 'chat_id', v)}
                      placeholder="your-chat-id"
                      mono
                    />
                  </Field>
                  <Field label="防抖延时" description="批量合并通知（秒）">
                    <NumberInput
                      value={(cfg?.telegram as Record<string, unknown>)?.debounce_seconds as number | undefined}
                      onChange={(v) => set('telegram', 'debounce_seconds', v)}
                      min={0}
                      max={600}
                    />
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="mediavault" className="mt-0">
            <Card className="border-border/70 bg-card/40">
              <CardHeader>
                <CardTitle className="text-base">MediaVault</CardTitle>
                <CardDescription>整理完成后触发目录监控</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="mb-6">
                  <div className="mb-3 flex items-center gap-3">
                    <span className="text-[13px] font-medium">连接测试</span>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={handleMediaVaultTest}
                      disabled={mediavaultTestBusy || !(cfg?.mediavault as Record<string, unknown>)?.host || !(cfg?.mediavault as Record<string, unknown>)?.api_key}
                    >
                      {mediavaultTestBusy ? '测试中…' : '测试'}
                    </Button>
                    {mediavaultMessage && (
                      <span className={cn('text-xs', mediavaultMessage.type === 'success' ? 'text-success' : 'text-destructive')}>
                        {mediavaultMessage.text}
                      </span>
                    )}
                  </div>
                </div>

                <FieldGroup>
                  <Field label="启用 MediaVault">
                    <Toggle
                      value={(cfg?.mediavault as Record<string, unknown>)?.enabled as boolean | undefined}
                      onChange={(v) => set('mediavault', 'enabled', v)}
                    />
                  </Field>
                  <Field label="请求超时" description="秒">
                    <NumberInput
                      value={(cfg?.mediavault as Record<string, unknown>)?.timeout as number | undefined ?? 10}
                      onChange={(v) => set('mediavault', 'timeout', v)}
                      min={1}
                      max={120}
                    />
                  </Field>
                  <Field label="协议" description="地址不需要带协议">
                    <Select
                      value={(cfg?.mediavault as Record<string, unknown>)?.scheme as string | undefined ?? 'http'}
                      onValueChange={(v) => set('mediavault', 'scheme', v)}
                    >
                      <SelectTrigger>
                        <SelectValue placeholder="选择协议" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="http">http</SelectItem>
                        <SelectItem value="https">https</SelectItem>
                      </SelectContent>
                    </Select>
                  </Field>
                  <Field label="地址" description="只填主机或主机:端口">
                    <TextInput
                      value={(cfg?.mediavault as Record<string, unknown>)?.host as string | undefined}
                      onChange={(v) => set('mediavault', 'host', v)}
                      placeholder="mediavault:7811"
                      mono
                    />
                  </Field>
                  <Field label="API Key" description="MediaVault 系统配置中的 API Key" fullWidth>
                    <TextInput
                      value={(cfg?.mediavault as Record<string, unknown>)?.api_key as string | undefined}
                      onChange={(v) => set('mediavault', 'api_key', v)}
                      type="password"
                      placeholder="your-api-key"
                      mono
                    />
                  </Field>
                  <Field label="source_dir" description="留空则使用 MediaVault 默认目录" fullWidth>
                    <TextInput
                      value={(cfg?.mediavault as Record<string, unknown>)?.source_dir as string | undefined}
                      onChange={(v) => set('mediavault', 'source_dir', v)}
                      placeholder="/path/to/watch"
                      mono
                    />
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="pipeline" className="mt-0">
            <Card className="border-border/70 bg-card/40">
              <CardHeader>
                <CardTitle className="text-base">扫描与入库策略</CardTitle>
                <CardDescription>整理行为控制</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field label="跳过 TMDB 查询" description="只整理文件夹">
                    <Toggle
                      value={(cfg?.pipeline as Record<string, unknown>)?.skip_tmdb as boolean | undefined}
                      onChange={(v) => set('pipeline', 'skip_tmdb', v)}
                    />
                  </Field>
                  <Field label="TMDB 未找到仍移动" description="否则跳过">
                    <Toggle
                      value={(cfg?.pipeline as Record<string, unknown>)?.move_on_tmdb_miss as boolean | undefined}
                      onChange={(v) => set('pipeline', 'move_on_tmdb_miss', v)}
                    />
                  </Field>
                  <Field label="Dry Run 模式" description="只打印计划">
                    <Toggle
                      value={(cfg?.pipeline as Record<string, unknown>)?.dry_run as boolean | undefined}
                      onChange={(v) => set('pipeline', 'dry_run', v)}
                    />
                  </Field>
                  <Field label="替换同名视频" description="先移除再整理">
                    <Toggle
                      value={(cfg?.pipeline as Record<string, unknown>)?.replace_existing_video === true}
                      onChange={(v) => set('pipeline', 'replace_existing_video', v)}
                    />
                  </Field>
                  <Field label="跳过元数据上传" description="关闭时上传全部 NFO 和图片；开启时只移动媒体文件，不上传任何 NFO 或图片">
                    <Toggle
                      value={(cfg?.pipeline as Record<string, unknown>)?.skip_metadata_upload === true}
                      onChange={(v) => set('pipeline', 'skip_metadata_upload', v)}
                    />
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="subtitle" className="mt-0">
            <Card className="border-border/70 bg-card/40">
              <CardHeader>
                <CardTitle className="text-base">字幕整理</CardTitle>
                <CardDescription>外挂字幕文件自动跟随视频整理</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field label="启用字幕整理" description="自动移动外挂字幕">
                    <Toggle
                      value={(cfg?.subtitle as Record<string, unknown>)?.enabled !== false}
                      onChange={(v) => set('subtitle', 'enabled', v)}
                    />
                  </Field>
                </FieldGroup>
                <div className="mt-4 text-xs leading-6 text-muted-foreground">
                  <p>支持的字幕格式：<code className="rounded bg-muted px-1 font-mono">.srt</code> <code className="rounded bg-muted px-1 font-mono">.ass</code> <code className="rounded bg-muted px-1 font-mono">.ssa</code></p>
                  <p className="mt-1">语言代码标准化：简体中文 → <code className="rounded bg-muted px-1 font-mono">zh-CN</code>，繁体中文 → <code className="rounded bg-muted px-1 font-mono">zh-TW</code></p>
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="strm" className="mt-0">
            <Card className="border-border/70 bg-card/40">
              <CardHeader>
                <CardTitle className="text-base">STRM 生成与 302 播放</CardTitle>
                <CardDescription>为网盘媒体库生成本地 .strm 文件，播放时 302 直链串流，无需下载到本地</CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                <FieldGroup>
                  <Field label="启用 STRM" description="整理入库成功后自动生成">
                    <Toggle
                      value={((cfg?.strm as Record<string, unknown>)?.enabled as boolean) || false}
                      onChange={(v) => set('strm', 'enabled', v)}
                    />
                  </Field>
                  <Field label="定时同步间隔（秒）" description="0 仅手动/入库触发；≥60 开启定时增量">
                    <NumberInput
                      value={(cfg?.strm as Record<string, unknown>)?.sync_interval_seconds as number | undefined}
                      onChange={(v) => set('strm', 'sync_interval_seconds', v)}
                      min={0}
                      max={86400}
                    />
                  </Field>
                  <Field label="服务地址 (url_base)" description="写入 .strm 的地址，支持 IP:端口或域名（含反代/HTTPS）" fullWidth>
                    <TextInput
                      value={(cfg?.strm as Record<string, unknown>)?.url_base as string | undefined}
                      onChange={(v) => set('strm', 'url_base', v)}
                      placeholder="http://192.168.1.10:38765 或 https://strm.example.com"
                      mono
                    />
                  </Field>
                  <Field label="访问签名 (url_secret)" description="可选，防止未授权调用 302 直链；修改后需重新同步" fullWidth>
                    <div className="flex gap-2">
                      <Input
                        value={((cfg?.strm as Record<string, unknown>)?.url_secret as string) ?? ''}
                        onChange={(e) => set('strm', 'url_secret', e.target.value)}
                        placeholder="留空不启用签名校验"
                        className="font-mono text-[13px]"
                      />
                      <Button
                        variant="outline"
                        size="sm"
                        className="shrink-0"
                        title="生成 128 位随机密钥"
                        onClick={() => set('strm', 'url_secret', randomSecret())}
                      >
                        <Dices className="size-4" />
                        随机
                      </Button>
                    </div>
                  </Field>
                </FieldGroup>

                <div>
                  <div className="mb-2 flex items-baseline gap-2">
                    <label className="text-[13px] font-medium">路径映射</label>
                    <span className="text-[11px] text-muted-foreground/80">网盘媒体库目录 ID → 本地 STRM 输出目录，目录结构镜像网盘</span>
                  </div>
                  <div className="space-y-2">
                    {(((cfg?.strm as Record<string, unknown>)?.mappings as Array<Record<string, unknown>>) || []).map((m, i) => (
                      <div key={i} className="flex flex-col gap-2 sm:flex-row sm:items-center">
                        <Input
                          value={(m.pan_folder_id as string) ?? ''}
                          onChange={(e) => {
                            const list = [...(((cfg?.strm as Record<string, unknown>)?.mappings as Array<Record<string, unknown>>) || [])]
                            list[i] = { ...list[i], pan_folder_id: e.target.value }
                            set('strm', 'mappings', list)
                          }}
                          placeholder="网盘目录 ID（如媒体库根目录）"
                          className="font-mono text-[13px]"
                        />
                        <Input
                          value={(m.local_path as string) ?? ''}
                          onChange={(e) => {
                            const list = [...(((cfg?.strm as Record<string, unknown>)?.mappings as Array<Record<string, unknown>>) || [])]
                            list[i] = { ...list[i], local_path: e.target.value }
                            set('strm', 'mappings', list)
                          }}
                          placeholder="/mnt/media/115/movies"
                          className="font-mono text-[13px]"
                        />
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          className="text-muted-foreground hover:bg-destructive/15 hover:text-destructive"
                          onClick={() => {
                            const list = [...(((cfg?.strm as Record<string, unknown>)?.mappings as Array<Record<string, unknown>>) || [])]
                            list.splice(i, 1)
                            set('strm', 'mappings', list)
                          }}
                        >
                          <X className="size-3.5" />
                        </Button>
                      </div>
                    ))}
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        const list = [...(((cfg?.strm as Record<string, unknown>)?.mappings as Array<Record<string, unknown>>) || []), { pan_folder_id: '', local_path: '' }]
                        set('strm', 'mappings', list)
                      }}
                    >
                      <Plus className="size-4" />
                      添加映射
                    </Button>
                  </div>
                </div>

                <FieldGroup>
                  <Field label="媒体扩展名" description="这些扩展名的网盘文件会生成 STRM" fullWidth>
                    <ListField
                      value={((cfg?.strm as Record<string, unknown>)?.media_ext as string[]) || []}
                      onChange={(v) => set('strm', 'media_ext', v)}
                    />
                  </Field>
                </FieldGroup>

                <FieldGroup>
                  <Field label="下载伴随文件" description="外挂字幕/元数据/图片下载到本地，否则 STRM 播放无法加载外挂字幕">
                    <Toggle
                      value={((cfg?.strm as Record<string, unknown>)?.fetch_sidecar as boolean) || false}
                      onChange={(v) => set('strm', 'fetch_sidecar', v)}
                    />
                  </Field>
                  <Field label="伴随文件扩展名" description="单文件上限 20MB，与 .strm 同目录存放" fullWidth>
                    <ListField
                      value={((cfg?.strm as Record<string, unknown>)?.sidecar_ext as string[]) || []}
                      onChange={(v) => set('strm', 'sidecar_ext', v)}
                    />
                  </Field>
                </FieldGroup>

                <div className="text-xs leading-6 text-muted-foreground">
                  <p>.strm 内容形如 <code className="rounded bg-muted px-1 font-mono">http://服务地址/strm/redirect?pickcode=...</code>，播放时实时向 115 换取直链（直链与播放器 User-Agent 绑定，服务端自动透传并缓存）。</p>
                  <p className="mt-1">触发方式：整理入库成功后自动生成 + 定时增量同步 + 本页保存后到「STRM 同步」页手动触发。将本地 STRM 目录作为媒体库加入 Emby / Jellyfin / Infuse 即可播放。</p>
                </div>
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      )}
    </div>
  )
}
