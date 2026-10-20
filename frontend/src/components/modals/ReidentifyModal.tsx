import { useEffect, useMemo, useState } from 'react'
import { Check, LoaderCircle, PencilLine, RefreshCw, Search } from 'lucide-react'
import { getMediaClassification, reidentifyMediaItem, tmdbGetDetail, tmdbSearchMulti } from '@/api'
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
import { cn } from '@/lib/utils'
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
      className={cn(
        'flex w-full items-center gap-3 rounded-xl border px-3 py-3 text-left transition-all duration-200',
        active
          ? 'border-primary/45 bg-primary/10 shadow-[0_0_16px_-6px_oklch(76%_0.19_150/0.4)]'
          : 'border-border/70 hover:border-primary/25 hover:bg-muted/50'
      )}
    >
      {item.poster_url ? (
        <img src={item.poster_url} alt={item.title} className="h-16 w-11 rounded-md border border-border/60 object-cover" />
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
      {active && <Check className="size-4 shrink-0 text-primary" />}
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
  const [clsInfo, setClsInfo] = useState<{ current: string; expected: string } | null>(null)
  const [clsLoading, setClsLoading] = useState(false)

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

  // 分类核对：当前实际分类（115 实时）vs 应属分类（TMDB 最新流派 + 规则）
  useEffect(() => {
    setClsInfo(null)
    if (!open || !item?.drive_folder_id) return
    let cancelled = false
    setClsLoading(true)
    getMediaClassification(item.drive_folder_id)
      .then((res) => {
        if (!cancelled && res.data?.available) {
          setClsInfo({ current: res.data.current || '', expected: res.data.expected || '' })
        }
      })
      .catch(() => {})
      .finally(() => {
        if (!cancelled) setClsLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [open, item])

  const categoryMismatch = !!clsInfo && clsInfo.current !== clsInfo.expected

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
        toast.warning('未找到同类型结果', { description: '可尝试其他关键词，或直接输入 TMDB ID。' })
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
        toast.warning('修正完成，但有部分步骤失败', {
          description: [
            ...(payload.errors || []),
            ...(payload.rename_errors || []),
            ...(payload.reclassify_errors || []),
          ].join('；') || '请查看日志后重试。',
        })
      } else {
        const actions: string[] = []
        if (currentTmdbId !== item.tmdb_id) actions.push(`已切换到 TMDB ${currentTmdbId}`)
        if (payload.renamed) actions.push(`目录已重命名为 ${payload.folder_name || '新标题'}`)
        if (payload.reclassified) actions.push(`已归入 ${payload.category || '正确分类'}`)
        toast.success('修正完成', {
          description: actions.length > 0 ? actions.join('；') : 'TMDB 识别与分类均已正确，未做改动',
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

  // TMDB 与当前一致时也允许提交：分类放错位置的修正不走换绑，只按需重命名与归位
  const canSubmit = !!item?.drive_folder_id && !!currentTmdbId && !loadingPreview

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>修正识别</DialogTitle>
          <DialogDescription>
            重新绑定 TMDB 条目并更新本地库记录；目录名与分类归位会按需一并修正（TMDB 可与当前一致）。
          </DialogDescription>
        </DialogHeader>

        <div className="flex gap-2">
          <Button
            variant={mode === 'search' ? 'default' : 'outline'}
            size="sm"
            onClick={() => setMode('search')}
            className="rounded-full"
          >
            <Search className="size-4" />
            搜索选择
          </Button>
          <Button
            variant={mode === 'id' ? 'default' : 'outline'}
            size="sm"
            onClick={() => setMode('id')}
            className="rounded-full"
          >
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
                <div className="max-h-[360px] space-y-2 overflow-y-auto rounded-xl border border-border/70 bg-card/40 p-2">
                  {results.length === 0 ? (
                    <div className="px-3 py-8 text-center text-sm text-muted-foreground">
                      {searching ? '搜索中…' : '先搜索，再从结果里选择正确条目。'}
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
              <div className="space-y-3 rounded-xl border border-border/70 bg-card/40 p-4">
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

            <label className="flex cursor-pointer items-center gap-2.5 text-sm text-muted-foreground">
              <input
                type="checkbox"
                checked={renameFolder}
                onChange={(e) => setRenameFolder(e.target.checked)}
                className="size-4 rounded border-input accent-[oklch(76%_0.19_150)]"
              />
              同步重命名媒体目录
            </label>
            {currentTmdbId === item?.tmdb_id && (
              <p className="text-xs text-muted-foreground">
                TMDB 与当前绑定一致，确认后将仅核对目录名与分类归位（放错位置时自动移动）。
              </p>
            )}
          </div>

          <div className="space-y-4 rounded-xl border border-border/70 bg-card/40 p-4">
            <div>
              <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">当前绑定</div>
              <div className="space-y-2">
                <div className="text-sm font-semibold">{item?.title}</div>
                <div className="flex flex-wrap gap-1.5">
                  {item?.year && <Badge variant="secondary">{item.year}</Badge>}
                  <Badge variant="secondary">TMDB {item?.tmdb_id || '未绑定'}</Badge>
                </div>
              </div>
              <div className="mt-3 space-y-1 border-t border-border/60 pt-3 text-xs">
                {clsLoading ? (
                  <div className="flex items-center gap-1.5 text-muted-foreground">
                    <LoaderCircle className="size-3 animate-spin" />
                    分类核对中…
                  </div>
                ) : clsInfo ? (
                  <>
                    <div className="text-muted-foreground">
                      当前分类：
                      <span className={categoryMismatch ? 'font-medium text-warning' : 'text-foreground'}>
                        {clsInfo.current || '未分类'}
                      </span>
                    </div>
                    <div className="text-muted-foreground">
                      应属分类：
                      <span className="text-foreground">{clsInfo.expected || '未命中规则'}</span>
                    </div>
                    {categoryMismatch && (
                      <div className="text-warning">分类不一致，确认修正后将自动移动到应属分类</div>
                    )}
                  </>
                ) : null}
              </div>
            </div>

            <div className="border-t border-border/60 pt-4">
              <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">新绑定预览</div>
              {loadingPreview ? (
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <LoaderCircle className="size-4 animate-spin" />
                  加载中…
                </div>
              ) : preview ? (
                <div className="space-y-3">
                  {preview.poster_url && (
                    <img src={preview.poster_url} alt={preview.title} className="h-40 w-28 rounded-lg border border-border/70 object-cover shadow-lg" />
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
