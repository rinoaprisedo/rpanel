import { useEffect } from 'react'
import usePageActionsStore from '../store/pageActionsStore'

// Registers a page's toolbar actions (JSX) into the Navbar's fixed slot for
// the lifetime of the component, and clears them on unmount/navigation away.
function usePageActions(actions) {
  const setPageActions = usePageActionsStore((s) => s.setPageActions)

  useEffect(() => {
    setPageActions(actions)
    return () => setPageActions(null)
  }, [actions, setPageActions])
}

export default usePageActions
