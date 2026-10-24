import { useState, useRef, useEffect } from 'react'
import { ExternalLink, Settings2, Play, Square, ScrollText, Trash2, MoreHorizontal, Lock, LockOpen, FileCode2, Globe, ArrowLeftRight, FolderOpen, Waypoints } from 'lucide-react'
import { cn } from '../../utils/cn'
import { compact } from '../../components/charts/scale'

export const TYPES = {
  static: { label: 'Static', icon: Globe, color: '#2a78d6', desc: 'HTML, CSS and JavaScript files' },
  php: { label: 'PHP', icon: FileCode2, color: '#777bb3', desc: 'WordPress, Laravel and other PHP apps' },
  proxy: { label: 'Reverse proxy', icon: ArrowLeftRight, color: '#1baf7a', desc: 'Forward to an app on another port' },
}

function Stat({ label, value, tone }) {
  return (
    <div className="min-w-0">
      <p className="text-[11px] text-text-tertiary">{label}</p>
      <p className={cn('truncate text-sm font-semibold tabular-nums', tone || 'text-text-primary')}>{value}</p>
    </div>
  )
}

function Menu({ items, onClose }) {
  const ref = useRef(null)
  useEffect(() => {
    const h = (e) => { if (!ref.current?.contains(e.target)) onClose() }
    document.addEventListener('mousedown', h)
    return () => document.removeEventListener('mousedown', h)
  }, [onClose])
  return (
    <div ref={ref} className="absolute right-0 top-9 z-20 w-44 rounded-md border border-surface-border bg-surface-card py-1 shadow-md">
      {items.map((it) => (
        <button key={it.label} onClick={() => { onClose(); it.run() }} className={cn('flex w-full items-center gap-2 px-3 py-1.5 text-left text-[13px] hover:bg-surface-hover', it.danger ? 'text-danger' : 'text-text-primary')}>
          <it.icon size={14} />{it.label}
        </button>
      ))}
    </div>
  )
}

// One website. The whole card opens the manager; the buttons act without opening it.
function SiteCard({ site: s, stats, daysLeft, busy, onManage, onToggle, onLogs, onDelete, onFiles }) {
  const [menu, setMenu] = useState(false)
  const T = TYPES[s.type] || TYPES.static
  const running = s.status === 'running'
  const target = s.type === 'proxy' ? s.proxy_target : s.root_dir
  const certTone = daysLeft == null ? '' : daysLeft < 7 ? 'text-danger' : daysLeft < 14 ? 'text-warning' : ''
  const url = `${s.ssl_enabled ? 'https' : 'http'}://${s.domain}`
  const errRate = stats?.requests ? (stats.server_err / stats.requests) * 100 : 0

  return (
    <article className={cn('group relative flex flex-col rounded-lg border bg-surface-card p-4 shadow-card transition-shadow hover:shadow-md', running ? 'border-surface-border' : 'border-surface-border opacity-75')}>
      <div className="flex items-start gap-3">
        <div className="relative flex h-11 w-11 shrink-0 items-center justify-center rounded-lg text-base font-bold uppercase" style={{ backgroundColor: `${T.color}22`, color: T.color }}>
          {s.domain.replace(/^www\./, '').charAt(0)}
          <span className={cn('absolute -bottom-0.5 -right-0.5 h-3 w-3 rounded-full border-2 border-surface-card', running ? 'bg-success' : 'bg-text-tertiary')} title={running ? 'Running' : 'Stopped'} />
        </div>
        <div className="min-w-0 flex-1">
          <button onClick={onManage} className="block max-w-full truncate text-left text-[15px] font-semibold text-text-primary hover:text-primary">{s.domain}</button>
          <div className="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-text-tertiary">
            <span className="inline-flex items-center gap-1"><T.icon size={12} />{T.label}{s.php_version ? ` ${s.php_version}` : ''}</span>
            {s.proxy_rules?.length > 0 && <span className="inline-flex items-center gap-1" title="Reverse proxy rules"><Waypoints size={12} />{s.proxy_rules.length} proxy</span>}
            {!running && <span className="rounded bg-surface-hover px-1.5 py-px font-medium text-text-secondary">Stopped</span>}
          </div>
        </div>
        <div className="relative">
          <button onClick={() => setMenu((v) => !v)} aria-label="More actions" className="rounded-md p-1.5 text-text-tertiary hover:bg-surface-hover hover:text-text-primary"><MoreHorizontal size={16} /></button>
          {menu && (
            <Menu onClose={() => setMenu(false)} items={[
              { label: 'Manage', icon: Settings2, run: onManage },
              ...(s.type !== 'proxy' ? [{ label: 'Open files', icon: FolderOpen, run: onFiles }] : []),
              { label: 'View logs', icon: ScrollText, run: onLogs },
              { label: 'Delete', icon: Trash2, danger: true, run: onDelete },
            ]} />
          )}
        </div>
      </div>

      <p className="mt-3 truncate font-mono text-xs text-text-tertiary" title={target}>{target || '—'}</p>

      <div className="mt-3 grid grid-cols-3 gap-3 border-t border-surface-border pt-3">
        <Stat label="Requests 24h" value={stats ? compact(stats.requests) : '—'} />
        <Stat label="Errors 5xx" value={stats ? (stats.server_err ? `${stats.server_err} · ${errRate.toFixed(1)}%` : '0') : '—'} tone={stats?.server_err ? 'text-danger' : ''} />
        <Stat
          label="HTTPS"
          value={<span className={cn('inline-flex items-center gap-1', certTone)}>{s.ssl_enabled ? <Lock size={12} /> : <LockOpen size={12} className="text-text-tertiary" />}{s.ssl_enabled ? (daysLeft == null ? 'On' : daysLeft < 0 ? 'Expired' : `${daysLeft}d`) : 'Off'}</span>}
        />
      </div>

      <div className="mt-3 flex items-center justify-between gap-2">
        <a href={url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline">Visit site<ExternalLink size={12} /></a>
        <div className="flex items-center gap-1">
          <button onClick={onToggle} disabled={busy} className="inline-flex h-8 items-center gap-1.5 rounded-md border border-surface-border px-2.5 text-xs font-medium text-text-secondary transition-colors hover:bg-surface-hover disabled:opacity-50">
            {running ? <><Square size={12} />Stop</> : <><Play size={12} />Start</>}
          </button>
          <button onClick={onManage} className="inline-flex h-8 items-center gap-1.5 rounded-md bg-primary px-3 text-xs font-medium text-white transition-opacity hover:opacity-90"><Settings2 size={12} />Manage</button>
        </div>
      </div>
    </article>
  )
}

export default SiteCard
