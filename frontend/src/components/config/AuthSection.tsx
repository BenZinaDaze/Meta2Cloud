// AuthSection.tsx —— WebUI 认证配置区块：用户名/密码/会话/webhook。
import ConfigCard, { type ConfigSectionProps } from '@/components/config/ConfigCard'
import { FieldGroup, Field, TextInput, NumberInput } from '@/components/config/fields'

export default function AuthSection({ cfg, set }: ConfigSectionProps) {
  return (
    <ConfigCard title="WebUI 认证" description="登录凭据与会话设置">
      <FieldGroup>
        <Field label="用户名" description="默认 admin">
          <TextInput
            value={(cfg?.webui as Record<string, unknown>)?.username as string | undefined}
            onChange={(v) => set('webui', 'username', v)}
            placeholder="输入登录用户名"
          />
        </Field>
        <Field label="密码" description="留空保持当前密码不变">
          <TextInput
            value={(cfg?.webui as Record<string, unknown>)?.password as string | undefined}
            onChange={(v) => set('webui', 'password', v)}
            type="password"
            placeholder="输入新密码"
          />
        </Field>
        <Field label="Token 有效期" description="小时">
          <NumberInput
            value={(cfg?.webui as Record<string, unknown>)?.token_expire_hours as number | undefined}
            onChange={(v) => set('webui', 'token_expire_hours', v)}
            min={1}
            max={8760}
          />
        </Field>
        <Field label="日志保留" description="天">
          <NumberInput
            value={(cfg?.webui as Record<string, unknown>)?.log_retention_days as number | undefined}
            onChange={(v) => set('webui', 'log_retention_days', v)}
            min={1}
            max={365}
          />
        </Field>
        <Field label="Webhook 密钥" description="/trigger 端点校验" fullWidth>
          <TextInput
            value={(cfg?.webui as Record<string, unknown>)?.webhook_secret as string | undefined}
            onChange={(v) => set('webui', 'webhook_secret', v)}
            type="password"
            placeholder="留空则不校验"
            mono
          />
        </Field>
      </FieldGroup>
    </ConfigCard>
  )
}
