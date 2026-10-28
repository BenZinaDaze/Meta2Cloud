// 测试全局 setup：注入 Testing Library 的 DOM 断言扩展。
// afterEach 自动清理由 @testing-library/react 16+ 的 auto-cleanup 提供（检测到 globals 时启用；
// 本项目 globals=false，故显式注册 cleanup）。
import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

afterEach(() => {
  cleanup()
})
