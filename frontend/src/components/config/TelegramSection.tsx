// TelegramSection.tsx —— Telegram 通知配置区块：Bot 凭据 + 防抖 + 测试发送（自包含动作）。
import { Button } from '@/components/ui/button'
import { testTelegram } from '@/api'
import ConfigCard, { type ConfigSectionProps } from '@/components/config/ConfigCard'
import { FieldGroup, Field, TextInput, NumberInput } from '@/components/config/fields'
import { useAsyncAction } from '@/hooks/useAsyncAction'
import { cn } from '@/lib/utils'

export default function TelegramSection({ cfg, set }: ConfigSectionProps) {
  const telegramAction = useAsyncAction()
  const telegram = (cfg?.telegram as Record<string, unknown>) || {}

  const handleTelegramTest = () =>
    telegramAction.run(async () => {
      await testTelegram({
        bot_token: (telegram.bot_token as string) || '',
        chat_id: (telegram.chat_id as string) || '',
      })
      return '测试通知已发送，请在 Telegram 查看'
    }, '测试通知发送失败')

  return (
    <ConfigCard title="Telegram 通知" description="入库完成推送">
      <FieldGroup>
        <Field label="Bot Token" description="@BotFather 获取" fullWidth>
          <TextInput
            value={telegram.bot_token as string | undefined}
            onChange={(v) => set('telegram', 'bot_token', v)}
            type="password"
            placeholder="your-bot-token"
            mono
          />
        </Field>
        <Field label="Chat ID" description="接收通知的账号">
          <TextInput
            value={telegram.chat_id as string | undefined}
            onChange={(v) => set('telegram', 'chat_id', v)}
            placeholder="your-chat-id"
            mono
          />
        </Field>
        <Field label="防抖延时" description="批量合并通知（秒）">
          <NumberInput
            value={telegram.debounce_seconds as number | undefined}
            onChange={(v) => set('telegram', 'debounce_seconds', v)}
            min={0}
            max={600}
          />
        </Field>
      </FieldGroup>
      <div className="mt-4 flex items-center gap-3">
        <Button
          variant="outline"
          size="sm"
          onClick={handleTelegramTest}
          disabled={telegramAction.busy || !telegram.bot_token || !telegram.chat_id}
        >
          {telegramAction.busy ? '测试中…' : '发送测试通知'}
        </Button>
        {telegramAction.message && (
          <span className={cn('text-xs', telegramAction.message.type === 'success' ? 'text-success' : 'text-destructive')}>
            {telegramAction.message.text}
          </span>
        )}
      </div>
    </ConfigCard>
  )
}
