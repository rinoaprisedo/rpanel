import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { Plus, Trash2, Download, RotateCcw, KeyRound, Server, Compass, Users as UsersIcon, Archive } from 'lucide-react'
import Button from '../../components/ui/Button'
import Input from '../../components/ui/Input'
import Select from '../../components/ui/Select'
import Badge from '../../components/ui/Badge'
import Modal from '../../components/ui/Modal'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import Skeleton from '../../components/ui/Skeleton'
import * as api from '../../api/databases'
import { formatBytes } from '../../utils/format'
import { cn } from '../../utils/cn'
import Explorer from './Explorer'

const errMsg = (e) => e.response?.data?.message || 'Operation failed'
const tabs = [['Explorer', Compass], ['Users', UsersIcon], ['Backups', Archive]]
const emptyServer = { name: '', engine: 'mysql', host: '127.0.0.1', port: 3306, admin_user: '', admin_password: '' }

function Databases() {
  const qc = useQueryClient()
  const [tab, setTab] = useState('Explorer')
  const [serverId, setServerId] = useState('')
  const { data: srvRes, isLoading } = useQuery({ queryKey: ['db-servers'], queryFn: api.getServers })
  const servers = srvRes?.data || []
  const sid = serverId || servers[0]?.uuid
  const server = servers.find((s) => s.uuid === sid)
  const isMy = server?.engine === 'mysql'

  const enabled = !!sid
  const dbs = useQuery({ queryKey: ['db-dbs', sid], queryFn: () => api.getDatabases(sid), enabled: enabled && (tab === 'Explorer' || tab === 'Backups'), retry: false })
  const users = useQuery({ queryKey: ['db-users', sid], queryFn: () => api.getUsers(sid), enabled: enabled && tab === 'Users', retry: false })
  const backups = useQuery({ queryKey: ['db-backups', sid], queryFn: () => api.getBackups(sid), enabled: enabled && tab === 'Backups', retry: false })

  const [serverForm, setServerForm] = useState(null)
  const [dbForm, setDbForm] = useState(null)
  const [userForm, setUserForm] = useState(null)
  const [pwForm, setPwForm] = useState(null)
  const [confirm, setConfirm] = useState(null) // { title, message, run }

  const inv = (...keys) => keys.forEach((k) => qc.invalidateQueries({ queryKey: [k] }))
  const m = (fn, ok, after) => ({
    mutationFn: fn,
    onSuccess: () => { toast.success(ok); after?.() },
    onError: (e) => toast.error(errMsg(e)),
  })
  const addSrv = useMutation(m((d) => api.addServer(d), 'Server added', () => { setServerForm(null); inv('db-servers') }))
  const createDb = useMutation(m((d) => api.createDatabase(sid, d), 'Database created', () => { setDbForm(null); inv('db-dbs', 'db-users') }))
  const createUsr = useMutation(m((d) => api.createUser(sid, d), 'User created', () => { setUserForm(null); inv('db-users') }))
  const setPw = useMutation(m((d) => api.setPassword(sid, d), 'Password updated', () => setPwForm(null)))
  const backup = useMutation(m((db) => api.createBackup(sid, db), 'Backup created', () => inv('db-backups')))
  const doConfirm = useMutation(m(() => confirm.run(), 'Done', () => { setConfirm(null); inv('db-dbs', 'db-users', 'db-backups', 'db-servers') }))

  const f = (st, set) => (k) => (e) => set({ ...st, [k]: e.target.value })
  const genPw = () => Array.from(crypto.getRandomValues(new Uint8Array(12)), (b) => b.toString(16).padStart(2, '0')).join('')

  if (isLoading) return <Skeleton className="h-64 w-full" />

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        {servers.map((sv) => (
          <button key={sv.uuid} onClick={() => setServerId(sv.uuid)}
            className={cn('flex items-center gap-3 rounded-xl border px-4 py-2.5 text-left transition-colors', sv.uuid === sid ? 'border-primary bg-primary/10' : 'border-surface-border bg-surface-card hover:border-surface-border-hover')}>
            <Server size={18} className={sv.uuid === sid ? 'text-primary' : 'text-text-tertiary'} />
            <div>
              <div className="text-sm font-semibold text-text-primary">{sv.name}</div>
              <div className="text-[11px] text-text-secondary">{sv.engine === 'mysql' ? 'MySQL / MariaDB' : 'PostgreSQL'} · {sv.host}:{sv.port}</div>
            </div>
          </button>
        ))}
        <Button variant="secondary" className="h-[52px]" onClick={() => setServerForm(emptyServer)}><Plus size={14} />Add server</Button>
        {server && <Button variant="ghost" title="Forget this server connection" className="ml-auto" onClick={() => setConfirm({ title: `Remove ${server.name}`, message: 'Only the saved connection is removed; the database server itself is not touched.', run: () => api.deleteServer(server.uuid) })}><Trash2 size={14} className="text-danger" />Remove server</Button>}
      </div>

      {!server ? (
        <div className="rounded-lg border border-surface-border bg-surface-card p-8 text-center text-sm text-text-secondary">No database server yet. Install MariaDB or PostgreSQL from the App Store, then add it here with <b>Add server</b>.</div>
      ) : (
        <>
          <div className="flex gap-1 border-b border-surface-border">
            {tabs.map(([t, Icon]) => (
              <button key={t} onClick={() => setTab(t)} className={cn('flex items-center gap-2 px-4 py-2.5 text-sm font-medium', tab === t ? 'border-b-2 border-primary text-primary' : 'text-text-secondary hover:text-text-primary')}>
                <Icon size={15} />{t}
              </button>
            ))}
          </div>

          {tab === 'Explorer' && (
            <Explorer
              sid={sid}
              dbs={dbs}
              backupBusy={backup.isPending}
              onNew={() => setDbForm({ name: '', user: '', host: 'localhost', password: '' })}
              onBackup={(name) => backup.mutate(name)}
              onDrop={(name) => setConfirm({ title: `Drop ${name}`, message: `Permanently delete database "${name}" and all its data?`, run: () => api.dropDatabase(sid, name) })}
            />
          )}

          {tab === 'Users' && (
            <Panel
              query={users}
              action={<Button onClick={() => setUserForm({ user: '', host: 'localhost', password: '' })}><Plus size={14} />New user</Button>}
              head={isMy ? ['User', 'Host', ''] : ['Role', '']}
              rows={(users.data?.data || []).map((u) => {
                const actions = (
                  <div className="flex justify-end gap-1" key="a">
                    <Button variant="ghost" title="Change password" onClick={() => setPwForm({ user: u.name, host: u.host, password: '' })}><KeyRound size={14} /></Button>
                    <Button variant="ghost" title="Drop" onClick={() => setConfirm({ title: `Drop ${u.name}`, message: `Delete user "${u.name}"${u.host ? '@' + u.host : ''}?`, run: () => api.dropUser(sid, u.name, u.host) })}><Trash2 size={14} className="text-danger" /></Button>
                  </div>
                )
                return isMy ? [u.name, u.host, actions] : [u.name, actions]
              })}
            />
          )}

          {tab === 'Backups' && (
            <Panel
              query={backups}
              head={['File', 'Database', 'Size', 'Created', '']}
              rows={(backups.data?.data || []).map((b) => [
                <span className="font-mono text-xs" key="f">{b.file}</span>, b.database, formatBytes(b.size), new Date(b.created).toLocaleString(),
                <div className="flex justify-end gap-1" key="a">
                  <Button variant="ghost" title="Download" onClick={() => { window.location.href = api.backupDownloadURL(sid, b.file) }}><Download size={14} /></Button>
                  <Button variant="ghost" title="Restore" onClick={() => setConfirm({ title: `Restore ${b.database}`, message: `Load ${b.file} into "${b.database}"? Existing data with the same keys may be overwritten.`, confirmLabel: 'Restore', run: () => api.restoreBackup(sid, b.file, '') })}><RotateCcw size={14} /></Button>
                  <Button variant="ghost" title="Delete" onClick={() => setConfirm({ title: 'Delete backup', message: b.file, run: () => api.deleteBackup(sid, b.file) })}><Trash2 size={14} className="text-danger" /></Button>
                </div>,
              ])}
            />
          )}
        </>
      )}

      <Modal open={!!serverForm} onClose={() => setServerForm(null)} title="Add database server">
        {serverForm && (
          <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); addSrv.mutate({ ...serverForm, port: Number(serverForm.port) }) }}>
            <Input label="Name" value={serverForm.name} onChange={f(serverForm, setServerForm)('name')} autoFocus />
            <Select label="Engine" value={serverForm.engine} onChange={(e) => setServerForm({ ...serverForm, engine: e.target.value, port: e.target.value === 'mysql' ? 3306 : 5432 })}>
              <option value="mysql">MySQL / MariaDB</option><option value="postgres">PostgreSQL</option>
            </Select>
            <div className="grid grid-cols-3 gap-3">
              <div className="col-span-2"><Input label="Host" value={serverForm.host} onChange={f(serverForm, setServerForm)('host')} /></div>
              <Input label="Port" type="number" value={serverForm.port} onChange={f(serverForm, setServerForm)('port')} />
            </div>
            <Input label="Admin user" value={serverForm.admin_user} onChange={f(serverForm, setServerForm)('admin_user')} />
            <Input label="Admin password" type="password" value={serverForm.admin_password} onChange={f(serverForm, setServerForm)('admin_password')} />
            <p className="text-xs text-text-secondary">The connection is tested first. The password is stored encrypted (AES-256-GCM).</p>
            <Actions onCancel={() => setServerForm(null)} busy={addSrv.isPending} label="Connect & save" />
          </form>
        )}
      </Modal>

      <Modal open={!!dbForm} onClose={() => setDbForm(null)} title="New database">
        {dbForm && (
          <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); createDb.mutate(dbForm.user ? dbForm : { name: dbForm.name }) }}>
            <Input label="Database name" value={dbForm.name} onChange={f(dbForm, setDbForm)('name')} autoFocus />
            <Input label="Owner user (optional, will be created)" value={dbForm.user} onChange={f(dbForm, setDbForm)('user')} />
            {dbForm.user && (
              <>
                {isMy && <Input label="User host" value={dbForm.host} onChange={f(dbForm, setDbForm)('host')} />}
                <div className="flex items-end gap-2">
                  <div className="flex-1"><Input label="Password (min 8)" value={dbForm.password} onChange={f(dbForm, setDbForm)('password')} /></div>
                  <Button type="button" variant="secondary" onClick={() => setDbForm({ ...dbForm, password: genPw() })}>Generate</Button>
                </div>
              </>
            )}
            <Actions onCancel={() => setDbForm(null)} busy={createDb.isPending} disabled={!dbForm.name} label="Create" />
          </form>
        )}
      </Modal>

      <Modal open={!!userForm} onClose={() => setUserForm(null)} title="New user">
        {userForm && (
          <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); createUsr.mutate(userForm) }}>
            <Input label="User name" value={userForm.user} onChange={f(userForm, setUserForm)('user')} autoFocus />
            {isMy && <Input label="Host (localhost, %, or IP)" value={userForm.host} onChange={f(userForm, setUserForm)('host')} />}
            <div className="flex items-end gap-2">
              <div className="flex-1"><Input label="Password (min 8)" value={userForm.password} onChange={f(userForm, setUserForm)('password')} /></div>
              <Button type="button" variant="secondary" onClick={() => setUserForm({ ...userForm, password: genPw() })}>Generate</Button>
            </div>
            <Actions onCancel={() => setUserForm(null)} busy={createUsr.isPending} disabled={!userForm.user} label="Create" />
          </form>
        )}
      </Modal>

      <Modal open={!!pwForm} onClose={() => setPwForm(null)} title={`Password for ${pwForm?.user}`}>
        {pwForm && (
          <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); setPw.mutate(pwForm) }}>
            <div className="flex items-end gap-2">
              <div className="flex-1"><Input label="New password (min 8)" value={pwForm.password} onChange={f(pwForm, setPwForm)('password')} autoFocus /></div>
              <Button type="button" variant="secondary" onClick={() => setPwForm({ ...pwForm, password: genPw() })}>Generate</Button>
            </div>
            <Actions onCancel={() => setPwForm(null)} busy={setPw.isPending} disabled={!pwForm.password} label="Update" />
          </form>
        )}
      </Modal>

      <ConfirmDialog
        open={!!confirm}
        onClose={() => setConfirm(null)}
        loading={doConfirm.isPending}
        title={confirm?.title}
        message={confirm?.message}
        confirmLabel={confirm?.confirmLabel || 'Confirm'}
        loadingLabel="Working…"
        onConfirm={() => doConfirm.mutate()}
      />
    </div>
  )
}

function Actions({ onCancel, busy, disabled, label }) {
  return (
    <div className="flex justify-end gap-2">
      <Button type="button" variant="secondary" onClick={onCancel}>Cancel</Button>
      <Button type="submit" disabled={busy || disabled}>{busy ? 'Working…' : label}</Button>
    </div>
  )
}

function Panel({ query, action, head, rows }) {
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
              {rows.map((r, i) => (
                <tr key={i} className="border-b border-surface-border last:border-0">
                  {r.map((c, j) => <td key={j} className="p-3 text-text-primary">{c}</td>)}
                </tr>
              ))}
              {!rows.length && <tr><td colSpan={head.length} className="p-8 text-center text-text-secondary">Nothing here yet</td></tr>}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}

export default Databases
