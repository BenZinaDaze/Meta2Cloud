import { useCallback, useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { testParse } from '../api'

const STATUS_MAP = {
  'Returning Series': '连载中',
  'Ended': '已完结',
  'Canceled': '已取消',
  'In Production': '制作中',
  'Planned': '计划中',
  'Pilot': '试播',
  'In Limbo': '播出未定',
}

function formatStatus(status) {
  if (!status) return status
  return STATUS_MAP[status] ?? status
}

function InfoCard({ label, value }) {
  return (
    <div className="rounded-2xl px-3 py-3" style={{ background: 'rgba(255,255,255,0.02)', border: '1px solid rgba(255,255,255,0.04)' }}>
      <div className="text-[11px] font-semibold uppercase tracking-[0.18em]" style={{ color: 'var(--color-muted)' }}>
        {label}
      </div>
      <div className="mt-1 break-all text-sm" style={{ color: 'var(--color-text)' }}>
        {String(value)}
      </div>
    </div>
  )
}

export default function ParseTestModal({ onClose, initialFilename = '' }) {
  const [show, setShow] = useState(false)
  const [filename, setFilename] = useState(initialFilename)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState(null)
  const [showDetails, setShowDetails] = useState(false)
  const [showOverview, setShowOverview] = useState(false)

  const handleClose = useCallback(() => {
    setShow(false)
    setTimeout(onClose, 200)
  }, [onClose])

  useEffect(() => {
    requestAnimationFrame(() => setShow(true))
    const onKey = (e) => e.key === 'Escape' && handleClose()
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [handleClose])

  useEffect(() => {
    if (initialFilename) {
      setFilename(initialFilename)
    }
  }, [initialFilename])

  async function handleSubmit(e) {
    e.preventDefault()
    const value = filename.trim()
    if (!value || loading) return

    setLoading(true)
    setError('')
    try {
      const res = await testParse(value)
      setResult(res.data)
      setShowDetails(false)
      setShowOverview(false)
    } catch (err) {
      setResult(null)
      setError(err?.response?.data?.detail || err.message || '解析失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (!initialFilename?.trim()) return

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
          setShowDetails(false)
          setShowOverview(false)
        }
      } catch (err) {
        if (!cancelled) {
          setResult(null)
          setError(err?.response?.data?.detail || err.message || '解析失败')
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    runInitialParse()
    return () => { cancelled = true }
  }, [initialFilename])

  return createPortal(
    <div
      className="fixed inset-0 z-[140] flex items-end justify-center overflow-y-auto p-0 sm:items-start sm:p-4 sm:pt-24"
      style={{
        background: 'rgba(2, 8, 18, 0.78)',
        backdropFilter: 'blur(10px)',
        opacity: show ? 1 : 0,
        transition: 'opacity 0.2s',
      }}
      onClick={(e) => e.target === e.currentTarget && handleClose()}
    >
      <div
        className="relative flex h-[100dvh] w-full flex-col overflow-hidden rounded-none sm:h-auto sm:max-w-4xl sm:rounded-[30px]"
        style={{
          background: 'linear-gradient(180deg, rgba(15, 27, 45, 0.98) 0%, rgba(11, 22, 37, 0.98) 100%)',
          border: '1px solid var(--color-border)',
          transform: show ? 'translateY(0)' : 'translateY(20px)',
          transition: 'transform 0.2s',
          boxShadow: 'var(--shadow-strong)',
          maxHeight: '100dvh',
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <div
          className="sticky top-0 z-10 border-b px-4 pb-4 pt-[calc(env(safe-area-inset-top)+0.75rem)] sm:px-6 sm:py-5"
          style={{ borderColor: 'var(--color-border)', background: 'linear-gradient(180deg, rgba(15, 27, 45, 0.99) 0%, rgba(11, 22, 37, 0.98) 100%)' }}
        >
          <div className="mx-auto mb-3 h-1.5 w-12 rounded-full sm:hidden" style={{ background: 'rgba(255,255,255,0.14)' }} />
          <div className="flex items-start justify-between gap-4">
            <div>
              <div className="text-[11px] font-semibold uppercase tracking-[0.22em]" style={{ color: 'var(--color-accent-hover)' }}>
                Parser Sandbox
              </div>
              <h2 className="mt-1.5 text-[22px] font-bold leading-snug sm:mt-2 sm:text-[28px]" style={{ color: 'var(--color-text)' }}>
                解析测试
              </h2>
              <p className="mt-1.5 text-xs leading-5 sm:mt-2 sm:text-sm" style={{ color: 'var(--color-muted)' }}>
                输入文件名或路径，快速看解析结果和 TMDB 命中情况。
              </p>
            </div>
            <button
              type="button"
              onClick={handleClose}
              className="flex size-11 items-center justify-center rounded-2xl transition-all duration-150"
              style={{ background: 'rgba(255,255,255,0.04)', border: '1px solid var(--color-border)', color: 'var(--color-text)' }}
              aria-label="关闭解析测试"
            >
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round">
                <path d="M6 6l12 12" />
                <path d="M18 6L6 18" />
              </svg>
            </button>
          </div>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4 sm:px-6 sm:py-6" style={{ paddingBottom: 'calc(env(safe-area-inset-bottom) + 1rem)' }}>
          <form
            onSubmit={handleSubmit}
            className="overflow-hidden rounded-[24px]"
            style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)' }}
          >
            <div className="px-4 pb-4 pt-4 sm:px-5">
              <div className="text-xs font-semibold uppercase tracking-[0.18em]" style={{ color: 'var(--color-muted)' }}>
                输入样本
              </div>
              <textarea
                value={filename}
                onChange={(e) => setFilename(e.target.value)}
                placeholder="例如：Breaking.Bad.S01E03.1080p.BluRay.HEVC.mkv"
                className="mt-3 min-h-28 w-full rounded-[20px] px-4 py-3 text-sm outline-none transition-all"
                style={{
                  background: 'rgba(255,255,255,0.03)',
                  border: '1px solid rgba(255,255,255,0.06)',
                  color: 'var(--color-text)',
                  resize: 'none',
                }}
              />
            </div>

            <div
              className="sticky bottom-0 flex items-center justify-between gap-3 border-t px-4 py-3 sm:px-5"
              style={{
                borderColor: 'rgba(255,255,255,0.06)',
                background: 'linear-gradient(180deg, rgba(11, 22, 37, 0.72) 0%, rgba(11, 22, 37, 0.96) 100%)',
                backdropFilter: 'blur(8px)',
              }}
            >
              <div className="text-xs leading-5" style={{ color: 'var(--color-muted)' }}>
                {loading ? '正在请求后端解析并查询 TMDB' : '修改输入后可再次测试'}
              </div>
              <button
                type="submit"
                disabled={!filename.trim() || loading}
                className="min-h-11 shrink-0 rounded-full px-5 py-2.5 text-sm font-semibold transition-all disabled:opacity-40"
                style={{
                  background: 'linear-gradient(135deg, var(--color-accent) 0%, #a56d2c 100%)',
                  color: '#fff',
                  border: 'none',
                  cursor: !filename.trim() || loading ? 'not-allowed' : 'pointer',
                }}
              >
                {loading ? '解析中…' : '开始解析'}
              </button>
            </div>
          </form>

          {error ? (
            <div
              className="mt-4 rounded-[22px] px-4 py-3 text-sm"
              style={{ background: 'rgba(239,125,117,0.08)', border: '1px solid rgba(239,125,117,0.2)', color: 'var(--color-danger)' }}
            >
              {error}
            </div>
          ) : null}

          {result ? (
            <section
              className="mt-5 rounded-[24px] px-4 py-4 sm:px-5"
              style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)' }}
            >
              <div className="flex items-center justify-between gap-3">
                <div>
                  <h3 className="text-sm font-semibold" style={{ color: 'var(--color-text)' }}>解析摘要</h3>
                  <p className="mt-1 text-xs leading-5" style={{ color: 'var(--color-muted)' }}>
                    先看这几个字段，确认识别方向对不对。
                  </p>
                </div>
                <span
                  className="rounded-full px-2.5 py-1 text-xs font-semibold"
                  style={{
                    background: result.tmdb ? 'rgba(74, 201, 126, 0.12)' : 'rgba(255,255,255,0.04)',
                    color: result.tmdb ? 'var(--color-success)' : 'var(--color-muted)',
                    border: result.tmdb ? '1px solid rgba(74, 201, 126, 0.2)' : '1px solid rgba(255,255,255,0.08)',
                  }}
                >
                  {result.tmdb ? '已命中 TMDB' : '未命中 TMDB'}
                </span>
              </div>
              <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                <InfoCard label="识别名称" value={result.name || '-'} />
                <InfoCard label="媒体类型" value={result.type_label || '-'} />
                <InfoCard label="年份" value={result.year || '-'} />
                <InfoCard label="季 / 集" value={`${result.season || '-'} / ${result.episode || '-'}`} />
                <InfoCard label="资源项" value={result.resource_term || '-'} />
                <InfoCard label="TMDB ID" value={result.tmdbid ?? '-'} />
              </div>
            </section>
          ) : null}

          <div className="mt-5 grid gap-4 lg:grid-cols-[0.95fr_1.05fr]">
            <section
              className="rounded-[24px] px-4 py-4 sm:px-5 sm:py-5"
              style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)' }}
            >
              <button
                type="button"
                onClick={() => setShowDetails((value) => !value)}
                className="flex w-full items-center justify-between gap-3 text-left"
                aria-expanded={showDetails}
              >
                <div>
                  <h3 className="text-sm font-semibold" style={{ color: 'var(--color-text)' }}>详细字段</h3>
                  <p className="mt-1 text-xs leading-5" style={{ color: 'var(--color-muted)' }}>
                    手机端默认收起，确认摘要没问题后再看完整解析字段。
                  </p>
                </div>
                <span
                  className="flex size-9 shrink-0 items-center justify-center rounded-full"
                  style={{ background: 'rgba(255,255,255,0.04)', border: '1px solid rgba(255,255,255,0.06)', color: 'var(--color-text)' }}
                >
                  <svg
                    width="16"
                    height="16"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="2"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    style={{ transform: showDetails ? 'rotate(180deg)' : 'rotate(0deg)', transition: 'transform 0.18s ease' }}
                  >
                    <polyline points="6 9 12 15 18 9" />
                  </svg>
                </span>
              </button>
              {result ? (
                showDetails ? (
                  <div className="mt-4 grid gap-3 sm:grid-cols-2">
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
                      <InfoCard key={label} label={label} value={value} />
                    ))}
                  </div>
                ) : (
                  <div className="mt-4 rounded-[20px] px-4 py-4 text-sm leading-6" style={{ color: 'var(--color-muted)', background: 'rgba(6, 13, 24, 0.5)' }}>
                    已收起 12 个解析字段，按需展开查看完整结果。
                  </div>
                )
              ) : (
                <div className="mt-4 rounded-[20px] px-4 py-6 text-sm" style={{ color: 'var(--color-muted)', background: 'rgba(6, 13, 24, 0.5)' }}>
                  还没有解析结果。
                </div>
              )}
            </section>

            <section
              className="rounded-[24px] px-4 py-4 sm:px-5 sm:py-5"
              style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)' }}
            >
              <h3 className="text-sm font-semibold" style={{ color: 'var(--color-text)' }}>TMDB 信息</h3>
              {!result ? (
                <div className="mt-4 rounded-[20px] px-4 py-6 text-sm" style={{ color: 'var(--color-muted)', background: 'rgba(6, 13, 24, 0.5)' }}>
                  等待解析结果…
                </div>
              ) : !result.tmdb ? (
                <div className="mt-4 rounded-[20px] px-4 py-6 text-sm" style={{ color: 'var(--color-muted)', background: 'rgba(6, 13, 24, 0.5)' }}>
                  没有匹配到 TMDB 结果。请检查文件名，或确认已配置 TMDB API Key。
                </div>
              ) : (
                <div className="mt-4 space-y-4">
                    <div
                      className="relative overflow-hidden rounded-[22px]"
                    style={{
                      background: 'rgba(6, 13, 24, 0.72)',
                      border: '1px solid rgba(255,255,255,0.05)',
                    }}
                  >
                    {result.tmdb.backdrop_url ? (
                      <img
                        src={result.tmdb.backdrop_url}
                        alt=""
                        className="absolute inset-0 h-full w-full object-cover"
                        style={{ opacity: 0.3 }}
                      />
                    ) : null}
                    <div
                      className="absolute inset-0"
                      style={{ background: 'linear-gradient(180deg, rgba(7, 17, 31, 0.12) 0%, rgba(7, 17, 31, 0.94) 100%)' }}
                    />
                    <div className="relative flex flex-col gap-4 p-4 min-[430px]:flex-row">
                      {result.tmdb.poster_url ? (
                        <img
                          src={result.tmdb.poster_url}
                          alt={result.tmdb.title}
                          className="h-36 w-24 rounded-xl object-cover sm:h-40 sm:w-28"
                          style={{ border: '1px solid rgba(255,255,255,0.08)' }}
                        />
                      ) : null}
                      <div className="min-w-0 flex-1">
                        <div className="text-[11px] font-semibold uppercase tracking-[0.18em]" style={{ color: 'var(--color-accent-hover)' }}>
                          {result.tmdb.media_type_label || 'TMDB'}
                        </div>
                        <div className="mt-1 text-xl font-semibold" style={{ color: 'var(--color-text)' }}>
                          {result.tmdb.title || '-'}
                        </div>
                        {result.tmdb.original_title && result.tmdb.original_title !== result.tmdb.title ? (
                          <div className="mt-1 text-sm" style={{ color: 'var(--color-muted)' }}>
                            {result.tmdb.original_title}
                          </div>
                        ) : null}
                        <div className="mt-3 flex flex-wrap gap-2">
                          {result.tmdb.year && (
                            <span className="rounded-full px-2 py-1 text-xs"
                              style={{ background: 'rgba(255,255,255,0.04)', color: 'var(--color-muted)', border: '1px solid rgba(255,255,255,0.08)' }}>
                              {result.tmdb.year}
                            </span>
                          )}
                          {result.tmdb.rating > 0 && (
                            <span className="rounded-full px-2 py-1 text-xs"
                              style={{ background: 'rgba(255,255,255,0.04)', color: 'var(--color-warning)', border: '1px solid rgba(255,255,255,0.08)' }}>
                              ★ {result.tmdb.rating}
                            </span>
                          )}
                          {result.tmdb.status && (
                            <span className="rounded-full px-2 py-1 text-xs"
                              style={{ background: 'rgba(255,255,255,0.04)', color: 'var(--color-accent-hover)', border: '1px solid rgba(255,255,255,0.08)' }}>
                              {formatStatus(result.tmdb.status)}
                            </span>
                          )}
                          {result.tmdb.tmdb_id && (
                            <a
                              href={`https://www.themoviedb.org/${result.tmdb.media_type === 'tv' ? 'tv' : 'movie'}/${result.tmdb.tmdb_id}`}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="rounded-full px-2 py-1 text-xs transition-all duration-150"
                              style={{
                                background: 'rgba(1, 180, 228, 0.08)',
                                color: '#01b4e4',
                                border: '1px solid rgba(1, 180, 228, 0.35)',
                                textDecoration: 'none',
                                cursor: 'pointer',
                              }}
                              onMouseEnter={e => {
                                e.currentTarget.style.background = 'rgba(1, 180, 228, 0.18)'
                                e.currentTarget.style.borderColor = 'rgba(1, 180, 228, 0.65)'
                              }}
                              onMouseLeave={e => {
                                e.currentTarget.style.background = 'rgba(1, 180, 228, 0.08)'
                                e.currentTarget.style.borderColor = 'rgba(1, 180, 228, 0.35)'
                              }}
                            >
                              TMDB {result.tmdb.tmdb_id}
                            </a>
                          )}
                        </div>
                        {result.tmdb.overview ? (
                          <>
                            <div
                              className="mt-3 text-sm leading-6"
                              style={{
                                color: 'var(--color-muted)',
                                display: '-webkit-box',
                                WebkitLineClamp: showOverview ? 'unset' : 2,
                                WebkitBoxOrient: 'vertical',
                                overflow: 'hidden',
                              }}
                            >
                              {result.tmdb.overview}
                            </div>
                            {result.tmdb.overview.length > 80 ? (
                              <button
                                type="button"
                                onClick={() => setShowOverview((value) => !value)}
                                className="mt-2 text-xs font-semibold"
                                style={{ color: 'var(--color-accent-hover)' }}
                              >
                                {showOverview ? '收起简介' : '展开简介'}
                              </button>
                            ) : null}
                          </>
                        ) : null}
                      </div>
                    </div>
                  </div>

                  <div className="grid gap-3 sm:grid-cols-2">
                    {[
                      ['首播/上映', result.tmdb.release_date || '-'],
                      ['季 / 集', `${result.season || '-'} / ${result.episode || '-'}`],
                      ['媒体类型', result.tmdb.media_type_label || '-'],
                    ].map(([label, value]) => (
                      <InfoCard key={label} label={label} value={value} />
                    ))}
                    <InfoCard label="TMDB 编号" value={result.tmdb.tmdb_id || '-'} />
                  </div>
                </div>
              )}
            </section>
          </div>
        </div>
      </div>
    </div>,
    document.body
  )
}
