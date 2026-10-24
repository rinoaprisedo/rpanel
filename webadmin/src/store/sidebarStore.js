import { create } from 'zustand'

// Below the `lg` breakpoint the sidebar is an off-canvas drawer, so it must
// start collapsed on mobile; on desktop it's always-visible, so start open.
const isMobile = typeof window !== 'undefined' && window.innerWidth < 1024

const useSidebarStore = create((set) => ({
  isCollapsed: isMobile,
  activeMenu: null,
  toggle: () => set((state) => ({ isCollapsed: !state.isCollapsed })),
  close: () => set({ isCollapsed: true }),
  setActiveMenu: (menu) => set({ activeMenu: menu }),
}))

export default useSidebarStore
