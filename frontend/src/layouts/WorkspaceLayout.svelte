<script lang="ts">
  // Layout 2b: workspace split, technical treatment. Connections are
  // top-level tabs. Editor and grid stack. A row inspector and a live agent
  // feed sit on the right.
  import { app, type Rail } from '../lib/store.svelte'
  import { ENGINE, connColor, fmtK, mmss, MOD } from '../lib/format'
  import type { Link } from '../lib/types'
  import QueryPane from '../components/QueryPane.svelte'
  import RowInspector from '../components/RowInspector.svelte'
  import AgentFeed from '../components/AgentFeed.svelte'
  import LogView from '../components/LogView.svelte'
  import McpView from '../components/McpView.svelte'
  import SettingsView from '../components/SettingsView.svelte'
  import Icon from '../components/Icon.svelte'

  const cn = $derived(app.active)
  const s = $derived(cn ? app.session(cn.id) : null)
  const unl = $derived(cn ? app.unlocked(cn.id) : false)
  const until = $derived(cn ? app.locks[cn.id] : 0)
  const schema = $derived(cn ? app.schemas[cn.id] : undefined)
  const err = $derived(cn ? app.schemaErr[cn.id] : '')
  let filter = $state('')

  const hidden = (t: string) => {
    const p = cn?.mcp
    if (!p?.exposed || !app.mcp.running) return false
    return !!p.tables?.length && !p.tables.includes(t)
  }
  const srcTable = $derived(s?.result?.source?.table ?? '')

  const rails: [Rail, string, string][] = [
    ['tables', 'database', 'Tables and editor'],
    ['mcp', 'hub', 'MCP server'],
    ['log', 'receipt_long', 'Activity log'],
  ]

  function follow(l: Link) {
    if (!s) return
    s.run({ sql: l.sql.replace(/;\s*$/, ''), label: l.table })
  }

  function host() {
    if (!cn) return ''
    if (cn.engine === 'sqlite') return cn.file
    return `${cn.host}:${cn.port}${cn.ssh.enabled ? ' via ' + cn.ssh.host : ''}`
  }
</script>

<div class="ws">
  <header>
    <div class="logo">R</div>
    <div class="tabs">
      {#each app.conns as c (c.id)}
        <button
          class="tab"
          class:on={c.id === app.activeConn}
          style="--cc:{connColor(c.color)}"
          onclick={() => app.setActive(c.id)}
          ondblclick={() => (app.connDialog = { id: c.id })}
          title="Double-click to edit {c.name}"
        >
          <span class="sq" style="background:{connColor(c.color)}"></span>{c.name}{#if c.engine !== 'sqlite'}<span class="faint">{c.engine === 'postgres' ? 'pg' : 'mysql'}</span>{/if}
        </button>
      {/each}
      <button class="plus faint" onclick={() => (app.connDialog = { id: null })} title="New connection">+</button>
    </div>
    <span class="grow"></span>
    <button class="cell faint" onclick={() => (app.palette = true)} title="Search ({MOD}K)"><Icon name="search" size={15} /><span class="kbd">{MOD}K</span></button>
    <button class="cell faint" onclick={() => app.toggleLayout()} title="Switch to the command layout"><Icon name="space_dashboard" size={15} /></button>
    <button class="cell faint" onclick={() => app.toggleTheme()} title="Toggle light and dark"><Icon name={app.theme === 'dark' ? 'light_mode' : 'dark_mode'} size={15} /></button>
    {#if cn}
      <button class="cell" class:warn={unl} class:muted={!unl} onclick={() => (unl ? app.relock(cn.id) : app.requireWrite(cn.id))} title={unl ? 'Relock writes' : 'Unlock writes'}>
        <Icon name={unl ? 'lock_open' : 'lock'} size={15} />{unl ? (until === -1 ? 'writes unlocked' : `unlocked · ${mmss(until - app.now)}`) : 'read-only'}
      </button>
    {/if}
    <button class="cell" onclick={() => (app.queueOpen = !app.queueOpen)} title="Approval queue">
      <span class="sq" style="width:6px;height:6px;background:{app.mcp.running ? 'var(--ok)' : 'var(--line-strong)'}"></span>mcp
      {#if app.queue.length}<span class="accent">{app.queue.length} queued</span>{/if}
    </button>
  </header>

  <div class="main">
    <nav class="rail">
      {#each rails as [k, icon, label]}
        <button class:on={app.rail === k} onclick={() => (app.rail = k)} title={label}><Icon name={icon} size={18} /></button>
      {/each}
      <span class="grow"></span>
      <button class:on={app.rail === 'settings'} onclick={() => (app.rail = 'settings')} title="Settings"><Icon name="settings" size={18} /></button>
    </nav>

    {#if app.rail === 'tables'}
      {#if !cn}
        <div class="empty grow">
          <div>
            <p>Rowan has no connections.</p>
            <button class="btn primary" onclick={() => (app.connDialog = { id: null })}>New connection</button>
          </div>
        </div>
      {:else}
        <aside class="tables">
          {#if (schema?.groups?.reduce((n, g) => n + g.tables.length, 0) ?? 0) > 14}
            <input class="tfilter" placeholder="filter tables" bind:value={filter} spellcheck="false" />
          {/if}
          <div class="tl">
            {#if err}
              <div class="terr danger selectable">{err}</div>
              <button class="tr faint" onclick={() => app.loadSchema(cn.id, true)}>retry</button>
            {:else if !schema}
              <div class="tr faint">loading…</div>
            {:else}
              {#each schema.groups ?? [] as g}
                {@const tabs = g.tables.filter((t) => t.kind === 'table' && (!filter || t.name.toLowerCase().includes(filter.toLowerCase())))}
                {@const views = g.tables.filter((t) => t.kind === 'view' && (!filter || t.name.toLowerCase().includes(filter.toLowerCase())))}
                <div class="gh grp faint">{g.name}</div>
                {#each tabs as t (t.name)}
                  <button class="tr" class:on={srcTable === t.name} class:hid={hidden(t.name)} onclick={() => app.openTable(cn.id, t.schema, t.name)} title={t.name}>
                    <span class="ellipsis">{t.name}</span>
                    <span class="faint cnt">{hidden(t.name) ? 'hidden from agents' : (t.estimate ? '~' : '') + fmtK(t.rows)}</span>
                  </button>
                {/each}
                {#if views.length}
                  <div class="gh faint" style="padding-top:14px">views</div>
                  {#each views as t (t.name)}
                    <button class="tr" class:on={srcTable === t.name} onclick={() => app.openTable(cn.id, t.schema, t.name)} title={t.name}><span class="ellipsis muted">{t.name}</span></button>
                  {/each}
                {/if}
              {/each}
            {/if}
          </div>
          <button class="reload faint" onclick={() => app.loadSchema(cn.id, true)}><Icon name="refresh" size={14} />reload</button>
        </aside>
        <section class="center">
          {#key cn.id}
            {#if s}<QueryPane session={s} />{/if}
          {/key}
        </section>
        <aside class="right">
          <div class="insp">
            {#if s}<RowInspector conn={cn.id} result={s.result} selected={s.selected} onfollow={follow} />{/if}
          </div>
          <AgentFeed />
        </aside>
      {/if}
    {:else if app.rail === 'mcp'}
      <section class="full"><McpView /></section>
    {:else if app.rail === 'log'}
      <section class="full"><LogView q={app.logQuery} /></section>
    {:else}
      <section class="full"><SettingsView /></section>
    {/if}
  </div>

  <footer>
    {#if cn}
      <span>{host()}</span>
      {#if cn.engine !== 'sqlite'}<span>ssl {cn.sslMode || 'prefer'}</span>{/if}
      <span>{schema?.version ?? ENGINE[cn.engine].label}</span>
    {/if}
    <span class="grow"></span>
    <span>{app.statusLine().toLowerCase()}</span>
    <span>{app.configPath}</span>
  </footer>
</div>

<style>
  .ws { height: 100%; display: flex; flex-direction: column; font: 400 12.5px/1.4 var(--mono); }
  header { height: 38px; flex: none; display: flex; align-items: stretch; border-bottom: 1px solid var(--line); background: var(--sunken); }
  .logo { width: 44px; display: grid; place-items: center; border-right: 1px solid var(--line); font: 600 12px var(--mono); flex: none; }
  .tabs { display: flex; min-width: 0; overflow-x: auto; scrollbar-width: none; }
  .tabs::-webkit-scrollbar { display: none; }
  .tab { display: flex; align-items: center; gap: 8px; padding: 0 14px; border-right: 1px solid var(--line); color: var(--muted); white-space: nowrap; }
  .tab:hover { color: var(--text); }
  .tab.on { background: var(--bg); color: var(--text); box-shadow: inset 0 -2px 0 var(--cc); }
  .sq { width: 7px; height: 7px; flex: none; display: inline-block; }
  .plus { width: 38px; display: grid; place-items: center; font-size: 15px; }
  .plus:hover { color: var(--text); }
  .cell { display: flex; align-items: center; gap: 6px; padding: 0 14px; border-left: 1px solid var(--line); white-space: nowrap; }
  .cell:hover { background: var(--hover); color: var(--text); }
  .main { flex: 1; min-height: 0; display: flex; }
  .rail { width: 44px; flex: none; border-right: 1px solid var(--line); display: flex; flex-direction: column; align-items: center; padding: 8px 0; gap: 4px; color: var(--faint); }
  .rail button { width: 32px; height: 32px; display: grid; place-items: center; }
  .rail button:hover { color: var(--text); }
  .rail button.on { color: var(--text); background: var(--hover); }
  .tables { width: 224px; flex: none; border-right: 1px solid var(--line); display: flex; flex-direction: column; min-height: 0; }
  .tfilter { margin: 8px 8px 2px; height: 26px; border: 1px solid var(--line); background: var(--bg); padding: 0 8px; outline: none; font: inherit; }
  .tfilter:focus { border-color: var(--accent); }
  .tl { flex: 1; overflow: auto; padding: 10px 0; display: flex; flex-direction: column; gap: 1px; }
  .gh { padding: 0 12px 6px; }
  .gh.grp ~ .gh.grp { padding-top: 20px; }
  .tr { display: flex; justify-content: space-between; gap: 8px; padding: 4px 12px; color: var(--muted); text-align: left; min-width: 0; }
  .tr > .cnt { flex: none; font-size: 11.5px; }
  .tr:hover { background: var(--hover); color: var(--text); }
  .tr.on { background: var(--hover); color: var(--text); }
  .tr.hid { color: var(--faint); }
  .terr { padding: 4px 12px 8px; font-size: 11.5px; line-height: 1.45; }
  .reload { display: flex; align-items: center; gap: 6px; padding: 8px 12px; border-top: 1px solid var(--line); }
  .reload:hover { color: var(--text); }
  .center { flex: 1; min-width: 0; display: flex; flex-direction: column; }
  .right { width: 300px; flex: none; border-left: 1px solid var(--line); display: flex; flex-direction: column; min-height: 0; }
  .insp { max-height: 55%; display: flex; flex-direction: column; min-height: 0; border-bottom: 1px solid var(--line); }
  .full { flex: 1; min-width: 0; min-height: 0; }
  footer { height: 24px; flex: none; display: flex; align-items: center; gap: 16px; padding: 0 12px; border-top: 1px solid var(--line); font-size: 11px; color: var(--faint); background: var(--sunken); white-space: nowrap; overflow: hidden; }
  .empty p { margin: 0 0 12px; }
</style>
