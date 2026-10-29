// WatchedSyncPage.smoke.test.tsx —— 观看同步页冒烟测试：
// 验证状态加载后触发按钮、同步记录/忽略列表两个分区与空态呈现。
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import WatchedSyncPage from '@/components/pages/WatchedSyncPage'

function renderPage() {
  return render(
    <MemoryRouter>
      <WatchedSyncPage />
    </MemoryRouter>,
  )
}

describe('WatchedSyncPage 冒烟', () => {
  it('加载后呈现同步状态与触发入口', async () => {
    renderPage()
    expect(await screen.findByText('立即同步')).toBeInTheDocument()
    expect(await screen.findByText('同步记录')).toBeInTheDocument()
    expect(await screen.findByText('忽略列表')).toBeInTheDocument()
  })

  it('无同步记录时展示配对引导空态', async () => {
    renderPage()
    expect(
      await screen.findByText(
        /在「配置 → 观看同步」完成 Trakt 与 Plex 配对后/,
      ),
    ).toBeInTheDocument()
  })
})
