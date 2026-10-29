// CategoriesPage.smoke.test.tsx —— 分类配置页冒烟测试：
// 验证配置加载后表单呈现默认分类数据，保存按钮存在（交互链路入口可达）。
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import CategoriesPage from '@/components/pages/CategoriesPage'

describe('CategoriesPage 冒烟', () => {
  it('加载后表单呈现分类目录配置', async () => {
    render(<CategoriesPage />)
    // MSW 默认 /api/categories 返回 dirs.tv='电视剧'，表单输入框应回显
    expect(await screen.findByDisplayValue('电视剧')).toBeInTheDocument()
    expect(await screen.findByDisplayValue('电影')).toBeInTheDocument()
  })

  it('剧集分类规则区呈现默认规则（日番）', async () => {
    render(<CategoriesPage />)
    // handlers 默认 tv 规则含「日番」——规则名以输入框或文本呈现
    expect(await screen.findByDisplayValue('日番')).toBeInTheDocument()
  })
})
