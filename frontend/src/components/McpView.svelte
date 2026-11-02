<script lang="ts">
  import { app } from '../lib/store.svelte'
  import { api } from '../lib/api'
  import { connColor, ENGINE, initials, ago } from '../lib/format'
  import type { Connection, Policy, McpSettings } from '../lib/types'
  import Icon from './Icon.svelte'

  const TOOLS: [string, string][] = [
    ['list_connections', 'Lists shared connections with engine and access level.'],
    ['list_tables', 'Lists the tables a connection allows.'],
    ['describe_table', 'Returns columns, types, keys and indexes.'],
    ['query', 'Runs read-only statements. Results are capped at the row limit.'],
    ['explain', 'Returns the query plan without running the query.'],
    ['execute', 'Runs INSERT, UPDATE and DELETE. Goes to the approval queue when approvals are on.'],
  ]

  let client = $state('claude-desktop')
  let setup = $state<{ file: string; code: string } | null>(null)
  let expand = $state<string | null>(null)
  let tablesOf = $state<Record<string, string[]>>({})

  $effect(() => {
    const c = client
    api.mcpSetup(c).then((s) => (setup = s))
  })

  const m = $derived(app.config.mcp)

  async function setMcp(patch: Partial<McpSettings>) {
    const next = { ...m, ...patch }
    app.config = { ...app.config, mcp: next }
    try {
      app.mcp = await api.setMcp(next)
      if ('enabled' in patch) app.toast(next.enabled ? 'MCP server running' : 'MCP server stopped')
    } catch (e) {
      app.toast(String(e))
    }
  }

  async function setPol(c: Connection, patch: Partial<Policy>) {
    const p = { ...c.mcp, ...patch }
    app.config = await api.setPolicy(c.id, p)
  }

  async function edit(c: Connection) {
    if (expand === c.id) {
      expand = null
      return
    }
    expand = c.id
    if (!tablesOf[c.id]) {
      try {
        const s = await api.schema(c.id)
        tablesOf = { ...tablesOf, [c.id]: s.groups?.flatMap((g) => g.tables.map((t) => t.name)) ?? [] }
      } catch (e) {
        app.toast(String(e))
      }
    }
  }

  function toggleTable(c: Connection, t: string) {
    const all = tablesOf[c.id] ?? []
    const cur = c.mcp.tables?.length ? c.mcp.tables : all
    const next = cur.includes(t) ? cur.filter((x) => x !== t) : [...cur, t]
    setPol(c, { tables: next.length === all.length ? [] : next })
  }

  function toggleClient(name: string) {
    const b = m.blockedClients ?? []
    setMcp({ blockedClients: b.includes(name) ? b.filter((x) => x !== name) : [...b, name] })
  }

  function copy() {
    if (!setup) return
    api.copy(setup.code)
    app.toast('Copied to the clipboard')
  }
</script>

<div class="mv">
  <div class="inner">
    <div class="head">
      <div>
        <h1>MCP server</h1>
        <p class="muted">Agents connect over stdio through <span class="mono">rowan mcp --stdio</span>, or over Streamable HTTP on <span class="mono">127.0.0.1:{app.mcp.port}</span> with a bearer token.</p>
      </div>
      <span class="grow"></span>
      <span class="row" style="gap:8px">
        <span class="dot" style="background:{app.mcp.running ? 'var(--ok)' : 'var(--line-strong)'}"></span>
        <span>{app.mcp.running ? 'Running' : 'Stopped'}</span>
        <button class="switch" class:on={m.enabled} onclick={() => setMcp({ enabled: !m.enabled })} aria-label="Run the MCP server"></button>
      </span>
    </div>
    {#if app.mcp.error}<div class="err danger">{app.mcp.error}</div>{/if}

    <div class="cols">
      <section class="card">
        <div class="sh">Clients</div>
        {#each app.mcp.clients ?? [] as c, i}
          <div class="cl" class:bt={i > 0}>
            <span class="badge">{initials(c.name)}</span>
            <div class="grow">
              <div class="row" style="gap:6px"><span class="strong">{c.name}</span><span class="faint mono">{c.transport}</span></div>
              <div class="faint">{c.blocked ? 'Blocked by you' : c.connected ? `Connected ${ago(c.since, app.now).replace(' ago', '')} · ${c.calls} calls` : `Last seen ${ago(c.lastSeen, app.now)}`}</div>
            </div>
            <span class="dot" style="background:{c.connected && !c.blocked ? 'var(--ok)' : 'var(--line-strong)'}"></span>
            <button class="btn sm" onclick={() => toggleClient(c.name)}>{c.blocked ? 'Allow' : 'Block'}</button>
          </div>
        {:else}
          <div class="faint pad">No client has connected yet. Add Rowan to a client with the snippet below.</div>
        {/each}
      </section>

      <section class="card">
        <div class="sh row">
          <span>Connect a client</span>
          <span class="grow"></span>
          <div class="seg">
            {#each [['claude-desktop', 'Claude Desktop'], ['claude-code', 'Claude Code'], ['cursor', 'Cursor'], ['http', 'HTTP']] as [k, l]}
              <button class:on={client === k} onclick={() => (client = k)}>{l}</button>
            {/each}
          </div>
        </div>
        {#if setup}
          <div class="snip">
            <div class="row"><span class="faint mono">{setup.file}</span><span class="grow"></span><button class="btn sm" onclick={copy}><Icon name="content_copy" size={14} />Copy</button></div>
            <pre class="selectable">{setup.code}</pre>
          </div>
        {/if}
      </section>
    </div>

    <section class="card">
      <div class="sh">Connections</div>
      {#each app.conns as c, i (c.id)}
        {@const p = c.mcp}
        {@const all = tablesOf[c.id] ?? []}
        <div class="cn" class:bt={i > 0} style="opacity:{p.exposed ? 1 : 0.55}">
          <button class="switch" class:on={p.exposed} onclick={() => setPol(c, { exposed: !p.exposed })} aria-label="Share {c.name}"></button>
          <div class="nm">
            <div class="row" style="gap:6px"><span class="dot" style="background:{connColor(c.color)}"></span><span class="strong">{c.name}</span>{#if c.production}<span class="chip prod">production</span>{/if}</div>
            <div class="faint">{ENGINE[c.engine].label}</div>
          </div>
          <div class="seg">
            <button class:on={p.access === 'read-only'} onclick={() => setPol(c, { access: 'read-only' })}>Read-only</button>
            <button class:on={p.access === 'read-write'} onclick={() => setPol(c, { access: 'read-write', approveWrites: true })}>Read and write</button>
          </div>
          <label class="row ap" title={c.production ? 'Always on for production' : ''}>
            <button class="switch" class:on={p.access === 'read-write' && (p.approveWrites || c.production)} disabled={c.production || p.access === 'read-only'} onclick={() => setPol(c, { approveWrites: !p.approveWrites })} aria-label="Approve writes"></button>
            <span class="muted">{p.access === 'read-only' ? 'Agents can’t write' : c.production ? 'Approval always on' : p.approveWrites ? 'Writes wait for approval' : 'Writes run immediately'}</span>
          </label>
          <label class="row lim"><span class="faint">Rows</span><input class="input mono" type="number" min="1" value={p.rowLimit} onchange={(e) => setPol(c, { rowLimit: Math.max(1, +e.currentTarget.value) })} /></label>
          <button class="btn sm" onclick={() => edit(c)}>{p.tables?.length ? `${p.tables.length} tables` : 'All tables'}<Icon name={expand === c.id ? 'expand_more' : 'chevron_right'} size={14} /></button>
        </div>
        {#if expand === c.id}
          <div class="tables">
            <span class="faint">Agents see these tables. Statements that name a hidden table are blocked.</span>
            <div class="chips">
              {#each all as t}
                {@const on = !p.tables?.length || p.tables.includes(t)}
                <button class="chip tb" class:on onclick={() => toggleTable(c, t)}><Icon name={on ? 'check' : 'block'} size={13} />{t}</button>
              {/each}
            </div>
          </div>
        {/if}
      {:else}
        <div class="faint pad">No connections.</div>
      {/each}
    </section>

    <section class="card">
      <div class="sh">Tools</div>
      {#each TOOLS as [n, d], i}
        <div class="tool" class:bt={i > 0}>
          <span class="mono strong">{n}</span>
          {#if n === 'execute'}<span class="chip wr">writes</span>{/if}
          <span class="muted grow">{d}</span>
          <button class="switch" class:on={m.tools.includes(n)} onclick={() => setMcp({ tools: m.tools.includes(n) ? m.tools.filter((x) => x !== n) : [...m.tools, n] })} aria-label={n}></button>
        </div>
      {/each}
    </section>
  </div>
</div>

<style>
  .mv { height: 100%; overflow: auto; }
  .inner { max-width: 1080px; margin: 0 auto; padding: 28px 32px 48px; display: flex; flex-direction: column; gap: 16px; }
  .head { display: flex; align-items: flex-start; gap: 16px; }
  h1 { margin: 0 0 4px; font: 600 20px/1.25 var(--sans); letter-spacing: -0.015em; }
  :global(:root[data-layout='workspace']) h1 { font: 600 15px var(--mono); letter-spacing: 0; }
  p { margin: 0; line-height: 1.5; }
  .err { padding: 8px 12px; background: var(--danger-soft); border-radius: var(--r); }
  .cols { display: grid; grid-template-columns: 1fr 1.2fr; gap: 16px; }
  .card { overflow: hidden; }
  .sh { padding: 12px 16px; border-bottom: 1px solid var(--line); font-weight: 500; }
  .sh.row { gap: 8px; padding: 8px 10px 8px 16px; }
  .pad { padding: 14px 16px; }
  .bt { border-top: 1px solid var(--line); }
  .cl { display: flex; align-items: center; gap: 10px; padding: 10px 16px; }
  .badge { width: 28px; height: 28px; border-radius: 7px; display: grid; place-items: center; background: var(--sunken); border: 1px solid var(--line); font: 600 10px var(--mono); color: var(--muted); }
  :global(:root[data-layout='workspace']) .badge { border-radius: 0; }
  .strong { font-weight: 500; }
  .snip { padding: 10px 16px 14px; display: flex; flex-direction: column; gap: 8px; }
  .snip pre { margin: 0; padding: 10px 12px; background: var(--sunken); border-radius: var(--r); font: 400 12px/1.55 var(--mono); overflow: auto; white-space: pre; }
  .cn { display: grid; grid-template-columns: 30px minmax(160px, 1fr) 236px 220px 116px 120px; gap: 14px; align-items: center; padding: 10px 16px; }
  .cn > .btn { justify-content: space-between; }
  .nm { min-width: 0; }
  .ap { gap: 8px; font-size: 12px; }
  .lim { gap: 6px; }
  .lim .input { height: 26px; width: 74px; }
  .chip.prod { background: var(--danger-soft); color: var(--danger); font-size: 11px; padding: 1px 6px; }
  .chip.wr { background: var(--warn-soft); color: var(--warn); font-size: 11px; padding: 1px 6px; }
  .tables { padding: 4px 16px 14px 60px; display: flex; flex-direction: column; gap: 8px; font-size: 12px; }
  .chips { display: flex; flex-wrap: wrap; gap: 6px; }
  .chip.tb { border: 1px solid var(--line); color: var(--faint); font: 400 12px var(--mono); }
  .chip.tb.on { background: var(--accent-soft); border-color: transparent; color: var(--accent-text); }
  .tool { display: flex; align-items: center; gap: 12px; padding: 10px 16px; }
  .tool .mono { min-width: 130px; }
</style>
