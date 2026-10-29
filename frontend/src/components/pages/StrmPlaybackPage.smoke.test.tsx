// StrmPlaybackPage.smoke.test.tsx —— 播放记录页冒烟测试：
// 验证历史加载后统计与空态呈现（handlers 默认空记录）。
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import StrmPlaybackPage from '@/components/pages/StrmPlaybackPage'

describe('StrmPlaybackPage 冒烟', () => {
  it('无记录时展示空态与统计卡', async () => {
    render(
      <MemoryRouter>
        <StrmPlaybackPage />
      </MemoryRouter>,
    )
    expect(await screen.findByText(/暂无播放记录/)).toBeInTheDocument()
    expect(await screen.findByText('今日播放')).toBeInTheDocument()
  })
})
