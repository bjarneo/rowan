// Captures the screenshots for the README and the site from the real UI.
//
//   node scripts/promo/shots.mjs [out-dir]
//
// The images are 2880x1800: a 1440x900 window at 2x scale. Set
// ROWAN_DEMO_PG_PORT to capture the demo PostgreSQL database from
// scripts/demo-pg. Without it, the script uses the SQLite sample.
import { mkdirSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { ROOT, addDemoPG, browser, mcpCall, moveMCP, portFree, setLook, sleep, startRowan } from './lib.mjs'

const out = resolve(process.argv[2] ?? join(ROOT, 'site/images'))
mkdirSync(out, { recursive: true })
// The default port matches the docs. A running Rowan holds it, so fall back.
const MCP_PORT = (await portFree(7421)) ? 7421 : 7430
const rowan = await startRowan({ port: 5198, build: !process.env.NO_BUILD })
const b = await browser({ width: 1440, height: 900, scale: 2, port: 9334 })

async function shot(name) {
  await sleep(250)
  writeFileSync(join(out, name), await b.shot())
  console.log(join(out, name))
}
async function load(look, ready) {
  await setLook(rowan, look)
  await b.nav(rowan.base + '/')
  await b.until(`document.querySelector(${JSON.stringify(ready)})`, 20000)
  await sleep(600)
}
async function editor(sql) {
  await b.eval(`(() => { const ta = document.querySelector('.ed textarea'); ta.focus(); ta.select(); document.execCommand('delete') })()`)
  await sleep(300)
  if (sql) await b.type(sql)
}
const click = (sel, text) => b.eval(`(() => { const el = [...document.querySelectorAll(${JSON.stringify(sel)})].find((e) => ${text ? `e.textContent.includes(${JSON.stringify(text)})` : 'true'}); if (!el) throw new Error('missing ' + ${JSON.stringify(sel)}); el.click() })()`)

try {
  await moveMCP(rowan, MCP_PORT)
  let conn = { id: 'sample', name: 'sample.sqlite' }
  if (process.env.ROWAN_DEMO_PG_PORT) conn = await addDemoPG(rowan, process.env.ROWAN_DEMO_PG_PORT)
  const ids = (await rowan.rpc('PreviewSQL', conn.id, "SELECT id FROM orders WHERE status = 'paid' AND id >= 10440 ORDER BY id LIMIT 2")).rows.map((r) => r[0])

  // Agent calls fill the feed. The writes wait in the queue until the end.
  const agent = (client, tool, args) => mcpCall(rowan, MCP_PORT, client, tool, { connection: conn.name, ...args })
  await agent('claude-ai', 'list_tables', {})
  await agent('cursor', 'describe_table', { table: 'orders' })
  await agent('claude-code', 'query', { sql: "SELECT country, count(*) AS customers FROM customers WHERE plan <> 'free' GROUP BY country ORDER BY 2 DESC" })
  await agent('claude-code', 'query', { sql: 'SELECT * FROM orders' })
  await agent('cursor', 'query', { sql: "UPDATE customers SET plan = 'enterprise' WHERE id = 7" })
  const pending = [
    agent('claude-ai', 'execute', { sql: `UPDATE orders SET status = 'refunded' WHERE id = ${ids[0]}`, reason: `Refund the duplicate charge on order ${ids[0]}` }),
    agent('cursor', 'execute', { sql: `DELETE FROM order_items WHERE order_id = ${ids[1]}`, reason: `Remove the line items of test order ${ids[1]}` }),
    agent('claude-code', 'execute', { sql: "INSERT INTO products (id, sku, name, price_cents, active) VALUES (50001, 'SKU-050001', 'Desk mat', 2499, true)", reason: 'Add the desk mat to the catalog' }),
  ]
  await sleep(1500)

  // ---- Workspace, dark ----
  await load({ theme: 'dark', layout: 'workspace' }, '.ed textarea')
  await editor("SELECT *\nFROM orders\nWHERE status = 'refunded'\n  AND total_cents > 20000\nORDER BY placed_at DESC;")
  await b.key('Enter', ['ctrl'])
  await b.until(`document.querySelector('.status .ok')`, 15000)
  await b.eval(`(() => { const r = document.querySelector('.grid .r[data-i="2"] > :nth-child(3)'); for (const t of ['mousedown', 'mouseup', 'click']) r.dispatchEvent(new MouseEvent(t, { bubbles: true, button: 0 })) })()`)
  await sleep(700)
  await shot('workspace-dark.png')

  await editor('')
  await b.type('SELECT * FROM orders o\nJOIN customers c ON c.id = o.customer_id\nWHERE c.')
  await b.until(`document.querySelector('[role=option]')`)
  await b.key('ArrowDown')
  await b.key('ArrowDown')
  await shot('completion.png')
  await b.key('Escape')

  await click(".rail button[title='MCP server']")
  await sleep(900)
  // The snippet holds the path of the dev build. Show the usual install path.
  await b.eval(`(() => { const w = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT); for (let n; (n = w.nextNode()); ) n.nodeValue = n.nodeValue.split(${JSON.stringify(rowan.bin)}).join('/usr/local/bin/rowan') })()`)
  await shot('mcp.png')

  // ---- Workspace, light ----
  await load({ theme: 'light', layout: 'workspace' }, '.ed textarea')
  await editor(`UPDATE orders SET status = 'refunded' WHERE id = ${ids[0]};`)
  await b.key('Enter', ['ctrl'])
  await b.until(`document.querySelector('.status .warn')`)
  await shot('locked.png')

  // ---- Command layout, light ----
  await load({ theme: 'light', layout: 'command' }, '.cs input')
  await b.eval(`document.querySelector('.cs input').focus()`)
  await b.type('orders refunded')
  await b.until(`document.querySelector('.cs').innerText.includes("refunded")`)
  await sleep(1200)
  await shot('command-light.png')
  await b.eval(`document.activeElement.blur()`)
  await click("button[title='Approval queue']")
  await b.until(`document.querySelectorAll('.dr .item').length === 3`)
  await shot('approval.png')
  for (const p of pending) p.catch(() => {})
} finally {
  b.close()
  rowan.stop()
}
