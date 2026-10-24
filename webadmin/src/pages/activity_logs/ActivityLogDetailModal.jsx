import Modal from '../../components/ui/Modal'

// old_value/new_value come from the API as JSON-encoded strings (the
// activity log worker stores them as jsonb text), not parsed objects, so
// they need a parse pass before pretty-printing — otherwise JSON.stringify
// just re-quotes the whole thing as one unbroken line.
function formatValue(value) {
  if (value === null || value === undefined) return 'null'
  let parsed = value
  if (typeof value === 'string') {
    try {
      parsed = JSON.parse(value)
    } catch {
      return value
    }
  }
  return JSON.stringify(parsed, null, 2)
}

function JsonBlock({ value }) {
  return (
    <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-words rounded-md border border-surface-border bg-surface-bg p-3 text-xs text-text-primary">
      {formatValue(value)}
    </pre>
  )
}

function ActivityLogDetailModal({ open, onClose, log }) {
  return (
    <Modal open={open} onClose={onClose} title="Activity Log Detail" size="xl">
      {log && (
        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
            <div>
              <p className="text-text-secondary">Action</p>
              <p className="font-medium capitalize text-text-primary">{log.action}</p>
            </div>
            <div>
              <p className="text-text-secondary">Module</p>
              <p className="font-medium text-text-primary">{log.module}</p>
            </div>
            <div>
              <p className="text-text-secondary">Record ID</p>
              <p className="font-medium text-text-primary">{log.record_id || '-'}</p>
            </div>
            <div>
              <p className="text-text-secondary">IP Address</p>
              <p className="font-medium text-text-primary">{log.ip_address}</p>
            </div>
          </div>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <div>
              <p className="mb-1 text-sm font-medium text-text-secondary">Old Value</p>
              <JsonBlock value={log.old_value} />
            </div>
            <div>
              <p className="mb-1 text-sm font-medium text-text-secondary">New Value</p>
              <JsonBlock value={log.new_value} />
            </div>
          </div>
        </div>
      )}
    </Modal>
  )
}

export default ActivityLogDetailModal
