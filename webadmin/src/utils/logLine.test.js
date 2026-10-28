import { describe, it, expect } from 'vitest'
import { parseAccess, statusTone, splitHits } from './logLine'

const line = '13.222.121.174 - - [28/Oct/2026:04:38:38 +0700] "GET /favicon.ico HTTP/1.1" 200 956 "-" "Mozilla/5.0 (X11)"'

describe('logLine', () => {
  it('parses a combined access-log line', () => {
    const p = parseAccess(line)
    expect(p.ip).toBe('13.222.121.174')
    expect(p.time).toBe('28/Oct/2026:04:38:38 +0700')
    expect(p.request).toBe('GET /favicon.ico HTTP/1.1')
    expect(p.status).toBe(200)
    expect(p.size).toBe('956')
    expect(p.agent).toContain('Mozilla')
    expect(p.site).toBeUndefined()
  })

  it('keeps the [domain] prefix used by the "all websites" view', () => {
    expect(parseAccess(`[a.example.com] ${line}`).site).toBe('a.example.com')
  })

  it('returns null for other formats and colours status classes', () => {
    expect(parseAccess('2026/10/28 04:38:38 [error] 1#1: open() failed')).toBeNull()
    expect(statusTone(200)).toContain('emerald')
    expect(statusTone(404)).toContain('amber')
    expect(statusTone(502)).toContain('red')
  })

  it('splits keyword hits case-insensitively', () => {
    expect(splitHits('GET /Login now', 'login')).toEqual([
      { text: 'GET /', hit: false }, { text: 'Login', hit: true }, { text: ' now', hit: false },
    ])
    expect(splitHits('abc', '')).toEqual([{ text: 'abc', hit: false }])
  })
})
