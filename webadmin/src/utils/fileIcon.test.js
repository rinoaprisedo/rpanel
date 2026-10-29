import { describe, it, expect } from 'vitest'
import { iconFor } from './fileIcon'

describe('iconFor', () => {
  it('tells folders, special folders and links apart', () => {
    const plain = iconFor({ name: 'src', is_dir: true })
    expect(plain.label).toBe('Folder')
    expect(iconFor({ name: '.git', is_dir: true }).label).toBe('Git repository data')
    expect(iconFor({ name: '.cache', is_dir: true }).label).toBe('Hidden folder')
    expect(iconFor({ name: 'ssl', is_dir: true }).color).not.toBe(plain.color)
    expect(iconFor({ name: 'current', is_dir: true, is_link: true }).label).toBe('Folder link')
  })

  it('colors files by type and flags secrets', () => {
    expect(iconFor({ name: 'index.PHP' }).label).toBe('PHP')
    expect(iconFor({ name: 'app.js' }).color).not.toBe(iconFor({ name: 'app.css' }).color)
    expect(iconFor({ name: 'logo.webp' }).label).toBe('Image')
    expect(iconFor({ name: 'site.tar.gz' }).label).toBe('Archive')
    expect(iconFor({ name: '.env.production' }).label).toBe('Secret / credentials')
    expect(iconFor({ name: 'Dockerfile' }).label).toBe('Dockerfile')
    expect(iconFor({ name: 'LICENSE' }).label).toBe('File')
    expect(iconFor({ name: 'latest.log', is_link: true }).label).toBe('Link')
  })
})
