import { create } from 'zustand'

// Lets a page register the action buttons (Add New, Others dropdown, etc.)
// that the Navbar renders in its fixed top toolbar slot, so every module
// gets a consistent action-bar position instead of each page drawing its own.
const usePageActionsStore = create((set) => ({
  actions: null,
  setPageActions: (actions) => set({ actions }),
}))

export default usePageActionsStore
