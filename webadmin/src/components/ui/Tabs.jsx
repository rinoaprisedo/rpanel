import Select from './Select'

// Tab bar from `sm` up; below that the tabs collapse into a dropdown so a long
// list never hides behind a horizontal scroll nobody notices.
function Tabs({ tabs, value, onChange }) {
  return (
    <>
      <div className="sm:hidden">
        <Select value={value} onChange={(e) => onChange(e.target.value)} aria-label="Section">
          {tabs.map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </Select>
      </div>
      <div className="hidden gap-1 overflow-x-auto border-b border-surface-border [scrollbar-width:none] sm:flex [&::-webkit-scrollbar]:hidden">
        {tabs.map((t) => (
          <button
            key={t}
            onClick={() => onChange(t)}
            className={`shrink-0 whitespace-nowrap px-4 py-2 text-sm font-medium ${
              value === t ? 'border-b-2 border-primary text-primary' : 'text-text-secondary hover:text-text-primary'
            }`}
          >
            {t}
          </button>
        ))}
      </div>
    </>
  )
}

export default Tabs
