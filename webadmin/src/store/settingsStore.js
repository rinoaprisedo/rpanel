import { create } from 'zustand'
import { getPublicSettings } from '../api/settings'
import { fileURL } from '../utils/url'
import { applyPrimaryColor } from '../utils/color'

const DEFAULT_PRIMARY = '#35494d'

const useSettingsStore = create((set) => ({
  appName: 'BaseProject',
  logo: null,
  favicon: null,
  primaryColor: DEFAULT_PRIMARY,
  // Assumed reachable until the first check completes, so we don't flash the
  // offline screen before fetchSettings has had a chance to run.
  backendOnline: true,
  // Branding only — uses the unauthenticated /settings/public endpoint so it
  // resolves correctly on the login page too, not just after signing in.
  fetchSettings: async () => {
    try {
      const res = await getPublicSettings()
      // GET /settings/public returns a { key: value } map directly, not an array.
      const map = res.data || {}
      const primaryColor = map.primary_color || DEFAULT_PRIMARY
      applyPrimaryColor(primaryColor)
      set({
        appName: map.app_name || 'BaseProject',
        logo: fileURL(map.app_logo),
        favicon: fileURL(map.app_favicon),
        primaryColor,
        backendOnline: true,
      })
    } catch (err) {
      // No response at all (connection refused/timeout) means the backend is
      // down — an HTTP error response still means the server is reachable.
      if (!err.response) {
        set({ backendOnline: false })
      }
    }
  },
  setPrimaryColor: (hex) => {
    applyPrimaryColor(hex)
    set({ primaryColor: hex })
  },
}))

export default useSettingsStore
