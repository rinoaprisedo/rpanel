import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { highlightHtml, languageFor, LANG_LABEL } from '../../utils/highlight'

const INDENT = '  '

// A small IDE-style code view: line numbers, syntax colors, tab/auto-indent and Ctrl/Cmd+S.
//
// The <textarea> is the one and only scroller: it has a fixed size, so the browser scrolls it to keep the
// caret in view exactly like any native text field. The highlighted <pre> and the line numbers sit behind it
// and are moved with transforms to follow its scroll position. (An earlier version let the outer box scroll
// while the textarea was as tall as the whole file, and the caret then drifted, vanished or made the page jump.)
// The textarea text is transparent, so only the highlighted copy is seen; both use identical font metrics.
function CodeEditor({ path, value, onChange, onSave, readOnly = false, autoFocus = false, className = 'h-[60vh]' }) {
  const language = languageFor(path)
  const html = useMemo(() => highlightHtml(value, language) + '\n', [value, language])
  const lineCount = useMemo(() => value.split('\n').length, [value])
  const [pos, setPos] = useState({ line: 1, col: 1 })
  const areaRef = useRef(null)
  const preRef = useRef(null)
  const gutterRef = useRef(null)
  const gutterWidth = `calc(${Math.max(String(lineCount).length, 2)}ch + 28px)`

  const syncScroll = () => {
    const el = areaRef.current
    if (!el || !preRef.current || !gutterRef.current) return
    preRef.current.style.transform = `translate(${-el.scrollLeft}px, ${-el.scrollTop}px)`
    gutterRef.current.style.transform = `translateY(${-el.scrollTop}px)`
  }
  // Content changes can clamp the scroll position; keep the layers aligned right away.
  useLayoutEffect(syncScroll, [html, gutterWidth])

  useEffect(() => {
    if (autoFocus && !readOnly) areaRef.current?.focus({ preventScroll: true })
  }, [autoFocus, readOnly])

  const track = (el) => {
    const before = el.value.slice(0, el.selectionStart)
    const lines = before.split('\n')
    setPos({ line: lines.length, col: lines[lines.length - 1].length + 1 })
  }

  // Types text at the selection through the browser, which keeps Undo/Redo working and fires a normal
  // input event (so state follows). setRangeText is only a fallback.
  const insert = (el, text) => {
    el.focus({ preventScroll: true })
    if (document.execCommand && document.execCommand('insertText', false, text)) return
    el.setRangeText(text, el.selectionStart, el.selectionEnd, 'end')
    onChange(el.value)
    track(el)
  }

  // Tab / Shift+Tab over several lines indents / outdents every line they touch.
  const shiftLines = (el, outdent) => {
    const { value: v, selectionStart: s, selectionEnd: e } = el
    const from = v.lastIndexOf('\n', s - 1) + 1
    const endLine = v.indexOf('\n', e > s && v[e - 1] === '\n' ? e - 1 : e)
    const to = endLine === -1 ? v.length : endLine
    const block = v.slice(from, to)
    const next = block
      .split('\n')
      .map((l) => (outdent ? l.replace(/^( {1,2}|\t)/, '') : INDENT + l))
      .join('\n')
    if (next === block) return
    el.setSelectionRange(from, to)
    insert(el, next)
    el.setSelectionRange(from, from + next.length)
    track(el)
  }

  const onKeyDown = (e) => {
    const el = e.currentTarget
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
      e.preventDefault()
      onSave?.()
    } else if (readOnly || e.nativeEvent.isComposing) {
      return
    } else if (e.key === 'Tab') {
      e.preventDefault()
      const multi = el.value.slice(el.selectionStart, el.selectionEnd).includes('\n')
      if (multi || e.shiftKey) shiftLines(el, e.shiftKey)
      else insert(el, INDENT)
    } else if (e.key === 'Enter' && !e.metaKey && !e.ctrlKey && !e.altKey && el.selectionStart === el.selectionEnd) {
      e.preventDefault()
      const lineStart = el.value.lastIndexOf('\n', el.selectionStart - 1) + 1
      const indent = el.value.slice(lineStart, el.selectionStart).match(/^[ \t]*/)[0]
      insert(el, '\n' + indent)
    }
  }

  return (
    <div className="overflow-hidden rounded-md border border-ide-border">
      <div className={`code-ide relative overflow-hidden ${className}`}>
        <div className="code-ide-gutter absolute inset-y-0 left-0 select-none overflow-hidden" style={{ width: gutterWidth }} aria-hidden>
          <div ref={gutterRef} className="py-3 pr-3 text-right will-change-transform">
            {Array.from({ length: lineCount }, (_, i) => <div key={i}>{i + 1}</div>)}
          </div>
        </div>
        <div className="pointer-events-none absolute inset-y-0 right-0 overflow-hidden" style={{ left: gutterWidth }} aria-hidden>
          <pre ref={preRef} className="code-ide-text m-0 w-max min-w-full px-4 py-3 will-change-transform" dangerouslySetInnerHTML={{ __html: html }} />
        </div>
        <textarea
          ref={areaRef}
          spellCheck={false}
          autoCorrect="off"
          autoCapitalize="off"
          autoComplete="off"
          wrap="off"
          readOnly={readOnly}
          value={value}
          onChange={(e) => { onChange(e.target.value); track(e.target) }}
          onKeyDown={onKeyDown}
          onScroll={syncScroll}
          onSelect={(e) => track(e.currentTarget)}
          style={{ left: gutterWidth }}
          className="code-ide-text absolute inset-y-0 right-0 resize-none border-0 bg-transparent px-4 py-3 text-transparent outline-none"
        />
      </div>
      <div className="flex items-center justify-between bg-ide-bar px-3 py-1 font-mono text-[11px] text-ide-muted">
        <span>{LANG_LABEL[language]}{readOnly ? ' · read only' : ''}</span>
        <span>Ln {pos.line}, Col {pos.col} · {lineCount} lines</span>
      </div>
    </div>
  )
}

export default CodeEditor
