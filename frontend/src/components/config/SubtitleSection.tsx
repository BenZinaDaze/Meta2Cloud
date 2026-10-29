// SubtitleSection.tsx —— 字幕整理配置区块。
import ConfigCard, { type ConfigSectionProps } from '@/components/config/ConfigCard'
import { FieldGroup, Field, Toggle } from '@/components/config/fields'

export default function SubtitleSection({ cfg, set }: ConfigSectionProps) {
  return (
    <ConfigCard title="字幕整理" description="外挂字幕文件自动跟随视频整理">
      <FieldGroup>
        <Field label="启用字幕整理" description="自动移动外挂字幕">
          <Toggle
            value={(cfg?.subtitle as Record<string, unknown>)?.enabled !== false}
            onChange={(v) => set('subtitle', 'enabled', v)}
          />
        </Field>
      </FieldGroup>
      <div className="mt-4 text-xs leading-6 text-muted-foreground">
        <p>支持的字幕格式：<code className="rounded bg-muted px-1 font-mono">.srt</code> <code className="rounded bg-muted px-1 font-mono">.ass</code> <code className="rounded bg-muted px-1 font-mono">.ssa</code></p>
        <p className="mt-1">语言代码标准化：简体中文 → <code className="rounded bg-muted px-1 font-mono">zh-CN</code>，繁体中文 → <code className="rounded bg-muted px-1 font-mono">zh-TW</code></p>
      </div>
    </ConfigCard>
  )
}
