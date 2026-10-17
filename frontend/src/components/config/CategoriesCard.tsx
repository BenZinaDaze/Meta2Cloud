import { useState, useEffect, useCallback } from 'react'
import { Plus, ArrowUp, ArrowDown, Trash2 } from 'lucide-react'
import { getCategories, saveCategories } from '@/api'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
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

interface CategoriesCardProps {
  onToast?: (type: 'success' | 'error' | 'warning', title: string, message?: string) => void
}

export default function CategoriesCard({ onToast }: CategoriesCardProps) {
  const notify = useCallback(
    (type: 'success' | 'error' | 'warning', title: string, message?: string) => {
      const fn = onToast ?? ((title: string, message?: string) => toast[type](title, { description: message }))
      fn(type, title, message)
    },
    [onToast],
  )
  const [enabled, setEnabled] = useState(false)
  const [config, setConfig] = useState<CategoriesConfig>(DEFAULT_CONFIG)
  const [saving, setSaving] = useState(false)
  const [customGenre, setCustomGenre] = useState('')

  const load = useCallback(async () => {
    try {
      const res = await getCategories()
      setEnabled(!!res.data?.enabled)
      if (res.data?.config) setConfig(res.data.config)
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
    const rule = config[kind][idx]
    const ids = rule.genre_ids ?? []
    updateRule(kind, idx, {
      genre_ids: ids.includes(id) ? ids.filter((v) => v !== id) : [...ids, id],
    })
  }

  const toggleLang = (kind: 'tv' | 'movie', idx: number, v: string) => {
    const rule = config[kind][idx]
    const langs = rule.languages ?? []
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
    try {
      await saveCategories({ enabled, config })
      notify(
        'success',
        enabled ? '分类配置已保存并启用' : '分类已停用',
        enabled
          ? '之后的整理将按分类目录归档；媒体服务器需重建/刷新库'
          : '整理将回到电影根/剧集根布局',
      )
    } catch (e) {
      const msg = (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail
      notify('error', '分类配置保存失败', msg || (e instanceof Error ? e.message : String(e)))
    } finally {
      setSaving(false)
    }
  }, [enabled, config, notify])

  const renderRules = (kind: 'tv' | 'movie', kindLabel: string) => (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-[13px] font-medium">{kindLabel}分类规则</span>
        <Button variant="outline" size="sm" className="h-7 gap-1 rounded-full px-3" onClick={() => addRule(kind)}>
          <Plus className="size-3.5" />
          添加规则
        </Button>
      </div>
      {config[kind].map((rule, idx) => {
        const isLast = idx === config[kind].length - 1
        return (
          <div key={idx} className="space-y-2 rounded-xl border border-border/60 p-3">
            <div className="flex items-center gap-2">
              <Input
                value={rule.name}
                onChange={(e) => updateRule(kind, idx, { name: e.target.value })}
                placeholder="目录名"
                className="h-8"
              />
              {isLast && (
                <Badge variant="outline" className="shrink-0 border-primary/40 text-[10px] text-primary">
                  兜底
                </Badge>
              )}
              <div className="ml-auto flex shrink-0 gap-0.5">
                <Button variant="ghost" size="icon-sm" disabled={idx === 0} onClick={() => moveRule(kind, idx, -1)} title="上移（优先级更高）">
                  <ArrowUp className="size-3.5" />
                </Button>
                <Button variant="ghost" size="icon-sm" disabled={isLast} onClick={() => moveRule(kind, idx, 1)} title="下移">
                  <ArrowDown className="size-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  disabled={isLast}
                  onClick={() => removeRule(kind, idx)}
                  className="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                  title={isLast ? '兜底规则不可删除' : '删除'}
                >
                  <Trash2 className="size-3.5" />
                </Button>
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="mr-1 text-[11px] text-muted-foreground">流派</span>
              {GENRE_OPTIONS.map((g) => {
                const active = (rule.genre_ids ?? []).includes(g.id)
                return (
                  <button
                    key={g.id}
                    onClick={() => toggleGenre(kind, idx, g.id)}
                    className={cn(
                      'rounded-full border px-2 py-0.5 text-[11px] transition-colors',
                      active
                        ? 'border-primary/50 bg-primary/15 text-primary'
                        : 'border-border/60 text-muted-foreground hover:text-foreground',
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
                    className="rounded-full border border-primary/50 bg-primary/15 px-2 py-0.5 text-[11px] text-primary"
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
                className="h-6 w-20 text-[11px]"
              />
            </div>
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="mr-1 text-[11px] text-muted-foreground">语言</span>
              {LANG_OPTIONS.map((l) => {
                const active = (rule.languages ?? []).includes(l.v)
                return (
                  <button
                    key={l.v}
                    onClick={() => toggleLang(kind, idx, l.v)}
                    className={cn(
                      'rounded-full border px-2 py-0.5 text-[11px] transition-colors',
                      active
                        ? 'border-primary/50 bg-primary/15 text-primary'
                        : 'border-border/60 text-muted-foreground hover:text-foreground',
                    )}
                  >
                    {l.label}
                  </button>
                )
              })}
              <span className="text-[10px] text-muted-foreground">（都不选 = 不限语言）</span>
            </div>
          </div>
        )
      })}
    </div>
  )

  return (
    <Card className="border-border/70 bg-card/40">
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="text-base">媒体分类</CardTitle>
            <CardDescription>
              按 TMDB 流派把媒体归入 剧集/电影 下的分类目录；未命中的落兜底目录。停用后回到电影根/剧集根布局。
            </CardDescription>
          </div>
          <Switch checked={enabled} onCheckedChange={setEnabled} />
        </div>
      </CardHeader>
      {enabled && (
        <CardContent className="space-y-5">
          <div className="flex flex-wrap items-end gap-4">
            <div className="w-40">
              <div className="mb-1.5 text-[13px] font-medium">剧集目录名</div>
              <Input value={config.dirs.tv} onChange={(e) => updateDirs({ tv: e.target.value })} className="h-8" />
            </div>
            <div className="w-40">
              <div className="mb-1.5 text-[13px] font-medium">电影目录名</div>
              <Input value={config.dirs.movie} onChange={(e) => updateDirs({ movie: e.target.value })} className="h-8" />
            </div>
            <div className="w-40">
              <div className="mb-1.5 text-[13px] font-medium">其他目录名（无元数据兜底）</div>
              <Input value={config.dirs.other} onChange={(e) => updateDirs({ other: e.target.value })} className="h-8" />
            </div>
          </div>

          {renderRules('tv', '剧集')}
          {renderRules('movie', '电影')}

          <p className="text-xs text-muted-foreground">
            规则按从上到下的顺序匹配，首条命中即归入该目录；最后一条为兜底目录（不可删除/下移）。
            保存后目录结构在下次整理时生效并自动创建，媒体服务器需刷新/重建库。
          </p>

          <div className="flex items-center gap-3">
            <Button size="sm" onClick={handleSave} disabled={saving}>
              {saving ? '保存中…' : '保存分类'}
            </Button>
          </div>
        </CardContent>
      )}
    </Card>
  )
}
