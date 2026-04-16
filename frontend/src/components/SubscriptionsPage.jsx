import { useEffect, useRef, useState } from 'react'
import { checkSubscription, deleteSubscription, listSubscriptions, updateSubscription } from '../api'
import SubscriptionModal from './SubscriptionModal'
import { StatePanel } from './StatePanel'

function TinyPill({ children, tone = 'default' }) {
  const styles = {
    default: {
      background: 'rgba(255,255,255,0.05)',
      color: 'var(--color-text)',
      border: '1px solid rgba(255,255,255,0.06)',
    },
    success: {
      background: 'rgba(74,201,126,0.12)',
      color: 'var(--color-success)',
      border: '1px solid rgba(74,201,126,0.18)',
    },
    warning: {
      background: 'rgba(245,196,81,0.12)',
      color: 'var(--color-warning)',
      border: '1px solid rgba(245,196,81,0.18)',
    },
    danger: {
      background: 'rgba(239,125,117,0.12)',
      color: 'var(--color-danger)',
      border: '1px solid rgba(239,125,117,0.18)',
    },
  }[tone]

  return (
    <span
      className="inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-semibold"
      style={styles}
    >
      {children}
    </span>
  )
}

function SubscriptionMiniCard({ item, onToast, onRefresh, onEdit }) {
  const [checking, setChecking] = useState(false)
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [menuOpen, setMenuOpen] = useState(false)
  const menuRef = useRef(null)

  useEffect(() => {
    if (!menuOpen) return undefined

    function handlePointerDown(event) {
      if (!menuRef.current?.contains(event.target)) {
        setMenuOpen(false)
      }
    }

    document.addEventListener('pointerdown', handlePointerDown)
    return () => document.removeEventListener('pointerdown', handlePointerDown)
  }, [menuOpen])

  async function handleCheck(e) {
    e.stopPropagation()
    setMenuOpen(false)
    setChecking(true)
    try {
      const res = await checkSubscription(item.id)
      onToast?.(
        'success',
        '检查完成',
        `推送 ${res.data?.result?.pushed ?? 0} 条，跳过 ${res.data?.result?.skipped ?? 0} 条`
      )
      onRefresh?.()
    } catch (err) {
      onToast?.('error', '检查失败', err?.response?.data?.detail || err.message)
    } finally {
      setChecking(false)
    }
  }

  async function handleToggle(e) {
    e.stopPropagation()
    setMenuOpen(false)
    setSaving(true)
    try {
      await updateSubscription(item.id, { ...item, enabled: !item.enabled })
      onToast?.('success', item.enabled ? '订阅已暂停' : '订阅已启用', item.name)
      onRefresh?.()
    } catch (err) {
      onToast?.('error', '状态更新失败', err?.response?.data?.detail || err.message)
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete(e) {
    e.stopPropagation()
    setMenuOpen(false)
    if (!window.confirm(`确认删除订阅「${item.name}」吗？`)) return
    setDeleting(true)
    try {
      await deleteSubscription(item.id)
      onToast?.('success', '订阅已删除', item.name)
      onRefresh?.()
    } catch (err) {
      onToast?.('error', '删除失败', err?.response?.data?.detail || err.message)
    } finally {
      setDeleting(false)
    }
  }

  return (
    <article
      onClick={() => onEdit(item)}
      className="group relative flex cursor-pointer flex-col overflow-hidden rounded-[20px] transition-all duration-200 sm:rounded-[22px]"
      style={{
        background: 'linear-gradient(180deg, rgba(20, 37, 59, 0.96) 0%, rgba(13, 26, 44, 0.98) 100%)',
        border: '1px solid var(--color-border)',
        boxShadow: '0 18px 38px rgba(3, 11, 22, 0.22)',
      }}
    >
      <div className="relative aspect-square overflow-hidden bg-gray-800">
        {item.poster_url ? (
          <img
            src={item.poster_url}
            alt={item.media_title}
            className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
            loading="lazy"
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-4xl" style={{ background: 'var(--color-surface)' }}>
            📡
          </div>
        )}

        <div className="absolute left-2.5 top-2.5 flex flex-wrap gap-1 sm:left-3 sm:top-3">
          <TinyPill tone={item.enabled ? 'success' : 'warning'}>
            {item.enabled ? '启用' : '暂停'}
          </TinyPill>
          <TinyPill>{item.push_target === 'u115' ? '云下载' : '下载'}</TinyPill>
        </div>

        <div className="absolute right-2.5 top-2.5 rounded-full px-2 py-1 text-[10px] font-bold sm:right-3 sm:top-3 sm:text-[11px]"
          style={{ background: 'rgba(4, 11, 21, 0.84)', color: 'var(--color-muted)', border: '1px solid rgba(255,255,255,0.08)' }}>
          {item.hit_count || 0} 命中
        </div>

        <div
          className="absolute inset-x-0 bottom-0 z-10 p-2.5 sm:p-3"
          style={{ background: 'linear-gradient(to top, rgba(3, 9, 17, 0.88) 0%, rgba(3, 9, 17, 0.18) 100%)' }}
        >
          <div className="flex flex-wrap gap-1">
            <TinyPill>S{String(item.season_number).padStart(2, '0')}</TinyPill>
            <TinyPill>E{String(item.start_episode).padStart(2, '0')}+</TinyPill>
          </div>
        </div>
      </div>

      <div className="flex flex-col px-3 pt-3 pb-3 sm:px-3.5 sm:pt-3.5 sm:pb-3.5" style={{ minHeight: 92 }}>
        <p className="line-clamp-2 text-[13px] font-semibold leading-snug sm:text-sm" style={{ color: 'var(--color-text)' }}>
          {item.media_title}
        </p>
        <div className="mt-1 line-clamp-1 text-[11px] sm:text-xs" style={{ color: 'var(--color-muted)' }}>
          {item.subgroup_name || item.site?.toUpperCase()}
        </div>

        <div className="mt-2 flex min-h-[24px] flex-wrap gap-1">
          {(item.keyword_all || []).slice(0, 3).map(keyword => (
            <TinyPill key={keyword}>{keyword}</TinyPill>
          ))}
          {(item.keyword_all || []).length > 3 ? (
            <TinyPill>+{item.keyword_all.length - 3}</TinyPill>
          ) : null}
        </div>

        {item.last_error ? (
          <div className="mt-auto pt-2 line-clamp-2 text-[11px] leading-4" style={{ color: 'var(--color-danger)' }}>
            {item.last_error}
          </div>
        ) : null}
      </div>

      <div ref={menuRef} className="absolute bottom-3 right-3 z-30">
        <button
          onClick={(e) => {
            e.stopPropagation()
            setMenuOpen(prev => !prev)
          }}
          className="flex h-9 w-9 items-center justify-center rounded-full transition-colors"
          style={{
            background: 'rgba(10, 19, 32, 0.9)',
            color: 'var(--color-text)',
            border: '1px solid rgba(255,255,255,0.08)',
            boxShadow: '0 10px 22px rgba(3, 11, 22, 0.26)',
          }}
          aria-label="打开订阅操作菜单"
          title="更多操作"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
            <circle cx="5" cy="12" r="1.8" />
            <circle cx="12" cy="12" r="1.8" />
            <circle cx="19" cy="12" r="1.8" />
          </svg>
        </button>

        {menuOpen ? (
          <div
            className="absolute bottom-11 right-0 min-w-[124px] overflow-hidden rounded-2xl"
            style={{
              background: 'rgba(10, 19, 32, 0.96)',
              border: '1px solid var(--color-border)',
              boxShadow: '0 18px 38px rgba(3, 11, 22, 0.28)',
              backdropFilter: 'blur(14px)',
            }}
          >
            {[
              { key: 'check', label: checking ? '检查中…' : '立即检查', onClick: handleCheck, disabled: checking },
              { key: 'toggle', label: saving ? '处理中…' : (item.enabled ? '暂停订阅' : '启用订阅'), onClick: handleToggle, disabled: saving },
              { key: 'delete', label: deleting ? '删除中…' : '删除订阅', onClick: handleDelete, disabled: deleting, danger: true },
            ].map(action => (
              <button
                key={action.key}
                onClick={action.onClick}
                disabled={action.disabled}
                className="flex w-full items-center px-4 py-3 text-left text-xs font-semibold transition-colors disabled:opacity-40"
                style={{
                  color: action.danger ? 'var(--color-danger)' : 'var(--color-text)',
                  borderBottom: action.key !== 'delete' ? '1px solid rgba(255,255,255,0.05)' : 'none',
                }}
              >
                {action.label}
              </button>
            ))}
          </div>
        ) : null}
      </div>
    </article>
  )
}

export default function SubscriptionsPage({ onToast, aria2Enabled = false, u115Authorized = false }) {
  const [items, setItems] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [editingItem, setEditingItem] = useState(null)

  async function loadData() {
    setLoading(true)
    setError('')
    try {
      const res = await listSubscriptions()
      setItems(res.data?.subscriptions || [])
    } catch (err) {
      setError(err?.response?.data?.detail || err.message || '加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadData()
  }, [])

  return (
    <div className="flex-1">
      <section className="page-panel panel-surface flex flex-1 flex-col rounded-[22px] px-3 py-4 sm:rounded-[32px] sm:px-7 sm:py-7">
        <div className="mb-6 flex flex-col gap-3 sm:mb-7 sm:gap-5 lg:flex-row lg:items-end lg:justify-between">
          <div className="max-w-2xl">
            <div className="mb-1 text-[10px] font-semibold uppercase tracking-[0.24em] sm:mb-2 sm:text-[11px]" style={{ color: 'var(--color-accent-hover)' }}>
              Subscription Center
            </div>
            <h2 className="text-2xl font-bold text-white mb-2 sm:text-[34px]">RSS 订阅列表</h2>
            <p className="mt-2 text-sm leading-7" style={{ color: 'var(--color-muted)' }}>
              以媒体库风格的小卡片浏览 RSS 订阅。点击卡片即可编辑，关键状态与规则会直接显示在卡面上。
            </p>
          </div>
          <button
            onClick={loadData}
            className="min-h-11 rounded-full px-5 py-2.5 text-sm font-semibold self-start lg:self-auto"
            style={{ background: 'rgba(255,255,255,0.06)', color: 'var(--color-text)', border: '1px solid var(--color-border)' }}
          >
            刷新列表
          </button>
        </div>

        <div className="flex-1 min-h-0 overflow-y-auto" style={{ scrollbarWidth: 'thin', scrollbarColor: 'var(--color-border) transparent' }}>
          {loading ? (
            <StatePanel title="正在加载订阅列表" description="正在读取已保存的 RSS 订阅与命中状态。" compact />
          ) : error ? (
            <StatePanel icon="!" title="加载订阅失败" description={error} tone="danger" compact />
          ) : items.length === 0 ? (
            <StatePanel icon="📡" title="还没有 RSS 订阅" description="去资源检索页找到对应字幕组后，点击“订阅 RSS”即可创建。" />
          ) : (
            <div className="grid grid-cols-2 gap-3 pb-6 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-8">
              {items.map(item => (
                <SubscriptionMiniCard
                  key={item.id}
                  item={item}
                  onToast={onToast}
                  onRefresh={loadData}
                  onEdit={setEditingItem}
                />
              ))}
            </div>
          )}
        </div>
      </section>

      {editingItem ? (
        <SubscriptionModal
          mode="edit"
          initialValue={editingItem}
          aria2Enabled={aria2Enabled}
          u115Authorized={u115Authorized}
          onClose={() => setEditingItem(null)}
          onToast={onToast}
          onSaved={loadData}
        />
      ) : null}
    </div>
  )
}
