<script lang="ts">
  // The table view of the command layout: a full-height grid with the
  // filters from search, and the row inspector on the right.
  import { onMount } from 'svelte'
  import { app, Session } from '../lib/store.svelte'
  import { api } from '../lib/api'
  import { fmtN, fmtMs, MOD, plural } from '../lib/format'
  import type { Link } from '../lib/types'
  import Grid from '../components/Grid.svelte'
  import RowInspector from '../components/RowInspector.svelte'
  import Icon from '../components/Icon.svelte'

  let { view }: { view: { conn: string; schema: string; table: string; rest: string; sql: string } } = $props()

  // The parent recreates this component for each view, so the props are read once.
  // svelte-ignore state_referenced_locally
  const session = new Session(view.conn, '')
  // svelte-ignore state_referenced_locally
  let rest = $state(view.rest)
  // svelte-ignore state_referenced_locally
  let sql = $state(view.sql)

  onMount(() => {
    session.run({ sql })
  })

  const res = $derived(session.result)
  const where = $derived(/\bwhere\b([\s\S]+)$/i.exec(sql)?.[1].trim() ?? '')
  const total = $derived(app.tables(view.conn).find((t) => t.name === view.table && t.schema === view.schema))

  async function applyFilter() {
    const p = await api.preview({ conn: view.conn, schema: view.schema, table: view.table, rest, limit: 1 })
    if (p.error) {
      app.toast(p.error)
      return
    }
    sql = p.sql
    if (p.interp.unresolved?.length) app.toast(p.interp.note)
    session.run({ sql })
  }

  function follow(l: Link) {
    app.openTable(view.conn, l.schema, l.table, { sql: l.sql.replace(/;\s*$/, '') })
  }

  async function exportAs(fmt: string) {
    if (!res?.id) return
    const p = await api.exportResult(res.id, fmt).catch((e) => (app.toast(String(e)), ''))
    if (p) app.toast(`Exported ${plural(res.rows?.length ?? 0, 'row')} to ${p}`)
  }

  function onKey(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.preventDefault()
      app.openQuery(view.conn, sql + ';', true)
    }
  }
</script>

<svelte:window onkeydown={onKey} />

<div class="tv">
  <div class="hdr">
    <span class="mono strong">{view.table}</span>
    <div class="filter">
      <Icon name="filter_alt" size={15} fill="var(--faint)" />
      <input class="mono" bind:value={rest} placeholder="status:refunded total_cents>5000" onkeydown={(e) => e.key === 'Enter' && applyFilter()} spellcheck="false" />
    </div>
    <span class="faint ellipsis">
      {#if session.running}Loading…
      {:else if res?.error}<span class="danger">{res.error}</span>
      {:else if res}
        {where ? `where ${where} · ` : ''}{res.truncated ? `first ${fmtN(res.rows?.length ?? 0)} rows` : plural(res.rows?.length ?? 0, 'row')}{total ? ` · ${total.estimate ? '~' : ''}${fmtN(total.rows)} in table` : ''} · {fmtMs(res.ms)}
      {/if}
    </span>
    <span class="grow"></span>
    {#if res?.id}
      <button class="btn ghost sm" onclick={() => exportAs('csv')}>CSV</button>
      <button class="btn ghost sm" onclick={() => exportAs('json')}>JSON</button>
      <button class="btn ghost sm" onclick={() => exportAs('sql')}>SQL</button>
    {/if}
    <button class="btn ghost sm" onclick={() => session.run({ sql })} title="Reload"><Icon name="refresh" size={15} /></button>
    <button class="btn sm" onclick={() => app.openQuery(view.conn, sql + ';', true)}>Open as query <span class="kbd">{MOD}↵</span></button>
  </div>
  <div class="body">
    <div class="grid">
      {#if res?.columns}
        <Grid columns={res.columns} rows={res.rows ?? []} bind:selected={session.selected} mono={true} />
      {:else if !session.running}
        <div class="empty">{res?.error ? 'The query failed.' : ''}</div>
      {/if}
    </div>
    <aside class="side">
      <RowInspector conn={view.conn} result={res} selected={session.selected} onfollow={follow} />
    </aside>
  </div>
</div>

<style>
  .tv { display: flex; flex-direction: column; height: 100%; min-height: 0; }
  .hdr { height: 48px; flex: none; display: flex; align-items: center; gap: 12px; padding: 0 12px 0 16px; border-bottom: 1px solid var(--line); min-width: 0; }
  .strong { font-weight: 600; font-size: 14px; }
  .filter { display: flex; align-items: center; gap: 6px; height: 28px; padding: 0 8px; border: 1px solid var(--line); border-radius: var(--r); background: var(--raised); width: 300px; flex: none; }
  .filter:focus-within { border-color: var(--accent); }
  .filter input { border: 0; outline: none; background: transparent; flex: 1; min-width: 0; font-size: 12.5px; }
  .body { flex: 1; min-height: 0; display: flex; }
  .grid { flex: 1; min-width: 0; }
  .side { width: 320px; flex: none; border-left: 1px solid var(--line); display: flex; flex-direction: column; min-height: 0; overflow: auto; }
</style>
