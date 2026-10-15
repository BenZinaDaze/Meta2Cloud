import { useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
import { Badge } from '@/components/ui/badge'
import { RefreshCw, ExternalLink, Star } from 'lucide-react'
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
    ? 'text-success border-success/40 bg-success/10 hover:bg-success/20'
    : !isAired && ep.air_date
    ? 'text-muted-foreground/60 border-border bg-muted/40'
    : 'text-destructive border-destructive/35 bg-destructive/[0.07] hover:bg-destructive/15'

  return (
    <div
      title={`E${String(ep.episode_number).padStart(2, '0')} ${ep.episode_title}${ep.air_date ? ' · ' + ep.air_date : ''}`}
      className={`flex size-8 w-8 cursor-default items-center justify-center rounded-md border text-xs font-bold transition-transform duration-200 hover:scale-110 hover:-translate-y-0.5 ${color}`}
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
  const barColor = pct >= 100
    ? 'bg-success'
    : 'bg-lime'

  return (
    <div className="mb-6">
      <div className="flex items-center gap-3 mb-3">
        {season.poster_url && (
          <img
            src={season.poster_url}
            alt={season.season_name}
            className="w-10 flex-shrink-0 rounded-md border border-border/60 object-cover h-[60px]"
          />
        )}
        <div className="flex-1 min-w-0">
          <div className="flex items-center justify-between mb-1.5">
            <span className="text-sm font-semibold">{season.season_name}</span>
            <span className="text-xs tabular-nums text-muted-foreground">
              {inLibraryCount} / {episodeCount} 集 · {pct}%
            </span>
          </div>
          <div className="h-1.5 overflow-hidden rounded-full bg-muted/80">
            <div
              className={`h-full rounded-full ${barColor} transition-all duration-700 ease-[cubic-bezier(0.22,1,0.36,1)] ${pct > 50 ? 'glow-lime' : ''}`}
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
            <div className="absolute inset-0 bg-gradient-to-br from-accent to-card" />
          )}
          <div className="absolute inset-0 bg-gradient-to-b from-transparent via-background/40 to-popover" />
          <div className="absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-tide/40 to-transparent" />
        </div>

        {/* Poster and title */}
        <div
          className="relative z-10 -mt-8 flex flex-col gap-3 px-4 sm:-mt-12 sm:flex-row sm:items-end sm:gap-4 sm:px-6"
          style={{ opacity: loadingSlot ? 0.5 : 1 }}
        >
          {item.poster_url && (
            <img
              src={item.poster_url}
              alt={item.title}
              className="h-[112px] w-[76px] flex-shrink-0 rounded-lg border border-border/80 object-cover shadow-[0_12px_32px_-8px_oklch(5%_0.01_140/0.9)] sm:h-[132px] sm:w-[88px]"
            />
          )}

          <div className="min-w-0 flex-1 pb-2 sm:pb-3">
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
                <Badge variant="warning">
                  <Star className="size-3 fill-current" /> {item.rating}
                </Badge>
              )}
              {item.status && (
                <Badge>{formatStatus(item.status)}</Badge>
              )}
              {isTV && item.in_library_episodes !== undefined && item.in_library && (
                <Badge variant="success">
                  {item.in_library_episodes}/{item.total_episodes || '?'} 集已入库
                </Badge>
              )}
              {!isTV && item.in_library && (
                <Badge variant="success">已入库</Badge>
              )}
              {item.tmdb_id && (
                <div className="flex items-center gap-1.5">
                  <a
                    href={`https://www.themoviedb.org/${isTV ? 'tv' : 'movie'}/${item.tmdb_id}`}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-1 rounded-full border border-info/30 bg-info/10 px-2 py-0.5 text-xs text-info transition-colors hover:bg-info/20"
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
          className={`relative flex-1 overflow-y-auto px-4 pb-4 sm:px-6 sm:pb-6 ${
            hasFooter ? '' : 'min-h-0'
          }`}
          style={{ paddingBottom: 'calc(env(safe-area-inset-bottom) + 1rem)' }}
        >
          {loadingSlot && (
            <div className="absolute inset-0 z-20 flex items-center justify-center bg-background/50 backdrop-blur-sm">
              <span className="flex items-center gap-2 animate-pulse text-sm text-muted-foreground">
                <RefreshCw className="size-4 animate-spin" />
                加载详情中...
              </span>
            </div>
          )}

          {summaryPairs.length > 0 && (
            <div className="mb-3 mt-4 grid grid-cols-2 gap-2 sm:hidden">
              {summaryPairs.map(([label, value]) => (
                <div
                  key={label}
                  className="rounded-xl border border-border/60 bg-muted/40 px-3 py-2.5"
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
              <div className="mb-4 mt-4 rounded-xl border border-border/60 bg-muted/40 px-4 py-3.5 sm:hidden">
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
                    className="mt-2 text-xs font-semibold text-primary"
                  >
                    {showOverview ? '收起简介' : '展开简介'}
                  </button>
                )}
              </div>
              <p className="mb-5 mt-4 hidden border-l-2 border-primary/30 pl-4 text-sm leading-7 text-muted-foreground sm:block">
                {item.overview}
              </p>
            </>
          )}

          {isTV && item.seasons && item.seasons.length > 0 ? (
            <div>
              <h3 className="mb-3 border-b border-border/60 pb-2 text-sm font-semibold">
                季集入库状态
              </h3>
              <div className="mb-3 flex flex-wrap gap-3 text-xs text-muted-foreground">
                {[
                  { className: 'border-success/40 bg-success/10', label: '已入库' },
                  { className: 'border-destructive/35 bg-background', label: '未入库' },
                  { className: 'border-border bg-muted/40', label: '未播出' },
                ].map(({ className, label }) => (
                  <span key={label} className="flex items-center gap-1">
                    <span className={`inline-block size-3 rounded-sm border ${className}`} />{' '}
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
          <div className="flex-shrink-0 border-t border-border/60 bg-card/60 px-4 pb-4 pt-3 sm:px-6 sm:pb-6">
            {footerSlot}
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
