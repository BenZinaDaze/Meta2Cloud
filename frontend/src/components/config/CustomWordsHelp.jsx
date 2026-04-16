import { useState } from 'react'

import { filenameRuleFormats } from './filenameRuleFormats'

export default function CustomWordsHelp({ defaultOpen = false }) {
  const [open, setOpen] = useState(defaultOpen)

  return (
    <div className="mt-2">
      <button
        type="button"
        onClick={() => setOpen(o => !o)}
        className="text-xs flex items-center gap-1 transition-opacity hover:opacity-70"
        style={{ color: 'var(--color-accent)', background: 'none', border: 'none', cursor: 'pointer', padding: 0 }}
      >
        <svg
          width="12"
          height="12"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2.5"
          style={{ transform: open ? 'rotate(90deg)' : 'rotate(0deg)', transition: 'transform 0.15s' }}
        >
          <polyline points="9 18 15 12 9 6" />
        </svg>
        {open ? '收起格式说明' : '查看格式说明'}
      </button>

      {open && (
        <div
          className="mt-3 rounded-lg overflow-hidden"
          style={{ border: '1px solid var(--color-border)', background: 'var(--color-surface)' }}
        >
          {filenameRuleFormats.map((format, index) => (
            <div
              key={format.tag}
              className="px-4 py-3"
              style={{
                borderBottom: index < filenameRuleFormats.length - 1 ? '1px solid var(--color-border)' : 'none',
              }}
            >
              <div className="flex items-center gap-2 mb-1.5">
                <span
                  className="text-xs font-semibold px-2 py-0.5 rounded"
                  style={{ background: format.bg, color: format.color }}
                >
                  {format.tag}
                </span>
                <span className="text-xs" style={{ color: 'var(--color-muted)' }}>{format.desc}</span>
              </div>

              <code
                className="block text-xs font-mono mb-2 px-2 py-1 rounded"
                style={{ background: 'rgba(255,255,255,0.04)', color: 'var(--color-text)' }}
              >
                {format.syntax}
              </code>

              <div className="space-y-1">
                {format.examples.map(example => (
                  <div key={example.rule} className="flex items-baseline gap-2">
                    <code
                      className="text-xs font-mono flex-shrink-0 px-1.5 py-0.5 rounded"
                      style={{ background: format.bg, color: format.color, whiteSpace: 'nowrap' }}
                    >
                      {example.rule}
                    </code>
                    <span className="text-xs" style={{ color: 'var(--color-muted)' }}>{example.effect}</span>
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
