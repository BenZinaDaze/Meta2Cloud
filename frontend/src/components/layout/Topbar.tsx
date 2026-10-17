import { useState } from 'react'
import { Zap, Search, Menu } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import { triggerPipeline } from '@/api'
import { toast } from 'sonner'
import Logo from '@/components/layout/Logo'

interface TopbarProps {
  onOpenParseTest?: () => void
  onToggleSidebar?: () => void
  /** 115 未授权时整理不可用，隐藏入口 */
  u115Authorized?: boolean
}

export default function Topbar({ onOpenParseTest, onToggleSidebar, u115Authorized = false }: TopbarProps) {
  const [confirmOpen, setConfirmOpen] = useState(false)
  const handleTriggerPipeline = async () => {
    try {
      await triggerPipeline()
      toast.success('已发送整理指令', { description: '后台整理将立即启动' })
    } catch (e: unknown) {
      const error = e as { response?: { data?: { detail?: string } }; message?: string }
      toast.error('触发失败', { description: error?.response?.data?.detail || error?.message || '未知错误' })
    }
  }

  return (
    <header className="fixed left-0 right-0 top-0 z-50 h-14 border-b border-border/60 glass-strong lg:left-[248px]">
      <div className="flex h-full items-center justify-between px-4 lg:px-6">
        <div className="flex items-center gap-3">
          <Button
            variant="ghost"
            size="icon"
            onClick={onToggleSidebar}
            className="lg:hidden"
          >
            <Menu className="size-5" />
            <span className="sr-only">打开导航</span>
          </Button>

          {/* 移动端品牌 */}
          <div className="flex items-center gap-2 lg:hidden">
            <Logo className="size-7" />
            <span className="text-sm font-bold tracking-tight">
              Meta<span className="text-gradient-tide">2</span>Cloud
            </span>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {u115Authorized && (
            <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
              <AlertDialogTrigger asChild>
                <Button size="sm" className="gap-1.5 rounded-full px-3.5">
                  <Zap className="size-3.5" />
                  <span className="hidden sm:inline">一键整理</span>
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>确认开始整理？</AlertDialogTitle>
                  <AlertDialogDescription>
                    将扫描云下载目录中的视频与字幕，识别后移动到媒体库分类目录；
                    完成后自动生成 STRM 并推送通知。进行中的整理无法中止。
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>取消</AlertDialogCancel>
                  <AlertDialogAction onClick={handleTriggerPipeline}>开始整理</AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          )}

          <Button
            variant="outline"
            size="sm"
            onClick={onOpenParseTest}
            className="hidden min-[430px]:flex gap-1.5 rounded-full px-3.5"
          >
            <Search className="size-3.5" />
            <span className="hidden sm:inline">解析测试</span>
          </Button>
        </div>
      </div>
    </header>
  )
}
