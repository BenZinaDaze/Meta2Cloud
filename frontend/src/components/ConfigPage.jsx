import { useState, useEffect, useCallback } from 'react'
import { getConfig, saveConfig } from '../api'

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
              <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94"/>
              <path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19"/>
              <line x1="1" y1="1" x2="23" y2="23"/>
            </svg>
          ) : (
            /* eye */
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
              <circle cx="12" cy="12" r="3"/>
            </svg>
          )}
        </button>
      )}
    </div>
  )
}

function NumberInput({ value, onChange, min, max }) {
  return (
    <input
      type="number"
      value={value ?? ''}
      min={min}
      max={max}
      onChange={e => onChange(Number(e.target.value))}
      className="text-sm px-3 py-2 rounded-lg outline-none transition-all"
      style={{
        width: 100,
        background: 'var(--color-surface)',
        border: '1px solid var(--color-border)',
        color: 'var(--color-text)',
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
      {value.map((item, i) => (
        <div key={i} className="flex items-start gap-2">
          <span
            className="flex-1 text-xs font-mono px-3 py-2 rounded-lg"
            style={{
              background: 'var(--color-surface)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-text)',
              wordBreak: 'break-all',
              lineHeight: 1.6,
            }}
          >{item}</span>
          <button
            onClick={() => onChange(value.filter((_, j) => j !== i))}
            className="flex-shrink-0 flex items-center justify-center w-8 h-8 rounded-lg transition-all hover:opacity-70"
            style={{ background: 'rgba(239,68,68,0.1)', color: '#ef4444', border: 'none' }}
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
              <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
            </svg>
          </button>
        </div>
      ))}
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

/** 自定义识别词格式帮助（可折叠） */
function CustomWordsHelp() {
  const [open, setOpen] = useState(false)

  const formats = [
    {
      tag: '屏蔽词', color: '#ef4444', bg: 'rgba(239,68,68,0.1)',
      desc: '直接写词，从文件名中删除',
      syntax: '关键词',
      examples: [
        { rule: '国语配音', effect: '「某剧 国语配音 S01E01.mkv」→「某剧 S01E01.mkv」' },
        { rule: '港剧',     effect: '「港剧 名侦探 01.mp4」→「名侦探 01.mp4」' },
      ],
    },
    {
      tag: '替换词', color: '#f59e0b', bg: 'rgba(245,158,11,0.1)',
      desc: '把旧词替换成新词，支持正则表达式',
      syntax: '旧词 => 新词',
      examples: [
        { rule: '港剧 => ',           effect: '替换为空 = 屏蔽' },
        { rule: 'OVA => SP',          effect: '「OVA 01」→「SP 01」' },
        { rule: '第(\\d+)话 => E\\1', effect: '「第12话」→「E12」（正则捕获组）' },
      ],
    },
    {
      tag: '集偏移', color: '#3b82f6', bg: 'rgba(59,130,246,0.1)',
      desc: '用前后缀夹住集号数字，修正集号偏差',
      syntax: '前缀 <> 后缀 >> EP+偏移量',
      examples: [
        { rule: '第 <> 话 >> EP+0',   effect: '「第12话」→ E12（无偏移）' },
        { rule: '第 <> 集 >> EP-1',   effect: '「第2集」→ E01（集号减1）' },
        { rule: 'Ep <> End >> EP+12', effect: '「Ep01End」→ E13（集号加12）' },
      ],
    },
    {
      tag: '组合', color: '#8b5cf6', bg: 'rgba(139,92,246,0.1)',
      desc: '先替换再偏移，用 && 连接两段规则',
      syntax: '旧词 => 新词 && 前缀 <> 后缀 >> EP+偏移量',
      examples: [
        { rule: 'OVA => SP && SP <> . >> EP+100', effect: '「OVA 01.」→ SP，集号+100' },
      ],
    },
  ]

  return (
    <div className="mt-2">
      <button
        type="button"
        onClick={() => setOpen(o => !o)}
        className="text-xs flex items-center gap-1 transition-opacity hover:opacity-70"
        style={{ color: 'var(--color-accent)', background: 'none', border: 'none', cursor: 'pointer', padding: 0 }}
      >
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5"
          style={{ transform: open ? 'rotate(90deg)' : 'rotate(0deg)', transition: 'transform 0.15s' }}>
          <polyline points="9 18 15 12 9 6"/>
        </svg>
        {open ? '收起格式说明' : '查看格式说明'}
      </button>

      {open && (
        <div className="mt-3 rounded-lg overflow-hidden"
          style={{ border: '1px solid var(--color-border)', background: 'var(--color-surface)' }}>
          {formats.map((f, fi) => (
            <div key={fi} className="px-4 py-3"
              style={{ borderBottom: fi < formats.length - 1 ? '1px solid var(--color-border)' : 'none' }}>
              {/* 类型标题行 */}
              <div className="flex items-center gap-2 mb-1.5">
                <span className="text-xs font-semibold px-2 py-0.5 rounded"
                  style={{ background: f.bg, color: f.color }}>{f.tag}</span>
                <span className="text-xs" style={{ color: 'var(--color-muted)' }}>{f.desc}</span>
              </div>
              {/* 语法 */}
              <code className="block text-xs font-mono mb-2 px-2 py-1 rounded"
                style={{ background: 'rgba(255,255,255,0.04)', color: 'var(--color-text)' }}>{f.syntax}</code>
              {/* 示例 */}
              <div className="space-y-1">
                {f.examples.map((ex, ei) => (
                  <div key={ei} className="flex items-baseline gap-2">
                    <code className="text-xs font-mono flex-shrink-0 px-1.5 py-0.5 rounded"
                      style={{ background: f.bg, color: f.color, whiteSpace: 'nowrap' }}>{ex.rule}</code>
                    <span className="text-xs" style={{ color: 'var(--color-muted)' }}>{ex.effect}</span>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ─── 主页面 ──────────────────────────────────────────────

export default function ConfigPage() {
  const [cfg, setCfg]         = useState(null)
  const [original, setOriginal] = useState(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving]   = useState(false)
  const [error, setError]     = useState(null)
  const [saved, setSaved]     = useState(false)

  useEffect(() => {
    setLoading(true)
    getConfig()
      .then(r => { setCfg(r.data); setOriginal(r.data) })
      .catch(e => setError(e?.response?.data?.detail || e.message))
      .finally(() => setLoading(false))
  }, [])

  const isDirty = JSON.stringify(cfg) !== JSON.stringify(original)

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
      await saveConfig(cfg)
      setOriginal(cfg)
      setSaved(true)
      setTimeout(() => setSaved(false), 3000)
    } catch (e) {
      setError(e?.response?.data?.detail || e.message)
    } finally {
      setSaving(false)
    }
  }

  if (loading) return (
    <div className="flex-1 space-y-4">
      {[1,2,3].map(i => (
        <div key={i} className="rounded-xl animate-pulse h-32"
          style={{ background: 'var(--color-surface-2)' }} />
      ))}
    </div>
  )

  return (
    <div className="flex-1 flex flex-col min-w-0" style={{ maxWidth: 1440 }}>
      {/* 页头 */}
      <div className="flex flex-wrap items-center gap-4 mb-6">
        <div>
          <h1 className="text-xl font-bold" style={{ color: 'var(--color-text)' }}>配置文件</h1>
          <p className="text-sm mt-0.5" style={{ color: 'var(--color-muted)' }}>
            维护媒体库、元数据与下载服务相关配置
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
              <polyline points="23 4 23 10 17 10"/>
              <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/>
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

      {/* ── 所有区块：两列网格（小屏单列） ── */}
      <div className="config-grid">
        {/* 左列 */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 0 }}>
          {/* ── WebUI 认证 ── */}
          <Section title="WebUI 认证">
            <FieldRow label="用户名" description="登录账号，默认 admin">
              <TextInput value={cfg?.webui?.username} onChange={v => set('webui','username',v)} placeholder="admin" />
            </FieldRow>
            <FieldRow label="密码" description="留空则允许任何人访问（不推荐）">
              <TextInput value={cfg?.webui?.password} type="password" onChange={v => set('webui','password',v)} placeholder="设置一个强密码" />
            </FieldRow>
            <FieldRow label="Token 有效期（小时）" description="默认 24 小时，重启服务不影响有效 token">
              <NumberInput value={cfg?.webui?.token_expire_hours} onChange={v => set('webui','token_expire_hours',v)} min={1} max={8760} />
            </FieldRow>
            <FieldRow label="Webhook 密钥" description="/trigger 端点校验密钥，空则不校验（仅内网时留空）">
              <TextInput value={cfg?.webui?.webhook_secret} type="password" onChange={v => set('webui','webhook_secret',v)} placeholder="留空则不校验" mono />
            </FieldRow>
            <FieldRow label="日志保留天数" description="日志按天分文件保存，超过这个天数会自动清理">
              <NumberInput value={cfg?.webui?.log_retention_days} onChange={v => set('webui','log_retention_days',v)} min={1} max={365} />
            </FieldRow>
          </Section>

          {/* ── TMDB ── */}
          <Section title="TMDB 设置">
            <FieldRow label="API Key" description="TMDB v3 API Key，必填">
              <TextInput value={cfg?.tmdb?.api_key} type="password"
                onChange={v => set('tmdb','api_key',v)} placeholder="0ec3b170d4c..." mono />
            </FieldRow>
            <FieldRow label="返回语言" description="如 zh-CN、en-US、ja-JP">
              <TextInput value={cfg?.tmdb?.language} onChange={v => set('tmdb','language',v)} placeholder="zh-CN" />
            </FieldRow>
            <FieldRow label="HTTP 代理" description="可选，如 http://127.0.0.1:7890">
              <TextInput value={cfg?.tmdb?.proxy} onChange={v => set('tmdb','proxy',v)} placeholder="留空则不使用代理" mono />
            </FieldRow>
            <FieldRow label="请求超时（秒）">
              <NumberInput value={cfg?.tmdb?.timeout} onChange={v => set('tmdb','timeout',v)} min={1} max={120} />
            </FieldRow>
          </Section>

          {/* ── Google Drive ── */}
          <Section title="Google Drive 设置">
            <FieldRow label="认证方式">
              <SelectInput value={cfg?.drive?.auth_mode}
                onChange={v => set('drive','auth_mode',v)}
                options={[
                  { value: 'oauth2',           label: 'OAuth2（个人账号）' },
                  { value: 'service_account',  label: 'Service Account（服务器）' },
                ]} />
            </FieldRow>
            <FieldRow label="OAuth2 凭据路径" description="credentials.json 文件路径">
              <TextInput value={cfg?.drive?.credentials_json} onChange={v => set('drive','credentials_json',v)} placeholder="config/credentials.json" mono />
            </FieldRow>
            <FieldRow label="OAuth2 Token 路径" description="首次授权后自动生成">
              <TextInput value={cfg?.drive?.token_json} onChange={v => set('drive','token_json',v)} placeholder="config/token.json" mono />
            </FieldRow>
            <FieldRow label="Service Account JSON" description="auth_mode = service_account 时使用">
              <TextInput value={cfg?.drive?.service_account_json} onChange={v => set('drive','service_account_json',v)} placeholder="config/service_account.json" mono />
            </FieldRow>
            <FieldRow label="扫描目录 ID" description="Drive 目标文件夹 ID">
              <TextInput value={cfg?.drive?.scan_folder_id} onChange={v => set('drive','scan_folder_id',v)} placeholder="1AbCdEfGhIjKlMn..." mono />
            </FieldRow>
          </Section>

          {/* ── 扫描与整理 ── */}
          <Section title="扫描与入库策略">
            <FieldRow label="跳过 TMDB 查询" description="开启后只整理文件夹，不生成 NFO 元数据">
              <Toggle value={cfg?.pipeline?.skip_tmdb} onChange={v => set('pipeline','skip_tmdb',v)} />
            </FieldRow>
            <FieldRow label="TMDB 未找到时仍移动" description="关闭则找不到元数据时跳过该文件">
              <Toggle value={cfg?.pipeline?.move_on_tmdb_miss} onChange={v => set('pipeline','move_on_tmdb_miss',v)} />
            </FieldRow>
            <FieldRow label="Dry Run 模式" description="只打印计划，不实际操作 Drive">
              <Toggle value={cfg?.pipeline?.dry_run} onChange={v => set('pipeline','dry_run',v)} />
            </FieldRow>
          </Section>
        </div>

        {/* 右列 */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 0 }}>
          {/* ── 解析器 ── */}
          <Section title="文件名识别规则">
            <FieldRow label="自定义识别词" description="每条一条规则，支持 4 种格式">
              <ListField value={cfg?.parser?.custom_words ?? []} onChange={v => set('parser','custom_words',v)} />
              <CustomWordsHelp />
            </FieldRow>
            <FieldRow label="自定义字幕组" description="补充内置列表没有的字幕组名称">
              <ListField value={cfg?.parser?.custom_release_groups ?? []} onChange={v => set('parser','custom_release_groups',v)} />
            </FieldRow>
          </Section>

          {/* ── 整理器 ── */}
          <Section title="媒体库目录映射">
            <FieldRow label="资料馆根目录 ID" description="电影和剧集默认归档到这个顶层文件夹">
              <TextInput value={cfg?.organizer?.root_folder_id} onChange={v => set('organizer','root_folder_id',v)} placeholder="Drive 文件夹 ID" mono />
            </FieldRow>
            <FieldRow label="电影归档目录 ID" description="留空则直接使用资料馆根目录">
              <TextInput value={cfg?.organizer?.movie_root_id} onChange={v => set('organizer','movie_root_id',v)} placeholder="留空同根目录" mono />
            </FieldRow>
            <FieldRow label="剧集归档目录 ID" description="留空则直接使用资料馆根目录">
              <TextInput value={cfg?.organizer?.tv_root_id} onChange={v => set('organizer','tv_root_id',v)} placeholder="留空同根目录" mono />
            </FieldRow>
          </Section>

          <Section title="Aria2 下载设置">
            <FieldRow label="RPC 主机" description="通常是 aria2 服务所在机器的 IP 或域名">
              <TextInput value={cfg?.aria2?.host} onChange={v => set('aria2','host',v)} placeholder="127.0.0.1" mono />
            </FieldRow>
            <FieldRow label="RPC 端口">
              <NumberInput value={cfg?.aria2?.port} onChange={v => set('aria2','port',v)} min={1} max={65535} />
            </FieldRow>
            <FieldRow label="RPC 路径" description="默认 /jsonrpc">
              <TextInput value={cfg?.aria2?.path} onChange={v => set('aria2','path',v)} placeholder="/jsonrpc" mono />
            </FieldRow>
            <FieldRow label="RPC 密钥" description="对应 aria2 的 rpc-secret">
              <TextInput value={cfg?.aria2?.secret} type="password" onChange={v => set('aria2','secret',v)} placeholder="留空表示无密钥" mono />
            </FieldRow>
            <FieldRow label="使用 HTTPS" description="如果 aria2 RPC 通过 HTTPS 暴露则开启">
              <Toggle value={cfg?.aria2?.secure} onChange={v => set('aria2','secure',v)} />
            </FieldRow>
          </Section>

          {/* ── Telegram ── */}
          <Section title="Telegram 通知">
            <FieldRow label="Bot Token" description="从 @BotFather 获取">
              <TextInput value={cfg?.telegram?.bot_token} type="password" onChange={v => set('telegram','bot_token',v)} placeholder="123456:ABC..." mono />
            </FieldRow>
            <FieldRow label="Chat ID" description="接收通知的账号或群组 ID">
              <TextInput value={cfg?.telegram?.chat_id} onChange={v => set('telegram','chat_id',v)} placeholder="371338215" mono />
            </FieldRow>
            <FieldRow label="防抖延时（秒）" description="批量入库时合并通知，0 = 立即触发">
              <NumberInput value={cfg?.telegram?.debounce_seconds} onChange={v => set('telegram','debounce_seconds',v)} min={0} max={600} />
            </FieldRow>
          </Section>

        </div>
      </div>
    </div>
  )
}
