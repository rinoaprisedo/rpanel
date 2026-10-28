import { Fragment } from 'react'

// Small, dependency-free Markdown renderer for GitHub release notes. It builds React elements only (no
// innerHTML), and links are limited to http(s) URLs, so untrusted text cannot inject markup.
function inline(text, keyBase) {
  const out = []
  const re = /(`[^`]+`)|(\*\*[^*]+\*\*)|(\[[^\]]+\]\((https?:\/\/[^)\s]+)\))|(\bhttps?:\/\/[^\s<)]+)/g
  let last = 0
  let m
  let i = 0
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) out.push(text.slice(last, m.index))
    const k = `${keyBase}-${i++}`
    if (m[1]) out.push(<code key={k} className="rounded bg-surface-hover px-1 py-0.5 font-mono text-[12px]">{m[1].slice(1, -1)}</code>)
    else if (m[2]) out.push(<strong key={k} className="font-semibold text-text-primary">{m[2].slice(2, -2)}</strong>)
    else if (m[3]) {
      const label = m[3].slice(1, m[3].indexOf(']('))
      out.push(<a key={k} href={m[4]} target="_blank" rel="noopener noreferrer" className="text-primary hover:underline">{label}</a>)
    } else out.push(<a key={k} href={m[5]} target="_blank" rel="noopener noreferrer" className="break-all text-primary hover:underline">{m[5]}</a>)
    last = m.index + m[0].length
  }
  if (last < text.length) out.push(text.slice(last))
  return out
}

function Markdown({ text, className }) {
  const lines = String(text || '').replace(/\r\n/g, '\n').split('\n')
  const blocks = []
  let i = 0
  while (i < lines.length) {
    const line = lines[i]
    if (/^```/.test(line)) {
      const code = []
      i++
      while (i < lines.length && !/^```/.test(lines[i])) code.push(lines[i++])
      i++
      blocks.push({ t: 'code', v: code.join('\n') })
    } else if (/^\s*$/.test(line)) {
      i++
    } else if (/^#{1,6}\s/.test(line)) {
      const level = line.match(/^#+/)[0].length
      blocks.push({ t: 'h', level, v: line.replace(/^#+\s*/, '') })
      i++
    } else if (/^\s*([-*+]|\d+\.)\s+/.test(line)) {
      const ordered = /^\s*\d+\./.test(line)
      const items = []
      while (i < lines.length && /^\s*([-*+]|\d+\.)\s+/.test(lines[i])) {
        items.push({ v: lines[i].replace(/^\s*([-*+]|\d+\.)\s+/, ''), depth: Math.min(2, Math.floor((lines[i].match(/^\s*/)[0].length) / 2)) })
        i++
      }
      blocks.push({ t: 'list', ordered, items })
    } else if (/^>\s?/.test(line)) {
      const q = []
      while (i < lines.length && /^>\s?/.test(lines[i])) q.push(lines[i++].replace(/^>\s?/, ''))
      blocks.push({ t: 'quote', v: q.join(' ') })
    } else if (/^(-{3,}|\*{3,})\s*$/.test(line)) {
      blocks.push({ t: 'hr' })
      i++
    } else {
      const p = []
      while (i < lines.length && !/^\s*$/.test(lines[i]) && !/^(#{1,6}\s|```|>\s?|\s*([-*+]|\d+\.)\s+)/.test(lines[i])) p.push(lines[i++])
      blocks.push({ t: 'p', v: p.join(' ') })
    }
  }

  return (
    <div className={className}>
      {blocks.map((b, n) => {
        const k = `b${n}`
        if (b.t === 'h') {
          const size = b.level <= 2 ? 'text-base mt-4 mb-2' : 'text-sm mt-3 mb-1.5'
          return <p key={k} className={`${size} font-semibold text-text-primary`}>{inline(b.v, k)}</p>
        }
        if (b.t === 'p') return <p key={k} className="my-2 text-sm leading-6 text-text-secondary">{inline(b.v, k)}</p>
        if (b.t === 'quote') return <p key={k} className="my-2 border-l-2 border-surface-border-hover pl-3 text-sm italic text-text-secondary">{inline(b.v, k)}</p>
        if (b.t === 'hr') return <hr key={k} className="my-3 border-surface-border" />
        if (b.t === 'code') return <pre key={k} className="my-2 overflow-auto rounded-md bg-surface-hover p-3 font-mono text-xs text-text-primary">{b.v}</pre>
        const Tag = b.ordered ? 'ol' : 'ul'
        return (
          <Tag key={k} className={`my-2 space-y-1 pl-5 text-sm leading-6 text-text-secondary ${b.ordered ? 'list-decimal' : 'list-disc'}`}>
            {b.items.map((it, j) => <li key={j} style={{ marginLeft: it.depth * 14 }}><Fragment>{inline(it.v, `${k}-${j}`)}</Fragment></li>)}
          </Tag>
        )
      })}
    </div>
  )
}

export default Markdown
