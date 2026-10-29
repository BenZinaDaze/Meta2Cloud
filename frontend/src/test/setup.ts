// 测试全局 setup：注入 Testing Library 的 DOM 断言扩展；挂接 MSW 拦截服务器。
// afterEach 自动清理由 @testing-library/react 16+ 的 auto-cleanup 提供（检测到 globals 时启用；
// 本项目 globals=false，故显式注册 cleanup 与 handler 复位）。
import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach, beforeAll, afterAll } from 'vitest'
import { server } from './mocks/server'

// jsdom 未实现 ResizeObserver：Dashboard 等页面的尺寸观测组件需要
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= ResizeObserverStub as unknown as typeof ResizeObserver

// MSW v3 默认即对未匹配请求告警（onUnhandledFrame 默认 warn），无需显式配置
beforeAll(() => server.listen())
afterEach(() => {
  cleanup()
  server.resetHandlers()
  localStorage.clear()
})
afterAll(() => server.close())
