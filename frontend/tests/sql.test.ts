// Tests for the range highlighter of the editor. Run them with `npm test`.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { Highlighter, highlight } from '../src/lib/sql'

const text = (html: string) => html.replace(/<[^>]+>/g, '').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&amp;/g, '&')

/** Builds a script with block comments and strings that cross line starts. */
function script(n: number) {
  const out: string[] = []
  for (let i = 0; i < n; i++) {
    if (i % 97 === 5) out.push(`/* a comment over\n   two lines ${i} */ SELECT 1;`)
    else if (i % 89 === 7) out.push(`SELECT 'text over\nlines', "a""b" FROM t # mysql ${i};`)
    else out.push(`UPDATE orders SET note = 'row ${i}' WHERE id = ${i} AND total_cents > 100 -- x < y & z`)
  }
  return out.join('\n')
}

function lineStarts(s: string) {
  const out = [0]
  for (let i = s.indexOf('\n'); i >= 0; i = s.indexOf('\n', i + 1)) out.push(i + 1)
  return out
}

test('a full range matches highlight', () => {
  const s = script(400)
  for (const hash of [false, true]) {
    assert.equal(new Highlighter().html(s, 0, s.length, hash), highlight(s, hash))
  }
})

test('ranges hold the text of their lines', () => {
  const s = script(3000)
  const h = new Highlighter()
  const starts = lineStarts(s)
  for (const [a, b] of [[0, 40], [1500, 1600], [2950, starts.length], [700, 760], [10, 12]]) {
    const from = starts[a]
    const to = b < starts.length ? starts[b] : s.length
    assert.equal(text(h.html(s, from, to)), s.slice(from, to), `lines ${a} to ${b}`)
  }
})

test('checkpoints survive edits after them and reset before them', () => {
  let s = script(3000)
  const h = new Highlighter()
  const starts = lineStarts(s)
  const from = starts[2500]
  const to = starts[2550]
  h.html(s, from, to)
  // An edit near the end keeps the checkpoints before it.
  s = s.slice(0, starts[2600]) + "'open string " + s.slice(starts[2600])
  assert.equal(h.html(s, from, to), new Highlighter().html(s, from, to))
  // An edit before the range opens a comment that covers the range.
  s = s.slice(0, starts[2499]) + '/*' + s.slice(starts[2499], starts[2560]) + '*/' + s.slice(starts[2560])
  const f2 = from + 2
  const t2 = to + 2
  assert.equal(h.html(s, f2, t2), new Highlighter().html(s, f2, t2))
  assert.match(h.html(s, f2, t2), /^<span class="c">[^<]*<\/span>$/)
})
