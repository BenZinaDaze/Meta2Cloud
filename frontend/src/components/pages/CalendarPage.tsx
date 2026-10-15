import { useState, useEffect, useRef } from 'react'
import { RefreshCw, Star, ChevronDown, Film } from 'lucide-react'
import { getBangumiCalendar, testParse } from '@/api'
import BackToTopButton from '@/components/BackToTopButton'
import { StatePanel } from '@/components/StatePanel'
import { Button } from '@/components/ui/button'
import PageHeader from '@/components/shared/PageHeader'
import { cn } from '@/lib/utils'

interface BangumiItem {
  id: number
  name: string
  name_cn: string
  images?: {
    large?: string
    common?: string
    medium?: string
  }
  rating?: {
    score: number
  }
}

interface BangumiWeekday {
  weekday: {
    id: number
    cn: string
    en: string
  }
  items: BangumiItem[]
}

function normalizeBangumiUrl(url: string | undefined): string | undefined {
  if (!url) return url
  if (url.startsWith('http://')) {
    return `https://${url.slice('http://'.length)}`
  }
  return url
}

async function fetchCalendar(): Promise<BangumiWeekday[]> {
  const res = await getBangumiCalendar()
  const json: BangumiWeekday[] = res.data

  return json.map((day) => ({
    ...day,
    items: day.items.map((item) => ({
      ...item,
      images: item.images
        ? {
            large: normalizeBangumiUrl(item.images.large),
            common: normalizeBangumiUrl(item.images.common),
            medium: normalizeBangumiUrl(item.images.medium),
          }
        : undefined,
    })),
  })).sort((a, b) => a.weekday.id - b.weekday.id)
}

function AnimeCard({ item, onSearch, index }: { item: BangumiItem; onSearch: (item: BangumiItem) => void; index: number }) {
  const cover = item.images?.large || item.images?.common || item.images?.medium
  const title = item.name_cn || item.name
  const score = item.rating?.score ?? 0

  return (
    <div
      onClick={() => onSearch(item)}
      style={{ animationDelay: `${Math.min(index * 30, 360)}ms` }}
      className="group animate-stagger-in relative flex cursor-pointer flex-col overflow-hidden rounded-xl border border-border/70 bg-card transition-all duration-300 ease-[cubic-bezier(0.22,1,0.36,1)] hover:-translate-y-1.5 hover:border-primary/40 hover:shadow-[0_16px_40px_-12px_oklch(8%_0.01_140/0.9),0_0_24px_-8px_oklch(76%_0.19_150/0.35)]"
    >
      <div className="relative aspect-[2/3] overflow-hidden bg-muted">
        {cover ? (
          <img
            src={cover}
            alt={title}
            loading="lazy"
            className="h-full w-full object-cover transition-transform duration-500 ease-[cubic-bezier(0.22,1,0.36,1)] group-hover:scale-[1.06]"
            onError={(e) => { e.currentTarget.style.display = 'none' }}
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-muted-foreground">
            <Film className="size-8" />
          </div>
        )}
        {score > 0 && (
          <div className="absolute bottom-2 left-2 flex items-center gap-1 rounded-full bg-black/65 px-2 py-0.5 text-[11px] font-bold backdrop-blur-md">
            <Star className="size-3 fill-warning text-warning" />
            {score.toFixed(1)}
          </div>
        )}
      </div>
      <div className="flex h-14 flex-col gap-1 bg-card p-2.5 sm:h-[65px]">
        <p className="line-clamp-2 text-[13px] font-semibold leading-snug">{title}</p>
      </div>
    </div>
  )
}

function WeekdaySection({
  weekday,
  items,
  isToday,
  onSearch,
}: {
  weekday: { id: number; cn: string; en: string }
  items: BangumiItem[]
  isToday: boolean
  onSearch: (item: BangumiItem) => void
}) {
  const [expanded, setExpanded] = useState(true)
  const sectionRef = useRef<HTMLElement>(null)

  useEffect(() => {
    if (isToday && sectionRef.current) {
      setTimeout(() => {
        sectionRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
      }, 300)
    }
  }, [isToday])

  if (!items || items.length === 0) return null

  return (
    <section ref={sectionRef} className="mb-8">
      <div onClick={() => setExpanded((v) => !v)} className="mb-4 flex cursor-pointer select-none items-center gap-3">
        {isToday && (
          <span className="flex h-5 shrink-0 items-center rounded-full bg-lime px-2 text-[10px] font-bold uppercase tracking-wide text-[oklch(18%_0.01_140)] glow-lime">
            今日
          </span>
        )}
        <span className={cn('shrink-0 text-[22px] font-bold tracking-tight', isToday && 'text-gradient-tide')}>
          {weekday.cn}
        </span>
        <span className="shrink-0 text-[15px] text-muted-foreground">{weekday.en}</span>
        <span
          className={cn(
            'shrink-0 rounded-full px-2 py-0.5 text-[13px] font-semibold tabular-nums',
            isToday ? 'bg-primary/15 text-primary' : 'bg-muted text-muted-foreground'
          )}
        >
          {items.length}
        </span>
        <span className="h-px flex-1 bg-gradient-to-r from-border via-border/50 to-transparent" />
        <ChevronDown
          className={cn(
            'size-4 text-muted-foreground transition-transform duration-300 ease-[cubic-bezier(0.22,1,0.36,1)]',
            expanded ? 'rotate-180' : ''
          )}
        />
      </div>

      <div
        className={cn(
          'grid overflow-hidden transition-all duration-300 ease-[cubic-bezier(0.22,1,0.36,1)]',
          expanded
            ? 'grid-cols-2 gap-3 opacity-100 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-10'
            : 'grid-rows-[0fr] opacity-0'
        )}
        style={expanded ? undefined : { gridTemplateRows: '0fr' }}
      >
        {expanded && items.map((item, index) => (
          <AnimeCard key={item.id} item={item} onSearch={onSearch} index={index} />
        ))}
      </div>
    </section>
  )
}

function LazyWeekdaySection({
  weekday,
  items,
  isToday,
  onSearch,
  eager = false,
}: {
  weekday: { id: number; cn: string; en: string }
  items: BangumiItem[]
  isToday: boolean
  onSearch: (item: BangumiItem) => void
  eager?: boolean
}) {
  const [revealed, setRevealed] = useState(eager)
  const anchorRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (eager) return
    const node = anchorRef.current
    if (!node || revealed) return

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) {
          setRevealed(true)
          observer.disconnect()
        }
      },
      { rootMargin: '900px 0px' }
    )
    observer.observe(node)
    return () => observer.disconnect()
  }, [eager, revealed])

  if (!items || items.length === 0) return null

  return (
    <div ref={anchorRef}>
      {revealed ? (
        <WeekdaySection weekday={weekday} items={items} isToday={isToday} onSearch={onSearch} />
      ) : (
        <section className="mb-8">
          <div className="mb-4 flex items-center gap-3">
            {isToday && (
              <span className="flex h-5 shrink-0 items-center rounded-full bg-lime px-2 text-[10px] font-bold uppercase tracking-wide text-[oklch(18%_0.01_140)]">
                今日
              </span>
            )}
            <span className={cn('shrink-0 text-[22px] font-bold tracking-tight', isToday && 'text-gradient-tide')}>
              {weekday.cn}
            </span>
            <span className="shrink-0 text-[15px] text-muted-foreground">{weekday.en}</span>
            <span
              className={cn(
                'shrink-0 rounded-full px-2 py-0.5 text-[13px] font-semibold tabular-nums',
                isToday ? 'bg-primary/15 text-primary' : 'bg-muted text-muted-foreground'
              )}
            >
              {items.length}
            </span>
            <span className="h-px flex-1 bg-gradient-to-r from-border via-border/50 to-transparent" />
          </div>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-10">
            {Array.from({ length: Math.min(items.length, 6) }, (_, index) => (
              <div
                key={`${weekday.id}-placeholder-${index}`}
                className="aspect-[2/3] animate-pulse rounded-xl bg-muted/70"
              />
            ))}
          </div>
        </section>
      )}
    </div>
  )
}

interface CalendarPageProps {
  onSearch?: (title: string) => void
}

export default function CalendarPage({ onSearch }: CalendarPageProps) {
  const [data, setData] = useState<BangumiWeekday[] | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null)

  const jsDayToBgm = (d: number) => (d === 0 ? 7 : d)
  const todayBgmId = jsDayToBgm(new Date().getDay())
  const eagerWeekdays = new Set([
    todayBgmId,
    todayBgmId === 1 ? 7 : todayBgmId - 1,
    todayBgmId === 7 ? 1 : todayBgmId + 1,
  ])

  useEffect(() => {
    fetchCalendar()
      .then((sorted) => {
        setData(sorted)
        setError(null)
        setLastUpdated(new Date())
        setLoading(false)
      })
      .catch((err) => {
        setError(err.message || '加载失败')
        setLoading(false)
      })
  }, [])

  function handleRefresh() {
    setLoading(true)
    setError(null)
    fetchCalendar()
      .then((sorted) => {
        setData(sorted)
        setLastUpdated(new Date())
        setLoading(false)
      })
      .catch((err) => {
        setError(err.message || '加载失败')
        setLoading(false)
      })
  }

  async function handleSearch(item: BangumiItem) {
    // 从新番列表跳转时，先解析标题提取作品名，再用解析后的名称进行 TMDB 搜索
    const rawTitle = item.name_cn || item.name
    try {
      const res = await testParse(rawTitle, true)  // skip_tmdb=true，只解析不查询
      const parsedName = res.data?.parsed?.name
      // 使用解析后的名称，如果没有解析出名称则使用原标题
      onSearch?.(parsedName || rawTitle)
    } catch {
      // 解析失败时使用原标题
      onSearch?.(rawTitle)
    }
  }

  return (
    <div className="py-2">
      <PageHeader
        kicker="Bangumi · 新番放送"
        title="新番列表"
        description="按星期浏览本季放送的新番，点击卡片可直接跳转到资源检索。"
        actions={
          <div className="flex items-center gap-3">
            {lastUpdated && (
              <p className="text-xs text-muted-foreground">
                更新于 {lastUpdated.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })}
              </p>
            )}
            <Button
              variant="outline"
              onClick={handleRefresh}
              disabled={loading}
              className="gap-2 rounded-full"
            >
              <RefreshCw className={cn('size-4', loading && 'animate-spin')} />
              {loading ? '加载中…' : '刷新'}
            </Button>
          </div>
        }
      />

      {error && !loading && (
        <div className="mb-6">
          <StatePanel
            icon="!"
            title={`加载失败：${error}`}
            description="请检查网络连接，或稍后刷新重试。"
            tone="danger"
            compact
          />
        </div>
      )}

      {loading && (
        <div className="space-y-8">
          {[1, 2, 3].map((i) => (
            <div key={i}>
              <div className="mb-4 h-10 w-48 animate-pulse rounded-xl bg-muted/70" />
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-10">
                {Array.from({ length: 8 }, (_, j) => (
                  <div key={j} className="aspect-[2/3] animate-pulse rounded-xl bg-muted/70" />
                ))}
              </div>
            </div>
          ))}
        </div>
      )}

      {!loading && data && (
        <div>
          {data.map(({ weekday, items }) => (
            <LazyWeekdaySection
              key={weekday.id}
              weekday={weekday}
              items={items || []}
              isToday={weekday.id === todayBgmId}
              onSearch={handleSearch}
              eager={eagerWeekdays.has(weekday.id)}
            />
          ))}
        </div>
      )}

      {!loading && data && (
        <div className="mt-4 text-center text-xs text-muted-foreground">
          数据来源：
          <a href="https://bgm.tv" target="_blank" rel="noopener noreferrer" className="text-primary transition-colors hover:underline">
            Bangumi 番组计划
          </a>
        </div>
      )}

      <BackToTopButton />
    </div>
  )
}
