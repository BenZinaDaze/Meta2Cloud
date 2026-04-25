import { useCallback, useEffect, useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Loader2, ChevronDown, ExternalLink } from 'lucide-react'
import { testParse } from '@/api'

const STATUS_MAP: Record<string, string> = {
  'Returning Series': '连载中',
  'Ended': '已完结',
  'Canceled': '已取消',
  'In Production': '制作中',
  'Planned': '计划中',
  'Pilot': '试播',
  'In Limbo': '播出未定',
}

function formatStatus(status: string | undefined): string | undefined {
  if (!status) return status
  return STATUS_MAP[status] ?? status
}

interface InfoCardProps {
  label: string
  value: string | number
}

function InfoCard({ label, value }: InfoCardProps) {
  return (
    <div className="rounded-xl px-3 py-3 bg-muted/50 border">
      <div className="text-[11px] font-semibold uppercase tracking-widest text-muted-foreground">
        {label}
      </div>
      <div className="mt-1 break-all text-sm">{String(value)}</div>
    </div>
  )
}

interface ParseTestModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  initialFilename?: string
}

export default function ParseTestModal({
  open,
  onOpenChange,
  initialFilename = '',
}: ParseTestModalProps) {
  const [filename, setFilename] = useState(initialFilename)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState<{
    name?: string
    type_label?: string
    year?: string
    season?: number
    episode?: number
    resource_term?: string
    video_term?: string
    audio_term?: string
    release_group?: string
    tmdbid?: number
    doubanid?: string
    apply_words?: string[]
    tmdb?: {
      backdrop_url?: string
      poster_url?: string
      title?: string
      original_title?: string
      year?: string
      rating?: number
      status?: string
      media_type_label?: string
      media_type?: string
      tmdb_id?: number
      overview?: string
      release_date?: string
    }
  } | null>(null)
  const [showDetails, setShowDetails] = useState(true)
  const [showOverview, setShowOverview] = useState(false)

  const handleClose = useCallback(() => {
    onOpenChange(false)
  }, [onOpenChange])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && open) {
        handleClose()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, handleClose])

  useEffect(() => {
    if (initialFilename) {
      setFilename(initialFilename)
    }
  }, [initialFilename])

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const value = filename.trim()
    if (!value || loading) return

    setLoading(true)
    setError('')
    try {
      const res = await testParse(value)
      setResult(res.data)
      setShowOverview(false)
    } catch (err) {
      setResult(null)
      setError(
        (err as { response?: { data?: { detail?: string } }; message?: string })?.response?.data
          ?.detail ||
          (err as { message?: string })?.message ||
          '解析失败'
      )
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (!initialFilename?.trim() || !open) return

    let cancelled = false

    async function runInitialParse() {
      const value = initialFilename.trim()
      setLoading(true)
      setError('')
      try {
        const res = await testParse(value)
        if (!cancelled) {
          setFilename(value)
          setResult(res.data)
          setShowOverview(false)
        }
      } catch (err) {
        if (!cancelled) {
          setResult(null)
          setError(
            (err as { response?: { data?: { detail?: string } }; message?: string })?.response?.data
              ?.detail ||
              (err as { message?: string })?.message ||
              '解析失败'
          )
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    runInitialParse()
    return () => {
      cancelled = true
    }
  }, [initialFilename, open])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl p-0 gap-0 overflow-hidden max-h-[calc(100dvh-2rem)] flex flex-col">
        <DialogTitle className="sr-only">解析测试</DialogTitle>
        <DialogDescription className="sr-only">
          输入文件名或路径，测试解析结果和 TMDB 命中情况
        </DialogDescription>
        {/* Header */}
        <div className="border-b px-4 pb-4 pt-6 sm:px-6">
          <div className="flex items-start justify-between gap-4">
            <div>
              <div className="text-[11px] font-semibold uppercase tracking-widest text-brand">
                Parser Sandbox
              </div>
              <h2 className="mt-1.5 text-xl font-bold leading-snug sm:text-2xl">解析测试</h2>
              <p className="mt-1.5 text-xs text-muted-foreground sm:text-sm">
                输入文件名或路径，快速看解析结果和 TMDB 命中情况。
              </p>
            </div>
          </div>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto px-4 py-4 sm:px-6 sm:py-6">
          <form onSubmit={handleSubmit} className="rounded-2xl border bg-card overflow-hidden">
            <div className="px-4 pb-4 pt-4 sm:px-5">
              <div className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">
                输入样本
              </div>
              <Textarea
                value={filename}
                onChange={(e) => setFilename(e.target.value)}
                placeholder="例如：Breaking.Bad.S01E03.1080p.BluRay.HEVC.mkv"
                className="mt-3 min-h-28 resize-none"
              />
            </div>

            <div className="sticky bottom-0 flex items-center justify-between gap-3 border-t px-4 py-3 sm:px-5 bg-muted/30">
              <div className="text-xs text-muted-foreground">
                {loading ? '正在请求后端解析并查询 TMDB' : '修改输入后可再次测试'}
              </div>
              <Button type="submit" disabled={!filename.trim() || loading}>
                {loading ? (
                  <>
                    <Loader2 className="size-4 animate-spin" />
                    解析中…
                  </>
                ) : (
                  '开始解析'
                )}
              </Button>
            </div>
          </form>

          {error && (
            <div className="mt-4 rounded-xl px-4 py-3 text-sm bg-destructive/10 text-destructive border border-destructive/20">
              {error}
            </div>
          )}

          {result && (
            <section className="mt-5 rounded-2xl border bg-card px-4 py-4 sm:px-5">
              <button
                type="button"
                onClick={() => setShowDetails((v) => !v)}
                className="flex w-full items-center justify-between gap-3 text-left"
                aria-expanded={showDetails}
              >
                <div className="flex items-center gap-3">
                  <h3 className="text-sm font-semibold">详细字段</h3>
                  <Badge
                    variant={result.tmdb ? 'default' : 'secondary'}
                    className={result.tmdb ? 'bg-success text-success-foreground' : ''}
                  >
                    {result.tmdb ? '已命中 TMDB' : '未命中 TMDB'}
                  </Badge>
                </div>
                <Button variant="ghost" size="icon-sm">
                  <ChevronDown
                    className="size-4 transition-transform"
                    style={{ transform: showDetails ? 'rotate(180deg)' : 'rotate(0deg)' }}
                  />
                </Button>
              </button>
              {showDetails ? (
                <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                  {[
                    ['识别名称', result.name || '-'],
                    ['媒体类型', result.type_label || '-'],
                    ['年份', result.year || '-'],
                    ['季', result.season || '-'],
                    ['集', result.episode || '-'],
                    ['资源项', result.resource_term || '-'],
                    ['视频编码', result.video_term || '-'],
                    ['音频编码', result.audio_term || '-'],
                    ['字幕组', result.release_group || '-'],
                    ['TMDB ID', result.tmdbid ?? '-'],
                    ['豆瓣 ID', result.doubanid || '-'],
                    ['应用规则', result.apply_words?.join(', ') || '-'],
                  ].map(([label, value]) => (
                    <InfoCard key={label} label={label as string} value={value as string} />
                  ))}
                </div>
              ) : (
                <div className="mt-4 rounded-xl px-4 py-4 text-sm leading-6 text-muted-foreground bg-muted/50">
                  已收起 12 个解析字段，点击展开查看。
                </div>
              )}
            </section>
          )}

          <div className="mt-5">
            <section className="rounded-2xl border bg-card px-4 py-4 sm:px-5 sm:py-5">
              <h3 className="text-sm font-semibold">TMDB 信息</h3>
              {!result ? (
                <div className="mt-4 rounded-xl px-4 py-6 text-sm text-muted-foreground bg-muted/50">
                  等待解析结果…
                </div>
              ) : !result.tmdb ? (
                <div className="mt-4 rounded-xl px-4 py-6 text-sm text-muted-foreground bg-muted/50">
                  没有匹配到 TMDB 结果。请检查文件名，或确认已配置 TMDB API Key。
                </div>
              ) : (
                <div className="mt-4 space-y-4">
                  <div className="relative overflow-hidden rounded-xl bg-muted/50 border">
                    {result.tmdb.backdrop_url && (
                      <img
                        src={result.tmdb.backdrop_url}
                        alt=""
                        className="absolute inset-0 h-full w-full object-cover opacity-30"
                      />
                    )}
                    <div className="absolute inset-0 bg-gradient-to-b from-background/0 via-background/50 to-background" />
                    <div className="relative flex flex-col gap-4 p-4 min-[430px]:flex-row">
                      {result.tmdb.poster_url && (
                        <img
                          src={result.tmdb.poster_url}
                          alt={result.tmdb.title}
                          className="h-36 w-24 rounded-xl object-cover border sm:h-40 sm:w-28"
                        />
                      )}
                      <div className="min-w-0 flex-1">
                        <div className="text-[11px] font-semibold uppercase tracking-widest text-brand">
                          {result.tmdb.media_type_label || 'TMDB'}
                        </div>
                        <div className="mt-1 text-xl font-semibold">
                          {result.tmdb.title || '-'}
                        </div>
                        {result.tmdb.original_title &&
                          result.tmdb.original_title !== result.tmdb.title && (
                            <div className="mt-1 text-sm text-muted-foreground">
                              {result.tmdb.original_title}
                            </div>
                          )}
                        <div className="mt-3 flex flex-wrap gap-2">
                          {result.tmdb.year && (
                            <Badge variant="secondary">{result.tmdb.year}</Badge>
                          )}
                          {result.tmdb.rating && result.tmdb.rating > 0 && (
                            <Badge variant="secondary" className="text-warning border-warning">
                              ★ {result.tmdb.rating}
                            </Badge>
                          )}
                          {result.tmdb.status && (
                            <Badge variant="secondary" className="text-brand">
                              {formatStatus(result.tmdb.status)}
                            </Badge>
                          )}
                          {result.tmdb.tmdb_id && (
                            <a
                              href={`https://www.themoviedb.org/${result.tmdb.media_type === 'tv' ? 'tv' : 'movie'}/${result.tmdb.tmdb_id}`}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="inline-flex items-center gap-1 rounded-full px-2 py-1 text-xs bg-blue-500/10 text-blue-500 border border-blue-500/30 hover:bg-blue-500/20 transition-colors"
                            >
                              TMDB {result.tmdb.tmdb_id}
                              <ExternalLink className="size-3" />
                            </a>
                          )}
                        </div>
                        {result.tmdb.overview && (
                          <>
                            <div
                              className="mt-3 text-sm leading-6 text-muted-foreground"
                              style={{
                                display: '-webkit-box',
                                WebkitLineClamp: showOverview ? 'unset' : 2,
                                WebkitBoxOrient: 'vertical',
                                overflow: 'hidden',
                              }}
                            >
                              {result.tmdb.overview}
                            </div>
                            {result.tmdb.overview.length > 80 && (
                              <button
                                type="button"
                                onClick={() => setShowOverview((v) => !v)}
                                className="mt-2 text-xs font-semibold text-brand"
                              >
                                {showOverview ? '收起简介' : '展开简介'}
                              </button>
                            )}
                          </>
                        )}
                      </div>
                    </div>
                  </div>

                  <div className="grid gap-3 sm:grid-cols-2">
                    {[
                      ['首播/上映', result.tmdb.release_date || '-'],
                      ['季 / 集', `${result.season || '-'} / ${result.episode || '-'}`],
                      ['媒体类型', result.tmdb.media_type_label || '-'],
                    ].map(([label, value]) => (
                      <InfoCard key={label} label={label as string} value={value as string} />
                    ))}
                    <InfoCard label="TMDB 编号" value={result.tmdb.tmdb_id || '-'} />
                  </div>
                </div>
              )}
            </section>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
