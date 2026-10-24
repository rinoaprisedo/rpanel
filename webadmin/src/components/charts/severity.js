// One four-step scale for "how loaded is it": green, yellow, orange, red.
export const SEV = {
  green: 'rgb(var(--c-success))',
  yellow: '#e6b422',
  orange: '#f08a24',
  red: 'var(--viz-crit)',
}

// Load-based: thresholds are [yellow, orange, red] on the same unit as v.
export function loadColor(v, warnAt = 75, critAt = 90) {
  if (v >= critAt) return SEV.red
  if (v >= warnAt) return SEV.orange
  if (v >= warnAt * 0.67) return SEV.yellow
  return SEV.green
}

// Health score based (higher is better).
export function scoreColor(s) {
  if (s >= 85) return SEV.green
  if (s >= 65) return SEV.yellow
  if (s >= 40) return SEV.orange
  return SEV.red
}
