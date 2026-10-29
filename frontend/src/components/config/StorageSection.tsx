// StorageSection.tsx —— 存储配置区块：主存储选择 + 媒体库全量刷新（自包含动作）。
import { RefreshCw } from 'lucide-react'
import { refreshLibrary } from '@/api'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import ConfigCard, { type ConfigSectionProps } from '@/components/config/ConfigCard'
import { FieldGroup, Field, NoticeMessage } from '@/components/config/fields'
import { useAsyncAction } from '@/hooks/useAsyncAction'
import { cn } from '@/lib/utils'

export default function StorageSection({ cfg, set }: ConfigSectionProps) {
  const fullRefreshAction = useAsyncAction()

  const handleFullRefresh = () =>
    fullRefreshAction.run(async () => {
      const res = await refreshLibrary(true)
      const data = res?.data || {}
      return `全量刷新完成：电影 ${data.total_movies ?? '-'} 部，剧集 ${data.total_tv ?? '-'} 部`
    }, '全量刷新失败')

  return (
    <ConfigCard
      title="云存储选择"
      description="选择媒体文件存放在哪个网盘"
      contentClassName="space-y-6"
    >
      <FieldGroup>
        <Field label="主存储">
          <Select
            value={((cfg?.storage as Record<string, unknown>)?.primary as string) || 'pan115'}
            onValueChange={(v) => set('storage', 'primary', v)}
          >
            <SelectTrigger className="w-full sm:w-48 focus:ring-0 focus:ring-offset-0">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="pan115">115 网盘</SelectItem>
            </SelectContent>
          </Select>
        </Field>
      </FieldGroup>

      <Separator className="opacity-60" />

      <div>
        <div className="mb-3 text-[13px] font-medium">媒体库全量刷新</div>
        <p className="mb-3 text-xs leading-5 text-muted-foreground">
          全量扫描所有文件并重建媒体库。日常使用无需操作，仅在增量刷新出现数据不一致时使用。
        </p>
        <Button
          variant="destructive"
          size="lg"
          onClick={handleFullRefresh}
          disabled={fullRefreshAction.busy}
        >
          <RefreshCw className={cn('size-5', fullRefreshAction.busy && 'animate-spin')} />
          {fullRefreshAction.busy ? '刷新中…' : '全量刷新媒体库'}
        </Button>
        <NoticeMessage message={fullRefreshAction.message} />
      </div>
    </ConfigCard>
  )
}
