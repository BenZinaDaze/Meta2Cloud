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
  Download,
  HardDrive,
  Rss,
  Bell,
  FolderSync,
  FileText,
  Users,
  BookOpen,
  Captions,
} from 'lucide-react'
import {
  getMainConfig,
  saveMainConfig,
  getParserRulesConfig,
  saveParserRulesConfig,
  getDriveOauthStatus,
  testDriveConnection,
  getU115OauthStatus,
  createU115OauthSession,
  testU115Connection,
  testU115Cookie,
  fetchU115QrCode,
  pollU115OauthStatus,
  refreshLibraryFull,
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
  { key: 'aria2', label: '下载', icon: Download },
  { key: 'drive', label: 'Drive', icon: HardDrive },
  { key: 'u115', label: '115', icon: HardDrive },
  { key: 'rss', label: 'RSS', icon: Rss },
  { key: 'telegram', label: '通知', icon: Bell },
  { key: 'pipeline', label: '策略', icon: FolderSync },
  { key: 'subtitle', label: '字幕', icon: Captions },
]

function normalizeConfig(data: Record<string, unknown>): Record<string, unknown> {
  const next = structuredClone(data || {})
  const aria2 = { ...(next.aria2 as Record<string, unknown> || {}) }
  const webui = { ...(next.webui as Record<string, unknown> || {}) }
  const tmdb = { ...(next.tmdb as Record<string, unknown> || {}) }
  const u115 = { ...(next.u115 as Record<string, unknown> || {}) }
  const rss = { ...(next.rss as Record<string, unknown> || {}) }
  const telegram = { ...(next.telegram as Record<string, unknown> || {}) }

  if (aria2.enabled === undefined) {
    aria2.enabled = aria2.auto_connect !== false
  }
  delete aria2.auto_connect

  if (webui.token_expire_hours === undefined || webui.token_expire_hours === null || webui.token_expire_hours === '') {
    webui.token_expire_hours = 24
  }
  if (webui.log_retention_days === undefined || webui.log_retention_days === null || webui.log_retention_days === '') {
    webui.log_retention_days = 7
  }
  if (tmdb.timeout === undefined || tmdb.timeout === null || tmdb.timeout === '') {
    tmdb.timeout = 10
  }
  if (u115.auto_organize_poll_seconds === undefined || u115.auto_organize_poll_seconds === null || u115.auto_organize_poll_seconds === '') {
    u115.auto_organize_poll_seconds = 45
  }
  if (u115.auto_organize_stable_seconds === undefined || u115.auto_organize_stable_seconds === null || u115.auto_organize_stable_seconds === '') {
    u115.auto_organize_stable_seconds = 30
  }
  if (rss.poll_seconds === undefined || rss.poll_seconds === null || rss.poll_seconds === '') {
    rss.poll_seconds = 300
  }
  if (telegram.debounce_seconds === undefined || telegram.debounce_seconds === null || telegram.debounce_seconds === '') {
    telegram.debounce_seconds = 60
  }

  next.aria2 = aria2
  next.webui = webui
  next.tmdb = tmdb
  next.u115 = u115
  next.rss = rss
  next.telegram = telegram
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
      <div className="mb-1.5 flex items-center gap-2">
        <label className="text-[13px] font-medium">{label}</label>
        {description && (
          <span className="text-[11px] text-muted-foreground">{description}</span>
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
      className="w-24"
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
              className="inline-flex max-w-full items-center gap-2 rounded-xl border bg-muted/50 px-3 py-2"
            >
              <span className="truncate text-xs font-mono">{item}</span>
              <Button
                variant="ghost"
                size="icon-xs"
                onClick={() => onChange(value.filter((_, j) => j !== i))}
                className="text-destructive"
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
          onKeyDown={(e) => e.key === 'Enter' && add()}
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
  const text = Array.isArray(value) ? value.join('\n') : ''
  const lineCount = text === '' ? 1 : text.split('\n').length

  function commit(nextText: string) {
    const next = nextText.split('\n').map((line) => line.trim()).filter(Boolean)
    onChange(next)
  }

  return (
    <div className="space-y-2">
      <Textarea
        value={text}
        onChange={(e) => commit(e.target.value)}
        placeholder={placeholder}
        spellCheck={false}
        className="font-mono resize-none"
        rows={Math.max(8, Math.min(18, lineCount + 2))}
      />
      <div className="text-xs text-muted-foreground">
        一行一个规则，空行会自动忽略。当前 {value.length} 条。
      </div>
    </div>
  )
}

interface ConfigPageProps {
  onAria2EnabledChange?: (enabled: boolean) => void
  page?: string
}

export default function ConfigPage({ onAria2EnabledChange, page = 'general' }: ConfigPageProps) {
  const [cfg, setCfg] = useState<Record<string, unknown> | null>(null)
  const [original, setOriginal] = useState<Record<string, unknown> | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)
  const [driveOauth, setDriveOauth] = useState<{ authorized: boolean; credentials_exists: boolean; token_exists: boolean } | null>(null)
  const [driveOauthMessage, setDriveOauthMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [driveTestBusy, setDriveTestBusy] = useState(false)
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

  const loadDriveOauthStatus = useCallback(async () => {
    try {
      const res = await getDriveOauthStatus()
      setDriveOauth(res.data)
    } catch {
      setDriveOauth(null)
    }
  }, [])

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
      loadDriveOauthStatus()
      loadU115OauthStatus()
    }
  }, [loadDriveOauthStatus, loadU115OauthStatus, page])

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
      if (page !== 'filenameRules' && onAria2EnabledChange) {
        const aria2Config = normalized?.aria2 as Record<string, unknown> | undefined
        onAria2EnabledChange(aria2Config?.enabled !== false)
      }
      setSaved(true)
      setTimeout(() => setSaved(false), 3000)
    } catch (e) {
      setError((e as { response?: { data?: { detail?: string } } })?.response?.data?.detail || '保存失败')
    } finally {
      setSaving(false)
    }
  }

  async function handleDriveTest() {
    setDriveTestBusy(true)
    setDriveOauthMessage(null)
    try {
      const res = await testDriveConnection()
      const data = res?.data || {}
      const identity = data.email || data.display_name || '当前账号'
      setDriveOauthMessage({ type: 'success', text: `连接成功：${identity}` })
      loadDriveOauthStatus()
    } catch (e) {
      setDriveOauthMessage({ type: 'error', text: (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail || 'Drive 连接测试失败' })
    } finally {
      setDriveTestBusy(false)
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
      const res = await refreshLibraryFull()
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
    <div className="flex flex-1 flex-col min-w-0">
      <div className="mb-6 flex flex-wrap items-center gap-4">
        <div>
          <h1 className="text-xl font-bold">{pageMeta.title}</h1>
          <p className="mt-0.5 text-sm text-muted-foreground">{pageMeta.description}</p>
        </div>
        <div className="ml-auto flex items-center gap-2">
          {isDirty && !saving && (
            <span className="rounded-full bg-warning/20 px-2 py-1 text-xs text-warning">未保存</span>
          )}
          {saved && (
            <span className="rounded-full bg-success/20 px-2 py-1 text-xs text-success">✓ 已保存</span>
          )}
          <Button
            variant="outline"
            size="sm"
            onClick={() => { setCfg(original); setError(null) }}
            disabled={!isDirty || saving}
          >
            <RotateCcw className="size-4" />
            重置
          </Button>
          <Button size="sm" onClick={handleSave} disabled={!isDirty || saving}>
            {saving ? <RefreshCw className="size-4 animate-spin" /> : <Save className="size-4" />}
            {saving ? '保存中…' : '保存'}
          </Button>
        </div>
      </div>

      {error && (
        <div className="mb-5 rounded-lg border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {isFilenameRulesPage ? (
        <Tabs defaultValue="words" className="w-full">
          <div className="mb-6 overflow-x-auto">
            <TabsList className="h-auto gap-1 bg-transparent p-0">
              {PARSER_TABS.map((tab) => {
                const Icon = tab.icon
                const wordsCount = ((cfg?.parser as Record<string, unknown>)?.custom_words as string[] | undefined)?.length ?? 0
                const groupsCount = ((cfg?.parser as Record<string, unknown>)?.custom_release_groups as string[] | undefined)?.length ?? 0
                const count = tab.key === 'words' ? wordsCount : tab.key === 'groups' ? groupsCount : null
                return (
                  <TabsTrigger
                    key={tab.key}
                    value={tab.key}
                    className="gap-1.5 rounded-md border border-transparent px-3 py-2 text-[13px] data-[state=active]:border-border data-[state=active]:bg-card data-[state=active]:shadow-sm"
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
            <Card>
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
            <Card>
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
            <Card>
              <CardHeader>
                <CardTitle className="text-base">规则格式说明</CardTitle>
                <CardDescription>
                  支持屏蔽、替换、偏移和组合四种规则类型
                </CardDescription>
              </CardHeader>
              <CardContent>
                <CustomWordsHelp />
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      ) : (
        <Tabs defaultValue="storage" className="w-full">
          <div className="mb-6 overflow-x-auto">
            <TabsList className="h-auto gap-1 bg-transparent p-0">
              {CONFIG_TABS.map((tab) => {
                const Icon = tab.icon
                return (
                  <TabsTrigger
                    key={tab.key}
                    value={tab.key}
                    className="gap-1.5 rounded-md border border-transparent px-3 py-2 text-[13px] data-[state=active]:border-border data-[state=active]:bg-card data-[state=active]:shadow-sm"
                  >
                    <Icon className="size-4" />
                    {tab.label}
                  </TabsTrigger>
                )
              })}
            </TabsList>
          </div>

          <TabsContent value="storage" className="mt-0">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">云存储选择</CardTitle>
                <CardDescription>选择媒体文件存放在哪个网盘</CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                <FieldGroup>
                  <Field label="主存储">
                    <Select
                      value={((cfg?.storage as Record<string, unknown>)?.primary as string) || 'google_drive'}
                      onValueChange={(v) => set('storage', 'primary', v)}
                    >
                      <SelectTrigger className="w-full sm:w-48 focus:ring-0 focus:ring-offset-0">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="google_drive">Google Drive</SelectItem>
                        <SelectItem value="pan115">115 网盘</SelectItem>
                      </SelectContent>
                    </Select>
                  </Field>
                </FieldGroup>

                <Separator />

                <div>
                  <div className="mb-3 text-[13px] font-medium">媒体库全量刷新</div>
                  <p className="mb-3 text-xs text-muted-foreground">
                    全量扫描所有文件并重建媒体库。日常使用无需操作，仅在增量刷新出现数据不一致时使用。
                  </p>
                  <Button
                    variant="destructive"
                    size="lg"
                    onClick={handleFullRefresh}
                    disabled={fullRefreshing}
                  >
                    <RefreshCw className={`size-5 ${fullRefreshing ? 'animate-spin' : ''}`} />
                    {fullRefreshing ? '刷新中…' : '全量刷新媒体库'}
                  </Button>
                  {fullRefreshMessage && (
                    <div className={`mt-3 rounded-md px-3 py-2 text-xs ${fullRefreshMessage.type === 'success' ? 'bg-success/10 text-success' : 'bg-destructive/10 text-destructive'}`}>
                      {fullRefreshMessage.text}
                    </div>
                  )}
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="auth" className="mt-0">
            <Card>
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
                      placeholder="admin"
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
            <Card>
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
                      placeholder="0ec3b170d4c..."
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
                      placeholder="http://127.0.0.1:7890"
                      mono
                    />
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="aria2" className="mt-0">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Aria2 下载</CardTitle>
                <CardDescription>RPC 连接配置</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field label="启用 Aria2">
                    <Toggle
                      value={(cfg?.aria2 as Record<string, unknown>)?.enabled !== false}
                      onChange={(v) => set('aria2', 'enabled', v)}
                    />
                  </Field>
                  <Field label="RPC 端口">
                    <NumberInput
                      value={(cfg?.aria2 as Record<string, unknown>)?.port as number | undefined}
                      onChange={(v) => set('aria2', 'port', v)}
                      min={1}
                      max={65535}
                    />
                  </Field>
                  <Field label="RPC 主机">
                    <TextInput
                      value={(cfg?.aria2 as Record<string, unknown>)?.host as string | undefined}
                      onChange={(v) => set('aria2', 'host', v)}
                      placeholder="127.0.0.1"
                      mono
                    />
                  </Field>
                  <Field label="RPC 密钥" description="可选">
                    <TextInput
                      value={(cfg?.aria2 as Record<string, unknown>)?.secret as string | undefined}
                      onChange={(v) => set('aria2', 'secret', v)}
                      type="password"
                      placeholder="留空表示无密钥"
                      mono
                    />
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="drive" className="mt-0">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Google Drive</CardTitle>
                <CardDescription>OAuth2 与目录配置</CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                <div>
                  <div className="mb-3 text-[13px] font-medium">授权状态</div>
                  <div className="flex flex-wrap items-center gap-3">
                    <Badge variant={driveOauth?.authorized ? 'success' : 'secondary'}>
                      {driveOauth?.authorized ? '已授权' : '未授权'}
                    </Badge>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={handleDriveTest}
                      disabled={driveTestBusy || driveOauth?.credentials_exists === false}
                    >
                      {driveTestBusy ? '测试中…' : '测试连接'}
                    </Button>
                  </div>
                  {driveOauthMessage && (
                    <div className={`mt-3 rounded-md px-3 py-2 text-xs ${driveOauthMessage.type === 'success' ? 'bg-success/10 text-success' : 'bg-destructive/10 text-destructive'}`}>
                      {driveOauthMessage.text}
                    </div>
                  )}
                </div>

                <Separator />

                <FieldGroup>
                  <Field label="凭据路径" description="credentials.json">
                    <TextInput
                      value={(cfg?.drive as Record<string, unknown>)?.credentials_json as string | undefined}
                      onChange={(v) => set('drive', 'credentials_json', v)}
                      placeholder="config/credentials.json"
                      mono
                    />
                  </Field>
                  <Field label="Token 路径">
                    <TextInput
                      value={(cfg?.drive as Record<string, unknown>)?.token_json as string | undefined}
                      onChange={(v) => set('drive', 'token_json', v)}
                      placeholder="config/token.json"
                      mono
                    />
                  </Field>
                  <Field label="扫描目录 ID" description="目标文件夹">
                    <TextInput
                      value={(cfg?.drive as Record<string, unknown>)?.scan_folder_id as string | undefined}
                      onChange={(v) => set('drive', 'scan_folder_id', v)}
                      placeholder="1AbCdEfGhIjKlMn..."
                      mono
                    />
                  </Field>
                  <Field label="媒体库根目录 ID">
                    <TextInput
                      value={(cfg?.drive as Record<string, unknown>)?.root_folder_id as string | undefined}
                      onChange={(v) => set('drive', 'root_folder_id', v)}
                      placeholder="Google Drive 文件夹 ID"
                      mono
                    />
                  </Field>
                  <Field label="电影归档目录" description="留空同根目录">
                    <TextInput
                      value={(cfg?.drive as Record<string, unknown>)?.movie_root_id as string | undefined}
                      onChange={(v) => set('drive', 'movie_root_id', v)}
                      placeholder="留空同根目录"
                      mono
                    />
                  </Field>
                  <Field label="剧集归档目录" description="留空同根目录">
                    <TextInput
                      value={(cfg?.drive as Record<string, unknown>)?.tv_root_id as string | undefined}
                      onChange={(v) => set('drive', 'tv_root_id', v)}
                      placeholder="留空同根目录"
                      mono
                    />
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="u115" className="mt-0">
            <Card>
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
                    <div className="mt-3 inline-block rounded-lg border bg-muted/50 p-3">
                      <div className="text-xs text-muted-foreground">使用 115 App 扫码</div>
                      <img src={u115QrPreviewUrl} alt="QR" className="mt-2 size-36 rounded" />
                    </div>
                  )}
                  {u115OauthMessage && (
                    <div className={`mt-3 rounded-md px-3 py-2 text-xs ${u115OauthMessage.type === 'success' ? 'bg-success/10 text-success' : 'bg-destructive/10 text-destructive'}`}>
                      {u115OauthMessage.text}
                    </div>
                  )}
                </div>

                <Separator />

                <FieldGroup>
                  <Field label="Client ID">
                    <TextInput
                      value={(cfg?.u115 as Record<string, unknown>)?.client_id as string | undefined}
                      onChange={(v) => set('u115', 'client_id', v)}
                      placeholder="100197847"
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

                <Separator />

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

                <Separator />

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
                      <span className={`text-xs ${u115CookieMessage.type === 'success' ? 'text-success' : 'text-destructive'}`}>
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
            <Card>
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
            <Card>
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
                      placeholder="123456:ABC..."
                      mono
                    />
                  </Field>
                  <Field label="Chat ID" description="接收通知的账号">
                    <TextInput
                      value={(cfg?.telegram as Record<string, unknown>)?.chat_id as string | undefined}
                      onChange={(v) => set('telegram', 'chat_id', v)}
                      placeholder="371338215"
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

          <TabsContent value="pipeline" className="mt-0">
            <Card>
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
                </FieldGroup>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="subtitle" className="mt-0">
            <Card>
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
                <div className="mt-4 text-xs text-muted-foreground">
                  <p>支持的字幕格式：<code className="rounded bg-muted px-1">.srt</code> <code className="rounded bg-muted px-1">.ass</code> <code className="rounded bg-muted px-1">.ssa</code></p>
                  <p className="mt-1">语言代码标准化：简体中文 → <code className="rounded bg-muted px-1">zh-CN</code>，繁体中文 → <code className="rounded bg-muted px-1">zh-TW</code></p>
                </div>
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      )}
    </div>
  )
}
