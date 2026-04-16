import { useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import {
  addAria2Torrent,
  addAria2Uri,
  getAria2Overview,
  pauseAria2Tasks,
  purgeAria2Tasks,
  removeAria2Tasks,
  retryAria2Tasks,
  unpauseAria2Tasks,
} from '../api'
import ParseTestModal from './ParseTestModal'
import { StatePanel } from './StatePanel'

function getAria2ErrorMessage(error) {
  const detail = error?.response?.data?.detail || error?.message || '下载中心加载失败'
  if (detail === 'Aria2 集成未启用') {
    return 'Aria2 已在服务端配置中禁用。请前往配置页启用后再使用下载中心。'
  }
  return detail
}

function formatBytes(bytes) {
  const value = Number(bytes) || 0
  if (value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  const size = value / 1024 ** index
  return `${size >= 100 ? size.toFixed(0) : size.toFixed(1)} ${units[index]}`
}

function formatSpeed(bytes) {
  return `${formatBytes(bytes)}/s`
}

function getFileName(path) {
  if (!path) return ''
  const normalized = String(path).replaceAll('\\', '/')
  return normalized.split('/').filter(Boolean).pop() || normalized
}

function statusLabel(status) {
  return {
    active: '下载中',
    waiting: '队列中',
    paused: '已暂停',
    complete: '已完成',
    error: '失败',
    removed: '已移除',
    stopped: '已停止',
  }[status] || status
}

function SummaryCard({ label, value, sub }) {
  return (
    <div
      className="relative flex min-h-[132px] flex-col justify-between gap-3 rounded-[24px] px-5 py-5"
      style={{
        background: 'linear-gradient(180deg, rgba(20, 37, 59, 0.96) 0%, rgba(14, 28, 46, 0.98) 100%)',
        border: '1px solid var(--color-border)',
        boxShadow: 'var(--shadow-soft)',
      }}
    >
      <div className="space-y-2">
        <span className="text-[11px] font-semibold uppercase tracking-[0.18em]" style={{ color: 'var(--color-muted-soft)' }}>{label}</span>
        <span className="block text-3xl font-bold tabular-nums" style={{ color: 'var(--color-text)' }}>{value}</span>
        {sub && <span className="block text-xs leading-5" style={{ color: 'var(--color-muted)' }}>{sub}</span>}
      </div>
    </div>
  )
}

function ToolButton({ children, onClick, danger = false, disabled = false, loading = false }) {
  return (
    <button
      onClick={onClick}
      disabled={disabled || loading}
      className="min-h-11 rounded-full px-3.5 py-2 text-xs font-semibold transition-all disabled:cursor-not-allowed disabled:opacity-40"
      style={{
        background: danger ? 'rgba(239, 125, 117, 0.12)' : 'rgba(255,255,255,0.03)',
        border: danger ? '1px solid rgba(239, 125, 117, 0.28)' : '1px solid var(--color-border)',
        color: danger ? 'var(--color-danger)' : 'var(--color-text)',
      }}
    >
      <span className="inline-flex items-center gap-1.5">
        {loading ? (
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" style={{ animation: 'spin 1s linear infinite' }}>
            <path d="M21 12a9 9 0 1 1-6.219-8.56" />
          </svg>
        ) : null}
        {children}
      </span>
    </button>
  )
}

function QueueTab({ active, label, count, onClick }) {
  return (
    <button
      onClick={onClick}
      className="min-h-11 rounded-full px-4 py-2 text-sm font-medium transition-all duration-150"
      style={{
        color: active ? 'var(--color-accent-hover)' : 'var(--color-muted)',
        background: active ? 'rgba(200, 146, 77, 0.14)' : 'rgba(255,255,255,0.03)',
        border: active ? '1px solid rgba(200, 146, 77, 0.22)' : '1px solid var(--color-border)',
      }}
    >
      {label} · {count}
    </button>
  )
}

function DetailRow({ label, value }) {
  return (
    <div className="rounded-2xl px-4 py-3" style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)' }}>
      <div className="text-[11px] uppercase tracking-[0.16em]" style={{ color: 'var(--color-muted-soft)' }}>{label}</div>
      <div className="mt-1 text-sm break-all" style={{ color: 'var(--color-text)' }}>{value || '-'}</div>
    </div>
  )
}

function ConfirmModal({ open, title, message, confirmLabel, onConfirm, onCancel, loading = false }) {
  if (!open) return null

  return createPortal(
    <div
      className="fixed inset-0 z-[130] flex items-center justify-center px-5"
      style={{ background: 'rgba(2,8,18,0.78)', backdropFilter: 'blur(10px)', WebkitBackdropFilter: 'blur(10px)' }}
      onClick={loading ? undefined : onCancel}
    >
      <div
        className="panel-surface w-full max-w-[520px] rounded-[28px] p-6"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="text-[11px] font-semibold uppercase tracking-[0.22em]" style={{ color: 'var(--color-danger)' }}>
          Confirm Action
        </div>
        <h3 className="mt-3 text-[28px] leading-tight" style={{ color: 'var(--color-text)' }}>{title}</h3>
        <p className="mt-3 text-sm leading-7" style={{ color: 'var(--color-muted)' }}>{message}</p>

        <div className="mt-6 flex items-center justify-end gap-3">
          <button
            onClick={onCancel}
            disabled={loading}
            className="rounded-full px-4 py-2 text-sm font-semibold disabled:opacity-40"
            style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)', color: 'var(--color-text)' }}
          >
            取消
          </button>
          <ToolButton danger onClick={onConfirm} loading={loading}>
            {loading ? '移除中…' : confirmLabel}
          </ToolButton>
        </div>
      </div>
    </div>,
    document.body
  )
}

function TaskDetailModal({ task, onClose, onPause, onResume, onRemove, onRetry, pendingAction, onParseFile }) {
  if (!task) return null

  const errored = task.status === 'error'
  const pausable = task.status === 'active' || task.status === 'waiting'
  const resumable = task.status === 'paused'
  const showPause = pendingAction ? pendingAction === 'pause' : pausable
  const showResume = pendingAction ? pendingAction === 'resume' : resumable
  const showRetry = pendingAction ? pendingAction === 'retry' : errored

  return createPortal(
    <div
      className="fixed inset-0 z-[120] flex items-end justify-center overflow-y-auto p-0 sm:items-start sm:p-4 sm:pt-24"
      style={{ background: 'rgba(2,8,18,0.78)', backdropFilter: 'blur(10px)', WebkitBackdropFilter: 'blur(10px)' }}
      onClick={(e) => e.target === e.currentTarget && onClose()}
    >
      <div
        className="relative flex w-full max-w-[1180px] flex-col overflow-hidden rounded-t-[28px] sm:rounded-[32px]"
        style={{
          background: 'linear-gradient(180deg, rgba(15,27,45,0.98) 0%, rgba(11,22,37,0.98) 100%)',
          border: '1px solid var(--color-border)',
          boxShadow: 'var(--shadow-strong)',
          maxHeight: 'calc(100dvh - env(safe-area-inset-top) - 0.75rem)',
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="border-b px-4 py-4 sm:px-6 sm:py-5" style={{ borderColor: 'var(--color-border)' }}>
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="text-[11px] font-semibold uppercase tracking-[0.22em]" style={{ color: 'var(--color-accent-hover)' }}>
                Download Detail
              </div>
              <h2 className="mt-2 line-clamp-2 text-base font-semibold leading-snug sm:text-xl" style={{ color: 'var(--color-text)' }}>{task.name}</h2>
              <p className="mt-1 text-xs" style={{ color: 'var(--color-muted)' }}>{task.dir || '未指定目录'}</p>
            </div>
            <button
              type="button"
              onClick={onClose}
              className="flex size-11 items-center justify-center rounded-2xl transition-all duration-150"
              style={{ background: 'rgba(255,255,255,0.04)', border: '1px solid var(--color-border)', color: 'var(--color-text)' }}
              aria-label="关闭下载详情"
            >
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round">
                <path d="M6 6l12 12" />
                <path d="M18 6L6 18" />
              </svg>
            </button>
          </div>
        </div>

        <div className="overflow-y-auto px-4 py-4 sm:px-6 sm:py-6" style={{ paddingBottom: 'calc(env(safe-area-inset-bottom) + 1rem)' }}>
          <div className="mb-4">
            <div className="mb-1.5 flex flex-wrap items-center gap-2">
              <span className="rounded-full px-2.5 py-0.5 text-[11px] font-semibold uppercase tracking-[0.14em]"
                style={{
                  background: errored ? 'rgba(239,125,117,0.12)' : 'rgba(200,146,77,0.12)',
                  color: errored ? 'var(--color-danger)' : 'var(--color-accent-hover)',
                }}>
                {statusLabel(task.status)}
              </span>
              <span className="text-xs font-mono" style={{ color: 'var(--color-muted-soft)' }}>{task.gid}</span>
            </div>
          </div>

          <div className="mb-4 grid grid-cols-2 gap-2 sm:flex sm:flex-wrap">
            {showPause  && <ToolButton onClick={() => onPause(task.gid)}  loading={pendingAction === 'pause'} >{pendingAction === 'pause'  ? '暂停中…' : '暂停'}</ToolButton>}
            {showResume && <ToolButton onClick={() => onResume(task.gid)} loading={pendingAction === 'resume'}>{pendingAction === 'resume' ? '继续中…' : '继续'}</ToolButton>}
            {showRetry  && <ToolButton onClick={() => onRetry(task.gid)}  loading={pendingAction === 'retry'} >{pendingAction === 'retry'  ? '重试中…' : '重试'}</ToolButton>}
            <ToolButton danger onClick={() => onRemove(task.gid)} loading={pendingAction === 'remove'}>{pendingAction === 'remove' ? '移除中…' : '移除'}</ToolButton>
          </div>

          {/* 进度条 */}
          <div className="mb-5">
            <div className="mb-1.5 flex justify-between text-xs" style={{ color: 'var(--color-muted)' }}>
              <span>{formatBytes(task.completedLength)} / {formatBytes(task.totalLength)}</span>
              <span>{task.progress.toFixed(1)}%</span>
            </div>
            <div className="h-1.5 overflow-hidden rounded-full" style={{ background: 'rgba(255,255,255,0.06)' }}>
              <div className="h-full rounded-full" style={{
                width: `${Math.max(task.progress, task.status === 'complete' ? 100 : 0)}%`,
                background: errored
                  ? 'linear-gradient(90deg,rgba(239,125,117,.9),rgba(239,125,117,.65))'
                  : 'linear-gradient(90deg,var(--color-accent),#dfb36f)',
              }} />
            </div>
          </div>

          {/* 详情网格 */}
          <div className="mb-5 grid grid-cols-2 gap-2 sm:grid-cols-3 sm:gap-3">
            <DetailRow label="下载速度" value={formatSpeed(task.downloadSpeed)} />
            <DetailRow label="上传速度" value={formatSpeed(task.uploadSpeed)} />
            <DetailRow label="连接数"   value={String(task.connections)} />
            <DetailRow label="文件数"   value={String(task.fileCount)} />
            <DetailRow label="做种数"   value={String(task.numSeeders)} />
            <DetailRow label="已上传"   value={formatBytes(task.uploadLength)} />
            {task.bittorrent?.mode    ? <DetailRow label="种子模式" value={task.bittorrent.mode}    /> : null}
            {task.bittorrent?.comment ? <DetailRow label="种子备注" value={task.bittorrent.comment} /> : null}
          </div>

          {task.errorMessage && (
            <div className="mb-5 rounded-2xl px-4 py-3 text-sm"
              style={{ background: 'rgba(239,125,117,0.08)', border: '1px solid rgba(239,125,117,0.2)', color: 'var(--color-danger)' }}>
              {task.errorMessage}
            </div>
          )}

          {task.files?.length ? (
            <section className="mb-5">
              <div className="mb-2 text-[11px] font-semibold uppercase tracking-[0.2em]" style={{ color: 'var(--color-accent-hover)' }}>
                文件列表
              </div>
              <div className="space-y-2">
                {task.files.map((file, index) => (
                  <div key={`${file.path}-${index}`} className="rounded-2xl px-4 py-3"
                    style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)' }}>
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0 break-all text-sm" style={{ color: 'var(--color-text)' }}>
                        {getFileName(file.path) || `文件 ${index + 1}`}
                      </div>
                      <button
                        onClick={(e) => { e.stopPropagation(); onParseFile?.(getFileName(file.path) || `文件 ${index + 1}`) }}
                        className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full transition-all hover:opacity-80"
                        style={{ background: 'rgba(255,255,255,0.04)', border: '1px solid var(--color-border)', color: 'var(--color-accent-hover)' }}
                        title="解析此文件"
                      >
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                          <circle cx="11" cy="11" r="7" /><line x1="21" y1="21" x2="16.65" y2="16.65" />
                        </svg>
                      </button>
                    </div>
                    <div className="mt-2 flex flex-wrap gap-4 text-xs" style={{ color: 'var(--color-muted)' }}>
                      <span>{formatBytes(file.completedLength)} / {formatBytes(file.length)}</span>
                      <span>{file.selected ? '已选择' : '未选择'}</span>
                    </div>
                  </div>
                ))}
              </div>
            </section>
          ) : null}

          {task.uris?.length ? (
            <section>
              <div className="mb-2 text-[11px] font-semibold uppercase tracking-[0.2em]" style={{ color: 'var(--color-accent-hover)' }}>
                来源链接
              </div>
              <div className="space-y-2">
                {task.uris.map((uri) => (
                  <div key={uri} className="rounded-2xl px-4 py-3 text-sm break-all"
                    style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)', color: 'var(--color-text)' }}>
                    {uri}
                  </div>
                ))}
              </div>
            </section>
          ) : null}
        </div>
      </div>
    </div>,
    document.body
  )
}

function TaskCard({ task, onPause, onResume, onRemove, onRetry, onOpen, pendingAction, selected, onToggleSelect }) {
  const errored = task.status === 'error'
  const pausable = task.status === 'active' || task.status === 'waiting'
  const resumable = task.status === 'paused'
  const showPause = pendingAction ? pendingAction === 'pause' : pausable
  const showResume = pendingAction ? pendingAction === 'resume' : resumable
  const showRetry = pendingAction ? pendingAction === 'retry' : errored

  return (
    <div
      className="rounded-[22px] px-4 py-4 transition-all duration-150 sm:rounded-[24px] sm:px-5 sm:py-5"
      onClick={() => onOpen(task)}
      style={{
        background: 'linear-gradient(180deg, rgba(20, 37, 59, 0.82) 0%, rgba(13, 24, 39, 0.96) 100%)',
        border: '1px solid var(--color-border)',
        cursor: 'pointer',
      }}
    >
      <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div className="min-w-0 flex-1 flex items-start gap-2 sm:gap-4">
          {/* 增大点击感应区（增加 padding，设置相对位置并阻止冒泡）保证选中不会误触详情 */}
          <div 
            className="flex-shrink-0 cursor-pointer p-3 -ml-3 -mt-2.5 transition-opacity hover:opacity-80" 
            onClick={e => { e.stopPropagation(); onToggleSelect(task.gid) }}
          >
            <div 
              className="flex items-center justify-center rounded-md transition-colors"
              style={{
                width: 20,
                height: 20,
                background: selected ? 'var(--color-accent)' : 'rgba(255,255,255,0.06)',
                border: selected ? 'none' : '1px solid var(--color-border)',
                color: '#fff',
                boxShadow: selected ? '0 0 10px rgba(200,146,77,0.3)' : 'none',
              }}
            >
              {selected && (
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round" className="size-3.5">
                  <polyline points="20 6 9 17 4 12"></polyline>
                </svg>
              )}
            </div>
          </div>
          <div className="min-w-0 flex-1">
            <div className="mb-2 flex flex-wrap items-center gap-2">
            <span className="rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.14em]"
              style={{
                background: errored ? 'rgba(239, 125, 117, 0.12)' : 'rgba(200, 146, 77, 0.12)',
                color: errored ? 'var(--color-danger)' : 'var(--color-accent-hover)',
              }}>
              {statusLabel(task.status)}
            </span>
            <span className="text-xs font-mono" style={{ color: 'var(--color-muted-soft)' }}>{task.gid}</span>
          </div>

          <div className="line-clamp-2 text-base font-semibold leading-snug sm:text-lg" style={{ color: 'var(--color-text)' }}>{task.name}</div>
          <div className="mt-1.5 line-clamp-2 text-xs leading-5 sm:mt-2 sm:text-sm sm:leading-6" style={{ color: 'var(--color-muted)' }}>
            {task.dir || '未指定目录'}
          </div>

          <div className="mt-3 sm:mt-4">
            <div className="mb-2 flex items-center justify-between text-xs" style={{ color: 'var(--color-muted)' }}>
              <span>{formatBytes(task.completedLength)} / {formatBytes(task.totalLength)}</span>
              <span>{task.progress.toFixed(1)}%</span>
            </div>
            <div className="h-2 overflow-hidden rounded-full" style={{ background: 'rgba(255,255,255,0.06)' }}>
              <div
                className="h-full rounded-full"
                style={{
                  width: `${Math.max(task.progress, task.status === 'complete' ? 100 : 0)}%`,
                  background: errored
                    ? 'linear-gradient(90deg, rgba(239,125,117,0.9) 0%, rgba(239,125,117,0.65) 100%)'
                    : 'linear-gradient(90deg, var(--color-accent) 0%, #dfb36f 100%)',
                }}
              />
            </div>
          </div>

          <div className="mt-3 grid grid-cols-2 gap-2 text-sm sm:mt-4 sm:gap-3 md:grid-cols-4">
            <div className="rounded-2xl px-3 py-2.5" style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.04)' }}>
              <div className="text-[11px]" style={{ color: 'var(--color-muted-soft)' }}>下载速度</div>
              <div className="mt-1 text-sm font-semibold" style={{ color: 'var(--color-text)' }}>{formatSpeed(task.downloadSpeed)}</div>
            </div>
            <div className="hidden rounded-2xl px-3 py-2.5 md:block" style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.04)' }}>
              <div className="text-[11px]" style={{ color: 'var(--color-muted-soft)' }}>上传速度</div>
              <div className="mt-1 text-sm font-semibold" style={{ color: 'var(--color-text)' }}>{formatSpeed(task.uploadSpeed)}</div>
            </div>
            <div className="rounded-2xl px-3 py-2.5" style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.04)' }}>
              <div className="text-[11px]" style={{ color: 'var(--color-muted-soft)' }}>文件数</div>
              <div className="mt-1 text-sm font-semibold" style={{ color: 'var(--color-text)' }}>{task.fileCount}</div>
            </div>
            <div className="hidden rounded-2xl px-3 py-2.5 md:block" style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.04)' }}>
              <div className="text-[11px]" style={{ color: 'var(--color-muted-soft)' }}>连接数</div>
              <div className="mt-1 text-sm font-semibold" style={{ color: 'var(--color-text)' }}>{task.connections}</div>
            </div>
          </div>

          {task.errorMessage && (
            <div className="mt-4 rounded-2xl px-4 py-3 text-sm"
              style={{ background: 'rgba(239,125,117,0.08)', border: '1px solid rgba(239,125,117,0.2)', color: 'var(--color-danger)' }}>
              {task.errorMessage}
            </div>
          )}
          </div>
        </div>

        <div className="grid grid-cols-2 gap-2 sm:flex sm:flex-wrap xl:max-w-[220px] xl:justify-end">
          {showPause && <ToolButton onClick={(e) => { e.stopPropagation(); onPause(task.gid) }} loading={pendingAction === 'pause'}>{pendingAction === 'pause' ? '暂停中…' : '暂停'}</ToolButton>}
          {showResume && <ToolButton onClick={(e) => { e.stopPropagation(); onResume(task.gid) }} loading={pendingAction === 'resume'}>{pendingAction === 'resume' ? '继续中…' : '继续'}</ToolButton>}
          {showRetry && <ToolButton onClick={(e) => { e.stopPropagation(); onRetry(task.gid) }} loading={pendingAction === 'retry'}>{pendingAction === 'retry' ? '重试中…' : '重试'}</ToolButton>}
          <ToolButton danger onClick={(e) => { e.stopPropagation(); onRemove(task.gid) }} loading={pendingAction === 'remove'}>{pendingAction === 'remove' ? '移除中…' : '移除'}</ToolButton>
        </div>
      </div>
    </div>
  )
}

function ActionPanel({ uriInput, setUriInput, onSubmitUri, onTorrentChange, torrentName }) {
  return (
    <div className="rounded-[24px] px-4 py-4 sm:rounded-[28px] sm:px-5 sm:py-5" style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)' }}>
      <div className="mb-3 text-[11px] font-semibold uppercase tracking-[0.2em]" style={{ color: 'var(--color-accent-hover)' }}>
        新建下载
      </div>
      <textarea
        value={uriInput}
        onChange={(e) => setUriInput(e.target.value)}
        placeholder="每行一个链接，支持 HTTP、HTTPS、FTP、磁力链"
        className="h-24 w-full rounded-[18px] px-4 py-3 text-sm outline-none"
        style={{
          background: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
          color: 'var(--color-text)',
          resize: 'none',
          overflowY: 'auto',
        }}
      />
      <div className="mt-3 flex flex-col gap-2.5 sm:flex-row sm:flex-wrap sm:items-center">
        <button
          onClick={onSubmitUri}
          className="min-h-11 rounded-full px-4 py-2 text-sm font-semibold"
          style={{ background: 'linear-gradient(135deg, var(--color-accent) 0%, #b37533 100%)', color: '#fff', border: 'none' }}
        >
          添加链接下载
        </button>

        <label
          className="inline-flex min-h-11 cursor-pointer items-center justify-center rounded-full px-4 py-2 text-sm font-semibold"
          style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid var(--color-border)', color: 'var(--color-text)' }}
        >
          上传 Torrent
          <input type="file" accept=".torrent,application/x-bittorrent" hidden onChange={onTorrentChange} />
        </label>

        {torrentName && <span className="text-xs" style={{ color: 'var(--color-muted)' }}>{torrentName}</span>}
      </div>
    </div>
  )
}

function PaginationBar({ pagination, onChange, busy = false }) {
  if (!pagination || pagination.total_pages <= 1) return null

  return (
    <div className="mt-6 flex flex-wrap items-center justify-between gap-3">
      <div className="text-xs" style={{ color: 'var(--color-muted)' }}>
        第 {pagination.page} / {pagination.total_pages} 页，共 {pagination.total} 条
      </div>
      <div className="flex items-center gap-2">
        <ToolButton onClick={() => onChange(pagination.page - 1)} disabled={busy || !pagination.has_prev}>上一页</ToolButton>
        <ToolButton onClick={() => onChange(pagination.page + 1)} disabled={busy || !pagination.has_next}>下一页</ToolButton>
      </div>
    </div>
  )
}

export default function DownloadsPage({ queue = 'all', onChangeQueue, onToast, initialOverview = null, aria2Enabled = null }) {
  const [overview, setOverview] = useState(initialOverview)
  const [uriInput, setUriInput] = useState('')
  const [torrentName, setTorrentName] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState(null)
  const [selectedTask, setSelectedTask] = useState(null)
  const [parseTestFile, setParseTestFile] = useState(null)
  const [pendingActions, setPendingActions] = useState({})
  const [confirmRemoveGids, setConfirmRemoveGids] = useState(null)
  const [selectedGids, setSelectedGids] = useState(new Set())
  const [searchQuery, setSearchQuery] = useState('')
  const [debouncedSearchQuery, setDebouncedSearchQuery] = useState('')
  const [page, setPage] = useState(1)
  const showDashboard = queue === 'all'

  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearchQuery(searchQuery)
    }, 250)
    return () => clearTimeout(timer)
  }, [searchQuery])

  useEffect(() => {
    if (initialOverview) {
      setOverview(initialOverview)
      setError(null)
      setLoading(false)
    } else if (aria2Enabled === null) {
      setOverview(null)
      setLoading(true)
    } else if (!aria2Enabled) {
      setOverview(null)
      setLoading(false)
    }
  }, [initialOverview, aria2Enabled])

  function applyOptimisticTaskUpdate(gid, patch) {
    setOverview((prev) => {
      if (!prev?.items) return prev
      if (!prev.items.some((item) => item.gid === gid)) {
        return prev
      }

      return {
        ...prev,
        items: prev.items.map((item) => (
          item.gid === gid ? { ...item, ...patch } : item
        )),
      }
    })
  }

  async function loadAll(silent = false) {
    try {
      if (!silent) setLoading(true)
      const overviewRes = await getAria2Overview({ queue, page, page_size: 20, search: debouncedSearchQuery.trim() || undefined })
      setOverview(overviewRes.data)
      setError(null)
    } catch (e) {
      setError(getAria2ErrorMessage(e))
    } finally {
      if (!silent) setLoading(false)
    }
  }

  const pollTimer = useRef(null)
  const errorCount = useRef(0)

  useEffect(() => {
    if (aria2Enabled === null) {
      setLoading(true)
      return
    }

    if (!aria2Enabled) {
      setLoading(false)
      return
    }

    let cancelled = false

    async function poll(silent = false) {
      try {
        if (!silent) setLoading(true)
        const overviewRes = await getAria2Overview({ queue, page, page_size: 20, search: debouncedSearchQuery.trim() || undefined })
        if (cancelled) return
        setOverview(overviewRes.data)
        setError(null)
        errorCount.current = 0          // 成功后重置退避计数器
      } catch (e) {
        if (cancelled) return
        setError(getAria2ErrorMessage(e))
        errorCount.current += 1
      } finally {
        if (!cancelled) {
          if (!silent) setLoading(false)
          // 逾退策略：0 次错=5s, 1次=10s, 2次=20s, ≥3次=30s
          const delays = [5000, 10000, 20000, 30000]
          const delay = delays[Math.min(errorCount.current, delays.length - 1)]
          pollTimer.current = setTimeout(() => poll(true), delay)
        }
      }
    }

    poll()
    return () => {
      cancelled = true
      if (pollTimer.current) clearTimeout(pollTimer.current)
    }
  }, [aria2Enabled, page, queue, debouncedSearchQuery])

  const tasks = useMemo(() => overview?.items || [], [overview])

  useEffect(() => {
    if (!selectedTask || !overview?.items) return
    const latest = overview.items.find((item) => item.gid === selectedTask.gid)
    if (latest) {
      setSelectedTask(latest)
    } else {
      setSelectedTask(null)
    }
  }, [overview, selectedTask])

  useEffect(() => {
    setPage(1)
    setSelectedGids(new Set())
  }, [queue])

  useEffect(() => {
    setPage(1)
    setSelectedGids(new Set())
  }, [debouncedSearchQuery])

  async function withAction(action, successMessage, gid = null, actionName = null, optimisticPatch = null) {
    try {
      setBusy(true)
      if (gid && actionName) {
        setPendingActions((prev) => ({ ...prev, [gid]: actionName }))
      }
      if (gid && optimisticPatch) {
        applyOptimisticTaskUpdate(gid, optimisticPatch)
      }
      await action()
      await loadAll(true)
      onToast?.('success', '下载管理', successMessage)
    } catch (e) {
      onToast?.('error', '下载管理', getAria2ErrorMessage(e))
    } finally {
      if (gid) {
        setPendingActions((prev) => {
          const next = { ...prev }
          delete next[gid]
          return next
        })
      }
      setBusy(false)
    }
  }

  function handleToggleSelect(gid) {
    setSelectedGids(prev => {
      const next = new Set(prev)
      if (next.has(gid)) next.delete(gid)
      else next.add(gid)
      return next
    })
  }

  function handleSelectAllGroup() {
    if (selectedGids.size === tasks.length && tasks.length > 0) {
      setSelectedGids(new Set())
    } else {
      setSelectedGids(new Set(tasks.map(t => t.gid)))
    }
  }

  function handleRemoveTask(gid) {
    setConfirmRemoveGids([gid])
  }

  function handleRemoveSelected() {
    if (!selectedGids.size) return
    setConfirmRemoveGids(Array.from(selectedGids))
  }

  async function confirmRemoveTask() {
    if (!confirmRemoveGids?.length) return
    const isSingle = confirmRemoveGids.length === 1
    const gid = isSingle ? confirmRemoveGids[0] : null
    await withAction(() => removeAria2Tasks(confirmRemoveGids), isSingle ? '任务已移除' : `已移除 ${confirmRemoveGids.length} 个任务`, gid, 'remove')
    setConfirmRemoveGids(null)
    setSelectedGids(new Set())
  }

  async function handleSubmitUri() {
    const uris = uriInput.split('\n').map(v => v.trim()).filter(Boolean)
    if (!uris.length) {
      onToast?.('warning', '下载管理', '请先输入至少一个下载链接')
      return
    }
    await withAction(async () => {
      await Promise.all(uris.map(uri => addAria2Uri({ uris: [uri] })))
      setUriInput('')
    }, `已添加下载任务`)
  }

  async function handleTorrentChange(event) {
    const file = event.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = async () => {
      const result = String(reader.result || '')
      const base64 = result.includes(',') ? result.split(',')[1] : result
      await withAction(async () => {
        await addAria2Torrent({ torrent: base64 })
        setTorrentName(file.name)
      }, '已添加 Torrent 任务')
    }
    reader.readAsDataURL(file)
  }

  const summary = overview?.summary

  const hasSelection = selectedGids.size > 0
  const selectedTasks = tasks.filter(t => selectedGids.has(t.gid))
  const canPauseSelected = selectedTasks.some(t => t.status === 'active' || t.status === 'waiting')
  const canResumeSelected = selectedTasks.some(t => t.status === 'paused')

  function handlePauseAction() {
    let gids = []
    if (hasSelection) {
      gids = selectedTasks.filter(t => t.status === 'active' || t.status === 'waiting').map(t => t.gid)
    } else {
      gids = tasks.filter(t => t.status === 'active' || t.status === 'waiting').map(t => t.gid)
    }
    if (!gids.length) return
    withAction(() => pauseAria2Tasks(gids), `已暂停 ${gids.length} 个任务`)
    setSelectedGids(new Set())
  }

  function handleResumeAction() {
    let gids = []
    if (hasSelection) {
      gids = selectedTasks.filter(t => t.status === 'paused').map(t => t.gid)
    } else {
      gids = tasks.filter(t => t.status === 'paused').map(t => t.gid)
    }
    if (!gids.length) return
    withAction(() => unpauseAria2Tasks(gids), `已继续 ${gids.length} 个任务`)
    setSelectedGids(new Set())
  }

  return (
    <div className="flex-1">
      <section className="page-panel panel-surface rounded-[22px] px-3 py-4 sm:rounded-[32px] sm:px-7 sm:py-7">
        <div className="mb-7 flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
          <div className="max-w-2xl">
            <div className="mb-2 text-[11px] font-semibold uppercase tracking-[0.24em]" style={{ color: 'var(--color-accent-hover)' }}>
              Native Download Center
            </div>
            <h1 className="text-[28px] font-bold leading-tight sm:text-[34px]" style={{ color: 'var(--color-text)' }}>
              {showDashboard ? '下载管理' : statusLabel(queue)}
            </h1>
          </div>

          <div className="-mx-1 flex flex-nowrap items-center gap-2 overflow-x-auto px-1 pb-1 sm:mx-0 sm:flex-wrap sm:overflow-visible sm:px-0">
            <ToolButton onClick={() => loadAll()} disabled={busy || !aria2Enabled}>刷新</ToolButton>
            {hasSelection ? (
              <label className="flex items-center gap-2.5 cursor-pointer text-sm font-medium mr-2 select-none hover:opacity-80 transition-opacity" style={{ color: 'var(--color-text)' }}>
                <div 
                  className="flex items-center justify-center rounded-md transition-colors"
                  style={{
                    width: 20,
                    height: 20,
                    background: (selectedGids.size === tasks.length && tasks.length > 0) ? 'var(--color-accent)' : 'rgba(255,255,255,0.06)',
                    border: (selectedGids.size === tasks.length && tasks.length > 0) ? 'none' : '1px solid var(--color-border)',
                    color: '#fff',
                    boxShadow: (selectedGids.size === tasks.length && tasks.length > 0) ? '0 0 10px rgba(200,146,77,0.3)' : 'none',
                  }}
                >
                  {(selectedGids.size === tasks.length && tasks.length > 0) && (
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round" className="size-3.5">
                      <polyline points="20 6 9 17 4 12"></polyline>
                    </svg>
                  )}
                </div>
                <input 
                  type="checkbox"
                  checked={selectedGids.size === tasks.length && tasks.length > 0}
                  onChange={handleSelectAllGroup}
                  className="hidden"
                />
                本页全选
              </label>
            ) : null}
            {(!hasSelection && tasks.some(t => t.status === 'active' || t.status === 'waiting')) || (hasSelection && canPauseSelected) ? (
              <ToolButton onClick={handlePauseAction} disabled={busy}>{hasSelection ? '暂停选中' : '暂停本页'}</ToolButton>
            ) : null}
            {(!hasSelection && tasks.some(t => t.status === 'paused')) || (hasSelection && canResumeSelected) ? (
              <ToolButton onClick={handleResumeAction} disabled={busy}>{hasSelection ? '继续选中' : '继续本页'}</ToolButton>
            ) : null}
            {hasSelection ? (
              <ToolButton danger onClick={handleRemoveSelected} disabled={busy}>删除选中</ToolButton>
            ) : null}
            {!hasSelection && showDashboard ? (
              <ToolButton danger onClick={() => withAction(() => purgeAria2Tasks(), '已清理已完成任务')} disabled={busy}>
                清理已完成
              </ToolButton>
            ) : null}
          </div>
        </div>

        {showDashboard ? (
          <div className="mb-8 grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <SummaryCard label="下载速度" value={summary ? formatSpeed(summary.downloadSpeed) : '--'} sub="当前全局吞吐" />
            <SummaryCard label="上传速度" value={summary ? formatSpeed(summary.uploadSpeed) : '--'} sub="适用于 BT 做种场景" />
            <SummaryCard label="活跃任务" value={summary?.activeCount ?? '--'} sub={`等待 ${summary?.waitingCount ?? '--'} · 已停止 ${summary?.stoppedCount ?? '--'}`} />
            <SummaryCard label="Aria2 版本" value={overview?.version?.version || '--'} sub={(overview?.version?.enabledFeatures || []).slice(0, 3).join(' · ') || '等待连接'} />
          </div>
        ) : null}

          <div className="mb-6 flex flex-col gap-4">
          <div className="-mx-1 flex flex-nowrap gap-2 overflow-x-auto px-1 pb-1 sm:mx-0 sm:flex-wrap sm:overflow-visible sm:px-0">
            <QueueTab active={queue === 'all'} label="全部" count={(summary?.activeCount || 0) + (summary?.waitingCount || 0) + (summary?.stoppedCount || 0)} onClick={() => onChangeQueue?.('all')} />
            <QueueTab active={queue === 'active'} label="下载中" count={summary?.activeCount || 0} onClick={() => onChangeQueue?.('active')} />
            <QueueTab active={queue === 'waiting'} label="等待中" count={summary?.waitingCount || 0} onClick={() => onChangeQueue?.('waiting')} />
            <QueueTab active={queue === 'stopped'} label="已停止" count={summary?.stoppedCount || 0} onClick={() => onChangeQueue?.('stopped')} />
          </div>
          
          <div className="w-full sm:max-w-xs">
            <input
              type="text"
              placeholder="搜索任务名称 或 GID..."
              value={searchQuery}
              onChange={e => setSearchQuery(e.target.value)}
              className="min-h-11 w-full rounded-full border px-4 py-2 text-sm outline-none transition-colors"
              style={{
                background: 'rgba(255,255,255,0.03)',
                borderColor: searchQuery ? 'var(--color-accent)' : 'var(--color-border)',
                color: 'var(--color-text)'
              }}
            />
          </div>
        </div>

        {showDashboard ? (
          <ActionPanel
            uriInput={uriInput}
            setUriInput={setUriInput}
            onSubmitUri={handleSubmitUri}
            onTorrentChange={handleTorrentChange}
            torrentName={torrentName}
          />
        ) : null}

        {loading && !overview && (
          <StatePanel
            title="正在连接 aria2"
            description="下载中心正在初始化任务状态和运行信息。"
            compact
          />
        )}

        {!loading && !error && !overview && !aria2Enabled && (
          <div className="mt-6">
            <StatePanel
              icon="⤓"
              title="Aria2 集成已关闭"
              description="请先在配置页启用 Aria2，再使用下载中心。"
              compact
            />
          </div>
        )}

        {error && (
          <div className="mt-6">
            <StatePanel
              icon="!"
              title={error}
              description="请检查 aria2 连接状态，或稍后重试。"
              tone="danger"
              compact
            />
          </div>
        )}

        {!loading && !error && (
          <div className={`${showDashboard ? 'mt-6' : 'mt-2'} space-y-4`}>
            {tasks.length === 0 ? (
              <StatePanel
                icon="⤓"
                title="当前分组没有任务"
                description="换一个任务分组，或者添加新的下载任务。"
              />
            ) : (
              tasks.map(task => (
                <TaskCard
                  key={task.gid}
                  task={task}
                  selected={selectedGids.has(task.gid)}
                  onToggleSelect={handleToggleSelect}
                  onOpen={setSelectedTask}
                  pendingAction={pendingActions[task.gid]}
                  onPause={(gid) => withAction(() => pauseAria2Tasks([gid]), '任务已暂停', gid, 'pause', { status: 'paused' })}
                  onResume={(gid) => withAction(() => unpauseAria2Tasks([gid]), '任务已继续', gid, 'resume', { status: 'active' })}
                  onRemove={handleRemoveTask}
                  onRetry={(gid) => withAction(() => retryAria2Tasks([gid]), '任务已重新加入队列', gid, 'retry')}
                />
              ))
            )}
          </div>
        )}

        <PaginationBar
          pagination={overview?.pagination}
          busy={busy}
          onChange={(nextPage) => {
            setSelectedGids(new Set())
            setPage(nextPage)
          }}
        />
      </section>

      <TaskDetailModal
        task={selectedTask}
        onClose={() => setSelectedTask(null)}
        pendingAction={selectedTask ? pendingActions[selectedTask.gid] : null}
        onPause={(gid) => withAction(() => pauseAria2Tasks([gid]), '任务已暂停', gid, 'pause', { status: 'paused' })}
        onResume={(gid) => withAction(() => unpauseAria2Tasks([gid]), '任务已继续', gid, 'resume', { status: 'active' })}
        onRemove={handleRemoveTask}
        onRetry={(gid) => withAction(() => retryAria2Tasks([gid]), '任务已重新加入队列', gid, 'retry')}
        onParseFile={setParseTestFile}
      />

      <ConfirmModal
        open={!!confirmRemoveGids}
        title={confirmRemoveGids?.length > 1 ? `移除 ${confirmRemoveGids.length} 个下载任务` : "移除下载任务"}
        message="已完成任务会从列表清除，未完成任务会停止下载。确认后将立即执行。"
        confirmLabel="确认移除"
        loading={confirmRemoveGids?.length === 1 ? pendingActions[confirmRemoveGids[0]] === 'remove' : false}
        onCancel={() => setConfirmRemoveGids(null)}
        onConfirm={confirmRemoveTask}
      />

      {parseTestFile ? (
        <ParseTestModal
          initialFilename={parseTestFile}
          onClose={() => setParseTestFile(null)}
        />
      ) : null}
    </div>
  )
}
