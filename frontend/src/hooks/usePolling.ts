import { useEffect, useRef } from 'react'

interface UsePollingOptions {
  /** false 时暂停轮询（回调与间隔保持不变，恢复后按既有节奏继续） */
  enabled?: boolean
  /** 页面不可见（后台标签页）时跳过本轮请求 */
  whenVisible?: boolean
}

/**
 * usePolling 声明式轮询：每 intervalMs 执行一次 fn。此前六个页面各自手写
 * setInterval/clearInterval + 门控，节奏、取消与可见性处理各异，收敛为同一实现：
 * - 首次不立即执行——初始加载由调用方自行处理，本 hook 只负责节奏；
 * - intervalMs 变化（「运行中 3s / 空闲 30s」自适应）自动按新节奏重建；
 * - enabled=false 暂停（运行态门控、busy 门控）；
 * - fn 经 ref 间接调用：回调身份数变化不重建定时器；
 * - 卸载自动清理。
 * 边沿检测类逻辑（如「运行结束自动刷新」）写在 fn 内部。
 */
export function usePolling(
  fn: () => void,
  intervalMs: number,
  { enabled = true, whenVisible = false }: UsePollingOptions = {},
) {
  // ref 经 effect 同步（react-hooks/refs 禁止渲染期写入）；
  // 定时器回调只会异步触发，同步时机始终先于下一次 tick
  const fnRef = useRef(fn)
  const whenVisibleRef = useRef(whenVisible)
  useEffect(() => {
    fnRef.current = fn
    whenVisibleRef.current = whenVisible
  })

  useEffect(() => {
    if (!enabled) return undefined
    const timer = setInterval(() => {
      if (whenVisibleRef.current && document.visibilityState !== 'visible') return
      fnRef.current()
    }, intervalMs)
    return () => clearInterval(timer)
  }, [intervalMs, enabled, whenVisible])
}
