import hljs from 'highlight.js/lib/core'
import xml from 'highlight.js/lib/languages/xml'
import css from 'highlight.js/lib/languages/css'
import javascript from 'highlight.js/lib/languages/javascript'
import typescript from 'highlight.js/lib/languages/typescript'
import json from 'highlight.js/lib/languages/json'
import php from 'highlight.js/lib/languages/php'
import bash from 'highlight.js/lib/languages/bash'
import yaml from 'highlight.js/lib/languages/yaml'
import ini from 'highlight.js/lib/languages/ini'
import nginx from 'highlight.js/lib/languages/nginx'
import sql from 'highlight.js/lib/languages/sql'
import python from 'highlight.js/lib/languages/python'
import markdown from 'highlight.js/lib/languages/markdown'
import dockerfile from 'highlight.js/lib/languages/dockerfile'
import go from 'highlight.js/lib/languages/go'
import apache from 'highlight.js/lib/languages/apache'

// Only the languages a server admin edits; keeps the bundle small.
const registry = { xml, css, javascript, typescript, json, php, bash, yaml, ini, nginx, sql, python, markdown, dockerfile, go, apache }
Object.entries(registry).forEach(([name, def]) => hljs.registerLanguage(name, def))

const byExt = {
  html: 'xml', htm: 'xml', xml: 'xml', svg: 'xml', vue: 'xml',
  css: 'css', scss: 'css', less: 'css',
  js: 'javascript', mjs: 'javascript', cjs: 'javascript', jsx: 'javascript',
  ts: 'typescript', tsx: 'typescript',
  json: 'json', php: 'php',
  sh: 'bash', bash: 'bash', zsh: 'bash',
  yml: 'yaml', yaml: 'yaml',
  ini: 'ini', cfg: 'ini', env: 'ini', properties: 'ini', toml: 'ini', service: 'ini', timer: 'ini',
  conf: 'ini', sql: 'sql', py: 'python', md: 'markdown', go: 'go', htaccess: 'apache',
}

export const LANG_LABEL = {
  xml: 'HTML/XML', css: 'CSS', javascript: 'JavaScript', typescript: 'TypeScript', json: 'JSON', php: 'PHP', bash: 'Shell',
  yaml: 'YAML', ini: 'INI/Config', nginx: 'Nginx', sql: 'SQL', python: 'Python', markdown: 'Markdown',
  dockerfile: 'Dockerfile', go: 'Go', apache: 'Apache', plaintext: 'Plain text',
}

// Picks a highlight.js language from the file path; 'plaintext' when unknown.
export function languageFor(path = '') {
  const lower = path.toLowerCase()
  const name = lower.split('/').pop() || ''
  if (name === 'dockerfile' || name.startsWith('dockerfile.')) return 'dockerfile'
  if (name === '.htaccess') return 'apache'
  if (name === '.env' || name.startsWith('.env.')) return 'ini'
  if (name === 'crontab' || name === 'makefile') return 'bash'
  if (lower.endsWith('.conf') && (lower.includes('/nginx/') || lower.includes('nginx'))) return 'nginx'
  const ext = name.includes('.') ? name.split('.').pop() : ''
  return byExt[ext] || 'plaintext'
}

export const IMAGE_EXT = ['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'ico', 'bmp', 'avif']
export const isImage = (path = '') => IMAGE_EXT.includes(path.toLowerCase().split('.').pop())

const escapeHtml = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')

// Highlighting a huge file on every keystroke would freeze the editor.
const MAX_HIGHLIGHT = 300_000

export function highlightHtml(code, language) {
  if (language === 'plaintext' || code.length > MAX_HIGHLIGHT) return escapeHtml(code)
  try {
    return hljs.highlight(code, { language, ignoreIllegals: true }).value
  } catch {
    return escapeHtml(code)
  }
}
