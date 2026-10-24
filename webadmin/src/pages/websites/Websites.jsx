import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { Plus, Play, Square, Lock, ScrollText, Trash2, Globe } from 'lucide-react'
import Button from '../../components/ui/Button'
import Input from '../../components/ui/Input'
import Select from '../../components/ui/Select'
import Badge from '../../components/ui/Badge'
import Modal from '../../components/ui/Modal'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import Skeleton from '../../components/ui/Skeleton'
import * as api from '../../api/websites'

const errMsg = (e) => e.response?.data?.message || 'Operation failed'
const emptyForm = { domain: '', type: 'static', root_dir: '', php_version: '', proxy_target: '' }

function Websites() {
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({ queryKey: ['websites'], queryFn: api.getWebsites })
  const { data: envRes } = useQuery({ queryKey: ['websites-env'], queryFn: api.getWebsiteEnv })
  const env = envRes?.data
  const sites = data?.data || []

  const [form, setForm] = useState(null)
  const [del, setDel] = useState(null)
  const [deleteFiles, setDeleteFiles] = useState(false)
  const [logs, setLogs] = useState(null) // { site, type, text }
  const [sslSite, setSslSite] = useState(null)
  const [email, setEmail] = useState('')

  const refresh = () => qc.invalidateQueries({ queryKey: ['websites'] })
  const act = (fn, ok, after) => ({
    mutationFn: fn,
    onSuccess: () => { toast.success(ok); refresh(); after?.() },
    onError: (e) => toast.error(errMsg(e)),
  })
  const create = useMutation(act((d) => api.createWebsite(d), 'Website created', () => setForm(null)))
  const toggle = useMutation(act((s) => (s.status === 'running' ? api.stopWebsite(s.uuid) : api.startWebsite(s.uuid)), 'Updated'))
  const forceHttps = useMutation(act((s) => api.updateWebsite(s.uuid, { force_https: !s.force_https }), 'Updated'))
  const ssl = useMutation(act((s) => api.issueSSL(s.uuid, email), 'SSL enabled', () => setSslSite(null)))
  const remove = useMutation(act((s) => api.deleteWebsite(s.uuid, deleteFiles), 'Website deleted', () => setDel(null)))
  const install = useMutation({
    mutationFn: ({ component, version }) => api.installComponent(component, version),
    onSuccess: () => { toast.success('Installed'); qc.invalidateQueries({ queryKey: ['websites-env'] }) },
    onError: (e) => toast.error(errMsg(e)),
  })

  const openLogs = async (site, type) => {
    try {
      const r = await api.getWebsiteLogs(site.uuid, type)
      setLogs({ site, type, text: r.data.log })
    } catch (e) {
      toast.error(errMsg(e))
    }
  }
  const set = (k) => (e) => setForm({ ...form, [k]: e.target.value })

  const submit = (e) => {
    e.preventDefault()
    const body = { domain: form.domain, type: form.type, root_dir: form.root_dir }
    if (form.type === 'php') body.php_version = form.php_version
    if (form.type === 'proxy') body.proxy_target = form.proxy_target
    create.mutate(body)
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-surface-border bg-surface-card p-4 text-sm">
        <div className="flex flex-wrap items-center gap-4 text-text-secondary">
          <span>Nginx: {env?.nginx ? <span className="text-success">{env.nginx_version || 'installed'}</span> : <span className="text-danger">not installed</span>}</span>
          <span>Certbot: {env?.certbot ? <span className="text-success">installed</span> : <span className="text-danger">not installed</span>}</span>
          <span>PHP: {env?.php_versions?.length ? <span className="text-success">{env.php_versions.join(', ')}</span> : <span className="text-text-secondary">none</span>}</span>
        </div>
        <div className="flex gap-2">
          {env && !env.nginx && <Button variant="secondary" disabled={install.isPending} onClick={() => install.mutate({ component: 'nginx' })}>Install Nginx</Button>}
          {env && !env.certbot && <Button variant="secondary" disabled={install.isPending} onClick={() => install.mutate({ component: 'certbot' })}>Install Certbot</Button>}
          <Button variant="secondary" disabled={install.isPending} onClick={() => { const v = window.prompt('PHP version to install (e.g. 8.3)'); if (v) install.mutate({ component: 'php', version: v }) }}>Install PHP</Button>
          <Button onClick={() => setForm({ ...emptyForm, php_version: env?.php_versions?.[0] || '' })}><Plus size={14} />Add website</Button>
        </div>
      </div>
      {install.isPending && <p className="text-sm text-text-secondary">Installing via apt, this can take a few minutes…</p>}

      <div className="overflow-x-auto rounded-lg border border-surface-border bg-surface-card">
        {isLoading ? <Skeleton className="h-48 w-full" /> : (
          <table className="w-full text-sm">
            <thead className="border-b border-surface-border text-left text-xs uppercase text-text-secondary">
              <tr><th className="p-3">Domain</th><th className="p-3">Type</th><th className="p-3">Target</th><th className="p-3">Status</th><th className="p-3">SSL</th><th className="p-3 text-right">Actions</th></tr>
            </thead>
            <tbody>
              {sites.map((s) => (
                <tr key={s.uuid} className="border-b border-surface-border last:border-0">
                  <td className="p-3 font-medium text-text-primary"><Globe size={13} className="mr-1 inline text-text-secondary" />{s.domain}</td>
                  <td className="p-3 text-text-secondary">{s.type}{s.php_version ? ` ${s.php_version}` : ''}</td>
                  <td className="max-w-[16rem] truncate p-3 font-mono text-xs text-text-secondary">{s.type === 'proxy' ? s.proxy_target : s.root_dir}</td>
                  <td className="p-3"><Badge variant={s.status === 'running' ? 'success' : 'neutral'}>{s.status}</Badge></td>
                  <td className="p-3">{s.ssl_enabled ? <Badge variant="success">{s.force_https ? 'https only' : 'enabled'}</Badge> : <Badge>off</Badge>}</td>
                  <td className="p-3">
                    <div className="flex justify-end gap-1">
                      <Button variant="ghost" title={s.status === 'running' ? 'Stop' : 'Start'} onClick={() => toggle.mutate(s)}>{s.status === 'running' ? <Square size={14} /> : <Play size={14} />}</Button>
                      {!s.ssl_enabled
                        ? <Button variant="ghost" title="Get SSL certificate" onClick={() => { setEmail(''); setSslSite(s) }}><Lock size={14} /></Button>
                        : <Button variant="ghost" title="Toggle force HTTPS" onClick={() => forceHttps.mutate(s)}>{s.force_https ? 'HTTP ok' : 'Force HTTPS'}</Button>}
                      <Button variant="ghost" title="Logs" onClick={() => openLogs(s, 'error')}><ScrollText size={14} /></Button>
                      <Button variant="ghost" title="Delete" onClick={() => { setDeleteFiles(false); setDel(s) }}><Trash2 size={14} className="text-danger" /></Button>
                    </div>
                  </td>
                </tr>
              ))}
              {!sites.length && <tr><td colSpan={6} className="p-8 text-center text-text-secondary">No websites yet</td></tr>}
            </tbody>
          </table>
        )}
      </div>

      <Modal open={!!form} onClose={() => setForm(null)} title="Add website">
        {form && (
          <form onSubmit={submit} className="space-y-4">
            <Input label="Domain" placeholder="example.com" value={form.domain} onChange={set('domain')} autoFocus />
            <Select label="Type" value={form.type} onChange={set('type')}>
              <option value="static">Static</option>
              <option value="php">PHP (FPM)</option>
              <option value="proxy">Reverse proxy</option>
            </Select>
            {form.type !== 'proxy' && <Input label="Root directory (optional)" placeholder="default: /www/wwwroot/<domain>" value={form.root_dir} onChange={set('root_dir')} />}
            {form.type === 'php' && (
              env?.php_versions?.length
                ? <Select label="PHP version" value={form.php_version} onChange={set('php_version')}>{env.php_versions.map((v) => <option key={v} value={v}>{v}</option>)}</Select>
                : <Input label="PHP version" placeholder="8.3 (install it first)" value={form.php_version} onChange={set('php_version')} />
            )}
            {form.type === 'proxy' && <Input label="Proxy target" placeholder="http://127.0.0.1:3000" value={form.proxy_target} onChange={set('proxy_target')} />}
            <div className="flex justify-end gap-2">
              <Button type="button" variant="secondary" onClick={() => setForm(null)}>Cancel</Button>
              <Button type="submit" disabled={create.isPending || !form.domain}>{create.isPending ? 'Creating…' : 'Create'}</Button>
            </div>
          </form>
        )}
      </Modal>

      <Modal open={!!sslSite} onClose={() => setSslSite(null)} title={`SSL for ${sslSite?.domain}`}>
        <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); ssl.mutate(sslSite) }}>
          <p className="text-sm text-text-secondary">The domain must already point to this server (port 80 reachable). Uses Let's Encrypt.</p>
          <Input label="Email (leave empty to use server default)" value={email} onChange={(e) => setEmail(e.target.value)} />
          <div className="flex justify-end gap-2">
            <Button type="button" variant="secondary" onClick={() => setSslSite(null)}>Cancel</Button>
            <Button type="submit" disabled={ssl.isPending}>{ssl.isPending ? 'Requesting…' : 'Request certificate'}</Button>
          </div>
        </form>
      </Modal>

      <Modal open={!!logs} onClose={() => setLogs(null)} title={`${logs?.site.domain} — ${logs?.type} log`} size="xl">
        {logs && (
          <div className="space-y-3">
            <div className="flex gap-2">
              <Button variant={logs.type === 'error' ? 'primary' : 'secondary'} onClick={() => openLogs(logs.site, 'error')}>Error</Button>
              <Button variant={logs.type === 'access' ? 'primary' : 'secondary'} onClick={() => openLogs(logs.site, 'access')}>Access</Button>
              <Button variant="secondary" onClick={() => openLogs(logs.site, logs.type)}>Refresh</Button>
            </div>
            <pre className="h-[55vh] overflow-auto whitespace-pre-wrap break-all rounded-md bg-[#1a1917] p-3 font-mono text-xs text-gray-200">{logs.text || '(empty)'}</pre>
          </div>
        )}
      </Modal>

      <ConfirmDialog
        open={!!del}
        onClose={() => setDel(null)}
        loading={remove.isPending}
        title={`Delete ${del?.domain}`}
        message={
          <span className="block space-y-3">
            <span className="block">The nginx config is removed and the site stops serving.</span>
            <label className="flex items-center gap-2"><input type="checkbox" checked={deleteFiles} onChange={(e) => setDeleteFiles(e.target.checked)} /> Also delete site files ({del?.root_dir || 'n/a'})</label>
          </span>
        }
        onConfirm={() => remove.mutate(del)}
      />
    </div>
  )
}

export default Websites
