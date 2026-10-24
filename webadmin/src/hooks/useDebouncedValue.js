import { useEffect, useState } from 'react'

// Returns value once it has stopped changing for delay ms, so a search box
// queries the server when typing pauses instead of on every keystroke.
export default function useDebouncedValue(value, delay = 350) {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), delay)
    return () => clearTimeout(t)
  }, [value, delay])
  return debounced
}
