import { useState, useEffect } from 'react'
import { RefreshCw, Pause, Play, Trash2, Edit, Rss } from 'lucide-react'
import { checkSubscription, deleteSubscription, listSubscriptions, tmdbGetDetail, updateSubscription } from '@/api'
import { StatePanel } from '@/components/StatePanel'
import { Button } from '@/components/ui/button'
import DetailModal from '@/components/modals/DetailModal'
import SubscriptionModal from '@/components/modals/SubscriptionModal'
import type { Subscription, MediaItem } from '@/types/api'
import { toast } from 'sonner'

function TinyPill({ children, tone = 'default' }: { children: React.ReactNode; tone?: 'default' | 'success' | 'warning' | 'danger' }) {
  const styles = {
    default: 'bg-muted text-muted-foreground',
    success: 'bg-success/20 text-success',
    warning: 'bg-warning/20 text-warning',
    danger: 'bg-destructive/20 text-destructive',
  }[tone]

  return (
    <span className={`inline-flex h-5 items-center rounded-full px-2 text-[10px] font-semibold leading-none ${styles}`}>
      {children}
    </span>
  )
}

function SubscriptionMiniCard({ item, onEdit }: { item: Subscription; onEdit: (item: Subscription) => void }) {
  const posterUrl = item.library?.poster_url || item.tmdb?.poster_url || item.poster_url
  const displayTitle = item.tmdb?.title || item.media_title
  const completionText = item.library?.total_episodes
    ? `${item.library?.in_library_episodes || 0}/${item.library.total_episodes}`
    : null

  return (
    <article
      onClick={() => onEdit(item)}
      className="group relative flex cursor-pointer flex-col overflow-hidden rounded-2xl border bg-card transition-all hover:-translate-y-1"
    >
      <div className="relative aspect-[2/3] overflow-hidden bg-muted">
        {posterUrl ? (
          <img
            src={posterUrl}
            alt={displayTitle}
            className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
            loading="lazy"
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-4xl">
            <Rss className="size-10 text-muted-foreground" />
          </div>
        )}

        <div className="absolute left-2.5 top-2.5 flex flex-wrap gap-2">
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

        <div className="absolute inset-x-0 bottom-0 z-10 bg-gradient-to-t from-black/80 to-transparent p-2.5">
          <div className="flex items-center justify-between gap-3 text-[11px] font-semibold text-white">
            <span className="min-w-0 flex-1 text-left">{`S${String(item.season_number).padStart(2, '0')}`}</span>
            <span className="min-w-0 flex-1 text-center">{`E${String(item.start_episode).padStart(2, '0')}+`}</span>
            <span className="min-w-0 flex-1 text-right">{completionText || '-'}</span>
          </div>
        </div>
      </div>

      <div className="flex h-14 flex-col gap-1 rounded-b-2xl bg-muted p-3 sm:h-[65px]">
        <p className="line-clamp-2 text-sm font-semibold leading-snug">{displayTitle}</p>
      </div>
    </article>
  )
}

function InfoBlock({ label, value, tone = 'default' }: { label: string; value: string; tone?: 'default' | 'success' | 'warning' }) {
  const color = {
    default: '',
    success: 'text-success',
    warning: 'text-warning',
  }[tone]

  return (
    <div className="rounded-2xl border bg-muted/50 p-4">
      <div className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">{label}</div>
      <div className={`mt-1 text-sm font-semibold leading-6 ${color}`}>{value}</div>
    </div>
  )
}

function SubscriptionDetailContent({ item }: { item: Subscription }) {
  return (
    <div className="grid gap-6">
      <section className="mb-1">
        <div className="mb-3 flex items-center justify-between gap-3">
          <h3 className="text-sm font-semibold">订阅信息</h3>
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
          <h3 className="mb-3 text-sm font-semibold">关键词规则</h3>
          <div className="flex flex-wrap gap-2">
            {item.keyword_all.map((keyword) => (
              <TinyPill key={keyword}>{keyword}</TinyPill>
            ))}
          </div>
        </section>
      ) : null}

      {item.recent_hits?.length ? (
        <section className="mt-1">
          <h3 className="mb-3 text-sm font-semibold">最近命中</h3>
          <div className="grid gap-2">
            {item.recent_hits.slice(0, 5).map((hit) => (
              <div key={hit.id} className="rounded-2xl border bg-muted/50 p-4">
                <div className="text-sm font-semibold">{hit.episode_title}</div>
                <div className="mt-1 text-xs text-muted-foreground">
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

interface SubscriptionsPageProps {
  onToast?: (type: 'success' | 'error' | 'warning', title: string, message?: string) => void
  aria2Enabled?: boolean
  u115Authorized?: boolean
}

export default function SubscriptionsPage({
  aria2Enabled = false,
  u115Authorized = false,
}: SubscriptionsPageProps) {
  const [items, setItems] = useState<Subscription[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [editingItem, setEditingItem] = useState<Subscription | null>(null)
  const [selectedItem, setSelectedItem] = useState<Subscription | null>(null)
  const [selectedDetail, setSelectedDetail] = useState<MediaItem | null>(null)
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
      setError((err as { response?: { data?: { detail?: string } } })?.response?.data?.detail || '加载失败')
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
        } as MediaItem)
      })
      .catch(() => {
        if (cancelled) return
        setSelectedDetail({
          ...selectedItem,
          title: selectedItem.tmdb?.title || selectedItem.media_title,
          original_title: selectedItem.tmdb?.original_title || '',
          poster_url: selectedItem.library?.poster_url || selectedItem.tmdb?.poster_url || selectedItem.poster_url,
          backdrop_url: selectedItem.tmdb?.backdrop_url || undefined,
          overview: selectedItem.tmdb?.overview || '',
          rating: selectedItem.tmdb?.rating || 0,
          status: selectedItem.tmdb?.status || '',
          year: selectedItem.library?.year || selectedItem.tmdb?.release_date?.slice(0, 4) || '',
          in_library: !!selectedItem.library?.in_library,
        } as MediaItem)
      })
      .finally(() => {
        if (!cancelled) setDetailLoading(false)
      })
    return () => { cancelled = true }
  }, [selectedItem])

  const filteredItems = items.filter((item) => {
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
      toast.success('检查完成', { description: `推送 ${res.data?.result?.pushed ?? 0} 条，跳过 ${res.data?.result?.skipped ?? 0} 条` })
      await loadData()
    } catch (err) {
      toast.error('检查失败', { description: (err as { response?: { data?: { detail?: string } } })?.response?.data?.detail || '未知错误' })
    } finally {
      setDetailActionLoading('')
    }
  }

  async function handleToggleSelected() {
    if (!selectedItem) return
    setDetailActionLoading('toggle')
    try {
      await updateSubscription(selectedItem.id, { ...selectedItem, enabled: !selectedItem.enabled })
      toast.success(selectedItem.enabled ? '订阅已暂停' : '订阅已启用', { description: selectedItem.name })
      await loadData()
      setSelectedItem((prev) => prev ? { ...prev, enabled: !prev.enabled } : prev)
    } catch (err) {
      toast.error('状态更新失败', { description: (err as { response?: { data?: { detail?: string } } })?.response?.data?.detail || '未知错误' })
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
      toast.success('订阅已删除', { description: selectedItem.name })
      setSelectedItem(null)
      setSelectedDetail(null)
      await loadData()
    } catch (err) {
      toast.error('删除失败', { description: (err as { response?: { data?: { detail?: string } } })?.response?.data?.detail || '未知错误' })
    } finally {
      setDetailActionLoading('')
    }
  }

  return (
    <div className="flex-1">
      <section className="flex flex-1 flex-col rounded-2xl border bg-card p-4 sm:rounded-3xl sm:p-7">
        <div className="mb-6 flex flex-col gap-3 sm:mb-7 sm:gap-5 lg:flex-row lg:items-end lg:justify-between">
          <div className="max-w-2xl">
            <div className="mb-1 text-[10px] font-semibold uppercase tracking-widest text-brand sm:mb-2 sm:text-[11px]">
              Subscription Center
            </div>
            <h2 className="text-2xl font-bold sm:text-[34px]">RSS 订阅列表</h2>
            <p className="mt-2 text-sm leading-7 text-muted-foreground">
              RSS 订阅详情
            </p>
          </div>
          <Button variant="outline" size="sm" onClick={loadData} className="gap-2">
            <RefreshCw className="size-4" />
            刷新列表
          </Button>
        </div>

        <div className="mb-4 flex flex-wrap gap-2 sm:mb-5">
          {[
            ['all', `全部 ${items.length}`],
            ['enabled', `启用 ${items.filter((item) => item.enabled).length}`],
            ['in_library', `已入库 ${items.filter((item) => item.library?.in_library).length}`],
            ['not_in_library', `未入库 ${items.filter((item) => !item.library?.in_library).length}`],
          ].map(([value, label]) => (
            <Button
              key={value}
              variant={filter === value ? 'default' : 'outline'}
              size="sm"
              onClick={() => setFilter(value)}
              className="rounded-full"
            >
              {label}
            </Button>
          ))}
        </div>

        <div className="flex-1 min-h-0 overflow-y-auto">
          {loading ? (
            <StatePanel title="正在加载订阅列表" description="正在读取已保存的 RSS 订阅与命中状态。" compact />
          ) : error ? (
            <StatePanel icon="!" title="加载订阅失败" description={error} tone="danger" compact />
          ) : items.length === 0 ? (
            <StatePanel icon="📡" title="还没有 RSS 订阅" description="去资源检索页找到对应字幕组后，点击「订阅 RSS」即可创建。" />
          ) : filteredItems.length === 0 ? (
            <StatePanel icon="🗂" title="当前筛选下没有订阅" description="换一个筛选条件，或者先创建新的订阅规则。" compact />
          ) : (
            <div className="grid grid-cols-2 gap-3 pb-6 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-10">
              {filteredItems.map((item) => (
                <SubscriptionMiniCard key={item.id} item={item} onEdit={setSelectedItem} />
              ))}
            </div>
          )}
        </div>
      </section>

      {selectedItem && (
        <DetailModal
          item={selectedDetail || {
            ...selectedItem,
            title: selectedItem.tmdb?.title || selectedItem.media_title,
            original_title: selectedItem.tmdb?.original_title || '',
            poster_url: selectedItem.library?.poster_url || selectedItem.tmdb?.poster_url || selectedItem.poster_url,
            backdrop_url: selectedItem.tmdb?.backdrop_url || undefined,
            overview: selectedItem.tmdb?.overview || '',
            rating: selectedItem.tmdb?.rating || 0,
            status: selectedItem.tmdb?.status || '',
            year: selectedItem.library?.year || selectedItem.tmdb?.release_date?.slice(0, 4) || '',
            in_library: !!selectedItem.library?.in_library,
          } as MediaItem}
          open={!!selectedItem}
          onOpenChange={(open) => {
            if (!open) {
              setSelectedItem(null)
              setSelectedDetail(null)
            }
          }}
          contentSlot={<SubscriptionDetailContent item={selectedItem} />}
          footerSlot={
            <div className="flex flex-wrap items-center justify-end gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => selectedItem && setEditingItem(selectedItem)}
                disabled={detailActionLoading !== ''}
              >
                <Edit className="size-4" />
                编辑订阅
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={handleCheckSelected}
                disabled={detailActionLoading !== ''}
              >
                {detailActionLoading === 'check' ? '检查中…' : '立即检查'}
              </Button>
              <Button
                variant={selectedItem.enabled ? 'outline' : 'default'}
                size="sm"
                onClick={handleToggleSelected}
                disabled={detailActionLoading !== ''}
              >
                {selectedItem.enabled ? <Pause className="size-4" /> : <Play className="size-4" />}
                {detailActionLoading === 'toggle' ? '处理中…' : (selectedItem.enabled ? '暂停订阅' : '启用订阅')}
              </Button>
              <Button
                variant="destructive"
                size="sm"
                onClick={handleDeleteSelected}
                disabled={detailActionLoading !== ''}
              >
                <Trash2 className="size-4" />
                {detailActionLoading === 'delete' ? '删除中…' : '删除订阅'}
              </Button>
            </div>
          }
          loadingSlot={detailLoading}
        />
      )}

      <SubscriptionModal
        mode="edit"
        initialValue={editingItem || undefined}
        aria2Enabled={aria2Enabled}
        u115Authorized={u115Authorized}
        open={!!editingItem}
        onOpenChange={(open) => { if (!open) setEditingItem(null) }}
        onSaved={async () => { await loadData() }}
      />
    </div>
  )
}
