// 搜索结果缓存（独立模块，避免与 ScraperResultsView 混合导出触发 Vite Fast Refresh 警告）
import type { SubgroupGroup } from '@/components/pages/ScraperResultsView'

const _resultsCache: Record<string, {
  searchState: 'idle' | 'searching' | 'done' | 'error'
  errorMsg: string
  groupedEpisodes: SubgroupGroup[]
}> = {}

export function clearResultsCache(key: string) {
  delete _resultsCache[key]
}

export default _resultsCache
