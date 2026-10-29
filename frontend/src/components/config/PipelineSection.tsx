// PipelineSection.tsx —— 扫描与入库策略配置区块。
import ConfigCard, { type ConfigSectionProps } from '@/components/config/ConfigCard'
import { FieldGroup, Field, Toggle } from '@/components/config/fields'

export default function PipelineSection({ cfg, set }: ConfigSectionProps) {
  return (
    <ConfigCard title="扫描与入库策略" description="整理行为控制">
      <FieldGroup>
        <Field label="跳过 TMDB 查询" description="只整理文件夹">
          <Toggle
            value={(cfg?.pipeline as Record<string, unknown>)?.skip_tmdb as boolean | undefined}
            onChange={(v) => set('pipeline', 'skip_tmdb', v)}
          />
        </Field>
        <Field label="TMDB 未找到仍移动" description="否则跳过">
          <Toggle
            value={(cfg?.pipeline as Record<string, unknown>)?.move_on_tmdb_miss as boolean | undefined}
            onChange={(v) => set('pipeline', 'move_on_tmdb_miss', v)}
          />
        </Field>
        <Field label="替换同名视频" description="先移除再整理">
          <Toggle
            value={(cfg?.pipeline as Record<string, unknown>)?.replace_existing_video === true}
            onChange={(v) => set('pipeline', 'replace_existing_video', v)}
          />
        </Field>
      </FieldGroup>
    </ConfigCard>
  )
}
