// server.ts —— MSW 拦截服务器（node 环境，经 setupServer 挂接 axios 的 http 层）。
import { setupServer } from 'msw/node'
import { handlers } from './handlers'

export const server = setupServer(...handlers)
