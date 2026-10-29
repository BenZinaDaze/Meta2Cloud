// api.interceptor.test.ts —— axios 实例拦截器行为：token 注入与 401 登出回调。
// 经 MSW 真实走 http 层（非 mock @/api 模块），拦截器与请求头一并被覆盖；
// 请求用 getMe（GET /auth/me，经 api 实例发出）。
import { describe, it, expect, vi, afterEach } from 'vitest'
import { http, HttpResponse } from 'msw'
import { getMe, setUnauthorizedHandler } from '@/api'
import { server } from '@/test/mocks/server'

describe('api 拦截器', () => {
  afterEach(() => {
    setUnauthorizedHandler(() => {})
  })

  it('localStorage 有 token 时自动附加 Bearer 头', async () => {
    localStorage.setItem('auth_token', 'tok-123')
    let authorization = ''
    server.use(
      http.get('/api/auth/me', ({ request }) => {
        authorization = request.headers.get('Authorization') ?? ''
        return HttpResponse.json({ username: 'tester' })
      }),
    )
    const res = await getMe()
    expect(res.data.username).toBe('tester')
    expect(authorization).toBe('Bearer tok-123')
  })

  it('无 token 时不带 Authorization 头', async () => {
    let authorization: string | null = ''
    server.use(
      http.get('/api/auth/me', ({ request }) => {
        authorization = request.headers.get('Authorization')
        return HttpResponse.json({ username: 'tester' })
      }),
    )
    await getMe()
    expect(authorization).toBeNull()
  })

  it('401 响应触发未授权回调（登出）', async () => {
    const onUnauthorized = vi.fn()
    setUnauthorizedHandler(onUnauthorized)
    server.use(http.get('/api/auth/me', () => new HttpResponse(null, { status: 401 })))
    await expect(getMe()).rejects.toMatchObject({ response: { status: 401 } })
    expect(onUnauthorized).toHaveBeenCalledTimes(1)
  })
})
