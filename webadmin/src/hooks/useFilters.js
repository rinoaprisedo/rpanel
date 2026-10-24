import { useCallback, useMemo, useState } from 'react'

// Draft/applied filter state for a Menubar filter panel: fields edit the
// draft, Apply commits it. `initial` must be a stable (module-level) object.
// Memoized on the draft so pages can list the helpers as usePageActions deps.
function useFilters(initial) {
  const [filters, setFilters] = useState(initial)
  const [draft, setDraft] = useState(initial)
  const helpers = useMemo(
    () => ({
      field: (key) => ({ value: draft[key], onChange: (e) => setDraft((d) => ({ ...d, [key]: e.target.value })) }),
      apply: () => setFilters(draft),
      reset: () => {
        setDraft(initial)
        setFilters(initial)
      },
    }),
    [draft, initial],
  )
  // Applies values from outside the panel (e.g. a link with a query param).
  const setBoth = useCallback((values) => {
    setDraft(values)
    setFilters(values)
  }, [])
  const active = Object.keys(initial).some((k) => filters[k] !== initial[k])
  return { filters, draft, active, setBoth, ...helpers }
}

// Drops empty values so they aren't sent as query params.
export const compact = (params) => Object.fromEntries(Object.entries(params).filter(([, v]) => v !== '' && v != null))

export default useFilters
