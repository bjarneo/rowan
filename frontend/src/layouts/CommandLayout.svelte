<script lang="ts">
  // Layout 2a: command-first. No permanent sidebar. Everything opens from
  // one search surface with a live preview.
  import { app } from '../lib/store.svelte'
  import { ENGINE, connColor, mmss, MOD } from '../lib/format'
  import type { Link } from '../lib/types'
  import CommandSurface from '../components/CommandSurface.svelte'
  import QueryPane from '../components/QueryPane.svelte'
  import RowInspector from '../components/RowInspector.svelte'
  import LogView from '../components/LogView.svelte'
  import McpView from '../components/McpView.svelte'
  import SettingsView from '../components/SettingsView.svelte'
  import Icon from '../components/Icon.svelte'
  import TableView from './TableView.svelte'

  let menu = $state(false)
  const cn = $derived(app.active)
  const unl = $derived(cn ? app.unlocked(cn.id) : false)
  const until = $derived(cn ? app.locks[cn.id] : 0)
  const v = $derived(app.view)
  const ver = $derived(cn ? app.schemas[cn.id]?.version ?? ENGINE[cn.engine].label : '')

  const title = $derived(
    v.kind === 'table' ? v.table : v.kind === 'query' ? 'Query' : v.kind === 'log' ? 'Activity log' : v.kind === 'mcp' ? 'MCP server' : v.kind === 'settings' ? 'Settings' : '',
  )

  function lockToggle() {
    if (!cn) return
    if (unl) app.relock(cn.id)
    else app.requireWrite(cn.id)
  }

  function follow(l: Link) {
    if (v.kind !== 'query') return
    app.openTable(v.conn, l.schema, l.table, { sql: l.sql.replace(/;\s*$/, '') })
  }
</script>

<div class="cmd">
  <header>
    <div class="logo">R</div>
    <span class="brand">Rowan</span>
    <div class="pillwrap">
      <button class="pill" onclick={() => (menu = !menu)} title="Switch connection">
        {#if cn}
          <span class="dot" style="background:{connColor(cn.color)}"></span>{cn.name}<span class="mono faint" style="font-size:11px">{ENGINE[cn.engine].short}</span>
        {:else}
          <span class="faint">No connection</span>
        {/if}
        <Icon name="expand_more" size={15} fill="var(--faint)" />
      </button>
      {#if menu}
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div class="menuscrim" onclick={() => (menu = false)}></div>
        <div class="menu">
          {#each app.conns as c (c.id)}
            <button class:on={c.id === app.activeConn} onclick={() => { app.setActive(c.id); menu = false }}>
              <span class="dot" style="background:{connColor(c.color)}"></span><span class="grow">{c.name}</span><span class="mono faint">{ENGINE[c.engine].short}</span>
            </button>
          {/each}
          <div class="sep"></div>
          <button onclick={() => { menu = false; app.connDialog = { id: null } }}><Icon name="add" size={16} /><span class="grow">New connection</span></button>
          {#if cn}<button onclick={() => { menu = false; app.connDialog = { id: cn.id } }}><Icon name="edit" size={16} /><span class="grow">Edit {cn.name}</span></button>{/if}
        </div>
      {/if}
    </div>
    {#if v.kind !== 'home'}
      <button class="back" onclick={() => app.home()} title="Back to search (Esc)"><Icon name="arrow_back" size={15} />Search</button>
      <span class="crumb faint">/</span>
      <span class="crumb ellipsis" class:mono={v.kind === 'table'}>{title}</span>
    {/if}
    <span class="grow"></span>
    <button class="icon-btn" onclick={() => (app.palette = true)} title="Search ({MOD}K)"><Icon name="search" size={17} /></button>
    <button class="icon-btn" onclick={() => app.toggleLayout()} title="Switch to the workspace layout"><Icon name="space_dashboard" size={17} /></button>
    <button class="icon-btn" onclick={() => app.toggleTheme()} title="Toggle light and dark"><Icon name={app.theme === 'dark' ? 'light_mode' : 'dark_mode'} size={17} /></button>
    {#if cn}
      <button class="tbtn" class:unl onclick={lockToggle} title={unl ? `Relock ${cn.name}` : `${cn.name} is read-only. Click to unlock writes.`}>
        <Icon name={unl ? 'lock_open' : 'lock'} size={16} />{unl ? (until === -1 ? 'Writes unlocked' : `Writes unlocked · ${mmss(until - app.now)}`) : 'Read-only'}
      </button>
    {/if}
    <button class="tbtn" class:on={app.queueOpen} onclick={() => (app.queueOpen = !app.queueOpen)} title="Approval queue">
      <span class="dot" style="background:{app.mcp.running ? 'var(--ok)' : 'var(--line-strong)'}"></span>MCP
      {#if app.queue.length}<span class="count">{app.queue.length}</span>{/if}
    </button>
  </header>

  <main>
    {#if v.kind === 'home'}
      <div class="home"><CommandSurface /></div>
    {:else}
      <div class="view">
        <div class="content">
          {#if v.kind === 'table'}
            {#key v}<TableView view={v} />{/key}
          {:else if v.kind === 'query'}
            {@const s = app.session(v.conn)}
            <div class="qv">
              <div class="qmain"><QueryPane session={s} editorHeight={240} autofocus /></div>
              <aside class="side"><RowInspector conn={v.conn} result={s.result} selected={s.selected} onfollow={follow} /></aside>
            </div>
          {:else if v.kind === 'log'}
            <LogView q={v.q} />
          {:else if v.kind === 'mcp'}
            <McpView />
          {:else if v.kind === 'settings'}
            <SettingsView />
          {/if}
        </div>
      </div>
    {/if}
  </main>

  <footer>
    {#if cn}<span>{cn.name} · {ver} · {unl ? 'writes unlocked' : 'read-only'}</span>{/if}
    <span class="grow"></span>
    <span>{app.statusLine()}</span>
  </footer>
</div>

<style>
  .cmd { height: 100%; display: flex; flex-direction: column; }
  header { height: 44px; flex: none; display: flex; align-items: center; gap: 12px; padding: 0 10px 0 16px; border-bottom: 1px solid var(--line); }
  .logo { width: 20px; height: 20px; border-radius: 5px; background: var(--text); color: var(--bg); display: grid; place-items: center; font: 600 11px/1 var(--mono); flex: none; }
  .brand { font: 600 13.5px var(--sans); letter-spacing: -0.01em; }
  .pillwrap { position: relative; }
  .pill { display: flex; align-items: center; gap: 7px; height: 26px; padding: 0 6px 0 10px; border: 1px solid var(--line); border-radius: 13px; font-size: 12.5px; }
  .pill:hover { border-color: var(--line-strong); }
  .menuscrim { position: fixed; inset: 0; z-index: 30; }
  .menu { position: absolute; z-index: 31; top: 32px; left: 0; min-width: 240px; padding: 5px; background: var(--raised); border: 1px solid var(--line); border-radius: var(--r-lg); box-shadow: var(--shadow); }
  .menu button { width: 100%; display: flex; align-items: center; gap: 9px; height: 32px; padding: 0 9px; border-radius: 6px; font-size: 12.5px; }
  .menu button:hover, .menu button.on { background: var(--hover); }
  .menu .sep { height: 1px; background: var(--line); margin: 5px 0; }
  .crumb { font-weight: 500; min-width: 0; }
  .crumb.mono { font: 500 13px var(--mono); }
  .tbtn { display: flex; align-items: center; gap: 6px; height: 28px; padding: 0 10px; border: 1px solid var(--line); border-radius: 7px; font: 500 12px var(--sans); color: var(--muted); white-space: nowrap; }
  .tbtn:hover { border-color: var(--line-strong); }
  .tbtn.unl { color: var(--warn); background: var(--warn-soft); border-color: transparent; }
  .tbtn.on { background: var(--hover); }
  .tbtn:last-child { color: var(--text); padding-right: 8px; }
  .count { min-width: 18px; height: 18px; padding: 0 5px; border-radius: 9px; background: var(--accent); color: var(--on-accent); font: 600 10.5px/18px var(--mono); text-align: center; }
  main { flex: 1; min-height: 0; }
  .home { height: 100%; overflow: auto; display: flex; justify-content: center; padding-top: 64px; padding-bottom: 32px; }
  .view { height: 100%; display: flex; flex-direction: column; }
  .back { display: flex; align-items: center; gap: 4px; height: 26px; padding: 0 8px 0 6px; border-radius: 6px; color: var(--muted); font-size: 12.5px; }
  .back:hover { background: var(--hover); color: var(--text); }
  .content { flex: 1; min-height: 0; }
  .qv { height: 100%; display: flex; }
  .qmain { flex: 1; min-width: 0; }
  .side { width: 320px; flex: none; border-left: 1px solid var(--line); display: flex; flex-direction: column; min-height: 0; overflow: auto; }
  footer { height: 26px; flex: none; display: flex; align-items: center; gap: 14px; padding: 0 14px; border-top: 1px solid var(--line); font-size: 11.5px; color: var(--muted); }
</style>
