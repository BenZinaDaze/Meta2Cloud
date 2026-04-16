import { useState, useEffect, useCallback, useRef } from 'react'
import {
  getMainConfig,
  saveMainConfig,
  getParserRulesConfig,
  saveParserRulesConfig,
  getDriveOauthStatus,
  testDriveConnection,
  getU115OauthStatus,
  createU115OauthSession,
  pollU115OauthStatus,
  exchangeU115OauthToken,
  testU115Connection,
  fetchU115QrCode,
} from '../api'
import CustomWordsHelp from './config/CustomWordsHelp'
import { SkeletonPanel } from './StatePanel'

const PAGE_VARIANTS = {
  general: {
    title: '配置文件',
    description: '维护媒体库、元数据与下载服务相关配置',
  },
  filenameRules: {
    title: '文件名识别规则',
    description: '集中维护文件名识别、自定义识别词和字幕组补充配置',
  },
}

function normalizeConfig(data) {
  const next = structuredClone(data || {})
  const aria2 = { ...(next.aria2 || {}) }
  const webui = { ...(next.webui || {}) }
  const tmdb = { ...(next.tmdb || {}) }
  const u115 = { ...(next.u115 || {}) }
  const telegram = { ...(next.telegram || {}) }

  if (aria2.enabled === undefined) {
    aria2.enabled = aria2.auto_connect !== false
  }
  delete aria2.auto_connect

  if (webui.token_expire_hours === undefined || webui.token_expire_hours === null || webui.token_expire_hours === '') {
    webui.token_expire_hours = 24
  }
  if (webui.log_retention_days === undefined || webui.log_retention_days === null || webui.log_retention_days === '') {
    webui.log_retention_days = 7
  }
  if (tmdb.timeout === undefined || tmdb.timeout === null || tmdb.timeout === '') {
    tmdb.timeout = 10
  }
  if (u115.auto_organize_poll_seconds === undefined || u115.auto_organize_poll_seconds === null || u115.auto_organize_poll_seconds === '') {
    u115.auto_organize_poll_seconds = 45
  }
  if (u115.auto_organize_stable_seconds === undefined || u115.auto_organize_stable_seconds === null || u115.auto_organize_stable_seconds === '') {
    u115.auto_organize_stable_seconds = 30
  }
  if (telegram.debounce_seconds === undefined || telegram.debounce_seconds === null || telegram.debounce_seconds === '') {
    telegram.debounce_seconds = 60
  }

  next.aria2 = aria2
  next.webui = webui
  next.tmdb = tmdb
  next.u115 = u115
  next.telegram = telegram
  return next
}

// ─── 通用字段组件 ────────────────────────────────────────

function Section({ title, children }) {
  return (
    <div className="rounded-xl mb-6 overflow-hidden"
      style={{ border: '1px solid var(--color-border)', background: 'var(--color-surface-2)' }}>
      <div className="px-5 py-3" style={{ borderBottom: '1px solid var(--color-border)', background: 'var(--color-surface)' }}>
        <h2 className="text-sm font-semibold" style={{ color: 'var(--color-text)' }}>{title}</h2>
      </div>
      <div className="divide-y" style={{ '--tw-divide-opacity': 1 }}>
        {children}
      </div>
    </div>
  )
}

function FieldRow({ label, description, children }) {
  return (
    <div className="flex flex-col gap-3 px-5 py-4 sm:flex-row sm:items-start sm:gap-6"
      style={{ borderBottom: '1px solid var(--color-border)' }}>
      <div className="flex-shrink-0 sm:w-48">
        <div className="text-sm font-medium" style={{ color: 'var(--color-text)' }}>{label}</div>
        {description && (
          <div className="text-xs mt-0.5" style={{ color: 'var(--color-muted)', lineHeight: 1.5 }}>
            {description}
          </div>
        )}
      </div>
      <div className="flex-1">{children}</div>
    </div>
  )
}

function TextInput({ value, onChange, placeholder, type = 'text', mono = false }) {
  const [show, setShow] = useState(false)
  const isPassword = type === 'password'
  const inputType = isPassword ? (show ? 'text' : 'password') : type

  return (
    <div className="relative flex items-center">
      <input
        type={inputType}
        value={value ?? ''}
        onChange={e => onChange(e.target.value)}
        placeholder={placeholder}
        className="w-full text-sm px-3 py-2 rounded-lg outline-none transition-all"
        style={{
          background: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
          color: 'var(--color-text)',
          fontFamily: mono ? 'monospace' : undefined,
          fontSize: mono ? 13 : undefined,
          paddingRight: isPassword ? 40 : undefined,
        }}
      />
      {isPassword && (
        <button
          type="button"
          onClick={() => setShow(s => !s)}
          className="absolute right-2 flex items-center justify-center transition-opacity hover:opacity-70"
          style={{ color: 'var(--color-muted)', background: 'none', border: 'none', cursor: 'pointer', padding: 4 }}
          tabIndex={-1}
          title={show ? '隐藏' : '显示'}
        >
          {show ? (
            /* eye-off */
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94" />
              <path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19" />
              <line x1="1" y1="1" x2="23" y2="23" />
            </svg>
          ) : (
            /* eye */
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
              <circle cx="12" cy="12" r="3" />
            </svg>
          )}
        </button>
      )}
    </div>
  )
}

function NumberInput({ value, onChange, min, max }) {
  const formatValue = (input) => (
    input === undefined || input === null || input === '' ? '' : String(input)
  )
  const [draft, setDraft] = useState('')
  const [editing, setEditing] = useState(false)
  const lastCommittedRef = useRef(value)
  const displayValue = editing ? draft : formatValue(value)

  return (
    <input
      type="text"
      inputMode="numeric"
      pattern="[0-9]*"
      value={displayValue}
      min={min}
      max={max}
      onFocus={() => {
        lastCommittedRef.current = value
        setDraft(formatValue(value))
        setEditing(true)
      }}
      onChange={(e) => {
        const next = e.target.value
        if (next === '') {
          setDraft('')
          return
        }
        if (!/^\d+$/.test(next)) {
          return
        }
        const normalized = next.replace(/^0+(?=\d)/, '')
        setDraft(normalized)
      }}
      onBlur={(e) => {
        setEditing(false)
        const raw = e.target.value.trim()
        if (raw === '') {
          const fallback = lastCommittedRef.current ?? min ?? ''
          if (fallback !== '' && fallback !== value) {
            onChange(fallback)
          }
          return
        }
        const num = Number(raw)
        if (!Number.isFinite(num)) {
          return
        }
        const clamped = Math.min(max ?? num, Math.max(min ?? num, num))
        lastCommittedRef.current = clamped
        onChange(clamped)
      }}
      className="text-sm px-3 py-2 rounded-lg outline-none transition-all"
      style={{
        width: 100,
        background: 'var(--color-surface)',
        border: '1px solid var(--color-border)',
        color: 'var(--color-text)',
        appearance: 'textfield',
        MozAppearance: 'textfield',
      }}
    />
  )
}

function Toggle({ value, onChange }) {
  return (
    <button
      role="switch"
      aria-checked={!!value}
      onClick={() => onChange(!value)}
      className="relative flex-shrink-0 transition-all duration-200"
      style={{
        width: 44,
        height: 24,
        borderRadius: 12,
        background: value ? 'var(--color-accent)' : 'var(--color-border)',
        border: 'none',
        cursor: 'pointer',
        padding: 0,
      }}
    >
      <span
        className="absolute top-1 transition-all duration-200"
        style={{
          left: value ? 22 : 2,
          width: 16,
          height: 16,
          borderRadius: 8,
          background: '#fff',
          boxShadow: '0 1px 3px rgba(0,0,0,0.3)',
        }}
      />
    </button>
  )
}

function SelectInput({ value, onChange, options }) {
  return (
    <select
      value={value ?? ''}
      onChange={e => onChange(e.target.value)}
      className="text-sm px-3 py-2 rounded-lg outline-none"
      style={{
        background: 'var(--color-surface)',
        border: '1px solid var(--color-border)',
        color: 'var(--color-text)',
        minWidth: 160,
      }}
    >
      {options.map(o => (
        <option key={o.value} value={o.value}>{o.label}</option>
      ))}
    </select>
  )
}

function formatStorageSize(bytes) {
  const value = Number(bytes)
  if (!Number.isFinite(value) || value < 0) return '-'

  const GB = 1024 ** 3
  const TB = 1024 ** 4
  const threshold = 5 * TB

  if (value < threshold) {
    return `${(value / GB).toFixed(2)} GB`
  }
  return `${(value / TB).toFixed(2)} TB`
}

/** 列表字段：每行一条，显示为 tag + 删除按钮 + 输入框添加 */
function ListField({ value = [], onChange }) {
  const [draft, setDraft] = useState('')

  function add() {
    const v = draft.trim()
    if (!v || value.includes(v)) { setDraft(''); return }
    onChange([...value, v])
    setDraft('')
  }

  return (
    <div className="space-y-2">
      {/* 已有条目 */}
      {value.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {value.map((item, i) => (
            <div
              key={i}
              className="inline-flex items-center gap-2 max-w-full px-3 py-2 rounded-xl"
              style={{
                background: 'var(--color-surface)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-text)',
              }}
            >
              <span
                className="text-xs font-mono"
                style={{
                  wordBreak: 'break-all',
                  lineHeight: 1.5,
                }}
              >
                {item}
              </span>
              <button
                onClick={() => onChange(value.filter((_, j) => j !== i))}
                className="flex-shrink-0 flex items-center justify-center w-5 h-5 rounded-md transition-all hover:opacity-70"
                style={{ background: 'rgba(239,68,68,0.1)', color: '#ef4444', border: 'none' }}
              >
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
                  <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
                </svg>
              </button>
            </div>
          ))}
        </div>
      )}
      {/* 添加新条目 */}
      <div className="flex gap-2">
        <input
          value={draft}
          onChange={e => setDraft(e.target.value)}
          onKeyDown={e => e.key === 'Enter' && add()}
          placeholder="输入后按 Enter 或点击添加"
          className="flex-1 text-xs font-mono px-3 py-2 rounded-lg outline-none"
          style={{
            background: 'var(--color-surface)',
            border: '1px dashed var(--color-border)',
            color: 'var(--color-text)',
          }}
        />
        <button
          onClick={add}
          disabled={!draft.trim()}
          className="flex-shrink-0 text-xs px-3 py-2 rounded-lg transition-all disabled:opacity-40"
          style={{
            background: 'var(--color-accent)',
            color: '#fff',
            border: 'none',
            cursor: draft.trim() ? 'pointer' : 'not-allowed',
          }}
        >添加</button>
      </div>
    </div>
  )
}

function MultilineRulesField({ value = [], onChange, placeholder = '' }) {
  const text = Array.isArray(value) ? value.join('\n') : ''
  const [draft, setDraft] = useState(text)
  const [editing, setEditing] = useState(false)
  const displayText = editing ? draft : text
  const lineCount = displayText === '' ? 1 : displayText.split('\n').length

  function commit(nextText) {
    const next = nextText
      .split('\n')
      .map(line => line.trim())
      .filter(Boolean)
    onChange(next)
  }

  return (
    <div className="space-y-2">
      <textarea
        value={displayText}
        onFocus={() => {
          setDraft(text)
          setEditing(true)
        }}
        onChange={(e) => {
          const nextText = e.target.value
          setDraft(nextText)
          commit(nextText)
        }}
        onBlur={() => {
          setEditing(false)
        }}
        placeholder={placeholder}
        spellCheck={false}
        className="w-full text-sm font-mono px-4 py-3 rounded-lg outline-none resize-none"
        rows={Math.max(8, Math.min(18, lineCount + 2))}
        style={{
          background: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
          color: 'var(--color-text)',
          lineHeight: 1.8,
        }}
      />
      <div className="text-xs" style={{ color: 'var(--color-muted)' }}>
        一行一个规则，空行会自动忽略。当前 {value.length} 条。
      </div>
    </div>
  )
}

// ─── 主页面 ──────────────────────────────────────────────

export default function ConfigPage({ onAria2EnabledChange = null, page = 'general' }) {
  const [cfg, setCfg] = useState(null)
  const [original, setOriginal] = useState(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState(null)
  const [saved, setSaved] = useState(false)
  const [driveOauth, setDriveOauth] = useState(null)
  const [driveOauthMessage, setDriveOauthMessage] = useState(null)
  const [driveTestBusy, setDriveTestBusy] = useState(false)
  const [u115Oauth, setU115Oauth] = useState(null)
  const [u115OauthMessage, setU115OauthMessage] = useState(null)
  const [u115QrUrl, setU115QrUrl] = useState('')
  const [u115QrPreviewUrl, setU115QrPreviewUrl] = useState('')
  const [u115QrPreviewError, setU115QrPreviewError] = useState('')
  const [u115AuthBusy, setU115AuthBusy] = useState(false)
  const [u115TestBusy, setU115TestBusy] = useState(false)
  const [u115Polling, setU115Polling] = useState(false)
  const [u115StatusLoading, setU115StatusLoading] = useState(true)
  const mountedRef = useRef(true)
  const u115CreateAbortRef = useRef(null)
  const u115QrFetchAbortRef = useRef(null)
  const u115PollAbortRef = useRef(null)
  const u115ExchangeAbortRef = useRef(null)
  const u115LastQrPreviewUrlRef = useRef('')

  const cancelU115AuthFlow = useCallback(() => {
    if (u115CreateAbortRef.current) {
      u115CreateAbortRef.current.abort()
      u115CreateAbortRef.current = null
    }
    if (u115QrFetchAbortRef.current) {
      u115QrFetchAbortRef.current.abort()
      u115QrFetchAbortRef.current = null
    }
    if (u115PollAbortRef.current) {
      u115PollAbortRef.current.abort()
      u115PollAbortRef.current = null
    }
    if (u115ExchangeAbortRef.current) {
      u115ExchangeAbortRef.current.abort()
      u115ExchangeAbortRef.current = null
    }
  }, [])

  const loadDriveOauthStatus = useCallback(async () => {
    try {
      const res = await getDriveOauthStatus()
      setDriveOauth(res.data)
    } catch (e) {
      setDriveOauth(null)
      setDriveOauthMessage({
        type: 'error',
        text: e?.response?.data?.detail || e.message || '读取 Google Drive 授权状态失败',
      })
    }
  }, [])

  const loadU115OauthStatus = useCallback(async () => {
    setU115StatusLoading(true)
    try {
      const res = await getU115OauthStatus()
      setU115Oauth(res.data)
    } catch (e) {
      setU115Oauth(null)
      setU115OauthMessage({
        type: 'error',
        text: e?.response?.data?.detail || e.message || '读取 115 授权状态失败',
      })
    } finally {
      setU115StatusLoading(false)
    }
  }, [])

  useEffect(() => {
    setLoading(true)
    const loader = page === 'filenameRules' ? getParserRulesConfig : getMainConfig
    loader()
      .then(r => {
        const source = page === 'filenameRules'
          ? { parser: r.data || {} }
          : r.data
        const normalized = normalizeConfig(source)
        setCfg(normalized)
        setOriginal(normalized)
      })
      .catch(e => setError(e?.response?.data?.detail || e.message))
      .finally(() => setLoading(false))
    if (page !== 'filenameRules') {
      loadDriveOauthStatus()
      loadU115OauthStatus()
    }
  }, [loadDriveOauthStatus, loadU115OauthStatus, page])

  const isDirty = JSON.stringify(cfg) !== JSON.stringify(original)
  const pageMeta = PAGE_VARIANTS[page] || PAGE_VARIANTS.general
  const showGeneralSections = page !== 'filenameRules'
  const showFilenameRuleSection = page === 'filenameRules'
  const isFilenameRulesPage = page === 'filenameRules'

  /** 深层更新某个字段 */
  const set = useCallback((section, key, val) => {
    setCfg(prev => ({
      ...prev,
      [section]: { ...(prev[section] || {}), [key]: val },
    }))
  }, [])

  async function handleSave() {
    setSaving(true); setError(null); setSaved(false)
    try {
      const normalized = normalizeConfig(cfg)
      if (page === 'filenameRules') {
        await saveParserRulesConfig(normalized?.parser || normalized)
      } else {
        await saveMainConfig(normalized)
      }
      setCfg(normalized)
      setOriginal(normalized)
      if (page !== 'filenameRules') {
        onAria2EnabledChange?.(normalized?.aria2?.enabled !== false)
      }
      setSaved(true)
      setTimeout(() => setSaved(false), 3000)
    } catch (e) {
      setError(e?.response?.data?.detail || e.message)
    } finally {
      setSaving(false)
    }
  }

  async function handleDriveTest() {
    setDriveTestBusy(true)
    setDriveOauthMessage(null)
    try {
      const res = await testDriveConnection()
      const data = res?.data || {}
      const identity = data.email || data.display_name || '当前账号'
      setDriveOauthMessage({
        type: 'success',
        text: `连接成功：${identity}`,
      })
      loadDriveOauthStatus()
    } catch (e) {
      setDriveOauthMessage({
        type: 'error',
        text: e?.response?.data?.detail || e.message || 'Drive 连接测试失败',
      })
    } finally {
      setDriveTestBusy(false)
    }
  }

  async function handleU115CreateQr() {
    setU115AuthBusy(true)
    setU115Polling(false)
    setU115OauthMessage(null)
    setU115QrPreviewError('')
    cancelU115AuthFlow()
    try {
      const createController = new AbortController()
      u115CreateAbortRef.current = createController
      const res = await createU115OauthSession({
        client_id: cfg?.u115?.client_id,
        token_json: cfg?.u115?.token_json,
      }, { signal: createController.signal })
      u115CreateAbortRef.current = null
      if (!mountedRef.current) return
      setU115QrUrl(res?.data?.qrcode || '')
      try {
        const controller = new AbortController()
        u115QrFetchAbortRef.current = controller
        const qrRes = await fetchU115QrCode({ signal: controller.signal })
        if (!mountedRef.current) return
        const objectUrl = URL.createObjectURL(qrRes.data)
        setU115QrPreviewUrl(objectUrl)
      } catch (e) {
        if (!mountedRef.current) return
        setU115QrPreviewUrl('')
        const status = e?.response?.status
        if (status === 404) {
          setU115QrPreviewError('当前服务端未提供二维码图片接口，请更新后端到最新版本。')
        } else {
          setU115QrPreviewError(e?.response?.data?.detail || e.message || '二维码图片加载失败')
        }
      } finally {
        u115QrFetchAbortRef.current = null
      }
      setU115Polling(true)
      setU115OauthMessage({
        type: 'success',
        text: '115 扫码会话已创建，请使用 App 扫码并确认，系统会自动轮询。',
      })
      loadU115OauthStatus()
    } catch (e) {
      if (e?.code === 'ERR_CANCELED' || e?.name === 'CanceledError') return
      setU115Polling(false)
      setU115OauthMessage({
        type: 'error',
        text: e?.response?.data?.detail || e.message || '创建 115 扫码会话失败',
      })
    } finally {
      setU115AuthBusy(false)
    }
  }

  const exchangeU115AfterConfirm = useCallback(async () => {
    setU115OauthMessage(null)
    try {
      const exchangeController = new AbortController()
      u115ExchangeAbortRef.current = exchangeController
      await exchangeU115OauthToken({
        client_id: cfg?.u115?.client_id,
        token_json: cfg?.u115?.token_json,
      }, { signal: exchangeController.signal })
      u115ExchangeAbortRef.current = null
      if (!mountedRef.current) return
      setU115OauthMessage({
        type: 'success',
        text: '115 授权成功，token 已写入本地。',
      })
      setU115QrUrl('')
      setU115QrPreviewUrl('')
      setU115Polling(false)
      loadU115OauthStatus()
    } catch (e) {
      if (e?.code === 'ERR_CANCELED' || e?.name === 'CanceledError') return
      setU115Polling(false)
      setU115OauthMessage({
        type: 'error',
        text: e?.response?.data?.detail || e.message || '115 换取 Token 失败',
      })
    }
  }, [cfg?.u115?.client_id, cfg?.u115?.token_json, loadU115OauthStatus])

  async function handleU115Test() {
    setU115TestBusy(true)
    setU115OauthMessage(null)
    try {
      const res = await testU115Connection()
      const data = res?.data || {}
      setU115OauthMessage({
        type: 'success',
        text: `连接成功：剩余空间 ${formatStorageSize(data.remain_space)} / 总空间 ${formatStorageSize(data.total_space)}`,
      })
      loadU115OauthStatus()
    } catch (e) {
      setU115OauthMessage({
        type: 'error',
        text: e?.response?.data?.detail || e.message || '115 连接测试失败',
      })
    } finally {
      setU115TestBusy(false)
    }
  }

  useEffect(() => {
    if (!u115Polling || !u115QrUrl) return

    let cancelled = false
    const timer = setTimeout(async () => {
      try {
        const controller = new AbortController()
        u115PollAbortRef.current = controller
        const res = await pollU115OauthStatus({ signal: controller.signal })
        if (cancelled) return
        const data = res?.data || {}
        if (data.confirmed) {
          setU115OauthMessage({
            type: 'success',
            text: '已扫码并确认，正在换取 Token…',
          })
          await exchangeU115AfterConfirm()
          return
        }

        setU115OauthMessage({
          type: data.exchange_error ? 'error' : 'success',
          text: data.exchange_error
            ? `115 已扫码，但换取 Token 失败：${data.exchange_error}`
            : (
              data.status >= 1
                ? (data.message || '已扫码，等待手机确认…')
                : `等待扫码：${data.message || data.status}`
            ),
        })
      } catch (e) {
        if (cancelled) return
        if (e?.code === 'ERR_CANCELED' || e?.name === 'CanceledError') return
        const status = e?.response?.status
        if (status === 502 || status === 503 || status === 504 || !status) {
          setU115OauthMessage({
            type: 'error',
            text: '115 扫码状态查询暂时超时，系统会继续自动重试…',
          })
          return
        }
        setU115Polling(false)
        setU115OauthMessage({
          type: 'error',
          text: e?.response?.data?.detail || e.message || '查询 115 扫码状态失败',
        })
      } finally {
        u115PollAbortRef.current = null
      }
    }, 2000)

    return () => {
      cancelled = true
      clearTimeout(timer)
      if (u115PollAbortRef.current) {
        u115PollAbortRef.current.abort()
        u115PollAbortRef.current = null
      }
    }
  }, [u115Polling, u115QrUrl, exchangeU115AfterConfirm])

  useEffect(() => {
    const previousUrl = u115LastQrPreviewUrlRef.current
    u115LastQrPreviewUrlRef.current = u115QrPreviewUrl

    if (previousUrl && previousUrl !== u115QrPreviewUrl) {
      URL.revokeObjectURL(previousUrl)
    }
  }, [u115QrPreviewUrl])

  useEffect(() => {
    mountedRef.current = true
    const handlePageHide = () => {
      cancelU115AuthFlow()
    }
    window.addEventListener('pagehide', handlePageHide)
    window.addEventListener('beforeunload', handlePageHide)
    return () => {
      mountedRef.current = false
      window.removeEventListener('pagehide', handlePageHide)
      window.removeEventListener('beforeunload', handlePageHide)
      cancelU115AuthFlow()
      if (u115LastQrPreviewUrlRef.current) {
        URL.revokeObjectURL(u115LastQrPreviewUrlRef.current)
        u115LastQrPreviewUrlRef.current = ''
      }
    }
  }, [cancelU115AuthFlow])

  if (loading) return (
    <div className="flex-1 space-y-4">
      {[1, 2, 3].map(i => <SkeletonPanel key={i} compact rows={3} />)}
    </div>
  )

  return (
    <div className="flex-1 flex flex-col min-w-0">
      {/* 页头 */}
      <div className="flex flex-wrap items-center gap-4 mb-6">
        <div>
          <h1 className="text-xl font-bold" style={{ color: 'var(--color-text)' }}>{pageMeta.title}</h1>
          <p className="text-sm mt-0.5" style={{ color: 'var(--color-muted)' }}>
            {pageMeta.description}
          </p>
        </div>
        <div className="ml-auto flex items-center gap-2">
          {isDirty && !saving && (
            <span className="text-xs px-2 py-1 rounded-full"
              style={{ background: 'rgba(234,179,8,0.15)', color: '#eab308' }}>未保存</span>
          )}
          {saved && (
            <span className="text-xs px-2 py-1 rounded-full"
              style={{ background: 'rgba(34,197,94,0.15)', color: '#22c55e' }}>✓ 已保存</span>
          )}
          <button onClick={() => { setCfg(original); setError(null) }}
            disabled={!isDirty || saving}
            className="text-sm px-4 py-2 rounded-lg transition-all disabled:opacity-40"
            style={{ background: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-muted)' }}>
            重置
          </button>
          <button onClick={handleSave} disabled={!isDirty || saving}
            className="text-sm px-5 py-2 rounded-lg font-medium transition-all flex items-center gap-2 disabled:opacity-40"
            style={{
              background: isDirty && !saving ? 'var(--color-accent)' : 'var(--color-surface-2)',
              color: isDirty && !saving ? '#fff' : 'var(--color-muted)',
              border: 'none',
            }}>
            {saving && <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5"
              style={{ animation: 'spin 1s linear infinite' }}>
              <polyline points="23 4 23 10 17 10" />
              <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10" />
            </svg>}
            {saving ? '保存中…' : '保存'}
          </button>
        </div>
      </div>

      {error && (
        <div className="mb-5 px-4 py-3 rounded-lg text-sm"
          style={{ background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.3)', color: '#ef4444' }}>
          {error}
        </div>
      )}

      {isFilenameRulesPage ? (
        <div className="flex flex-col gap-6 w-full">
          <div
            className="rounded-2xl p-5"
            style={{ border: '1px solid var(--color-border)', background: 'var(--color-surface-2)' }}
          >
            <div className="flex flex-wrap items-start gap-4 justify-between mb-4">
              <div>
                <h2 className="text-base font-semibold" style={{ color: 'var(--color-text)' }}>自定义识别词</h2>
                <p className="text-sm mt-1" style={{ color: 'var(--color-muted)' }}>
                  适合批量粘贴和集中维护。一行一个规则，保存后写入 `config/parser-rules.yaml`。
                </p>
              </div>
              <div
                className="text-xs px-3 py-2 rounded-xl"
                style={{ background: 'rgba(255,255,255,0.04)', color: 'var(--color-muted)', border: '1px solid var(--color-border)' }}
              >
                当前 {cfg?.parser?.custom_words?.length ?? 0} 条规则
              </div>
            </div>

            <MultilineRulesField
              value={cfg?.parser?.custom_words ?? []}
              onChange={v => set('parser', 'custom_words', v)}
              placeholder={'国语配音\nOVA => SP\n第 <> 集 >> EP-1'}
            />
          </div>

          <div
            className="rounded-2xl p-5"
            style={{ border: '1px solid var(--color-border)', background: 'var(--color-surface-2)' }}
          >
            <div className="mb-4">
              <h2 className="text-base font-semibold" style={{ color: 'var(--color-text)' }}>自定义字幕组</h2>
              <p className="text-sm mt-1" style={{ color: 'var(--color-muted)' }}>
                补充内置列表没有的字幕组名称。每个字幕组单独显示为一个胶囊，便于逐项管理。
              </p>
            </div>
            <ListField value={cfg?.parser?.custom_release_groups ?? []} onChange={v => set('parser', 'custom_release_groups', v)} />
          </div>

          <div
            className="rounded-2xl p-5"
            style={{ border: '1px solid var(--color-border)', background: 'var(--color-surface)' }}
          >
            <div className="flex items-center justify-between gap-3 mb-2">
              <h2 className="text-base font-semibold" style={{ color: 'var(--color-text)' }}>规则格式说明</h2>
              <span className="text-xs" style={{ color: 'var(--color-muted)' }}>支持屏蔽、替换、偏移和组合规则</span>
            </div>
            <CustomWordsHelp />
          </div>
        </div>
      ) : (
      <div className={isFilenameRulesPage ? '' : 'config-grid'}>
        {/* 左列 */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 0 }}>
          {showGeneralSections && (
            <>
              {/* ── 存储后端 ── */}
              <Section title="云存储选择">
                <FieldRow label="使用的网盘" description="选择媒体文件存放在哪个网盘，扫描整理和元数据上传都会使用该网盘">
                  <div className="flex items-center gap-3">
                    <SelectInput
                      value={cfg?.storage?.primary || 'google_drive'}
                      onChange={v => set('storage', 'primary', v)}
                      options={[
                        { value: 'google_drive', label: 'Google Drive' },
                        { value: 'pan115', label: '115 网盘' },
                      ]}
                    />
                    <span className="text-xs px-2.5 py-1 rounded-full flex items-center gap-1.5"
                      style={{
                        background: (cfg?.storage?.primary || 'google_drive') === 'google_drive'
                          ? 'rgba(66,133,244,0.12)' : 'rgba(34,197,94,0.12)',
                        color: (cfg?.storage?.primary || 'google_drive') === 'google_drive'
                          ? '#4285f4' : '#22c55e',
                      }}>
                      <span style={{
                        width: 6, height: 6, borderRadius: '50%',
                        background: 'currentColor',
                      }} />
                      {(cfg?.storage?.primary || 'google_drive') === 'google_drive' ? 'Google Drive' : '115 网盘'}
                    </span>
                  </div>
                </FieldRow>
              </Section>

              {/* ── WebUI 认证 ── */}
              <Section title="WebUI 认证">
                <FieldRow label="用户名" description="登录账号，默认 admin">
                  <TextInput value={cfg?.webui?.username} onChange={v => set('webui', 'username', v)} placeholder="admin" />
                </FieldRow>
                <FieldRow label="密码" description="留空则允许任何人访问（不推荐）">
                  <TextInput value={cfg?.webui?.password} type="password" onChange={v => set('webui', 'password', v)} placeholder="设置一个强密码" />
                </FieldRow>
                <FieldRow label="Token 有效期（小时）" description="默认 24 小时，重启服务不影响有效 token">
                  <NumberInput value={cfg?.webui?.token_expire_hours} onChange={v => set('webui', 'token_expire_hours', v)} min={1} max={8760} />
                </FieldRow>
                <FieldRow label="Webhook 密钥" description="/trigger 端点校验密钥，空则不校验（仅内网时留空）">
                  <TextInput value={cfg?.webui?.webhook_secret} type="password" onChange={v => set('webui', 'webhook_secret', v)} placeholder="留空则不校验" mono />
                </FieldRow>
                <FieldRow label="日志保留天数" description="日志按天分文件保存，超过这个天数会自动清理">
                  <NumberInput value={cfg?.webui?.log_retention_days} onChange={v => set('webui', 'log_retention_days', v)} min={1} max={365} />
                </FieldRow>
              </Section>

              {/* ── Google Drive ── */}
              <Section title="Google Drive 设置">
            <FieldRow label="OAuth2 凭据路径" description="credentials.json 文件路径">
              <TextInput value={cfg?.drive?.credentials_json} onChange={v => set('drive', 'credentials_json', v)} placeholder="config/credentials.json" mono />
            </FieldRow>
            <FieldRow label="OAuth2 Token 路径" description="首次授权后自动生成">
              <TextInput value={cfg?.drive?.token_json} onChange={v => set('drive', 'token_json', v)} placeholder="config/token.json" mono />
            </FieldRow>
            <FieldRow label="授权状态" description="显示当前 Google Drive OAuth 状态">
              <div className="space-y-3">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-xs px-2 py-1 rounded-full"
                    style={{
                      background: driveOauth?.authorized ? 'rgba(34,197,94,0.15)' : 'rgba(148,163,184,0.14)',
                      color: driveOauth?.authorized ? '#22c55e' : 'var(--color-muted)',
                    }}>
                    {driveOauth?.authorized ? '已授权' : '未授权'}
                  </span>
                  <button
                    onClick={handleDriveTest}
                    disabled={driveTestBusy || driveOauth?.credentials_exists === false}
                    className="text-xs px-3 py-1 rounded-full transition-all disabled:opacity-40"
                    style={{
                      background: 'linear-gradient(135deg, var(--color-accent) 0%, #b37533 100%)',
                      border: 'none',
                      color: '#fff',
                    }}
                  >
                    {driveTestBusy ? '测试中…' : '测试 Drive 连接'}
                  </button>
                  {driveOauth?.credentials_exists === false && (
                    <span className="text-xs px-2 py-1 rounded-full"
                      style={{ background: 'rgba(239,68,68,0.12)', color: '#ef4444' }}>
                      未找到 credentials.json
                    </span>
                  )}
                  {driveOauth?.token_exists && !driveOauth?.authorized && (
                    <span className="text-xs px-2 py-1 rounded-full"
                      style={{ background: 'rgba(245,158,11,0.12)', color: '#f59e0b' }}>
                      token 存在但当前不可用
                    </span>
                  )}
                </div>

                {driveOauthMessage && (
                  <div className="px-3 py-2 rounded-lg text-xs"
                    style={{
                      background: driveOauthMessage.type === 'success' ? 'rgba(34,197,94,0.12)' : 'rgba(239,68,68,0.1)',
                      border: `1px solid ${driveOauthMessage.type === 'success' ? 'rgba(34,197,94,0.28)' : 'rgba(239,68,68,0.25)'}`,
                      color: driveOauthMessage.type === 'success' ? '#22c55e' : '#ef4444',
                    }}>
                    {driveOauthMessage.text}
                  </div>
                )}
              </div>
            </FieldRow>
            <FieldRow label="扫描目录 ID" description="Drive 目标文件夹 ID">
              <TextInput value={cfg?.drive?.scan_folder_id} onChange={v => set('drive', 'scan_folder_id', v)} placeholder="1AbCdEfGhIjKlMn..." mono />
            </FieldRow>
            <FieldRow label="媒体库根目录 ID" description="Google Drive 媒体库顶层目录 ID，电影和剧集默认归档到这里">
              <TextInput value={cfg?.drive?.root_folder_id} onChange={v => set('drive', 'root_folder_id', v)} placeholder="Google Drive 文件夹 ID" mono />
            </FieldRow>
            <FieldRow label="电影归档目录 ID" description="Google Drive 电影目录 ID，留空则直接使用媒体库根目录">
              <TextInput value={cfg?.drive?.movie_root_id} onChange={v => set('drive', 'movie_root_id', v)} placeholder="留空同根目录" mono />
            </FieldRow>
            <FieldRow label="剧集归档目录 ID" description="Google Drive 剧集目录 ID，留空则直接使用媒体库根目录">
              <TextInput value={cfg?.drive?.tv_root_id} onChange={v => set('drive', 'tv_root_id', v)} placeholder="留空同根目录" mono />
            </FieldRow>
          </Section>

          <Section title="115 网盘设置">
            <FieldRow label="Client ID" description="115 开放平台应用 client_id">
              <TextInput value={cfg?.u115?.client_id} onChange={v => set('u115', 'client_id', v)} placeholder="100197847" mono />
            </FieldRow>
            <FieldRow label="Token 路径" description="扫码授权成功后自动生成">
              <TextInput value={cfg?.u115?.token_json} onChange={v => set('u115', 'token_json', v)} placeholder="config/115-token.json" mono />
            </FieldRow>
            <FieldRow label="授权状态" description="创建二维码后会自动轮询扫码状态，并在确认后自动换取 115 Token">
              <div className="space-y-3">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-xs px-2 py-1 rounded-full"
                    style={{
                      background: u115StatusLoading
                        ? 'rgba(59,130,246,0.12)'
                        : (u115Oauth?.authorized ? 'rgba(34,197,94,0.15)' : 'rgba(148,163,184,0.14)'),
                      color: u115StatusLoading
                        ? '#3b82f6'
                        : (u115Oauth?.authorized ? '#22c55e' : 'var(--color-muted)'),
                    }}>
                    {u115StatusLoading ? '刷新授权中' : (u115Oauth?.authorized ? '已授权' : '未授权')}
                  </span>
                  <button
                    onClick={handleU115CreateQr}
                    disabled={u115AuthBusy || !cfg?.u115?.client_id}
                    className="text-xs px-3 py-1 rounded-full transition-all disabled:opacity-40"
                    style={{ background: 'linear-gradient(135deg, var(--color-accent) 0%, #b37533 100%)', border: 'none', color: '#fff' }}
                  >
                    {u115AuthBusy ? '处理中…' : (u115Polling ? '等待扫码中…' : '开始授权')}
                  </button>
                  <button
                    onClick={handleU115Test}
                    disabled={u115TestBusy || u115Oauth?.token_exists === false}
                    className="text-xs px-3 py-1 rounded-full transition-all disabled:opacity-40"
                    style={{ background: 'var(--color-surface)', border: '1px solid var(--color-border)', color: 'var(--color-text)' }}
                  >
                    {u115TestBusy ? '测试中…' : '测试 115 连接'}
                  </button>
                </div>

                {u115QrUrl && (
                  <div className="rounded-lg p-3 inline-flex flex-col gap-2"
                    style={{ background: 'var(--color-surface)', border: '1px solid var(--color-border)' }}>
                    <div className="text-xs" style={{ color: 'var(--color-muted)' }}>请使用 115 App 扫码</div>
                    {u115QrPreviewUrl ? (
                      <img src={u115QrPreviewUrl} alt="115 OAuth QR Code" className="w-44 h-44 rounded-lg" />
                    ) : u115QrPreviewError ? (
                      <div className="w-44 h-44 rounded-lg flex items-center justify-center text-xs text-center p-3"
                        style={{ background: 'rgba(239,68,68,0.08)', color: '#ef4444', border: '1px solid rgba(239,68,68,0.18)' }}>
                        {u115QrPreviewError}
                      </div>
                    ) : (
                      <div className="w-44 h-44 rounded-lg flex items-center justify-center text-xs"
                        style={{ background: 'var(--color-surface-2)', color: 'var(--color-muted)' }}>
                        二维码加载中…
                      </div>
                    )}
                  </div>
                )}

                {u115OauthMessage && (
                  <div className="px-3 py-2 rounded-lg text-xs"
                    style={{
                      background: u115OauthMessage.type === 'success' ? 'rgba(34,197,94,0.12)' : 'rgba(239,68,68,0.1)',
                      border: `1px solid ${u115OauthMessage.type === 'success' ? 'rgba(34,197,94,0.28)' : 'rgba(239,68,68,0.25)'}`,
                      color: u115OauthMessage.type === 'success' ? '#22c55e' : '#ef4444',
                    }}>
                    {u115OauthMessage.text}
                  </div>
                )}
              </div>
            </FieldRow>
            <FieldRow label="云下载目录 ID" description="115 离线下载默认保存目录 ID">
              <TextInput value={cfg?.u115?.download_folder_id} onChange={v => set('u115', 'download_folder_id', v)} placeholder="115 云下载目录 ID" mono />
            </FieldRow>
            <FieldRow label="自动整理" description="监听 115 云下载任务，任务完成后自动触发整理流程">
              <div className="flex items-center gap-3">
                <Toggle
                  value={cfg?.u115?.auto_organize_enabled}
                  onChange={v => set('u115', 'auto_organize_enabled', v)}
                />
                <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
                  {cfg?.u115?.auto_organize_enabled ? '已开启' : '已关闭'}
                </span>
              </div>
            </FieldRow>
            <FieldRow label="轮询间隔（秒）" description="后台检查 115 云下载任务完成状态的频率">
              <NumberInput
                value={cfg?.u115?.auto_organize_poll_seconds ?? 45}
                onChange={v => set('u115', 'auto_organize_poll_seconds', v)}
                min={10}
                max={3600}
              />
            </FieldRow>
            <FieldRow label="完成稳定等待（秒）" description="任务显示完成后，再额外等待多少秒才触发整理">
              <NumberInput
                value={cfg?.u115?.auto_organize_stable_seconds ?? 30}
                onChange={v => set('u115', 'auto_organize_stable_seconds', v)}
                min={0}
                max={600}
              />
            </FieldRow>
            <FieldRow label="媒体库根目录 ID" description="115 媒体库顶层目录 ID，电影和剧集默认归档到这里">
              <TextInput value={cfg?.u115?.root_folder_id} onChange={v => set('u115', 'root_folder_id', v)} placeholder="115 目录 ID" mono />
            </FieldRow>
            <FieldRow label="电影归档目录 ID" description="115 电影目录 ID，留空则直接使用媒体库根目录">
              <TextInput value={cfg?.u115?.movie_root_id} onChange={v => set('u115', 'movie_root_id', v)} placeholder="留空同根目录" mono />
            </FieldRow>
            <FieldRow label="剧集归档目录 ID" description="115 剧集目录 ID，留空则直接使用媒体库根目录">
              <TextInput value={cfg?.u115?.tv_root_id} onChange={v => set('u115', 'tv_root_id', v)} placeholder="留空同根目录" mono />
            </FieldRow>
              </Section>
            </>
          )}

        </div>

        {/* 右列 */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 0 }}>
          {showGeneralSections && (
            <>
              {/* ── TMDB ── */}
              <Section title="TMDB 设置">
                <FieldRow label="API Key" description="TMDB v3 API Key，必填">
                  <TextInput value={cfg?.tmdb?.api_key} type="password"
                    onChange={v => set('tmdb', 'api_key', v)} placeholder="0ec3b170d4c..." mono />
                </FieldRow>
                <FieldRow label="返回语言" description="如 zh-CN、en-US、ja-JP">
                  <TextInput value={cfg?.tmdb?.language} onChange={v => set('tmdb', 'language', v)} placeholder="zh-CN" />
                </FieldRow>
                <FieldRow label="HTTP 代理" description="可选，如 http://127.0.0.1:7890">
                  <TextInput value={cfg?.tmdb?.proxy} onChange={v => set('tmdb', 'proxy', v)} placeholder="留空则不使用代理" mono />
                </FieldRow>
                <FieldRow label="请求超时（秒）">
                  <NumberInput value={cfg?.tmdb?.timeout} onChange={v => set('tmdb', 'timeout', v)} min={1} max={120} />
                </FieldRow>
              </Section>

              {/* ── 扫描与整理 ── */}
              <Section title="扫描与入库策略">
                <FieldRow label="跳过 TMDB 查询" description="开启后只整理文件夹，不生成 NFO 元数据">
                  <Toggle value={cfg?.pipeline?.skip_tmdb} onChange={v => set('pipeline', 'skip_tmdb', v)} />
                </FieldRow>
                <FieldRow label="TMDB 未找到时仍移动" description="关闭则找不到元数据时跳过该文件">
                  <Toggle value={cfg?.pipeline?.move_on_tmdb_miss} onChange={v => set('pipeline', 'move_on_tmdb_miss', v)} />
                </FieldRow>
                <FieldRow label="Dry Run 模式" description="只打印计划，不实际操作 Drive">
                  <Toggle value={cfg?.pipeline?.dry_run} onChange={v => set('pipeline', 'dry_run', v)} />
                </FieldRow>
              </Section>

              <Section title="Aria2 下载设置">
                <FieldRow label="启用 Aria2" description="关闭后禁用整个 Aria2 集成，前端不会连接 RPC，也不会提供下载管理操作">
                  <Toggle value={cfg?.aria2?.enabled !== false} onChange={v => set('aria2', 'enabled', v)} />
                </FieldRow>
                <FieldRow label="RPC 主机" description="通常是 aria2 服务所在机器的 IP 或域名">
                  <TextInput value={cfg?.aria2?.host} onChange={v => set('aria2', 'host', v)} placeholder="127.0.0.1" mono />
                </FieldRow>
                <FieldRow label="RPC 端口">
                  <NumberInput value={cfg?.aria2?.port} onChange={v => set('aria2', 'port', v)} min={1} max={65535} />
                </FieldRow>
                <FieldRow label="RPC 路径" description="默认 /jsonrpc">
                  <TextInput value={cfg?.aria2?.path} onChange={v => set('aria2', 'path', v)} placeholder="/jsonrpc" mono />
                </FieldRow>
                <FieldRow label="RPC 密钥" description="对应 aria2 的 rpc-secret">
                  <TextInput value={cfg?.aria2?.secret} type="password" onChange={v => set('aria2', 'secret', v)} placeholder="留空表示无密钥" mono />
                </FieldRow>
                <FieldRow label="使用 HTTPS" description="如果 aria2 RPC 通过 HTTPS 暴露则开启">
                  <Toggle value={cfg?.aria2?.secure} onChange={v => set('aria2', 'secure', v)} />
                </FieldRow>
              </Section>

              {/* ── Telegram ── */}
              <Section title="Telegram 通知">
                <FieldRow label="Bot Token" description="从 @BotFather 获取">
                  <TextInput value={cfg?.telegram?.bot_token} type="password" onChange={v => set('telegram', 'bot_token', v)} placeholder="123456:ABC..." mono />
                </FieldRow>
                <FieldRow label="Chat ID" description="接收通知的账号或群组 ID">
                  <TextInput value={cfg?.telegram?.chat_id} onChange={v => set('telegram', 'chat_id', v)} placeholder="371338215" mono />
                </FieldRow>
                <FieldRow label="防抖延时（秒）" description="批量入库时合并通知，0 = 立即触发">
                  <NumberInput value={cfg?.telegram?.debounce_seconds} onChange={v => set('telegram', 'debounce_seconds', v)} min={0} max={600} />
                </FieldRow>
              </Section>
            </>
          )}

          {showFilenameRuleSection && (
            <Section title="文件名识别规则">
              <FieldRow label="自定义识别词" description="每条一条规则，支持 4 种格式">
                <MultilineRulesField
                  value={cfg?.parser?.custom_words ?? []}
                  onChange={v => set('parser', 'custom_words', v)}
                  placeholder={'国语配音\nOVA => SP\n第 <> 集 >> EP-1'}
                />
                <CustomWordsHelp />
              </FieldRow>
              <FieldRow label="自定义字幕组" description="补充内置列表没有的字幕组名称">
                <ListField value={cfg?.parser?.custom_release_groups ?? []} onChange={v => set('parser', 'custom_release_groups', v)} />
              </FieldRow>
            </Section>
          )}
        </div>
      </div>
      )}
    </div>
  )
}
