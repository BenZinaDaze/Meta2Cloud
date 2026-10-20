import { useCallback, useEffect, useRef, useState } from 'react'
import { Plus, ArrowUp, ArrowDown, Trash2, Folder, ChevronDown, ChevronRight, FolderSync, Loader2 } from 'lucide-react'
import { getCategories, saveCategories, reclassifyDry, reclassifyStart, reclassifyStatus } from '@/api'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card } from '@/components/ui/card'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import PageHeader from '@/components/shared/PageHeader'
import type { CategoryDirs, CategoryRule, CategoriesConfig } from '@/types/api'

const GENRE_OPTIONS = [
  { id: 16, label: '动画' },
  { id: 35, label: '喜剧' },
  { id: 18, label: '剧情' },
  { id: 878, label: '科幻' },
  { id: 28, label: '动作' },
  { id: 10749, label: '爱情' },
  { id: 9648, label: '悬疑' },
  { id: 10751, label: '家庭' },
  { id: 99, label: '纪录片' },
  { id: 80, label: '犯罪' },
]

const LANG_OPTIONS = [
  { v: 'ja', label: '日语' },
  { v: 'en', label: '英语' },
  { v: 'ko', label: '韩语' },
  { v: 'zh', label: '中文' },
  { v: 'cn', label: '国语' },
  { v: 'tw', label: '台剧' },
]

const DEFAULT_CONFIG: CategoriesConfig = {
  dirs: { tv: '剧集', movie: '电影', other: '其他' },
  tv: [
    { name: '动漫', genre_ids: [16], languages: ['ja'] },
    { name: '国漫', genre_ids: [16], languages: ['zh', 'cn', 'tw'] },
    { name: '普通电视剧' },
  ],
  movie: [
    { name: '动漫电影', genre_ids: [16] },
    { name: '普通电影' },
  ],
}

interface CategoriesPageProps {
  onToast?: (type: 'success' | 'error' | 'warning', title: string, message?: string) => void
}

export default function CategoriesPage({ onToast }: CategoriesPageProps) {
  const notify = useCallback(
    (type: 'success' | 'error' | 'warning', title: string, text?: string) => {
      const fn = onToast ?? ((t: 'success' | 'error' | 'warning', x: string, d?: string) => toast[t](x, { description: d }))
      fn(type, title, text)
    },
    [onToast],
  )

  const [config, setConfig] = useState<CategoriesConfig>(DEFAULT_CONFIG)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [customGenre, setCustomGenre] = useState('')
  const [collapsed, setCollapsed] = useState<{ tv: boolean; movie: boolean }>({ tv: false, movie: false })
  // 全库重新归类：干跑统计 → 二次确认 → 后台执行 → 轮询进度
  const [reclassifyOpen, setReclassifyOpen] = useState(false)
  const [dryLoading, setDryLoading] = useState(false)
  const [dryData, setDryData] = useState<{
    total: number
    uncached: number
    targets: Record<string, number>
  } | null>(null)
  const [executing, setExecuting] = useState(false)
  const [progress, setProgress] = useState<{ running: boolean; moved: number; failed: number; total: number } | null>(null)
  const pollTimer = useRef<ReturnType<typeof setInterval> | null>(null)

  const stopPolling = () => {
    if (pollTimer.current) {
      clearInterval(pollTimer.current)
      pollTimer.current = null
    }
  }

  // 后台归位进度轮询：running 变为 false 时报完成并停止
  const startPolling = useCallback(() => {
    stopPolling()
    pollTimer.current = setInterval(async () => {
      try {
        const res = await reclassifyStatus()
        const st: {
          running: boolean
          moved: number
          failed: number
          total: number
          moves?: string[]
          errors?: string[]
        } = res.data
        setProgress(st)
        if (st && !st.running) {
          stopPolling()
          if (st.failed > 0) {
            const errs = (st.errors || []).slice(0, 3).join('\n')
            notify('warning', `重新归类完成：移动 ${st.moved} 项，失败 ${st.failed} 项`, errs || undefined)
          } else {
            const moves = st.moves || []
            const detail =
              moves.length > 0
                ? moves.slice(0, 3).join('\n') + (moves.length > 3 ? `\n等 ${st.moved} 项` : '')
                : `已移动 ${st.moved} 项`
            notify('success', '全库重新归类完成', detail)
          }
        }
      } catch {
        /* 静默重试 */
      }
    }, 3000)
  }, [notify])

  useEffect(() => stopPolling, [])

  const openReclassify = useCallback(async () => {
    setDryLoading(true)
    try {
      const res = await reclassifyDry()
      setDryData(res.data)
      setReclassifyOpen(true)
    } catch (e) {
      const text = (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail || '获取归类统计失败'
      notify('error', '全库重新归类', text)
    } finally {
      setDryLoading(false)
    }
  }, [notify])

  const handleReclassifyExecute = useCallback(async () => {
    setExecuting(true)
    try {
      await reclassifyStart()
      setReclassifyOpen(false)
      notify('success', '已开始全库重新归类', '仅移动位置不正确的目录，完成后将自动同步 STRM')
      setProgress({ running: true, moved: 0, failed: 0, total: dryData?.total ?? 0 })
      startPolling()
    } catch (e) {
      const resp = e as { response?: { data?: { state?: { running: boolean } }; status?: number } }
      if (resp?.response?.status === 409) {
        notify('warning', '已有归位任务进行中', '等待当前任务完成后再试')
      } else {
        const text = (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail || '启动失败'
        notify('error', '全库重新归类', text)
      }
    } finally {
      setExecuting(false)
    }
  }, [dryData, notify, startPolling])

  const toggleCollapsed = (kind: 'tv' | 'movie') =>
    setCollapsed((c) => ({ ...c, [kind]: !c[kind] }))

  const load = useCallback(async () => {
    try {
      const res = await getCategories()
      if (res.data) setConfig(res.data)
    } catch {
      notify('error', '分类配置加载失败')
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const updateDirs = (patch: Partial<CategoryDirs>) =>
    setConfig((c) => ({ ...c, dirs: { ...c.dirs, ...patch } }))

  const updateRule = (kind: 'tv' | 'movie', idx: number, patch: Partial<CategoryRule>) =>
    setConfig((c) => ({
      ...c,
      [kind]: c[kind].map((r, i) => (i === idx ? { ...r, ...patch } : r)),
    }))

  const addRule = (kind: 'tv' | 'movie') =>
    setConfig((c) => ({ ...c, [kind]: [...c[kind], { name: '' }] }))

  const removeRule = (kind: 'tv' | 'movie', idx: number) =>
    setConfig((c) => {
      if (c[kind].length <= 1) {
        notify('warning', '至少保留一条规则', '最后一条为兜底目录')
        return c
      }
      return { ...c, [kind]: c[kind].filter((_, i) => i !== idx) }
    })

  const moveRule = (kind: 'tv' | 'movie', idx: number, dir: -1 | 1) =>
    setConfig((c) => {
      const list = [...c[kind]]
      const target = idx + dir
      if (target < 0 || target >= list.length) return c
      ;[list[idx], list[target]] = [list[target], list[idx]]
      return { ...c, [kind]: list }
    })

  const toggleGenre = (kind: 'tv' | 'movie', idx: number, id: number) => {
    const ids = config[kind][idx].genre_ids ?? []
    updateRule(kind, idx, {
      genre_ids: ids.includes(id) ? ids.filter((v) => v !== id) : [...ids, id],
    })
  }

  const toggleLang = (kind: 'tv' | 'movie', idx: number, v: string) => {
    const langs = config[kind][idx].languages ?? []
    updateRule(kind, idx, {
      languages: langs.includes(v) ? langs.filter((x) => x !== v) : [...langs, v],
    })
  }

  const addCustomGenre = (kind: 'tv' | 'movie', idx: number) => {
    const id = Number.parseInt(customGenre, 10)
    if (!Number.isFinite(id) || id <= 0) return
    const ids = config[kind][idx].genre_ids ?? []
    if (!ids.includes(id)) updateRule(kind, idx, { genre_ids: [...ids, id] })
    setCustomGenre('')
  }

  const handleSave = useCallback(async () => {
    setSaving(true)
    setMessage(null)
    try {
      const res = await saveCategories(config)
      setMessage({ type: 'success', text: '已保存，目录结构已创建；下次整理生效' })
      const rulesChanged = res.data?.rules_changed === true
      notify('success', '分类配置已保存', rulesChanged ? '分类规则已变化，可对存量媒体重新归位' : '目录结构已创建，下次整理生效')
      if (rulesChanged) {
        void openReclassify()
      }
    } catch (e) {
      const text = (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail || '分类配置保存失败'
      setMessage({ type: 'error', text })
      notify('error', '分类配置保存失败', text)
    } finally {
      setSaving(false)
    }
  }, [config, notify, openReclassify])

  // 单条二级规则卡片
  const renderRuleItem = (kind: 'tv' | 'movie', idx: number, rule: CategoryRule, isLast: boolean) => (
    <div className="rounded-xl border border-border/60 bg-card/60 p-4">
      <div className="flex items-center gap-2">
        <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-muted text-[11px] font-bold text-muted-foreground">
          {idx + 1}
        </span>
        <Input
          value={rule.name}
          onChange={(e) => updateRule(kind, idx, { name: e.target.value })}
          placeholder="子分类目录名"
          className="h-9 flex-1"
        />
        <div className="flex shrink-0 gap-0.5">
          <Button variant="ghost" size="icon-sm" disabled={idx === 0} onClick={() => moveRule(kind, idx, -1)} title="上移（优先级更高）">
            <ArrowUp className="size-4" />
          </Button>
          <Button variant="ghost" size="icon-sm" disabled={isLast} onClick={() => moveRule(kind, idx, 1)} title="下移">
            <ArrowDown className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            disabled={isLast}
            onClick={() => removeRule(kind, idx)}
            className="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
            title={isLast ? '兜底目录不可删除' : '删除'}
          >
            <Trash2 className="size-4" />
          </Button>
        </div>
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-1.5">
        <span className="w-10 shrink-0 text-xs text-muted-foreground">流派</span>
        {GENRE_OPTIONS.map((g) => {
          const active = (rule.genre_ids ?? []).includes(g.id)
          return (
            <button
              key={g.id}
              onClick={() => toggleGenre(kind, idx, g.id)}
              className={cn(
                'rounded-full border px-2.5 py-1 text-xs transition-colors',
                active
                  ? 'border-primary/50 bg-primary/15 text-primary'
                  : 'border-border/60 text-muted-foreground hover:border-foreground/30 hover:text-foreground',
              )}
            >
              {g.label}
            </button>
          )
        })}
        {(rule.genre_ids ?? [])
          .filter((id) => !GENRE_OPTIONS.some((g) => g.id === id))
          .map((id) => (
            <button
              key={id}
              onClick={() => toggleGenre(kind, idx, id)}
              className="rounded-full border border-primary/50 bg-primary/15 px-2.5 py-1 text-xs text-primary"
              title="点击移除自定义流派 ID"
            >
              ID {id}
            </button>
          ))}
        <Input
          value={customGenre}
          onChange={(e) => setCustomGenre(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && addCustomGenre(kind, idx)}
          placeholder="自定义ID"
          className="h-7 w-24 text-xs"
        />
      </div>

      <div className="mt-2.5 flex flex-wrap items-center gap-1.5">
        <span className="w-10 shrink-0 text-xs text-muted-foreground">语言</span>
        {LANG_OPTIONS.map((l) => {
          const active = (rule.languages ?? []).includes(l.v)
          return (
            <button
              key={l.v}
              onClick={() => toggleLang(kind, idx, l.v)}
              className={cn(
                'rounded-full border px-2.5 py-1 text-xs transition-colors',
                active
                  ? 'border-primary/50 bg-primary/15 text-primary'
                  : 'border-border/60 text-muted-foreground hover:border-foreground/30 hover:text-foreground',
              )}
            >
              {l.label}
            </button>
          )
        })}
        <span className="text-[11px] text-muted-foreground/70">都不选 = 不限语言</span>
      </div>
    </div>
  )

  // 一级分类卡片：标题目录 + 嵌套的二级分类规则
  const renderGroup = (kind: 'tv' | 'movie', kindLabel: string) => {
    const list = config[kind]
    const isLast = (idx: number) => idx === list.length - 1
    const isCollapsed = collapsed[kind]
    return (
      <Card className="border-border/70 bg-card/40">
        <div className="flex flex-wrap items-center gap-3 px-5 py-4">
          <button
            onClick={() => toggleCollapsed(kind)}
            className="flex size-7 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted/70 hover:text-foreground"
            title={isCollapsed ? '展开' : '收起'}
          >
            {isCollapsed ? <ChevronRight className="size-4" /> : <ChevronDown className="size-4" />}
          </button>
          <Folder className="size-5 shrink-0 text-primary" />
          <Input
            value={config.dirs[kind]}
            onChange={(e) => updateDirs({ [kind]: e.target.value })}
            className="h-9 w-44 font-medium"
            title="一级目录名（位于媒体库根下）"
          />
          <span className="hidden text-xs text-muted-foreground sm:inline">一级目录 · 挂在媒体库根下</span>
          <div className="ml-auto flex shrink-0 items-center gap-2">
            <span className="hidden text-[11px] text-muted-foreground md:inline">{list.length} 个子分类</span>
            <Button
              variant="outline"
              size="sm"
              className="h-8 shrink-0 gap-1.5 rounded-full px-3.5"
              onClick={() => addRule(kind)}
            >
              <Plus className="size-3.5" />
              添加子分类
            </Button>
          </div>
        </div>
        {!isCollapsed && (
          <div className="border-t border-border/60 p-5">
            <div className="mb-3 flex items-center gap-2 text-xs text-muted-foreground">
              <span className="h-3 w-0.5 rounded-full bg-border" />
              二级分类 · 从上到下逐条判定，首条命中即归入；全部未命中落兜底目录
            </div>
            <div className="ml-1 space-y-3 border-l-2 border-border/40 pl-4">
              {list.map((rule, idx) => (
                <div key={idx}>
                  {renderRuleItem(kind, idx, rule, isLast(idx))}
                  {isLast(idx) && (
                    <p className="ml-2 mt-2 text-[11px] text-muted-foreground/80">
                      所有规则都未命中的{kindLabel}将进入此目录。
                    </p>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}
      </Card>
    )
  }

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        kicker="Categories"
        title="分类配置"
        description="按 TMDB 流派把媒体归入 剧集/电影 下的分类子目录；TMDB 未识别的落「其他」一级目录。"
        actions={
          <div className="flex items-center gap-3">
            {message && (
              <span className={cn('text-xs', message.type === 'success' ? 'text-success' : 'text-destructive')}>
                {message.text}
              </span>
            )}
            <Button
              size="sm"
              variant="outline"
              onClick={openReclassify}
              disabled={dryLoading || progress?.running}
              title="按当前规则把存量媒体移动到正确的分类目录"
              className="h-10 gap-1.5 rounded-full px-4"
            >
              {dryLoading ? <Loader2 className="size-4 animate-spin" /> : <FolderSync className="size-4" />}
              全库重新归类
            </Button>
            <Button size="sm" onClick={handleSave} disabled={saving} className="h-10 gap-1.5 rounded-full px-5">
              {saving ? '保存中…' : '保存分类'}
            </Button>
          </div>
        }
      />

      {renderGroup('tv', '剧集')}
      {renderGroup('movie', '电影')}

      <Card className="border-border/70 bg-card/40">
        <div className="flex flex-wrap items-center gap-3 px-5 py-4">
          <Folder className="size-5 shrink-0 text-warning" />
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-3">
              <Input
                value={config.dirs.other}
                onChange={(e) => updateDirs({ other: e.target.value })}
                className="h-9 w-44 font-medium"
                title="一级目录名"
              />
              <span className="text-xs text-muted-foreground">一级目录 · TMDB 未识别的媒体落此（无二级分类，按解析出的标题建目录）</span>
            </div>
          </div>
        </div>
      </Card>

      <p className="text-xs leading-relaxed text-muted-foreground">
        说明：目录名即网盘上的实际目录，保存在媒体库根下自动创建；二级分类按「流派（任一命中）+
        语言（任一命中，不选为不限）」判定，首条命中即归入。媒体服务器需刷新/重建库。
      </p>

      <AlertDialog open={reclassifyOpen} onOpenChange={setReclassifyOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>全库重新归类</AlertDialogTitle>
            <AlertDialogDescription asChild>
              <div className="space-y-2 text-sm text-muted-foreground">
                <p>
                  按当前规则，共 <span className="font-semibold text-foreground">{dryData?.total ?? 0}</span>{' '}
                  项在库媒体将归入以下分类（仅移动位置不正确的目录，文件与命名不动）：
                </p>
                <div className="flex flex-wrap gap-1.5">
                  {Object.entries(dryData?.targets ?? {}).map(([cat, count]) => (
                    <span key={cat} className="rounded-md bg-muted px-2 py-0.5 text-xs text-foreground">
                      {cat} · {count}
                    </span>
                  ))}
                </div>
                {(dryData?.uncached ?? 0) > 0 && (
                  <p className="text-warning">
                    其中 {dryData?.uncached} 项暂无 TMDB 缓存，上方分布按兜底规则预览；
                    执行时会拉取 TMDB 最新分类，无需先刷新缓存。
                  </p>
                )}
              </div>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              className="bg-primary text-primary-foreground hover:bg-primary/90"
              disabled={executing}
              onClick={handleReclassifyExecute}
            >
              {executing ? <Loader2 className="mr-1 size-4 animate-spin" /> : null}
              开始归位
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
