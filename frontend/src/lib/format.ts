export const CC: Record<string, string> = {
  red: 'oklch(0.63 0.16 25)',
  amber: 'oklch(0.74 0.14 75)',
  green: 'oklch(0.66 0.14 150)',
  blue: 'oklch(0.63 0.14 250)',
  violet: 'oklch(0.62 0.15 300)',
  gray: 'oklch(0.62 0.01 265)',
}
export const connColor = (c?: string) => CC[c ?? ''] ?? CC.gray

export const HUES: Record<string, number> = { violet: 290, blue: 255, green: 150, teal: 195 }

export const ENGINE: Record<string, { label: string; short: string; port: number }> = {
  postgres: { label: 'PostgreSQL', short: 'PG', port: 5432 },
  mysql: { label: 'MySQL', short: 'MySQL', port: 3306 },
  sqlite: { label: 'SQLite', short: 'SQLite', port: 0 },
}

const nf = new Intl.NumberFormat('en-US')
export const fmtN = (n: number) => nf.format(n)
export const fmtK = (n: number) => (n >= 1e6 ? (n / 1e6).toFixed(1) + 'M' : n >= 1000 ? (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k' : String(n))
export const fmtMs = (ms: number) => (ms < 10 ? ms.toFixed(1) : String(Math.round(ms))) + ' ms'
export const plural = (n: number, w: string, p = w + 's') => `${fmtN(n)} ${n === 1 ? w : p}`

const pad = (n: number) => String(n).padStart(2, '0')
export const clock = (ts: number) => {
  const d = new Date(ts)
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

export function ago(ts: number, now = Date.now()) {
  const d = (now - ts) / 1000
  if (d < 45) return 'just now'
  if (d < 3600) return Math.round(d / 60) + 'm ago'
  if (d < 86400) return Math.round(d / 3600) + 'h ago'
  if (d < 172800) return 'yesterday'
  return Math.round(d / 86400) + 'd ago'
}

export const mmss = (ms: number) => {
  ms = Math.max(0, ms)
  return Math.floor(ms / 60000) + ':' + pad(Math.floor(ms / 1000) % 60)
}

export const initials = (name: string) => {
  const known: Record<string, string> = { 'Claude Desktop': 'CD', Cursor: 'Cu', 'Claude Code': 'CC', 'VS Code': 'VS' }
  if (known[name]) return known[name]
  const parts = name.split(/[\s-_]+/).filter(Boolean)
  return (parts.length > 1 ? parts[0][0] + parts[1][0] : name.slice(0, 2)).toUpperCase()
}

export const isToday = (ts: number) => new Date(ts).toDateString() === new Date().toDateString()

let seq = 0
export const uid = (p = 'x') => p + Date.now().toString(36) + (seq++).toString(36)

export const isMac = typeof navigator !== 'undefined' && /Mac/.test(navigator.platform)
export const MOD = isMac ? '⌘' : 'Ctrl '

/** Column width in pixels from the header and a sample of values. */
export function colWidth(name: string, type: string, values: Iterable<string | null>, charW: number) {
  let len = Math.max(name.length + 2, Math.min(type.length, 12))
  let i = 0
  for (const v of values) {
    if (v != null && v.length > len) len = v.length
    if (++i > 80) break
  }
  return Math.round(Math.min(360, Math.max(64, len * charW + 26)))
}

/** Fuzzy score of query against text. Returns -1 when not every char matches in order. */
export function fuzzy(query: string, text: string): number {
  if (!query) return 0
  const q = query.toLowerCase()
  const t = text.toLowerCase()
  if (t === q) return 1000
  if (t.startsWith(q)) return 800 - t.length
  const at = t.indexOf(q)
  if (at >= 0) return 600 - at * 4 - t.length
  let score = 0
  let ti = 0
  let prev = -2
  for (const ch of q) {
    const i = t.indexOf(ch, ti)
    if (i < 0) return -1
    score += i === prev + 1 ? 12 : 2
    if (i === 0 || /[_\s.-]/.test(t[i - 1])) score += 8
    prev = i
    ti = i + 1
  }
  return 200 + score - t.length
}

/** Scores text against every word of query, in any order. Returns -1 when a word does not match. */
export function matchWords(query: string, text: string): number {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean)
  if (!words.length) return 0
  let total = 0
  for (const w of words) {
    const s = fuzzy(w, text)
    if (s < 0) return -1
    total += s
  }
  return total / words.length
}

/** Picks the columns that fit in width and returns them with a grid template. */
export function fitColumns<T extends { name: string; type: string }>(cols: T[], rows: (string | null)[][], width: number, charW = 7.2) {
  const out: T[] = []
  const ws: number[] = []
  let used = 0
  for (let j = 0; j < cols.length; j++) {
    const w = Math.min(170, colWidth(cols[j].name, '', rows.map((r) => r[j]), charW) - 10)
    if (used + w > width && out.length >= 3) break
    out.push(cols[j])
    ws.push(w)
    used += w + 10
  }
  const template = ws.map((w, i) => (i === ws.length - 1 ? 'minmax(0,1fr)' : w + 'px')).join(' ')
  return { cols: out, template }
}
