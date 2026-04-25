import { useCallback, useEffect, useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
import { Badge } from '@/components/ui/badge'
import { RefreshCw, ExternalLink } from 'lucide-react'
import type { MediaItem, SeasonStatus } from '@/types/api'

const STATUS_MAP: Record<string, string> = {
  'Returning Series': '连载中',
  'Ended': '已完结',
  'Canceled': '已取消',
  'In Production': '制作中',
  'Planned': '计划中',
  'Pilot': '试播',
  'In Limbo': '播出未定',
}

function formatStatus(status: string | undefined): string | undefined {
  if (!status) return status
  return STATUS_MAP[status] ?? status
}

interface EpisodePillProps {
  ep: {
    episode_number: number
    episode_title: string
    air_date: string
    in_library: boolean
  }
}

function EpisodePill({ ep }: EpisodePillProps) {
  const isAired = ep.air_date && new Date(ep.air_date) <= new Date()
  const color = ep.in_library
    ? 'text-success border-success bg-success/10'
    : !isAired && ep.air_date
    ? 'text-muted-foreground border-muted-foreground'
    : 'text-destructive border-destructive'

  return (
    <div
      title={`E${String(ep.episode_number).padStart(2, '0')} ${ep.episode_title}${ep.air_date ? ' · ' + ep.air_date : ''}`}
      className={`w-8 h-8 rounded flex items-center justify-center text-xs font-bold cursor-default transition-transform hover:scale-110 border ${color}`}
    >
      {ep.episode_number}
    </div>
  )
}

interface SeasonBlockProps {
  season: SeasonStatus
}

function SeasonBlock({ season }: SeasonBlockProps) {
  const episodeCount = season.episode_count || season.episodes?.length || 0
  const inLibraryCount = season.in_library_count ?? 0
  const pct = episodeCount > 0 ? Math.round((inLibraryCount / episodeCount) * 100) : 0
  const barColor = pct >= 100 ? 'bg-success' : pct > 50 ? 'bg-brand' : 'bg-warning'

  return (
    <div className="mb-6">
      <div className="flex items-center gap-3 mb-3">
        {season.poster_url && (
          <img
            src={season.poster_url}
            alt={season.season_name}
            className="rounded object-cover flex-shrink-0 w-10 h-[60px]"
          />
        )}
        <div className="flex-1 min-w-0">
          <div className="flex items-center justify-between mb-1">
            <span className="text-sm font-semibold">{season.season_name}</span>
            <span className="text-xs text-muted-foreground">
              {inLibraryCount} / {episodeCount} 集 · {pct}%
            </span>
          </div>
          <div className="h-1.5 rounded-full overflow-hidden bg-muted">
            <div
              className={`h-full rounded-full transition-all duration-500 ${barColor}`}
              style={{ width: `${pct}%` }}
            />
          </div>
        </div>
      </div>
      <div className="flex flex-wrap gap-1.5">
        {(season.episodes || []).map((ep) => (
          <EpisodePill key={ep.episode_number} ep={ep} />
        ))}
      </div>
    </div>
  )
}

interface DetailModalProps {
  item: MediaItem | null
  open: boolean
  onOpenChange: (open: boolean) => void
  contentSlot?: React.ReactNode
  footerSlot?: React.ReactNode
  loadingSlot?: boolean
  headerRightSlot?: React.ReactNode
  titleActionSlot?: React.ReactNode
}

export default function DetailModal({
  item,
  open,
  onOpenChange,
  contentSlot,
  footerSlot,
  loadingSlot,
  headerRightSlot,
  titleActionSlot,
}: DetailModalProps) {
  const [showOverview, setShowOverview] = useState(false)
  const hasFooter = !!footerSlot

  const handleClose = useCallback(() => {
    onOpenChange(false)
  }, [onOpenChange])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && open) {
        handleClose()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, handleClose])

  if (!item) return null

  const isTV = item.media_type === 'tv'
  const summaryPairs = [
    item.year ? ['年份', item.year] : null,
    (item as MediaItem & { genre_names?: string[] }).genre_names?.length
      ? ['类型', (item as MediaItem & { genre_names?: string[] }).genre_names!.slice(0, 2).join(' / ')]
      : null,
    (item as MediaItem & { original_language?: string }).original_language
      ? ['语言', String((item as MediaItem & { original_language?: string }).original_language).toUpperCase()]
      : null,
    isTV && item.total_episodes ? ['总集数', `${item.total_episodes} 集`] : null,
    !isTV && (item as MediaItem & { runtime?: number }).runtime
      ? ['时长', `${(item as MediaItem & { runtime?: number }).runtime} 分钟`] : null,
  ].filter(Boolean) as [string, string][]

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl p-0 gap-0 overflow-hidden max-h-[calc(100dvh-2rem)] flex flex-col">
        <DialogTitle className="sr-only">{item?.title || '媒体详情'}</DialogTitle>
        <DialogDescription className="sr-only">
          {item?.overview || '查看媒体详情和入库状态'}
        </DialogDescription>
        {/* Header with backdrop */}
        <div className="relative h-28 sm:h-40 flex-shrink-0">
          {item.backdrop_url ? (
            <img
              src={item.backdrop_url}
              alt=""
              className="absolute inset-0 h-full w-full object-cover transition-opacity duration-300"
              style={{ opacity: loadingSlot ? 0.3 : 1 }}
            />
          ) : (
            <div className="absolute inset-0 bg-muted" />
          )}
          <div className="absolute inset-0 bg-gradient-to-b from-transparent via-background/50 to-background" />
        </div>

        {/* Poster and title */}
        <div
          className="flex flex-col gap-3 px-4 -mt-8 sm:-mt-12 sm:flex-row sm:items-end sm:gap-4 sm:px-6 relative z-10"
          style={{ opacity: loadingSlot ? 0.5 : 1 }}
        >
          {item.poster_url && (
            <img
              src={item.poster_url}
              alt={item.title}
              className="h-[112px] w-[76px] flex-shrink-0 rounded-lg shadow-xl border-2 border-border object-cover sm:h-[132px] sm:w-[88px]"
            />
          )}

          <div className="min-w-0 flex-1 pb-2 sm:pb-3">
            <div className="mb-2 hidden text-[11px] font-semibold uppercase tracking-widest text-brand sm:block">
              Metadata detail
            </div>
            <h2 className="text-lg font-bold leading-snug sm:text-2xl">
              {item.title}
            </h2>
            {item.original_title && item.original_title !== item.title && (
              <p className="mb-2 text-sm text-muted-foreground">
                {item.original_title}
              </p>
            )}
            <div className="mt-2 flex flex-wrap gap-1.5">
              {item.year && (
                <Badge variant="secondary">{item.year}</Badge>
              )}
              {item.rating > 0 && (
                <Badge variant="secondary" className="text-warning border-warning">
                  ★ {item.rating}
                </Badge>
              )}
              {item.status && (
                <Badge variant="secondary" className="text-brand">
                  {formatStatus(item.status)}
                </Badge>
              )}
              {isTV && item.in_library_episodes !== undefined && item.in_library && (
                <Badge className="bg-success/10 text-success border-success">
                  {item.in_library_episodes}/{item.total_episodes || '?'} 集已入库
                </Badge>
              )}
              {!isTV && item.in_library && (
                <Badge variant="secondary" className="text-success border-success">
                  已入库
                </Badge>
              )}
              {item.tmdb_id && (
                <div className="flex items-center gap-1.5">
                  <a
                    href={`https://www.themoviedb.org/${isTV ? 'tv' : 'movie'}/${item.tmdb_id}`}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs bg-blue-500/10 text-blue-500 border border-blue-500/30 hover:bg-blue-500/20 transition-colors"
                  >
                    TMDB {item.tmdb_id}
                    <ExternalLink className="size-3" />
                  </a>
                  {titleActionSlot}
                </div>
              )}
            </div>
            {headerRightSlot && (
              <div className="mt-2 flex items-center gap-2 sm:hidden">
                {headerRightSlot}
              </div>
            )}
          </div>

          {headerRightSlot && (
            <div className="hidden flex-shrink-0 self-start pb-3 sm:block sm:self-center">
              {headerRightSlot}
            </div>
          )}
        </div>

        {/* Content */}
        <div
          className={`relative px-4 pb-4 sm:px-6 sm:pb-6 overflow-y-auto flex-1 ${
            hasFooter ? '' : 'min-h-0'
          }`}
          style={{ paddingBottom: 'calc(env(safe-area-inset-bottom) + 1rem)' }}
        >
          {loadingSlot && (
            <div className="absolute inset-0 z-20 flex items-center justify-center bg-background/50 backdrop-blur-sm">
              <span className="text-muted-foreground text-sm animate-pulse flex items-center gap-2">
                <RefreshCw className="size-4 animate-spin" />
                加载详情中...
              </span>
            </div>
          )}

          {summaryPairs.length > 0 && (
            <div className="mb-3 grid grid-cols-2 gap-2 sm:hidden">
              {summaryPairs.map(([label, value]) => (
                <div
                  key={label}
                  className="rounded-xl px-3 py-2.5 bg-muted/50 border"
                >
                  <div className="text-[11px] font-semibold uppercase tracking-widest text-muted-foreground">
                    {label}
                  </div>
                  <div className="mt-1 text-sm font-medium">{value}</div>
                </div>
              ))}
            </div>
          )}

          {item.overview && (
            <>
              <div className="mb-4 rounded-xl px-4 py-3.5 bg-muted/50 border sm:hidden">
                <div className="text-[11px] font-semibold uppercase tracking-widest text-muted-foreground">
                  内容简介
                </div>
                <p
                  className="mt-2 text-sm leading-6 text-muted-foreground"
                  style={{
                    display: '-webkit-box',
                    WebkitLineClamp: showOverview ? 'unset' : 4,
                    WebkitBoxOrient: 'vertical',
                    overflow: 'hidden',
                  }}
                >
                  {item.overview}
                </p>
                {item.overview.length > 120 && (
                  <button
                    type="button"
                    onClick={() => setShowOverview((v) => !v)}
                    className="mt-2 text-xs font-semibold text-brand"
                  >
                    {showOverview ? '收起简介' : '展开简介'}
                  </button>
                )}
              </div>
              <p className="mb-5 hidden text-sm leading-7 text-muted-foreground sm:block">
                {item.overview}
              </p>
            </>
          )}

          {isTV && item.seasons && item.seasons.length > 0 ? (
            <div>
              <h3 className="text-sm font-semibold mb-3 pb-2 border-b">
                季集入库状态
              </h3>
              <div className="mb-3 flex flex-wrap gap-3 text-xs text-muted-foreground">
                {[
                  { className: 'border-success bg-success/10', label: '已入库' },
                  { className: 'border-destructive bg-background', label: '未入库' },
                  { className: 'border-muted-foreground bg-background', label: '未播出' },
                ].map(({ className, label }) => (
                  <span key={label} className="flex items-center gap-1">
                    <span className={`w-3 h-3 rounded-sm inline-block border ${className}`} />{' '}
                    {label}
                  </span>
                ))}
              </div>
              {(item.seasons || []).map((season) => (
                <SeasonBlock key={season.season_number} season={season} />
              ))}
            </div>
          ) : isTV ? (
            <div className="text-sm text-muted-foreground">暂无季集信息</div>
          ) : null}

          {contentSlot && <div className="mt-5">{contentSlot}</div>}
        </div>

        {/* Footer */}
        {hasFooter && (
          <div className="border-t px-4 pb-4 pt-3 sm:px-6 sm:pb-6 bg-muted/30 flex-shrink-0">
            {footerSlot}
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
