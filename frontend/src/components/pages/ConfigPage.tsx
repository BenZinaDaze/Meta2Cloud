import { useState, useEffect, useCallback } from 'react'
import {
  Save,
  RotateCcw,
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
  Activity,
} from 'lucide-react'
import {
  getMainConfig,
  saveMainConfig,
  getParserRulesConfig,
  saveParserRulesConfig,
} from '@/api'
import CustomWordsHelp from '@/components/config/CustomWordsHelp'
import WatchedSyncSection from '@/components/config/WatchedSyncSection'
import U115Section from '@/components/config/U115Section'
import StorageSection from '@/components/config/StorageSection'
import AuthSection from '@/components/config/AuthSection'
import TmdbSection from '@/components/config/TmdbSection'
import RssSection from '@/components/config/RssSection'
import TelegramSection from '@/components/config/TelegramSection'
import MediaVaultSection from '@/components/config/MediaVaultSection'
import PipelineSection from '@/components/config/PipelineSection'
import SubtitleSection from '@/components/config/SubtitleSection'
import StrmSection from '@/components/config/StrmSection'
import { Button } from '@/components/ui/button'
import {
  Tabs, TabsList, TabsTrigger, TabsContent,
} from '@/components/ui/tabs'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { MultilineRulesField, ListField } from '@/components/config/fields'
import { SkeletonPanel } from '@/components/StatePanel'
import PageHeader from '@/components/shared/PageHeader'
import { apiDetail } from '@/hooks/useAsyncAction'
import { normalizeConfig } from '@/lib/configNormalize'

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
            <StorageSection cfg={cfg} set={set} />
          </TabsContent>

          <TabsContent value="auth" className="mt-0">
            <AuthSection cfg={cfg} set={set} />
          </TabsContent>

          <TabsContent value="tmdb" className="mt-0">
            <TmdbSection cfg={cfg} set={set} />
          </TabsContent>

          <TabsContent value="u115" className="mt-0">
            <U115Section cfg={cfg} set={set} original={original} />
          </TabsContent>

          <TabsContent value="watched" className="mt-0">
            <WatchedSyncSection cfg={cfg} set={set} />
          </TabsContent>

          <TabsContent value="pipeline" className="mt-0">
            <PipelineSection cfg={cfg} set={set} />
          </TabsContent>

          <TabsContent value="strm" className="mt-0">
            <StrmSection cfg={cfg} set={set} />
          </TabsContent>

          <TabsContent value="rss" className="mt-0">
            <RssSection cfg={cfg} set={set} />
          </TabsContent>

          <TabsContent value="telegram" className="mt-0">
            <TelegramSection cfg={cfg} set={set} />
          </TabsContent>

          <TabsContent value="mediavault" className="mt-0">
            <MediaVaultSection cfg={cfg} set={set} />
          </TabsContent>

          <TabsContent value="subtitle" className="mt-0">
            <SubtitleSection cfg={cfg} set={set} />
          </TabsContent>
        </Tabs>
      )}
    </div>
  )
}
