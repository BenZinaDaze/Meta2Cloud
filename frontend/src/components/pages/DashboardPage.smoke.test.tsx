// DashboardPage.smoke.test.tsx —— 仪表盘页面冒烟测试：
// 一次聚合 9 个端点，验证加载后统计卡与分区标题正常呈现（数据经 MSW 默认 handlers）。
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import DashboardPage from '@/components/pages/DashboardPage'

function renderPage() {
  return render(
    <MemoryRouter>
      <DashboardPage />
    </MemoryRouter>,
  )
}

describe('DashboardPage 冒烟', () => {
  it('加载后呈现统计卡数据与分区标题', async () => {
    renderPage()
    // 统计卡：MSW 默认 /api/stats total_movies=3、在库集数 20
    expect(await screen.findByText('媒体库在库电影')).toBeInTheDocument()
    expect(await screen.findByText('已入库 20 集')).toBeInTheDocument()
    // 分区标题
    expect(await screen.findByText('入库趋势')).toBeInTheDocument()
    expect(await screen.findByText('最近入库')).toBeInTheDocument()
  })

  it('近 7 天无记录时展示空态而非报错', async () => {
    // handlers 默认 /api/ingest/stats total=5，但分布端点未 mock 空记录场景——
    // 这里覆盖为全零验证空态分支
    const { server } = await import('@/test/mocks/server')
    const { http, HttpResponse } = await import('msw')
    server.use(
      http.get('/api/ingest/stats', () =>
        HttpResponse.json({ days: 7, total: 0, movies: 0, tv_episodes: 0, success: 0, failed: 0, no_tmdb: 0, deleted: 0 }),
      ),
    )
    renderPage()
    expect(await screen.findByText('近 7 天暂无入库记录')).toBeInTheDocument()
  })
})
