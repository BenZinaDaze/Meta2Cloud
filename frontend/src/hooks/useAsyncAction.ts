// useAsyncAction —— 配置页「busy + 提示消息」状态对的通用封装。
// 收敛 8 组重复的 setXxxBusy(true) / try / catch / finally 样板：
// run(action, onFail) 自动管理 busy 与 message；action 返回字符串即成功提示，
// 抛出异常即失败提示（取 axios response.data.detail，无则用 onFail 文案）。
import { useCallback, useState } from 'react'

export type ActionMessage = { type: 'success' | 'error'; text: string } | null

// apiDetail 提取后端返回的 detail 文案（无则空串）。
export function apiDetail(e: unknown): string {
  return (e as { response?: { data?: { detail?: string } } })?.response?.data?.detail || ''
}

export function useAsyncAction() {
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState<ActionMessage>(null)

  const run = useCallback(
    async (action: () => Promise<string | void>, onFail: string | ((e: unknown) => string)) => {
      setBusy(true)
      setMessage(null)
      try {
        const ok = await action()
        if (typeof ok === 'string') {
          setMessage({ type: 'success', text: ok })
        }
      } catch (e) {
        setMessage({ type: 'error', text: typeof onFail === 'function' ? onFail(e) : apiDetail(e) || onFail })
      } finally {
        setBusy(false)
      }
    },
    [],
  )

  return { busy, message, setMessage, run }
}
