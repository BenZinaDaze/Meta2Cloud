// StrmSection.tsx —— STRM 生成与 302 播放配置区块：开关/定时/服务地址/签名/路径映射/伴随文件。
import { Plus, X, Dices } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import FolderSelectInput from '@/components/config/FolderSelectInput'
import ConfigCard, { type ConfigSectionProps } from '@/components/config/ConfigCard'
import {
  FieldGroup,
  Field,
  TextInput,
  Toggle,
  ListField,
  randomSecret,
} from '@/components/config/fields'

export default function StrmSection({ cfg, set }: ConfigSectionProps) {
  const strm = (cfg?.strm as Record<string, unknown>) || {}
  const mappings = (strm.mappings as Array<Record<string, unknown>>) || []

  const setMapping = (i: number, patch: Partial<Record<string, unknown>>) => {
    const list = [...mappings]
    list[i] = { ...list[i], ...patch }
    set('strm', 'mappings', list)
  }

  return (
    <ConfigCard
      title="STRM 生成与 302 播放"
      description="为网盘媒体库生成本地 .strm 文件，播放时 302 直链串流，无需下载到本地"
      contentClassName="space-y-6"
    >
      <FieldGroup>
        <Field label="启用 STRM" description="整理入库成功后自动生成">
          <Toggle
            value={(strm.enabled as boolean) || false}
            onChange={(v) => set('strm', 'enabled', v)}
          />
        </Field>
        <Field label="定时规则" description="cron 表达式：分 时 日 月 周（服务器时区），支持 @every 30m 简写；留空仅整理入库/手动/删除后触发">
          <TextInput
            value={strm.sync_cron as string | undefined}
            onChange={(v) => set('strm', 'sync_cron', v)}
            placeholder="0 8 * * *"
            mono
          />
        </Field>
        <Field label="服务地址 (url_base)" description="写入 .strm 的地址，支持 IP:端口或域名（含反代/HTTPS）；省略 http:// 时自动按 http 处理" fullWidth>
          <TextInput
            value={strm.url_base as string | undefined}
            onChange={(v) => set('strm', 'url_base', v)}
            placeholder="http://192.168.1.10:38765 或 https://strm.example.com"
            mono
          />
        </Field>
        <Field label="访问签名 (url_secret)" description="可选，防止未授权调用 302 直链；修改后需重新同步" fullWidth>
          <div className="flex gap-2">
            <Input
              value={(strm.url_secret as string) ?? ''}
              onChange={(e) => set('strm', 'url_secret', e.target.value)}
              placeholder="留空不启用签名校验"
              className="font-mono text-[13px]"
            />
            <Button
              variant="outline"
              size="sm"
              className="shrink-0"
              title="生成 128 位随机密钥"
              onClick={() => set('strm', 'url_secret', randomSecret())}
            >
              <Dices className="size-4" />
              随机
            </Button>
          </div>
        </Field>
      </FieldGroup>

      <div>
        <div className="mb-2 flex items-baseline gap-2">
          <label className="text-[13px] font-medium">路径映射</label>
          <span className="text-[11px] text-muted-foreground/80">网盘媒体库目录 → 本地 STRM 输出目录，目录结构镜像网盘</span>
        </div>
        <div className="space-y-2">
          {mappings.map((m, i) => (
            <div key={i} className="flex flex-col gap-2 sm:flex-row sm:items-center">
              <FolderSelectInput
                value={(m.pan_folder_id as string) ?? ''}
                onChange={(v) => setMapping(i, { pan_folder_id: v })}
                placeholder="选择网盘目录"
              />
              <Input
                value={(m.local_path as string) ?? ''}
                onChange={(e) => setMapping(i, { local_path: e.target.value })}
                placeholder="/mnt/media/115/movies"
                className="font-mono text-[13px]"
              />
              <Button
                variant="ghost"
                size="icon-xs"
                className="text-muted-foreground hover:bg-destructive/15 hover:text-destructive"
                onClick={() => {
                  const list = [...mappings]
                  list.splice(i, 1)
                  set('strm', 'mappings', list)
                }}
              >
                <X className="size-3.5" />
              </Button>
            </div>
          ))}
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              set('strm', 'mappings', [...mappings, { pan_folder_id: '', local_path: '' }])
            }}
          >
            <Plus className="size-4" />
            添加映射
          </Button>
        </div>
      </div>

      <FieldGroup>
        <Field label="媒体扩展名" description="这些扩展名的网盘文件会生成 STRM" fullWidth>
          <ListField
            value={(strm.media_ext as string[]) || []}
            onChange={(v) => set('strm', 'media_ext', v)}
          />
        </Field>
      </FieldGroup>

      <FieldGroup>
        <Field label="下载伴随文件" description="外挂字幕/元数据/图片下载到本地，否则 STRM 播放无法加载外挂字幕">
          <Toggle
            value={(strm.fetch_sidecar as boolean) || false}
            onChange={(v) => set('strm', 'fetch_sidecar', v)}
          />
        </Field>
        <Field label="同步后通知 Plex" description="同步有增/改/删后自动通知 Plex 扫描对应媒体库（需已配对 Plex）">
          <Toggle
            value={(strm.refresh_plex as boolean) || false}
            onChange={(v) => set('strm', 'refresh_plex', v)}
          />
        </Field>
        <Field label="伴随文件扩展名" description="单文件上限 20MB，与 .strm 同目录存放" fullWidth>
          <ListField
            value={(strm.sidecar_ext as string[]) || []}
            onChange={(v) => set('strm', 'sidecar_ext', v)}
          />
        </Field>
      </FieldGroup>

      <div className="text-xs leading-6 text-muted-foreground">
        <p>.strm 内容形如 <code className="rounded bg-muted px-1 font-mono">http://服务地址/strm/redirect?pickcode=...</code>，播放时实时向 115 换取直链（直链与播放器 User-Agent 绑定，服务端自动透传并缓存）。</p>
        <p className="mt-1">触发方式：整理入库成功后自动生成 + 定时增量同步 + 本页保存后到「STRM 同步」页手动触发。开启「同步后通知 Plex」后，同步有增/改/删会自动通知 Plex 扫库。将本地 STRM 目录作为媒体库加入 Emby / Jellyfin / Infuse 即可播放。</p>
      </div>
    </ConfigCard>
  )
}
