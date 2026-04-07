import React, { useState, useEffect, useRef } from 'react'
import { searchMedia, getEpisodes, addAria2Uri, tmdbGetAlternativeNames } from '../api'

let _resultsCache = {}
export function clearResultsCache(key) { delete _resultsCache[key] }

export default function ScraperResultsView({ item, onBack, onToast }) {
  const searchKey = item.title || item.original_title || item.name

  const [searchState, setSearchState] = useState(() => _resultsCache[searchKey]?.searchState ?? 'idle')
  const [errorMsg, setErrorMsg] = useState(() => _resultsCache[searchKey]?.errorMsg ?? '')
  // groupedEpisodes: [{name, rssUrl, episodes}] — 每个字幕组一条，直接从各自 RSS 获取
  const [groupedEpisodes, setGroupedEpisodes] = useState(() => _resultsCache[searchKey]?.groupedEpisodes ?? [])
  const [collapsedGroups, setCollapsedGroups] = useState(new Set())
  const [showTop, setShowTop] = useState(false)
  const [usedSearchKey, setUsedSearchKey] = useState(null)  // 实际命中的搜索词（如使用了别名则不为 null）
  const [currentSearchKey, setCurrentSearchKey] = useState(null)  // 当前正在尝试的搜索词
  const scrollRef = useRef(null)

  const MIKAN_BASE = 'https://mikan.tangbai.cc'

  // 同步状态到缓存
  useEffect(() => {
    _resultsCache[searchKey] = { searchState, errorMsg, groupedEpisodes }
  }, [searchKey, searchState, errorMsg, groupedEpisodes])

  useEffect(() => {
    if (searchState === 'idle') {
      startSearch()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  /**
   * 构建候补搜索关键词列表：zh别名 → ja别名 → 其他语言别名。
   * 收录全部别名（不限语言），避免因 TMDB 语言码映射不准确而遗漏。
   * 去除与主 key 重复的项，各语言内去重（保留唯一）。
   */
  const buildFallbackKeys = (altNames, primaryKey) => {
    const seen = new Set([primaryKey])
    const zhKeys = []
    const jaKeys = []
    const otherKeys = []
    for (const { name, iso_639_1 } of (altNames || [])) {
      if (!name || seen.has(name)) continue
      seen.add(name)
      if (iso_639_1 === 'zh') zhKeys.push(name)
      else if (iso_639_1 === 'ja') jaKeys.push(name)
      else otherKeys.push(name)
    }
    return { zhKeys, jaKeys, otherKeys }
  }

  /**
   * 用别名搜索返回结果时，过滤掉与当前作品无关的条目。
   * 策略：只保留名字与已知名称存在包含关系的条目。
   */
  const filterByKnownNames = (candidates, knownNames) => {
    const normalize = (s) => s.toLowerCase().replace(/\s+/g, ' ').trim()
    const normalizedKnown = knownNames.map(normalize).filter(Boolean)
    if (normalizedKnown.length === 0) return candidates
    return candidates.filter(agg => {
      const aggName = normalize(agg.name)
      return normalizedKnown.some(k => aggName.includes(k) || k.includes(aggName))
    })
  }

  const startSearch = async () => {
    if (!item) return
    setSearchState('searching')
    setErrorMsg('')
    setGroupedEpisodes([])
    try {
      const primaryKey = item.title || item.original_title || item.name

      // 第一步：先用主标题搜索
      let aggregates = []
      setCurrentSearchKey(primaryKey)
      const primaryRes = await searchMedia(primaryKey)
      const primaryCandidates = primaryRes.data.results || []
      if (primaryCandidates.length > 0) {
        aggregates = primaryCandidates
      }

      // 第二步：主标题无结果时，才请求 TMDB 别名并尝试
      if (aggregates.length === 0 && item.tmdb_id) {
        let altNames = item.alternative_names || []

        // 库内条目的 alternative_names 为空，需单独从 TMDB 获取
        if (altNames.length === 0) {
          try {
            const res = await tmdbGetAlternativeNames(item.media_type, item.tmdb_id)
            altNames = res.data?.alternative_names || []
            console.info(`[ScraperSearch] 主标题无结果，从 TMDB 获取别名，共 ${altNames.length} 条`)
          } catch (e) {
            console.warn('[ScraperSearch] 获取别名失败', e)
          }
        }

        if (altNames.length > 0) {
          const { zhKeys, jaKeys, otherKeys } = buildFallbackKeys(altNames, primaryKey)

          // 收集已知名字用于别名搜索结果过滤
          const allKnownNames = [
            item.title, item.original_title, item.name,
            ...altNames.map(a => a.name),
          ].filter(Boolean)

          const aliasKeys = [
            ...zhKeys.map(k => ({ key: k, label: '中文别名' })),
            ...jaKeys.map(k => ({ key: k, label: '日文别名' })),
            ...otherKeys.map(k => ({ key: k, label: '其他别名' })),
          ]

          for (const { key, label } of aliasKeys) {
            setCurrentSearchKey(key)
            const res = await searchMedia(key)
            const rawCandidates = res.data.results || []
            const candidates = filterByKnownNames(rawCandidates, allKnownNames)
            if (candidates.length > 0) {
              aggregates = candidates
              console.info(`[ScraperSearch] 改用${label}「${key}」找到 ${candidates.length} 项（过滤前 ${rawCandidates.length} 项）`)
              setUsedSearchKey(key)
              break
            }
            if (rawCandidates.length > 0) {
              console.info(`[ScraperSearch] ${label}「${key}」有 ${rawCandidates.length} 项但过滤后全为无关作品，继续`)
            }
          }
        }
      }

      if (aggregates.length === 0) {
        setSearchState('error')
        setErrorMsg('各大爬虫站点均未匹配到此资源。')
        return
      }

      // 只取有 subgroup_id 的 aggregate（跳过"全部"那条）
      const subgroupAggs = aggregates.filter(agg =>
        agg.sources.some(s => s.subgroup_id)
      )

      // 立即展示所有字幕组占位，不等剧集加载
      const initialGroups = subgroupAggs.map(agg => {
        const src = agg.sources.find(s => s.subgroup_id)
        const rssUrl = src?.site === 'mikan'
          ? `${MIKAN_BASE}/RSS/Bangumi?bangumiId=${src.media_id}&subgroupid=${src.subgroup_id}`
          : null
        const nameMatch = agg.name.match(/\[(.+?)\]\s*$/)
        const name = nameMatch ? nameMatch[1] : agg.name
        return { name, rssUrl, src, episodes: [], loading: true }
      })
      setGroupedEpisodes(initialGroups)
      setSearchState('done')  // 先切到 done，让占位 UI 立即渲染

      // Worker pool：始终保持 CONCURRENCY 个并发请求
      // 每个 worker 独立跑，谁先完成谁先渲染 UI，不等其他人
      const CONCURRENCY = 2
      let nextIndex = 0

      const worker = async () => {
        while (nextIndex < subgroupAggs.length) {
          const idx = nextIndex++
          const agg = subgroupAggs[idx]
          const src = agg.sources.find(s => s.subgroup_id)
          if (!src) continue
          try {
            const epRes = await getEpisodes(src.site, src.media_id, src.subgroup_id)
            const episodes = epRes.data.episodes.map(e => ({ ...e, _site: src.site }))
            setGroupedEpisodes(prev => {
              const updated = [...prev]
              updated[idx] = { ...updated[idx], episodes, loading: false, src }
              return updated
            })
          } catch (e) {
            console.error(`Failed to load episodes for subgroup ${src.subgroup_id}:`, e)
            setGroupedEpisodes(prev => {
              const updated = [...prev]
              updated[idx] = { ...updated[idx], loading: false }
              return updated
            })
          }
        }
      }

      // 启动 CONCURRENCY 个 worker，全部完成后继续
      await Promise.all(Array.from({ length: Math.min(CONCURRENCY, subgroupAggs.length) }, worker))


    } catch (e) {
      setSearchState('error')
      setErrorMsg(e?.response?.data?.detail || e.message)
    }
  }

  const refreshGroup = async (index) => {
    setGroupedEpisodes(prev => {
      const updated = [...prev]
      updated[index] = { ...updated[index], loading: true }
      return updated
    })
    const src = groupedEpisodes[index]?.src
    if (!src) return
    try {
      const epRes = await getEpisodes(src.site, src.media_id, src.subgroup_id)
      const episodes = epRes.data.episodes.map(e => ({ ...e, _site: src.site }))
      setGroupedEpisodes(prev => {
        const updated = [...prev]
        updated[index] = { ...updated[index], episodes, loading: false }
        return updated
      })
    } catch (e) {
      console.error(`Failed to refresh subgroup ${src.subgroup_id}:`, e)
      setGroupedEpisodes(prev => {
        const updated = [...prev]
        updated[index] = { ...updated[index], loading: false }
        return updated
      })
    }
  }

  const handleDownload = async (ep) => {
    const url = ep.magnet_url || ep.torrent_url
    if (!url) {
      onToast?.('warning', '无效下载链接', '该资源缺少真实下载地址')
      return
    }
    try {
      await addAria2Uri({ uris: [url], title: ep.title })
      onToast?.('success', '已推送到下载器', ep.title)
    } catch (e) {
      onToast?.('error', '下载失败', e.message)
    }
  }

  const handleCopy = (ep) => {
    const url = ep.magnet_url || ep.torrent_url
    if (!url) return
    navigator.clipboard.writeText(url)
      .then(() => onToast?.('success', '已复制链接', '链接已复制到剪贴板'))
      .catch((e) => onToast?.('error', '复制失败', e.message))
  }

  return (
    <div className="flex flex-col flex-1 w-full h-full min-h-0 relative">
      <div
        ref={scrollRef}
        onScroll={(e) => setShowTop(e.target.scrollTop > 300)}
        className="flex flex-col flex-1 w-full h-full min-h-0 overflow-y-auto pr-2 sm:pr-4"
        style={{ scrollbarWidth: 'thin', scrollbarColor: 'var(--color-border) transparent' }}
      >
        {/* 顶部返回及标题区域（现已加入统一滚动） */}
        <div className="flex items-center gap-4 mb-6 sm:mb-8 shrink-0">
          <button
            onClick={onBack}
            className="flex items-center justify-center size-10 rounded-full transition-all hover:scale-105 active:scale-95 z-20 shrink-0"
            style={{ background: 'var(--color-surface)', color: 'var(--color-text)', border: '1px solid var(--color-border)' }}
          >
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <polyline points="15 18 9 12 15 6" />
            </svg>
          </button>
          <div className="flex-1 min-w-0">
            <h2 className="text-xl sm:text-2xl font-bold truncate text-white">{searchKey}</h2>
            <div className="text-xs sm:text-sm text-white/50 mt-1 flex items-center gap-2 flex-wrap">
              资源检索结果
              {usedSearchKey && (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-medium bg-amber-500/15 text-amber-300/80 border border-amber-500/20">
                  <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
                  以别名「{usedSearchKey}」检索
                </span>
              )}
            </div>
          </div>
        </div>

        <div className="flex flex-col w-full">
          {searchState === 'searching' && (
            <div className="flex flex-col items-center justify-center py-12 gap-3 text-white/50">
              <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="animate-spin text-[#c8924d]">
                <line x1="12" y1="2" x2="12" y2="6" /><line x1="12" y1="18" x2="12" y2="22" /><line x1="4.93" y1="4.93" x2="7.76" y2="7.76" /><line x1="16.24" y1="16.24" x2="19.07" y2="19.07" /><line x1="2" y1="12" x2="6" y2="12" /><line x1="18" y1="12" x2="22" y2="12" /><line x1="4.93" y1="19.07" x2="7.76" y2="16.24" /><line x1="16.24" y1="7.76" x2="19.07" y2="4.93" />
              </svg>
              <div className="flex flex-col items-center gap-1">
                <span className="text-sm">
                  以「<span className="text-white/80 font-medium">{currentSearchKey || searchKey}</span>」检索中...
                </span>
                {currentSearchKey && currentSearchKey !== searchKey && (
                  <span className="text-xs text-white/30">主标题无结果，尝试别名</span>
                )}
              </div>
            </div>
          )}

          {searchState === 'error' && (
            <div className="bg-red-500/20 text-red-200 px-6 py-4 mb-6 rounded-xl border border-red-500/30 w-full mx-auto max-w-4xl">
              检索失败或无匹配资源: {errorMsg}
            </div>
          )}

          {searchState === 'done' && (
            <div className="w-full pb-8">
              <div className="mb-6 pt-2 pb-4" style={{ borderBottom: '1px solid var(--color-border)' }}>
                <h3 className="text-base font-bold mb-3" style={{ color: 'var(--color-text)' }}>
                  匹配到的下载渠道 ({groupedEpisodes.reduce((s, g) => s + g.episodes.length, 0)})
                </h3>
                {groupedEpisodes.length > 0 && (
                  <div className="flex flex-wrap gap-2 pb-1">
                    {groupedEpisodes.map(({ name, episodes }) => (
                      <button
                        key={name}
                        onClick={() => {
                          const el = document.getElementById(`fansub-${encodeURIComponent(name)}`)
                          if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' })
                        }}
                        className="px-3 py-1.5 rounded-lg text-xs font-medium transition-all border border-white/10 text-white/70 hover:text-white hover:border-white/30 hover:bg-white/10 shrink-0"
                        style={{ background: 'rgba(255,255,255,0.03)' }}
                      >
                        {name} <span className="opacity-50 ml-0.5">({episodes.length})</span>
                      </button>
                    ))}
                  </div>
                )}
              </div>

              {groupedEpisodes.length === 0 ? (
                <div className="text-center py-16 text-white/40 flex flex-col items-center gap-3">
                  <span style={{ fontSize: 64, lineHeight: 1 }}>📭</span>
                  <p>没有获取到有效的剧集种子/磁力项。</p>
                </div>
              ) : (
                <div className="flex flex-col gap-8">
                  {groupedEpisodes.map(({ name, rssUrl, episodes, loading }, index) => {
                    const isExpanded = !collapsedGroups.has(name)
                    const toggle = () => setCollapsedGroups(prev => {
                      const next = new Set(prev)
                      next.has(name) ? next.delete(name) : next.add(name)
                      return next
                    })
                    return (
                      <div key={name} id={`fansub-${encodeURIComponent(name)}`} className="flex flex-col scroll-mt-24">
                        {/* 标题行：整行可点击展开/折叠 */}
                        <button
                          onClick={toggle}
                          className="flex items-center gap-3 w-full text-left py-2 group"
                        >
                          <div className={`w-1.5 rounded-full shrink-0 bg-[linear-gradient(135deg,#e3b778,#c8924d)] transition-all duration-300 ${isExpanded ? 'h-6 shadow-[0_2px_8px_rgba(200,146,77,0.4)] opacity-100' : 'h-3 shadow-none opacity-40'}`} />
                          <h4 className="text-lg sm:text-xl font-bold text-white/95 flex-1">
                            {name}
                            {loading
                              ? <span className="text-sm font-normal text-white/30 ml-2">加载中...</span>
                              : <span className="text-sm font-medium text-white/40 ml-2">({episodes.length})</span>
                            }
                          </h4>
                          {rssUrl && (
                            <span
                              role="button"
                              onClick={e => { e.stopPropagation(); navigator.clipboard.writeText(rssUrl).then(() => onToast?.('success', '已复制 RSS 链接', rssUrl)).catch(err => onToast?.('error', '复制失败', err.message)) }}
                              title="复制该字幕组的 RSS 订阅链接"
                              className="flex items-center gap-1.5 px-2.5 py-1 rounded-lg text-xs font-medium transition-all border border-orange-400/30 text-orange-300/80 hover:text-orange-200 hover:border-orange-400/60 hover:bg-orange-400/10 shrink-0"
                            >
                              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
                                <path d="M4 11a9 9 0 0 1 9 9" /><path d="M4 4a16 16 0 0 1 16 16" /><circle cx="5" cy="19" r="1" />
                              </svg>
                              复制 RSS
                            </span>
                          )}
                          <span
                            role="button"
                            onClick={e => { e.stopPropagation(); refreshGroup(index) }}
                            title="刷新该字幕组的资源列表"
                            className="flex items-center justify-center w-6 h-6 rounded-md text-white/30 hover:text-white/70 hover:bg-white/10 transition-all shrink-0"
                          >
                            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round"
                              className={loading ? 'animate-spin' : ''}>
                              <polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/>
                              <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
                            </svg>
                          </span>
                          <svg
                            width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"
                            className={`shrink-0 text-white/30 group-hover:text-white/60 transition-all duration-200 ${isExpanded ? 'rotate-180' : ''}`}
                          >
                            <polyline points="6 9 12 15 18 9" />
                          </svg>
                        </button>
                        {/* 箭头已有 rotate-180 → 折叠时朝下，展开时朝上 */}
                        {/* 内容区：loading 始终展示骨架；否则用 grid-rows 动画平滑展开/折叠 */}
                        {loading ? (
                          <div className="flex flex-col gap-2 mt-1 pb-4">
                            {[1, 2, 3].map(i => (
                              <div key={i} className="h-16 rounded-2xl border border-white/5 bg-white/5 animate-pulse" />
                            ))}
                          </div>
                        ) : (
                          <div className={`grid transition-[grid-template-rows] duration-300 ease-in-out ${isExpanded ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]'}`}>
                            <div className="overflow-hidden">
                              <div className="flex flex-col gap-3 mt-1 pb-4">
                                {episodes.length === 0 ? (
                                  <div className="text-sm text-white/30 px-2 py-3">该字幕组暂无资源</div>
                                ) : episodes.map((ep, idx) => (
                                  <div key={idx} className="flex flex-col md:flex-row gap-4 p-5 rounded-2xl border border-white/5 bg-white/5 hover:bg-white/10 transition-colors">
                                    <div className="flex-1 min-w-0">
                                      <div className="font-semibold text-white/95 break-all leading-relaxed text-sm md:text-base mb-2">{ep.title}</div>
                                      <div className="flex flex-wrap gap-2 text-xs font-medium">
                                        <span className="rounded bg-blue-500/20 text-blue-300 px-2 py-0.5">{ep._site.toUpperCase()}</span>
                                        {ep.file_size_mb && <span className="rounded bg-white/10 text-white/60 px-2 py-0.5">{ep.file_size_mb} MB</span>}
                                        {ep.publish_time && <span className="rounded bg-white/10 text-white/60 px-2 py-0.5">{ep.publish_time}</span>}
                                      </div>
                                    </div>
                                    <div className="flex md:flex-col items-center justify-end gap-2 shrink-0">
                                      <button onClick={() => handleDownload(ep)} className="px-5 py-2 md:py-1.5 bg-[linear-gradient(135deg,#e3b778,#c8924d)] rounded-xl text-[#0A1320] font-bold text-sm w-full md:w-auto hover:opacity-90 transition-opacity">推送下载</button>
                                      <button onClick={() => handleCopy(ep)} className="px-5 py-2 md:py-1.5 bg-white/10 rounded-xl text-white font-bold text-sm w-full md:w-auto hover:bg-white/20 transition-colors">复制链接</button>
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
              )}
            </div>
          )}
        </div>
      </div>

      {showTop && (
        <button
          onClick={() => {
            scrollRef.current?.scrollTo({ top: 0, behavior: 'smooth' })
          }}
          className="absolute right-6 bottom-6 sm:right-8 sm:bottom-8 z-50 flex items-center justify-center size-12 rounded-full bg-[linear-gradient(135deg,#e3b778,#c8924d)] shadow-[0_4px_16px_rgba(200,146,77,0.4)] text-[#0A1320] hover:scale-110 active:scale-95 transition-all"
          title="回顶部"
        >
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
            <line x1="12" y1="19" x2="12" y2="5"></line>
            <polyline points="5 12 12 5 19 12"></polyline>
          </svg>
        </button>
      )}
    </div>
  )
}
