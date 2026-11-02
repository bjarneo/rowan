// Drives headless Chromium over the DevTools protocol.
// usage: node cdp.mjs <steps.json>
// Steps: {nav}, {wait}, {eval}, {type}, {key, mods}, {click}, {shot}, {size:[w,h]}
import { spawn } from 'node:child_process'
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs'
import { dirname } from 'node:path'

const steps = JSON.parse(readFileSync(process.argv[2], 'utf8'))
const dir = process.env.ROWAN_CDP_DIR ?? dirname(new URL(import.meta.url).pathname) + '/../.cdp'
const port = 9333
mkdirSync(dir + '/chrome', { recursive: true })
const chrome = spawn('chromium', ['--headless=new', `--remote-debugging-port=${port}`, `--user-data-dir=${dir}/chrome`, '--hide-scrollbars', '--force-color-profile=srgb', '--window-size=1440,900', 'about:blank'], { stdio: 'ignore' })
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
let targets
for (let i = 0; i < 50; i++) {
  try { targets = await (await fetch(`http://127.0.0.1:${port}/json`)).json(); if (targets.some((t) => t.type === 'page')) break } catch {}
  await sleep(150)
}
const page = targets.find((t) => t.type === 'page')
const ws = new WebSocket(page.webSocketDebuggerUrl)
await new Promise((r) => (ws.onopen = r))
let seq = 0
const pending = new Map()
ws.onmessage = (e) => {
  const m = JSON.parse(e.data)
  if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id) }
  if (m.method === 'Runtime.consoleAPICalled' && m.params.type !== 'debug') console.log('[console]', m.params.args.map((a) => a.value ?? a.description).join(' '))
  if (m.method === 'Runtime.exceptionThrown') console.log('[exception]', m.params.exceptionDetails.exception?.description ?? m.params.exceptionDetails.text)
}
const send = (method, params = {}) => new Promise((r) => { const id = ++seq; pending.set(id, r); ws.send(JSON.stringify({ id, method, params })) })
await send('Runtime.enable')
await send('Page.enable')
let W = 1440, H = 900
await send('Emulation.setDeviceMetricsOverride', { width: W, height: H, deviceScaleFactor: 1, mobile: false })
const KEYS = { Enter: 13, Escape: 27, Tab: 9, ArrowDown: 40, ArrowUp: 38, Backspace: 8, k: 75, K: 75 }
for (const s of steps) {
  if (s.size) { [W, H] = s.size; await send('Emulation.setDeviceMetricsOverride', { width: W, height: H, deviceScaleFactor: 1, mobile: false }) }
  if (s.nav) { await send('Page.navigate', { url: s.nav }); await sleep(s.wait ?? 1200); continue }
  if (s.wait && Object.keys(s).length === 1) { await sleep(s.wait); continue }
  if (s.eval) {
    const r = await send('Runtime.evaluate', { expression: s.eval, awaitPromise: true, returnByValue: true })
    const v = r.result?.result?.value ?? r.result?.exceptionDetails?.exception?.description
    if (s.print !== false) console.log('[eval]', typeof v === 'string' ? v : JSON.stringify(v))
  }
  if (s.click) {
    const r = await send('Runtime.evaluate', { expression: `(() => { const el = [...document.querySelectorAll(${JSON.stringify(s.click)})].find((e) => ${s.text ? `e.textContent.includes(${JSON.stringify(s.text)})` : 'true'}); if (!el) return 'missing'; el.click(); return 'ok' })()`, returnByValue: true })
    if (r.result.result.value !== 'ok') console.log('[click missing]', s.click, s.text ?? '')
  }
  if (s.type) await send('Input.insertText', { text: s.type })
  if (s.key) {
    const mods = (s.mods ?? []).reduce((m, x) => m | { alt: 1, ctrl: 2, meta: 4, shift: 8 }[x], 0)
    const base = { key: s.key, code: s.key.length === 1 ? 'Key' + s.key.toUpperCase() : s.key, windowsVirtualKeyCode: KEYS[s.key] ?? s.key.toUpperCase().charCodeAt(0), modifiers: mods }
    await send('Input.dispatchKeyEvent', { type: 'rawKeyDown', ...base })
    await send('Input.dispatchKeyEvent', { type: 'keyUp', ...base })
  }
  if (s.after) await sleep(s.after)
  if (s.shot) {
    const r = await send('Page.captureScreenshot', { format: 'png' })
    writeFileSync(s.shot, Buffer.from(r.result.data, 'base64'))
    console.log('[shot]', s.shot)
  }
}
ws.close()
chrome.kill()
process.exit(0)
