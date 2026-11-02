// Context-aware SQL completion for the editor. complete() is a pure function
// of the text, the caret and the schema. The editor calls it on each key, so
// it makes one pass over the text and does no I/O.

import type { TableSummary } from './types'
import { fmtK, fuzzy } from './format'
import { KW, quoteIdent } from './sql'

export type Kind = 'keyword' | 'function' | 'table' | 'view' | 'column' | 'schema' | 'alias'

export interface Item {
  label: string
  insert: string
  kind: Kind
  detail: string
  /** The caret position inside insert after the insert. The default is the end. */
  caret?: number
  /** Open the list again after the insert, as after "schema.". */
  reopen?: boolean
}

export interface Completion {
  /** The range in the text that the insert replaces. */
  from: number
  to: number
  /** The part of the word before the caret. */
  prefix: string
  items: Item[]
}

export interface Col {
  name: string
  type: string
}

export interface Source {
  engine: string
  tables: TableSummary[]
  /**
   * The columns of a table, or undefined when they are not loaded yet. When
   * fetch is true, the source can start to load them for a later call.
   */
  cols: (t: TableSummary, fetch?: boolean) => Col[] | undefined
}

export interface Options {
  /** MySQL '#' comments. */
  hash?: boolean
  /** The user asked for the list with Ctrl+Space. Show it even for an empty word. */
  manual?: boolean
}

const MAX_ITEMS = 80

// ---- Tokens ----

type TT = 'id' | 'q' | 's' | 'n' | 'c' | 'o'
interface Tok {
  t: TT
  s: string
  i: number
  e: number
  /** For strings, quoted names and comments: the token has no end yet. */
  open: boolean
}

const TOK = /(--[^\n]*|\/\*[\s\S]*?(?:\*\/|$))|(#[^\n]*)|('(?:[^']|'')*'?)|("(?:[^"]|"")*"?|`(?:[^`]|``)*`?)|(\d+(?:\.\d+)?(?:e[+-]?\d+)?)|([A-Za-z_À-￿][\w$À-￿]*)|(\s+)|([\s\S])/gi
const CLOSED_STR = /^'(?:[^']|'')*'$/
const CLOSED_Q = /^(?:"(?:[^"]|"")*"|`(?:[^`]|``)*`)$/

/**
 * The tokens of the last text, from the start to the end of the statement at
 * the caret. Most edits change the text near the caret, so the next call keeps
 * the tokens before the edit and lexes only from there.
 */
let memo = { text: '', hash: false, toks: [] as Tok[], end: 0 }

/** Lexes text from the start to the end of the statement that holds the caret. */
function lex(text: string, caret: number, hash: boolean): Tok[] {
  let keep = 0
  if (memo.hash === hash && memo.toks.length) {
    const lim = Math.min(memo.end, text.length)
    let d = 0
    while (d < lim && text.charCodeAt(d) === memo.text.charCodeAt(d)) d++
    // The regex can look up to 4 characters past the end of a token, as in
    // "1e+5". Keep only the tokens that end at least that far before the edit.
    const toks = memo.toks
    let lo = 0
    let hi = toks.length
    while (lo < hi) {
      const mid = (lo + hi) >> 1
      if (toks[mid].e + 4 <= d) lo = mid + 1
      else hi = mid
    }
    keep = lo
  }
  const out = memo.toks
  out.length = keep
  TOK.lastIndex = keep ? out[keep - 1].e : 0
  let end = text.length
  let m: RegExpExecArray | null
  while ((m = TOK.exec(text))) {
    const s = m[0]
    const i = m.index
    const e = i + s.length
    if (m[7]) continue
    if (m[1]) out.push({ t: 'c', s, i, e, open: s[0] === '-' || s.length < 4 || !s.endsWith('*/') })
    else if (m[2]) {
      if (hash) out.push({ t: 'c', s, i, e, open: true })
      else {
        out.push({ t: 'o', s: '#', i, e: i + 1, open: false })
        TOK.lastIndex = i + 1
      }
    } else if (m[3]) out.push({ t: 's', s, i, e, open: !CLOSED_STR.test(s) })
    else if (m[4]) out.push({ t: 'q', s, i, e, open: !CLOSED_Q.test(s) })
    else if (m[5]) out.push({ t: 'n', s, i, e, open: false })
    else if (m[6]) out.push({ t: 'id', s, i, e, open: false })
    else {
      out.push({ t: 'o', s, i, e, open: false })
      if (s === ';' && i >= caret) {
        end = e
        break
      }
    }
  }
  memo = { text, hash, toks: out, end }
  return out
}

const lw = (tk: Tok | undefined) => (tk?.t === 'id' ? tk.s.toLowerCase() : '')
const isName = (tk: Tok | undefined): tk is Tok => tk?.t === 'id' || tk?.t === 'q'
const isOp = (tk: Tok | undefined, s: string) => tk?.t === 'o' && tk.s === s

function unq(tk: Tok): string {
  if (tk.t !== 'q') return tk.s
  const qc = tk.s[0]
  return tk.s.slice(1, tk.open ? undefined : -1).replaceAll(qc + qc, qc)
}

// ---- Words ----

const START = 'SELECT|WITH|INSERT INTO|UPDATE|DELETE FROM|EXPLAIN|VALUES|CREATE TABLE|CREATE INDEX|CREATE VIEW|ALTER TABLE|DROP TABLE|TRUNCATE|BEGIN|COMMIT|ROLLBACK|SET'
const START_BY: Record<string, string> = { postgres: 'SHOW|TABLE', mysql: 'SHOW TABLES|SHOW|DESCRIBE|USE', sqlite: 'PRAGMA' }
const CLAUSES =
  'FROM|WHERE|GROUP BY|ORDER BY|HAVING|LIMIT|OFFSET|JOIN|LEFT JOIN|RIGHT JOIN|INNER JOIN|FULL JOIN|CROSS JOIN|ON|USING|AS|AND|OR|NOT|IN|IS NULL|IS NOT NULL|LIKE|BETWEEN|EXISTS|DISTINCT|CASE|WHEN|THEN|ELSE|END|NULL|TRUE|FALSE|ASC|DESC|UNION|UNION ALL|INTERSECT|EXCEPT|SET|VALUES|DEFAULT|OVER|PARTITION BY|SELECT|INTO|CAST|CURRENT_DATE|CURRENT_TIMESTAMP|ALL|ANY'
const CLAUSES_BY: Record<string, string> = {
  postgres: 'ILIKE|RETURNING|NULLS FIRST|NULLS LAST|FILTER|LATERAL|ON CONFLICT|DO NOTHING|INTERVAL|WINDOW',
  mysql: 'ON DUPLICATE KEY UPDATE|INTERVAL|REGEXP|WINDOW',
  sqlite: 'RETURNING|NULLS FIRST|NULLS LAST|FILTER|ON CONFLICT|DO NOTHING|GLOB|WINDOW',
}
const FN = 'count sum avg min max coalesce nullif lower upper length trim replace round abs row_number rank dense_rank lag lead'
const FN_BY: Record<string, string> = {
  postgres:
    'now date_trunc extract to_char to_timestamp age string_agg array_agg json_agg jsonb_agg json_build_object jsonb_build_object generate_series greatest least concat substring split_part regexp_replace position left right first_value last_value ntile unnest array_length md5 gen_random_uuid',
  mysql:
    'now date date_format str_to_date date_add date_sub datediff timestampdiff unix_timestamp from_unixtime group_concat ifnull if concat concat_ws substring greatest least left right json_extract json_object json_arrayagg first_value last_value',
  sqlite: 'date datetime strftime julianday unixepoch group_concat ifnull iif instr substr printf json_extract json_object json_group_array total random',
}

interface Words {
  start: string[]
  clauses: string[]
  fns: string[]
}
const wordCache = new Map<string, Words>()
function words(engine: string): Words {
  let w = wordCache.get(engine)
  if (!w) {
    const uniq = (s: string, sep: string | RegExp) => [...new Set(s.split(sep).filter(Boolean))]
    w = {
      start: uniq(START + '|' + (START_BY[engine] ?? ''), '|'),
      clauses: uniq(CLAUSES + '|' + (CLAUSES_BY[engine] ?? ''), '|'),
      fns: uniq(FN + ' ' + (FN_BY[engine] ?? ''), ' '),
    }
    wordCache.set(engine, w)
  }
  return w
}

/** Words that end an expression. After them the user types a keyword, not a column. */
const END_EXPR = new Set(['null', 'true', 'false', 'end', 'asc', 'desc', 'first', 'last'])
/** Words after which a table name follows. */
const TABLE_AFTER = new Set(['from', 'join', 'update', 'into', 'table', 'truncate', 'describe'])
/** Words that start a list of table references. */
const REF_START = new Set(['from', 'join', 'update', 'into', 'table', 'using'])
/** Words that the editor must quote when it inserts them as a name. */
const RESERVED = new Set(
  `all and any array as asc between both by case cast check collate column constraint create cross current_date current_time
  current_timestamp current_user default desc distinct do else end except false fetch for foreign from full grant group having
  in inner intersect into is join key lateral leading left like limit natural not null offset on only or order outer primary
  references right select session_user some table then to trailing true union unique user using values when where window with`.split(/\s+/),
)
const DEFAULT_SCHEMA: Record<string, string> = { postgres: 'public', sqlite: 'main' }

/** Quotes a name only when the engine needs it. */
export function ident(engine: string, name: string): string {
  if (RESERVED.has(name.toLowerCase())) return engine === 'mysql' ? '`' + name + '`' : '"' + name + '"'
  // MySQL and SQLite keep the case of a bare name. PostgreSQL folds it to lower case.
  if (engine !== 'postgres' && /^[A-Za-z_][A-Za-z0-9_]*$/.test(name)) return name
  return quoteIdent(engine, name)
}

function tableInsert(engine: string, t: TableSummary): string {
  if (engine === 'postgres' && t.schema !== 'public') return ident(engine, t.schema) + '.' + ident(engine, t.name)
  return ident(engine, t.name)
}

// ---- Matching ----

/**
 * Scores text against what the user typed. -1 means no match. A loose match
 * also accepts a match inside the text, as "id" in "customer_id".
 */
function score(prefix: string, text: string, loose: boolean): number {
  if (!prefix) return 0
  const p = prefix.toLowerCase()
  const t = text.toLowerCase()
  if (t === p) return 1000
  if (t.startsWith(p)) return 900 - t.length
  if (!loose) return -1
  const at = t.indexOf(p)
  if (at > 0) return (/[_.\s]/.test(t[at - 1]) ? 700 : 500) - at - t.length
  if (p.length < 3) return -1
  const f = fuzzy(p, t)
  return f < 0 ? -1 : Math.min(f, 300)
}

/** Keywords follow the case of what the user typed. Upper case is the default. */
function kwCase(prefix: string, word: string) {
  return /[a-z]/.test(prefix) && prefix === prefix.toLowerCase() ? word.toLowerCase() : word.toUpperCase()
}

/** Functions are lower case unless the user types upper case. */
function fnCase(prefix: string, word: string) {
  return /[A-Z]/.test(prefix) && prefix === prefix.toUpperCase() ? word.toUpperCase() : word.toLowerCase()
}

// ---- Statement analysis ----

interface Ref {
  parts: string[]
  alias: string
  table: TableSummary | null
}

function skipParens(st: Tok[], p: number): number {
  let d = 0
  for (; p < st.length; p++) {
    if (isOp(st[p], '(')) d++
    else if (isOp(st[p], ')') && --d === 0) return p + 1
  }
  return p
}

function readCTEs(st: Tok[], p: number, out: string[]) {
  if (lw(st[p]) === 'recursive') p++
  while (isName(st[p])) {
    out.push(unq(st[p]).toLowerCase())
    p++
    if (isOp(st[p], '(')) p = skipParens(st, p)
    if (lw(st[p]) !== 'as') return
    p++
    if (lw(st[p]) === 'not') p++
    if (lw(st[p]) === 'materialized') p++
    if (!isOp(st[p], '(')) return
    p = skipParens(st, p)
    if (!isOp(st[p], ',')) return
    p++
  }
}

/** Finds the tables, aliases and CTE names in a statement. */
function scanRefs(st: Tok[], src: Source): { refs: Ref[]; ctes: string[] } {
  const refs: Ref[] = []
  const ctes: string[] = []
  for (let j = 0; j < st.length; j++) {
    const w = lw(st[j])
    if (w === 'with') readCTEs(st, j + 1, ctes)
    if (!REF_START.has(w)) continue
    let p = j + 1
    for (;;) {
      while (lw(st[p]) === 'only' || lw(st[p]) === 'lateral') p++
      if (!isName(st[p])) break
      const parts = [unq(st[p++])]
      while (isOp(st[p], '.') && isName(st[p + 1])) {
        parts.push(unq(st[p + 1]))
        p += 2
      }
      if (lw(st[p]) === 'as') p++
      let alias = ''
      const a = st[p]
      if (a && (a.t === 'q' || (a.t === 'id' && !KW.has(a.s.toLowerCase())))) {
        alias = unq(a)
        p++
      }
      const cte = parts.length === 1 && ctes.includes(parts[0].toLowerCase())
      refs.push({ parts, alias, table: cte ? null : resolve(src, parts) })
      if (w === 'from' && isOp(st[p], ',')) {
        p++
        continue
      }
      break
    }
  }
  return { refs, ctes }
}

/** Finds a table by name, with or without a schema. */
function resolve(src: Source, parts: string[]): TableSummary | null {
  const exact = parts[parts.length - 1]
  const name = exact.toLowerCase()
  const schema = parts.length > 1 ? parts[parts.length - 2].toLowerCase() : ''
  const def = DEFAULT_SCHEMA[src.engine]
  let best: TableSummary | null = null
  let bs = -1
  for (const t of src.tables) {
    if (t.name.toLowerCase() !== name || (schema && t.schema.toLowerCase() !== schema)) continue
    const sc = (t.name === exact ? 2 : 0) + (t.schema === def ? 1 : 0)
    if (sc > bs) {
      best = t
      bs = sc
    }
  }
  return best
}

/** Walks back at the same paren depth to the clause word or the open paren. */
function clauseOf(before: Tok[], p: number): { word: string; at: number } {
  let depth = 0
  for (let j = p - 1; j >= 0; j--) {
    const tk = before[j]
    if (isOp(tk, ')')) depth++
    else if (isOp(tk, '(')) {
      if (depth === 0) return { word: '(', at: j }
      depth--
    } else if (depth === 0 && tk.t === 'id') {
      const w = tk.s.toLowerCase()
      if (w === 'select' || w === 'from' || w === 'where' || w === 'set' || w === 'by' || w === 'on' || w === 'having' || w === 'values') return { word: w, at: j }
    }
  }
  return { word: '', at: -1 }
}

const CASE_WORDS = new Set(['WHEN', 'THEN', 'ELSE', 'END'])

/** Reports whether the position is inside an open CASE at the same paren depth. */
function inCase(before: Tok[], p: number): boolean {
  let depth = 0
  let ends = 0
  for (let j = p - 1; j >= 0; j--) {
    const tk = before[j]
    if (isOp(tk, ')')) depth++
    else if (isOp(tk, '(')) {
      if (depth === 0) return false
      depth--
    } else if (depth === 0) {
      const w = lw(tk)
      if (w === 'end') ends++
      else if (w === 'case') {
        if (ends === 0) return true
        ends--
      }
    }
  }
  return false
}

/** For INSERT INTO t ( ... returns t when the paren at "at" opens the column list. */
function insertTarget(before: Tok[], at: number, src: Source): TableSummary | null {
  let p = at - 1
  if (!isName(before[p])) return null
  const parts = [unq(before[p])]
  while (isOp(before[p - 1], '.') && isName(before[p - 2])) {
    parts.unshift(unq(before[p - 2]))
    p -= 2
  }
  return lw(before[p - 1]) === 'into' ? resolve(src, parts) : null
}

function endsExpr(tk: Tok): boolean {
  if (tk.t === 'q' || tk.t === 'n' || tk.t === 's') return true
  if (tk.t === 'id') {
    const w = tk.s.toLowerCase()
    return !KW.has(w) || END_EXPR.has(w)
  }
  return tk.s === ')' || tk.s === '*'
}

// ---- Completion ----

interface Ranked {
  make: () => Item
  s: number
  o: number
}

/**
 * Returns the completions at the caret, or null when the editor must not show
 * the list, as inside a string or a comment.
 */
export function complete(sql: string, caret: number, src: Source, opts: Options = {}): Completion | null {
  const text = sql
  const c = caret
  const toks = lex(text, c, !!opts.hash)
  const manual = !!opts.manual

  // The token that contains the caret or ends at it: the last token that starts before the caret.
  let lo = 0
  let hi = toks.length
  while (lo < hi) {
    const mid = (lo + hi) >> 1
    if (toks[mid].i < c) lo = mid + 1
    else hi = mid
  }
  const at = lo - 1
  const cur = at >= 0 && toks[at].e >= c ? toks[at] : null
  let from = c
  let to = c
  let prefix = ''
  if (cur) {
    if (cur.t === 'c' || cur.t === 's' || cur.t === 'q') {
      if (c < cur.e || cur.open) return null
    } else if (cur.t === 'n') return null
    else if (cur.t === 'id') {
      from = cur.i
      to = cur.e
      prefix = text.slice(cur.i, c)
    }
  }

  // The statement around the caret, without comments.
  let k = at + 1
  while (k > 0 && toks[k - 1].e > from) k--
  let s0 = 0
  for (let j = k - 1; j >= 0; j--) {
    if (isOp(toks[j], ';')) {
      s0 = j + 1
      break
    }
  }
  let s1 = toks.length
  for (let j = k; j < toks.length; j++) {
    if (isOp(toks[j], ';')) {
      s1 = j
      break
    }
  }
  const stmt = toks.slice(s0, s1).filter((tk) => tk.t !== 'c')
  let kk = 0
  while (kk < stmt.length && stmt[kk].e <= from) kk++
  const word = cur?.t === 'id'
  const before = stmt.slice(0, kk)
  const others = word ? stmt.slice(0, kk).concat(stmt.slice(kk + 1)) : stmt

  // A qualifier before the word: alias.col, table.col, schema.table.
  let p = kk
  const qual: string[] = []
  while (qual.length < 2 && isOp(before[p - 1], '.') && isName(before[p - 2])) {
    qual.unshift(unq(before[p - 2]))
    p -= 2
  }
  const prev = before[p - 1]
  const pw = lw(prev)

  if (!manual && !qual.length && !prefix) return null

  const { engine } = src
  const W = words(engine)
  const ranked: Ranked[] = []
  // make builds the item only when it makes the list, which keeps a large schema cheap.
  const add = (match: string, weight: number, loose: boolean, make: () => Item) => {
    const m = score(prefix, match, loose)
    if (m >= 0) ranked.push({ make, s: m + weight, o: ranked.length })
  }
  const addKeywords = (list: string[]) => {
    for (const w of list) {
      add(w, 0, false, () => {
        const t = kwCase(prefix, w)
        return { label: t, insert: t, kind: 'keyword', detail: '' }
      })
    }
  }
  const addColumns = (t: TableSummary, weight: number, detail = '', fetch = true) => {
    for (const col of src.cols(t, fetch) ?? []) {
      add(col.name, weight, true, () => ({ label: col.name, insert: ident(engine, col.name), kind: 'column', detail: detail + col.type }))
    }
  }
  const addTables = (list: TableSummary[], insert: (t: TableSummary) => string) => {
    const def = DEFAULT_SCHEMA[engine]
    for (const t of list) {
      add(t.name, t.kind === 'view' ? 25 : 30, true, () => {
        const where = engine === 'postgres' && t.schema !== def ? t.schema + ' · ' : ''
        const size = t.kind === 'view' ? 'view' : t.rows ? `${t.estimate ? '~' : ''}${fmtK(t.rows)} rows` : 'table'
        return { label: t.name, insert: insert(t), kind: t.kind === 'view' ? 'view' : 'table', detail: where + size }
      })
    }
  }

  if (qual.length) {
    // alias.| or table.| lists columns. schema.| lists tables.
    const q = qual[qual.length - 1].toLowerCase()
    const { refs, ctes } = scanRefs(others, src)
    let t: TableSummary | null = null
    if (qual.length === 1) {
      const r = refs.find((r) => r.alias.toLowerCase() === q) ?? refs.find((r) => !r.alias && r.parts[r.parts.length - 1].toLowerCase() === q)
      if (r) t = r.table
      else if (ctes.includes(q)) return null
    }
    t ??= resolve(src, qual)
    if (t) addColumns(t, 0)
    else if (qual.length === 1) {
      const inSchema = src.tables.filter((x) => x.schema.toLowerCase() === q)
      if (!inSchema.length) return null
      addTables(inSchema, (x) => ident(engine, x.name))
    } else return null
  } else if (p === 0 || pw === 'explain' || (pw === 'analyze' && lw(before[p - 2]) === 'explain')) {
    addKeywords(W.start)
  } else if (
    pw === 'as' ||
    pw === 'with' ||
    pw === 'recursive' ||
    pw === 'limit' ||
    pw === 'offset' ||
    (isOp(prev, ':') && isOp(before[p - 2], ':')) ||
    ((pw === 'table' || pw === 'index' || pw === 'view') && lw(before[p - 2]) === 'create') ||
    (pw === 'exists' && lw(before[p - 2]) === 'not')
  ) {
    // The user types a new name or a value here.
    return null
  } else if (
    TABLE_AFTER.has(pw) ||
    (pw === 'desc' && p === 1) ||
    (pw === 'exists' && lw(before[p - 2]) === 'if') ||
    (pw === 'only' && TABLE_AFTER.has(lw(before[p - 2]))) ||
    (isOp(prev, ',') && clauseOf(before, p).word === 'from')
  ) {
    const { ctes } = scanRefs(others, src)
    for (const name of ctes) add(name, 35, true, () => ({ label: name, insert: name, kind: 'table', detail: 'cte' }))
    addTables(src.tables, (t) => tableInsert(engine, t))
    if (engine === 'postgres') {
      const schemas = [...new Set(src.tables.map((t) => t.schema))].filter((s) => s !== 'public')
      for (const s of schemas) add(s, 10, true, () => ({ label: s, insert: ident(engine, s) + '.', kind: 'schema', detail: 'schema', reopen: true }))
    }
  } else {
    const cl = clauseOf(before, p)
    const target = cl.word === '(' ? insertTarget(before, cl.at, src) : null
    if (target) {
      addColumns(target, 0)
    } else if (endsExpr(prev)) {
      // After a value or a name, the next word is a keyword: AS, FROM, AND, ORDER BY.
      if (!manual && prefix.length < 2) return null
      // WHEN, THEN, ELSE and END belong to CASE, so WH gives WHERE outside one.
      addKeywords(inCase(before, p) ? W.clauses : W.clauses.filter((w) => !CASE_WORDS.has(w)))
    } else {
      const { refs } = scanRefs(others, src)
      const known = refs.filter((r) => r.table && src.cols(r.table))
      const counts = new Map<string, number>()
      for (const r of known) for (const col of src.cols(r.table!)!) counts.set(col.name, (counts.get(col.name) ?? 0) + 1)
      for (const r of known) {
        const t = r.table!
        const q = r.alias || t.name
        const qi = r.alias ? ident(engine, r.alias) : ident(engine, t.name)
        for (const col of src.cols(t)!) {
          // Qualify a name that two tables share, so the query is not ambiguous.
          const amb = known.length > 1 && counts.get(col.name)! > 1
          add(col.name, 40, true, () => ({
            label: amb ? q + '.' + col.name : col.name,
            insert: (amb ? qi + '.' : '') + ident(engine, col.name),
            kind: 'column',
            detail: (known.length > 1 ? q + ' · ' : '') + col.type,
          }))
        }
      }
      for (const r of refs) {
        if (r.alias) add(r.alias, 35, false, () => ({ label: r.alias, insert: ident(engine, r.alias), kind: 'alias', detail: r.parts.join('.') }))
      }
      if (!known.length && prefix) {
        // No FROM yet. Offer the columns of every table, below the other words.
        for (const t of src.tables) addColumns(t, -60, t.name + ' · ', false)
      }
      const paren = text[to] === '('
      for (const f of W.fns) {
        add(f, 15, false, () => {
          const t = fnCase(prefix, f)
          return { label: t, insert: paren ? t : t + '()', kind: 'function', detail: '', caret: t.length + (paren ? 0 : 1) }
        })
      }
      addKeywords(W.clauses)
    }
  }

  if (!ranked.length) return null
  ranked.sort((a, b) => b.s - a.s || a.o - b.o)
  const items = ranked.slice(0, MAX_ITEMS).map((r) => r.make())
  // Do not open the list for a word that is already complete.
  if (!manual && items.length === 1 && items[0].insert.toLowerCase() === prefix.toLowerCase()) return null
  return { from, to, prefix, items }
}
