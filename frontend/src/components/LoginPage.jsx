import { useState, useRef, useEffect } from 'react'
import { login } from '../api'

export default function LoginPage({ onLogin }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [showPwd, setShowPwd]   = useState(false)
  const [loading, setLoading]   = useState(false)
  const [error, setError]       = useState('')
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
      className="min-h-screen flex items-center justify-center p-4"
      style={{
        background: 'radial-gradient(ellipse 80% 60% at 50% 0%, rgba(99,102,241,0.18) 0%, transparent 70%), var(--color-bg)',
      }}
    >
      {/* 背景装饰圆 */}
      <div style={{
        position: 'fixed', inset: 0, pointerEvents: 'none', overflow: 'hidden',
      }}>
        <div style={{
          position: 'absolute', top: '-120px', left: '50%', transform: 'translateX(-50%)',
          width: 600, height: 600, borderRadius: '50%',
          background: 'radial-gradient(circle, rgba(99,102,241,0.12) 0%, transparent 70%)',
        }} />
        <div style={{
          position: 'absolute', bottom: '-80px', right: '-80px',
          width: 300, height: 300, borderRadius: '50%',
          background: 'radial-gradient(circle, rgba(139,92,246,0.10) 0%, transparent 70%)',
        }} />
      </div>

      {/* 登录卡片 */}
      <div
        style={{
          width: '100%', maxWidth: 400, position: 'relative',
          background: 'rgba(30,31,46,0.80)',
          backdropFilter: 'blur(24px)',
          border: '1px solid rgba(255,255,255,0.08)',
          borderRadius: 20,
          boxShadow: '0 32px 64px rgba(0,0,0,0.6), 0 0 0 1px rgba(99,102,241,0.1)',
          overflow: 'hidden',
        }}
      >
        {/* 顶部渐变条 */}
        <div style={{
          height: 3,
          background: 'linear-gradient(90deg, var(--color-accent) 0%, #8b5cf6 50%, #ec4899 100%)',
        }} />

        <div style={{ padding: '36px 32px 32px' }}>
          {/* Logo */}
          <div className="flex items-center gap-3 mb-8">
            <div
              className="w-10 h-10 rounded-xl flex items-center justify-center text-white font-bold text-base flex-shrink-0"
              style={{
                background: 'linear-gradient(135deg, var(--color-accent) 0%, #7c3aed 100%)',
                boxShadow: '0 4px 16px rgba(99,102,241,0.4)',
              }}
            >
              M
            </div>
            <div>
              <div className="font-bold text-lg tracking-tight" style={{ color: 'var(--color-text)' }}>
                Metadata<span style={{ color: 'var(--color-accent)' }}>2GD</span>
              </div>
              <div className="text-xs mt-0.5" style={{ color: 'var(--color-muted)' }}>
                媒体库管理系统
              </div>
            </div>
          </div>

          {/* 标题 */}
          <h1 className="text-xl font-semibold mb-1" style={{ color: 'var(--color-text)' }}>
            欢迎回来
          </h1>
          <p className="text-sm mb-7" style={{ color: 'var(--color-muted)' }}>
            请输入您的账号信息继续
          </p>

          {/* 表单 */}
          <form onSubmit={handleSubmit} className="space-y-4">
            {/* 用户名 */}
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
                    <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/>
                    <circle cx="12" cy="7" r="4"/>
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
                    background: 'rgba(255,255,255,0.04)',
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

            {/* 密码 */}
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
                    <rect x="3" y="11" width="18" height="11" rx="2" ry="2"/>
                    <path d="M7 11V7a5 5 0 0 1 10 0v4"/>
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
                    background: 'rgba(255,255,255,0.04)',
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
                      <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94"/>
                      <path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19"/>
                      <line x1="1" y1="1" x2="23" y2="23"/>
                    </svg>
                  ) : (
                    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
                      <circle cx="12" cy="12" r="3"/>
                    </svg>
                  )}
                </button>
              </div>
            </div>

            {/* 错误提示 */}
            {error && (
              <div
                className="flex items-center gap-2 text-xs px-3 py-2.5 rounded-lg"
                style={{ background: 'rgba(239,68,68,0.12)', border: '1px solid rgba(239,68,68,0.25)', color: '#f87171' }}
              >
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" className="flex-shrink-0">
                  <circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/>
                </svg>
                {error}
              </div>
            )}

            {/* 登录按钮 */}
            <button
              type="submit"
              disabled={loading || !username.trim() || !password}
              className="w-full text-sm font-semibold py-3 rounded-xl transition-all flex items-center justify-center gap-2"
              style={{
                marginTop: 8,
                background: loading || !username.trim() || !password
                  ? 'rgba(99,102,241,0.3)'
                  : 'linear-gradient(135deg, var(--color-accent) 0%, #7c3aed 100%)',
                color: loading || !username.trim() || !password ? 'rgba(255,255,255,0.4)' : '#fff',
                border: 'none',
                cursor: loading || !username.trim() || !password ? 'not-allowed' : 'pointer',
                boxShadow: loading || !username.trim() || !password
                  ? 'none'
                  : '0 4px 20px rgba(99,102,241,0.4)',
                transform: loading || !username.trim() || !password ? 'none' : undefined,
              }}
            >
              {loading ? (
                <>
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5"
                    style={{ animation: 'spin 0.8s linear infinite' }}>
                    <polyline points="23 4 23 10 17 10"/>
                    <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/>
                  </svg>
                  登录中…
                </>
              ) : '登录'}
            </button>
          </form>

          {/* 底部提示 */}
          <p className="text-center text-xs mt-6" style={{ color: 'var(--color-muted)' }}>
            账号密码在 <code style={{ color: 'var(--color-accent)', fontSize: 11 }}>config/config.yaml</code> 中配置
          </p>
        </div>
      </div>
    </div>
  )
}
