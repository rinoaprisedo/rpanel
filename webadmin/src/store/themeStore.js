import { create } from 'zustand'
import useSettingsStore from './settingsStore'
import { applyPrimaryColor } from '../utils/color'

const KEY = 'rpanel-theme'

// Dark unless the user explicitly chose light.
const initial = () => {
  try {
    return localStorage.getItem(KEY) === 'light' ? 'light' : 'dark'
  } catch {
    return 'dark'
  }
}

// The brand color is recomputed on every switch because dark mode uses a lighter shade of it.
const apply = (theme) => {
  document.documentElement.classList.toggle('dark', theme === 'dark')
  applyPrimaryColor(useSettingsStore.getState().primaryColor)
}

const useThemeStore = create((set, get) => ({
  theme: initial(),
  setTheme: (theme) => {
    try {
      localStorage.setItem(KEY, theme)
    } catch {
      // storage can be blocked; the theme then lasts for this session only
    }
    apply(theme)
    set({ theme })
  },
  toggle: () => get().setTheme(get().theme === 'dark' ? 'light' : 'dark'),
}))

// Applied at import time, before React renders, so a reload does not flash the light theme.
apply(useThemeStore.getState().theme)

export default useThemeStore
