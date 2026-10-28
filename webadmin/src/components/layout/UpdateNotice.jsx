import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ArrowUpCircle } from 'lucide-react'
import toast from 'react-hot-toast'
import useAuthStore from '../../store/authStore'
import { getSelfUpdate } from '../../api/selfUpdate'
import UpdateDialog from './UpdateDialog'

const SKIP_KEY = 'rpanel_skipped_version'
const readSkip = () => { try { return localStorage.getItem(SKIP_KEY) } catch { return null } }

// "Update available" pill for the top bar (superadmin only), with the release notes dialog.
function UpdateNotice() {
  const isSuper = useAuthStore((s) => !!s.user?.is_superadmin)
  const [open, setOpen] = useState(false)
  const [skipped, setSkipped] = useState(readSkip())
  const { data } = useQuery({ queryKey: ['self-update'], queryFn: () => getSelfUpdate(), enabled: isSuper, staleTime: 10 * 60 * 1000, refetchInterval: 30 * 60 * 1000, retry: false })
  const info = data?.data
  if (!isSuper || !info?.available || skipped === info.latest) return null

  const skip = () => {
    try { localStorage.setItem(SKIP_KEY, info.latest) } catch { /* private mode */ }
    setSkipped(info.latest)
    setOpen(false)
    toast(`Skipped v${info.latest}. You will be notified about the next release.`)
  }

  return (
    <>
      <button
        onClick={() => setOpen(true)}
        title={`RPanel ${info.latest} is available`}
        className="inline-flex h-8 items-center gap-1.5 rounded-full bg-primary/10 px-3 text-xs font-medium text-primary transition-colors hover:bg-primary/20"
      >
        <span className="relative flex h-2 w-2"><span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-primary opacity-60" /><span className="relative inline-flex h-2 w-2 rounded-full bg-primary" /></span>
        <ArrowUpCircle size={14} className="hidden sm:block" />
        <span className="hidden sm:inline">Update available</span>
        <span className="font-mono sm:hidden">v{info.latest}</span>
      </button>
      {open && <UpdateDialog info={info} onClose={() => setOpen(false)} onSkip={skip} />}
    </>
  )
}

export default UpdateNotice
