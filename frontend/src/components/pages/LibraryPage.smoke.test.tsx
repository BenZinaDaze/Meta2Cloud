// LibraryPage.smoke.test.tsx —— 媒体库页冒烟测试：
// 空库快照下页面正常渲染（标题/筛选可达），为 library_media 数据层改造护航。
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import LibraryPage from '@/components/pages/LibraryPage'

describe('LibraryPage 冒烟', () => {
  it('空库快照下正常呈现页头与筛选', async () => {
    render(
      <MemoryRouter>
        <LibraryPage filter="movies" />
      </MemoryRouter>,
    )
    expect(await screen.findByText('电影')).toBeInTheDocument()
  })
})
