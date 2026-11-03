// Measures how fast Rowan responds on a large database. It starts the dev
// server with the demo database, times the backend calls, then runs the UI
// steps in suite.js and prints one line per step.
//
//   scripts/demo-pg/start.sh
//   ROWAN_DEMO_PG_PORT=55433 node scripts/perf/bench.mjs
//   ROWAN_DEMO_PG_PORT=55433 ENGINE=webkit node scripts/perf/bench.mjs
//
// ENGINE=webkit runs the steps in WebKitGTK, the engine of the Linux app. It
// needs PyGObject and WebKitGTK 4.1. CPU_SLOWDOWN=4 runs Chromium on a CPU
// that is 4 times slower. NO_BUILD=1 skips the frontend build.
import { readFileSync } from 'node:fs'
import { spawnSync } from 'node:child_process'
import { join } from 'node:path'
import { startRowan, browser, addDemoPG, setLook } from '../promo/lib.mjs'

const PG = process.env.ROWAN_DEMO_PG_PORT
if (!PG) {
  console.error('Set ROWAN_DEMO_PG_PORT to the port of the demo database.')
  process.exit(2)
}
const ENGINE = process.env.ENGINE ?? 'chromium'
const SLOW = Number(process.env.CPU_SLOWDOWN ?? 1)
const suite = join(import.meta.dirname, 'suite.js')
const rows = []

const rowan = await startRowan({ port: 5197, build: !process.env.NO_BUILD })
try {
  const cn = await addDemoPG(rowan, PG)
  await setLook(rowan, { theme: 'dark', layout: 'workspace' })

  // ---- Backend calls, without the UI ----
  const call = async (label, method, ...args) => {
    const t0 = performance.now()
    const text = await (await fetch(`${rowan.base}/rpc/${method}`, { method: 'POST', body: JSON.stringify(args) })).text()
    const ms = performance.now() - t0
    const x = JSON.parse(text)
    if (x.error) throw new Error(`${method}: ${x.error}`)
    if (x.result?.error) throw new Error(`${method}: ${x.result.error}`)
    rows.push({ step: label, ms, note: `${(text.length / 1024).toFixed(0)} KB` })
    return x.result
  }
  const run = (label, sql, limit = 1000) => call(label, 'Run', { runId: 'bench', conn: cn.id, sql, limit, force: false })
  await call('Schema, first call', 'Schema', cn.id, true)
  await call('Schema, cached', 'Schema', cn.id, false)
  await call('Columns for completion', 'Columns', cn.id)
  await run('Run SELECT * FROM orders', 'SELECT * FROM orders')
  await run('Run SELECT * FROM orders again', 'SELECT * FROM orders')
  await run('Run order_items at 10,000 rows', 'SELECT * FROM order_items', 10000)
  await run('Run wide_metrics, 120 columns', 'SELECT * FROM wide_metrics')
  await run('Run a join with ORDER BY, no LIMIT', 'SELECT * FROM orders o JOIN customers c ON c.id = o.customer_id ORDER BY o.placed_at DESC')
  await run('Run a sort without an index, no LIMIT', 'SELECT * FROM order_items ORDER BY unit_cents DESC')
  await call('Preview "orders refunded"', 'Preview', { conn: cn.id, schema: 'public', table: 'orders', rest: 'refunded', limit: 8 })
  await call('Preview "customers email~acme"', 'Preview', { conn: cn.id, schema: 'public', table: 'customers', rest: 'email~acme', limit: 8 })

  // ---- The UI ----
  let ui
  if (ENGINE === 'webkit') {
    const r = spawnSync('python3', [join(import.meta.dirname, 'webkit.py'), rowan.base + '/', suite], { encoding: 'utf8', maxBuffer: 1 << 26 })
    const line = r.stdout.trim().split('\n').findLast((l) => l.startsWith('['))
    if (r.status !== 0 || !line) throw new Error('WebKitGTK run failed:\n' + r.stderr + r.stdout.slice(-2000))
    ui = JSON.parse(line)
  } else {
    const b = await browser({ width: 1440, height: 900, port: 9335 })
    try {
      if (SLOW > 1) await b.send('Emulation.setCPUThrottlingRate', { rate: SLOW })
      await b.send('Page.navigate', { url: rowan.base + '/' })
      await b.until('document.readyState !== "loading"', 20000)
      ui = await b.eval(`(async () => {\n${readFileSync(suite, 'utf8')}\n})()`)
    } finally {
      b.close()
    }
  }
  rows.push(...ui)
} catch (e) {
  console.error('failed:', e.message)
  process.exitCode = 1
} finally {
  rowan.stop()
  const w = Math.max(...rows.map((r) => r.step.length))
  console.log(`\nUI engine: ${ENGINE}${ENGINE === 'chromium' && SLOW > 1 ? `, CPU ${SLOW}x slower` : ''}\n`)
  for (const r of rows) console.log(`${r.step.padEnd(w)}  ${r.ms.toFixed(1).padStart(8)} ms  ${r.note}`)
}
