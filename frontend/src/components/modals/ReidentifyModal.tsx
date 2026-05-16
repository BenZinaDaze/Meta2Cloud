import { useEffect, useMemo, useState } from 'react'
import { Check, LoaderCircle, PencilLine, RefreshCw, Search } from 'lucide-react'
import { reidentifyMediaItem, tmdbGetDetail, tmdbSearchMulti } from '@/api'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { toast } from 'sonner'
import type { MediaItem, ReidentifyResponse } from '@/types/api'

interface ReidentifyModalProps {
  item: MediaItem | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onUpdated?: (item: MediaItem) => void
}

function ResultRow({
  item,
  active,
  onClick,
}: {
  item: MediaItem
  active: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`flex w-full items-center gap-3 rounded-2xl border px-3 py-3 text-left transition-colors ${
        active ? 'border-brand bg-brand/10' : 'hover:bg-muted/50'
      }`}
    >
      {item.poster_url ? (
        <img src={item.poster_url} alt={item.title} className="h-16 w-11 rounded-md object-cover" />
      ) : (
        <div className="h-16 w-11 rounded-md bg-muted" />
      )}
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-semibold">{item.title}</div>
        {item.original_title && item.original_title !== item.title && (
          <div className="truncate text-xs text-muted-foreground">{item.original_title}</div>
        )}
        <div className="mt-1 flex flex-wrap gap-1.5">
          {item.year && <Badge variant="secondary">{item.year}</Badge>}
          <Badge variant="secondary">{item.media_type === 'tv' ? '剧集' : '电影'}</Badge>
          <Badge variant="secondary">TMDB {item.tmdb_id}</Badge>
        </div>
      </div>
      {active && <Check className="size-4 text-brand" />}
    </button>
  )
}

export default function ReidentifyModal({
  item,
  open,
  onOpenChange,
  onUpdated,
}: ReidentifyModalProps) {
  const [mode, setMode] = useState<'search' | 'id'>('search')
  const [query, setQuery] = useState('')
  const [inputTmdbId, setInputTmdbId] = useState('')
  const [renameFolder, setRenameFolder] = useState(true)
  const [results, setResults] = useState<MediaItem[]>([])
  const [preview, setPreview] = useState<MediaItem | null>(null)
  const [selectedTmdbId, setSelectedTmdbId] = useState<number | null>(null)
  const [searching, setSearching] = useState(false)
  const [loadingPreview, setLoadingPreview] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    if (!open || !item) return
    setMode('search')
    setQuery(item.title || item.original_title || '')
    setInputTmdbId(item.tmdb_id ? String(item.tmdb_id) : '')
    setRenameFolder(true)
    setResults([])
    setPreview(null)
    setSelectedTmdbId(null)
    setSearching(false)
    setLoadingPreview(false)
    setSubmitting(false)
  }, [item, open])

  const currentTmdbId = useMemo(() => {
    if (mode === 'id') {
      const parsed = Number(inputTmdbId)
      return Number.isInteger(parsed) && parsed > 0 ? parsed : null
    }
    return selectedTmdbId
  }, [inputTmdbId, mode, selectedTmdbId])

  async function handleSearch() {
    if (!query.trim() || !item) return
    setSearching(true)
    setResults([])
    setPreview(null)
    setSelectedTmdbId(null)
    try {
      const res = await tmdbSearchMulti(query.trim())
      const filtered = (res.data?.results || []).filter((entry: MediaItem) => entry.media_type === item.media_type)
      setResults(filtered)
      if (filtered.length === 0) {
        toast.warning('没有找到同类型结果', { description: '可以换关键词，或者直接输入 TMDB ID。' })
      }
    } catch (e) {
      toast.error('TMDB 搜索失败', {
        description:
          (e as { response?: { data?: { detail?: string } }; message?: string })?.response?.data?.detail ||
          (e as { message?: string })?.message ||
          '未知错误',
      })
    } finally {
      setSearching(false)
    }
  }

  async function loadPreview(tmdbId: number) {
    if (!item) return
    setLoadingPreview(true)
    try {
      const res = await tmdbGetDetail(item.media_type, tmdbId)
      setPreview(res.data?.detail || null)
    } catch (e) {
      toast.error('获取 TMDB 详情失败', {
        description:
          (e as { response?: { data?: { detail?: string } }; message?: string })?.response?.data?.detail ||
          (e as { message?: string })?.message ||
          '未知错误',
      })
      setPreview(null)
    } finally {
      setLoadingPreview(false)
    }
  }

  async function handleSubmit() {
    if (!item?.drive_folder_id || !currentTmdbId) return
    setSubmitting(true)
    try {
      const res = await reidentifyMediaItem(
        currentTmdbId,
        item.media_type,
        item.drive_folder_id,
        item.title,
        item.year,
        renameFolder
      )
      const payload = (res.data || {}) as ReidentifyResponse
      const updatedItem = payload.item || preview
      if (updatedItem) {
        onUpdated?.(updatedItem)
      }
      if (payload.partial) {
        toast.warning('识别已修正，但有部分步骤失败', {
          description: [...(payload.errors || []), ...(payload.rename_errors || [])].join('；') || '请查看日志后重试。',
        })
      } else {
        toast.success('识别已修正', {
          description: payload.renamed ? `目录已同步为 ${payload.folder_name || '新标题'}` : `已切换到 TMDB ${currentTmdbId}`,
        })
      }
      onOpenChange(false)
    } catch (e) {
      toast.error('修正识别失败', {
        description:
          (e as { response?: { data?: { detail?: string } }; message?: string })?.response?.data?.detail ||
          (e as { message?: string })?.message ||
          '未知错误',
      })
    } finally {
      setSubmitting(false)
    }
  }

  const canSubmit = !!item?.drive_folder_id && !!currentTmdbId && currentTmdbId !== item?.tmdb_id && !loadingPreview

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>修正识别</DialogTitle>
          <DialogDescription>
            为当前媒体重新绑定正确的 TMDB 条目，并重写 NFO、封面与本地库记录。
          </DialogDescription>
        </DialogHeader>

        <div className="flex gap-2">
          <Button variant={mode === 'search' ? 'default' : 'outline'} size="sm" onClick={() => setMode('search')}>
            <Search className="size-4" />
            搜索选择
          </Button>
          <Button variant={mode === 'id' ? 'default' : 'outline'} size="sm" onClick={() => setMode('id')}>
            <PencilLine className="size-4" />
            直接填 ID
          </Button>
        </div>

        <div className="grid gap-4 lg:grid-cols-[1.2fr_0.8fr]">
          <div className="space-y-4">
            {mode === 'search' ? (
              <>
                <div className="flex gap-2">
                  <Input
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    placeholder="输入电影或剧集标题"
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault()
                        handleSearch()
                      }
                    }}
                  />
                  <Button onClick={handleSearch} disabled={searching || !query.trim()}>
                    {searching ? <LoaderCircle className="size-4 animate-spin" /> : <Search className="size-4" />}
                    搜索
                  </Button>
                </div>
                <div className="max-h-[360px] space-y-2 overflow-y-auto rounded-2xl border p-2">
                  {results.length === 0 ? (
                    <div className="px-3 py-8 text-center text-sm text-muted-foreground">
                      {searching ? '搜索中...' : '先搜索，再从结果里选择正确条目。'}
                    </div>
                  ) : (
                    results.map((result) => (
                      <ResultRow
                        key={`${result.media_type}-${result.tmdb_id}`}
                        item={result}
                        active={selectedTmdbId === result.tmdb_id}
                        onClick={() => {
                          setSelectedTmdbId(result.tmdb_id)
                          loadPreview(result.tmdb_id)
                        }}
                      />
                    ))
                  )}
                </div>
              </>
            ) : (
              <div className="space-y-3 rounded-2xl border p-4">
                <div className="text-sm font-medium">TMDB ID</div>
                <div className="flex gap-2">
                  <Input
                    value={inputTmdbId}
                    onChange={(e) => setInputTmdbId(e.target.value.replace(/[^\d]/g, ''))}
                    placeholder="例如 1396"
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault()
                        const parsed = Number(inputTmdbId)
                        if (parsed > 0) loadPreview(parsed)
                      }
                    }}
                  />
                  <Button
                    variant="outline"
                    onClick={() => {
                      const parsed = Number(inputTmdbId)
                      if (parsed > 0) loadPreview(parsed)
                    }}
                    disabled={loadingPreview || !currentTmdbId}
                  >
                    {loadingPreview ? <LoaderCircle className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
                    预览
                  </Button>
                </div>
              </div>
            )}

            <label className="flex items-center gap-2 text-sm text-muted-foreground">
              <input
                type="checkbox"
                checked={renameFolder}
                onChange={(e) => setRenameFolder(e.target.checked)}
              />
              同步重命名媒体目录
            </label>
          </div>

          <div className="space-y-4 rounded-2xl border p-4">
            <div>
              <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">当前绑定</div>
              <div className="space-y-2">
                <div className="text-sm font-semibold">{item?.title}</div>
                <div className="flex flex-wrap gap-1.5">
                  {item?.year && <Badge variant="secondary">{item.year}</Badge>}
                  <Badge variant="secondary">TMDB {item?.tmdb_id || '未绑定'}</Badge>
                </div>
              </div>
            </div>

            <div className="border-t pt-4">
              <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">新绑定预览</div>
              {loadingPreview ? (
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <LoaderCircle className="size-4 animate-spin" />
                  加载中...
                </div>
              ) : preview ? (
                <div className="space-y-3">
                  {preview.poster_url && (
                    <img src={preview.poster_url} alt={preview.title} className="h-40 w-28 rounded-lg object-cover" />
                  )}
                  <div className="text-sm font-semibold">{preview.title}</div>
                  {preview.original_title && preview.original_title !== preview.title && (
                    <div className="text-xs text-muted-foreground">{preview.original_title}</div>
                  )}
                  <div className="flex flex-wrap gap-1.5">
                    {preview.year && <Badge variant="secondary">{preview.year}</Badge>}
                    <Badge variant="secondary">TMDB {preview.tmdb_id}</Badge>
                  </div>
                  {preview.overview && (
                    <p className="line-clamp-6 text-xs leading-5 text-muted-foreground">{preview.overview}</p>
                  )}
                </div>
              ) : (
                <div className="text-sm text-muted-foreground">选择一个结果，或者输入 TMDB ID 后点“预览”。</div>
              )}
            </div>
          </div>
        </div>

        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={submitting}>
            取消
          </Button>
          <Button onClick={handleSubmit} disabled={!canSubmit || submitting}>
            {submitting ? <LoaderCircle className="size-4 animate-spin" /> : <Check className="size-4" />}
            确认修正
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
