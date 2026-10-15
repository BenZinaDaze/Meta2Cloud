import { useState, useRef, useEffect } from 'react'
import { User, Lock, Eye, EyeOff, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { login } from '@/api'
import Logo from '@/components/layout/Logo'

interface LoginPageProps {
  onLogin: (token: string) => void
}

export default function LoginPage({ onLogin }: LoginPageProps) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [showPwd, setShowPwd] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const userRef = useRef<HTMLInputElement>(null)

  useEffect(() => { userRef.current?.focus() }, [])

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!username.trim() || !password) return
    setLoading(true)
    setError('')
    try {
      const res = await login(username.trim(), password)
      onLogin(res.data.token)
    } catch (err: unknown) {
      const error = err as { response?: { data?: { detail?: string } } }
      setError(error?.response?.data?.detail || '登录失败，请检查用户名和密码')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="relative flex min-h-dvh items-center justify-center overflow-hidden p-4">
      {/* 环境光晕 + 网格 */}
      <div className="pointer-events-none absolute inset-0 bg-ambient" />
      <div className="pointer-events-none absolute inset-0 bg-grid-fade opacity-40" />

      <div className="relative z-10 grid w-full max-w-4xl overflow-hidden rounded-3xl border border-border/70 bg-card/50 shadow-[0_32px_96px_-24px_oklch(5%_0.01_140/0.9)] backdrop-blur-xl lg:grid-cols-[1fr_1.1fr] animate-stagger-in">
        {/* 左侧品牌区 */}
        <div className="relative hidden flex-col justify-between overflow-hidden bg-gradient-to-br from-tide/[0.08] via-transparent to-tide-2/[0.08] p-10 lg:flex">
          <div className="pointer-events-none absolute -left-20 -top-20 size-64 rounded-full bg-tide/10 blur-3xl animate-tide-float" />
          <div className="pointer-events-none absolute -bottom-24 -right-16 size-72 rounded-full bg-tide-2/10 blur-3xl" />

          <div className="relative flex items-center gap-3">
            <Logo className="size-10" />
            <div>
              <div className="text-lg font-bold tracking-tight">
                Meta<span className="text-gradient-tide">2</span>Cloud
              </div>
              <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-muted-foreground/70">
                Media Archive
              </div>
            </div>
          </div>

          <div className="relative space-y-4">
            <h2 className="bg-gradient-to-br from-foreground to-foreground/60 bg-clip-text text-3xl font-bold leading-snug tracking-tight text-transparent">
              让每一部影片
              <br />
              都归位于潮汐之中
            </h2>
            <p className="text-sm leading-7 text-muted-foreground">
              自动解析、刮削元数据并整理云端媒体库，
              <br />
              电影、剧集与订阅一气呵成。
            </p>
          </div>

          <div className="relative flex items-center gap-2 text-[11px] text-muted-foreground/60">
            <span className="h-px flex-1 bg-gradient-to-r from-tide/40 to-transparent" />
            媒体库管理系统
          </div>
        </div>

        {/* 右侧表单区 */}
        <div className="p-6 sm:p-10">
          {/* 移动端品牌 */}
          <div className="mb-8 flex items-center gap-3 lg:hidden">
            <Logo className="size-9" />
            <div>
              <div className="font-bold tracking-tight">
                Meta<span className="text-gradient-tide">2</span>Cloud
              </div>
              <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-muted-foreground/70">
                Media Archive
              </div>
            </div>
          </div>

          <div className="mb-6">
            <h1 className="text-2xl font-bold tracking-tight">欢迎回来</h1>
            <p className="mt-1.5 text-sm text-muted-foreground">
              进入你的媒体档案馆，继续管理电影、剧集和扫描配置。
            </p>
          </div>

          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="username">用户名</Label>
              <div className="relative">
                <User className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  ref={userRef}
                  id="username"
                  type="text"
                  autoComplete="username"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder="输入用户名"
                  disabled={loading}
                  className="h-10 pl-10"
                />
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="password">密码</Label>
              <div className="relative">
                <Lock className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  id="password"
                  type={showPwd ? 'text' : 'password'}
                  autoComplete="current-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="••••••••"
                  disabled={loading}
                  className="h-10 pl-10 pr-10"
                />
                <button
                  type="button"
                  onClick={() => setShowPwd((s) => !s)}
                  tabIndex={-1}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground transition-colors hover:text-foreground"
                >
                  {showPwd ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                </button>
              </div>
            </div>

            {error && (
              <div className="flex items-center gap-2 rounded-lg border border-destructive/25 bg-destructive/10 px-3 py-2.5 text-sm text-destructive animate-stagger-in">
                <span>{error}</span>
              </div>
            )}

            <Button
              type="submit"
              className="h-10 w-full rounded-xl text-[15px]"
              disabled={loading || !username.trim() || !password}
            >
              {loading ? (
                <>
                  <Loader2 className="size-4 animate-spin" />
                  登录中…
                </>
              ) : (
                '登录'
              )}
            </Button>
          </form>

          <p className="mt-6 text-center text-xs text-muted-foreground">
            账号密码在 <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-primary/90">config/config.yaml</code> 中配置
          </p>
        </div>
      </div>
    </div>
  )
}
