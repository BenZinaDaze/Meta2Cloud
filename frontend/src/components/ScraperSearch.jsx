import React, { useState, useEffect } from 'react'
import { tmdbSearchMulti } from '../api'
import ScraperDetailModal from './ScraperDetailModal'
import ScraperResultsView from './ScraperResultsView'
import MediaCard from './MediaCard'
import { clearResultsCache } from '../utils/resultsCache'
import { StatePanel } from './StatePanel'

// Module-level cache to preserve state across remounts
let _cachedQuery = ''
let _cachedResults = []
let _cachedSearchModeItem = null

export default function ScraperSearch({ onToast, initialSearchItem, onClearInitialSearchItem, initialQuery, onClearInitialQuery, aria2Enabled = false }) {
  const [query, setQuery] = useState(_cachedQuery)
  const [loading, setLoading] = useState(false)
  const [results, setResults] = useState(_cachedResults)
  const [error, setError] = useState(null)
  const [selectedMedia, setSelectedMedia] = useState(null)
  const [searchModeItem, setSearchModeItem] = useState(_cachedSearchModeItem)

  // Sync state to cache
  useEffect(() => {
    _cachedQuery = query
    _cachedResults = results
    _cachedSearchModeItem = searchModeItem
  }, [query, results, searchModeItem])

  useEffect(() => {
    if (initialSearchItem) {
      setSearchModeItem(initialSearchItem)
      onClearInitialSearchItem?.()
    }
  }, [initialSearchItem, onClearInitialSearchItem])

  useEffect(() => {
    if (initialQuery) {
      // Clear caches so stale results don't flash
      _cachedResults = []
      _cachedSearchModeItem = null
      _cachedQuery = initialQuery
      setSearchModeItem(null)
      setQuery(initialQuery)
      setLoading(true)
      setError(null)
      setResults([])
      tmdbSearchMulti(initialQuery)
        .then(res => setResults(res.data.results || []))
        .catch(err => setError(err?.response?.data?.detail || err.message))
        .finally(() => setLoading(false))
      onClearInitialQuery?.()
    }
  }, [initialQuery, onClearInitialQuery])

  const handleSearch = async (e) => {
    e.preventDefault()
    if (!query.trim()) return
    setLoading(true)
    setError(null)
    setResults([])
    try {
      const res = await tmdbSearchMulti(query)
      setResults(res.data.results || [])
    } catch (err) {
      setError(err?.response?.data?.detail || err.message)
    } finally {
      setLoading(false)
    }
  }

  if (searchModeItem) {
    return (
      <div className="flex flex-col flex-1 relative min-h-0">
        <section
          className="panel-surface absolute inset-0 rounded-[24px] px-4 py-4 sm:rounded-[32px] sm:px-7 sm:py-7 flex flex-col overflow-hidden"
        >
          <ScraperResultsView 
            key={searchModeItem.id || searchModeItem.tmdb_id || searchModeItem.title}
            item={searchModeItem}
            onBack={() => {
              setSelectedMedia(searchModeItem)
              setSearchModeItem(null)
            }} 
            onToast={onToast}
            aria2Enabled={aria2Enabled}
          />
        </section>
      </div>
    )
  }

  return (
    <div className="flex flex-col flex-1">
      <section
        className="page-panel panel-surface flex flex-1 flex-col rounded-[22px] px-3 py-4 sm:rounded-[32px] sm:px-7 sm:py-7"
      >
        <div className="mb-6 flex flex-col gap-3 sm:mb-7 sm:gap-5 lg:flex-row lg:items-end lg:justify-between shrink-0">
          <div className="max-w-2xl">
            <div className="mb-1 text-[10px] font-semibold uppercase tracking-[0.24em] sm:mb-2 sm:text-[11px]" style={{ color: 'var(--color-accent-hover)' }}>
              Source Detector
            </div>
            <h2 className="text-2xl font-bold text-white mb-2 sm:text-[34px]">全局资源检索</h2>
            <p className="mt-2 text-sm leading-7" style={{ color: 'var(--color-muted)' }}>
              TMDB搜索结果
            </p>
          </div>

          <form onSubmit={handleSearch} className="relative w-full max-w-[480px] shrink-0">
            <svg className="absolute left-4 top-1/2 -translate-y-1/2 text-gray-400" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="11" cy="11" r="8" /><line x1="21" y1="21" x2="16.65" y2="16.65" />
            </svg>
            <input
              type="text"
              placeholder="搜索任何电影或电视剧..."
              className="min-h-12 w-full rounded-full py-3.5 pl-12 pr-[88px] text-sm outline-none transition-all min-[430px]:pr-[100px]"
              style={{
                background: 'rgba(255,255,255,0.03)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-text)',
              }}
              value={query}
              onChange={e => setQuery(e.target.value)}
            />
            <button
              type="submit"
              disabled={loading}
              className="absolute right-1.5 top-1.5 bottom-1.5 flex min-w-[72px] items-center justify-center rounded-full bg-[linear-gradient(135deg,#e3b778,#c8924d)] px-5 text-sm font-bold text-[#0A1320] shadow-[0_2px_8px_rgba(200,146,77,0.3)] transition-all hover:opacity-90 disabled:opacity-50 min-[430px]:min-w-[84px] min-[430px]:px-8"
            >
              {loading ? '搜索中...' : '搜索'}
            </button>
          </form>
        </div>

        {error && (
          <div className="mb-6 shrink-0">
            <StatePanel
              icon="!"
              title={`搜索失败：${error}`}
              description="请检查网络连接，或者稍后重新搜索。"
              tone="danger"
              compact
            />
          </div>
        )}

        {/* Grid Area */}
        <div className="flex-1 min-h-0 overflow-y-auto" style={{ scrollbarWidth: 'thin', scrollbarColor: 'var(--color-border) transparent' }}>
          {results.length === 0 && !loading && !error && (
            <StatePanel
              icon="🔍"
              title="全局元数据探索引擎"
              description="输入电影或剧集名称，从 TMDB 定位条目后再继续检索资源。"
            />
          )}

          <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-8 gap-3 pb-8">
            {results.map((item, idx) => (
              <MediaCard
                key={idx}
                item={item}
                onClick={setSelectedMedia}
              />
            ))}
          </div>
        </div>

        {selectedMedia && (
          <ScraperDetailModal 
            item={selectedMedia} 
            onClose={() => setSelectedMedia(null)} 
            onToast={onToast} 
            onSearchResources={(item) => {
              clearResultsCache(item.title || item.original_title || item.name)
              setSearchModeItem(item)
              setSelectedMedia(null)
            }}
          />
        )}
      </section>
    </div>
  )
}
