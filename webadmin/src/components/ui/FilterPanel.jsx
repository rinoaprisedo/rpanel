import Button from './Button'
import Input from './Input'

// Body of a Menubar "Filter" dropdown: fields + Reset/Apply.
function FilterPanel({ close, onApply, onReset, children }) {
  return (
    <div className="w-full p-2 sm:w-80">
      <div className="grid grid-cols-2 gap-3">{children}</div>
      <div className="mt-3 flex items-center justify-end gap-2 border-t border-surface-border pt-3">
        <Button
          variant="secondary"
          className="h-[32px] px-3"
          onClick={() => {
            onReset()
            close()
          }}
        >
          Reset
        </Button>
        <Button
          className="h-[32px] px-3"
          onClick={() => {
            onApply()
            close()
          }}
        >
          Apply
        </Button>
      </div>
    </div>
  )
}

export function DateRangeFields({ from, to, labels = ['Dari tanggal', 'Sampai tanggal'] }) {
  return (
    <>
      <Input label={labels[0]} type="date" {...from} />
      <Input label={labels[1]} type="date" {...to} />
    </>
  )
}

export default FilterPanel
