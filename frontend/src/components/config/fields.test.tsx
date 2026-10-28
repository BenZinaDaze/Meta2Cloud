// fields.test.tsx —— 配置页表单原语交互回归测试。
// 重点锁定 NumberInput 的「编辑期本地草稿、失焦/回车才钳位」行为（防 4→15→155 键入断裂），
// 该行为曾因边输边钳而破坏 45/100 这类含下限以下前缀的数值输入。
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { NumberInput, TextInput, ListField, Toggle } from '@/components/config/fields'

describe('NumberInput', () => {
  it('失焦时钳位到 [min, max]', async () => {
    const onChange = vi.fn()
    render(<NumberInput value={10} onChange={onChange} min={1} max={10} />)
    const input = screen.getByRole('spinbutton')
    await userEvent.clear(input)
    await userEvent.type(input, '99')
    await userEvent.tab()
    expect(onChange).toHaveBeenCalledWith(10)
  })

  it('编辑期不提交（草稿态）——「4」不被立刻钳成 1', async () => {
    const onChange = vi.fn()
    render(<NumberInput value={10} onChange={onChange} min={5} max={100} />)
    const input = screen.getByRole('spinbutton')
    // 输入 4（小于 min）：若边输边钳会被改成 5，后续拼成 54
    await userEvent.clear(input)
    await userEvent.type(input, '4')
    expect(onChange).not.toHaveBeenCalled()
    // 补成 45 后失焦 → 正常提交
    await userEvent.type(input, '5')
    await userEvent.tab()
    expect(onChange).toHaveBeenCalledWith(45)
  })

  it('回车提交并钳位', async () => {
    const onChange = vi.fn()
    render(<NumberInput value={1} onChange={onChange} min={1} max={10} />)
    const input = screen.getByRole('spinbutton')
    await userEvent.clear(input)
    await userEvent.type(input, '50{Enter}')
    expect(onChange).toHaveBeenCalledWith(10)
  })

  it('失焦时空值提交 undefined（交由保存时取默认值）', async () => {
    const onChange = vi.fn()
    render(<NumberInput value={5} onChange={onChange} min={1} />)
    const input = screen.getByRole('spinbutton')
    await userEvent.clear(input)
    await userEvent.tab()
    expect(onChange).toHaveBeenCalledWith(undefined)
  })

  it('number input 丢弃字母输入，失焦空草稿提交 undefined', async () => {
    const onChange = vi.fn()
    render(<NumberInput value={7} onChange={onChange} />)
    const input = screen.getByRole('spinbutton')
    // 浏览器对 type=number 的字母键入不改变值（无 onChange 触发）
    await userEvent.type(input, 'abc')
    expect(onChange).not.toHaveBeenCalled()
    // 清空后失焦 → 提交 undefined（交由保存时取默认值）
    await userEvent.clear(input)
    await userEvent.tab()
    expect(onChange).toHaveBeenCalledWith(undefined)
  })
})

describe('TextInput', () => {
  it('type=password 时输入显示为圆点，点小眼睛切换明文', async () => {
    render(
      <TextInput value="secret-token" onChange={() => {}} type="password" />,
    )
    const input = screen.getByDisplayValue('secret-token')
    expect(input).toHaveAttribute('type', 'password')
    // 小眼睛按钮（aria-label 缺失，用 svg 定位：lucide Eye 图标按钮是唯一按钮）
    const eye = input.parentElement!.querySelector('button')!
    await userEvent.click(eye)
    expect(input).toHaveAttribute('type', 'text')
    await userEvent.click(eye)
    expect(input).toHaveAttribute('type', 'password')
  })

  it('type=text 时不渲染小眼睛', () => {
    render(<TextInput value="plain" onChange={() => {}} />)
    expect(screen.getByDisplayValue('plain').parentElement!.querySelector('button')).toBeNull()
  })

  it('输入触发 onChange', async () => {
    const onChange = vi.fn()
    render(<TextInput value="" onChange={onChange} />)
    await userEvent.type(screen.getByRole('textbox'), 'x')
    expect(onChange).toHaveBeenCalledWith('x')
  })
})

describe('ListField', () => {
  it('输入回车添加条目并清空草稿', async () => {
    const onChange = vi.fn()
    render(<ListField value={[]} onChange={onChange} />)
    const input = screen.getByPlaceholderText('输入后按 Enter 或点击添加')
    await userEvent.type(input, '新词{Enter}')
    expect(onChange).toHaveBeenCalledWith(['新词'])
  })

  it('重复条目不添加', async () => {
    const onChange = vi.fn()
    render(<ListField value={['已有']} onChange={onChange} />)
    const input = screen.getByPlaceholderText('输入后按 Enter 或点击添加')
    await userEvent.type(input, '已有{Enter}')
    expect(onChange).not.toHaveBeenCalled()
  })

  it('点击删除按钮移除对应条目', async () => {
    const onChange = vi.fn()
    render(<ListField value={['a', 'b', 'c']} onChange={onChange} />)
    // 每个条目一个删除按钮
    const buttons = screen.getAllByRole('button').filter((b) => b.querySelector('svg'))
    await userEvent.click(buttons[1])
    expect(onChange).toHaveBeenCalledWith(['a', 'c'])
  })
})

describe('Toggle', () => {
  it('点击开关触发 onChange(true/false)', async () => {
    const onChange = vi.fn()
    const { rerender } = render(<Toggle value={false} onChange={onChange} />)
    // Switch 是 role=switch 的按钮
    await userEvent.click(screen.getByRole('switch'))
    expect(onChange).toHaveBeenLastCalledWith(true)
    rerender(<Toggle value={true} onChange={onChange} />)
    await userEvent.click(screen.getByRole('switch'))
    expect(onChange).toHaveBeenLastCalledWith(false)
  })

  it('状态文案跟随 value', () => {
    const { rerender } = render(<Toggle value={false} onChange={() => {}} />)
    expect(screen.getByText('已关闭')).toBeInTheDocument()
    rerender(<Toggle value={true} onChange={() => {}} />)
    expect(screen.getByText('已开启')).toBeInTheDocument()
  })
})
