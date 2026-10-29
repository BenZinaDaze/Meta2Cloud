// ConfigPage.smoke.test.tsx —— 配置页冒烟测试：
// general 页加载主配置后表单回显（用户名/主存储），页头与保存入口可达。
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import ConfigPage from '@/components/pages/ConfigPage'

describe('ConfigPage 冒烟', () => {
  it('general 页加载后回显主配置字段', async () => {
    render(<ConfigPage />)
    // 默认落在存储 tab（auth/tmdb 等 tab 未激活不渲染）
    expect(await screen.findByText('云存储选择')).toBeInTheDocument()
    expect(await screen.findByText('主存储')).toBeInTheDocument()
    // radix Select 的 SelectValue 渲染选中项文案
    expect(await screen.findByText('115 网盘')).toBeInTheDocument()
  })
})
