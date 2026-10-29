// usePolling.test.ts —— 声明式轮询 hook：节奏/启停/可见性门控回归测试。
// 此前六个页面各自手写 setInterval + 门控，收敛后在此锁定行为。
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { usePolling } from '@/hooks/usePolling'

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('usePolling', () => {
  it('按间隔重复执行，首次不立即执行', () => {
    const fn = vi.fn()
    renderHook(() => usePolling(fn, 3000))
    expect(fn).not.toHaveBeenCalled() // 初始加载由调用方负责
    act(() => {
      vi.advanceTimersByTime(9000)
    })
    expect(fn).toHaveBeenCalledTimes(3)
  })

  it('enabled=false 暂停，恢复后继续', () => {
    const fn = vi.fn()
    const { rerender } = renderHook(({ enabled }) => usePolling(fn, 1000, { enabled }), {
      initialProps: { enabled: false },
    })
    act(() => {
      vi.advanceTimersByTime(5000)
    })
    expect(fn).not.toHaveBeenCalled()
    rerender({ enabled: true })
    act(() => {
      vi.advanceTimersByTime(2000)
    })
    expect(fn).toHaveBeenCalledTimes(2)
  })

  it('intervalMs 变化按新节奏执行（自适应轮询）', () => {
    const fn = vi.fn()
    const { rerender } = renderHook(({ ms }) => usePolling(fn, ms), { initialProps: { ms: 30000 } })
    act(() => {
      vi.advanceTimersByTime(30000)
    })
    expect(fn).toHaveBeenCalledTimes(1)
    rerender({ ms: 3000 }) // 任务开始：加密到 3s
    act(() => {
      vi.advanceTimersByTime(6000)
    })
    expect(fn).toHaveBeenCalledTimes(3)
  })

  it('whenVisible=true 时后台标签页跳过本轮', () => {
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
    const fn = vi.fn()
    renderHook(() => usePolling(fn, 1000, { whenVisible: true }))
    act(() => {
      vi.advanceTimersByTime(3000)
    })
    expect(fn).not.toHaveBeenCalled()
  })

  it('始终执行最新的 fn（回调身份数变化不重建定时器）', () => {
    const seen: number[] = []
    const { rerender } = renderHook(({ v }) => usePolling(() => seen.push(v), 1000), {
      initialProps: { v: 0 },
    })
    rerender({ v: 1 })
    act(() => {
      vi.advanceTimersByTime(1000)
    })
    expect(seen).toEqual([1])
  })
})
