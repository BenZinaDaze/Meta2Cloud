import { useEffect, useState } from 'react'
import { tmdbGetDetail, refreshMediaItem, removeLibraryItem } from '@/api'
import { toast } from 'sonner'
import DetailModal from './DetailModal'
import ReidentifyModal from './ReidentifyModal'
import { PencilLine, RefreshCw, Search, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
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
import type { MediaItem } from '@/types/api'

interface ScraperDetailModalProps {
  item: MediaItem | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onSearchResources?: (item: MediaItem) => void
  onRemoved?: (item: MediaItem) => void
  onUpdated?: (item: MediaItem) => void
}

export default function ScraperDetailModal({
  item: initialItem,
  open,
  onOpenChange,
  onSearchResources,
  onRemoved,
  onUpdated,
}: ScraperDetailModalProps) {
  const [item, setItem] = useState<MediaItem | null>(null)
  const [loadingDetail, setLoadingDetail] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [confirmRemove, setConfirmRemove] = useState(false)
  const [removing, setRemoving] = useState(false)
  const [reidentifyOpen, setReidentifyOpen] = useState(false)

  useEffect(() => {
    if (!open || !initialItem) return

    setItem(initialItem)
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
    if (!item) return
    const folderId = item.drive_folder_id
    if (!folderId) {
      toast.error('该媒体项缺少 Drive 文件夹 ID，无法刷新')
      return
    }
    setRefreshing(true)
    try {
      const res = await refreshMediaItem(
        item.tmdb_id,
        item.media_type,
        folderId,
        item.title,
        item.year
      )
      const { uploaded = [], errors = [] } = res.data || {}
      if (errors.length === 0) {
        toast.success(`元数据已刷新，共上传 ${uploaded.length} 个文件`)
      } else {
        toast.warning(`部分文件上传失败（${errors.length} 个错误）`)
      }
      if (res.data?.item) {
        setItem(res.data.item)
        onUpdated?.(res.data.item)
      }
      const tmdbId = (res.data?.item?.tmdb_id as number | undefined) || item.tmdb_id || (item as MediaItem & { id?: number }).id
      if (tmdbId) {
        try {
          const detailRes = await tmdbGetDetail(item.media_type, tmdbId)
          if (detailRes.data?.detail) {
            setItem(detailRes.data.detail)
            onUpdated?.(detailRes.data.detail)
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

  async function handleRemoveFromLibrary() {
    if (!item?.drive_folder_id) {
      toast.error('该媒体项缺少 Drive 文件夹 ID，无法移出媒体库')
      return
    }
    setRemoving(true)
    try {
      await removeLibraryItem(item.drive_folder_id)
      toast.success('已移出媒体库', { description: item.title })
      setConfirmRemove(false)
      onOpenChange(false)
      onRemoved?.(item)
    } catch (e) {
      toast.error('移出失败', {
        description:
          (e as { response?: { data?: { detail?: string } }; message?: string })?.response?.data?.detail ||
          (e as { message?: string })?.message ||
          '未知错误',
      })
    } finally {
      setRemoving(false)
    }
  }

  if (!initialItem) return null

  const displayItem = item || initialItem

  const titleAction = displayItem.drive_folder_id ? (
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
    <div className="flex items-center gap-2">
      {displayItem.drive_folder_id && (
        <Button
          variant="outline"
          size="icon-lg"
          onClick={() => setReidentifyOpen(true)}
          disabled={loadingDetail || refreshing}
          className="rounded-full"
          title="修正识别"
        >
          <PencilLine className="size-4" />
        </Button>
      )}
      {displayItem.drive_folder_id && (
        <Button
          variant="outline"
          size="icon-lg"
          onClick={() => setConfirmRemove(true)}
          disabled={loadingDetail || removing}
          className="rounded-full border-destructive/40 text-destructive hover:bg-destructive/10 hover:text-destructive"
          title="移出媒体库"
        >
          <Trash2 className="size-4" />
        </Button>
      )}
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
    </div>
  )

  return (
    <>
      <DetailModal
        item={displayItem}
        open={open}
        onOpenChange={onOpenChange}
        loadingSlot={loadingDetail}
        headerRightSlot={headerRight}
        titleActionSlot={titleAction}
      />
      <ReidentifyModal
        item={displayItem}
        open={reidentifyOpen}
        onOpenChange={setReidentifyOpen}
        onUpdated={(updatedItem) => {
          setItem(updatedItem)
          onUpdated?.(updatedItem)
        }}
      />
      <AlertDialog open={confirmRemove} onOpenChange={setConfirmRemove}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认移出媒体库</AlertDialogTitle>
            <AlertDialogDescription>
              这只会移除本地媒体库记录，不会删除网盘中的实际文件。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={removing}>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleRemoveFromLibrary}
              disabled={removing}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {removing ? '移出中...' : '确认移出'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
