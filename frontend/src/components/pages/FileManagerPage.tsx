import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Folder,
  Film,
  FileText,
  File,
  RefreshCw,
  FolderPlus,
  Edit3,
  Trash2,
  ArrowRight,
  ArrowLeft,
  Search,
  CheckSquare,
  Square,
  X,
} from 'lucide-react'
import { listFiles, createFolder, updateFile, deleteFile, batchAction } from '@/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import ParseTestModal from '@/components/modals/ParseTestModal'
import { StatePanel } from '@/components/StatePanel'
import type { CloudFile, ListFilesResponse } from '@/types/api'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { formatBytes } from '@/lib/format'
import { formatDateTime } from '@/lib/time'

function getFileIcon(file: CloudFile) {
  if (file.file_type === 'folder') {
    return <Folder className="size-5 text-warning" />
  }
  if (file.is_video) {
    return <Film className="size-5 text-brand" />
  }
  if (file.is_subtitle) {
    return <FileText className="size-5 text-info" />
  }
  return <File className="size-5 text-muted-foreground" />
}

type SortKey = 'name' | 'size' | 'modified_time'
type SortOrder = 'asc' | 'desc'

export default function FileManagerPage() {
  const [data, setData] = useState<ListFilesResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [currentFolderId, setCurrentFolderId] = useState<string | undefined>(undefined)
  const [currentFolderName, setCurrentFolderName] = useState<string | undefined>(undefined)
  const [pathStack, setPathStack] = useState<{ id: string; name: string }[]>([])
  const [searchQuery, setSearchQuery] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [sortBy, setSortBy] = useState<SortKey>('name')
  const [sortOrder, setSortOrder] = useState<SortOrder>('asc')
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())

  // Modals
  const [createFolderOpen, setCreateFolderOpen] = useState(false)
  const [newFolderName, setNewFolderName] = useState('')
  const [renameOpen, setRenameOpen] = useState(false)
  const [renameFile, setRenameFile] = useState<CloudFile | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const [moveOpen, setMoveOpen] = useState(false)
  const [moveFileIds, setMoveFileIds] = useState<string[]>([])
  const [moveTitle, setMoveTitle] = useState('移动文件')
  const [deleteFile_, setDeleteFile_] = useState<CloudFile | null>(null)
  const [parseTestOpen, setParseTestOpen] = useState(false)
  const [parseTestFilename, setParseTestFilename] = useState('')

  const fetchFiles = useCallback(async () => {
    setLoading(true)
    setError(null)
    setSelectedIds(new Set())
    try {
      const params: { folder_id?: string; folder_name?: string; search?: string } = {}
      if (currentFolderId) params.folder_id = currentFolderId
      if (currentFolderName) params.folder_name = currentFolderName
      if (debouncedSearch) params.search = debouncedSearch
      const res = await listFiles(params)
      setData(res.data)
    } catch (err) {
      setError((err as Error).message || '加载失败')
    } finally {
      setLoading(false)
    }
  }, [currentFolderId, currentFolderName, debouncedSearch])

  useEffect(() => {
    const timer = setTimeout(() => setDebouncedSearch(searchQuery), 250)
    return () => clearTimeout(timer)
  }, [searchQuery])

  useEffect(() => {
    fetchFiles()
  }, [fetchFiles])

  const sortedItems = useMemo(() => {
    if (!data?.items) return []
    const items = [...data.items]
    items.sort((a, b) => {
      // 文件夹始终在前
      if (a.file_type !== b.file_type) {
        return a.file_type === 'folder' ? -1 : 1
      }
      let cmp = 0
      switch (sortBy) {
        case 'name':
          cmp = a.name.localeCompare(b.name, 'zh-CN')
          break
        case 'size':
          cmp = (a.size || 0) - (b.size || 0)
          break
        case 'modified_time':
          cmp = (a.modified_time || '').localeCompare(b.modified_time || '')
          break
      }
      return sortOrder === 'asc' ? cmp : -cmp
    })
    return items
  }, [data, sortBy, sortOrder])

  const handleEnterFolder = (folderId: string, folderName: string) => {
    if (currentFolderId) {
      setPathStack([...pathStack, { id: currentFolderId, name: currentFolderName || '' }])
    }
    setCurrentFolderId(folderId)
    setCurrentFolderName(folderName)
    setSearchQuery('')
    setDebouncedSearch('')
  }

  const handleGoBack = () => {
    if (pathStack.length > 0) {
      const prev = pathStack[pathStack.length - 1]
      setPathStack(pathStack.slice(0, -1))
      setCurrentFolderId(prev.id)
      setCurrentFolderName(prev.name)
    } else {
      setCurrentFolderId(undefined)
      setCurrentFolderName(undefined)
    }
    setSearchQuery('')
    setDebouncedSearch('')
  }

  
  const handleDoubleClick = (file: CloudFile) => {
    if (file.file_type === 'folder') {
      handleEnterFolder(file.id, file.name)
    }
  }

  const handleCreateFolder = async () => {
    if (!newFolderName.trim()) return
    try {
      await createFolder({ name: newFolderName.trim(), parent_id: currentFolderId })
      toast.success('文件夹创建成功')
      setCreateFolderOpen(false)
      setNewFolderName('')
      fetchFiles()
    } catch (err) {
      toast.error((err as Error).message || '创建失败')
    }
  }

  const handleRename = async () => {
    if (!renameFile || !renameValue.trim()) return
    try {
      await updateFile(renameFile.id, { name: renameValue.trim() })
      toast.success('重命名成功')
      setRenameOpen(false)
      setRenameFile(null)
      setRenameValue('')
      fetchFiles()
    } catch (err) {
      toast.error((err as Error).message || '重命名失败')
    }
  }

  const handleDelete = async () => {
    if (!deleteFile_) return
    try {
      await deleteFile(deleteFile_.id)
      toast.success('删除成功')
      setDeleteFile_(null)
      fetchFiles()
    } catch (err) {
      toast.error((err as Error).message || '删除失败')
    }
  }

  const handleBatchDelete = async () => {
    if (selectedIds.size === 0) return
    try {
      const res = await batchAction({ action: 'delete', file_ids: Array.from(selectedIds) })
      toast.success(`成功删除 ${res.data.deleted_count} 个文件`)
      setSelectedIds(new Set())
      fetchFiles()
    } catch (err) {
      toast.error((err as Error).message || '批量删除失败')
    }
  }

  const handleOpenParseTest = (file: CloudFile) => {
    setParseTestFilename(file.name)
    setParseTestOpen(true)
  }

  const toggleSelect = (id: string) => {
    const newSet = new Set(selectedIds)
    if (newSet.has(id)) {
      newSet.delete(id)
    } else {
      newSet.add(id)
    }
    setSelectedIds(newSet)
  }

  const handleSortToggle = (key: SortKey) => {
    if (sortBy === key) {
      setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc')
    } else {
      setSortBy(key)
      setSortOrder('asc')
    }
  }

  const toggleSelectAll = () => {
    if (selectedIds.size === sortedItems.length) {
      setSelectedIds(new Set())
    } else {
      setSelectedIds(new Set(sortedItems.map(f => f.id)))
    }
  }

  if (loading && !data) {
    return <StatePanel title="加载中..." />
  }

  if (error) {
    return (
      <StatePanel
        title="加载失败"
        description={error}
        tone="danger"
        action={
          <Button onClick={fetchFiles}>重试</Button>
        }
      />
    )
  }

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div className="border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="flex items-center justify-between gap-4 px-4 py-3">
          <div className="flex items-center gap-2">
            <Folder className="size-5 text-brand" />
            <h1 className="text-lg font-semibold">文件管理</h1>
          </div>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={() => setCreateFolderOpen(true)}>
              <FolderPlus className="size-4" />
              新建文件夹
            </Button>
            <Button variant="outline" size="sm" onClick={fetchFiles}>
              <RefreshCw className={cn('size-4', loading && 'animate-spin')} />
              刷新
            </Button>
          </div>
        </div>

        {/* Current folder & Search & Batch Actions */}
        <div className="flex items-center justify-between gap-4 px-4 pb-3">
          <div className="flex items-center gap-2 min-w-0">
            {(currentFolderId || pathStack.length > 0) && (
              <Button
                variant="ghost"
                size="sm"
                onClick={handleGoBack}
              >
                <ArrowLeft className="size-4" />
                返回上一层
              </Button>
            )}
            <span className="text-sm text-muted-foreground truncate">
              {currentFolderName || '根目录'}
            </span>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            {selectedIds.size > 0 && (
              <>
                <span className="text-sm text-muted-foreground">已选择 {selectedIds.size} 项</span>
                <Button variant="outline" size="sm" onClick={() => { setMoveFileIds(Array.from(selectedIds)); setMoveTitle(`移动 ${selectedIds.size} 个项目`); setMoveOpen(true) }}>
                  <ArrowRight className="size-4" />
                  批量移动
                </Button>
                <Button variant="destructive" size="sm" onClick={handleBatchDelete}>
                  <Trash2 className="size-4" />
                  批量删除
                </Button>
                <Button variant="ghost" size="sm" onClick={() => setSelectedIds(new Set())}>
                  <X className="size-4" />
                  取消
                </Button>
              </>
            )}
            <div className="relative">
              <Search className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="搜索文件..."
                className="w-48 pl-8"
              />
            </div>
          </div>
        </div>
      </div>

      {/* File List Header */}
      <div className="flex items-center gap-4 border-b bg-muted/30 px-4 py-2 text-xs font-medium text-muted-foreground">
        <button
          onClick={toggleSelectAll}
          className="flex-shrink-0"
        >
          {selectedIds.size === sortedItems.length && sortedItems.length > 0 ? (
            <CheckSquare className="size-4 text-brand" />
          ) : (
            <Square className="size-4" />
          )}
        </button>
        <button
          className="flex-1 text-left hover:text-foreground"
          onClick={() => handleSortToggle('name')}
        >
          名称 {sortBy === 'name' && (sortOrder === 'asc' ? '↑' : '↓')}
        </button>
        <button
          className="w-24 text-right hover:text-foreground"
          onClick={() => handleSortToggle('size')}
        >
          大小 {sortBy === 'size' && (sortOrder === 'asc' ? '↑' : '↓')}
        </button>
        <button
          className="w-40 text-right hover:text-foreground"
          onClick={() => handleSortToggle('modified_time')}
        >
          修改时间 {sortBy === 'modified_time' && (sortOrder === 'asc' ? '↑' : '↓')}
        </button>
        <div className="w-32" />
      </div>

      {/* File List */}
      <div className="flex-1 overflow-y-auto">
        {sortedItems.length === 0 ? (
          <div className="flex h-full items-center justify-center text-muted-foreground">
            {searchQuery ? '没有找到匹配的文件' : '文件夹为空'}
          </div>
        ) : (
          <div className="divide-y">
            {sortedItems.map((file) => (
              <div
                key={file.id}
                className={cn(
                  'flex items-center gap-4 px-4 py-2.5 hover:bg-muted/50 transition-colors',
                  selectedIds.has(file.id) && 'bg-muted/30'
                )}
              >
                <button
                  onClick={() => toggleSelect(file.id)}
                  className="flex-shrink-0"
                >
                  {selectedIds.has(file.id) ? (
                    <CheckSquare className="size-4 text-brand" />
                  ) : (
                    <Square className="size-4 text-muted-foreground/50 hover:text-muted-foreground" />
                  )}
                </button>
                <div
                  className="flex flex-1 cursor-pointer items-center gap-3 select-none"
                  onDoubleClick={() => handleDoubleClick(file)}
                >
                  {getFileIcon(file)}
                  <span className="truncate">{file.name}</span>
                </div>
                <div className="w-24 text-right text-sm text-muted-foreground select-none">
                  {formatBytes(file.size)}
                </div>
                <div className="w-40 text-right text-sm text-muted-foreground select-none">
                  {formatDateTime(file.modified_time)}
                </div>
                <div className="flex w-32 justify-end gap-1">
                  {file.is_video && (
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      onClick={() => handleOpenParseTest(file)}
                      title="测试解析"
                    >
                      <Search className="size-4" />
                    </Button>
                  )}
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    onClick={() => { setRenameFile(file); setRenameValue(file.name); setRenameOpen(true) }}
                    title="重命名"
                  >
                    <Edit3 className="size-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    onClick={() => { setMoveFileIds([file.id]); setMoveTitle('移动文件'); setMoveOpen(true) }}
                    title="移动"
                  >
                    <ArrowRight className="size-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    onClick={() => setDeleteFile_(file)}
                    className="text-destructive hover:text-destructive"
                    title="删除"
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Create Folder Modal */}
      <Dialog open={createFolderOpen} onOpenChange={setCreateFolderOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>新建文件夹</DialogTitle>
            <DialogDescription>在当前目录创建新文件夹</DialogDescription>
          </DialogHeader>
          <Input
            value={newFolderName}
            onChange={(e) => setNewFolderName(e.target.value)}
            placeholder="文件夹名称"
            onKeyDown={(e) => e.key === 'Enter' && handleCreateFolder()}
          />
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateFolderOpen(false)}>取消</Button>
            <Button onClick={handleCreateFolder}>创建</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Rename Modal */}
      <Dialog open={renameOpen} onOpenChange={setRenameOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>重命名</DialogTitle>
          </DialogHeader>
          <Input
            value={renameValue}
            onChange={(e) => setRenameValue(e.target.value)}
            placeholder="新名称"
            onKeyDown={(e) => e.key === 'Enter' && handleRename()}
          />
          <DialogFooter>
            <Button variant="outline" onClick={() => setRenameOpen(false)}>取消</Button>
            <Button onClick={handleRename}>确定</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Move Modal */}
      <MoveModal
        open={moveOpen}
        onOpenChange={setMoveOpen}
        fileIds={moveFileIds}
        title={moveTitle}
        currentFolderId={currentFolderId}
        rootId={data?.root_id}
        onSuccess={() => { setMoveOpen(false); setSelectedIds(new Set()); fetchFiles() }}
      />

      {/* Delete Confirm */}
      <AlertDialog open={!!deleteFile_} onOpenChange={() => setDeleteFile_(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认删除</AlertDialogTitle>
            <AlertDialogDescription>
              确定要删除 "{deleteFile_?.name}" 吗？此操作不可恢复。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction onClick={handleDelete} className="bg-destructive text-destructive-foreground hover:bg-destructive/90">
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Parse Test Modal */}
      <ParseTestModal
        open={parseTestOpen}
        onOpenChange={setParseTestOpen}
        initialFilename={parseTestFilename}
      />
    </div>
  )
}

// Move Modal Component
function MoveModal({
  open,
  onOpenChange,
  fileIds,
  title,
  currentFolderId,
  rootId,
  onSuccess,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  fileIds: string[]
  title: string
  currentFolderId?: string
  rootId?: string
  onSuccess: () => void
}) {
  const [targetFolderId, setTargetFolderId] = useState<string | undefined>(undefined)
  const [folders, setFolders] = useState<CloudFile[]>([])
  const [loading, setLoading] = useState(false)
  const [pathStack, setPathStack] = useState<{ id: string; name: string }[]>([])

  const loadFolders = useCallback(async (folderId?: string, folderName?: string) => {
    setLoading(true)
    try {
      const res = await listFiles({ folder_id: folderId || rootId })
      const folderItems = res.data.items.filter((f: CloudFile) => f.file_type === 'folder')
      setFolders(folderItems)
      setTargetFolderId(folderId || rootId)
      if (folderId && folderName) {
        setPathStack(prev => [...prev, { id: folderId, name: folderName }])
      } else if (!folderId) {
        setPathStack([])
      }
    } catch {
      // ignore
    } finally {
      setLoading(false)
    }
  }, [rootId])

  const handleGoBack = useCallback(async () => {
    if (pathStack.length > 1) {
      const prev = pathStack[pathStack.length - 2]
      setPathStack(pathStack.slice(0, -1))
      setLoading(true)
      try {
        const res = await listFiles({ folder_id: prev.id })
        setFolders(res.data.items.filter((f: CloudFile) => f.file_type === 'folder'))
        setTargetFolderId(prev.id)
      } catch {
        // ignore
      } finally {
        setLoading(false)
      }
    } else {
      setPathStack([])
      loadFolders(rootId)
    }
  }, [pathStack, rootId, loadFolders])

  useEffect(() => {
    if (open) {
      setTargetFolderId(undefined)
      setPathStack([])
      loadFolders(rootId)
    }
  }, [open, rootId, loadFolders])

  const handleMove = async () => {
    if (fileIds.length === 0 || !targetFolderId) return
    try {
      if (fileIds.length === 1) {
        await updateFile(fileIds[0], { parent_id: targetFolderId })
      } else {
        await batchAction({ action: 'move', file_ids: fileIds, parent_id: targetFolderId })
      }
      toast.success('移动成功')
      onSuccess()
    } catch (err) {
      toast.error((err as Error).message || '移动失败')
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>选择目标文件夹</DialogDescription>
        </DialogHeader>
        <div className="min-h-[200px] space-y-2">
          <div className="flex items-center gap-2">
            {pathStack.length > 0 && (
              <Button variant="ghost" size="sm" onClick={handleGoBack}>
                <ArrowLeft className="size-4" />
                返回上一层
              </Button>
            )}
            <span className="text-sm text-muted-foreground">
              {pathStack.length > 0 ? pathStack[pathStack.length - 1].name : '根目录'}
            </span>
          </div>
          <div className="max-h-[200px] overflow-y-auto rounded border select-none">
            {loading ? (
              <div className="p-4 text-center text-muted-foreground">加载中...</div>
            ) : folders.length === 0 ? (
              <div className="p-4 text-center text-muted-foreground">无子文件夹</div>
            ) : (
              <div className="divide-y">
                {folders.map((folder) => (
                  <button
                    key={folder.id}
                    onClick={() => loadFolders(folder.id, folder.name)}
                    className={cn(
                      'flex w-full items-center gap-2 px-3 py-2 text-left hover:bg-muted/50',
                      targetFolderId === folder.id && 'bg-muted/50'
                    )}
                  >
                    <Folder className="size-4 text-warning" />
                    {folder.name}
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>取消</Button>
          <Button onClick={handleMove} disabled={!targetFolderId || targetFolderId === currentFolderId}>
            移动到此文件夹
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
