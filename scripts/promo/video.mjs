// Records the promo video from the real UI.
//
//   node scripts/promo/video.mjs [out.mp4]
//
// The script drives the app frame by frame. Each frame is a screenshot of the
// stage at time t, so the video is the same on a slow or a fast machine.
// Needs Go, Node, Chromium and ffmpeg. To record against the demo PostgreSQL
// database, start it with scripts/demo-pg/start.sh and set ROWAN_DEMO_PG_PORT.
import { spawn, spawnSync } from 'node:child_process'
import { once } from 'node:events'
import { mkdirSync, rmSync, writeFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { ROOT, addDemoPG, browser, mcpCall, moveMCP, setLook, sleep, startRowan } from './lib.mjs'

const FPS = 30
const MCP_PORT = 7429
const out = resolve(process.argv[2] ?? join(ROOT, 'site/media/rowan-promo.mp4'))
const poster = out.replace(/\.mp4$/, '.jpg')
mkdirSync(dirname(out), { recursive: true })

const rowan = await startRowan({ build: !process.env.NO_BUILD })
const b = await browser({ width: 1920, height: 1080 })
const ff = spawn('ffmpeg', [
  '-y', '-loglevel', 'error',
  '-f', 'image2pipe', '-c:v', 'png', '-framerate', String(FPS), '-i', '-',
  // X accepts silent video, but some players expect an audio track.
  '-f', 'lavfi', '-i', 'anullsrc=channel_layout=stereo:sample_rate=48000',
  '-map', '0:v', '-map', '1:a', '-shortest',
  '-c:v', 'libx264', '-preset', 'slow', '-crf', '20', '-pix_fmt', 'yuv420p', '-profile:v', 'high', '-level', '4.2', '-g', String(FPS * 2),
  '-c:a', 'aac', '-b:a', '96k', '-movflags', '+faststart', out,
], { stdio: ['pipe', 'inherit', 'inherit'] })

let t = 0
let frames = 0
let posterAt = -1
// A fixed seed keeps the typing rhythm the same on every run.
let seed = 7
const jitter = () => [2, 2, 3, 2, 1, 2, 3, 2, 2, 1][(seed = (seed * 31 + 11) % 997) % 10]

const S = (js) => b.eval(`void (() => { ${js} })()`)
async function frame() {
  await b.eval(`stage.render(${t.toFixed(4)})`)
  const png = await b.shot()
  if (frames === posterAt) writeFileSync(poster.replace(/\.jpg$/, '.png'), png)
  if (!ff.stdin.write(png)) await once(ff.stdin, 'drain')
  t += 1 / FPS
  frames++
}
async function hold(sec) {
  for (let n = Math.round(sec * FPS); n > 0; n--) await frame()
}
const tween = (name, to, dur, ease = 'inOut', delay = 0) => S(`stage.tween('${name}', ${JSON.stringify(to)}, ${t + delay}, ${dur}, '${ease}')`)
const caption = (html, delay = 0) => S(`stage.caption(${JSON.stringify(html)}, ${t + delay})`)
const rect = (sel, text) => b.eval(`stage.rect(${JSON.stringify(sel)}, ${JSON.stringify(text ?? '')})`)
async function camTo(r, opts = {}, dur = 1.1) {
  const c = await b.eval(`stage.fit(${JSON.stringify(r)}, ${JSON.stringify(opts)})`)
  await tween('cam', c, dur)
}
async function type(text) {
  for (const ch of text) {
    if (ch === '\n') await b.key('Enter')
    else await b.type(ch)
    for (let n = jitter(); n > 0; n--) await frame()
  }
}
async function press(key, mods) {
  await b.key(key, mods)
  await hold(2 / FPS)
}
/** Moves the pointer to an element and clicks it. */
async function click(sel, text, dur = 0.75) {
  const r = await rect(sel, text)
  await tween('cur', { x: r.x + r.w / 2 + 4, y: r.y + r.h / 2 + 2 }, dur)
  await hold(dur + 0.1)
  await tween('cur', { press: 1 }, 0.08, 'out')
  await hold(0.1)
  await b.eval(`(() => { const els = [...stage.doc.querySelectorAll(${JSON.stringify(sel)})]; const el = ${text ? `els.find((e) => e.textContent.includes(${JSON.stringify(text)}))` : 'els[0]'}; el.click() })()`)
  await tween('cur', { press: 0 }, 0.22, 'out')
  await hold(0.24)
}

try {
  await moveMCP(rowan, MCP_PORT)
  await setLook(rowan, { theme: 'dark', layout: 'workspace' })
  let conn = { id: 'sample', name: 'sample.sqlite' }
  if (process.env.ROWAN_DEMO_PG_PORT) conn = await addDemoPG(rowan, process.env.ROWAN_DEMO_PG_PORT)
  // An order that is still paid makes the approval show a real change.
  const pick = await rowan.rpc('PreviewSQL', conn.id,"SELECT id FROM orders WHERE status = 'paid' AND id >= 10440 ORDER BY id LIMIT 1")
  const orderID = pick.rows[0][0]

  await b.nav(rowan.base + '/stage/index.html')
  await b.until('window.stage')
  await b.eval(`stage.load('/', '.ed textarea')`)
  await b.eval(`(() => { const ta = stage.doc.querySelector('.ed textarea'); ta.focus(); ta.select(); stage.doc.execCommand('delete') })()`)
  // Focus loads the columns for completion. Give the call time to finish.
  await sleep(900)
  const ed = await rect('.ed')
  const center = await rect('section.center')

  // ---- Intro ----
  await S(`stage.set('win', { o: 0, dy: 70, s: 0.95 }); stage.set('intro', { o: 0, dy: 24 })`)
  await tween('intro', { o: 1, dy: 0 }, 0.9, 'out', 0.15)
  await hold(2.3)
  await tween('intro', { o: 0, dy: -20 }, 0.5)
  await tween('win', { o: 1, dy: 0, s: 1 }, 1.2, 'out', 0.25)
  await caption('Every connection opens read-only', 0.7)
  await hold(1.5)
  const ro = await rect('.cell', 'read-only')
  await camTo({ x: ro.x - 380, y: 0, w: 520, h: 190 }, { max: 2.3 }, 1.2)
  await hold(2.0)

  // ---- Completion ----
  await camTo({ x: ed.x - 8, y: ed.y - 36, w: 720, h: 330 }, {}, 1.1)
  await caption('Completion for tables after <code>FROM</code> and <code>JOIN</code>', 0.3)
  await hold(1.2)
  await type('SELECT * FROM ord')
  await hold(0.6)
  await press('Tab')
  await type(' o\n')
  await type('JOIN cus')
  await hold(0.5)
  await press('Tab')
  await type(' c ON c.')
  await caption('Columns for each alias in the statement')
  posterAt = frames + 12
  await hold(0.9)
  await press('Tab')
  await type(' = o.cus')
  await hold(0.45)
  await press('Tab')
  await camTo({ x: ed.x - 8, y: ed.y + 4, w: 720, h: 330 }, {}, 0.9)
  await type('\n')
  await caption('Keywords in the case that you type')
  await type('WH')
  await hold(0.45)
  await press('Tab')
  await type(' o.st')
  await hold(0.4)
  await press('Tab')
  await type(" = 'shipped'\n")
  await type('ORD')
  await hold(0.4)
  await press('Tab')
  await type(' o.pl')
  await hold(0.4)
  await press('Tab')
  await type(' DE')
  await hold(0.4)
  await press('Tab')
  await type('\nLIM')
  await hold(0.4)
  await press('Tab')
  await type(' 100')
  await hold(0.7)

  // ---- Run ----
  await caption('<kbd>Ctrl</kbd> <kbd>Enter</kbd> runs the query')
  await hold(0.5)
  await press('Enter', ['ctrl'])
  await b.until(`stage.doc.querySelector('.status .ok')`, 60000)
  await camTo({ x: center.x, y: ed.y + 30, w: center.w, h: 430 }, { max: 1.7 }, 1.1)
  await hold(2.6)

  // ---- Locked write ----
  await camTo({ x: center.x, y: ed.y - 36, w: center.w, h: 300 }, { max: 2.0 }, 1.0)
  await caption('A write does not run until you unlock the connection', 0.2)
  await hold(1.0)
  await b.eval(`stage.doc.querySelector('.ed textarea').select()`)
  await b.type(`UPDATE orders SET status = 'refunded' WHERE id = ${orderID};`)
  await hold(0.6)
  await press('Enter', ['ctrl'])
  await b.until(`stage.doc.querySelector('.status .warn')`)
  await hold(2.4)

  // ---- Agent write ----
  const call = mcpCall(rowan, MCP_PORT, 'claude-code', 'execute', {
    connection: conn.name,
    sql: `UPDATE orders SET status = 'refunded' WHERE id = ${orderID}`,
    reason: `Refund the duplicate charge on order ${orderID}`,
  })
  await b.until(`stage.doc.querySelector('.feed .it')`)
  const feed = await rect('.feed')
  await camTo({ x: feed.x - 160, y: feed.y - 60, w: 470, h: 210 }, { max: 2.4 }, 1.2)
  await caption('Agents connect over MCP. Their writes wait for you.', 0.2)
  const item = await rect('.feed .it')
  await S(`stage.set('cur', { x: ${item.x + 120}, y: ${item.y + 150}, o: 0, press: 0 })`)
  await tween('cur', { o: 1 }, 0.3, 'out', 0.9)
  await hold(1.9)
  await click('.feed .it', null, 0.7)
  await b.until(`stage.doc.querySelector('.dr .item .acts')`)
  const card = await rect('.dr .item')
  await camTo({ x: card.x - 10, y: card.y - 60, w: card.w + 20, h: card.h + 70 }, { max: 2.0 }, 1.1)
  await caption('You see the reason, the SQL and the rows that change', 0.2)
  await hold(2.8)
  await click('.dr .item .acts .btn.dark', null, 0.8)
  await call
  await b.until(`[...stage.doc.querySelectorAll('.feed .it')].some((e) => e.textContent.includes('approved'))`)
  await caption('Rowan runs the write only after you approve it')
  await hold(0.5)
  await tween('cur', { o: 0 }, 0.3)
  await b.key('Escape')
  await camTo({ x: feed.x - 160, y: feed.y - 60, w: 470, h: 210 }, { max: 2.4 }, 1.1)
  await hold(2.4)

  // ---- Command layout ----
  await tween('win', { o: 0, s: 0.97 }, 0.45)
  await hold(0.5)
  await setLook(rowan, { theme: 'light', layout: 'command' })
  await b.eval(`stage.load('/', '.cs input')`)
  await sleep(600)
  await S(`stage.tween('cam', { cx: 720, cy: 450, k: 0.9, fy: 505 }, ${t}, 0)`)
  await tween('win', { o: 1, s: 1 }, 0.7, 'out')
  await caption('Two layouts: a search surface or a full workspace', 0.1)
  await hold(1.0)
  await b.eval(`stage.doc.querySelector('.cs input').focus()`)
  const cs = await rect('.cs')
  await camTo({ x: cs.x, y: cs.y, w: cs.w, h: 480 }, { max: 1.7 }, 1.3)
  await type('orders refunded')
  await sleep(900)
  await hold(2.6)

  // ---- Outro ----
  await caption('')
  await tween('win', { o: 0, s: 0.96 }, 0.6)
  await tween('scrim', { o: 0 }, 0.6)
  await tween('outro', { o: 1, dy: 0 }, 0.9, 'out', 0.35)
  await hold(3.6)

  ff.stdin.end()
  await once(ff, 'close')
  if (ff.exitCode !== 0) throw new Error('ffmpeg failed')
  const png = poster.replace(/\.jpg$/, '.png')
  if (spawnSync('ffmpeg', ['-v', 'error', '-y', '-i', png, '-q:v', '3', poster]).status !== 0) throw new Error('poster failed')
  rmSync(png)
  console.log(`${out}: ${frames} frames, ${(frames / FPS).toFixed(1)} s`)
} catch (e) {
  // Show what the app showed, so a failed run is easy to fix.
  const state = await b.eval(`JSON.stringify({ sql: stage.doc.querySelector('.ed textarea')?.value, status: stage.doc.querySelector('.status')?.innerText })`).catch(() => '')
  console.error('state at failure:', state)
  writeFileSync(out.replace(/\.mp4$/, '-failure.png'), await b.shot())
  ff.kill('SIGKILL')
  await once(ff, 'close')
  rmSync(out, { force: true })
  throw e
} finally {
  b.close()
  rowan.stop()
}
