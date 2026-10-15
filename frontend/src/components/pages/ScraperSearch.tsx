import { useState, useEffect } from 'react'
import { Search } from 'lucide-react'
import { tmdbSearchMulti } from '@/api'
import MediaCard from '@/components/MediaCard'
import { StatePanel } from '@/components/StatePanel'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import PageHeader from '@/components/shared/PageHeader'
import ScraperDetailModal from '@/components/modals/ScraperDetailModal'
import ScraperResultsView from '@/components/pages/ScraperResultsView'
import { clearResultsCache } from '@/utils/resultsCache'
import type { MediaItem } from '@/types/api'

let _cachedQuery = ''
let _cachedResults: MediaItem[] = []
let _cachedSearchModeItem: MediaItem | null = null

interface ScraperSearchProps {
  initialSearchItem?: MediaItem | null
  onClearInitialSearchItem?: () => void
  initialQuery?: string
  onClearInitialQuery?: () => void
  aria2Enabled?: boolean
}

export default function ScraperSearch({
  initialSearchItem,
  onClearInitialSearchItem,
  initialQuery,
  onClearInitialQuery,
  aria2Enabled = false,
}: ScraperSearchProps) {
  const [query, setQuery] = useState(_cachedQuery)
  const [loading, setLoading] = useState(false)
  const [results, setResults] = useState<MediaItem[]>(_cachedResults)
  const [error, setError] = useState<string | null>(null)
  const [selectedMedia, setSelectedMedia] = useState<MediaItem | null>(null)
  const [searchModeItem, setSearchModeItem] = useState<MediaItem | null>(_cachedSearchModeItem)

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
      _cachedResults = []
      _cachedSearchModeItem = null
      _cachedQuery = initialQuery
      setSearchModeItem(null)
      setQuery(initialQuery)
      setLoading(true)
      setError(null)
      setResults([])
      tmdbSearchMulti(initialQuery)
        .then((res) => setResults(res.data.results || []))
        .catch((err) => setError(err?.response?.data?.detail || err.message))
        .finally(() => setLoading(false))
      onClearInitialQuery?.()
    }
  }, [initialQuery, onClearInitialQuery])

  const handleSearch = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!query.trim()) return
    setLoading(true)
    setError(null)
    setResults([])
    try {
      const res = await tmdbSearchMulti(query)
      setResults(res.data.results || [])
    } catch (err) {
      setError((err as { response?: { data?: { detail?: string } } })?.response?.data?.detail || '搜索失败')
    } finally {
      setLoading(false)
    }
  }

  if (searchModeItem) {
    return (
      <ScraperResultsView
        item={searchModeItem}
        onBack={() => setSearchModeItem(null)}
        aria2Enabled={aria2Enabled}
      />
    )
  }

  return (
    <div className="flex flex-1 flex-col gap-5">
      <PageHeader
        kicker="Source Detector"
        title="全局资源检索"
        description="从 TMDB 定位条目，再聚合各大站点的字幕组资源。"
        actions={
          <form onSubmit={handleSearch} className="relative w-full max-w-[480px]">
            <Search className="pointer-events-none absolute left-4 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              type="text"
              placeholder="搜索任何电影或电视剧..."
              className="h-11 rounded-full pl-11 pr-24"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
            <Button
              type="submit"
              disabled={loading}
              className="absolute bottom-1.5 right-1.5 top-1.5 h-auto min-w-[72px] rounded-full"
            >
              {loading ? '搜索中...' : '搜索'}
            </Button>
          </form>
        }
      />

      {error && (
        <StatePanel
          icon="!"
          title={`搜索失败：${error}`}
          description="请检查网络连接，或者稍后重新搜索。"
          tone="danger"
          compact
        />
      )}

      <div className="min-h-0 flex-1 overflow-y-auto">
        {results.length === 0 && !loading && !error && (
          <StatePanel
            icon={<Search className="size-6" />}
            title="全局元数据探索引擎"
            description="输入电影或剧集名称，从 TMDB 定位条目后再继续检索资源。"
          />
        )}

        {loading && results.length === 0 && (
          <div className="grid grid-cols-2 gap-3 pb-8 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-8">
            {Array.from({ length: 12 }, (_, i) => (
              <div key={i} className="animate-pulse">
                <div className="aspect-[2/3] rounded-xl bg-muted/70" />
                <div className="mt-2 h-3 w-3/4 rounded bg-muted/50" />
              </div>
            ))}
          </div>
        )}

        <div className="grid grid-cols-2 gap-3 pb-8 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-8">
          {results.map((item, idx) => (
            <div
              key={idx}
              className="animate-stagger-in"
              style={{ animationDelay: `${Math.min(idx * 25, 360)}ms` }}
            >
              <MediaCard item={item} onClick={setSelectedMedia} />
            </div>
          ))}
        </div>
      </div>

      {selectedMedia && (
        <ScraperDetailModal
          item={selectedMedia}
          open={!!selectedMedia}
          onOpenChange={(open) => !open && setSelectedMedia(null)}
          onSearchResources={(item) => {
            clearResultsCache(`${item.tmdb_id || ''}:${item.title || item.original_title || ''}`)
            setSearchModeItem(item)
            setSelectedMedia(null)
          }}
        />
      )}
    </div>
  )
}
