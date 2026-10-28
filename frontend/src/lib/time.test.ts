// time.test.ts —— relativeTime/formatDateTime 展示格式化回归测试。
import { describe, it, expect, vi, afterEach } from 'vitest'
import { relativeTime, formatDateTime } from '@/lib/time'

afterEach(() => {
  vi.useRealTimers()
})

describe('relativeTime', () => {
  it('空输入返回 fallback', () => {
    expect(relativeTime(undefined)).toBe('未知')
    expect(relativeTime('')).toBe('未知')
    expect(relativeTime(undefined, { fallback: '从未' })).toBe('从未')
  })

  it('1 分钟内返回「刚刚」', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-01T12:00:00Z'))
    expect(relativeTime('2026-01-01T11:59:30Z')).toBe('刚刚')
  })

  it('分钟/小时/天区间正确换算', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-01T12:00:00Z'))
    expect(relativeTime('2026-01-01T11:30:00Z')).toBe('30 分钟前')
    expect(relativeTime('2026-01-01T10:00:00Z')).toBe('2 小时前')
    expect(relativeTime('2026-01-01T00:00:00Z')).toBe('12 小时前')
    expect(relativeTime('2025-12-30T12:00:00Z')).toBe('2 天前')
  })

  it('超过 dayThreshold（默认 7 天）回退为日期展示', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-01T12:00:00Z'))
    const out = relativeTime('2025-12-01T12:00:00Z')
    expect(out).not.toContain('天前')
    expect(out).toMatch(/2025/)
  })

  it('自定义 dayThreshold 生效', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-01T12:00:00Z'))
    expect(relativeTime('2025-12-01T12:00:00Z', { dayThreshold: 400 })).toContain('天前')
  })
})

describe('formatDateTime', () => {
  it('空输入返回 fallback（默认 -）', () => {
    expect(formatDateTime(undefined)).toBe('-')
    expect(formatDateTime(null)).toBe('-')
    expect(formatDateTime('')).toBe('-')
    expect(formatDateTime(null, { fallback: '无记录' })).toBe('无记录')
  })

  it('秒级时间戳自动升级为毫秒', () => {
    // 2026-01-01T00:00:00Z = 1767225600 秒
    const out = formatDateTime(1767225600)
    expect(out).toContain('2026')
    expect(out).toContain('01/01')
  })

  it('毫秒时间戳直接使用', () => {
    const out = formatDateTime(1767225600000)
    expect(out).toContain('2026')
  })

  it('非法字符串原样返回', () => {
    expect(formatDateTime('不是日期')).toBe('不是日期')
  })

  it('合法 ISO 字符串格式化为年月日时分', () => {
    const out = formatDateTime('2026-01-01T08:30:00Z')
    expect(out).toContain('2026')
  })
})
