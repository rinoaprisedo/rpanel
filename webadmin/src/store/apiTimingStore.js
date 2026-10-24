import { create } from 'zustand'

// Tracks the most recent API call's response time — the frontend is an SPA
// (client-side routing, no full page reload between pages), so this is the
// only per-navigation "how long did loading take" signal available; the
// footer's page-load timer only ever reflects the very first load.
const useApiTimingStore = create((set) => ({
  lastDuration: null,
  setLastDuration: (ms) => set({ lastDuration: ms }),
}))

export default useApiTimingStore
