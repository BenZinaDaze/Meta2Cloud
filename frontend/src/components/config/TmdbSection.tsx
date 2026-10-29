// TmdbSection.tsx —— TMDB 元数据源配置区块。
import ConfigCard, { type ConfigSectionProps } from '@/components/config/ConfigCard'
import { FieldGroup, Field, TextInput, NumberInput } from '@/components/config/fields'

export default function TmdbSection({ cfg, set }: ConfigSectionProps) {
  return (
    <ConfigCard title="TMDB 设置" description="元数据源配置">
      <FieldGroup>
        <Field label="API Key" description="TMDB v3 必填" fullWidth>
          <TextInput
            value={(cfg?.tmdb as Record<string, unknown>)?.api_key as string | undefined}
            onChange={(v) => set('tmdb', 'api_key', v)}
            type="password"
            placeholder="your-tmdb-api-key"
            mono
          />
        </Field>
        <Field label="返回语言" description="如 zh-CN">
          <TextInput
            value={(cfg?.tmdb as Record<string, unknown>)?.language as string | undefined}
            onChange={(v) => set('tmdb', 'language', v)}
            placeholder="zh-CN"
          />
        </Field>
        <Field label="请求超时" description="秒">
          <NumberInput
            value={(cfg?.tmdb as Record<string, unknown>)?.timeout as number | undefined}
            onChange={(v) => set('tmdb', 'timeout', v)}
            min={1}
            max={120}
          />
        </Field>
        <Field label="HTTP 代理" description="可选" fullWidth>
          <TextInput
            value={(cfg?.tmdb as Record<string, unknown>)?.proxy as string | undefined}
            onChange={(v) => set('tmdb', 'proxy', v)}
            placeholder="http://proxy-host:port"
            mono
          />
        </Field>
        <Field label="TMDB 图片代理域名" description="可选，仅填根域名" fullWidth>
          <TextInput
            value={(cfg?.tmdb as Record<string, unknown>)?.image_base_url as string | undefined}
            onChange={(v) => set('tmdb', 'image_base_url', v)}
            placeholder="https://image-proxy.example.com"
            mono
          />
        </Field>
      </FieldGroup>
    </ConfigCard>
  )
}
