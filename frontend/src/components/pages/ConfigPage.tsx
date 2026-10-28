import { useState, useEffect, useCallback } from 'react'
import {
  Save,
  RotateCcw,
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
  Activity,
} from 'lucide-react'
import {
  getMainConfig,
  saveMainConfig,
  getParserRulesConfig,
  saveParserRulesConfig,
  testMediaVault,
  testTelegram,
  refreshLibrary,
} from '@/api'
import CustomWordsHelp from '@/components/config/CustomWordsHelp'
import WatchedSyncSection from '@/components/config/WatchedSyncSection'
import U115Section from '@/components/config/U115Section'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
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
import FolderSelectInput from '@/components/config/FolderSelectInput'
import {
  FieldGroup,
  Field,
  TextInput,
  NumberInput,
  Toggle,
  ListField,
  MultilineRulesField,
  NoticeMessage,
  randomSecret,
} from '@/components/config/fields'
import { apiDetail, useAsyncAction } from '@/hooks/useAsyncAction'
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
  { key: 'watched', label: '观看同步', icon: Activity },
  { key: 'pipeline', label: '整理策略', icon: FolderSync },
  { key: 'strm', label: 'STRM', icon: FileVideo },
  { key: 'rss', label: 'RSS', icon: Rss },
  { key: 'telegram', label: '通知', icon: Bell },
  { key: 'mediavault', label: 'MediaVault', icon: RadioTower },
  { key: 'subtitle', label: '字幕', icon: Captions },
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
  const fullRefreshAction = useAsyncAction()
  const mediavaultAction = useAsyncAction()
  const telegramAction = useAsyncAction()
  // 轮询 effect 内使用的稳定 setter 别名（useState setter 恒稳定，可安全列入依赖）

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
  }, [page])



  // ── 观看同步：状态（历史明细在「Trakt 同步」页查看）──────
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
      setError(apiDetail(e) || '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const handleFullRefresh = () =>
    fullRefreshAction.run(async () => {
      const res = await refreshLibrary(true)
      const data = res?.data || {}
      return `全量刷新完成：电影 ${data.total_movies ?? '-'} 部，剧集 ${data.total_tv ?? '-'} 部`
    }, '全量刷新失败')

  const handleTelegramTest = () => {
    const telegram = (cfg?.telegram as Record<string, unknown>) || {}
    return telegramAction.run(async () => {
      await testTelegram({
        bot_token: (telegram.bot_token as string) || '',
        chat_id: (telegram.chat_id as string) || '',
      })
      return '测试通知已发送，请在 Telegram 查看'
    }, '测试通知发送失败')
  }

  const handleMediaVaultTest = () => {
    const mediavault = (cfg?.mediavault as Record<string, unknown>) || {}
    return mediavaultAction.run(async () => {
      const res = await testMediaVault({
        enabled: mediavault.enabled,
        scheme: mediavault.scheme,
        host: mediavault.host,
        api_key: mediavault.api_key,
        timeout: mediavault.timeout,
      })
      return res?.data?.message || 'MediaVault 测试成功'
    }, 'MediaVault 测试失败')
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
                    disabled={fullRefreshAction.busy}
                  >
                    <RefreshCw className={cn('size-5', fullRefreshAction.busy && 'animate-spin')} />
                    {fullRefreshAction.busy ? '刷新中…' : '全量刷新媒体库'}
                  </Button>
                  <NoticeMessage message={fullRefreshAction.message} />
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
                  <Field label="密码" description="留空保持当前密码不变">
                    <TextInput
                      value={(cfg?.webui as Record<string, unknown>)?.password as string | undefined}
                      onChange={(v) => set('webui', 'password', v)}
                      type="password"
                      placeholder="输入新密码"
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
            <U115Section cfg={cfg} set={set} original={original} />
          </TabsContent>

          <TabsContent value="watched" className="mt-0">
            <WatchedSyncSection cfg={cfg} set={set} />
          </TabsContent>

          <TabsContent value="rss" className="mt-0">
            <Card className="border-border/70 bg-card/40">
              <CardHeader>
                <CardTitle className="text-base">RSS 订阅</CardTitle>
                <CardDescription>后台轮询设置</CardDescription>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field label="定时规则" description="cron 表达式：分 时 日 月 周（服务器时区），支持 @every 30m 简写；留空表示不定时">
                    <TextInput
                      value={(cfg?.rss as Record<string, unknown>)?.poll_cron as string | undefined}
                      onChange={(v) => set('rss', 'poll_cron', v)}
                      placeholder="*/5 * * * *"
                      mono
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
                <div className="mt-4 flex items-center gap-3">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleTelegramTest}
                    disabled={telegramAction.busy || !((cfg?.telegram as Record<string, unknown>)?.bot_token) || !((cfg?.telegram as Record<string, unknown>)?.chat_id)}
                  >
                    {telegramAction.busy ? '测试中…' : '发送测试通知'}
                  </Button>
                  {telegramAction.message && (
                    <span className={cn('text-xs', telegramAction.message.type === 'success' ? 'text-success' : 'text-destructive')}>
                      {telegramAction.message.text}
                    </span>
                  )}
                </div>
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
                      disabled={mediavaultAction.busy || !(cfg?.mediavault as Record<string, unknown>)?.host || !(cfg?.mediavault as Record<string, unknown>)?.api_key}
                    >
                      {mediavaultAction.busy ? '测试中…' : '测试'}
                    </Button>
                    {mediavaultAction.message && (
                      <span className={cn('text-xs', mediavaultAction.message.type === 'success' ? 'text-success' : 'text-destructive')}>
                        {mediavaultAction.message.text}
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
                  <Field label="定时规则" description="cron 表达式：分 时 日 月 周（服务器时区），支持 @every 30m 简写；留空仅整理入库/手动/删除后触发">
                    <TextInput
                      value={(cfg?.strm as Record<string, unknown>)?.sync_cron as string | undefined}
                      onChange={(v) => set('strm', 'sync_cron', v)}
                      placeholder="0 8 * * *"
                      mono
                    />
                  </Field>
                  <Field label="服务地址 (url_base)" description="写入 .strm 的地址，支持 IP:端口或域名（含反代/HTTPS）；省略 http:// 时自动按 http 处理" fullWidth>
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
                    <span className="text-[11px] text-muted-foreground/80">网盘媒体库目录 → 本地 STRM 输出目录，目录结构镜像网盘</span>
                  </div>
                  <div className="space-y-2">
                    {(((cfg?.strm as Record<string, unknown>)?.mappings as Array<Record<string, unknown>>) || []).map((m, i) => (
                      <div key={i} className="flex flex-col gap-2 sm:flex-row sm:items-center">
                        <FolderSelectInput
                          value={(m.pan_folder_id as string) ?? ''}
                          onChange={(v) => {
                            const list = [...(((cfg?.strm as Record<string, unknown>)?.mappings as Array<Record<string, unknown>>) || [])]
                            list[i] = { ...list[i], pan_folder_id: v }
                            set('strm', 'mappings', list)
                          }}
                          placeholder="选择网盘目录"
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
                  <Field label="同步后通知 Plex" description="同步有增/改/删后自动通知 Plex 扫描对应媒体库（需已配对 Plex）">
                    <Toggle
                      value={((cfg?.strm as Record<string, unknown>)?.refresh_plex as boolean) || false}
                      onChange={(v) => set('strm', 'refresh_plex', v)}
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
                  <p className="mt-1">触发方式：整理入库成功后自动生成 + 定时增量同步 + 本页保存后到「STRM 同步」页手动触发。开启「同步后通知 Plex」后，同步有增/改/删会自动通知 Plex 扫库。将本地 STRM 目录作为媒体库加入 Emby / Jellyfin / Infuse 即可播放。</p>
                </div>
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      )}
    </div>
  )
}
