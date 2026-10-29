// StrmSyncPage.smoke.test.tsx —— STRM 同步页冒烟测试：
// 验证状态加载后触发入口与分区呈现（状态经 MSW 默认 handlers）。
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import StrmSyncPage from '@/components/pages/StrmSyncPage'

describe('StrmSyncPage 冒烟', () => {
  it('加载后呈现同步状态与触发入口', async () => {
    render(
      <MemoryRouter>
        <StrmSyncPage />
      </MemoryRouter>,
    )
    expect(await screen.findByText('立即同步')).toBeInTheDocument()
  })
})
