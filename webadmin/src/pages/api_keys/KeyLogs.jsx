import { useInfiniteQuery } from '@tanstack/react-query'
import Modal from '../../components/ui/Modal'
import Button from '../../components/ui/Button'
import Skeleton from '../../components/ui/Skeleton'
import { cn } from '../../utils/cn'
import * as api from '../../api/apiKeys'

const PREFIX = '/api/v1/integrations'
const statusTone = (s) => (s >= 500 ? 'text-danger' : s >= 400 ? 'text-warning' : 'text-success')

// Request log of one API key: what it did, from which IP, and whether it worked.
function KeyLogs({ keyItem, onClose }) {
  const q = useInfiniteQuery({
    queryKey: ['api-key-logs', keyItem?.uuid],
    queryFn: ({ pageParam }) => api.getKeyLogs(keyItem.uuid, pageParam),
    initialPageParam: undefined,
    getNextPageParam: (last) => (last.data.has_more ? last.data.items[last.data.items.length - 1].id : undefined),
    enabled: !!keyItem,
    refetchInterval: 10000,
  })
  const items = q.data?.pages.flatMap((p) => p.data.items) || []

  return (
    <Modal open={!!keyItem} onClose={onClose} title={`Activity · ${keyItem?.name || ''}`} size="xl">
      {q.isLoading ? <Skeleton className="h-40 w-full" /> : !items.length ? (
        <p className="py-10 text-center text-sm text-text-secondary">No requests yet. Activity appears here as soon as a tool uses this key.</p>
      ) : (
        <div className="space-y-3">
          <div className="max-h-[60vh] overflow-auto rounded-lg border border-surface-border">
            <table className="w-full text-left text-xs">
              <thead className="sticky top-0 bg-surface-card text-text-secondary">
                <tr><th className="px-3 py-2 font-medium">Time</th><th className="px-3 py-2 font-medium">Request</th><th className="px-3 py-2 font-medium">Result</th><th className="px-3 py-2 font-medium">IP</th></tr>
              </thead>
              <tbody className="divide-y divide-surface-border">
                {items.map((l) => (
                  <tr key={l.id} className="align-top">
                    <td className="whitespace-nowrap px-3 py-2 text-text-secondary" title={l.user_agent}>{new Date(l.created_at).toLocaleString()}</td>
                    <td className="px-3 py-2">
                      <span className="font-mono text-text-primary"><span className="text-text-secondary">{l.method}</span> {(l.route || '').replace(PREFIX, '')}</span>
                      {l.note && <span className="block break-all text-text-secondary">{l.note}</span>}
                    </td>
                    <td className={cn('px-3 py-2 font-mono font-medium', statusTone(l.status))}>{l.status}</td>
                    <td className="whitespace-nowrap px-3 py-2 font-mono text-text-secondary">{l.ip_address}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {q.hasNextPage && <div className="flex justify-center"><Button variant="secondary" onClick={() => q.fetchNextPage()} disabled={q.isFetchingNextPage}>{q.isFetchingNextPage ? 'Loading…' : 'Load older'}</Button></div>}
        </div>
      )}
    </Modal>
  )
}

export default KeyLogs
