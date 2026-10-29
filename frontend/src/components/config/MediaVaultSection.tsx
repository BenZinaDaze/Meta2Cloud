// MediaVaultSection.tsx —— MediaVault 配置区块：连接参数 + 连接测试（自包含动作）。
import { Button } from '@/components/ui/button'
import { testMediaVault } from '@/api'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import ConfigCard, { type ConfigSectionProps } from '@/components/config/ConfigCard'
import { FieldGroup, Field, TextInput, NumberInput, Toggle } from '@/components/config/fields'
import { useAsyncAction } from '@/hooks/useAsyncAction'
import { cn } from '@/lib/utils'

export default function MediaVaultSection({ cfg, set }: ConfigSectionProps) {
  const mediavaultAction = useAsyncAction()
  const mediavault = (cfg?.mediavault as Record<string, unknown>) || {}

  const handleMediaVaultTest = () =>
    mediavaultAction.run(async () => {
      const res = await testMediaVault({
        enabled: mediavault.enabled,
        scheme: mediavault.scheme,
        host: mediavault.host,
        api_key: mediavault.api_key,
        timeout: mediavault.timeout,
      })
      return res?.data?.message || 'MediaVault 测试成功'
    }, 'MediaVault 测试失败')

  return (
    <ConfigCard title="MediaVault" description="整理完成后触发目录监控">
      <div className="mb-6">
        <div className="mb-3 flex items-center gap-3">
          <span className="text-[13px] font-medium">连接测试</span>
          <Button
            variant="outline"
            size="sm"
            onClick={handleMediaVaultTest}
            disabled={mediavaultAction.busy || !mediavault.host || !mediavault.api_key}
          >
            {mediavaultAction.busy ? '测试中…' : '测试'}
          </Button>
          {mediavaultAction.message && (
            <span className={cn('text-xs', mediavaultAction.message.type === 'success' ? 'text-success' : 'text-destructive')}>
              {mediavaultAction.message.text}
            </span>
          )}
        </div>
      </div>

      <FieldGroup>
        <Field label="启用 MediaVault">
          <Toggle
            value={mediavault.enabled as boolean | undefined}
            onChange={(v) => set('mediavault', 'enabled', v)}
          />
        </Field>
        <Field label="请求超时" description="秒">
          <NumberInput
            value={(mediavault.timeout as number | undefined) ?? 10}
            onChange={(v) => set('mediavault', 'timeout', v)}
            min={1}
            max={120}
          />
        </Field>
        <Field label="协议" description="地址不需要带协议">
          <Select
            value={(mediavault.scheme as string | undefined) ?? 'http'}
            onValueChange={(v) => set('mediavault', 'scheme', v)}
          >
            <SelectTrigger>
              <SelectValue placeholder="选择协议" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="http">http</SelectItem>
              <SelectItem value="https">https</SelectItem>
            </SelectContent>
          </Select>
        </Field>
        <Field label="地址" description="只填主机或主机:端口">
          <TextInput
            value={mediavault.host as string | undefined}
            onChange={(v) => set('mediavault', 'host', v)}
            placeholder="mediavault:7811"
            mono
          />
        </Field>
        <Field label="API Key" description="MediaVault 系统配置中的 API Key" fullWidth>
          <TextInput
            value={mediavault.api_key as string | undefined}
            onChange={(v) => set('mediavault', 'api_key', v)}
            type="password"
            placeholder="your-api-key"
            mono
          />
        </Field>
        <Field label="source_dir" description="留空则使用 MediaVault 默认目录" fullWidth>
          <TextInput
            value={mediavault.source_dir as string | undefined}
            onChange={(v) => set('mediavault', 'source_dir', v)}
            placeholder="/path/to/watch"
            mono
          />
        </Field>
      </FieldGroup>
    </ConfigCard>
  )
}
