// RssSection.tsx —— RSS 订阅轮询配置区块。
import ConfigCard, { type ConfigSectionProps } from '@/components/config/ConfigCard'
import { FieldGroup, Field, TextInput } from '@/components/config/fields'

export default function RssSection({ cfg, set }: ConfigSectionProps) {
  return (
    <ConfigCard title="RSS 订阅" description="后台轮询设置">
      <FieldGroup>
        <Field label="定时规则" description="cron 表达式：分 时 日 月 周（服务器时区），支持 @every 30m 简写；留空表示不定时">
          <TextInput
            value={(cfg?.rss as Record<string, unknown>)?.poll_cron as string | undefined}
            onChange={(v) => set('rss', 'poll_cron', v)}
            placeholder="*/5 * * * *"
            mono
          />
        </Field>
      </FieldGroup>
    </ConfigCard>
  )
}
