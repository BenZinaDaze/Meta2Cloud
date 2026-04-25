import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { Loader2 } from 'lucide-react'
import { createSubscription, testSubscription, updateSubscription } from '@/api'
import { toast } from 'sonner'
import type { Subscription } from '@/types/api'

function parseKeywords(value: string): string[] {
  return value
    .split(/[\n,，]+/)
    .map((item) => item.trim())
    .filter(Boolean)
}

interface SubscriptionModalProps {
  mode?: 'create' | 'edit'
  initialValue?: Subscription
  aria2Enabled?: boolean
  u115Authorized?: boolean
  open: boolean
  onOpenChange: (open: boolean) => void
  onSaved?: (subscription: Subscription) => void
}

export default function SubscriptionModal({
  mode = 'create',
  initialValue,
  aria2Enabled = false,
  u115Authorized = false,
  open,
  onOpenChange,
  onSaved,
}: SubscriptionModalProps) {
  const [form, setForm] = useState(() => ({
    name: initialValue?.name || '',
    media_title: initialValue?.media_title || '',
    media_type: initialValue?.media_type || 'tv',
    tmdb_id: initialValue?.tmdb_id || null,
    poster_url: initialValue?.poster_url || null,
    site: initialValue?.site || 'mikan',
    rss_url: initialValue?.rss_url || '',
    subgroup_name: initialValue?.subgroup_name || '',
    season_number: initialValue?.season_number || 1,
    start_episode: initialValue?.start_episode || 1,
    keyword_text: (initialValue?.keyword_all || []).join(', '),
    push_target: initialValue?.push_target || (u115Authorized ? 'u115' : 'aria2'),
    enabled: initialValue?.enabled ?? true,
  }))
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<{
    summary?: {
      total_items: number
      parsed_items: number
      matched_items: number
    }
    matches?: Array<{
      title: string
      season_number: number
      episode_number?: number
      keyword_hits?: string[]
      all_keywords_hit: boolean
      would_push: boolean
      publish_time?: string
    }>
  } | null>(null)
  const [error, setError] = useState('')

  const availableTargets = useMemo(() => {
    const items: { value: string; label: string }[] = []
    if (aria2Enabled) items.push({ value: 'aria2', label: '推送下载' })
    if (u115Authorized) items.push({ value: 'u115', label: '推送云下载' })
    return items
  }, [aria2Enabled, u115Authorized])

  // Reset form when initialValue changes (e.g., opening modal with new draft)
  useEffect(() => {
    if (open && initialValue) {
      setForm({
        name: initialValue.name || '',
        media_title: initialValue.media_title || '',
        media_type: initialValue.media_type || 'tv',
        tmdb_id: initialValue.tmdb_id || null,
        poster_url: initialValue.poster_url || null,
        site: initialValue.site || 'mikan',
        rss_url: initialValue.rss_url || '',
        subgroup_name: initialValue.subgroup_name || '',
        season_number: initialValue.season_number || 1,
        start_episode: initialValue.start_episode || 1,
        keyword_text: (initialValue.keyword_all || []).join(', '),
        push_target: initialValue.push_target || (u115Authorized ? 'u115' : 'aria2'),
        enabled: initialValue.enabled ?? true,
      })
      setTestResult(null)
      setError('')
    }
  }, [open, initialValue, u115Authorized])

  useEffect(() => {
    if (availableTargets.length === 0) return
    if (!availableTargets.some((item) => item.value === form.push_target)) {
      setForm((prev) => ({ ...prev, push_target: availableTargets[0].value }))
    }
  }, [availableTargets, form.push_target])

  const handleClose = useCallback(() => {
    onOpenChange(false)
  }, [onOpenChange])

  function updateField(key: string, value: unknown) {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  async function handleTest() {
    if (!form.rss_url.trim()) return
    setTesting(true)
    setError('')
    try {
      const res = await testSubscription({
        media_title: form.media_title,
        poster_url: form.poster_url,
        site: form.site,
        rss_url: form.rss_url,
        season_number: Number(form.season_number),
        start_episode: Number(form.start_episode),
        keyword_all: parseKeywords(form.keyword_text),
      })
      setTestResult(res.data)
      toast.success('规则测试完成', {
        description: `命中 ${res.data?.summary?.matched_items ?? 0} 条`,
      })
    } catch (err) {
      setTestResult(null)
      const message = (err as { response?: { data?: { detail?: string } }; message?: string })?.response?.data?.detail ||
        (err as { message?: string })?.message ||
        '测试失败'
      setError(message)
    } finally {
      setTesting(false)
    }
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!form.name.trim() || !form.rss_url.trim() || !form.media_title.trim()) return
    if (availableTargets.length === 0) {
      setError('当前没有可用的推送目标，请先连接下载器或授权 115')
      return
    }
    setSaving(true)
    setError('')
    const payload = {
      name: form.name.trim(),
      media_title: form.media_title.trim(),
      media_type: form.media_type,
      tmdb_id: form.tmdb_id || null,
      poster_url: form.poster_url || null,
      site: form.site,
      rss_url: form.rss_url.trim(),
      subgroup_name: form.subgroup_name.trim(),
      season_number: Number(form.season_number),
      start_episode: Number(form.start_episode),
      keyword_all: parseKeywords(form.keyword_text),
      push_target: form.push_target,
      enabled: !!form.enabled,
    }
    try {
      const res = mode === 'edit' && initialValue?.id
        ? await updateSubscription(initialValue.id, payload)
        : await createSubscription(payload)
      onSaved?.(res.data.subscription)
      toast.success(mode === 'edit' ? '订阅已更新' : '订阅已创建', {
        description: payload.name,
      })
      handleClose()
    } catch (err) {
      const message = (err as { response?: { data?: { detail?: string } }; message?: string })?.response?.data?.detail ||
        (err as { message?: string })?.message ||
        '保存失败'
      setError(message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-5xl p-0 gap-0 overflow-hidden max-h-[calc(100dvh-2rem)] flex flex-col">
        <DialogTitle className="sr-only">
          {mode === 'edit' ? '编辑订阅' : '创建订阅'}
        </DialogTitle>
        <DialogDescription className="sr-only">
          配置 RSS 订阅规则，包括剧集标题、字幕组、关键字等
        </DialogDescription>
        {/* Header */}
        <div className="border-b px-4 pb-4 pt-6 sm:px-6">
          <div className="flex items-start justify-between gap-4">
            <div>
              <div className="text-[11px] font-semibold uppercase tracking-widest text-brand">
                RSS Subscription
              </div>
              <h2 className="mt-1.5 text-xl font-bold leading-snug sm:text-2xl">
                {mode === 'edit' ? '编辑订阅' : '创建订阅'}
              </h2>
              <p className="mt-1.5 text-xs text-muted-foreground sm:text-sm">
                订阅详情
              </p>
            </div>
          </div>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto px-4 py-4 sm:px-6 sm:py-6">
          <form onSubmit={handleSubmit} className="grid gap-5 lg:grid-cols-2">
            {/* Left: Form */}
            <section className="rounded-2xl p-4 sm:p-5 border bg-card">
              <div className="grid gap-4">
                <div>
                  <Label>订阅名称</Label>
                  <Input
                    value={form.name}
                    onChange={(e) => updateField('name', e.target.value)}
                    className="mt-2"
                  />
                </div>
                <div className="grid gap-4 sm:grid-cols-2">
                  <div>
                    <Label>剧集标题</Label>
                    <Input
                      value={form.media_title}
                      onChange={(e) => updateField('media_title', e.target.value)}
                      className="mt-2"
                    />
                  </div>
                  <div>
                    <Label>字幕组</Label>
                    <Input
                      value={form.subgroup_name}
                      onChange={(e) => updateField('subgroup_name', e.target.value)}
                      className="mt-2"
                    />
                  </div>
                </div>
                <div>
                  <Label>RSS 地址</Label>
                  <Textarea
                    value={form.rss_url}
                    onChange={(e) => updateField('rss_url', e.target.value)}
                    rows={3}
                    className="mt-2 resize-none"
                  />
                </div>
                <div className="grid gap-4 sm:grid-cols-2">
                  <div>
                    <Label>第几季</Label>
                    <Input
                      type="number"
                      min={1}
                      value={form.season_number}
                      onChange={(e) => updateField('season_number', e.target.value)}
                      className="mt-2"
                    />
                  </div>
                  <div>
                    <Label>起始集数</Label>
                    <Input
                      type="number"
                      min={1}
                      value={form.start_episode}
                      onChange={(e) => updateField('start_episode', e.target.value)}
                      className="mt-2"
                    />
                  </div>
                </div>
                <div>
                  <Label>关键字</Label>
                  <Textarea
                    value={form.keyword_text}
                    onChange={(e) => updateField('keyword_text', e.target.value)}
                    rows={3}
                    placeholder="例如：1080p, 简中"
                    className="mt-2 resize-none"
                  />
                  <div className="mt-2 text-xs text-muted-foreground">
                    用逗号分隔。这里填写的关键字必须全部命中，测试结果会展示最终会下哪些。
                  </div>
                </div>
                <div>
                  <Label>推送目标</Label>
                  <div className="grid gap-2 sm:grid-cols-2 mt-2">
                    {availableTargets.length > 0 ? (
                      availableTargets.map((target) => (
                        <Button
                          key={target.value}
                          type="button"
                          variant={form.push_target === target.value ? 'default' : 'outline'}
                          onClick={() => updateField('push_target', target.value)}
                        >
                          {target.label}
                        </Button>
                      ))
                    ) : (
                      <div className="rounded-xl px-4 py-3 text-sm bg-destructive/10 text-destructive border border-destructive/20 col-span-2">
                        当前没有可用推送目标
                      </div>
                    )}
                  </div>
                </div>
                <div
                  className="flex w-full items-center justify-between gap-4 rounded-xl border px-4 py-3 transition-all"
                  style={{
                    background: form.enabled ? 'hsl(var(--brand) / 0.1)' : 'transparent',
                    borderColor: form.enabled ? 'hsl(var(--brand) / 0.4)' : 'hsl(var(--border))',
                  }}
                >
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-semibold">创建后立即启用</span>
                      <span
                        className="rounded-full px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider"
                        style={{
                          background: form.enabled ? 'hsl(var(--brand))' : 'hsl(var(--muted))',
                          color: form.enabled ? 'hsl(var(--brand-foreground))' : 'hsl(var(--muted-foreground))',
                        }}
                      >
                        {form.enabled ? '已启用' : '已禁用'}
                      </span>
                    </div>
                    <div className="mt-1 text-xs text-muted-foreground">
                      关闭后会保存订阅规则，但不会参与后台轮询，直到你手动启用。
                    </div>
                  </div>
                  <Switch
                    checked={form.enabled}
                    onCheckedChange={(checked) => updateField('enabled', checked)}
                  />
                </div>
              </div>
            </section>

            {/* Right: Test results */}
            <section className="rounded-2xl p-4 sm:p-5 border bg-card">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <div className="text-sm font-semibold">规则测试</div>
                  <div className="mt-1 text-xs text-muted-foreground">
                    使用当前表单配置测试 RSS 会命中哪些条目，但不会真的推送下载。
                  </div>
                </div>
                <Button
                  type="button"
                  variant="outline"
                  onClick={handleTest}
                  disabled={testing || !form.rss_url.trim()}
                >
                  {testing ? (
                    <>
                      <Loader2 className="size-4 animate-spin" />
                      测试中…
                    </>
                  ) : (
                    '测试规则'
                  )}
                </Button>
              </div>

              {error && (
                <div className="mt-4 rounded-xl px-4 py-3 text-sm bg-destructive/10 text-destructive border border-destructive/20">
                  {error}
                </div>
              )}

              {testResult ? (
                <>
                  <div className="mt-4 grid gap-3 sm:grid-cols-3">
                    {[
                      { label: 'RSS 条目', value: testResult.summary?.total_items ?? 0 },
                      { label: '解析成功', value: testResult.summary?.parsed_items ?? 0 },
                      { label: '最终命中', value: testResult.summary?.matched_items ?? 0 },
                    ].map((item) => (
                      <div key={item.label} className="rounded-xl px-4 py-3 bg-muted/50 border">
                        <div className="text-[11px] font-semibold uppercase tracking-widest text-muted-foreground">
                          {item.label}
                        </div>
                        <div className="mt-1 text-2xl font-bold tabular-nums">{item.value}</div>
                      </div>
                    ))}
                  </div>

                  <div className="mt-4 max-h-[48dvh] overflow-y-auto pr-1">
                    <div className="flex flex-col gap-3">
                      {(testResult.matches || []).map((item, index) => (
                        <div
                          key={`${item.title}-${index}`}
                          className={`rounded-xl px-4 py-3 border ${
                            item.would_push
                              ? 'bg-success/5 border-success/20'
                              : 'bg-muted/50 border-border'
                          }`}
                        >
                          <div className="flex flex-wrap items-center gap-2">
                            <div className="flex-1 text-sm font-semibold leading-6">
                              {item.title}
                            </div>
                            <Badge
                              variant={item.would_push ? 'default' : 'secondary'}
                              className={item.would_push ? 'bg-success text-success-foreground' : ''}
                            >
                              {item.would_push ? '会推送' : '不会推送'}
                            </Badge>
                          </div>
                          <div className="mt-2 flex flex-wrap gap-2 text-xs">
                            <Badge variant="outline">
                              S{String(item.season_number || 1).padStart(2, '0')}E
                              {item.episode_number
                                ? String(item.episode_number).padStart(2, '0')
                                : '--'}
                            </Badge>
                            <Badge
                              variant={item.all_keywords_hit ? 'default' : 'secondary'}
                              className={
                                item.all_keywords_hit
                                  ? 'bg-success text-success-foreground'
                                  : ''
                              }
                            >
                              关键字 {item.keyword_hits?.length || 0}/
                              {parseKeywords(form.keyword_text).length}
                            </Badge>
                            {item.publish_time && (
                              <Badge variant="outline">{item.publish_time}</Badge>
                            )}
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                </>
              ) : (
                <div className="mt-5 rounded-xl px-4 py-5 text-sm leading-6 bg-muted/50 border text-muted-foreground">
                  先填写季、起始集数和关键字，然后点击"测试规则"，确认当前规则到底会下哪些资源。
                </div>
              )}
            </section>

            {/* Footer */}
            <div className="lg:col-span-2 flex items-center justify-end gap-3">
              <Button type="button" variant="outline" onClick={handleClose}>
                取消
              </Button>
              <Button type="submit" disabled={saving}>
                {saving ? (
                  <>
                    <Loader2 className="size-4 animate-spin" />
                    保存中…
                  </>
                ) : mode === 'edit' ? (
                  '保存订阅'
                ) : (
                  '创建订阅'
                )}
              </Button>
            </div>
          </form>
        </div>
      </DialogContent>
    </Dialog>
  )
}
