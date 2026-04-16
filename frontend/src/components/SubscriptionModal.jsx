import { useCallback, useEffect, useMemo, useState } from 'react'
import { createPortal } from 'react-dom'
import { createSubscription, testSubscription, updateSubscription } from '../api'

function FieldLabel({ children }) {
  return (
    <div className="mb-2 text-[11px] font-semibold uppercase tracking-[0.18em]" style={{ color: 'var(--color-muted)' }}>
      {children}
    </div>
  )
}

function NumberInput({ value, onChange, min = 1 }) {
  return (
    <input
      type="number"
      min={min}
      value={value}
      onChange={onChange}
      inputMode="numeric"
      className="min-h-11 w-full rounded-[16px] px-4 text-sm outline-none [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
      style={{
        background: 'rgba(255,255,255,0.03)',
        border: '1px solid rgba(255,255,255,0.06)',
        color: 'var(--color-text)',
      }}
    />
  )
}

function ToggleSwitch({ checked, onChange, label, description }) {
  return (
    <button
      type="button"
      onClick={() => onChange(!checked)}
      className="flex w-full items-center justify-between gap-4 rounded-[16px] px-4 py-3 text-left transition-all"
      style={{
        background: checked ? 'rgba(227,183,120,0.1)' : 'rgba(255,255,255,0.02)',
        border: `1px solid ${checked ? 'rgba(227,183,120,0.22)' : 'rgba(255,255,255,0.05)'}`,
      }}
      aria-pressed={checked}
    >
      <div className="min-w-0">
        <div className="text-sm font-semibold" style={{ color: 'var(--color-text)' }}>{label}</div>
        {description ? (
          <div className="mt-1 text-xs leading-5" style={{ color: 'var(--color-muted)' }}>{description}</div>
        ) : null}
      </div>
      <span
        className="relative flex h-7 w-12 shrink-0 rounded-full transition-colors"
        style={{ background: checked ? 'linear-gradient(135deg, var(--color-accent) 0%, #a56d2c 100%)' : 'rgba(255,255,255,0.12)' }}
      >
        <span
          className="absolute top-1 h-5 w-5 rounded-full bg-white transition-transform"
          style={{ left: 4, transform: checked ? 'translateX(20px)' : 'translateX(0)' }}
        />
      </span>
    </button>
  )
}

function parseKeywords(value) {
  return value
    .split(/[\n,，]+/)
    .map(item => item.trim())
    .filter(Boolean)
}

export default function SubscriptionModal({
  mode = 'create',
  initialValue,
  aria2Enabled = false,
  u115Authorized = false,
  onClose,
  onToast,
  onSaved,
}) {
  const [show, setShow] = useState(false)
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
  const [testResult, setTestResult] = useState(null)
  const [error, setError] = useState('')

  const availableTargets = useMemo(() => {
    const items = []
    if (aria2Enabled) items.push({ value: 'aria2', label: '推送下载' })
    if (u115Authorized) items.push({ value: 'u115', label: '推送云下载' })
    return items
  }, [aria2Enabled, u115Authorized])

  useEffect(() => {
    requestAnimationFrame(() => setShow(true))
    const onKey = (e) => e.key === 'Escape' && handleClose()
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  useEffect(() => {
    if (availableTargets.length === 0) return
    if (!availableTargets.some(item => item.value === form.push_target)) {
      setForm(prev => ({ ...prev, push_target: availableTargets[0].value }))
    }
  }, [availableTargets, form.push_target])

  const handleClose = useCallback(() => {
    setShow(false)
    setTimeout(onClose, 180)
  }, [onClose])

  function updateField(key, value) {
    setForm(prev => ({ ...prev, [key]: value }))
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
      onToast?.('success', '规则测试完成', `命中 ${res.data?.summary?.matched_items ?? 0} 条`)
    } catch (err) {
      setTestResult(null)
      setError(err?.response?.data?.detail || err.message || '测试失败')
    } finally {
      setTesting(false)
    }
  }

  async function handleSubmit(e) {
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
      const res = mode === 'edit'
        ? await updateSubscription(initialValue.id, payload)
        : await createSubscription(payload)
      onSaved?.(res.data.subscription)
      onToast?.('success', mode === 'edit' ? '订阅已更新' : '订阅已创建', payload.name)
      handleClose()
    } catch (err) {
      setError(err?.response?.data?.detail || err.message || '保存失败')
    } finally {
      setSaving(false)
    }
  }

  return createPortal(
    <div
      className="fixed inset-0 z-[140] flex items-end justify-center overflow-y-auto p-0 sm:items-start sm:p-4 sm:pt-20"
      style={{ background: 'rgba(2, 8, 18, 0.78)', backdropFilter: 'blur(10px)', opacity: show ? 1 : 0, transition: 'opacity 0.18s' }}
      onClick={(e) => e.target === e.currentTarget && handleClose()}
    >
      <div
        className="relative flex h-[100dvh] w-full flex-col overflow-hidden rounded-none sm:h-auto sm:max-w-5xl sm:rounded-[30px]"
        style={{
          background: 'linear-gradient(180deg, rgba(15, 27, 45, 0.98) 0%, rgba(11, 22, 37, 0.98) 100%)',
          border: '1px solid var(--color-border)',
          transform: show ? 'translateY(0)' : 'translateY(18px)',
          transition: 'transform 0.18s',
          boxShadow: 'var(--shadow-strong)',
          maxHeight: '100dvh',
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="sticky top-0 z-10 border-b px-4 pb-4 pt-[calc(env(safe-area-inset-top)+0.75rem)] sm:px-6 sm:py-5"
          style={{ borderColor: 'var(--color-border)', background: 'linear-gradient(180deg, rgba(15, 27, 45, 0.99) 0%, rgba(11, 22, 37, 0.98) 100%)' }}>
          <div className="mx-auto mb-3 h-1.5 w-12 rounded-full sm:hidden" style={{ background: 'rgba(255,255,255,0.14)' }} />
          <div className="flex items-start justify-between gap-4">
            <div>
              <div className="text-[11px] font-semibold uppercase tracking-[0.22em]" style={{ color: 'var(--color-accent-hover)' }}>
                RSS Subscription
              </div>
              <h2 className="mt-1.5 text-[22px] font-bold leading-snug sm:mt-2 sm:text-[28px]" style={{ color: 'var(--color-text)' }}>
                {mode === 'edit' ? '编辑订阅' : '创建订阅'}
              </h2>
              <p className="mt-1.5 text-xs leading-5 sm:mt-2 sm:text-sm" style={{ color: 'var(--color-muted)' }}>
                订阅详情
              </p>
            </div>
            <button
              type="button"
              onClick={handleClose}
              className="flex size-11 items-center justify-center rounded-2xl transition-all duration-150"
              style={{ background: 'rgba(255,255,255,0.04)', border: '1px solid var(--color-border)', color: 'var(--color-text)' }}
            >
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round">
                <path d="M6 6l12 12" />
                <path d="M18 6L6 18" />
              </svg>
            </button>
          </div>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4 sm:px-6 sm:py-6" style={{ paddingBottom: 'calc(env(safe-area-inset-bottom) + 1rem)' }}>
          <form onSubmit={handleSubmit} className="grid gap-5 lg:grid-cols-[0.96fr_1.04fr]">
            <section className="rounded-[24px] p-4 sm:p-5" style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)' }}>
              <div className="grid gap-4">
                <div>
                  <FieldLabel>订阅名称</FieldLabel>
                  <input value={form.name} onChange={e => updateField('name', e.target.value)} className="min-h-11 w-full rounded-[16px] px-4 text-sm outline-none"
                    style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.06)', color: 'var(--color-text)' }} />
                </div>
                <div className="grid gap-4 sm:grid-cols-2">
                  <div>
                    <FieldLabel>剧集标题</FieldLabel>
                    <input value={form.media_title} onChange={e => updateField('media_title', e.target.value)} className="min-h-11 w-full rounded-[16px] px-4 text-sm outline-none"
                      style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.06)', color: 'var(--color-text)' }} />
                  </div>
                  <div>
                    <FieldLabel>字幕组</FieldLabel>
                    <input value={form.subgroup_name} onChange={e => updateField('subgroup_name', e.target.value)} className="min-h-11 w-full rounded-[16px] px-4 text-sm outline-none"
                      style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.06)', color: 'var(--color-text)' }} />
                  </div>
                </div>
                <div>
                  <FieldLabel>RSS 地址</FieldLabel>
                  <textarea value={form.rss_url} onChange={e => updateField('rss_url', e.target.value)} rows={3} className="w-full rounded-[16px] px-4 py-3 text-sm outline-none"
                    style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.06)', color: 'var(--color-text)', resize: 'none' }} />
                </div>
                <div className="grid gap-4 sm:grid-cols-2">
                  <div>
                    <FieldLabel>第几季</FieldLabel>
                    <NumberInput value={form.season_number} onChange={e => updateField('season_number', e.target.value)} />
                  </div>
                  <div>
                    <FieldLabel>起始集数</FieldLabel>
                    <NumberInput value={form.start_episode} onChange={e => updateField('start_episode', e.target.value)} />
                  </div>
                </div>
                <div>
                  <FieldLabel>关键字</FieldLabel>
                  <textarea value={form.keyword_text} onChange={e => updateField('keyword_text', e.target.value)} rows={3} placeholder="例如：1080p, 简中"
                    className="w-full rounded-[16px] px-4 py-3 text-sm outline-none"
                    style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.06)', color: 'var(--color-text)', resize: 'none' }} />
                  <div className="mt-2 text-xs" style={{ color: 'var(--color-muted)' }}>
                    用逗号分隔。这里填写的关键字必须全部命中，测试结果会展示最终会下哪些。
                  </div>
                </div>
                <div>
                  <FieldLabel>推送目标</FieldLabel>
                  <div className="grid gap-2 sm:grid-cols-2">
                    {availableTargets.length > 0 ? availableTargets.map(target => (
                      <button
                        key={target.value}
                        type="button"
                        onClick={() => updateField('push_target', target.value)}
                        className="min-h-11 rounded-[16px] px-4 text-sm font-semibold transition-all"
                        style={{
                          background: form.push_target === target.value ? 'linear-gradient(135deg, var(--color-accent) 0%, #a56d2c 100%)' : 'rgba(255,255,255,0.03)',
                          border: `1px solid ${form.push_target === target.value ? 'rgba(227,183,120,0.45)' : 'rgba(255,255,255,0.06)'}`,
                          color: form.push_target === target.value ? '#fff' : 'var(--color-text)',
                        }}
                      >
                        {target.label}
                      </button>
                    )) : (
                      <div className="rounded-[16px] px-4 py-3 text-sm" style={{ background: 'rgba(239,125,117,0.08)', border: '1px solid rgba(239,125,117,0.2)', color: 'var(--color-danger)' }}>
                        当前没有可用推送目标
                      </div>
                    )}
                  </div>
                </div>
                <ToggleSwitch
                  checked={!!form.enabled}
                  onChange={(value) => updateField('enabled', value)}
                  label="创建后立即启用"
                  description="关闭后会保存订阅规则，但不会参与后台轮询，直到你手动启用。"
                />
              </div>
            </section>

            <section className="rounded-[24px] p-4 sm:p-5" style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)' }}>
              <div className="flex items-center justify-between gap-3">
                <div>
                  <div className="text-sm font-semibold" style={{ color: 'var(--color-text)' }}>规则测试</div>
                  <div className="mt-1 text-xs leading-5" style={{ color: 'var(--color-muted)' }}>
                    使用当前表单配置测试 RSS 会命中哪些条目，但不会真的推送下载。
                  </div>
                </div>
                <button
                  type="button"
                  onClick={handleTest}
                  disabled={testing || !form.rss_url.trim()}
                  className="min-h-11 shrink-0 rounded-full px-5 py-2.5 text-sm font-semibold transition-all disabled:opacity-40"
                  style={{ background: 'rgba(255,255,255,0.06)', color: 'var(--color-text)', border: '1px solid var(--color-border)' }}
                >
                  {testing ? '测试中…' : '测试规则'}
                </button>
              </div>

              {error ? (
                <div className="mt-4 rounded-[18px] px-4 py-3 text-sm" style={{ background: 'rgba(239,125,117,0.08)', border: '1px solid rgba(239,125,117,0.2)', color: 'var(--color-danger)' }}>
                  {error}
                </div>
              ) : null}

              {testResult ? (
                <>
                  <div className="mt-4 grid gap-3 sm:grid-cols-3">
                    {[
                      { label: 'RSS 条目', value: testResult.summary?.total_items ?? 0 },
                      { label: '解析成功', value: testResult.summary?.parsed_items ?? 0 },
                      { label: '最终命中', value: testResult.summary?.matched_items ?? 0 },
                    ].map(item => (
                      <div key={item.label} className="rounded-[18px] px-4 py-3" style={{ background: 'rgba(255,255,255,0.02)', border: '1px solid rgba(255,255,255,0.04)' }}>
                        <div className="text-[11px] font-semibold uppercase tracking-[0.18em]" style={{ color: 'var(--color-muted)' }}>{item.label}</div>
                        <div className="mt-1 text-2xl font-bold tabular-nums" style={{ color: 'var(--color-text)' }}>{item.value}</div>
                      </div>
                    ))}
                  </div>

                  <div className="mt-4 max-h-[48dvh] overflow-y-auto pr-1">
                    <div className="flex flex-col gap-3">
                      {(testResult.matches || []).map((item, index) => (
                        <div key={`${item.title}-${index}`} className="rounded-[18px] px-4 py-3" style={{ background: item.would_push ? 'rgba(74,201,126,0.08)' : 'rgba(255,255,255,0.02)', border: `1px solid ${item.would_push ? 'rgba(74,201,126,0.18)' : 'rgba(255,255,255,0.05)'}` }}>
                          <div className="flex flex-wrap items-center gap-2">
                            <div className="flex-1 text-sm font-semibold leading-6" style={{ color: 'var(--color-text)' }}>{item.title}</div>
                            <span className="rounded-full px-2.5 py-1 text-[11px] font-semibold" style={{ background: item.would_push ? 'rgba(74,201,126,0.15)' : 'rgba(255,255,255,0.04)', color: item.would_push ? 'var(--color-success)' : 'var(--color-muted)' }}>
                              {item.would_push ? '会推送' : '不会推送'}
                            </span>
                          </div>
                          <div className="mt-2 flex flex-wrap gap-2 text-xs">
                            <span className="rounded-full px-2 py-1" style={{ background: 'rgba(255,255,255,0.05)', color: 'var(--color-text)' }}>
                              S{String(item.season_number || 1).padStart(2, '0')}E{item.episode_number ? String(item.episode_number).padStart(2, '0') : '--'}
                            </span>
                            <span className="rounded-full px-2 py-1" style={{ background: item.all_keywords_hit ? 'rgba(74,201,126,0.12)' : 'rgba(255,255,255,0.05)', color: item.all_keywords_hit ? 'var(--color-success)' : 'var(--color-muted)' }}>
                              关键字 {item.keyword_hits?.length || 0}/{parseKeywords(form.keyword_text).length}
                            </span>
                            {item.publish_time ? (
                              <span className="rounded-full px-2 py-1" style={{ background: 'rgba(255,255,255,0.05)', color: 'var(--color-muted)' }}>
                                {item.publish_time}
                              </span>
                            ) : null}
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                </>
              ) : (
                <div className="mt-5 rounded-[20px] px-4 py-5 text-sm leading-6" style={{ background: 'rgba(255,255,255,0.02)', border: '1px solid rgba(255,255,255,0.04)', color: 'var(--color-muted)' }}>
                  先填写季、起始集数和关键字，然后点击“测试规则”，确认当前规则到底会下哪些资源。
                </div>
              )}
            </section>

            <div className="lg:col-span-2 flex items-center justify-end gap-3">
              <button type="button" onClick={handleClose} className="min-h-11 rounded-full px-5 py-2.5 text-sm font-semibold"
                style={{ background: 'rgba(255,255,255,0.05)', color: 'var(--color-text)', border: '1px solid var(--color-border)' }}>
                取消
              </button>
              <button type="submit" disabled={saving} className="min-h-11 rounded-full px-5 py-2.5 text-sm font-semibold disabled:opacity-40"
                style={{ background: 'linear-gradient(135deg, var(--color-accent) 0%, #a56d2c 100%)', color: '#fff' }}>
                {saving ? '保存中…' : mode === 'edit' ? '保存订阅' : '创建订阅'}
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>,
    document.body
  )
}
