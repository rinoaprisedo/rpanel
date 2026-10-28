import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import Markdown from './Markdown'

describe('Markdown', () => {
  it('renders headings, lists, bold, code and https links', () => {
    const { container } = render(
      <Markdown text={'## 1.4.0\n\n- **Fail2ban** page\n- uses `apt`\n\nSee [notes](https://github.com/x/y) now.\n\n1. one\n2. two'} />,
    )
    expect(screen.getByText('1.4.0')).toBeTruthy()
    expect(container.querySelectorAll('ul li').length).toBe(2)
    expect(container.querySelectorAll('ol li').length).toBe(2)
    expect(container.querySelector('strong').textContent).toBe('Fail2ban')
    expect(container.querySelector('code').textContent).toBe('apt')
    const a = container.querySelector('a')
    expect(a.getAttribute('href')).toBe('https://github.com/x/y')
    expect(a.getAttribute('rel')).toContain('noopener')
  })

  it('never turns untrusted text into markup or non-http links', () => {
    const { container } = render(
      <Markdown text={'<img src=x onerror=alert(1)> [x](javascript:alert(1)) <script>alert(1)</script>'} />,
    )
    expect(container.querySelector('img')).toBeNull()
    expect(container.querySelector('script')).toBeNull()
    expect(container.querySelector('a[href^="javascript"]')).toBeNull()
    expect(container.textContent).toContain('<script>alert(1)</script>')
  })

  it('renders fenced code blocks verbatim', () => {
    const { container } = render(<Markdown text={'```\nsudo rpanel-update\n```'} />)
    expect(container.querySelector('pre').textContent).toBe('sudo rpanel-update')
  })
})
