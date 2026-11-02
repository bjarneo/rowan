// SQL highlighting and formatting for the editor.

/** Words the highlighter colors as keywords. */
export const KW = new Set(
  `select from where and or not null is in as on join left right inner outer full cross natural using group by order
  limit offset having insert into values update set delete create table alter drop truncate primary key references
  default unique index distinct asc desc case when then else end explain analyze with recursive returning union all
  like ilike between exists true false begin commit rollback over partition window filter lateral cast interval
  view materialized if replace ignore conflict do nothing show describe pragma grant revoke to nulls first last
  fetch next rows only for share nowait skip locked any some escape intersect except start transaction read write
  constraint foreign check generated identity always serial bigserial engine charset auto_increment unsigned
  temporary temp cascade restrict add column rename schema database use vacuum copy trigger function procedure
  returns language begin declare`.split(/\s+/),
)

const TOKEN = /(--[^\n]*|#[^\n]*|\/\*[\s\S]*?(?:\*\/|$))|('(?:[^']|'')*'?)|("(?:[^"]|"")*"?|`[^`]*`?)|(\b\d+(?:\.\d+)?\b)|([A-Za-z_][\w$]*)|(\s+)|([\s\S])/g

const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')

/** Returns HTML with syntax colors. hashComments enables MySQL '#' comments. */
export function highlight(src: string, hashComments = false): string {
  let out = ''
  let m: RegExpExecArray | null
  TOKEN.lastIndex = 0
  while ((m = TOKEN.exec(src))) {
    const t = m[0]
    if (m[1]) {
      if (t[0] === '#' && !hashComments) {
        out += esc(t[0])
        TOKEN.lastIndex = m.index + 1
        continue
      }
      out += `<span class="c">${esc(t)}</span>`
    } else if (m[2]) out += `<span class="s">${esc(t)}</span>`
    else if (m[3]) out += esc(t)
    else if (m[4]) out += `<span class="n">${t}</span>`
    else if (m[5]) {
      const w = t.toLowerCase()
      if (KW.has(w)) out += `<span class="k">${t}</span>`
      else if (src[TOKEN.lastIndex] === '(') out += `<span class="f">${t}</span>`
      else out += t
    } else if (m[6]) out += t
    else out += `<span class="p">${esc(t)}</span>`
  }
  return out
}

const BREAK_BEFORE = new Set(['select', 'from', 'where', 'group', 'order', 'having', 'limit', 'offset', 'union', 'values', 'set', 'returning', 'join', 'left', 'right', 'inner', 'full', 'cross', 'on', 'insert', 'update', 'delete', 'with'])
const JOIN_WORDS = new Set(['left', 'right', 'inner', 'full', 'cross', 'outer'])

/** A small formatter: upper-case keywords, one clause per line, AND/OR indented. */
export function formatSQL(src: string): string {
  const stmts: string[] = []
  let cur = ''
  let depth = 0
  let prevWord = ''
  let m: RegExpExecArray | null
  TOKEN.lastIndex = 0
  const push = (s: string) => {
    cur += s
  }
  while ((m = TOKEN.exec(src))) {
    const t = m[0]
    if (m[6]) {
      if (!cur.endsWith(' ') && !cur.endsWith('\n') && !cur.endsWith('(') && cur) push(' ')
      continue
    }
    if (m[1]) {
      push((cur && !cur.endsWith('\n') ? '\n' : '') + t + '\n')
      continue
    }
    if (m[5]) {
      const w = t.toLowerCase()
      const kw = KW.has(w)
      const word = kw ? t.toUpperCase() : t
      if (depth === 0 && BREAK_BEFORE.has(w) && cur.trim() && !(w === 'join' && JOIN_WORDS.has(prevWord)) && !(prevWord === 'union') && !(w === 'on' && false)) {
        cur = cur.trimEnd() + '\n'
      } else if (depth === 0 && (w === 'and' || w === 'or') && cur.trim()) {
        cur = cur.trimEnd() + '\n  '
      }
      push(word)
      prevWord = w
      continue
    }
    if (t === '(') depth++
    if (t === ')') depth = Math.max(0, depth - 1)
    if (t === ';') {
      stmts.push(cur.trim() + ';')
      cur = ''
      prevWord = ''
      continue
    }
    if (t === ',') {
      cur = cur.trimEnd() + ','
      push(depth === 0 ? '\n  ' : ' ')
      continue
    }
    if (t === ')' || t === '.') cur = cur.trimEnd()
    push(t)
    if (t === '.' || t === '(') continue
  }
  if (cur.trim()) stmts.push(cur.trim())
  return stmts
    .map((s) => s.replace(/\n(SELECT|FROM|WHERE|GROUP|ORDER|HAVING|LIMIT|SET|VALUES|RETURNING)\n  /g, '\n$1 ').replace(/^(SELECT)\n  /, '$1 '))
    .join('\n\n') + '\n'
}

const WRITE = /^\s*(?:\(\s*)*(insert|update|delete|merge|create|alter|drop|truncate|grant|revoke|replace|copy|call|do|vacuum|attach)\b/i
export const looksLikeWrite = (sql: string) => WRITE.test(stripComments(sql))
export const looksLikeSQL = (s: string) => /^\s*(select|with|show|explain|describe|desc|pragma|values|table|insert|update|delete)\b/i.test(s)

export function stripComments(sql: string) {
  return sql.replace(/--[^\n]*/g, '').replace(/\/\*[\s\S]*?\*\//g, '')
}

/** Quote a value as a SQL literal. */
export function sqlLit(v: string | null, numeric: boolean) {
  if (v == null) return 'NULL'
  if (numeric && /^-?\d+(\.\d+)?$/.test(v)) return v
  return "'" + v.replace(/'/g, "''") + "'"
}

export function quoteIdent(engine: string, name: string) {
  if (/^[a-z_][a-z0-9_]*$/.test(name)) return name
  return engine === 'mysql' ? '`' + name.replace(/`/g, '``') + '`' : '"' + name.replace(/"/g, '""') + '"'
}

export function tableRef(engine: string, schema: string, table: string) {
  if (engine === 'postgres' && schema && schema !== 'public') return quoteIdent(engine, schema) + '.' + quoteIdent(engine, table)
  return quoteIdent(engine, table)
}
