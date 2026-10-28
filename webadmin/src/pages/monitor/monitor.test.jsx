import { describe, it, expect, vi, beforeAll } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'

vi.mock('../../api/dashboard', () => {
  const now = Math.floor(Date.now() / 1000)
  const points = Array.from({ length: 12 }, (_, i) => ({ t: now - (12 - i) * 60, cpu: 10 + i, mem: 40, net_rx: 1000 * i, net_tx: 500 * i, load1: 0.3, disk: 40, disk_read: 100 * i, disk_write: 50 * i }))
  const overview = {
    hostname: 'toi-production', os: 'ubuntu 26.04', kernel: '7.0.0', arch: 'x86_64', uptime: 90000, cpu_model: 'Intel(R) Xeon(R) CPU', cpu_cores: 2, cpu_percent: 12, per_cpu: [10, 90],
    mem_total: 4e9, mem_used: 1e9, mem_percent: 25, mem_available: 2.5e9, mem_cached: 1e9, mem_buffers: 1e8, swap_total: 0, swap_used: 0, load1: 0.2, load5: 0.3, load15: 0.4,
    net_rx_bps: 1000, net_tx_bps: 500, net_rx_total: 1e9, net_tx_total: 5e8, disk_read_bps: 2000, disk_write_bps: 1000, processes: 150, boot_time: now - 90000,
    interfaces: [{ name: 'eth0', ip: '213.163.199.221', rx_bps: 10, tx_bps: 5, rx_total: 100, tx_total: 50 }],
    tcp: { established: 12, listen: 8, time_wait: 3, close_wait: 0, syn_recv: 0, other: 0, total: 23 },
    disks: [{ mount: '/', fstype: 'ext4', total: 1e11, used: 3e10, percent: 30, inodes_percent: 5 }],
  }
  const traffic = {
    range: '24h', since: now - 86400, until: now, bucket_sec: 3600, available: true, truncated: false, timezone: 'WIB +07:00',
    totals: { requests: 1000, page_views: 600, visitors: 120, bytes: 5e7, ok: 900, client_err: 80, server_err: 20, bots: 300 },
    previous: { requests: 800, page_views: 500, visitors: 100, bytes: 4e7, ok: 700, client_err: 80, server_err: 20, bots: 100 },
    buckets: Array.from({ length: 24 }, (_, i) => ({ t: now - (24 - i) * 3600, ok: 30, client_err: 3, server_err: 1, bytes: 1e6, visitors: 5 })),
    sites: [{ domain: 'a.com', requests: 1000, visitors: 120, client_err: 80, server_err: 20, bytes: 5e7, ok: 900, bots: 300 }],
    top_paths: [{ key: '/', count: 300, errors: 0 }], top_ips: [{ key: '1.1.1.1', count: 50 }], statuses: [{ key: '200', count: 900 }, { key: '404', count: 80 }],
    methods: [{ key: 'GET', count: 950 }], referrers: [{ key: 'google.com', count: 40 }], browsers: [{ key: 'Chrome', count: 500 }], systems: [{ key: 'Windows', count: 400 }],
    devices: [{ key: 'Desktop', count: 500 }, { key: 'Mobile', count: 200 }, { key: 'Bot', count: 300 }], bots: [{ key: 'Googlebot', count: 100 }],
    not_found: [{ key: '/wp-login.php', count: 30 }], error_paths: [{ key: '/api/pay', count: 5 }],
    hours: Array.from({ length: 24 }, (_, i) => i * 3), weekdays: [10, 20, 30, 40, 50, 60, 70],
    classes: [{ key: '2xx', count: 850 }, { key: '3xx', count: 50 }, { key: '4xx', count: 80 }, { key: '5xx', count: 20 }],
    latest: [{ t: now - 20, site: 'a.com', ip: '1.1.1.1', method: 'GET', path: '/pricing', status: 200, bytes: 2048, client: 'Chrome · Windows' }],
    live: { last_5m: 25, per_min: 5, visitors: 7 }, domains: ['a.com'],
  }
  const insights = {
    health: { score: 90, level: 'smooth', label: 'Running smoothly', bottleneck: '', factors: [{ id: 'cpu', label: 'CPU', value: '12%', used: 12, score: 95, weight: 25, hint: '' }] },
    insights: [{ id: 'ok', severity: 'ok', title: 'Everything looks healthy', detail: '' }], updates: { known: true, total: 3 }, firewall: { known: true, active: true },
    services: [{ id: 'nginx', name: 'Nginx', running: true, version: '1.28' }], sites: [{ domain: 'a.com', status: 'running', ssl_enabled: true, days_left: 60 }],
  }
  return {
    getOverview: vi.fn(() => Promise.resolve({ data: overview })),
    getHistory: vi.fn(() => Promise.resolve({ data: points })),
    getHistoryRange: vi.fn(() => Promise.resolve({ data: points })),
    getProcesses: vi.fn(() => Promise.resolve({ data: [{ pid: 1, name: 'nginx', user: 'root', cpu: 1.5, mem: 0.5 }] })),
    getInsights: vi.fn(() => Promise.resolve({ data: insights })),
    getTraffic: vi.fn(() => Promise.resolve({ data: traffic })),
  }
})
vi.mock('../../api/activityLogs', () => ({ getActivityLogs: vi.fn(() => Promise.resolve({ data: [] })) }))

import MonitorServer from './MonitorServer'
import MonitorTraffic from './MonitorTraffic'

beforeAll(() => {
  globalThis.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} }
})

function renderPage(ui) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={qc}><MemoryRouter>{ui}</MemoryRouter></QueryClientProvider>)
}

describe('monitor pages', () => {
  it('Monitor Server shows system facts, per-core CPU, memory, connections, interfaces and processes', async () => {
    renderPage(<MonitorServer />)
    await waitFor(() => expect(screen.getAllByText('toi-production').length).toBeGreaterThan(0))
    expect(screen.getByText('CPU per core')).toBeTruthy()
    expect(screen.getByText('Memory breakdown')).toBeTruthy()
    expect(screen.getByText('Connections', { selector: 'h3' })).toBeTruthy()
    expect(screen.getByText('Network interfaces')).toBeTruthy()
    expect(screen.getByText('eth0')).toBeTruthy()
    expect(screen.getByText('Disk I/O', { selector: 'h3' })).toBeTruthy()
    expect(screen.getByText('Top processes')).toBeTruthy()
    expect(screen.getByText('Core 2')).toBeTruthy()
  })

  it('Monitor Traffic shows headline numbers, audience, problems and the live feed', async () => {
    renderPage(<MonitorTraffic />)
    await waitFor(() => expect(screen.getByText('Unique visitors', { selector: 'p' })).toBeTruthy())
    expect(screen.getByText('Page views')).toBeTruthy()
    expect(screen.getByText('By hour of day')).toBeTruthy()
    expect(screen.getByText('Top referrers')).toBeTruthy()
    expect(screen.getByText('Browsers')).toBeTruthy()
    expect(screen.getByText('Most requested missing pages (404)')).toBeTruthy()
    expect(screen.getByText('/wp-login.php')).toBeTruthy()
    expect(screen.getByText('Latest requests')).toBeTruthy()
    expect(screen.getByText('/pricing', { exact: false })).toBeTruthy()
    expect(screen.getByText('Googlebot')).toBeTruthy()
  })
})
