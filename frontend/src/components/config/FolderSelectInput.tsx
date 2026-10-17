import { useCallback, useState } from 'react'
import { ChevronRight, Folder, Loader2 } from 'lucide-react'
import { listFiles } from '@/api'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { CloudFile, PathBreadcrumbItem } from '@/types/api'

interface FolderSelectInputProps {
  /** 存储值：目录名（网盘根目录下的直接子目录） */
  value?: string
  onChange: (name: string) => void
  placeholder?: string
}

export default function FolderSelectInput({ value, onChange, placeholder }: FolderSelectInputProps) {
  const [open, setOpen] = useState(false)
  const [loading, setLoading] = useState(false)
  const [items, setItems] = useState<CloudFile[]>([])
  const [crumbs, setCrumbs] = useState<PathBreadcrumbItem[]>([])

  const browse = useCallback(async (folderId?: string) => {
    setLoading(true)
    try {
      const res = await listFiles(folderId ? { folder_id: folderId } : undefined)
      setItems(((res.data?.items || []) as CloudFile[]).filter((f) => f.file_type === 'folder'))
      setCrumbs(res.data?.path || [])
    } finally {
      setLoading(false)
    }
  }, [])

  const openDialog = () => {
    setOpen(true)
    browse(crumbs.length ? crumbs[crumbs.length - 1].id : undefined)
  }

  const parent = crumbs.length >= 2 ? crumbs[crumbs.length - 2].id : ''

  const choose = () => {
    // 保存目录名而非 ID：后端按名解析，改名/重建无需重新配置
    const name = crumbs.length ? crumbs[crumbs.length - 1].name : ''
    if (!name) return
    onChange(name)
    setOpen(false)
  }


  return (
    <>
      <button
        type="button"
        onClick={openDialog}
        className="flex h-10 w-full items-center gap-2 rounded-md border border-input bg-transparent px-3 text-left text-sm transition-colors hover:border-foreground/30"
      >
        <Folder className="size-4 shrink-0 text-warning" />
        <span className={cn('truncate', value ? '' : 'text-muted-foreground')}>
          {value || placeholder || '点击选择目录…'}
        </span>
        <span className="ml-auto shrink-0 text-[11px] text-muted-foreground">选择</span>
      </button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>选择目录</DialogTitle>
            <DialogDescription>浏览 115 网盘，选中目录后以目录名保存</DialogDescription>
          </DialogHeader>

          <div className="flex flex-wrap items-center gap-1 text-xs text-muted-foreground">
            <button onClick={() => browse()} className="hover:text-foreground">
              根目录
            </button>
            {crumbs
              .filter((c, i) => i === 0 || c.name !== crumbs[i - 1].name)
              .map((c, i, arr) => (
                <span key={c.id} className="flex items-center gap-1">
                  {i > 0 && <ChevronRight className="size-3" />}
                  <button
                    onClick={() => browse(c.id)}
                    className={i === arr.length - 1 ? 'text-foreground' : 'hover:text-foreground'}
                  >
                    {c.name || c.id}
                  </button>
                </span>
              ))}
          </div>

          <div className="max-h-72 min-h-32 overflow-y-auto rounded-lg border border-border/60">
            {loading ? (
              <div className="flex items-center justify-center gap-2 p-6 text-sm text-muted-foreground">
                <Loader2 className="size-4 animate-spin" />
                加载中…
              </div>
            ) : items.length === 0 ? (
              <div className="p-6 text-center text-sm text-muted-foreground">没有子目录</div>
            ) : (
              items.map((f) => (
                <button
                  key={f.id}
                  onClick={() => browse(f.id)}
                  className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm transition-colors hover:bg-muted/50"
                >
                  <Folder className="size-4 shrink-0 text-warning" />
                  <span className="flex-1 truncate">{f.name}</span>
                  <ChevronRight className="size-3.5 shrink-0 text-muted-foreground" />
                </button>
              ))
            )}
          </div>

          <DialogFooter className="gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => browse(parent)}
              disabled={crumbs.length === 0}
            >
              返回上级
            </Button>
            <Button size="sm" onClick={choose}>
              选择当前目录
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
