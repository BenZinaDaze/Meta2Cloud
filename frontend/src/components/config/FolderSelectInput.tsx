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
import type { CloudFile } from '@/types/api'

interface Crumb {
  id: string
  name: string
}

const ROOT_CRUMB: Crumb = { id: '0', name: '根目录' }

interface FolderSelectInputProps {
  /** 存储值：网盘根目录起算的相对路径（如 Media 或 Media/剧集），后端按名解析 */
  value?: string
  onChange: (value: string) => void
  placeholder?: string
}

export default function FolderSelectInput({ value, onChange, placeholder }: FolderSelectInputProps) {
  const [open, setOpen] = useState(false)
  const [loading, setLoading] = useState(false)
  const [items, setItems] = useState<CloudFile[]>([])
  // 面包屑在前端自维护：进入子目录时名字就在列表里，栈式追加/截断，
  // 不依赖后端 path（其目录名需额外传参且层级有限），也不产生额外 API 调用
  const [crumbs, setCrumbs] = useState<Crumb[]>([ROOT_CRUMB])

  const browse = useCallback(async (folderId?: string, folderName?: string) => {
    setLoading(true)
    try {
      const res = await listFiles(folderId ? { folder_id: folderId } : undefined)
      setItems(((res.data?.items || []) as CloudFile[]).filter((f) => f.file_type === 'folder'))
      setCrumbs((prev) => {
        if (!folderId) return [ROOT_CRUMB]
        const idx = prev.findIndex((c) => c.id === folderId)
        if (idx >= 0) return prev.slice(0, idx + 1) // 点面包屑回跳：截断
        return [...prev, { id: folderId, name: folderName || folderId }]
      })
    } finally {
      setLoading(false)
    }
  }, [])

  const openDialog = () => {
    setOpen(true)
    const last = crumbs[crumbs.length - 1]
    browse(last.id === ROOT_CRUMB.id ? undefined : last.id, last.name)
  }

  const parent = crumbs.length >= 2 ? crumbs[crumbs.length - 2] : null

  const choose = () => {
    // 根目录不可选（根下目录才有意义）；保存相对路径而非 ID：后端按名解析，改名/重建无需重新配置
    if (crumbs.length <= 1) return
    const selected = crumbs.slice(1).map((c) => c.name).join('/')
    if (!selected) return
    onChange(selected)
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
            <DialogDescription>浏览 115 网盘，选中目录后以相对路径保存</DialogDescription>
          </DialogHeader>

          <div className="flex flex-wrap items-center gap-1 text-xs text-muted-foreground">
            {crumbs.map((c, i) => (
              <span key={c.id} className="flex items-center gap-1">
                {i > 0 && <ChevronRight className="size-3" />}
                <button
                  onClick={() => browse(c.id === ROOT_CRUMB.id ? undefined : c.id, c.name)}
                  className={i === crumbs.length - 1 ? 'text-foreground' : 'hover:text-foreground'}
                >
                  {c.name}
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
                  onClick={() => browse(f.id, f.name)}
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
              onClick={() => parent && browse(parent.id === ROOT_CRUMB.id ? undefined : parent.id, parent.name)}
              disabled={!parent}
            >
              返回上级
            </Button>
            <Button size="sm" onClick={choose} disabled={crumbs.length <= 1}>
              选择当前目录
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
