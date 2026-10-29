import { describe, it, expect } from 'vitest'
import { computeHealth } from './health'

const idle = { cpu_percent: 2, cpu_cores: 2, load5: 0.02, mem_percent: 23, swap_total: 0, swap_used: 0, disks: [{ mount: '/', percent: 17 }] }

describe('computeHealth', () => {
  it('is smooth with no bottleneck on an idle server', () => {
    const h = computeHealth(idle, [], undefined)
    expect(h.level).toBe('smooth')
    expect(h.score).toBeGreaterThan(95)
    expect(h.bottleneck).toBe('')
  })

  it('shows the free space of the fullest disk in the label', () => {
    const o = { ...idle, disks: [{ mount: '/', percent: 17, total: 20 * 1024 ** 3, used: 5 * 1024 ** 3 }] }
    expect(computeHealth(o, [], undefined).factors.find((f) => f.id === 'disk').value).toBe('15.0 GB free · /')
  })

  it('uses sustained CPU, not one spike', () => {
    const spike = [{ cpu: 10 }, { cpu: 12 }, { cpu: 14 }, { cpu: 100 }]
    expect(computeHealth({ ...idle, cpu_percent: 100 }, spike, undefined).level).toBe('smooth')
    const hot = [{ cpu: 98 }, { cpu: 99 }, { cpu: 100 }, { cpu: 100 }]
    const h = computeHealth(idle, hot, undefined)
    expect(h.bottleneck).toBe('cpu')
    expect(h.level).not.toBe('smooth')
  })

  it('sees CPU pinned for the last 15 seconds even after minutes of calm', () => {
    const calm = Array.from({ length: 95 }, () => ({ cpu: 5 }))
    const pinned = (n) => [...calm, ...Array.from({ length: n }, () => ({ cpu: 100 }))]
    expect(computeHealth(idle, pinned(1), undefined).level).toBe('smooth') // a 3 s blip is ignored
    const h = computeHealth(idle, pinned(5), undefined) // 5 samples x 3 s = 15 s
    expect(h.bottleneck).toBe('cpu')
    expect(h.level).not.toBe('smooth')
  })

  it('caps the score when one factor fails and keeps server-side factors', () => {
    const server = { factors: [{ id: 'services', label: 'Services', value: '0 of 2 running', used: 100, score: 0, weight: 10, hint: '' }] }
    const h = computeHealth({ ...idle, disks: [{ mount: '/', percent: 99 }] }, [], server)
    expect(h.score).toBeLessThanOrEqual(60)
    expect(h.factors.some((f) => f.id === 'services')).toBe(true)
    expect(h.factors[0].score).toBeLessThanOrEqual(h.factors[1].score)
  })

  it('falls back to the server verdict until the overview loads', () => {
    const s = { score: 90 }
    expect(computeHealth(undefined, [], s)).toBe(s)
  })
})
