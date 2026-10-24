import { Inbox } from 'lucide-react'

function EmptyState({ icon: Icon = Inbox, title = 'No data found', message }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 py-16 text-center">
      <Icon size={40} className="text-text-tertiary" strokeWidth={1.7} />
      <p className="text-sm font-medium text-text-primary">{title}</p>
      {message && <p className="text-xs text-text-tertiary">{message}</p>}
    </div>
  )
}

export default EmptyState
