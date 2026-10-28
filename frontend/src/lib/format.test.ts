// format.test.ts —— formatBytes 字节展示回归测试。
import { describe, it, expect } from 'vitest'
import { formatBytes } from '@/lib/format'

describe('formatBytes', () => {
  it('非法输入（负数/非有限值/undefined/null）返回 -', () => {
    expect(formatBytes(-1)).toBe('-')
    expect(formatBytes(0)).toBe('-')
    expect(formatBytes(Number.NaN)).toBe('-')
    expect(formatBytes(Number.POSITIVE_INFINITY)).toBe('-')
    expect(formatBytes(undefined)).toBe('-')
    expect(formatBytes(null)).toBe('-')
  })

  it('字符串数字可用（后端可能返回字符串）', () => {
    expect(formatBytes('1024')).toBe('1.0 KB')
  })

  it('字节与各级单位换算', () => {
    expect(formatBytes(1)).toBe('1.0 B')
    expect(formatBytes(512)).toBe('512 B') // ≥100 省略小数
    expect(formatBytes(1024)).toBe('1.0 KB')
    expect(formatBytes(1536)).toBe('1.5 KB')
    expect(formatBytes(1024 * 1024)).toBe('1.0 MB')
    expect(formatBytes(1024 ** 3)).toBe('1.0 GB')
    expect(formatBytes(1024 ** 4)).toBe('1.0 TB')
  })

  it('≥100 时省略小数位', () => {
    expect(formatBytes(100 * 1024)).toBe('100 KB')
    expect(formatBytes(2 * 1024 * 1024)).toBe('2.0 MB')
  })

  it('超过 TB 封顶在 TB 单位', () => {
    expect(formatBytes(1024 ** 5)).toBe('1024 TB') // 封顶 TB 且 ≥100 省略小数
  })
})
