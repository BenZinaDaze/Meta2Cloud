// 搜索结果缓存（独立模块，避免与 ScraperResultsView 混合导出触发 Vite Fast Refresh 警告）
const _resultsCache = {}

export function clearResultsCache(key) {
  delete _resultsCache[key]
}

export default _resultsCache
