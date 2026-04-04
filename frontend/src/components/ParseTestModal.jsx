import { useCallback, useEffect, useState } from 'react'
import { testParse } from '../api'

export default function ParseTestModal({ onClose, initialFilename = '' }) {
  const [show, setShow] = useState(false)
  const [filename, setFilename] = useState(initialFilename)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState(null)

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

  return (
    <div
      className="fixed inset-0 z-[120] flex items-start justify-center overflow-y-auto p-4 pt-24"
      style={{
        background: 'rgba(2, 8, 18, 0.78)',
        backdropFilter: 'blur(10px)',
        opacity: show ? 1 : 0,
        transition: 'opacity 0.2s',
      }}
      onClick={(e) => e.target === e.currentTarget && handleClose()}
    >
      <div
        className="relative w-full max-w-4xl overflow-hidden rounded-[30px]"
        style={{
          background: 'linear-gradient(180deg, rgba(15, 27, 45, 0.98) 0%, rgba(11, 22, 37, 0.98) 100%)',
          border: '1px solid var(--color-border)',
          transform: show ? 'translateY(0)' : 'translateY(20px)',
          transition: 'transform 0.2s',
          boxShadow: 'var(--shadow-strong)',
        }}
      >
        <div className="border-b px-6 py-5" style={{ borderColor: 'var(--color-border)' }}>
          <div className="text-[11px] font-semibold uppercase tracking-[0.22em]" style={{ color: 'var(--color-accent-hover)' }}>
            Parser Sandbox
          </div>
          <h2 className="mt-2 text-[28px] font-bold leading-snug" style={{ color: 'var(--color-text)' }}>
            解析测试
          </h2>
          <p className="mt-2 text-sm" style={{ color: 'var(--color-muted)' }}>
            输入文件名或路径，先解析名称，再自动查询 TMDB 并展示结果。
          </p>
        </div>

        <div className="px-6 py-6">
          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <textarea
              value={filename}
              onChange={(e) => setFilename(e.target.value)}
              placeholder="例如：Breaking.Bad.S01E03.1080p.BluRay.HEVC.mkv"
              className="min-h-28 w-full rounded-[24px] px-4 py-3 text-sm outline-none transition-all"
              style={{
                background: 'rgba(255,255,255,0.03)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-text)',
                resize: 'vertical',
              }}
            />

            <div className="flex items-center gap-3">
              <button
                type="submit"
                disabled={!filename.trim() || loading}
                className="rounded-full px-5 py-2.5 text-sm font-semibold transition-all disabled:opacity-40"
                style={{
                  background: 'linear-gradient(135deg, var(--color-accent) 0%, #a56d2c 100%)',
                  color: '#fff',
                  border: 'none',
                  cursor: !filename.trim() || loading ? 'not-allowed' : 'pointer',
                }}
              >
                {loading ? '解析中…' : '开始解析'}
              </button>
              <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
                支持直接输入文件名，也支持带目录路径的字符串。
              </span>
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

          <div className="mt-5 grid gap-4 lg:grid-cols-[0.95fr_1.05fr]">
            <section
              className="rounded-[24px] px-5 py-5"
              style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)' }}
            >
              <h3 className="text-sm font-semibold" style={{ color: 'var(--color-text)' }}>关键字段</h3>
              {result ? (
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
                    <div key={label} className="rounded-2xl px-3 py-3" style={{ background: 'rgba(255,255,255,0.02)', border: '1px solid rgba(255,255,255,0.04)' }}>
                      <div className="text-[11px] font-semibold uppercase tracking-[0.18em]" style={{ color: 'var(--color-muted)' }}>
                        {label}
                      </div>
                      <div className="mt-1 break-all text-sm" style={{ color: 'var(--color-text)' }}>
                        {String(value)}
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="mt-4 rounded-[20px] px-4 py-6 text-sm" style={{ color: 'var(--color-muted)', background: 'rgba(6, 13, 24, 0.5)' }}>
                  还没有解析结果。
                </div>
              )}
            </section>

            <section
              className="rounded-[24px] px-5 py-5"
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
                    <div className="relative flex gap-4 p-4">
                      {result.tmdb.poster_url ? (
                        <img
                          src={result.tmdb.poster_url}
                          alt={result.tmdb.title}
                          className="h-40 w-28 rounded-xl object-cover"
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
                          {[
                            result.tmdb.year,
                            result.tmdb.rating ? `★ ${result.tmdb.rating}` : '',
                            result.tmdb.status,
                            result.tmdb.tmdb_id ? `TMDB ${result.tmdb.tmdb_id}` : '',
                          ].filter(Boolean).map((item) => (
                            <span
                              key={item}
                              className="rounded-full px-2 py-1 text-xs"
                              style={{ background: 'rgba(255,255,255,0.06)', color: 'var(--color-text)', border: '1px solid rgba(255,255,255,0.08)' }}
                            >
                              {item}
                            </span>
                          ))}
                        </div>
                        {result.tmdb.overview ? (
                          <div
                            className="mt-3 text-sm leading-6"
                            style={{
                              color: 'var(--color-muted)',
                              display: '-webkit-box',
                              WebkitLineClamp: 4,
                              WebkitBoxOrient: 'vertical',
                              overflow: 'hidden',
                            }}
                          >
                            {result.tmdb.overview}
                          </div>
                        ) : null}
                      </div>
                    </div>
                  </div>

                  <div className="grid gap-3 sm:grid-cols-2">
                    {[
                      ['首播/上映', result.tmdb.release_date || '-'],
                      ['季 / 集', `${result.season || '-'} / ${result.episode || '-'}`],
                      ['媒体类型', result.tmdb.media_type_label || '-'],
                      ['TMDB 编号', result.tmdb.tmdb_id || '-'],
                    ].map(([label, value]) => (
                      <div key={label} className="rounded-2xl px-3 py-3" style={{ background: 'rgba(255,255,255,0.02)', border: '1px solid rgba(255,255,255,0.04)' }}>
                        <div className="text-[11px] font-semibold uppercase tracking-[0.18em]" style={{ color: 'var(--color-muted)' }}>
                          {label}
                        </div>
                        <div className="mt-1 break-all text-sm" style={{ color: 'var(--color-text)' }}>
                          {String(value)}
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </section>
          </div>
        </div>

        <button
          onClick={handleClose}
          className="absolute right-4 top-4 flex size-9 items-center justify-center rounded-full text-sm transition-colors hover:bg-black/40"
          style={{ background: 'rgba(0,0,0,0.45)', color: 'var(--color-text)', border: '1px solid rgba(255,255,255,0.08)' }}
        >
          ✕
        </button>
      </div>
    </div>
  )
}
