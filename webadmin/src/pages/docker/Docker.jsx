import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { Plus, Play, Square, RotateCw, Trash2, ScrollText, Download, FileCode2, ArrowUpFromLine, ArrowDownToLine } from 'lucide-react'
import Tabs from '../../components/ui/Tabs'
import Button from '../../components/ui/Button'
import Input from '../../components/ui/Input'
import Select from '../../components/ui/Select'
import Badge from '../../components/ui/Badge'
import Modal from '../../components/ui/Modal'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import Skeleton from '../../components/ui/Skeleton'
import * as api from '../../api/docker'

const errMsg = (e) => e.response?.data?.message || 'Operation failed'
const tabs = ['Containers', 'Images', 'Compose']
const lines = (s) => s.split('\n').map((x) => x.trim()).filter(Boolean)
const emptyRun = { image: '', name: '', ports: '', env: '', volumes: '', restart: 'unless-stopped' }
const sampleCompose = 'services:\n  web:\n    image: nginx:latest\n    ports:\n      - "8080:80"\n    restart: unless-stopped\n'

function Docker() {
  const qc = useQueryClient()
  const [tab, setTab] = useState('Containers')
  const { data: stRes, isLoading } = useQuery({ queryKey: ['docker-status'], queryFn: api.getDockerStatus })
  const st = stRes?.data
  const ready = st?.installed && st?.running

  const containers = useQuery({ queryKey: ['d-containers'], queryFn: api.getContainers, enabled: !!ready && tab === 'Containers', refetchInterval: 5000, retry: false })
  const images = useQuery({ queryKey: ['d-images'], queryFn: api.getImages, enabled: !!ready && tab === 'Images', retry: false })
  const projects = useQuery({ queryKey: ['d-projects'], queryFn: api.getProjects, enabled: tab === 'Compose', retry: false })

  const [runForm, setRunForm] = useState(null)
  const [pullRef, setPullRef] = useState(null)
  const [logs, setLogs] = useState(null) // { title, text, reload }
  const [editor, setEditor] = useState(null) // { name, content, isNew }
  const [confirm, setConfirm] = useState(null)
  const [output, setOutput] = useState(null) // { title, text }

  const inv = (...k) => k.forEach((x) => qc.invalidateQueries({ queryKey: [x] }))
  const m = (fn, ok, after) => ({ mutationFn: fn, onSuccess: (r) => { if (ok) toast.success(ok); after?.(r) }, onError: (e) => toast.error(errMsg(e)) })
  const install = useMutation(m(api.installDocker, 'Docker installed', () => inv('docker-status')))
  const act = useMutation(m(({ id, action }) => api.containerAction(id, action), 'Done', () => inv('d-containers')))
  const run = useMutation(m((d) => api.runContainer(d), 'Container started', () => { setRunForm(null); inv('d-containers', 'd-images') }))
  const pull = useMutation(m((r) => api.pullImage(r), 'Image pulled', () => { setPullRef(null); inv('d-images') }))
  const save = useMutation(m(({ name, content, isNew }) => (isNew ? api.createProject(name, content) : api.updateProject(name, content)), 'Saved', () => { setEditor(null); inv('d-projects') }))
  const projOp = useMutation({
    mutationFn: ({ name, op }) => (op === 'up' ? api.projectUp(name) : api.projectDown(name)),
    onSuccess: (r, v) => { setOutput({ title: `${v.name}: ${v.op}`, text: r.data.output || '(no output)' }); inv('d-containers') },
    onError: (e) => toast.error(errMsg(e)),
  })
  const doConfirm = useMutation(m(() => confirm.run(), 'Done', () => { setConfirm(null); inv('d-containers', 'd-images', 'd-projects') }))

  const showLogs = async (title, fetcher) => {
    const reload = async () => {
      try { const r = await fetcher(); setLogs({ title, text: r.data.log ?? r.data.output ?? '', reload }) } catch (e) { toast.error(errMsg(e)) }
    }
    await reload()
  }
  const openProject = async (name) => {
    try { const r = await api.getProject(name); setEditor({ name, content: r.data.content, isNew: false }) } catch (e) { toast.error(errMsg(e)) }
  }

  if (isLoading) return <Skeleton className="h-64 w-full" />

  if (!ready) {
    return (
      <div className="rounded-lg border border-surface-border bg-surface-card p-8 text-center text-sm">
        {!st?.installed ? (
          <>
            <p className="mb-4 text-text-secondary">Docker is not installed on this server.</p>
            <Button disabled={install.isPending} onClick={() => install.mutate()}>{install.isPending ? 'Installing via apt…' : 'Install Docker'}</Button>
          </>
        ) : (
          <p className="text-danger">Docker is installed but the daemon is not reachable: {st.error || 'unknown error'}</p>
        )}
      </div>
    )
  }

  const stateBadge = (s) => <Badge variant={s === 'running' ? 'success' : s === 'paused' ? 'warning' : 'neutral'}>{s}</Badge>

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between rounded-lg border border-surface-border bg-surface-card px-4 py-3 text-sm text-text-secondary">
        <span>Docker {st.version} · Compose {st.compose ? <span className="text-success">available</span> : <span className="text-danger">missing</span>}</span>
      </div>
      <Tabs tabs={tabs} value={tab} onChange={setTab} />

      {tab === 'Containers' && (
        <Table query={containers} action={<Button onClick={() => setRunForm(emptyRun)}><Plus size={14} />Run container</Button>}
          head={['Name', 'Image', 'State', 'Ports', '']}
          rows={(containers.data?.data || []).map((c) => [
            <span key="n" className="font-medium">{c.name}</span>, <span key="i" className="font-mono text-xs">{c.image}</span>,
            <span key="s" title={c.status}>{stateBadge(c.state)}</span>, <span key="p" className="text-xs text-text-secondary">{c.ports}</span>,
            <div key="a" className="flex justify-end gap-1">
              {c.state === 'running'
                ? <><Button variant="ghost" title="Stop" onClick={() => act.mutate({ id: c.id, action: 'stop' })}><Square size={14} /></Button>
                    <Button variant="ghost" title="Restart" onClick={() => act.mutate({ id: c.id, action: 'restart' })}><RotateCw size={14} /></Button></>
                : <Button variant="ghost" title="Start" onClick={() => act.mutate({ id: c.id, action: 'start' })}><Play size={14} /></Button>}
              <Button variant="ghost" title="Logs" onClick={() => showLogs(c.name, () => api.getContainerLogs(c.id))}><ScrollText size={14} /></Button>
              <Button variant="ghost" title="Remove" onClick={() => setConfirm({ title: `Remove ${c.name}`, message: 'The container is force-removed. Data in anonymous volumes is lost.', run: () => api.removeContainer(c.id) })}><Trash2 size={14} className="text-danger" /></Button>
            </div>,
          ])} />
      )}

      {tab === 'Images' && (
        <Table query={images} action={<Button onClick={() => setPullRef('')}><Download size={14} />Pull image</Button>}
          head={['Repository', 'Tag', 'Size', 'Created', '']}
          rows={(images.data?.data || []).map((i) => [
            i.repository, i.tag, i.size, i.created,
            <div key="a" className="flex justify-end"><Button variant="ghost" title="Remove" onClick={() => setConfirm({ title: `Remove ${i.repository}:${i.tag}`, message: 'Fails if a container still uses the image.', run: () => api.removeImage(i.id) })}><Trash2 size={14} className="text-danger" /></Button></div>,
          ])} />
      )}

      {tab === 'Compose' && (
        <Table query={projects} action={<Button onClick={() => setEditor({ name: '', content: sampleCompose, isNew: true })}><Plus size={14} />New project</Button>}
          head={['Project', '']}
          rows={(projects.data?.data || []).map((p) => [
            <span key="n" className="font-medium">{p.name}</span>,
            <div key="a" className="flex justify-end gap-1">
              <Button variant="ghost" title="Edit compose file" onClick={() => openProject(p.name)}><FileCode2 size={14} /></Button>
              <Button variant="ghost" title="Up" disabled={projOp.isPending} onClick={() => projOp.mutate({ name: p.name, op: 'up' })}><ArrowUpFromLine size={14} /></Button>
              <Button variant="ghost" title="Down" disabled={projOp.isPending} onClick={() => projOp.mutate({ name: p.name, op: 'down' })}><ArrowDownToLine size={14} /></Button>
              <Button variant="ghost" title="Logs" onClick={() => showLogs(`${p.name} logs`, () => api.projectLogs(p.name))}><ScrollText size={14} /></Button>
              <Button variant="ghost" title="Delete project" onClick={() => setConfirm({ title: `Delete ${p.name}`, message: 'Runs "compose down" and deletes the project directory.', run: () => api.deleteProject(p.name) })}><Trash2 size={14} className="text-danger" /></Button>
            </div>,
          ])} />
      )}
      {projOp.isPending && <p className="text-sm text-text-secondary">Running compose, this may take a while (images may be pulled)…</p>}

      <Modal open={!!runForm} onClose={() => setRunForm(null)} title="Run container" size="lg">
        {runForm && (
          <form className="space-y-4" onSubmit={(e) => {
            e.preventDefault()
            run.mutate({ image: runForm.image, name: runForm.name, restart: runForm.restart, ports: lines(runForm.ports), env: lines(runForm.env), volumes: lines(runForm.volumes) })
          }}>
            <div className="grid grid-cols-2 gap-3">
              <Input label="Image" placeholder="nginx:latest" value={runForm.image} onChange={(e) => setRunForm({ ...runForm, image: e.target.value })} autoFocus />
              <Input label="Name (optional)" value={runForm.name} onChange={(e) => setRunForm({ ...runForm, name: e.target.value })} />
            </div>
            <Select label="Restart policy" value={runForm.restart} onChange={(e) => setRunForm({ ...runForm, restart: e.target.value })}>
              <option value="no">no</option><option value="unless-stopped">unless-stopped</option><option value="always">always</option><option value="on-failure">on-failure</option>
            </Select>
            <Area label="Ports (one per line: host:container)" value={runForm.ports} onChange={(v) => setRunForm({ ...runForm, ports: v })} placeholder="8080:80" />
            <Area label="Environment (one per line: KEY=value)" value={runForm.env} onChange={(v) => setRunForm({ ...runForm, env: v })} placeholder="MODE=production" />
            <Area label="Volumes (one per line: /host:/container[:ro])" value={runForm.volumes} onChange={(v) => setRunForm({ ...runForm, volumes: v })} placeholder="/www/data:/data" />
            <div className="flex justify-end gap-2">
              <Button type="button" variant="secondary" onClick={() => setRunForm(null)}>Cancel</Button>
              <Button type="submit" disabled={run.isPending || !runForm.image}>{run.isPending ? 'Starting (may pull image)…' : 'Run'}</Button>
            </div>
          </form>
        )}
      </Modal>

      <Modal open={pullRef !== null} onClose={() => setPullRef(null)} title="Pull image">
        <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); pull.mutate(pullRef) }}>
          <Input label="Image reference" placeholder="nginx:latest" value={pullRef || ''} onChange={(e) => setPullRef(e.target.value)} autoFocus />
          <div className="flex justify-end gap-2">
            <Button type="button" variant="secondary" onClick={() => setPullRef(null)}>Cancel</Button>
            <Button type="submit" disabled={pull.isPending || !pullRef}>{pull.isPending ? 'Pulling…' : 'Pull'}</Button>
          </div>
        </form>
      </Modal>

      <Modal open={!!editor} onClose={() => setEditor(null)} title={editor?.isNew ? 'New compose project' : `Compose: ${editor?.name}`} size="xl">
        {editor && (
          <div className="space-y-3">
            {editor.isNew && <Input label="Project name" placeholder="my-app" value={editor.name} onChange={(e) => setEditor({ ...editor, name: e.target.value })} />}
            <textarea spellCheck={false} className="h-[50vh] w-full resize-none rounded-md border border-surface-border bg-white p-3 font-mono text-xs outline-none focus:border-primary" value={editor.content} onChange={(e) => setEditor({ ...editor, content: e.target.value })} />
            <p className="text-xs text-text-secondary">Saving does not restart anything; press “Up” on the project to apply changes.</p>
            <div className="flex justify-end gap-2">
              <Button variant="secondary" onClick={() => setEditor(null)}>Close</Button>
              <Button disabled={save.isPending || !editor.name} onClick={() => save.mutate(editor)}>Save</Button>
            </div>
          </div>
        )}
      </Modal>

      <Modal open={!!logs} onClose={() => setLogs(null)} title={logs?.title} size="xl">
        {logs && (
          <div className="space-y-3">
            <Button variant="secondary" onClick={() => logs.reload()}>Refresh</Button>
            <pre className="h-[55vh] overflow-auto whitespace-pre-wrap break-all rounded-md bg-[#1a1917] p-3 font-mono text-xs text-gray-200">{logs.text || '(empty)'}</pre>
          </div>
        )}
      </Modal>

      <Modal open={!!output} onClose={() => setOutput(null)} title={output?.title} size="lg">
        <pre className="max-h-[55vh] overflow-auto whitespace-pre-wrap break-all rounded-md bg-[#1a1917] p-3 font-mono text-xs text-gray-200">{output?.text}</pre>
      </Modal>

      <ConfirmDialog open={!!confirm} onClose={() => setConfirm(null)} loading={doConfirm.isPending} title={confirm?.title} message={confirm?.message}
        confirmLabel="Confirm" loadingLabel="Working…" onConfirm={() => doConfirm.mutate()} />
    </div>
  )
}

function Area({ label, value, onChange, placeholder }) {
  return (
    <div className="flex flex-col gap-1">
      <label className="text-xs font-medium uppercase tracking-[.05em] text-text-secondary">{label}</label>
      <textarea rows={3} spellCheck={false} placeholder={placeholder} value={value} onChange={(e) => onChange(e.target.value)}
        className="w-full rounded-md border-[1.5px] border-surface-border bg-white p-3 font-mono text-xs outline-none focus:border-primary" />
    </div>
  )
}

function Table({ query, action, head, rows }) {
  return (
    <div className="space-y-3">
      {action && <div className="flex justify-end">{action}</div>}
      <div className="overflow-x-auto rounded-lg border border-surface-border bg-surface-card">
        {query.isLoading ? <Skeleton className="h-40 w-full" /> : query.error ? (
          <p className="p-6 text-sm text-danger">{errMsg(query.error)}</p>
        ) : (
          <table className="w-full text-sm">
            <thead className="border-b border-surface-border text-left text-xs uppercase text-text-secondary">
              <tr>{head.map((h, i) => <th key={i} className="p-3">{h}</th>)}</tr>
            </thead>
            <tbody>
              {rows.map((r, i) => <tr key={i} className="border-b border-surface-border last:border-0">{r.map((c, j) => <td key={j} className="p-3 text-text-primary">{c}</td>)}</tr>)}
              {!rows.length && <tr><td colSpan={head.length} className="p-8 text-center text-text-secondary">Nothing here yet</td></tr>}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}

export default Docker
