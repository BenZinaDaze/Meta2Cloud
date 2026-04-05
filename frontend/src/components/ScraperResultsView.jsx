import React, { useState, useEffect, useRef } from 'react'
import { searchMedia, getEpisodes, addAria2Uri } from '../api'

let _resultsCache = {}
export function clearResultsCache(key) { delete _resultsCache[key] }

export default function ScraperResultsView({ item, onBack, onToast }) {
  const searchKey = item.title || item.original_title || item.name

  const [searchState, setSearchState] = useState(() => _resultsCache[searchKey] ? _resultsCache[searchKey].searchState : 'idle')
  const [errorMsg, setErrorMsg] = useState(() => _resultsCache[searchKey] ? _resultsCache[searchKey].errorMsg : '')
  const [episodesList, setEpisodesList] = useState(() => _resultsCache[searchKey] ? _resultsCache[searchKey].episodesList : [])
  const [showTop, setShowTop] = useState(false)
  const scrollRef = useRef(null)


  const groupedEpisodes = React.useMemo(() => {
    const groups = {}
    episodesList.forEach(ep => {
      const match = ep.title.match(/^\[(.*?)\]|^【(.*?)】/)
      const fansub = match ? (match[1] || match[2]) : '其他/未知'
      if (!groups[fansub]) groups[fansub] = []
      groups[fansub].push(ep)
    })

    // Sort episodes within each group by time descending
    Object.values(groups).forEach(group => {
      group.sort((a, b) => new Date(b.publish_time).getTime() - new Date(a.publish_time).getTime())
    })

    // Sort groups themselves by number of episodes in descending order
    return Object.entries(groups).sort((a, b) => b[1].length - a[1].length)
  }, [episodesList])

  // 同步状态到缓存（用于切页面后返回时恢复）
  useEffect(() => {
    _resultsCache[searchKey] = { searchState, errorMsg, episodesList }
  }, [searchKey, searchState, errorMsg, episodesList])

  useEffect(() => {
    if (searchState === 'idle') {
      startSearch()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const startSearch = async () => {
    if (!item) return
    setSearchState('searching')
    setErrorMsg('')
    setEpisodesList([])
    try {
      const key = item.title || item.original_title || item.name
      const res = await searchMedia(key)
      const aggregates = res.data.results || []

      if (aggregates.length === 0) {
        setSearchState('error')
        setErrorMsg('各大爬虫站点均未匹配到此资源。')
        return
      }

      const bestMatch = aggregates[0]
      let allEps = []
      for (const src of bestMatch.sources) {
        try {
          const epRes = await getEpisodes(src.site, src.media_id, src.subgroup_id)
          const eps = epRes.data.episodes.map(e => ({ ...e, _site: src.site }))
          allEps = allEps.concat(eps)
        } catch (e) {
          console.error(`Failed to load episodes for ${src.site}:`, e)
        }
      }

      // 根据 magnet_url 或 torrent_url 去重，解决多次抓取同一资源的显示冗余
      const uniqueTags = new Set()
      const uniqueEps = []
      for (const ep of allEps) {
        const tag = ep.magnet_url || ep.torrent_url || ep.title
        if (!uniqueTags.has(tag)) {
          uniqueTags.add(tag)
          uniqueEps.push(ep)
        }
      }

      setEpisodesList(uniqueEps)
      setSearchState('done')
    } catch (e) {
      setSearchState('error')
      setErrorMsg(e?.response?.data?.detail || e.message)
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
            <div className="text-xs sm:text-sm text-white/50 mt-1">资源检索结果</div>
          </div>
        </div>

        <div className="flex flex-col w-full">
          {searchState === 'searching' && (
            <div className="flex flex-col items-center justify-center py-12 gap-4 text-white/50">
              <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="animate-spin text-[#c8924d]">
                <line x1="12" y1="2" x2="12" y2="6" /><line x1="12" y1="18" x2="12" y2="22" /><line x1="4.93" y1="4.93" x2="7.76" y2="7.76" /><line x1="16.24" y1="16.24" x2="19.07" y2="19.07" /><line x1="2" y1="12" x2="6" y2="12" /><line x1="18" y1="12" x2="22" y2="12" /><line x1="4.93" y1="19.07" x2="7.76" y2="16.24" /><line x1="16.24" y1="7.76" x2="19.07" y2="4.93" />
              </svg>
              资源检索中...
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
                  匹配到的下载渠道 ({episodesList.length})
                </h3>
                {episodesList.length > 0 && (
                  <div className="flex flex-wrap gap-2 pb-1">
                    {groupedEpisodes.map(([fansub, eps]) => (
                      <button
                        key={fansub}
                        onClick={() => {
                          const safeId = `fansub-${encodeURIComponent(fansub)}`
                          const el = document.getElementById(safeId)
                          if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' })
                        }}
                        className="px-3 py-1.5 rounded-lg text-xs font-medium transition-all border border-white/10 text-white/70 hover:text-white hover:border-white/30 hover:bg-white/10 shrink-0"
                        style={{ background: 'rgba(255,255,255,0.03)' }}
                      >
                        {fansub} <span className="opacity-50 ml-0.5">({eps.length})</span>
                      </button>
                    ))}
                  </div>
                )}
              </div>

              {episodesList.length === 0 ? (
                <div className="text-center py-16 text-white/40 flex flex-col items-center gap-3">
                  <span style={{ fontSize: 64, lineHeight: 1 }}>📭</span>
                  <p>没有获取到有效的剧集种子/磁力项。</p>
                </div>
              ) : (
                <div className="flex flex-col gap-8">
                  {groupedEpisodes.map(([fansub, eps]) => (
                    <div key={fansub} id={`fansub-${encodeURIComponent(fansub)}`} className="flex flex-col gap-3 scroll-mt-24">
                      <div className="flex items-center gap-3 mb-2">
                        <div className="w-1.5 h-6 bg-[linear-gradient(135deg,#e3b778,#c8924d)] rounded-full shadow-[0_2px_8px_rgba(200,146,77,0.3)]"></div>
                        <h4 className="text-lg sm:text-xl font-bold text-white/95">{fansub} <span className="text-sm font-medium text-white/40 ml-2">({eps.length})</span></h4>
                      </div>
                      {eps.map((ep, idx) => (
                        <div key={idx} className="flex flex-col md:flex-row gap-4 p-5 rounded-2xl border border-white/5 bg-white/5 hover:bg-white/10 transition-colors">
                          <div className="flex-1 min-w-0">
                            <div className="font-semibold text-white/95 break-all leading-relaxed text-sm md:text-base mb-2">{ep.title}</div>
                            <div className="flex flex-wrap gap-2 text-xs font-medium">
                              <span className="rounded bg-blue-500/20 text-blue-300 px-2 py-0.5">{ep._site.toUpperCase()}</span>
                              {ep.file_size_mb && (
                                <span className="rounded bg-white/10 text-white/60 px-2 py-0.5">{ep.file_size_mb} MB</span>
                              )}
                              {ep.publish_time && (
                                <span className="rounded bg-white/10 text-white/60 px-2 py-0.5">{ep.publish_time}</span>
                              )}
                            </div>
                          </div>
                          <div className="flex md:flex-col items-center justify-end gap-2 shrink-0">
                            <button onClick={() => handleDownload(ep)} className="px-5 py-2 md:py-1.5 bg-[linear-gradient(135deg,#e3b778,#c8924d)] rounded-xl text-[#0A1320] font-bold text-sm w-full md:w-auto hover:opacity-90 transition-opacity">推送下载</button>
                            <button onClick={() => handleCopy(ep)} className="px-5 py-2 md:py-1.5 bg-white/10 rounded-xl text-white font-bold text-sm w-full md:w-auto hover:bg-white/20 transition-colors">复制链接</button>
                          </div>
                        </div>
                      ))}
                    </div>
                  ))}
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
