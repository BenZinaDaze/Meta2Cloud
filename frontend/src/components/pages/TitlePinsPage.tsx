import { useState, useEffect, useCallback } from 'react'
import { Search, RefreshCw, Film, Tv, ArrowRightLeft, Loader2, Tag, Trash2, ExternalLink } from 'lucide-react'
import { listTitlePins, adoptTitlePin, deleteTitlePin } from '@/api'
import { StatePanel } from '@/components/StatePanel'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
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
import PageHeader from '@/components/shared/PageHeader'
import { relativeTime } from '@/lib/time'
import type { TitlePin } from '@/types/api'

interface TitlePinsPageProps {
  onToast: (type: 'success' | 'error' | 'warning', title: string, message?: string) => void
}

const sourceLabels: Record<string, string> = {
  ingest_history: '首次入库',
  tmdb: '首次整理',
  manual: '手动指定',
}

function PinCard({
  pin,
  adopting,
  onAdopt,
  onDelete,
}: {
  pin: TitlePin
  adopting: boolean
  onAdopt: () => void
  onDelete: () => void
}) {
  const isTV = pin.media_type === 'tv'
  return (
    <div
      className={`overflow-hidden rounded-xl border p-3 transition-colors duration-200 hover:bg-card/80 sm:p-4 ${
        pin.drifted
          ? 'border-warning/40 bg-warning/[0.04] hover:border-warning/60'
          : 'border-border/60 bg-card/50 hover:border-primary/20'
      }`}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            {isTV ? <Tv className="size-4 shrink-0 text-muted-foreground" /> : <Film className="size-4 shrink-0 text-muted-foreground" />}
            <h3 className="truncate font-semibold">
              {pin.pinned_title}
              {pin.pinned_year && <span className="ml-1.5 font-normal text-muted-foreground">({pin.pinned_year})</span>}
            </h3>
            <Badge variant="outline" className="shrink-0 border-border/60 text-[10px] text-muted-foreground">
              {isTV ? '电视剧' : '电影'}
            </Badge>
          </div>

          <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            <a
              href={`https://www.themoviedb.org/${isTV ? 'tv' : 'movie'}/${pin.tmdb_id}?language=zh-CN`}
              target="_blank"
              rel="noreferrer"
              className="flex items-center gap-1 rounded-md bg-muted px-1.5 py-0.5 font-mono transition-colors hover:bg-accent hover:text-foreground"
              title="在 TMDB 打开"
            >
              tmdb {pin.tmdb_id}
              <ExternalLink className="size-3 opacity-60" />
            </a>
            <span className="text-border">·</span>
            <span>{sourceLabels[pin.source] || pin.source}</span>
            <span className="text-border">·</span>
            <span>{relativeTime(pin.created_at)}建立</span>
          </div>

          {pin.drifted && (
            <div className="mt-2.5 flex flex-wrap items-center gap-x-2 gap-y-1 rounded-lg border border-warning/30 bg-warning/[0.06] px-2.5 py-1.5 text-xs">
              <ArrowRightLeft className="size-3 shrink-0 text-warning" />
              <span className="text-muted-foreground">TMDB 当前译名：</span>
              <span className="font-medium text-foreground">
                {pin.current_title}
                {pin.current_year && pin.current_year !== pin.pinned_year && (
                  <span className="ml-1 font-normal text-muted-foreground">({pin.current_year})</span>
                )}
              </span>
            </div>
          )}
        </div>

        <div className="flex shrink-0 items-center gap-2">
          {pin.drifted && (
            <Button
              size="sm"
              variant="outline"
              className="gap-1.5 border-warning/40 text-warning hover:bg-warning/10 hover:text-warning"
              disabled={adopting}
              onClick={onAdopt}
            >
              {adopting ? <Loader2 className="size-3.5 animate-spin" /> : <ArrowRightLeft className="size-3.5" />}
              采用最新译名
            </Button>
          )}
          <Button
            size="sm"
            variant="outline"
            className="gap-1.5 border-destructive/30 text-destructive hover:bg-destructive/10 hover:text-destructive"
            onClick={onDelete}
          >
            <Trash2 className="size-3.5" />
            删除
          </Button>
        </div>
      </div>
    </div>
  )
}

export default function TitlePinsPage({ onToast }: TitlePinsPageProps) {
  const [pins, setPins] = useState<TitlePin[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [keyword, setKeyword] = useState('')
  const [adoptingKey, setAdoptingKey] = useState('')
  const [deleteTarget, setDeleteTarget] = useState<TitlePin | null>(null)
  const [deleting, setDeleting] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const res = await listTitlePins()
      setPins(res.data?.items || [])
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const handleAdopt = useCallback(
    async (pin: TitlePin) => {
      setAdoptingKey(`${pin.media_type}:${pin.tmdb_id}`)
      try {
        const res = await adoptTitlePin(pin.media_type, pin.tmdb_id)
        const renamed = res.data?.renamed_folders ?? 0
        onToast(
          'success',
          `已采用最新译名：${res.data?.title || pin.current_title}`,
          renamed > 0 ? `已重命名 ${renamed} 个网盘目录，正在触发整理与 STRM 同步` : '已更新整理名称记录，正在触发整理与 STRM 同步',
        )
        await load()
      } catch (e) {
        const msg = (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail
        onToast('error', '采用新译名失败', msg || (e instanceof Error ? e.message : String(e)))
      } finally {
        setAdoptingKey('')
      }
    },
    [onToast, load],
  )

  const handleDelete = useCallback(async () => {
    if (!deleteTarget) return
    const pin = deleteTarget
    setDeleting(true)
    try {
      await deleteTitlePin(pin.media_type, pin.tmdb_id)
      onToast('success', `已删除整理名称：${pin.pinned_title}`, '网盘目录不受影响；下次整理将重新确定名称')
      await load()
    } catch (e) {
      const msg = (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail
      onToast('error', '删除失败', msg || (e instanceof Error ? e.message : String(e)))
    } finally {
      setDeleting(false)
      setDeleteTarget(null)
    }
  }, [deleteTarget, onToast, load])

  const kw = keyword.trim().toLowerCase()
  const filtered = kw
    ? pins.filter(
        (p) =>
          p.pinned_title.toLowerCase().includes(kw) ||
          p.current_title.toLowerCase().includes(kw) ||
          String(p.tmdb_id).includes(kw),
      )
    : pins

  const driftedCount = pins.filter((p) => p.drifted).length

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        kicker="Naming Pins"
        title="整理名称"
        description="每个媒体整理时使用的固定目录名。TMDB 译名变更后仍沿用首次整理名，避免产生重复目录；如需改用最新译名可一键切换，识别有误可删除记录重新整理。"
        actions={
          <div className="flex items-center gap-2">
            <div className="relative w-full min-w-0 sm:w-56">
              <Search className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={keyword}
                onChange={(e) => setKeyword(e.target.value)}
                placeholder="搜索标题 / tmdb_id"
                className="h-10 rounded-full pl-10"
              />
            </div>
            <Button
              variant="outline"
              size="icon"
              onClick={load}
              disabled={loading}
              title="刷新"
              className="size-10 shrink-0 rounded-full"
            >
              <RefreshCw className={`size-4 ${loading ? 'animate-spin' : ''}`} />
            </Button>
          </div>
        }
      />

      {loading ? (
        <StatePanel title="正在加载" description="整理名称列表加载中..." />
      ) : error ? (
        <StatePanel icon="!" title={`加载失败：${error}`} description="请检查后端服务状态" tone="danger" />
      ) : filtered.length === 0 ? (
        <StatePanel
          icon={<Tag className="size-5" />}
          title={kw ? '没有匹配的记录' : '暂无整理名称记录'}
          description={kw ? '换个关键词试试' : '完成一次媒体整理后会自动记录每个媒体的整理名称'}
        />
      ) : (
        <div className="flex flex-col gap-3">
          {driftedCount > 0 && (
            <div className="flex items-center gap-2 rounded-xl border border-warning/30 bg-warning/[0.06] px-3 py-2 text-xs text-muted-foreground">
              <ArrowRightLeft className="size-3.5 shrink-0 text-warning" />
              <span>
                {driftedCount} 个媒体的 TMDB 译名已变化（高亮显示），整理仍沿用首次目录名
              </span>
            </div>
          )}
          {filtered.map((pin) => (
            <PinCard
              key={`${pin.media_type}:${pin.tmdb_id}`}
              pin={pin}
              adopting={adoptingKey === `${pin.media_type}:${pin.tmdb_id}`}
              onAdopt={() => handleAdopt(pin)}
              onDelete={() => setDeleteTarget(pin)}
            />
          ))}
        </div>
      )}

      <AlertDialog open={!!deleteTarget} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除这条整理名称？</AlertDialogTitle>
            <AlertDialogDescription>
              将删除「{deleteTarget?.pinned_title}」的固定名称记录，网盘目录与媒体文件不受影响。
              下次整理会重新确定名称（优先取最早入库记录，其次 TMDB 匹配结果）；
              若此前是识别错了媒体，重新整理前建议确认文件命名可被正确识别。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              disabled={deleting}
              onClick={handleDelete}
            >
              {deleting ? <Loader2 className="mr-1 size-4 animate-spin" /> : null}
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
