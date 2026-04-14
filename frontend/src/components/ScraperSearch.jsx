import React, { useState, useEffect } from 'react'
import { tmdbSearchMulti } from '../api'
import ScraperDetailModal from './ScraperDetailModal'
import ScraperResultsView, { clearResultsCache } from './ScraperResultsView'
import MediaCard from './MediaCard'

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
        className="panel-surface rounded-[24px] px-4 py-4 sm:rounded-[32px] sm:px-7 sm:py-7 flex-1 flex flex-col"
        style={{ minHeight: 'calc(100dvh - 128px)' }}
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
              className="w-full rounded-full py-3.5 pl-12 pr-[100px] text-sm outline-none transition-all"
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
              className="absolute right-1.5 top-1.5 bottom-1.5 bg-[linear-gradient(135deg,#e3b778,#c8924d)] shadow-[0_2px_8px_rgba(200,146,77,0.3)] text-[#0A1320] px-8 rounded-full font-bold text-sm hover:opacity-90 disabled:opacity-50 flex items-center justify-center transition-all"
            >
              {loading ? '搜索中...' : '搜索'}
            </button>
          </form>
        </div>

        {error && (
          <div className="bg-red-500/10 border border-red-500/20 text-red-100 rounded-2xl p-4 mb-6 shrink-0 text-sm">
            搜索失败: {error}
          </div>
        )}

        {/* Grid Area */}
        <div className="flex-1 min-h-0 overflow-y-auto" style={{ scrollbarWidth: 'thin', scrollbarColor: 'var(--color-border) transparent' }}>
          {results.length === 0 && !loading && !error && (
            <div className="flex flex-col items-center justify-center py-32 gap-4">
              <span style={{ fontSize: 100, lineHeight: 1 }}>🔍</span>
              <p className="text-xl font-medium" style={{ color: 'var(--color-muted)' }}>全局元数据探索引擎</p>
              <p className="text-sm text-center" style={{ color: 'var(--color-muted)', opacity: 0.6 }}>基于 TMDB 定位内容，智能匹配所有支持的源探测器。</p>
            </div>
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
