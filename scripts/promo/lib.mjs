// Shared helpers for the promo video and the screenshots. They start the dev
// server with a fresh config and drive headless Chromium over the DevTools
// protocol.
import { spawn, spawnSync } from 'node:child_process'
import { cpSync, mkdtempSync, mkdirSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'

export const ROOT = resolve(import.meta.dirname, '../..')
export const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

function run(cmd, args, cwd) {
  const r = spawnSync(cmd, args, { cwd, stdio: 'inherit' })
  if (r.status !== 0) throw new Error(`${cmd} ${args.join(' ')} failed`)
}

/**
 * Builds the frontend and the dev server, then starts the server on a fresh
 * config directory. The UI dist is a copy, so the stage page can sit next to
 * the app on the same origin.
 */
export async function startRowan({ port = 5199, build = true } = {}) {
  const work = mkdtempSync(join(tmpdir(), 'rowan-promo-'))
  const bin = join(work, 'rowan-dev')
  if (build) run('npm', ['run', 'build', '--silent'], join(ROOT, 'frontend'))
  const tags = process.platform === 'linux' ? 'devserve webkit2_41' : 'devserve'
  run('go', ['build', '-tags', tags, '-o', bin, '.'], ROOT)
  const dist = join(work, 'dist')
  cpSync(join(ROOT, 'frontend/dist'), dist, { recursive: true })
  cpSync(join(ROOT, 'scripts/promo/stage'), join(dist, 'stage'), { recursive: true })
  for (const f of ['geist-latin-wght-normal.woff2']) cpSync(join(ROOT, 'frontend/node_modules/@fontsource-variable/geist/files', f), join(dist, 'stage', f))
  for (const w of [400, 500]) cpSync(join(ROOT, `frontend/node_modules/@fontsource/ibm-plex-mono/files/ibm-plex-mono-latin-${w}-normal.woff2`), join(dist, 'stage', `plex-${w}.woff2`))
  cpSync(join(ROOT, 'build/appicon.svg'), join(dist, 'stage/appicon.svg'))
  // A home directory of its own makes the UI show ~/.config/rowan, not a temp path.
  const home = join(work, 'home')
  const cfg = join(home, '.config/rowan')
  mkdirSync(cfg, { recursive: true })
  const env = { ...process.env, HOME: home, ROWAN_CONFIG_DIR: cfg, ROWAN_CREDENTIALS: 'file', ROWAN_DIST: dist }
  const proc = spawn(bin, ['--serve', `127.0.0.1:${port}`], { env, stdio: ['ignore', 'ignore', 'inherit'] })
  const base = `http://127.0.0.1:${port}`
  const rpc = async (method, ...args) => {
    const r = await (await fetch(`${base}/rpc/${method}`, { method: 'POST', body: JSON.stringify(args) })).json()
    if (r.error) throw new Error(`${method}: ${r.error}`)
    return r.result
  }
  for (let i = 0; ; i++) {
    try {
      await rpc('Bootstrap')
      break
    } catch (e) {
      if (i > 100) throw e
      await sleep(100)
    }
  }
  return {
    base,
    bin,
    cfg,
    rpc,
    stop() {
      proc.kill()
      rmSync(work, { recursive: true, force: true })
    },
  }
}

/** Sets the theme and the layout through the same call that Settings uses. */
export async function setLook(rowan, { theme, layout }) {
  const b = await rowan.rpc('Bootstrap')
  await rowan.rpc('SaveSettings', { ...b.config.app, theme, layout }, b.config.safety)
}

/** Moves the MCP server to another port, so a running Rowan does not collide. */
export async function moveMCP(rowan, port) {
  const b = await rowan.rpc('Bootstrap')
  await rowan.rpc('SetMCP', { ...b.config.mcp, enabled: true, httpPort: port })
}

/** Calls an MCP tool as an agent. The promise resolves when Rowan answers. */
export async function mcpCall(rowan, mcpPort, client, tool, args) {
  const { readFileSync } = await import('node:fs')
  const token = readFileSync(join(rowan.cfg, 'mcp-token'), 'utf8').trim()
  const url = `http://127.0.0.1:${mcpPort}/mcp`
  const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json', Accept: 'application/json, text/event-stream' }
  const init = await fetch(url, { method: 'POST', headers, body: JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: '2025-06-18', capabilities: {}, clientInfo: { name: client, version: '1' } } }) })
  const sid = init.headers.get('mcp-session-id')
  await init.text()
  const r = await fetch(url, { method: 'POST', headers: { ...headers, 'Mcp-Session-Id': sid }, body: JSON.stringify({ jsonrpc: '2.0', id: 2, method: 'tools/call', params: { name: tool, arguments: args } }) })
  return r.text()
}

const KEYS = { Enter: 13, Escape: 27, Tab: 9, ArrowDown: 40, ArrowUp: 38, ArrowLeft: 37, ArrowRight: 39, Backspace: 8, Home: 36, End: 35, ' ': 32 }

/** Starts headless Chromium and returns a small DevTools client. */
export async function browser({ width, height, scale = 1, port = 9333 }) {
  const prof = mkdtempSync(join(tmpdir(), 'rowan-chrome-'))
  const bin = process.env.CHROME ?? ['chromium', 'google-chrome-stable', 'google-chrome'].find((b) => spawnSync('which', [b]).status === 0)
  const chrome = spawn(bin, ['--headless=new', `--remote-debugging-port=${port}`, `--user-data-dir=${prof}`, '--hide-scrollbars', '--force-color-profile=srgb', '--font-render-hinting=none', '--disable-lcd-text', `--window-size=${width},${height}`, 'about:blank'], { stdio: 'ignore' })
  let targets
  for (let i = 0; ; i++) {
    try {
      targets = await (await fetch(`http://127.0.0.1:${port}/json`)).json()
      if (targets.some((t) => t.type === 'page')) break
    } catch {}
    if (i > 80) throw new Error('Chromium did not start')
    await sleep(150)
  }
  const ws = new WebSocket(targets.find((t) => t.type === 'page').webSocketDebuggerUrl)
  await new Promise((r) => (ws.onopen = r))
  let seq = 0
  const pending = new Map()
  ws.onmessage = (e) => {
    const m = JSON.parse(e.data)
    if (m.id && pending.has(m.id)) {
      pending.get(m.id)(m)
      pending.delete(m.id)
    }
    if (m.method === 'Runtime.exceptionThrown') console.error('[page]', m.params.exceptionDetails.exception?.description ?? m.params.exceptionDetails.text)
    if (m.method === 'Runtime.consoleAPICalled' && m.params.type === 'error') console.error('[page]', m.params.args.map((a) => a.value ?? a.description).join(' '))
  }
  const send = (method, params = {}) =>
    new Promise((res, rej) => {
      const id = ++seq
      pending.set(id, (m) => (m.error ? rej(new Error(`${method}: ${m.error.message}`)) : res(m.result)))
      ws.send(JSON.stringify({ id, method, params }))
    })
  await send('Runtime.enable')
  await send('Page.enable')
  await send('Emulation.setFocusEmulationEnabled', { enabled: true })
  await send('Emulation.setDeviceMetricsOverride', { width, height, deviceScaleFactor: scale, mobile: false })

  const b = {
    send,
    async eval(expr) {
      const r = await send('Runtime.evaluate', { expression: expr, awaitPromise: true, returnByValue: true })
      if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description ?? r.exceptionDetails.text)
      return r.result.value
    },
    async nav(url) {
      await send('Page.navigate', { url })
      await sleep(300)
    },
    /** Waits in real time until expr is truthy. */
    async until(expr, ms = 8000) {
      const t0 = Date.now()
      for (;;) {
        if (await b.eval(`!!(${expr})`)) return
        if (Date.now() - t0 > ms) throw new Error('timed out: ' + expr)
        await sleep(40)
      }
    },
    type: (text) => send('Input.insertText', { text }),
    async key(key, mods = []) {
      const m = mods.reduce((a, x) => a | { alt: 1, ctrl: 2, meta: 4, shift: 8 }[x], 0)
      const code = key.length === 1 ? (key === ' ' ? 'Space' : 'Key' + key.toUpperCase()) : key
      const base = { key, code, windowsVirtualKeyCode: KEYS[key] ?? key.toUpperCase().charCodeAt(0), modifiers: m }
      // Enter carries its text, so the browser inserts the line break that a
      // real key inserts when no handler prevents it.
      if (key === 'Enter') await send('Input.dispatchKeyEvent', { type: 'keyDown', text: '\r', unmodifiedText: '\r', ...base })
      else await send('Input.dispatchKeyEvent', { type: 'rawKeyDown', ...base })
      await send('Input.dispatchKeyEvent', { type: 'keyUp', ...base })
    },
    async shot(opts = {}) {
      const r = await send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false, ...opts })
      return Buffer.from(r.data, 'base64')
    },
    close() {
      ws.close()
      chrome.kill()
      setTimeout(() => rmSync(prof, { recursive: true, force: true }), 500)
    },
  }
  return b
}

/**
 * Adds the demo PostgreSQL database from scripts/demo-pg as "acme-prod" and
 * makes it the open connection. Returns the connection.
 */
export async function addDemoPG(rowan, port) {
  const cn = await rowan.rpc('SaveConnection', {
    name: 'acme-prod', engine: 'postgres', host: '127.0.0.1', port: Number(port), database: 'acme', user: 'postgres', sslMode: 'disable',
    color: 'red', production: true, ssh: { enabled: false },
    mcp: { exposed: true, access: 'read-write', approveWrites: true, rowLimit: 500, tables: [] },
    password: 'rowan', passwordChanged: true,
  })
  // New connections start read-only for agents. The policy view changes that.
  await rowan.rpc('SetPolicy', cn.id, { exposed: true, access: 'read-write', approveWrites: true, rowLimit: 500, tables: [] })
  await rowan.rpc('SetLastConn', cn.id)
  return cn
}

/** Reports whether a TCP port on 127.0.0.1 is free. */
export async function portFree(port) {
  const { createServer } = await import('node:net')
  return new Promise((res) => {
    const srv = createServer().once('error', () => res(false))
    srv.listen(port, '127.0.0.1', () => srv.close(() => res(true)))
  })
}
