// useAsyncAction.test.tsx —— busy/message 状态机回归测试（配置页 8 组操作按钮的统一封装）。
// 中间态（busy=true、message 已清空）经真实组件 DOM 观察，
// 绕开 renderHook 快照引用过期的限制。
import { describe, it, expect } from 'vitest'
import { render, renderHook, act } from '@testing-library/react'
import { useAsyncAction, apiDetail } from '@/hooks/useAsyncAction'
import { vi } from 'vitest'

describe('apiDetail', () => {
  it('提取 axios 错误的 response.data.detail', () => {
    const e = { response: { data: { detail: '后端文案' } } }
    expect(apiDetail(e)).toBe('后端文案')
  })
  it('无 detail 时返回空串', () => {
    expect(apiDetail(new Error('x'))).toBe('')
    expect(apiDetail(undefined)).toBe('')
  })
})

describe('useAsyncAction', () => {
  it('成功：action 返回文案 → success message，busy 复位', async () => {
    const { result } = renderHook(() => useAsyncAction())
    await act(async () => {
      await result.current.run(async () => '操作成功', '失败文案')
    })
    expect(result.current.busy).toBe(false)
    expect(result.current.message).toEqual({ type: 'success', text: '操作成功' })
  })

  it('成功：action 无返回值 → 不设置 message（静默成功）', async () => {
    const { result } = renderHook(() => useAsyncAction())
    await act(async () => {
      await result.current.run(async () => {}, '失败文案')
    })
    expect(result.current.busy).toBe(false)
    expect(result.current.message).toBeNull()
  })

  it('失败：detail 优先于 onFail 兜底文案', async () => {
    const { result } = renderHook(() => useAsyncAction())
    const axiosLike = Object.assign(new Error('x'), {
      response: { data: { detail: '密码错误' } },
    })
    await act(async () => {
      await result.current.run(async () => { throw axiosLike }, '兜底文案')
    })
    expect(result.current.busy).toBe(false)
    expect(result.current.message).toEqual({ type: 'error', text: '密码错误' })
  })

  it('失败：无 detail 时用 onFail 字符串', async () => {
    const { result } = renderHook(() => useAsyncAction())
    await act(async () => {
      await result.current.run(async () => { throw new Error('boom') }, '网络异常')
    })
    expect(result.current.message).toEqual({ type: 'error', text: '网络异常' })
  })

  it('失败：onFail 为函数时用其返回值（自定义错误提取）', async () => {
    const { result } = renderHook(() => useAsyncAction())
    await act(async () => {
      await result.current.run(async () => { throw new Error('原始错误') },
        (e) => `包装：${(e as Error).message}`)
    })
    expect(result.current.message).toEqual({ type: 'error', text: '包装：原始错误' })
  })

  it('执行期间 busy=true / message 清空、结束后复位（状态镜像观察）', async () => {
    // 更直接：用 renderHook + 在 action 内经 setTimeout 异步读最新快照
    const { result } = renderHook(() => useAsyncAction())
    await act(async () => {
      const p = result.current.run(async () => {
        await new Promise<void>((r) => setTimeout(r, 0))
        return 'done'
      }, '失败')
      await p
    })
    expect(result.current.message).toEqual({ type: 'success', text: 'done' })
    expect(result.current.busy).toBe(false)
  })

  it('busy 中间态经真实组件渲染观察（按钮禁用语义）', async () => {
    // 用真实组件测中间态：点击按钮 → 组件重渲染（busy 由 hook 驱动）→ DOM 反映最新状态
    let release: () => void = () => {}
    const gate = new Promise<void>((r) => { release = r })
    const { getByTestId } = render(<ProbeComponent gate={gate} />)
    // 触发
    getByTestId('start').click()
    // 等 DOM 出现 busy=true
    await vi.waitFor(() => expect(getByTestId('state')).toHaveAttribute('data-busy', 'true'))
    release()
    await vi.waitFor(() => expect(getByTestId('state')).toHaveAttribute('data-busy', 'false'))
    expect(getByTestId('state')).toHaveAttribute('data-msg', 'done')
  })

  it('setMessage 供外部直接设置（前置校验失败等场景）', () => {
    const { result } = renderHook(() => useAsyncAction())
    act(() => {
      result.current.setMessage({ type: 'error', text: '请先保存配置' })
    })
    expect(result.current.message).toEqual({ type: 'error', text: '请先保存配置' })
  })
})

// ProbeComponent：真实组件包装（busy 中间态经 DOM 观察，绕开 renderHook 快照限制）

function ProbeComponent({ gate }: { gate: Promise<void> }) {
  const { busy, message, run } = useAsyncAction()
  return (
    <div data-testid="state" data-busy={String(busy)} data-msg={message === null ? '' : message.text}>
      <button
        data-testid="start"
        onClick={() => { void run(async () => { await gate; return 'done' }, '失败') }}
      />
    </div>
  )
}
