import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { ShieldCheck, ShieldOff } from 'lucide-react'
import Button from '../../components/ui/Button'
import Input from '../../components/ui/Input'
import Badge from '../../components/ui/Badge'
import Modal from '../../components/ui/Modal'
import useAuthStore from '../../store/authStore'
import { me, twoFASetup, twoFAEnable, twoFADisable, twoFARecovery } from '../../api/auth'

const errMsg = (e) => e.response?.data?.message || 'Operation failed'

function Security() {
  const user = useAuthStore((s) => s.user)
  const setUser = useAuthStore((s) => s.setUser)
  const enabled = !!user?.two_factor_enabled
  const [enrol, setEnrol] = useState(null) // { secret, qr }
  const [code, setCode] = useState('')
  const [codes, setCodes] = useState(null) // recovery codes shown once
  const [disable, setDisable] = useState(null) // { password, code }
  const [regen, setRegen] = useState(null) // string code

  const reload = async () => { const r = await me(); setUser(r.data) }
  const onErr = (e) => toast.error(errMsg(e))

  const setup = useMutation({ mutationFn: twoFASetup, onSuccess: (r) => { setEnrol(r.data); setCode('') }, onError: onErr })
  const enable = useMutation({
    mutationFn: () => twoFAEnable(code),
    onSuccess: async (r) => { setEnrol(null); setCodes(r.data.recovery_codes); await reload(); toast.success('Two-factor authentication enabled') },
    onError: onErr,
  })
  const off = useMutation({
    mutationFn: () => twoFADisable(disable.password, disable.code),
    onSuccess: async () => { setDisable(null); await reload(); toast.success('Two-factor authentication disabled') },
    onError: onErr,
  })
  const newCodes = useMutation({
    mutationFn: () => twoFARecovery(regen),
    onSuccess: (r) => { setRegen(null); setCodes(r.data.recovery_codes) },
    onError: onErr,
  })

  return (
    <div className="max-w-2xl space-y-4">
      <div className="rounded-lg border border-surface-border bg-surface-card p-6">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h3 className="flex items-center gap-2 text-base font-semibold text-text-primary">
              {enabled ? <ShieldCheck size={18} className="text-success" /> : <ShieldOff size={18} className="text-text-secondary" />}
              Two-factor authentication
              <Badge variant={enabled ? 'success' : 'neutral'}>{enabled ? 'on' : 'off'}</Badge>
            </h3>
            <p className="mt-2 text-sm text-text-secondary">
              Adds a 6-digit code from an authenticator app (Google Authenticator, Authy, 1Password…) to every sign-in. Strongly recommended: this panel can run commands as root.
            </p>
          </div>
          <div className="flex shrink-0 gap-2">
            {enabled ? (
              <>
                <Button variant="secondary" onClick={() => setRegen('')}>New recovery codes</Button>
                <Button variant="danger" onClick={() => setDisable({ password: '', code: '' })}>Disable</Button>
              </>
            ) : (
              <Button disabled={setup.isPending} onClick={() => setup.mutate()}>Enable</Button>
            )}
          </div>
        </div>
      </div>

      <Modal open={!!enrol} onClose={() => setEnrol(null)} title="Set up authenticator">
        {enrol && (
          <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); enable.mutate() }}>
            <p className="text-sm text-text-secondary">1. Scan this QR code with your authenticator app.</p>
            <img src={enrol.qr} alt="2FA QR code" className="mx-auto h-48 w-48 rounded border border-surface-border" />
            <p className="break-all text-center font-mono text-xs text-text-secondary">Or enter manually: {enrol.secret}</p>
            <p className="text-sm text-text-secondary">2. Enter the 6-digit code it shows to confirm.</p>
            <Input label="Code" autoFocus inputMode="numeric" autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value)} />
            <div className="flex justify-end gap-2">
              <Button type="button" variant="secondary" onClick={() => setEnrol(null)}>Cancel</Button>
              <Button type="submit" disabled={enable.isPending || code.trim().length < 6}>Confirm & enable</Button>
            </div>
          </form>
        )}
      </Modal>

      <Modal open={!!codes} onClose={() => setCodes(null)} title="Recovery codes">
        <div className="space-y-4">
          <p className="text-sm text-text-secondary">Save these codes somewhere safe. Each works once if you lose your authenticator. <b>They will not be shown again.</b></p>
          <div className="grid grid-cols-2 gap-2 rounded-md bg-surface-hover p-4 font-mono text-sm">
            {(codes || []).map((c) => <span key={c}>{c}</span>)}
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => navigator.clipboard?.writeText((codes || []).join('\n')).then(() => toast.success('Copied'))}>Copy</Button>
            <Button onClick={() => setCodes(null)}>I saved them</Button>
          </div>
        </div>
      </Modal>

      <Modal open={!!disable} onClose={() => setDisable(null)} title="Disable two-factor authentication">
        {disable && (
          <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); off.mutate() }}>
            <Input label="Password" type="password" value={disable.password} onChange={(e) => setDisable({ ...disable, password: e.target.value })} autoFocus />
            <Input label="Code (or recovery code)" value={disable.code} onChange={(e) => setDisable({ ...disable, code: e.target.value })} />
            <div className="flex justify-end gap-2">
              <Button type="button" variant="secondary" onClick={() => setDisable(null)}>Cancel</Button>
              <Button type="submit" variant="danger" disabled={off.isPending || !disable.password || !disable.code}>Disable</Button>
            </div>
          </form>
        )}
      </Modal>

      <Modal open={regen !== null} onClose={() => setRegen(null)} title="Replace recovery codes">
        <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); newCodes.mutate() }}>
          <p className="text-sm text-text-secondary">Existing recovery codes stop working. Confirm with a current code.</p>
          <Input label="Code" autoFocus value={regen || ''} onChange={(e) => setRegen(e.target.value)} />
          <div className="flex justify-end gap-2">
            <Button type="button" variant="secondary" onClick={() => setRegen(null)}>Cancel</Button>
            <Button type="submit" disabled={newCodes.isPending || !regen}>Generate</Button>
          </div>
        </form>
      </Modal>
    </div>
  )
}

export default Security
