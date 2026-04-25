import { useState, useEffect, useRef } from 'react'
import { ChevronLeft, ChevronDown, RefreshCw, Rss, Copy, ArrowUp } from 'lucide-react'
import { searchMedia, getEpisodes, addAria2Uri, addU115OfflineUrls, getU115OauthStatus, tmdbGetAlternativeNames } from '@/api'
import { StatePanel } from '@/components/StatePanel'
import { Button } from '@/components/ui/button'
import SubscriptionModal from '@/components/modals/SubscriptionModal'
import type { MediaItem, Subscription } from '@/types/api'
import { toast } from 'sonner'
import _resultsCache from '@/utils/resultsCache'

interface Episode {
  title: string
  magnet_url?: string
  torrent_url?: string
  file_size_mb?: number
  publish_time?: string
  _site?: string
}

export interface SubgroupGroup {
  name: string
  mediaTitle: string
  uniqueKey: string
  rssUrl: string | null
  src: { site: string; media_id: string; subgroup_id: string | undefined } | null
  episodes: Episode[]
  loading: boolean
}

interface ScraperResultsViewProps {
  item: MediaItem
  onBack: () => void
  aria2Enabled?: boolean
}

const MIKAN_BASE = 'https://mikan.tangbai.cc'

function jaccardSimilarity(a: string, b: string): number {
  const setA = new Set(a)
  const setB = new Set(b)
  if (setA.size === 0 && setB.size === 0) return 1
  const intersection = new Set([...setA].filter(x => setB.has(x)))
  const union = new Set([...setA, ...setB])
  return intersection.size / union.size
}

function filterByKnownNames(candidates: { name: string; sources: { site: string; media_id: string; subgroup_id?: string }[] }[], knownNames: string[]): typeof candidates {
  const normalize = (s: string) => s.toLowerCase().replace(/\s+/g, ' ').trim()
  const stripSubgroup = (s: string) => s.replace(/\s*\[[^\]]+\]\s*$/, '').trim()
  const normalizedKnown = knownNames.map(normalize).filter(Boolean)
  if (normalizedKnown.length === 0) return candidates

  return candidates.filter(agg => {
    const rawName = normalize(agg.name)
    const cleanName = normalize(stripSubgroup(agg.name))

    return normalizedKnown.some(k => {
      if (rawName.includes(k) || k.includes(rawName)) return true
      if (cleanName.includes(k) || k.includes(cleanName)) return true
      if (jaccardSimilarity(cleanName, k) >= 0.50) return true
      return false
    })
  })
}

export default function ScraperResultsView({ item, onBack, aria2Enabled = false }: ScraperResultsViewProps) {
  const displayTitle = item.title || item.original_title || ''
  const searchKey = `${item.tmdb_id || ''}:${displayTitle}`

  const [searchState, setSearchState] = useState<'idle' | 'searching' | 'done' | 'error'>(() => _resultsCache[searchKey]?.searchState ?? 'idle')
  const [errorMsg, setErrorMsg] = useState(() => _resultsCache[searchKey]?.errorMsg ?? '')
  const [groupedEpisodes, setGroupedEpisodes] = useState<SubgroupGroup[]>(() => _resultsCache[searchKey]?.groupedEpisodes ?? [])
  const [stateSearchKey, setStateSearchKey] = useState(searchKey)
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(new Set())
  const [showTop, setShowTop] = useState(false)
  const [usedSearchKey, setUsedSearchKey] = useState<string | null>(null)
  const [currentSearchKey, setCurrentSearchKey] = useState<string | null>(null)
  const [activeMediaTitle, setActiveMediaTitle] = useState<string | null>(null)
  const [u115Authorized, setU115Authorized] = useState(false)
  const [subscriptionDraft, setSubscriptionDraft] = useState<Partial<Subscription> | null>(null)
  const scrollRef = useRef<HTMLDivElement>(null)
  const abortControllerRef = useRef<AbortController | null>(null)

  // 同步状态到缓存
  useEffect(() => {
    if (stateSearchKey !== searchKey) return
    _resultsCache[searchKey] = { searchState, errorMsg, groupedEpisodes }
  }, [searchKey, stateSearchKey, searchState, errorMsg, groupedEpisodes])

  useEffect(() => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort()
      abortControllerRef.current = null
    }

    const cached = _resultsCache[searchKey]
    setStateSearchKey(searchKey)
    setSearchState(cached?.searchState ?? 'idle')
    setErrorMsg(cached?.errorMsg ?? '')
    setGroupedEpisodes(cached?.groupedEpisodes ?? [])
    setCollapsedGroups(new Set())
    setUsedSearchKey(null)
    setCurrentSearchKey(null)
    setActiveMediaTitle(null)
    setShowTop(false)

    if (!cached || cached.searchState === 'idle') {
      startSearch()
    }
    return () => {
      // 组件卸载时取消所有进行中的请求
      if (abortControllerRef.current) {
        abortControllerRef.current.abort()
        abortControllerRef.current = null
      }
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchKey])

  useEffect(() => {
    let cancelled = false
    getU115OauthStatus()
      .then((res) => {
        if (!cancelled) setU115Authorized(!!res?.data?.authorized)
      })
      .catch(() => {
        if (!cancelled) setU115Authorized(false)
      })
    return () => { cancelled = true }
  }, [])

  const buildFallbackKeys = (altNames: { name: string; iso_639_1?: string }[], primaryKey: string) => {
    const seen = new Set([primaryKey])
    const zhKeys: string[] = []
    const jaKeys: string[] = []
    const otherKeys: string[] = []
    for (const { name, iso_639_1 } of (altNames || [])) {
      if (!name || seen.has(name)) continue
      seen.add(name)
      if (iso_639_1 === 'zh') zhKeys.push(name)
      else if (iso_639_1 === 'ja') jaKeys.push(name)
      else otherKeys.push(name)
    }
    return { zhKeys, jaKeys, otherKeys }
  }

  const startSearch = async () => {
    if (!item) return
    // 取消之前的请求
    if (abortControllerRef.current) {
      abortControllerRef.current.abort()
    }
    abortControllerRef.current = new AbortController()
    const signal = abortControllerRef.current.signal

    setSearchState('searching')
    setErrorMsg('')
    setGroupedEpisodes([])

    try {
      const primaryKey = item.title || item.original_title || ''
      let aggregates: { name: string; sources: { site: string; media_id: string; subgroup_id?: string }[] }[] = []
      setCurrentSearchKey(primaryKey)

      const primaryRes = await searchMedia(primaryKey, { signal })
      const primaryCandidates = primaryRes.data.results || []
      if (primaryCandidates.length > 0) {
        aggregates = primaryCandidates
      }

      if (aggregates.length === 0 && item.tmdb_id) {
        let altNames: { name: string; iso_639_1?: string }[] = []
        try {
          const res = await tmdbGetAlternativeNames(item.media_type, item.tmdb_id, { signal })
          altNames = res.data?.alternative_names || []
        } catch {
          // ignore
        }

        if (altNames.length > 0) {
          const { zhKeys, jaKeys, otherKeys } = buildFallbackKeys(altNames, primaryKey)
          const allKnownNames = [item.title, item.original_title, ...altNames.map(a => a.name)].filter(Boolean) as string[]

          const aliasKeys = [
            ...zhKeys.map(k => ({ key: k, label: '中文别名' })),
            ...jaKeys.map(k => ({ key: k, label: '日文别名' })),
            ...otherKeys.map(k => ({ key: k, label: '其他别名' })),
          ]

          for (const { key } of aliasKeys) {
            if (signal.aborted) return
            setCurrentSearchKey(key)
            const res = await searchMedia(key, { signal })
            const rawCandidates = res.data.results || []
            const candidates = filterByKnownNames(rawCandidates, allKnownNames)
            if (candidates.length > 0) {
              aggregates = candidates
              setUsedSearchKey(key)
              break
            }
          }
        }
      }

      if (signal.aborted) return

      if (aggregates.length === 0) {
        setSearchState('error')
        setErrorMsg('各大爬虫站点均未匹配到此资源。')
        return
      }

      const subgroupAggs = aggregates.filter(agg => agg.sources.some(s => s.subgroup_id))

      const initialGroups: SubgroupGroup[] = subgroupAggs.map(agg => {
        const src = agg.sources.find(s => s.subgroup_id)
        const rssUrl = src?.site === 'mikan'
          ? `${MIKAN_BASE}/RSS/Bangumi?bangumiId=${src.media_id}&subgroupid=${src.subgroup_id}`
          : null

        const match = agg.name.match(/^(.*?)\s*\[(.+?)\]\s*$/)
        const mediaTitle = match ? match[1].trim() : agg.name
        const name = match ? match[2].trim() : agg.name
        const uniqueKey = `${mediaTitle}-${name}`

        return { name, mediaTitle, uniqueKey, rssUrl, src: src ? { site: src.site, media_id: src.media_id, subgroup_id: src.subgroup_id } : null, episodes: [], loading: true }
      })
      setGroupedEpisodes(initialGroups)
      setSearchState('done')

      const CONCURRENCY = 2
      let nextIndex = 0

      const worker = async () => {
        while (nextIndex < subgroupAggs.length && !signal.aborted) {
          const idx = nextIndex++
          const agg = subgroupAggs[idx]
          const src = agg.sources.find(s => s.subgroup_id)
          if (!src) continue
          try {
            const epRes = await getEpisodes(src.site, src.media_id, src.subgroup_id, { signal })
            if (signal.aborted) return
            const episodes = (epRes.data.episodes || []).map((e: Episode) => ({ ...e, _site: src.site }))
            setGroupedEpisodes(prev => {
              const updated = [...prev]
              updated[idx] = { ...updated[idx], episodes, loading: false, src: updated[idx].src }
              return updated
            })
          } catch {
            if (signal.aborted) return
            setGroupedEpisodes(prev => {
              const updated = [...prev]
              updated[idx] = { ...updated[idx], loading: false }
              return updated
            })
          }
        }
      }

      await Promise.all(Array.from({ length: Math.min(CONCURRENCY, subgroupAggs.length) }, worker))

    } catch (e) {
      if (signal.aborted) return
      setSearchState('error')
      setErrorMsg((e as { response?: { data?: { detail?: string } }; message?: string })?.response?.data?.detail || (e as Error).message)
    }
  }

  const refreshGroup = async (index: number) => {
    setGroupedEpisodes(prev => {
      const updated = [...prev]
      updated[index] = { ...updated[index], loading: true }
      return updated
    })
    const src = groupedEpisodes[index]?.src
    if (!src) return
    try {
      const epRes = await getEpisodes(src.site, src.media_id, src.subgroup_id)
      const episodes = (epRes.data.episodes || []).map((e: Episode) => ({ ...e, _site: src.site }))
      setGroupedEpisodes(prev => {
        const updated = [...prev]
        updated[index] = { ...updated[index], episodes, loading: false }
        return updated
      })
    } catch {
      setGroupedEpisodes(prev => {
        const updated = [...prev]
        updated[index] = { ...updated[index], loading: false }
        return updated
      })
    }
  }

  const handlePushAria2 = async (ep: Episode) => {
    const url = ep.magnet_url || ep.torrent_url
    if (!url) {
      toast.warning('无效下载链接', { description: '该资源缺少真实下载地址' })
      return
    }
    try {
      await addAria2Uri({ uris: [url], title: ep.title })
      toast.success('已推送到下载器', { description: ep.title })
    } catch (e) {
      toast.error('下载失败', { description: (e as Error).message })
    }
  }

  const handlePushU115 = async (ep: Episode) => {
    const url = ep.magnet_url || ep.torrent_url
    if (!url) {
      toast.warning('无效下载链接', { description: '该资源缺少真实下载地址' })
      return
    }
    try {
      await addU115OfflineUrls({ urls: url })
      toast.success('已推送到云下载', { description: ep.title })
    } catch (e) {
      toast.error('云下载失败', { description: (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail || (e as Error).message })
    }
  }

  const handleCopy = (ep: Episode) => {
    const url = ep.magnet_url || ep.torrent_url
    if (!url) return
    navigator.clipboard.writeText(url)
      .then(() => toast.success('已复制链接'))
      .catch((e) => toast.error('复制失败', { description: e.message }))
  }

  // Group by media title
  const groupedByMedia = new Map<string, (SubgroupGroup & { originalIndex: number })[]>()
  groupedEpisodes.forEach((item, index) => {
    if (!groupedByMedia.has(item.mediaTitle)) {
      groupedByMedia.set(item.mediaTitle, [])
    }
    groupedByMedia.get(item.mediaTitle)!.push({ ...item, originalIndex: index })
  })
  const mediaGroups = Array.from(groupedByMedia.entries())
  const currentActiveMedia = activeMediaTitle && groupedByMedia.has(activeMediaTitle)
    ? activeMediaTitle
    : (mediaGroups[0]?.[0] || null)

  return (
    <div className="flex flex-1 flex-col">
      <section className="flex flex-1 flex-col rounded-2xl border bg-card p-4 sm:rounded-3xl sm:p-7">
        <div
          ref={scrollRef}
          onScroll={(e) => setShowTop((e.target as HTMLDivElement).scrollTop > 300)}
          className="flex-1 min-h-0 overflow-y-auto"
        >
          {/* Header */}
          <div className="mb-6 flex items-center gap-4">
            <Button variant="outline" size="icon" onClick={onBack} className="shrink-0 rounded-full">
              <ChevronLeft className="size-5" />
            </Button>
            <div className="min-w-0 flex-1">
              <h2 className="truncate text-xl font-bold sm:text-2xl">{displayTitle}</h2>
              <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground sm:text-sm">
                资源检索结果
                {usedSearchKey && (
                  <span className="inline-flex items-center gap-1 rounded-full bg-warning/10 px-2 py-0.5 text-[10px] font-medium text-warning border border-warning/20">
                    以别名「{usedSearchKey}」检索
                  </span>
                )}
              </div>
            </div>
          </div>

          {/* Search states */}
          {searchState === 'searching' && (
            <StatePanel
              title={`以「${currentSearchKey || displayTitle}」检索中`}
              description={currentSearchKey && currentSearchKey !== displayTitle ? '主标题无结果，正在尝试别名。' : '正在从源站聚合可用资源，请稍候。'}
              compact
            />
          )}

          {searchState === 'error' && (
            <StatePanel
              icon="!"
              title="检索失败或无匹配资源"
              description={errorMsg || '请更换关键词，或稍后重新尝试。'}
              tone="danger"
              compact
            />
          )}

          {searchState === 'done' && (
            <div className="pb-8">
              {/* Media groups selector */}
              <div className="mb-6 border-b pb-4">
                <h3 className="mb-3 text-base font-bold">匹配到的相关番剧内容 ({mediaGroups.length}部)</h3>
                {mediaGroups.length > 0 && (
                  <div className="flex flex-wrap gap-2">
                    {mediaGroups.map(([mediaTitle, subgroups]) => {
                      const count = subgroups.reduce((s, g) => s + g.episodes.length, 0)
                      const isActive = mediaTitle === currentActiveMedia
                      return (
                        <Button
                          key={mediaTitle}
                          variant={isActive ? 'default' : 'outline'}
                          size="sm"
                          onClick={() => setActiveMediaTitle(mediaTitle)}
                          className="rounded-xl"
                        >
                          {mediaTitle || '未分类源'} ({count}源)
                        </Button>
                      )
                    })}
                  </div>
                )}
              </div>

              {groupedEpisodes.length === 0 ? (
                <StatePanel icon="📭" title="没有获取到有效的剧集种子/磁力项" description="请尝试其他关键词或字幕组。" />
              ) : (
                <div className="flex flex-col gap-10">
                  {mediaGroups.filter(([mediaTitle]) => mediaTitle === currentActiveMedia).map(([mediaTitle, subgroups]) => (
                    <div key={mediaTitle} className="flex flex-col gap-4">
                      {/* Subgroup quick nav */}
                      {subgroups.length > 0 && (
                        <div className="flex flex-wrap gap-2 pb-2">
                          {subgroups.map(({ name, episodes, uniqueKey }) => (
                            <Button
                              key={uniqueKey}
                              variant="outline"
                              size="sm"
                              onClick={() => {
                                const el = document.getElementById(`fansub-${encodeURIComponent(uniqueKey)}`)
                                if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' })
                              }}
                              className="rounded-lg text-xs"
                            >
                              {name} ({episodes.length})
                            </Button>
                          ))}
                        </div>
                      )}

                      {/* Subgroup sections */}
                      <div className="flex flex-col gap-6">
                        {subgroups.map((subItem) => {
                          const { name, rssUrl, episodes, loading, originalIndex, uniqueKey } = subItem
                          const isExpanded = !collapsedGroups.has(uniqueKey)
                          const toggle = () => setCollapsedGroups(prev => {
                            const next = new Set(prev)
                            if (next.has(uniqueKey)) {
                              next.delete(uniqueKey)
                            } else {
                              next.add(uniqueKey)
                            }
                            return next
                          })

                          return (
                            <div key={uniqueKey} id={`fansub-${encodeURIComponent(uniqueKey)}`} className="flex flex-col">
                              <button onClick={toggle} className="flex w-full items-center gap-3 py-2 text-left">
                                <div className={`w-1.5 shrink-0 rounded-full bg-brand transition-all duration-300 ${isExpanded ? 'h-6 opacity-100' : 'h-3 opacity-40'}`} />
                                <h4 className="flex-1 text-lg font-bold">
                                  {name}
                                  {loading ? (
                                    <span className="ml-2 text-sm font-normal text-muted-foreground">加载中...</span>
                                  ) : (
                                    <span className="ml-2 text-sm font-medium text-muted-foreground">({episodes.length})</span>
                                  )}
                                </h4>
                                {rssUrl && (
                                  <div className="flex shrink-0 items-center gap-2">
                                    <Button
                                      variant="outline"
                                      size="sm"
                                      onClick={(e) => {
                                        e.stopPropagation()
                                        navigator.clipboard.writeText(rssUrl)
                                          .then(() => toast.success('已复制 RSS 链接'))
                                          .catch((err) => toast.error('复制失败', { description: err.message }))
                                      }}
                                      className="gap-1.5 rounded-lg border-warning/30 text-warning hover:border-warning/60"
                                    >
                                      <Rss className="size-3" />
                                      复制 RSS
                                    </Button>
                                    <Button
                                      variant="outline"
                                      size="sm"
                                      onClick={(e) => {
                                        e.stopPropagation()
                                        setSubscriptionDraft({
                                          name: `${mediaTitle || displayTitle} [${name}]`,
                                          media_title: mediaTitle || displayTitle,
                                          media_type: item.media_type || 'tv',
                                          tmdb_id: item.tmdb_id,
                                          poster_url: item.poster_url,
                                          site: 'mikan',
                                          rss_url: rssUrl,
                                          subgroup_name: name,
                                          season_number: 1,
                                          start_episode: 1,
                                          keyword_all: [],
                                          push_target: u115Authorized ? 'u115' : 'aria2',
                                          enabled: true,
                                        })
                                      }}
                                      className="gap-1.5 rounded-lg border-success/30 text-success hover:border-success/60"
                                    >
                                      <Rss className="size-3" />
                                      订阅 RSS
                                    </Button>
                                  </div>
                                )}
                                <Button
                                  variant="ghost"
                                  size="icon-xs"
                                  onClick={(e) => { e.stopPropagation(); refreshGroup(originalIndex) }}
                                  title="刷新该字幕组的资源列表"
                                >
                                  <RefreshCw className={`size-3 ${loading ? 'animate-spin' : ''}`} />
                                </Button>
                                <ChevronDown className={`size-4 shrink-0 text-muted-foreground transition-all duration-200 ${isExpanded ? 'rotate-180' : ''}`} />
                              </button>

                              {loading ? (
                                <div className="mt-1 flex flex-col gap-2 pb-4">
                                  {[1, 2, 3].map(i => (
                                    <div key={i} className="h-16 animate-pulse rounded-2xl border bg-muted" />
                                  ))}
                                </div>
                              ) : (
                                <div className={`grid transition-[grid-template-rows] duration-300 ease-in-out ${isExpanded ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]'}`}>
                                  <div className="overflow-hidden">
                                    <div className="mt-1 flex flex-col gap-3 pb-4">
                                      {episodes.length === 0 ? (
                                        <div className="px-2 py-3 text-sm text-muted-foreground">该字幕组暂无资源</div>
                                      ) : episodes.map((ep, idx) => (
                                        <div key={idx} className="flex flex-col gap-4 rounded-2xl border bg-muted/30 p-4 transition-colors hover:bg-muted/50 md:flex-row md:p-5">
                                          <div className="min-w-0 flex-1">
                                            <div className="mb-2 break-all text-sm font-semibold leading-relaxed md:text-base">{ep.title}</div>
                                            <div className="flex flex-wrap gap-2 text-xs font-medium">
                                              <span className="rounded bg-info/20 px-2 py-0.5 text-info">{ep._site?.toUpperCase()}</span>
                                              {ep.file_size_mb && <span className="rounded bg-muted px-2 py-0.5 text-muted-foreground">{ep.file_size_mb} MB</span>}
                                              {ep.publish_time && <span className="rounded bg-muted px-2 py-0.5 text-muted-foreground">{ep.publish_time}</span>}
                                            </div>
                                          </div>
                                          <div className="flex shrink-0 flex-col gap-2 md:max-w-[320px] md:items-end">
                                            {u115Authorized ? (
                                              <Button onClick={() => handlePushU115(ep)} className="w-full md:w-[132px]">
                                                推送云下载
                                              </Button>
                                            ) : aria2Enabled ? (
                                              <Button onClick={() => handlePushAria2(ep)} className="w-full md:w-[132px]">
                                                推送下载
                                              </Button>
                                            ) : null}
                                            <div className="grid grid-cols-2 gap-2 md:flex">
                                              {u115Authorized && aria2Enabled ? (
                                                <Button variant="outline" onClick={() => handlePushAria2(ep)} className="md:w-[132px]">
                                                  推送下载
                                                </Button>
                                              ) : null}
                                              <Button variant="outline" onClick={() => handleCopy(ep)}>
                                                <Copy className="size-4" />
                                                复制链接
                                              </Button>
                                            </div>
                                          </div>
                                        </div>
                                      ))}
                                    </div>
                                  </div>
                                </div>
                              )}
                            </div>
                          )
                        })}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>

        {/* Back to top button */}
        {showTop && (
          <Button
            size="icon"
            onClick={() => scrollRef.current?.scrollTo({ top: 0, behavior: 'smooth' })}
            className="absolute bottom-6 right-6 z-50 rounded-full shadow-lg"
          >
            <ArrowUp className="size-5" />
          </Button>
        )}
      </section>

      <SubscriptionModal
        mode="create"
        initialValue={subscriptionDraft as Subscription}
        aria2Enabled={aria2Enabled}
        u115Authorized={u115Authorized}
        open={!!subscriptionDraft}
        onOpenChange={(open) => !open && setSubscriptionDraft(null)}
        onSaved={() => setSubscriptionDraft(null)}
      />
    </div>
  )
}
