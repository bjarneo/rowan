<script lang="ts">
  // A textarea over a highlighted layer. The textarea holds the caret and
  // selection. The layer below draws the colors. A completion list opens at
  // the caret while the user types a name. The layer and the line numbers
  // render only the lines near the view, so a script of thousands of lines
  // types and scrolls without delay.
  import { untrack } from 'svelte'
  import { Highlighter } from '../lib/sql'
  import { complete, type Completion, type Item, type Kind, type Source } from '../lib/complete'

  let {
    value,
    onchange,
    onrun,
    onselection,
    onfocus,
    hashComments = false,
    autofocus = false,
    placeholder = '',
    completion = null,
  }: {
    value: string
    onchange: (v: string) => void
    onrun?: () => void
    onselection?: (s: string) => void
    onfocus?: () => void
    hashComments?: boolean
    autofocus?: boolean
    placeholder?: string
    completion?: Source | null
  } = $props()

  let ta: HTMLTextAreaElement

  // The editor draws lines first to last. That window moves in steps of
  // CHUNK lines, so most scroll steps only move it with a transform.
  const LINE_H = 21
  const PAD = 10
  const CHUNK = 40
  let top = $state(0)
  let left = $state(0)
  let viewH = $state(400)
  const starts = $derived.by(() => {
    const out = [0]
    for (let i = value.indexOf('\n'); i >= 0; i = value.indexOf('\n', i + 1)) out.push(i + 1)
    return out
  })
  const lines = $derived(starts.length)
  const first = $derived(Math.min(lines - 1, Math.max(0, Math.floor(Math.max(0, top - PAD) / LINE_H / CHUNK) * CHUNK - CHUNK)))
  const last = $derived(Math.min(lines, first + Math.ceil(viewH / LINE_H) + 3 * CHUNK))
  const hl = new Highlighter()
  const html = $derived(hl.html(value, starts[first], last < lines ? starts[last] : value.length, hashComments))
  const shift = $derived(first * LINE_H - top)

  $effect(() => {
    if (autofocus) ta?.focus()
  })

  // A shorter value from outside, such as a new draft, can clamp the scroll
  // position without a scroll event. Read it again after each change.
  $effect(() => {
    value
    if (ta && (ta.scrollTop !== top || ta.scrollLeft !== left)) untrack(sync)
  })

  function sync() {
    top = ta.scrollTop
    left = ta.scrollLeft
    if (ac) ac = { ...ac, ...place(ac.from) }
  }

  function selChange() {
    onselection?.(ta.value.slice(ta.selectionStart, ta.selectionEnd))
  }

  // Edits go through insertText, so Ctrl+Z undoes them like typed text.
  let inserting = false
  // closer is the index of a ")" that completion inserted after a function
  // name. Typing ")" there steps over it. -1 means none.
  let closer = -1
  let lastLen = 0

  /** Moves closer when an edit adds or removes text before it. */
  function trackCloser() {
    const len = ta.value.length
    const d = len - lastLen
    lastLen = len
    if (closer < 0) return
    const at = ta.selectionStart
    if (d && at - Math.max(d, 0) <= closer) closer += d
    if (ta.value[closer] !== ')' || at > closer) closer = -1
  }
  function replace(a: number, b: number, text: string, caret = a + text.length) {
    ta.focus()
    ta.setSelectionRange(a, b)
    inserting = true
    const ok = document.execCommand('insertText', false, text)
    inserting = false
    if (!ok) {
      const v = ta.value.slice(0, a) + text + ta.value.slice(b)
      ta.value = v
      lastLen = v.length
      onchange(v)
    }
    ta.setSelectionRange(caret, caret)
    selChange()
  }

  // ---- Large pastes ----
  // WebKit inserts pasted text one line at a time, and the cost grows faster
  // than the text: 1,000 lines take more than a second, 3,000 lines about
  // 10 seconds. A large paste goes in with setRangeText instead. The browser
  // does not record that edit, so the editor keeps its own undo for it.

  const BIG_PASTE = 4000
  type Paste = { before: string; after: string; a: number; b: number; caret: number }
  let undoPastes: Paste[] = []
  let redoPastes: Paste[] = []

  function setText(v: string, a: number, b: number) {
    ta.value = v
    ta.setSelectionRange(a, b)
    lastLen = v.length
    closer = -1
    onchange(v)
    selChange()
  }

  function onPaste(e: ClipboardEvent) {
    const raw = e.clipboardData?.getData('text/plain')
    if (!raw || raw.length < BIG_PASTE) return
    e.preventDefault()
    closeAC()
    const text = raw.replace(/\r\n?/g, '\n')
    const a = ta.selectionStart
    const b = ta.selectionEnd
    const before = ta.value
    ta.setRangeText(text, a, b, 'end')
    undoPastes = [...undoPastes.slice(-19), { before, after: ta.value, a, b, caret: a + text.length }]
    redoPastes = []
    lastLen = ta.value.length
    closer = -1
    onchange(ta.value)
    selChange()
  }

  /** Undoes or redoes a large paste. Returns true when it handled the key. */
  function pasteHistory(e: KeyboardEvent) {
    const k = e.key.toLowerCase()
    const redo = (k === 'z' && e.shiftKey) || (k === 'y' && !e.shiftKey)
    if (k === 'z' && !e.shiftKey) {
      const u = undoPastes[undoPastes.length - 1]
      if (!u || ta.value !== u.after) return false
      undoPastes = undoPastes.slice(0, -1)
      redoPastes = [...redoPastes, u]
      setText(u.before, u.a, u.b)
      return true
    }
    if (redo) {
      const r = redoPastes[redoPastes.length - 1]
      if (!r || ta.value !== r.before) return false
      redoPastes = redoPastes.slice(0, -1)
      undoPastes = [...undoPastes, r]
      setText(r.after, r.caret, r.caret)
      return true
    }
    return false
  }

  // ---- Completion ----

  type Open = Completion & { x: number; y: number; up: boolean; manual: boolean }
  let ac = $state.raw<Open | null>(null)
  let sel = $state(0)
  let list = $state<HTMLDivElement>()
  const BADGE: Record<Kind, string> = { keyword: 'K', function: 'ƒ', table: 'T', view: 'V', column: 'C', schema: 'S', alias: 'A' }
  const MAX_H = 248

  // missed is a request that found no items. Columns that load later can
  // fill it, so the list opens when they arrive and the caret did not move.
  let missed: { at: number; len: number; manual: boolean } | null = null

  function openAC(manual: boolean) {
    if (!completion || ta.selectionStart !== ta.selectionEnd) return closeAC()
    const r = complete(ta.value, ta.selectionStart, completion, { hash: hashComments, manual })
    if (!r) {
      closeAC()
      missed = { at: ta.selectionStart, len: ta.value.length, manual }
      return
    }
    missed = null
    const keep = ac && ac.from === r.from ? ac.items[sel]?.label : undefined
    ac = { ...r, ...place(r.from), manual }
    const i = keep ? r.items.findIndex((it) => it.label === keep) : -1
    sel = i >= 0 ? i : 0
  }

  function closeAC() {
    missed = null
    if (ac) ac = null
  }

  let charW = 0
  let charFont = ''
  /** Screen position for the list: under the word, or above it when there is no room. */
  function place(at: number) {
    const cs = getComputedStyle(ta)
    const font = `${cs.fontWeight} ${cs.fontSize} ${cs.fontFamily}`
    if (font !== charFont) {
      const cx = document.createElement('canvas').getContext('2d')!
      cx.font = font
      charW = cx.measureText('0'.repeat(64)).width / 64
      charFont = font
    }
    const lh = parseFloat(cs.lineHeight) || 21
    const v = ta.value
    const ls = v.lastIndexOf('\n', at - 1) + 1
    let line = 0
    for (let i = v.indexOf('\n'); i >= 0 && i < ls; i = v.indexOf('\n', i + 1)) line++
    let col = 0
    for (let i = ls; i < at; i++) col += v[i] === '\t' ? 2 - (col % 2) : 1
    const r = ta.getBoundingClientRect()
    const x = r.left + parseFloat(cs.paddingLeft) + col * charW - ta.scrollLeft
    const top = r.top + parseFloat(cs.paddingTop) + line * lh - ta.scrollTop
    const up = top + lh + MAX_H > window.innerHeight && top - MAX_H > 0
    return {
      // The label in a row starts 31px from the left edge of the list.
      x: Math.max(4, Math.min(x - 31, window.innerWidth - 364)),
      y: up ? window.innerHeight - top + 2 : top + lh + 2,
      up,
    }
  }

  function accept(it: Item) {
    if (!ac) return
    const { from, to } = ac
    ac = null
    replace(from, to, it.insert, from + (it.caret ?? it.insert.length))
    if (it.caret !== undefined && it.insert.endsWith(')')) closer = from + it.insert.length - 1
    if (it.reopen) openAC(true)
  }

  // New schema data, as when columns finish loading, updates an open list
  // and retries a request that found nothing.
  $effect(() => {
    completion
    untrack(() => {
      if (ac) openAC(ac.manual)
      else if (missed && document.activeElement === ta && ta.selectionStart === missed.at && ta.value.length === missed.len) openAC(missed.manual)
    })
  })

  $effect(() => {
    const i = sel
    if (ac) untrack(() => list?.children[i]?.scrollIntoView({ block: 'nearest' }))
  })

  function onInput(e: Event) {
    onchange(ta.value)
    selChange()
    trackCloser()
    if (inserting) return
    const ie = e as InputEvent
    if (ie.isComposing) return
    if (ie.inputType === 'insertText' && ie.data && /[\w$.À-￿]$/.test(ie.data)) openAC(false)
    else if (ac && ie.inputType?.startsWith('delete')) openAC(ac.manual)
    else closeAC()
  }

  function onKey(e: KeyboardEvent) {
    const mod = e.metaKey || e.ctrlKey
    if (mod && !e.altKey && pasteHistory(e)) {
      e.preventDefault()
      closeAC()
      return
    }
    if (ac) {
      const n = ac.items.length
      const plain = !mod && !e.shiftKey && !e.altKey
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault()
        sel = (sel + (e.key === 'ArrowDown' ? 1 : n - 1)) % n
        return
      }
      if (e.key === 'PageDown' || e.key === 'PageUp') {
        e.preventDefault()
        sel = Math.max(0, Math.min(n - 1, sel + (e.key === 'PageDown' ? 8 : -8)))
        return
      }
      if (e.key === 'Escape') {
        e.preventDefault()
        e.stopPropagation()
        closeAC()
        return
      }
      if ((e.key === 'Tab' || e.key === 'Enter') && plain) {
        const it = ac.items[sel]
        // Enter on a word that is already complete adds a new line instead.
        if (e.key === 'Tab' || it.insert.toLowerCase() !== ta.value.slice(ac.from, ac.to).toLowerCase()) {
          e.preventDefault()
          accept(it)
          return
        }
        closeAC()
      } else if (e.key === 'ArrowLeft' || e.key === 'ArrowRight' || e.key === 'Home' || e.key === 'End') {
        closeAC()
      }
      // Other keys type text. onInput updates the list.
    }
    if (e.key === ')' && !mod && ta.selectionStart === closer && ta.selectionEnd === closer) {
      e.preventDefault()
      ta.setSelectionRange(closer + 1, closer + 1)
      closer = -1
      return
    }
    if (e.ctrlKey && (e.key === ' ' || e.code === 'Space')) {
      e.preventDefault()
      openAC(true)
      return
    }
    if (mod && e.key === 'Enter') {
      e.preventDefault()
      e.stopPropagation()
      closeAC()
      onrun?.()
      return
    }
    if (e.key === 'Tab' && !e.shiftKey) {
      e.preventDefault()
      replace(ta.selectionStart, ta.selectionEnd, '  ')
      return
    }
    if (e.key === 'Enter' && !e.shiftKey && !e.altKey) {
      // Keep the indent of the current line.
      const a = ta.selectionStart
      const lineStart = ta.value.lastIndexOf('\n', a - 1) + 1
      const indent = /^[ \t]*/.exec(ta.value.slice(lineStart, a))?.[0] ?? ''
      if (indent) {
        e.preventDefault()
        replace(a, ta.selectionEnd, '\n' + indent)
      }
    }
  }

  function prefixLen(label: string, prefix: string) {
    return prefix && label.toLowerCase().startsWith(prefix.toLowerCase()) ? prefix.length : 0
  }

  export function focus() {
    ta?.focus()
  }
</script>

<div class="ed">
  <div class="gut" aria-hidden="true">
    <div style="transform:translateY({shift}px)">
      {#each { length: last - first } as _, k}<div>{first + k + 1}</div>{/each}
    </div>
  </div>
  <div class="body">
    <pre aria-hidden="true"><div style="transform:translate({-left}px,{shift}px)">{@html html}</div></pre>
    <textarea
      bind:this={ta}
      bind:clientHeight={viewH}
      {value}
      {placeholder}
      spellcheck="false"
      autocomplete="off"
      autocapitalize="off"
      role="combobox"
      aria-expanded={!!ac}
      aria-controls="sql-completion"
      aria-autocomplete="list"
      oninput={onInput}
      onpaste={onPaste}
      onscroll={sync}
      onkeydown={onKey}
      onselect={selChange}
      onkeyup={selChange}
      onmousedown={() => {
        closer = -1
        closeAC()
      }}
      onmouseup={selChange}
      onfocus={() => onfocus?.()}
      onblur={() => {
        selChange()
        closer = -1
        closeAC()
      }}
    ></textarea>
  </div>
</div>

{#if ac}
  <!-- mousedown keeps the focus in the textarea, so a click picks an item. -->
  <div
    class="ac"
    id="sql-completion"
    role="listbox"
    tabindex="-1"
    bind:this={list}
    style="left:{ac.x}px;{ac.up ? `bottom:${ac.y}px` : `top:${ac.y}px`};max-height:{MAX_H}px"
    onmousedown={(e) => e.preventDefault()}
  >
    {#each ac.items as it, i}
      {@const n = prefixLen(it.label, ac.prefix)}
      <div
        class="it"
        class:on={i === sel}
        role="option"
        tabindex="-1"
        aria-selected={i === sel}
        onmousedown={(e) => {
          e.preventDefault()
          accept(it)
        }}
        onmousemove={() => sel !== i && (sel = i)}
      >
        <span class="b {it.kind}">{BADGE[it.kind]}</span>
        <span class="l">{#if n}<b>{it.label.slice(0, n)}</b>{/if}{it.label.slice(n)}</span>
        <span class="d">{it.detail}</span>
      </div>
    {/each}
  </div>
{/if}

<style>
  .ed { display: flex; height: 100%; min-height: 0; font: 400 13px/21px var(--mono); background: var(--bg); }
  .gut { width: 44px; flex: none; overflow: hidden; padding: 10px 14px 10px 0; text-align: right; color: var(--faint); user-select: none; }
  .body { position: relative; flex: 1; min-width: 0; }
  pre, textarea {
    position: absolute; inset: 0; margin: 0; padding: 10px 12px 10px 0; border: 0;
    font: inherit; white-space: pre; overflow: auto; tab-size: 2; letter-spacing: 0;
  }
  pre { color: var(--text); pointer-events: none; overflow: hidden; }
  textarea { background: transparent; color: transparent; caret-color: var(--text); resize: none; outline: none; }
  textarea::selection { background: oklch(0.6 0.15 var(--acc-h) / 0.28); color: transparent; }
  textarea::placeholder { color: var(--faint); }
  pre :global(.k) { color: var(--syn-kw); }
  pre :global(.s) { color: var(--syn-str); }
  pre :global(.n) { color: var(--syn-num); }
  pre :global(.f) { color: var(--syn-fn); }
  pre :global(.c) { color: var(--syn-com); }
  pre :global(.p) { color: var(--muted); }

  .ac {
    position: fixed; z-index: 35; width: 360px; overflow-y: auto; padding: 4px;
    background: var(--raised); border: 1px solid var(--line); border-radius: var(--r-lg); box-shadow: var(--shadow);
    font: 400 12.5px/18px var(--mono); overscroll-behavior: contain;
  }
  .it { display: flex; align-items: center; gap: 8px; height: 24px; padding: 0 6px; border-radius: var(--r-sm); color: var(--text); cursor: default; white-space: nowrap; }
  .it.on { background: var(--accent-soft); }
  .b {
    flex: none; width: 16px; height: 16px; display: grid; place-items: center; border-radius: var(--r-sm);
    font: 600 10px/1 var(--mono); background: var(--sunken); color: var(--muted);
  }
  .b.keyword { color: var(--syn-kw); }
  .b.function { color: var(--syn-fn); }
  .b.table, .b.view { color: var(--accent-text); }
  .b.column { color: var(--syn-str); }
  .b.alias { color: var(--syn-num); }
  .l { flex: none; max-width: 210px; overflow: hidden; text-overflow: ellipsis; }
  .l b { font-weight: 600; color: var(--accent-text); }
  .d { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; text-align: right; color: var(--faint); font-size: 11.5px; }
</style>
