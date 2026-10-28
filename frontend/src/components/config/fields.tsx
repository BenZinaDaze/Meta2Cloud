// fields.tsx —— 配置页表单基础组件（自 ConfigPage 抽离，页面无关可复用）。
// FieldGroup/Field 负责两列栅格布局；TextInput 带敏感值小眼睛；
// NumberInput 编辑期本地草稿失焦钳位；ListField 标签式增删；NoticeMessage 操作反馈条。
import { useEffect, useState, useCallback } from 'react'
import { Eye, EyeOff, Plus, X } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'

export function FieldGroup({ children }: { children: React.ReactNode }) {
  // items-end：描述行数不同的相邻字段输入框保持底对齐（输入框均位于块尾）
  return <div className="grid items-end gap-x-8 gap-y-4 sm:grid-cols-2">{children}</div>
}

export function Field({
  label,
  description,
  children,
  fullWidth = false,
}: {
  label: string
  description?: string
  children: React.ReactNode
  fullWidth?: boolean
}) {
  return (
    <div className={fullWidth ? 'sm:col-span-2' : ''}>
      <div className="mb-1.5 flex items-baseline gap-2">
        <label className="text-[13px] font-medium">{label}</label>
        {description && (
          <span className="text-[11px] text-muted-foreground/80">{description}</span>
        )}
      </div>
      {children}
    </div>
  )
}

export function TextInput({ value, onChange, placeholder, type = 'text', mono = false }: {
  value: string | undefined
  onChange: (v: string) => void
  placeholder?: string
  type?: string
  mono?: boolean
}) {
  const [show, setShow] = useState(false)
  const isPassword = type === 'password'
  const inputType = isPassword ? (show ? 'text' : 'password') : type

  return (
    <div className="relative flex items-center">
      <Input
        type={inputType}
        value={value ?? ''}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className={mono ? 'font-mono text-[13px]' : ''}
      />
      {isPassword && (
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          onClick={() => setShow((s) => !s)}
          className="absolute right-2"
        >
          {show ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
        </Button>
      )}
    </div>
  )
}

export function NumberInput({ value, onChange, min, max }: {
  value: number | undefined
  onChange: (v: number | undefined) => void
  min?: number
  max?: number
}) {
  // 编辑期用本地草稿，失焦/回车才钳位提交——边输边钳会把 4 立刻变成 15，
  // 后续输入拼成 155，导致 45、100 这类含下限以下前缀的数值无法正常键入
  const [draft, setDraft] = useState<string | null>(null)
  const finish = () => {
    if (draft == null) return
    const raw = draft.trim()
    setDraft(null)
    if (raw === '') {
      onChange(undefined) // 清空交由保存时取默认值
      return
    }
    const v = Number(raw)
    if (Number.isNaN(v)) return // 非法输入直接还原为原值
    onChange(Math.min(max ?? v, Math.max(min ?? v, v)))
  }
  return (
    <Input
      type="number"
      value={draft ?? value ?? ''}
      onChange={(e) => setDraft(e.target.value)}
      onBlur={finish}
      onKeyDown={(e) => {
        if (e.key === 'Enter') finish()
      }}
      min={min}
      max={max}
      className="w-24 tabular-nums"
    />
  )
}

export function Toggle({ value, onChange }: { value: boolean | undefined; onChange: (v: boolean) => void }) {
  return (
    <div className="flex items-center gap-3">
      <Switch checked={!!value} onCheckedChange={onChange} />
      <span className="text-xs text-muted-foreground">{value ? '已开启' : '已关闭'}</span>
    </div>
  )
}

export function ListField({ value = [], onChange }: { value: string[]; onChange: (v: string[]) => void }) {
  const [draft, setDraft] = useState('')

  function add() {
    const v = draft.trim()
    if (!v || value.includes(v)) {
      setDraft('')
      return
    }
    onChange([...value, v])
    setDraft('')
  }

  return (
    <div className="space-y-2">
      {value.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {value.map((item, i) => (
            <div
              key={i}
              className="inline-flex max-w-full items-center gap-2 rounded-full border border-border/70 bg-muted/50 py-1.5 pl-3 pr-1.5"
            >
              <span className="truncate font-mono text-xs">{item}</span>
              <Button
                variant="ghost"
                size="icon-xs"
                onClick={() => onChange(value.filter((_, j) => j !== i))}
                className="text-muted-foreground hover:bg-destructive/15 hover:text-destructive"
              >
                <X className="size-3" />
              </Button>
            </div>
          ))}
        </div>
      )}
      <div className="flex gap-2">
        <Input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), add())}
          placeholder="输入后按 Enter 或点击添加"
          className="font-mono text-xs"
        />
        <Button variant="outline" size="sm" onClick={add} disabled={!draft.trim()}>
          <Plus className="size-4" />
          添加
        </Button>
      </div>
    </div>
  )
}

export function MultilineRulesField({ value = [], onChange, placeholder = '' }: {
  value: string[]
  onChange: (v: string[]) => void
  placeholder?: string
}) {
  const serialize = useCallback((lines: string[]) => (Array.isArray(lines) ? lines.join('\n') : ''), [])
  const parse = useCallback(
    (nextText: string) => nextText.split('\n').map((line) => line.trim()).filter(Boolean),
    [],
  )
  const [draftText, setDraftText] = useState(() => serialize(value))
  const lineCount = draftText === '' ? 1 : draftText.split('\n').length

  useEffect(() => {
    const incomingText = serialize(value)
    const draftMatchesValue =
      JSON.stringify(parse(draftText)) === JSON.stringify(Array.isArray(value) ? value : [])
    if (!draftMatchesValue && incomingText !== draftText) {
      setDraftText(incomingText)
    }
  }, [draftText, parse, serialize, value])

  return (
    <div className="space-y-2">
      <Textarea
        value={draftText}
        onChange={(e) => {
          const nextText = e.target.value
          setDraftText(nextText)
          onChange(parse(nextText))
        }}
        placeholder={placeholder}
        spellCheck={false}
        className="resize-none font-mono text-[13px] leading-6"
        rows={Math.max(8, Math.min(18, lineCount + 2))}
      />
      <div className="text-xs text-muted-foreground">
        一行一个规则，空行会自动忽略。当前 {value.length} 条。
      </div>
    </div>
  )
}

/** 生成 128 位熵的随机签名密钥（浏览器密码学安全随机源）。 */
export function randomSecret(): string {
  const bytes = new Uint8Array(16)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
}

export function NoticeMessage({ message }: { message: { type: 'success' | 'error'; text: string } | null }) {
  if (!message) return null
  return (
    <div
      className={cn(
        'mt-3 animate-stagger-in rounded-lg border px-3 py-2 text-xs',
        message.type === 'success'
          ? 'border-success/25 bg-success/10 text-success'
          : 'border-destructive/25 bg-destructive/10 text-destructive'
      )}
    >
      {message.text}
    </div>
  )
}
