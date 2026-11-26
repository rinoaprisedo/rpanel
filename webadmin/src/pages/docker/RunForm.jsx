import { useState } from 'react'
import { Plus, Trash2, ChevronDown, ChevronUp } from 'lucide-react'
import Button from '../../components/ui/Button'
import Input from '../../components/ui/Input'
import Select from '../../components/ui/Select'
import { cn } from '../../utils/cn'

const lines = (s) => s.split('\n').map((x) => x.trim()).filter(Boolean)

export const emptyRun = {
  image: '', name: '', exposeAll: false, ports: [{ ip: '', host: '', container: '', proto: 'tcp' }],
  restart: 'unless-stopped', networks: [''], mounts: [{ src: '', dst: '', mode: 'rw' }],
  command: '', entrypoint: '', autoRemove: false, tty: false, interactive: false, privileged: false,
  minMemory: 0, cpus: 0, memory: 0, labels: '', env: '', remark: '', owner: '',
}

// Turns the form state into the payload accepted by POST /docker/containers.
export function runPayload(f) {
  const ports = f.exposeAll ? [] : f.ports.filter((p) => p.host && p.container)
    .map((p) => `${p.ip ? p.ip + ':' : ''}${p.host}:${p.container}${p.proto === 'udp' ? '/udp' : ''}`)
  const volumes = f.mounts.filter((m) => m.src && m.dst).map((m) => `${m.src}:${m.dst}${m.mode === 'ro' ? ':ro' : ''}`)
  return {
    image: f.image.trim(), name: f.name.trim(), restart: f.restart, ports, volumes, env: lines(f.env), labels: lines(f.labels),
    expose_all: f.exposeAll, networks: f.networks.map((n) => n.trim()).filter(Boolean), command: f.command, entrypoint: f.entrypoint,
    auto_remove: f.autoRemove, tty: f.tty, interactive: f.interactive, privileged: f.privileged,
    min_memory: Number(f.minMemory) || 0, cpus: Number(f.cpus) || 0, memory: Number(f.memory) || 0, remark: f.remark, owner: f.owner.trim(),
  }
}

const labelCls = 'text-xs font-medium uppercase tracking-[.05em] text-text-secondary'
const fieldCls = 'w-full rounded-md border-[1.5px] border-surface-border bg-field p-3 font-mono text-xs outline-none focus:border-primary'

function Row({ label, children }) {
  return (
    <div className="grid grid-cols-[130px_1fr] items-start gap-3">
      <span className={cn(labelCls, 'pt-3')}>{label}</span>
      <div className="min-w-0">{children}</div>
    </div>
  )
}

function Seg({ value, options, onChange }) {
  return (
    <div className="inline-flex overflow-hidden rounded-md border-[1.5px] border-surface-border">
      {options.map(([v, l]) => (
        <button key={String(v)} type="button" onClick={() => onChange(v)}
          className={cn('h-9 px-4 text-sm transition-colors', value === v ? 'bg-primary text-white' : 'bg-field text-text-secondary hover:text-text-primary')}>{l}</button>
      ))}
    </div>
  )
}

function Check({ label, checked, onChange }) {
  return (
    <label className="flex cursor-pointer items-center gap-2 text-sm">
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} className="accent-[var(--color-primary,#e5484d)]" />{label}
    </label>
  )
}

function Limit({ value, max, unit, onChange }) {
  return (
    <div className="flex items-center gap-3">
      <input type="range" min={0} max={max} step={unit === 'Cores' ? 0.5 : 64} value={value} onChange={(e) => onChange(e.target.value)} className="flex-1 accent-[var(--color-primary,#e5484d)]" />
      <input type="number" min={0} step="any" value={value} onChange={(e) => onChange(e.target.value)} className="h-9 w-24 rounded-md border-[1.5px] border-surface-border bg-field px-2 text-sm outline-none focus:border-primary" />
      <span className="w-12 text-xs text-text-secondary">{unit}</span>
    </div>
  )
}

function RunForm({ form, setForm, images, volumes, onSubmit, onCancel, pending }) {
  const [more, setMore] = useState(false)
  const set = (patch) => setForm({ ...form, ...patch })
  const setList = (key, i, patch) => set({ [key]: form[key].map((x, j) => (j === i ? { ...x, ...patch } : x)) })
  const del = (key, i) => set({ [key]: form[key].filter((_, j) => j !== i) })
  const policy = form.restart
  const restartOpts = [['no', 'No reboot'], ['on-failure:5', 'Reboot after failure (5 times)'], ['on-failure', 'Reboot on abnormal exit only'], ['unless-stopped', 'Reboot right after stopping']]
  const imageList = [...new Set((images || []).filter((i) => i.repository && i.repository !== '<none>').map((i) => (i.tag && i.tag !== '<none>' ? `${i.repository}:${i.tag}` : i.repository)))]

  return (
    <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); onSubmit(runPayload(form)) }}>
      <Row label="Container name">
        <Input placeholder="Optional, e.g. CentralMariaDB" value={form.name} onChange={(e) => set({ name: e.target.value })} />
      </Row>
      <Row label="Image">
        <Input list="run-images" placeholder="Enter or select image, e.g. mariadb:11.8" value={form.image} onChange={(e) => set({ image: e.target.value })} autoFocus />
        <datalist id="run-images">{imageList.map((i) => <option key={i} value={i} />)}</datalist>
      </Row>

      <Row label="Port">
        <div className="space-y-2">
          <Seg value={form.exposeAll} options={[[false, 'Expose port'], [true, 'Expose all']]} onChange={(v) => set({ exposeAll: v })} />
          {!form.exposeAll && form.ports.map((p, i) => (
            <div key={i} className="grid grid-cols-[1fr_1fr_1fr_80px_auto] items-center gap-2">
              <Input placeholder="IP (optional)" value={p.ip} onChange={(e) => setList('ports', i, { ip: e.target.value })} />
              <Input placeholder="Host port" inputMode="numeric" value={p.host} onChange={(e) => setList('ports', i, { host: e.target.value })} />
              <Input placeholder="Container port" inputMode="numeric" value={p.container} onChange={(e) => setList('ports', i, { container: e.target.value })} />
              <Select value={p.proto} onChange={(e) => setList('ports', i, { proto: e.target.value })}><option value="tcp">tcp</option><option value="udp">udp</option></Select>
              <Button type="button" variant="ghost" aria-label="Remove port" onClick={() => del('ports', i)}><Trash2 size={14} className="text-danger" /></Button>
            </div>
          ))}
          {!form.exposeAll && <div><Button type="button" variant="secondary" onClick={() => set({ ports: [...form.ports, { ip: '', host: '', container: '', proto: 'tcp' }] })}><Plus size={14} />Add</Button></div>}
          {!form.exposeAll && <p className="text-xs text-text-secondary">Use IP 127.0.0.1 to keep a port off the internet.</p>}
        </div>
      </Row>

      <Row label="Reboot rule">
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          {restartOpts.map(([v, l]) => (
            <label key={v} className="flex cursor-pointer items-center gap-2 text-sm">
              <input type="radio" name="restart" checked={policy === v} onChange={() => set({ restart: v })} className="accent-[var(--color-primary,#e5484d)]" />{l}
            </label>
          ))}
        </div>
        {form.autoRemove && <p className="mt-1 text-xs text-text-secondary">Ignored while “Automatic deletion” is on.</p>}
      </Row>

      <button type="button" onClick={() => setMore(!more)} className="flex items-center gap-1 text-sm font-medium text-primary">
        {more ? <ChevronUp size={14} /> : <ChevronDown size={14} />}More config, click to {more ? 'hide' : 'view'}
      </button>

      {more && (
        <div className="space-y-4">
          <Row label="Network">
            <div className="space-y-2">
              {form.networks.map((n, i) => (
                <div key={i} className="flex items-center gap-2">
                  <Input className="flex-1" placeholder="Network name, e.g. bridge or appnet" value={n} onChange={(e) => set({ networks: form.networks.map((x, j) => (j === i ? e.target.value : x)) })} />
                  <Button type="button" variant="ghost" aria-label="Remove network" onClick={() => set({ networks: form.networks.filter((_, j) => j !== i) })}><Trash2 size={14} className="text-danger" /></Button>
                </div>
              ))}
              {form.networks.length < 1 && <div><Button type="button" variant="secondary" onClick={() => set({ networks: [''] })}><Plus size={14} />Add</Button></div>}
            </div>
          </Row>
          <Row label="Mount / Map">
            <div className="space-y-2">
              {form.mounts.map((m, i) => (
                <div key={i} className="grid grid-cols-[1fr_1fr_90px_auto] items-center gap-2">
                  <Input list="run-volumes" placeholder="Host path or volume name" value={m.src} onChange={(e) => setList('mounts', i, { src: e.target.value })} />
                  <Input placeholder="Container path" value={m.dst} onChange={(e) => setList('mounts', i, { dst: e.target.value })} />
                  <Select value={m.mode} onChange={(e) => setList('mounts', i, { mode: e.target.value })}><option value="rw">rw</option><option value="ro">ro</option></Select>
                  <Button type="button" variant="ghost" aria-label="Remove mount" onClick={() => del('mounts', i)}><Trash2 size={14} className="text-danger" /></Button>
                </div>
              ))}
              <datalist id="run-volumes">{(volumes || []).map((v) => <option key={v.name} value={v.name} />)}</datalist>
              <div><Button type="button" variant="secondary" onClick={() => set({ mounts: [...form.mounts, { src: '', dst: '', mode: 'rw' }] })}><Plus size={14} />Add</Button></div>
            </div>
          </Row>
          <Row label="Folder owner">
            <div className="space-y-2">
              <div className="flex flex-wrap items-center gap-2">
                <Input className="w-40" placeholder="uid:gid, e.g. 999:999" value={form.owner} onChange={(e) => set({ owner: e.target.value })} />
                <Button type="button" variant="secondary" onClick={() => set({ owner: '999:999' })}>mysql / mariadb / postgres (999)</Button>
                <Button type="button" variant="secondary" onClick={() => set({ owner: '' })}>None</Button>
              </div>
              <p className="text-xs text-text-secondary">Creates the host folders of your mounts if missing and gives them to this user before starting, so databases can write there (fixes “Permission denied”). Named volumes need nothing. System folders are refused.</p>
            </div>
          </Row>
          <Row label="Command"><Input placeholder="Overrides the image CMD" value={form.command} onChange={(e) => set({ command: e.target.value })} /></Row>
          <Row label="Entrypoint"><Input placeholder="Overrides the image ENTRYPOINT" value={form.entrypoint} onChange={(e) => set({ entrypoint: e.target.value })} /></Row>
          <Row label="Options">
            <div className="flex flex-wrap gap-x-6 gap-y-2 pt-2.5">
              <Check label="Automatic deletion of containers after exit" checked={form.autoRemove} onChange={(v) => set({ autoRemove: v })} />
              <Check label="Pseudo-TTY (-t)" checked={form.tty} onChange={(v) => set({ tty: v })} />
              <Check label="Standard input (-i)" checked={form.interactive} onChange={(v) => set({ interactive: v })} />
              <Check label="Privilege mode" checked={form.privileged} onChange={(v) => set({ privileged: v })} />
            </div>
            {form.privileged && <p className="mt-1 text-xs text-danger">Privileged containers have full access to the host. Only enable this if you trust the image.</p>}
          </Row>
          <Row label="Min memory"><Limit value={form.minMemory} max={8192} unit="MB" onChange={(v) => set({ minMemory: v })} /></Row>
          <Row label="CPU limit"><Limit value={form.cpus} max={16} unit="Cores" onChange={(v) => set({ cpus: v })} /></Row>
          <Row label="Memory limit"><Limit value={form.memory} max={8192} unit="MB" onChange={(v) => set({ memory: v })} /></Row>
          <Row label="Tag">
            <textarea rows={3} spellCheck={false} placeholder="Container labels, one per line, e.g. key=value" value={form.labels} onChange={(e) => set({ labels: e.target.value })} className={fieldCls} />
          </Row>
        </div>
      )}

      <Row label="Env variable">
        <textarea rows={4} spellCheck={false} placeholder={'One per line:\nMARIADB_ROOT_PASSWORD=yourpassword\nTZ=Asia/Jakarta'} value={form.env} onChange={(e) => set({ env: e.target.value })} className={fieldCls} />
      </Row>
      <Row label="Remark">
        <textarea rows={2} placeholder="Container remark" value={form.remark} onChange={(e) => set({ remark: e.target.value })} className={fieldCls} />
      </Row>

      <div className="flex justify-end gap-2">
        <Button type="button" variant="secondary" onClick={onCancel}>Cancel</Button>
        <Button type="submit" disabled={pending || !form.image.trim()}>{pending ? 'Starting (may pull image)…' : 'Confirm'}</Button>
      </div>
    </form>
  )
}

export default RunForm
