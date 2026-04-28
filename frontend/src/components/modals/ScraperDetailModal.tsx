import { useEffect, useState } from 'react'
import { tmdbGetDetail, refreshMediaItem } from '@/api'
import { toast } from 'sonner'
import DetailModal from './DetailModal'
import { RefreshCw, Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import type { MediaItem } from '@/types/api'

interface ScraperDetailModalProps {
  item: MediaItem | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onSearchResources?: (item: MediaItem) => void
}

export default function ScraperDetailModal({
  item: initialItem,
  open,
  onOpenChange,
  onSearchResources,
}: ScraperDetailModalProps) {
  const [item, setItem] = useState<MediaItem | null>(null)
  const [loadingDetail, setLoadingDetail] = useState(true)
  const [refreshing, setRefreshing] = useState(false)

  useEffect(() => {
    if (!open || !initialItem) return

    const currentItem = initialItem

    async function loadDetail() {
      setLoadingDetail(true)
      const tmdbId = currentItem.tmdb_id || (currentItem as MediaItem & { id?: number }).id
      if (!tmdbId) {
        setItem(currentItem)
        setLoadingDetail(false)
        return
      }
      try {
        const res = await tmdbGetDetail(currentItem.media_type, tmdbId)
        if (res.data?.detail) {
          setItem(res.data.detail)
        } else {
          setItem(currentItem)
        }
      } catch {
        setItem(currentItem)
      } finally {
        setLoadingDetail(false)
      }
    }
    loadDetail()
  }, [initialItem, open])

  async function handleRefreshMeta() {
    if (!initialItem) return
    const folderId = initialItem.drive_folder_id
    if (!folderId) {
      toast.error('该媒体项缺少 Drive 文件夹 ID，无法刷新')
      return
    }
    setRefreshing(true)
    try {
      const res = await refreshMediaItem(
        initialItem.tmdb_id,
        initialItem.media_type,
        folderId,
        initialItem.title,
        initialItem.year
      )
      const { uploaded = [], errors = [] } = res.data || {}
      if (errors.length === 0) {
        toast.success(`元数据已刷新，共上传 ${uploaded.length} 个文件`)
      } else {
        toast.warning(`部分文件上传失败（${errors.length} 个错误）`)
      }
      // 刷新成功后重新获取详情
      const tmdbId = initialItem.tmdb_id || (initialItem as MediaItem & { id?: number }).id
      if (tmdbId) {
        try {
          const detailRes = await tmdbGetDetail(initialItem.media_type, tmdbId)
          if (detailRes.data?.detail) {
            setItem(detailRes.data.detail)
          }
        } catch {
          // 忽略重新获取详情的错误
        }
      }
    } catch (e) {
      toast.error(
        `刷新失败：${(e as { response?: { data?: { detail?: string } }; message?: string })?.response?.data?.detail || (e as { message?: string })?.message}`
      )
    } finally {
      setRefreshing(false)
    }
  }

  if (!initialItem) return null

  const displayItem = item || initialItem

  // 标题旁小刷新按钮（仅在有 drive_folder_id 时显示）
  const titleAction = initialItem.drive_folder_id ? (
    <Button
      variant="ghost"
      size="icon-xs"
      onClick={handleRefreshMeta}
      disabled={refreshing || loadingDetail}
      title="刷新媒体信息（NFO + 封面）"
      className="ml-2"
    >
      <RefreshCw
        className="size-3"
        style={{ animation: refreshing ? 'spin 0.9s linear infinite' : 'none' }}
      />
    </Button>
  ) : null

  const headerRight = (
    <Button
      variant="default"
      size="icon-lg"
      onClick={() => onSearchResources?.(displayItem)}
      disabled={loadingDetail}
      className="rounded-full shadow-lg bg-brand text-brand-foreground hover:bg-brand/80"
      title="检索资源"
    >
      <Search className="size-4" />
    </Button>
  )

  return (
    <DetailModal
      item={displayItem}
      open={open}
      onOpenChange={onOpenChange}
      loadingSlot={loadingDetail}
      headerRightSlot={headerRight}
      titleActionSlot={titleAction}
    />
  )
}
