// Tests for SQL completion. Run them with `npm test`.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { complete, type Col, type Source } from '../src/lib/complete'
import type { TableSummary } from '../src/lib/types'

const COLS: Record<string, string[]> = {
  'public.customers': ['id', 'name', 'email', 'country', 'plan', 'mrr_cents', 'created_at'],
  'public.orders': ['id', 'customer_id', 'status', 'total_cents', 'currency', 'placed_at', 'shipped_at'],
  'public.order_items': ['id', 'order_id', 'product_id', 'quantity', 'unit_cents'],
  'public.v_monthly_revenue': ['month', 'revenue_cents'],
  'billing.invoices': ['id', 'order_id', 'number', 'issued_at', 'paid_at'],
  'public.Audit Log': ['Event Type', 'user'],
}

function source(engine = 'postgres'): Source {
  const tables: TableSummary[] = Object.keys(COLS).map((k) => {
    const [schema, name] = k.split('.')
    return { schema, name, kind: name.startsWith('v_') ? 'view' : 'table', rows: 100, estimate: false }
  })
  const cols = (t: TableSummary): Col[] => COLS[`${t.schema}.${t.name}`].map((name) => ({ name, type: 'text' }))
  return { engine, tables, cols }
}

/** Runs complete() with the caret at the "|" in sql. */
function at(sql: string, opts: { engine?: string; manual?: boolean; hash?: boolean } = {}) {
  const caret = sql.indexOf('|')
  return complete(sql.replace('|', ''), caret, source(opts.engine), opts)
}
const labels = (sql: string, opts = {}) => at(sql, opts)?.items.map((i) => i.label) ?? null
const top = (sql: string, opts = {}) => at(sql, opts)?.items[0] ?? null

test('tables after FROM and JOIN', () => {
  assert.equal(top('SELECT * FROM ord|')?.insert, 'orders')
  assert.equal(top('SELECT * FROM orders o JOIN cus|')?.insert, 'customers')
  assert.equal(top('select * from orders, cust|')?.insert, 'customers')
  assert.equal(top('DELETE FROM inv|')?.insert, 'billing.invoices')
  assert.equal(top('SELECT * FROM aud|')?.insert, '"Audit Log"')
})

test('the replaced range covers the whole word', () => {
  const c = at('SELECT * FROM ord|ers')!
  assert.deepEqual([c.from, c.to, c.prefix], [14, 20, 'ord'])
})

test('columns of the tables in the statement', () => {
  assert.equal(top('SELECT * FROM orders WHERE sta|')?.label, 'status')
  assert.equal(top('UPDATE orders SET sta|')?.label, 'status')
  assert.equal(top('SELECT * FROM orders o ORDER BY pla|')?.label, 'placed_at')
  // Columns of a table that is not in the statement do not show.
  assert.ok(!labels('SELECT * FROM orders WHERE |', { manual: true })!.includes('email'))
})

test('columns after alias, table and schema qualifiers', () => {
  assert.deepEqual(labels('SELECT o.| FROM orders o'), COLS['public.orders'])
  assert.equal(top('SELECT * FROM orders o JOIN customers c ON c.id = o.cus|')?.label, 'customer_id')
  assert.equal(top('SELECT customers.em| FROM customers')?.label, 'email')
  assert.equal(top('SELECT * FROM billing.inv|')?.insert, 'invoices')
  assert.equal(top('SELECT i.pa| FROM billing.invoices i')?.label, 'paid_at')
})

test('a name in two tables is qualified', () => {
  const items = at('SELECT id| FROM orders o JOIN customers c ON c.id = o.customer_id')!.items
  const ids = items.filter((i) => i.kind === 'column' && i.label.endsWith('.id'))
  assert.deepEqual(ids.map((i) => i.insert), ['o.id', 'c.id'])
})

test('INSERT column list shows only the target columns', () => {
  const items = at('INSERT INTO orders (cu|')!.items
  assert.deepEqual(items.map((i) => i.label).sort(), ['currency', 'customer_id'])
})

test('CTE names complete as tables', () => {
  assert.equal(top('WITH recent AS (SELECT * FROM orders) SELECT * FROM rec|')?.label, 'recent')
})

test('subquery columns', () => {
  assert.equal(top('SELECT * FROM orders WHERE customer_id IN (SELECT id FROM customers WHERE pl|')?.label, 'plan')
})

test('only the current statement counts', () => {
  const l = labels('SELECT * FROM orders; SELECT * FROM customers WHERE |', { manual: true })!
  assert.ok(l.includes('email') && !l.includes('status'))
})

test('keywords follow the case the user types', () => {
  assert.equal(top('sel|')?.insert, 'select')
  assert.equal(top('SEL|')?.insert, 'SELECT')
  assert.equal(top('SELECT * FROM orders WHERE id = 1 ORD|')?.insert, 'ORDER BY')
  assert.equal(top('SELECT * FROM orders ORDER BY placed_at de|')?.insert, 'desc')
  assert.equal(top('SELECT * FROM orders WHERE status = \'x\' AN|')?.insert, 'AND')
})

test('CASE words only inside CASE', () => {
  assert.equal(top('SELECT * FROM orders o JOIN customers c ON c.id = o.customer_id\nWH|')?.insert, 'WHERE')
  assert.equal(top('SELECT CASE WHEN total_cents > 100 TH|')?.insert, 'THEN')
  assert.equal(top('SELECT CASE WHEN total_cents > 100 THEN 1 WH|')?.insert, 'WHEN')
  assert.equal(top('SELECT CASE WHEN a THEN 1 END FROM orders WH|')?.insert, 'WHERE')
  assert.equal(top('SELECT (CASE WHEN a THEN 1 EL|')?.insert, 'ELSE')
})

test('functions insert parens and put the caret inside', () => {
  const f = top('SELECT cou|')!
  assert.deepEqual([f.insert, f.caret], ['count()', 6])
  assert.equal(top('SELECT cou|(*)')?.insert, 'count')
})

test('no list inside strings, comments, numbers and new names', () => {
  assert.equal(at("SELECT 'ord|'"), null)
  assert.equal(at('-- from ord|'), null)
  assert.equal(at('/* from ord|'), null)
  assert.equal(at('# from ord|', { engine: 'mysql', hash: true }), null)
  assert.equal(at('SELECT 12|'), null)
  assert.equal(at('SELECT count(*) AS to|'), null)
  assert.equal(at('SELECT x::te|'), null)
  assert.equal(at('CREATE TABLE ord|'), null)
})

test('no list for a short word after a table name, so an alias stays an alias', () => {
  assert.equal(at('SELECT * FROM orders o|'), null)
  assert.equal(top('SELECT * FROM orders o|', { manual: true })?.kind, 'keyword')
})

test('a complete word does not open the list', () => {
  assert.equal(at('SELECT|'), null)
})

test('names that need quotes', () => {
  assert.equal(top('SELECT * FROM "Audit Log" a WHERE a.Ev|')?.insert, '"Event Type"')
  assert.equal(top('SELECT * FROM "Audit Log" a WHERE a.us|')?.insert, '"user"')
  assert.equal(top('SELECT * FROM `Audit Log` a WHERE a.us|', { engine: 'mysql' })?.insert, '`user`')
})

test('schemas complete and open the list again', () => {
  const s = at('SELECT * FROM bil|')!.items.find((i) => i.kind === 'schema')!
  assert.deepEqual([s.insert, s.reopen], ['billing.', true])
})

test('statement start shows statement keywords', () => {
  const l = labels('|', { manual: true })!
  assert.equal(l[0], 'SELECT')
  assert.ok(labels('|', { manual: true, engine: 'sqlite' })!.includes('PRAGMA'))
})

test('speed on a large schema and a long script', () => {
  const tables: TableSummary[] = []
  const cols = new Map<TableSummary, Col[]>()
  for (let i = 0; i < 2000; i++) {
    const t: TableSummary = { schema: 'public', name: `table_${i}`, kind: 'table', rows: i, estimate: true }
    tables.push(t)
    cols.set(t, Array.from({ length: 10 }, (_, j) => ({ name: `column_${j}_${i}`, type: 'int' })))
  }
  const src: Source = { engine: 'postgres', tables, cols: (t) => cols.get(t) }
  const script = 'SELECT a, b, c FROM table_1 t JOIN table_2 u ON u.id = t.id WHERE t.x = 1;\n'.repeat(2000)
  const cases = [script + 'SELECT * FROM tab', script + 'SELECT colu', script + 'SELECT * FROM table_1 t WHERE t.']
  const t0 = performance.now()
  let n = 0
  for (let r = 0; r < 20; r++) {
    for (const s of cases) {
      assert.ok(complete(s, s.length, src)?.items.length)
      n++
    }
  }
  const per = (performance.now() - t0) / n
  console.log(`complete(): ${per.toFixed(2)} ms per call, ${(script.length / 1024).toFixed(0)} KB script, 20000 columns`)
  assert.ok(per < 16, `too slow: ${per} ms`)
})
