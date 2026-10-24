import { useMemo, useRef, useState } from 'react'
import { highlightHtml, languageFor, LANG_LABEL } from '../../utils/highlight'

// A small IDE-style code view: line numbers, syntax colors, tab/auto-indent and Ctrl/Cmd+S.
// The textarea is transparent and sits exactly on top of the highlighted <pre>, so both
// scroll together inside one container and the caret/selection always line up.
function CodeEditor({ path, value, onChange, onSave, readOnly = false, className = 'h-[60vh]' }) {
  const language = languageFor(path)
  const html = useMemo(() => highlightHtml(value, language) + '\n', [value, language])
  const lineCount = useMemo(() => value.split('\n').length, [value])
  const [pos, setPos] = useState({ line: 1, col: 1 })
  const ref = useRef(null)

  const track = (el) => {
    const before = el.value.slice(0, el.selectionStart)
    const lines = before.split('\n')
    setPos({ line: lines.length, col: lines[lines.length - 1].length + 1 })
  }

  const edit = (el, text, from, to, caret) => {
    el.setRangeText(text, from, to, 'end')
    if (caret !== undefined) el.setSelectionRange(caret, caret)
    onChange(el.value)
    track(el)
  }

  const onKeyDown = (e) => {
    const el = e.currentTarget
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
      e.preventDefault()
      onSave?.()
    } else if (readOnly) {
      return
    } else if (e.key === 'Tab' && !e.shiftKey) {
      e.preventDefault()
      edit(el, '  ', el.selectionStart, el.selectionEnd)
    } else if (e.key === 'Enter' && el.selectionStart === el.selectionEnd) {
      e.preventDefault()
      const lineStart = el.value.lastIndexOf('\n', el.selectionStart - 1) + 1
      const indent = el.value.slice(lineStart, el.selectionStart).match(/^[ \t]*/)[0]
      edit(el, '\n' + indent, el.selectionStart, el.selectionEnd)
    }
  }

  return (
    <div className="overflow-hidden rounded-md border border-ide-border">
      <div className={`code-ide flex overflow-auto ${className}`}>
        <div className="code-ide-gutter sticky left-0 z-10 shrink-0 select-none py-3 pl-3 pr-3 text-right" aria-hidden>
          {Array.from({ length: lineCount }, (_, i) => <div key={i}>{i + 1}</div>)}
        </div>
        <div className="relative w-max min-w-full">
          <pre className="code-ide-text m-0 px-4 py-3" aria-hidden dangerouslySetInnerHTML={{ __html: html }} />
          <textarea
            ref={ref}
            spellCheck={false}
            wrap="off"
            readOnly={readOnly}
            value={value}
            onChange={(e) => { onChange(e.target.value); track(e.target) }}
            onKeyDown={onKeyDown}
            onSelect={(e) => track(e.currentTarget)}
            className="code-ide-text absolute inset-0 h-full w-full resize-none overflow-hidden border-0 bg-transparent px-4 py-3 text-transparent outline-none"
          />
        </div>
      </div>
      <div className="flex items-center justify-between bg-ide-bar px-3 py-1 font-mono text-[11px] text-ide-muted">
        <span>{LANG_LABEL[language]}{readOnly ? ' · read only' : ''}</span>
        <span>Ln {pos.line}, Col {pos.col} · {lineCount} lines</span>
      </div>
    </div>
  )
}

export default CodeEditor
