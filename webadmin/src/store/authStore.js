import { create } from 'zustand'

const useAuthStore = create((set) => ({
  user: null,
  roles: [],
  permissions: [],
  isAuthenticated: false,
  setUser: (payload) =>
    set({
      user: payload?.user ?? payload ?? null,
      roles: payload?.roles ?? [],
      permissions: payload?.permissions ?? [],
      isAuthenticated: !!(payload?.user ?? payload),
    }),
  logout: () =>
    set({ user: null, roles: [], permissions: [], isAuthenticated: false }),
}))

export default useAuthStore
