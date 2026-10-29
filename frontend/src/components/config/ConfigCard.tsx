import type { ReactNode } from 'react'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card'

/** 配置分节共享的 section 属性：cfg 为整份规范化配置 dict，set 写回某段某键。 */
export interface ConfigSectionProps {
  cfg: Record<string, unknown> | null
  set: (section: string, key: string, val: unknown) => void
}

/** 配置页分节卡片：统一边框/底色与标题排版（各 Section 复用）。 */
export default function ConfigCard({ title, description, children, contentClassName }: {
  title: string
  description: string
  children: ReactNode
  contentClassName?: string
}) {
  return (
    <Card className="border-border/70 bg-card/40">
      <CardHeader>
        <CardTitle className="text-base">{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className={contentClassName}>{children}</CardContent>
    </Card>
  )
}
