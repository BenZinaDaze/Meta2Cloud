import { useEffect, useState } from 'react'
import { checkSubscription, deleteSubscription, listSubscriptions, tmdbGetDetail, updateSubscription } from '../api'
import DetailModal from './DetailModal'
import SubscriptionModal from './SubscriptionModal'
import { StatePanel } from './StatePanel'

function TinyPill({ children, tone = 'default' }) {
  const styles = {
    default: {
      background: 'rgba(4, 11, 21, 0.82)',
      color: 'rgba(255,255,255,0.94)',
      border: '1px solid rgba(255,255,255,0.12)',
    },
    success: {
      background: 'rgba(8, 45, 28, 0.84)',
      color: '#7ee2a8',
      border: '1px solid rgba(74,201,126,0.3)',
    },
    warning: {
      background: 'rgba(58, 42, 8, 0.84)',
      color: '#f5cf78',
      border: '1px solid rgba(245,196,81,0.32)',
    },
    danger: {
      background: 'rgba(63, 22, 20, 0.84)',
      color: '#ff9f97',
      border: '1px solid rgba(239,125,117,0.32)',
    },
  }[tone]

  return (
    <span
      className="inline-flex h-5.5 items-center rounded-full px-1.5 text-[9px] font-semibold leading-none sm:h-6 sm:px-2 sm:text-[10px]"
      style={{ ...styles, textShadow: '0 1px 2px rgba(0,0,0,0.28)' }}
    >
      {children}
    </span>
  )
}

function SubscriptionMiniCard({ item, onEdit }) {
  const posterUrl = item.library?.poster_url || item.poster_url
  const displayTitle = item.tmdb?.title || item.media_title
  const completionText = item.library?.total_episodes
    ? `${item.library?.in_library_episodes || 0}/${item.library.total_episodes}`
    : null

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
      <div className="relative aspect-[2/3] overflow-hidden bg-gray-800">
        {posterUrl ? (
          <img
            src={posterUrl}
            alt={displayTitle}
            className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
            loading="lazy"
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-4xl" style={{ background: 'var(--color-surface)' }}>
            📡
          </div>
        )}

        <div className="absolute left-2.5 top-2.5 flex flex-wrap gap-2 sm:left-3 sm:top-3">
          <TinyPill tone={item.enabled ? 'success' : 'warning'}>
            {item.enabled ? '启用' : '暂停'}
          </TinyPill>
          <TinyPill>
            {item.push_target === 'u115' ? '云下载' : '下载'}
          </TinyPill>
          <TinyPill tone="warning">
            {item.hit_count || 0} 命中
          </TinyPill>
        </div>

        <div
          className="absolute inset-x-0 bottom-0 z-10 p-2.5 sm:p-3"
          style={{ background: 'linear-gradient(to top, rgba(3, 9, 17, 0.88) 0%, rgba(3, 9, 17, 0.18) 100%)' }}
        >
          <div
            className="flex items-center justify-between gap-3 text-[11px] font-semibold sm:text-xs"
            style={{ color: 'rgba(255,255,255,0.92)', textShadow: '0 1px 2px rgba(0,0,0,0.35)' }}
          >
            <span className="min-w-0 flex-1 text-left">{`S${String(item.season_number).padStart(2, '0')}`}</span>
            <span className="min-w-0 flex-1 text-center">{`E${String(item.start_episode).padStart(2, '0')}+`}</span>
            <span className="min-w-0 flex-1 text-right">{completionText || '-'}</span>
          </div>
        </div>
      </div>

      <div className="flex flex-col px-3 pt-3 pb-3 sm:px-3.5 sm:pt-3.5 sm:pb-3.5" style={{ minHeight: 76 }}>
        <p className="line-clamp-2 text-[13px] font-semibold leading-snug sm:text-sm" style={{ color: 'var(--color-text)' }}>
          {displayTitle}
        </p>

        {item.last_error ? (
          <div className="mt-auto pt-2 line-clamp-2 text-[11px] leading-4" style={{ color: 'var(--color-danger)' }}>
            {item.last_error}
          </div>
        ) : null}
      </div>

    </article>
  )
}

function InfoBlock({ label, value, tone = 'default' }) {
  const color = {
    default: 'var(--color-text)',
    success: 'var(--color-success)',
    warning: 'var(--color-warning)',
  }[tone]

  return (
    <div
      className="rounded-[18px] px-4 py-3"
      style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.06)' }}
    >
      <div className="text-[11px] font-semibold uppercase tracking-[0.18em]" style={{ color: 'var(--color-muted)' }}>
        {label}
      </div>
      <div className="mt-1 text-sm font-semibold leading-6" style={{ color }}>
        {value}
      </div>
    </div>
  )
}

function SubscriptionDetailContent({ item }) {
  return (
    <div className="grid gap-6">
      <section className="mb-1">
        <div className="mb-3 flex items-center justify-between gap-3">
          <h3 className="text-sm font-semibold" style={{ color: 'var(--color-text)' }}>订阅信息</h3>
        </div>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <InfoBlock label="订阅名称" value={item.name || '-'} />
          <InfoBlock label="站点 / 字幕组" value={[item.site?.toUpperCase(), item.subgroup_name].filter(Boolean).join(' · ') || '-'} />
          <InfoBlock label="推送目标" value={item.push_target === 'u115' ? '115 云下载' : '下载器'} />
          <InfoBlock label="订阅范围" value={`S${String(item.season_number).padStart(2, '0')} · E${String(item.start_episode).padStart(2, '0')}+`} />
          <InfoBlock label="订阅状态" value={item.enabled ? '启用中' : '已暂停'} tone={item.enabled ? 'success' : 'warning'} />
          <InfoBlock label="命中次数" value={`${item.hit_count || 0} 次`} />
        </div>
      </section>

      {item.keyword_all?.length ? (
        <section className="mt-1">
          <h3 className="mb-3 text-sm font-semibold" style={{ color: 'var(--color-text)' }}>关键词规则</h3>
          <div className="flex flex-wrap gap-2">
            {item.keyword_all.map(keyword => (
              <TinyPill key={keyword}>{keyword}</TinyPill>
            ))}
          </div>
        </section>
      ) : null}

      {item.recent_hits?.length ? (
        <section className="mt-1">
          <h3 className="mb-3 text-sm font-semibold" style={{ color: 'var(--color-text)' }}>最近命中</h3>
          <div className="grid gap-2">
            {item.recent_hits.slice(0, 5).map(hit => (
              <div
                key={hit.id}
                className="rounded-[18px] px-4 py-3"
                style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.06)' }}
              >
                <div className="text-sm font-semibold" style={{ color: 'var(--color-text)' }}>{hit.episode_title}</div>
                <div className="mt-1 text-xs" style={{ color: 'var(--color-muted)' }}>
                  {[
                    hit.season_number ? `S${String(hit.season_number).padStart(2, '0')}` : null,
                    hit.episode_number ? `E${String(hit.episode_number).padStart(2, '0')}` : null,
                    hit.push_status || null,
                    hit.created_at ? new Date(hit.created_at).toLocaleString('zh-CN', { hour12: false }) : null,
                  ].filter(Boolean).join(' · ')}
                </div>
              </div>
            ))}
          </div>
        </section>
      ) : null}

    </div>
  )
}

export default function SubscriptionsPage({ onToast, aria2Enabled = false, u115Authorized = false }) {
  const [items, setItems] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [editingItem, setEditingItem] = useState(null)
  const [selectedItem, setSelectedItem] = useState(null)
  const [selectedDetail, setSelectedDetail] = useState(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailActionLoading, setDetailActionLoading] = useState('')
  const [filter, setFilter] = useState('all')

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

  useEffect(() => {
    if (!selectedItem?.tmdb_id || !selectedItem?.media_type) {
      setSelectedDetail(null)
      return
    }
    let cancelled = false
    setDetailLoading(true)
    setSelectedDetail(null)
    tmdbGetDetail(selectedItem.media_type, selectedItem.tmdb_id)
      .then((res) => {
        if (cancelled) return
        const detail = res.data?.detail || {}
        setSelectedDetail({
          ...selectedItem,
          ...detail,
          tmdb_id: selectedItem.tmdb_id,
          media_type: selectedItem.media_type,
          title: detail.title || selectedItem.tmdb?.title || selectedItem.media_title,
          original_title: detail.original_title || selectedItem.tmdb?.original_title || '',
          poster_url: detail.poster_url || selectedItem.library?.poster_url || selectedItem.poster_url,
          backdrop_url: detail.backdrop_url || selectedItem.tmdb?.backdrop_url || null,
          overview: detail.overview || selectedItem.tmdb?.overview || '',
          rating: detail.rating ?? selectedItem.tmdb?.rating ?? 0,
          status: detail.status || selectedItem.tmdb?.status || '',
          year: detail.year || selectedItem.library?.year || selectedItem.tmdb?.release_date?.slice(0, 4) || '',
          in_library: selectedItem.library?.in_library ?? detail.in_library,
          library: selectedItem.library,
          tmdb: selectedItem.tmdb,
          recent_hits: selectedItem.recent_hits,
          hit_count: selectedItem.hit_count,
          name: selectedItem.name,
          site: selectedItem.site,
          subgroup_name: selectedItem.subgroup_name,
          push_target: selectedItem.push_target,
          season_number: selectedItem.season_number,
          start_episode: selectedItem.start_episode,
          enabled: selectedItem.enabled,
          keyword_all: selectedItem.keyword_all,
          media_title: selectedItem.media_title,
        })
      })
      .catch(() => {
        if (cancelled) return
        setSelectedDetail({
          ...selectedItem,
          title: selectedItem.tmdb?.title || selectedItem.media_title,
          original_title: selectedItem.tmdb?.original_title || '',
          poster_url: selectedItem.library?.poster_url || selectedItem.poster_url,
          backdrop_url: null,
          overview: selectedItem.tmdb?.overview || '',
          rating: selectedItem.tmdb?.rating || 0,
          status: selectedItem.tmdb?.status || '',
          year: selectedItem.library?.year || selectedItem.tmdb?.release_date?.slice(0, 4) || '',
          in_library: !!selectedItem.library?.in_library,
        })
      })
      .finally(() => {
        if (!cancelled) setDetailLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [selectedItem])

  const filteredItems = items.filter(item => {
    if (filter === 'all') return true
    if (filter === 'in_library') return !!item.library?.in_library
    if (filter === 'not_in_library') return !item.library?.in_library
    if (filter === 'enabled') return !!item.enabled
    if (filter === 'missing') {
      const total = item.library?.total_episodes
      const inLibraryEpisodes = item.library?.in_library_episodes || 0
      return !!item.library?.in_library && !!total && inLibraryEpisodes < total
    }
    return true
  })

  async function handleCheckSelected() {
    if (!selectedItem) return
    setDetailActionLoading('check')
    try {
      const res = await checkSubscription(selectedItem.id)
      onToast?.('success', '检查完成', `推送 ${res.data?.result?.pushed ?? 0} 条，跳过 ${res.data?.result?.skipped ?? 0} 条`)
      await loadData()
    } catch (err) {
      onToast?.('error', '检查失败', err?.response?.data?.detail || err.message)
    } finally {
      setDetailActionLoading('')
    }
  }

  async function handleToggleSelected() {
    if (!selectedItem) return
    setDetailActionLoading('toggle')
    try {
      await updateSubscription(selectedItem.id, { ...selectedItem, enabled: !selectedItem.enabled })
      onToast?.('success', selectedItem.enabled ? '订阅已暂停' : '订阅已启用', selectedItem.name)
      await loadData()
      setSelectedItem(prev => prev ? { ...prev, enabled: !prev.enabled } : prev)
      setSelectedDetail(prev => prev ? { ...prev, enabled: !prev.enabled } : prev)
    } catch (err) {
      onToast?.('error', '状态更新失败', err?.response?.data?.detail || err.message)
    } finally {
      setDetailActionLoading('')
    }
  }

  async function handleDeleteSelected() {
    if (!selectedItem) return
    if (!window.confirm(`确认删除订阅「${selectedItem.name}」吗？`)) return
    setDetailActionLoading('delete')
    try {
      await deleteSubscription(selectedItem.id)
      onToast?.('success', '订阅已删除', selectedItem.name)
      setSelectedItem(null)
      setSelectedDetail(null)
      await loadData()
    } catch (err) {
      onToast?.('error', '删除失败', err?.response?.data?.detail || err.message)
    } finally {
      setDetailActionLoading('')
    }
  }

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
              RSS 订阅详情
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

        <div className="mb-4 flex flex-wrap gap-2 sm:mb-5">
          {[
            ['all', `全部 ${items.length}`],
            ['enabled', `启用 ${items.filter(item => item.enabled).length}`],
            ['in_library', `已入库 ${items.filter(item => item.library?.in_library).length}`],
            ['not_in_library', `未入库 ${items.filter(item => !item.library?.in_library).length}`],
            ['missing', `未补全 ${items.filter(item => {
              const total = item.library?.total_episodes
              const inLibraryEpisodes = item.library?.in_library_episodes || 0
              return !!item.library?.in_library && !!total && inLibraryEpisodes < total
            }).length}`],
          ].map(([value, label]) => (
            <button
              key={value}
              onClick={() => setFilter(value)}
              className="rounded-full px-3 py-2 text-xs font-semibold transition-colors"
              style={{
                background: filter === value ? 'rgba(227,183,120,0.14)' : 'rgba(255,255,255,0.04)',
                color: filter === value ? 'var(--color-accent-hover)' : 'var(--color-muted)',
                border: filter === value ? '1px solid rgba(227,183,120,0.24)' : '1px solid rgba(255,255,255,0.06)',
              }}
            >
              {label}
            </button>
          ))}
        </div>

        <div className="flex-1 min-h-0 overflow-y-auto" style={{ scrollbarWidth: 'thin', scrollbarColor: 'var(--color-border) transparent' }}>
          {loading ? (
            <StatePanel title="正在加载订阅列表" description="正在读取已保存的 RSS 订阅与命中状态。" compact />
          ) : error ? (
            <StatePanel icon="!" title="加载订阅失败" description={error} tone="danger" compact />
          ) : items.length === 0 ? (
            <StatePanel icon="📡" title="还没有 RSS 订阅" description="去资源检索页找到对应字幕组后，点击“订阅 RSS”即可创建。" />
          ) : filteredItems.length === 0 ? (
            <StatePanel icon="🗂" title="当前筛选下没有订阅" description="换一个筛选条件，或者先创建新的订阅规则。" compact />
          ) : (
            <div className="grid grid-cols-2 gap-3 pb-6 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-8">
              {filteredItems.map(item => (
                <SubscriptionMiniCard
                  key={item.id}
                  item={item}
                  onEdit={setSelectedItem}
                />
              ))}
            </div>
          )}
        </div>
      </section>

      {selectedItem ? (
        <DetailModal
          item={selectedDetail || {
            ...selectedItem,
            title: selectedItem.tmdb?.title || selectedItem.media_title,
            original_title: selectedItem.tmdb?.original_title || '',
            poster_url: selectedItem.library?.poster_url || selectedItem.poster_url,
            overview: selectedItem.tmdb?.overview || '',
            rating: selectedItem.tmdb?.rating || 0,
            status: selectedItem.tmdb?.status || '',
            year: selectedItem.library?.year || selectedItem.tmdb?.release_date?.slice(0, 4) || '',
            in_library: !!selectedItem.library?.in_library,
          }}
          onClose={() => {
            setSelectedItem(null)
            setSelectedDetail(null)
          }}
          titleActionSlot={(
            null
          )}
          headerRightSlot={null}
          contentSlot={(
            <SubscriptionDetailContent
              item={selectedDetail || selectedItem}
            />
          )}
          footerSlot={(
            <div className="flex flex-wrap items-center justify-end gap-2">
              <button
                type="button"
                onClick={() => setEditingItem(selectedItem)}
                disabled={detailActionLoading !== ''}
                className="rounded-full px-4 py-2 text-xs font-semibold transition-all duration-150 disabled:opacity-50"
                style={{
                  background: 'rgba(227,183,120,0.14)',
                  color: 'var(--color-accent-hover)',
                  border: '1px solid rgba(227,183,120,0.24)',
                }}
              >
                编辑订阅
              </button>
              <button
                type="button"
                onClick={handleCheckSelected}
                disabled={detailActionLoading !== ''}
                className="rounded-full px-4 py-2 text-xs font-semibold transition-all duration-150 disabled:opacity-50"
                style={{
                  background: 'rgba(255,255,255,0.05)',
                  color: 'var(--color-text)',
                  border: '1px solid rgba(255,255,255,0.08)',
                }}
              >
                {detailActionLoading === 'check' ? '检查中…' : '立即检查'}
              </button>
              <button
                type="button"
                onClick={handleToggleSelected}
                disabled={detailActionLoading !== ''}
                className="rounded-full px-4 py-2 text-xs font-semibold transition-all duration-150 disabled:opacity-50"
                style={{
                  background: selectedItem.enabled ? 'rgba(245,196,81,0.12)' : 'rgba(74,201,126,0.12)',
                  color: selectedItem.enabled ? 'var(--color-warning)' : 'var(--color-success)',
                  border: `1px solid ${selectedItem.enabled ? 'rgba(245,196,81,0.18)' : 'rgba(74,201,126,0.18)'}`,
                }}
              >
                {detailActionLoading === 'toggle' ? '处理中…' : (selectedItem.enabled ? '暂停订阅' : '启用订阅')}
              </button>
              <button
                type="button"
                onClick={handleDeleteSelected}
                disabled={detailActionLoading !== ''}
                className="rounded-full px-4 py-2 text-xs font-semibold transition-all duration-150 disabled:opacity-50"
                style={{
                  background: 'rgba(239,125,117,0.12)',
                  color: 'var(--color-danger)',
                  border: '1px solid rgba(239,125,117,0.18)',
                }}
              >
                {detailActionLoading === 'delete' ? '删除中…' : '删除订阅'}
              </button>
            </div>
          )}
          loadingSlot={detailLoading ? <span /> : null}
        />
      ) : null}

      {editingItem ? (
        <SubscriptionModal
          mode="edit"
          initialValue={editingItem}
          aria2Enabled={aria2Enabled}
          u115Authorized={u115Authorized}
          onClose={() => setEditingItem(null)}
          onToast={onToast}
          onSaved={async () => {
            await loadData()
          }}
        />
      ) : null}
    </div>
  )
}
