import { Zap, Search, Menu } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { ThemeToggle } from '@/components/ThemeToggle'
import { triggerPipeline } from '@/api'
import { toast } from 'sonner'
import BrandMark from '@/components/BrandMark'

interface TopbarProps {
  onOpenParseTest?: () => void
  onToggleSidebar?: () => void
}

export default function Topbar({ onOpenParseTest, onToggleSidebar }: TopbarProps) {
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
    <header className="fixed left-0 right-0 top-0 z-50 h-14 border-b bg-background">
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

          <BrandMark className="size-8" compact />
          <div>
            <div className="hidden text-[9px] font-semibold uppercase tracking-widest text-muted-foreground sm:block">
              Media Archive
            </div>
            <span className="block text-sm font-semibold">
              Meta<span className="text-primary">2Cloud</span>
            </span>
          </div>
        </div>

        <div className="flex items-center gap-2">
          <ThemeToggle />

          <Button
            onClick={handleTriggerPipeline}
            size="sm"
            className="gap-1.5"
          >
            <Zap className="size-3.5" />
            <span className="hidden sm:inline">一键整理</span>
          </Button>

          <Button
            variant="outline"
            size="sm"
            onClick={onOpenParseTest}
            className="hidden min-[430px]:flex gap-1.5"
          >
            <Search className="size-3.5" />
            <span className="hidden sm:inline">解析测试</span>
          </Button>
        </div>
      </div>
    </header>
  )
}
