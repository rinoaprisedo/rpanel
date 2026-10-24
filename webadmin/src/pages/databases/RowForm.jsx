import { useState } from 'react'
import Modal from '../../components/ui/Modal'
import Button from '../../components/ui/Button'

const isLong = (t) => /text|json|blob|xml/i.test(t)

// Edits one row. On update only changed columns are sent, so large values that
// the grid showed truncated are never written back by accident.
function RowForm({ columns, row, busy, onSubmit, onClose }) {
  const editing = !!row
  const [vals, setVals] = useState(() => Object.fromEntries(columns.map((c, i) => [c.name, {
    v: row ? (row[i] ?? '') : '',
    null: row ? row[i] === null : false,
    touched: false,
  }])))
  const set = (name, patch) => setVals((s) => ({ ...s, [name]: { ...s[name], ...patch, touched: true } }))

  const submit = (e) => {
    e.preventDefault()
    const out = {}
    columns.forEach((c) => {
      const x = vals[c.name]
      if (editing ? (x.touched && !c.primary) : (x.touched && (x.null || x.v !== ''))) out[c.name] = x.null ? null : String(x.v)
    })
    onSubmit(out)
  }

  return (
    <Modal open onClose={onClose} title={editing ? 'Edit row' : 'Insert row'} size="lg">
      <form onSubmit={submit} className="space-y-4">
        <div className="max-h-[55vh] space-y-3 overflow-y-auto pr-1">
          {columns.map((c) => {
            const x = vals[c.name]
            const locked = editing && c.primary
            const cls = 'w-full rounded-md border-[1.5px] border-surface-border bg-field px-3 py-2 font-mono text-xs outline-none focus:border-primary disabled:opacity-60'
            return (
              <div key={c.name}>
                <div className="mb-1 flex items-center justify-between gap-2">
                  <label className="text-xs font-medium text-text-primary">
                    {c.primary && '🔑 '}{c.name} <span className="ml-1 font-normal text-text-tertiary">{c.type}{c.extra ? ` · ${c.extra}` : ''}</span>
                  </label>
                  {c.nullable && !locked && (
                    <label className="flex items-center gap-1 text-[11px] text-text-secondary">
                      <input type="checkbox" checked={x.null} onChange={(e) => set(c.name, { null: e.target.checked })} />NULL
                    </label>
                  )}
                </div>
                {isLong(c.type)
                  ? <textarea rows={3} spellCheck={false} className={cls} disabled={locked || x.null} value={x.null ? '' : x.v} onChange={(e) => set(c.name, { v: e.target.value, null: false })} />
                  : <input className={cls} disabled={locked || x.null} value={x.null ? '' : x.v} placeholder={!editing && c.default ? `default: ${c.default}` : ''} onChange={(e) => set(c.name, { v: e.target.value, null: false })} />}
              </div>
            )
          })}
        </div>
        {!editing && <p className="text-xs text-text-secondary">Columns left empty use their default value (auto-increment, current timestamp, …).</p>}
        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={onClose}>Cancel</Button>
          <Button type="submit" disabled={busy}>{busy ? 'Saving…' : editing ? 'Save changes' : 'Insert'}</Button>
        </div>
      </form>
    </Modal>
  )
}

export default RowForm
