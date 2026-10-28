import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import toast from 'react-hot-toast'
import { Lock, ShieldAlert } from 'lucide-react'
import Input from '../../components/ui/Input'
import Textarea from '../../components/ui/Textarea'
import Button from '../../components/ui/Button'
import Skeleton from '../../components/ui/Skeleton'
import { getPanelSSL, issuePanelSSL, customPanelSSL } from '../../api/panelSsl'

const modeLabel = { letsencrypt: "Let's Encrypt", custom: 'Custom certificate', legacy: 'Self-signed (IP access)' }

function PanelSsl() {
  const queryClient = useQueryClient()
  const { data, isLoading } = useQuery({ queryKey: ['panel-ssl'], queryFn: getPanelSSL })
  const st = data?.data

  const [domain, setDomain] = useState('')
  const [email, setEmail] = useState('')
  const [useCustom, setUseCustom] = useState(false)
  const [cert, setCert] = useState('')
  const [key, setKey] = useState('')

  const onDone = (res) => {
    queryClient.invalidateQueries({ queryKey: ['panel-ssl'] })
    const url = res?.data?.url
    toast.success('Certificate installed')
    if (url) {
      // The panel now answers on the new host only; send the browser there.
      setTimeout(() => {
        if (window.location.host !== new URL(url).host) window.location.href = url + '/'
      }, 1500)
    }
  }
  const onError = (err) => toast.error(err.response?.data?.message || 'Failed', { duration: 8000 })

  const issue = useMutation({ mutationFn: issuePanelSSL, onSuccess: onDone, onError })
  const custom = useMutation({ mutationFn: customPanelSSL, onSuccess: onDone, onError })

  if (isLoading) return <Skeleton className="h-48 w-full" />
  if (!st?.enabled) {
    return <p className="text-sm text-text-secondary">Panel SSL is only available on servers installed with the RPanel installer.</p>
  }

  return (
    <div className="max-w-xl space-y-6">
      <div className="flex items-start gap-3 rounded-lg border border-surface-border p-4">
        {st.needs_domain ? <ShieldAlert className="mt-0.5 text-warning" size={20} /> : <Lock className="mt-0.5 text-success" size={20} />}
        <div className="min-w-0 text-sm">
          <p className="font-medium text-text-primary">
            {st.needs_domain ? 'No panel domain yet' : st.domain || 'Panel certificate'}
          </p>
          <p className="text-text-secondary">
            {modeLabel[st.mode] || st.mode}
            {st.cert && ` · ${st.cert.issuer} · expires in ${st.cert.days_left} days`}
          </p>
          {st.url && (
            <a className="break-all text-primary hover:underline" href={st.url + '/'}>
              {st.url}
            </a>
          )}
        </div>
      </div>

      {st.needs_domain && (
        <p className="rounded-lg bg-warning-bg p-3 text-sm text-warning">
          This panel is still reachable by IP address with a self-signed certificate. Set a domain below: browsers then
          show a valid padlock, HTTP is redirected to HTTPS and requests by IP are refused.
        </p>
      )}

      <div className="space-y-4">
        <h3 className="text-sm font-semibold text-text-primary">{st.domain ? 'Change domain / renew' : 'Set panel domain'}</h3>
        <Input
          label="Domain"
          placeholder="panel.example.com"
          value={domain}
          onChange={(e) => setDomain(e.target.value)}
        />
        <p className="-mt-2 text-xs text-text-tertiary">
          The domain&apos;s A record must point at this server and port 80 must be reachable. Use a dedicated host name:
          it cannot also be a website.
        </p>

        {!useCustom ? (
          <>
            <Input label="E-mail (Let's Encrypt notices)" type="email" placeholder="you@example.com" value={email} onChange={(e) => setEmail(e.target.value)} />
            <div className="flex items-center gap-3">
              <Button onClick={() => issue.mutate({ domain, email })} disabled={issue.isPending || !domain || !email}>
                {issue.isPending ? 'Requesting certificate…' : "Issue Let's Encrypt certificate"}
              </Button>
              <button type="button" className="text-sm text-text-secondary hover:underline" onClick={() => setUseCustom(true)}>
                Use my own certificate
              </button>
            </div>
            {issue.isPending && <p className="text-xs text-text-tertiary">This can take up to a minute. Keep this page open.</p>}
          </>
        ) : (
          <>
            <Textarea label="Certificate (PEM, full chain)" rows={5} value={cert} onChange={(e) => setCert(e.target.value)} className="font-mono text-xs" />
            <Textarea label="Private key (PEM)" rows={5} value={key} onChange={(e) => setKey(e.target.value)} className="font-mono text-xs" />
            <div className="flex items-center gap-3">
              <Button onClick={() => custom.mutate({ domain, cert, key })} disabled={custom.isPending || !domain || !cert || !key}>
                {custom.isPending ? 'Installing…' : 'Install certificate'}
              </Button>
              <button type="button" className="text-sm text-text-secondary hover:underline" onClick={() => setUseCustom(false)}>
                Use Let&apos;s Encrypt instead
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  )
}

export default PanelSsl
