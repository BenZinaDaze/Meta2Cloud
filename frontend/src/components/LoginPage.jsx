import { useState, useRef, useEffect } from 'react'
import { login } from '../api'
import BrandMark from './BrandMark'

export default function LoginPage({ onLogin }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [showPwd, setShowPwd] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const userRef = useRef(null)

  useEffect(() => { userRef.current?.focus() }, [])

  async function handleSubmit(e) {
    e.preventDefault()
    if (!username.trim() || !password) return
    setLoading(true); setError('')
    try {
      const res = await login(username.trim(), password)
      onLogin(res.data.token)
    } catch (err) {
      setError(err?.response?.data?.detail || '登录失败，请检查用户名和密码')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div
      className="flex min-h-dvh items-center justify-center p-4"
      style={{
        background:
          'radial-gradient(circle at 20% 15%, rgba(64,111,178,0.22) 0%, transparent 28%), radial-gradient(circle at 85% 12%, rgba(200,146,77,0.16) 0%, transparent 24%), linear-gradient(180deg, rgba(8,19,33,1) 0%, rgba(7,17,31,1) 100%)',
      }}
    >
      <div style={{
        position: 'fixed', inset: 0, pointerEvents: 'none', overflow: 'hidden',
      }}>
        <div style={{
          position: 'absolute', top: '-120px', left: '50%', transform: 'translateX(-50%)',
          width: 600, height: 600, borderRadius: '50%',
          background: 'radial-gradient(circle, rgba(64,111,178,0.16) 0%, transparent 70%)',
        }} />
        <div style={{
          position: 'absolute', bottom: '-80px', right: '-80px',
          width: 300, height: 300, borderRadius: '50%',
          background: 'radial-gradient(circle, rgba(200,146,77,0.10) 0%, transparent 70%)',
        }} />
      </div>

      <div
        style={{
          width: '100%', maxWidth: 'min(35%, 720px)', minWidth: '320px', position: 'relative',
          background: 'linear-gradient(180deg, rgba(17,31,50,0.9) 0%, rgba(10,20,34,0.96) 100%)',
          backdropFilter: 'blur(24px)',
          border: '1px solid rgba(255,255,255,0.08)',
          borderRadius: 28,
          boxShadow: 'var(--shadow-strong)',
          overflow: 'hidden',
        }}
      >
        <div style={{
          height: 3,
          background: 'linear-gradient(90deg, rgba(64,111,178,1) 0%, var(--color-accent) 100%)',
        }} />

        <div style={{ padding: '40px 36px 34px' }}>
          <div className="mb-9 flex items-center gap-4">
            <div
              className="flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-2xl"
              style={{
                boxShadow: '0 12px 28px rgba(200,146,77,0.24)',
              }}
            >
              <BrandMark className="h-11 w-11" compact />
            </div>
            <div>
              <div className="text-[10px] font-semibold uppercase tracking-[0.24em]" style={{ color: 'var(--color-muted)' }}>
                Archive access
              </div>
              <div className="font-bold text-lg tracking-tight" style={{ color: 'var(--color-text)' }}>
                Meta<span style={{ color: 'var(--color-accent)' }}>2Cloud</span>
              </div>
              <div className="text-xs mt-0.5" style={{ color: 'var(--color-muted)' }}>
                媒体库管理系统
              </div>
            </div>
          </div>

          <h1 className="mb-2 text-[30px] font-semibold leading-tight" style={{ color: 'var(--color-text)' }}>
            欢迎回来
          </h1>
          <p className="mb-8 text-sm leading-7" style={{ color: 'var(--color-muted)' }}>
            进入你的媒体档案馆，继续管理电影、剧集和扫描配置。
          </p>

          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="block text-xs font-medium mb-1.5" style={{ color: 'var(--color-muted)' }}>
                用户名
              </label>
              <div className="relative">
                <span style={{
                  position: 'absolute', left: 12, top: '50%', transform: 'translateY(-50%)',
                  color: 'var(--color-muted)',
                }}>
                  <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                    <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2" />
                    <circle cx="12" cy="7" r="4" />
                  </svg>
                </span>
                <input
                  ref={userRef}
                  type="text"
                  autoComplete="username"
                  value={username}
                  onChange={e => setUsername(e.target.value)}
                  placeholder="admin"
                  disabled={loading}
                  className="w-full text-sm rounded-xl outline-none transition-all"
                  style={{
                    background: 'rgba(255,255,255,0.035)',
                    border: '1px solid rgba(255,255,255,0.1)',
                    color: 'var(--color-text)',
                    padding: '11px 12px 11px 38px',
                    fontFamily: 'inherit',
                  }}
                  onFocus={e => e.target.style.borderColor = 'rgba(99,102,241,0.6)'}
                  onBlur={e => e.target.style.borderColor = 'rgba(255,255,255,0.1)'}
                />
              </div>
            </div>

            <div>
              <label className="block text-xs font-medium mb-1.5" style={{ color: 'var(--color-muted)' }}>
                密码
              </label>
              <div className="relative">
                <span style={{
                  position: 'absolute', left: 12, top: '50%', transform: 'translateY(-50%)',
                  color: 'var(--color-muted)',
                }}>
                  <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                    <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
                    <path d="M7 11V7a5 5 0 0 1 10 0v4" />
                  </svg>
                </span>
                <input
                  type={showPwd ? 'text' : 'password'}
                  autoComplete="current-password"
                  value={password}
                  onChange={e => setPassword(e.target.value)}
                  placeholder="••••••••"
                  disabled={loading}
                  className="w-full text-sm rounded-xl outline-none transition-all"
                  style={{
                    background: 'rgba(255,255,255,0.035)',
                    border: '1px solid rgba(255,255,255,0.1)',
                    color: 'var(--color-text)',
                    padding: '11px 40px 11px 38px',
                    fontFamily: 'inherit',
                  }}
                  onFocus={e => e.target.style.borderColor = 'rgba(99,102,241,0.6)'}
                  onBlur={e => e.target.style.borderColor = 'rgba(255,255,255,0.1)'}
                />
                <button
                  type="button"
                  onClick={() => setShowPwd(s => !s)}
                  tabIndex={-1}
                  style={{
                    position: 'absolute', right: 12, top: '50%', transform: 'translateY(-50%)',
                    color: 'var(--color-muted)', background: 'none', border: 'none',
                    cursor: 'pointer', padding: 2,
                  }}
                >
                  {showPwd ? (
                    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94" />
                      <path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19" />
                      <line x1="1" y1="1" x2="23" y2="23" />
                    </svg>
                  ) : (
                    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
                      <circle cx="12" cy="12" r="3" />
                    </svg>
                  )}
                </button>
              </div>
            </div>

            {error && (
              <div
                className="flex items-center gap-2 text-xs px-3 py-2.5 rounded-lg"
                style={{ background: 'rgba(239,68,68,0.12)', border: '1px solid rgba(239,68,68,0.25)', color: '#f87171' }}
              >
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" className="flex-shrink-0">
                  <circle cx="12" cy="12" r="10" /><line x1="12" y1="8" x2="12" y2="12" /><line x1="12" y1="16" x2="12.01" y2="16" />
                </svg>
                {error}
              </div>
            )}

            <button
              type="submit"
              disabled={loading || !username.trim() || !password}
              className="w-full text-sm font-semibold py-3 rounded-xl transition-all flex items-center justify-center gap-2"
              style={{
                marginTop: 8,
                background: loading || !username.trim() || !password
                  ? 'rgba(200,146,77,0.2)'
                  : 'linear-gradient(135deg, var(--color-accent) 0%, #a56d2c 100%)',
                color: loading || !username.trim() || !password ? 'rgba(255,255,255,0.4)' : '#fff',
                border: 'none',
                cursor: loading || !username.trim() || !password ? 'not-allowed' : 'pointer',
                boxShadow: loading || !username.trim() || !password
                  ? 'none'
                  : '0 16px 32px rgba(200,146,77,0.25)',
                transform: loading || !username.trim() || !password ? 'none' : undefined,
              }}
            >
              {loading ? (
                <>
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5"
                    style={{ animation: 'spin 0.8s linear infinite' }}>
                    <polyline points="23 4 23 10 17 10" />
                    <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10" />
                  </svg>
                  登录中…
                </>
              ) : '登录'}
            </button>
          </form>

          <p className="text-center text-xs mt-6" style={{ color: 'var(--color-muted)' }}>
            账号密码在 <code style={{ color: 'var(--color-accent)', fontSize: 11 }}>config/config.yaml</code> 中配置
          </p>
        </div>
      </div>
    </div>
  )
}
