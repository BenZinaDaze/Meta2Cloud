import { useState, useEffect, useRef } from 'react'
import { RefreshCw, Star, ChevronDown, Film } from 'lucide-react'
import { testParse } from '@/api'
import { StatePanel } from '@/components/StatePanel'
import { Button } from '@/components/ui/button'

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
  const res = await fetch('https://api.bgm.tv/calendar', {
    headers: { 'User-Agent': 'Meta2Cloud/1.0 (https://github.com/BenZinaDaze/Meta2Cloud)' },
  })
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  const json: BangumiWeekday[] = await res.json()

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

function AnimeCard({ item, onSearch }: { item: BangumiItem; onSearch: (item: BangumiItem) => void }) {
  const cover = item.images?.large || item.images?.common || item.images?.medium
  const title = item.name_cn || item.name
  const score = item.rating?.score ?? 0

  return (
    <div
      onClick={() => onSearch(item)}
      className="group relative flex cursor-pointer flex-col overflow-hidden rounded-xl transition-all duration-200 hover:-translate-y-1"
    >
      <div className="relative aspect-[2/3] overflow-hidden bg-muted">
        {cover ? (
          <img
            src={cover}
            alt={title}
            loading="lazy"
            className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
            onError={(e) => { e.currentTarget.style.display = 'none' }}
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-muted-foreground">
            <Film className="size-8" />
          </div>
        )}
        {score > 0 && (
          <div className="absolute bottom-2 left-2 flex items-center gap-1 rounded-full bg-black/70 px-2 py-0.5 text-[11px] font-bold backdrop-blur-sm">
            <Star className="size-3 fill-warning text-warning" />
            {score.toFixed(1)}
          </div>
        )}
      </div>
      <div className="flex h-14 flex-col gap-1 rounded-b-xl bg-muted p-2.5 sm:h-[65px]">
        <p className="line-clamp-2 text-sm font-semibold leading-snug">{title}</p>
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
      <div onClick={() => setExpanded((v) => !v)} className="mb-4 flex cursor-pointer items-center gap-3 select-none">
        {isToday && (
          <span className="flex h-5 shrink-0 items-center rounded-full bg-brand px-2 text-[10px] font-bold uppercase tracking-wide text-primary-foreground">
            今日
          </span>
        )}
        <span className={`shrink-0 text-[22px] font-bold ${isToday ? 'text-brand' : ''}`}>
          {weekday.cn}
        </span>
        <span className="shrink-0 text-[15px] text-muted-foreground">{weekday.en}</span>
        <span
          className={`shrink-0 rounded-full px-2 py-0.5 text-[13px] font-semibold ${
            isToday ? 'bg-brand/20 text-brand' : 'bg-muted text-muted-foreground'
          }`}
        >
          {items.length}
        </span>
        <span className="flex-1 h-px bg-border/50" />
        <ChevronDown
          className={`size-4 text-muted-foreground transition-transform duration-200 ${expanded ? 'rotate-180' : ''}`}
        />
      </div>

      <div
        className={`grid gap-3 overflow-hidden transition-all duration-300 ease-in-out ${
          expanded ? 'grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-10 opacity-100' : 'max-h-0 opacity-0'
        }`}
      >
        {items.map((item) => (
          <AnimeCard key={item.id} item={item} onSearch={onSearch} />
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
              <span className="flex h-5 shrink-0 items-center rounded-full bg-brand px-2 text-[10px] font-bold uppercase tracking-wide text-primary-foreground">
                今日
              </span>
            )}
            <span className={`shrink-0 text-[22px] font-bold ${isToday ? 'text-brand' : ''}`}>
              {weekday.cn}
            </span>
            <span className="shrink-0 text-[15px] text-muted-foreground">{weekday.en}</span>
            <span
              className={`shrink-0 rounded-full px-2 py-0.5 text-[13px] font-semibold ${
                isToday ? 'bg-brand/20 text-brand' : 'bg-muted text-muted-foreground'
              }`}
            >
              {items.length}
            </span>
            <span className="flex-1 h-px bg-border/50" />
          </div>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-10">
            {Array.from({ length: Math.min(items.length, 6) }, (_, index) => (
              <div
                key={`${weekday.id}-placeholder-${index}`}
                className="aspect-[2/3] animate-pulse rounded-xl bg-muted"
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
    <div className="py-6">
      <div className="mb-8 flex items-start justify-between gap-4">
        <div>
          <div className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
            Bangumi · 新番放送
          </div>
          <h1 className="mt-1 text-3xl font-bold">新番列表</h1>
          {lastUpdated && (
            <p className="mt-1 text-xs text-muted-foreground">
              更新于 {lastUpdated.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })}
            </p>
          )}
        </div>
        <Button
          variant="outline"
          onClick={handleRefresh}
          disabled={loading}
          className="gap-2"
        >
          <RefreshCw className={`size-4 ${loading ? 'animate-spin' : ''}`} />
          {loading ? '加载中…' : '刷新'}
        </Button>
      </div>

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
              <div className="mb-4 h-10 w-48 animate-pulse rounded-xl bg-muted" />
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-10">
                {Array.from({ length: 8 }, (_, j) => (
                  <div key={j} className="aspect-[2/3] animate-pulse rounded-xl bg-muted" />
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
          <a href="https://bgm.tv" target="_blank" rel="noopener noreferrer" className="text-brand hover:underline">
            Bangumi 番组计划
          </a>
        </div>
      )}
    </div>
  )
}
