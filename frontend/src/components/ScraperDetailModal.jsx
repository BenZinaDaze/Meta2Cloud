import { useEffect, useState } from 'react'
import { tmdbGetDetail, refreshMediaItem } from '../api'
import DetailModal from './DetailModal'

export default function ScraperDetailModal({ item: initialItem, onClose, onToast, onSearchResources }) {
  const [item, setItem] = useState(null)
  const [loadingDetail, setLoadingDetail] = useState(true)
  const [refreshing, setRefreshing] = useState(false)

  useEffect(() => {
    async function loadDetail() {
      setLoadingDetail(true)
      const tmdbId = initialItem.tmdb_id || initialItem.id
      if (!tmdbId) {
        setItem(initialItem)
        setLoadingDetail(false)
        return
      }
      try {
        const res = await tmdbGetDetail(initialItem.media_type, tmdbId)
        if (res.data?.detail) {
          setItem(res.data.detail)
        } else {
          setItem(initialItem)
        }
      } catch {
        setItem(initialItem)
      } finally {
        setLoadingDetail(false)
      }
    }
    loadDetail()
  }, [initialItem])

  async function handleRefreshMeta() {
    const folderId = initialItem.drive_folder_id
    if (!folderId) {
      onToast?.('该媒体项缺少 Drive 文件夹 ID，无法刷新', 'error')
      return
    }
    setRefreshing(true)
    try {
      const res = await refreshMediaItem(
        initialItem.tmdb_id,
        initialItem.media_type,
        folderId,
        initialItem.title,
        initialItem.year,
      )
      const { uploaded = [], errors = [] } = res.data || {}
      if (errors.length === 0) {
        onToast?.(`元数据已刷新，共上传 ${uploaded.length} 个文件`, 'success')
      } else {
        onToast?.(`部分文件上传失败（${errors.length} 个错误）`, 'warning')
      }
    } catch (e) {
      onToast?.(`刷新失败：${e?.response?.data?.detail || e.message}`, 'error')
    } finally {
      setRefreshing(false)
    }
  }

  if (!initialItem) return null

  const displayItem = item || initialItem

  // 标题旁小刷新按钮（仅在有 drive_folder_id 时显示）
  const titleAction = initialItem.drive_folder_id ? (
    <button
      onClick={handleRefreshMeta}
      disabled={refreshing || loadingDetail}
      title="刷新媒体信息（NFO + 封面）"
      className="inline-flex items-center justify-center transition-all duration-150 hover:opacity-80 active:scale-95 disabled:opacity-40 disabled:cursor-not-allowed"
      style={{
        width: 22,
        height: 22,
        color: 'var(--color-muted)',
        padding: 0,
        background: 'none',
        border: 'none',
        cursor: 'pointer',
      }}
    >
      <svg
        width="13" height="13" viewBox="0 0 24 24" fill="none"
        stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"
        style={{ animation: refreshing ? 'spin 0.9s linear infinite' : 'none' }}
      >
        <polyline points="23 4 23 10 17 10" />
        <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10" />
      </svg>
    </button>
  ) : null

  const headerRight = (
    <button
      onClick={() => onSearchResources(displayItem)}
      disabled={loadingDetail}
      className="size-10 flex shrink-0 items-center justify-center rounded-full transition-all hover:scale-105 shadow-[0_4px_16px_rgba(200,146,77,0.3)] active:scale-95 disabled:opacity-50 disabled:cursor-not-allowed"
      style={{ background: 'linear-gradient(135deg, #e3b778, #c8924d)', color: '#0A1320' }}
      title="检索资源"
    >
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round">
        <circle cx="11" cy="11" r="8"></circle>
        <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
      </svg>
    </button>
  )

  return (
    <DetailModal
      item={displayItem}
      onClose={onClose}
      loadingSlot={loadingDetail}
      headerRightSlot={headerRight}
      titleActionSlot={titleAction}
    />
  )
}
