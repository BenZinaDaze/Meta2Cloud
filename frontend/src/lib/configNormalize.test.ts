// configNormalize.test.ts —— normalizeConfig 配置规范化回归测试。
// 这是配置保存链路的前端心脏：补默认值语义与后端 config.Default() 对齐，
// 回归会直接把用户的定时规则/开关写成空值。
import { describe, it, expect } from 'vitest'
import { normalizeConfig } from '@/lib/configNormalize'

describe('normalizeConfig', () => {
  it('空对象补全全部默认值', () => {
    const out = normalizeConfig({})
    expect(out.webui).toMatchObject({ token_expire_hours: 24, log_retention_days: 7 })
    expect(out.tmdb).toMatchObject({ timeout: 10, image_base_url: '' })
    expect(out.u115).toMatchObject({ auto_organize_cron: '* * * * *', auto_organize_stable_seconds: 30, qps: 3 })
    expect(out.rss).toMatchObject({ poll_cron: '*/5 * * * *' })
    expect(out.telegram).toMatchObject({ debounce_seconds: 60 })
    expect(out.mediavault).toMatchObject({ scheme: 'http', timeout: 10 })
    expect(out.strm).toMatchObject({
      enabled: false, fetch_sidecar: true, refresh_plex: false, sync_cron: '',
      media_ext: expect.arrayContaining(['mp4', 'mkv']),
      sidecar_ext: expect.arrayContaining(['srt', 'ass']),
      mappings: [],
    })
    expect(out.trakt).toMatchObject({
      enabled: false, sync_cron: '*/15 * * * *', session_poll_seconds: 45,
      sse_enabled: true, sync_to_plex: true, sync_to_trakt: true,
    })
    expect(out.plex).toMatchObject({ timeout: 10 })
  })

  it('已有值不被覆盖（含 0/空串等合法假值之外的场景）', () => {
    const out = normalizeConfig({
      webui: { token_expire_hours: 48 },
      u115: { qps: 5, auto_organize_stable_seconds: 0 }, // 0 是合法值，不得被 30 覆盖
      trakt: { sync_cron: '0 8 * * *' },
    })
    expect((out.webui as Record<string, unknown>).token_expire_hours).toBe(48)
    expect((out.u115 as Record<string, unknown>).qps).toBe(5)
    expect((out.u115 as Record<string, unknown>).auto_organize_stable_seconds).toBe(0)
    expect((out.trakt as Record<string, unknown>).sync_cron).toBe('0 8 * * *')
  })

  it('空串数值/定时字段补默认（空串视为未设置）', () => {
    const out = normalizeConfig({
      webui: { token_expire_hours: '' },
      u115: { auto_organize_stable_seconds: '' },
      trakt: { session_poll_seconds: '' },
      telegram: { debounce_seconds: '' },
    })
    expect((out.webui as Record<string, unknown>).token_expire_hours).toBe(24)
    expect((out.u115 as Record<string, unknown>).auto_organize_stable_seconds).toBe(30)
    expect((out.trakt as Record<string, unknown>).session_poll_seconds).toBe(45)
    expect((out.telegram as Record<string, unknown>).debounce_seconds).toBe(60)
  })

  it('poll_cron/sync_cron 的空串保留（空串=不定时，与数值字段语义不同）', () => {
    const out = normalizeConfig({
      rss: { poll_cron: '' },
      trakt: { sync_cron: '' },
      strm: { sync_cron: '' },
    })
    expect((out.rss as Record<string, unknown>).poll_cron).toBe('')
    expect((out.trakt as Record<string, unknown>).sync_cron).toBe('')
    expect((out.strm as Record<string, unknown>).sync_cron).toBe('')
  })

  it('strm 媒体/字幕扩展名：非数组或空数组补默认，已有列表保留', () => {
    const defaults = normalizeConfig({ strm: {} })
    expect(((defaults.strm as Record<string, unknown>).media_ext as string[]).length).toBeGreaterThan(5)
    expect((defaults.strm as Record<string, unknown>).sidecar_ext).toEqual(['srt', 'ass', 'ssa'])

    const kept = normalizeConfig({ strm: { media_ext: ['mkv'], sidecar_ext: ['pgs'] } })
    expect((kept.strm as Record<string, unknown>).media_ext).toEqual(['mkv'])
    expect((kept.strm as Record<string, unknown>).sidecar_ext).toEqual(['pgs'])
  })

  it('strm.mappings 非数组补为空数组', () => {
    expect((normalizeConfig({ strm: {} }).strm as Record<string, unknown>).mappings).toEqual([])
    expect((normalizeConfig({ strm: { mappings: 'x' } }).strm as Record<string, unknown>).mappings).toEqual([])
    expect((normalizeConfig({ strm: { mappings: [{ pan_folder_id: 'a', local_path: '/b' }] } }).strm as Record<string, unknown>).mappings)
      .toEqual([{ pan_folder_id: 'a', local_path: '/b' }])
  })

  it('null 值与 undefined 同样触发默认值', () => {
    const out = normalizeConfig({
      webui: { token_expire_hours: null },
      trakt: { sse_enabled: null, sync_to_plex: null, sync_to_trakt: null },
      strm: { fetch_sidecar: null },
    })
    expect((out.webui as Record<string, unknown>).token_expire_hours).toBe(24)
    expect((out.trakt as Record<string, unknown>).sse_enabled).toBe(true)
    expect((out.trakt as Record<string, unknown>).sync_to_plex).toBe(true)
    expect((out.trakt as Record<string, unknown>).sync_to_trakt).toBe(true)
    expect((out.strm as Record<string, unknown>).fetch_sidecar).toBe(true)
  })

  it('不改动入参对象（保存流程依赖 original 对照）', () => {
    const input = { webui: { token_expire_hours: '' } }
    normalizeConfig(input)
    expect(input.webui.token_expire_hours).toBe('')
  })

  it('布尔开关：已有 false 不被默认 true 覆盖', () => {
    const out = normalizeConfig({
      trakt: { sse_enabled: false, sync_to_plex: false, sync_to_trakt: false },
      strm: { enabled: false, fetch_sidecar: false, refresh_plex: false },
    })
    expect((out.trakt as Record<string, unknown>).sse_enabled).toBe(false)
    expect((out.trakt as Record<string, unknown>).sync_to_plex).toBe(false)
    expect((out.trakt as Record<string, unknown>).sync_to_trakt).toBe(false)
    expect((out.strm as Record<string, unknown>).enabled).toBe(false)
    expect((out.strm as Record<string, unknown>).fetch_sidecar).toBe(false)
    expect((out.strm as Record<string, unknown>).refresh_plex).toBe(false)
  })
})
