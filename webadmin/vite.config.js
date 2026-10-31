import { readFileSync } from 'fs'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const pkg = JSON.parse(readFileSync(new URL('./package.json', import.meta.url)))

// The release version (the GitHub tag vX.Y.Z) is `const AppVersion` in backend/main.go; the UI ships with the
// backend in the same release, so the footer uses it. Falls back to package.json when the backend source is absent.
function releaseVersion() {
  try {
    const src = readFileSync(new URL('../backend/main.go', import.meta.url), 'utf8')
    return src.match(/^const AppVersion = "([^"]+)"/m)?.[1] || pkg.version
  } catch {
    return pkg.version
  }
}

export default defineConfig({
  plugins: [react()],
  define: {
    __APP_VERSION__: JSON.stringify(releaseVersion()),
  },
  server: { port: 8801 },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: './src/setupTests.js',
  },
})
