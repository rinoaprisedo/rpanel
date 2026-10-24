// Converts a `#rrggbb` hex color into a Tailwind-compatible "r g b" channel
// string, so it can back a CSS var that Tailwind's rgb(var(...) / <alpha>)
// colors (and their opacity modifiers, e.g. bg-primary/10) consume directly.
export function hexToRgbChannels(hex) {
  const clean = hex.replace('#', '')
  const r = parseInt(clean.slice(0, 2), 16)
  const g = parseInt(clean.slice(2, 4), 16)
  const b = parseInt(clean.slice(4, 6), 16)
  return `${r} ${g} ${b}`
}

// Darkens a hex color by `amount` (0-1) for the "dark" shade variant used on
// hover states, keeping it a single source of truth (the picked base color).
export function darkenHex(hex, amount = 0.15) {
  const clean = hex.replace('#', '')
  const r = Math.round(parseInt(clean.slice(0, 2), 16) * (1 - amount))
  const g = Math.round(parseInt(clean.slice(2, 4), 16) * (1 - amount))
  const b = Math.round(parseInt(clean.slice(4, 6), 16) * (1 - amount))
  return `${r} ${g} ${b}`
}

export function applyPrimaryColor(hex) {
  if (!hex) return
  const root = document.documentElement
  root.style.setProperty('--color-primary-rgb', hexToRgbChannels(hex))
  root.style.setProperty('--color-primary-dark-rgb', darkenHex(hex))
}
