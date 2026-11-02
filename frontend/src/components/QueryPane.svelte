<script lang="ts">
  import type { Session } from '../lib/store.svelte'
  import { app } from '../lib/store.svelte'
  import { api } from '../lib/api'
  import { formatSQL } from '../lib/sql'
  import { fmtN, fmtMs, MOD, plural } from '../lib/format'
  import SqlEditor from './SqlEditor.svelte'
  import Grid from './Grid.svelte'
  import Icon from './Icon.svelte'

  let { session, editorHeight = 210, autofocus = false }: { session: Session; editorHeight?: number; autofocus?: boolean } = $props()

  let edH = $state(0)
  $effect.pre(() => {
    if (!edH) edH = editorHeight
  })
  const ws = $derived(app.layout === 'workspace')
  const conn = $derived(app.conn(session.conn))
  const res = $derived(session.result)
  const hasGrid = $derived(!!res && !!res.columns && !res.error)
  const limits = [100, 500, 1000, 5000, 10000]
  const completion = $derived(conn ? app.completion(conn.id, conn.engine) : null)

  function startResize(e: PointerEvent) {
    const y0 = e.clientY
    const h0 = edH
    const el = e.currentTarget as HTMLElement
    el.setPointerCapture(e.pointerId)
    const move = (ev: PointerEvent) => (edH = Math.max(72, Math.min(window.innerHeight - 240, h0 + ev.clientY - y0)))
    const up = () => {
      el.removeEventListener('pointermove', move)
      el.removeEventListener('pointerup', up)
    }
    el.addEventListener('pointermove', move)
    el.addEventListener('pointerup', up)
  }

  async function exportAs(fmt: string) {
    if (!res?.id) return
    try {
      const p = await api.exportResult(res.id, fmt)
      if (p) app.toast(`Exported ${plural(res.rows?.length ?? 0, 'row')} to ${p}`)
    } catch (e) {
      app.toast(String(e))
    }
  }

  async function saveSnippet() {
    const sql = session.sql.trim()
    if (!sql) return
    const m = /^--\s*(.+)/.exec(sql)
    const name = m ? m[1].trim() : sql.replace(/\s+/g, ' ').slice(0, 48)
    app.config = await api.saveSnippet(name, sql)
    app.toast('Saved to saved queries')
  }

  function format() {
    session.setSQL(formatSQL(session.sql))
  }

  async function setLimit(n: number) {
    await app.saveApp({ rowLimit: n })
  }
</script>

<div class="qp" class:ws>
  <div class="tb">
    {#if session.running}
      <button class="act stop" onclick={() => session.cancel()}><Icon name="stop" size={15} />{ws ? 'stop' : 'Stop'}</button>
    {:else}
      <button class="act run" onclick={() => session.run()} title="Run ({MOD}↵). Runs the selection when there is one."><Icon name="play_arrow" size={16} />{ws ? 'run' : 'Run'}</button>
    {/if}
    <span class="kbd">{MOD}↵</span>
    <span class="sep"></span>
    <button class="act" onclick={format}>{ws ? 'format' : 'Format'}</button>
    <button class="act" onclick={() => session.explain()}>{ws ? 'explain' : 'Explain'}</button>
    <button class="act" onclick={saveSnippet} title="Save as a saved query">{ws ? 'save' : 'Save'}</button>
    <span class="grow"></span>
    {#if conn}
      {#if app.unlocked(conn.id)}
        <span class="lockhint warn"><Icon name="lock_open" size={14} />{ws ? 'writes unlocked' : 'Writes unlocked'}</span>
      {:else}
        <span class="lockhint faint"><Icon name="lock" size={14} />{ws ? 'read-only' : 'Read-only'}</span>
      {/if}
    {/if}
    <select class="limit" value={app.config.app.rowLimit} onchange={(e) => setLimit(+e.currentTarget.value)} title="Rows fetched per query">
      {#each limits as l}<option value={l}>limit {fmtN(l)}</option>{/each}
    </select>
  </div>

  <div class="editor" style="height:{edH}px">
    <SqlEditor
      value={session.sql}
      onchange={(v) => session.setSQL(v)}
      onrun={() => session.run()}
      onselection={(s) => (session.selection = s)}
      hashComments={conn?.engine === 'mysql'}
      {completion}
      onfocus={() => conn && app.prepareCompletion(conn.id)}
      {autofocus}
      placeholder="Write SQL. Press {MOD}↵ to run."
    />
  </div>
  <div class="resize" onpointerdown={startResize} role="separator" aria-orientation="horizontal"></div>

  <div class="status">
    {#if session.running}
      <span class="muted row gap"><Icon name="hourglass_top" size={14} />{ws ? 'running…' : 'Running…'}</span>
    {:else if !res}
      {#if !ws}<span class="faint">Press {MOD}↵ to run. Results are capped at {fmtN(app.config.app.rowLimit)} rows.</span>{/if}
    {:else if res.locked}
      <span class="warn row gap"><Icon name="lock" size={14} />{res.error}</span>
      <button class="btn sm" onclick={() => app.requireWrite(session.conn, () => session.run({ sql: session.ranSQL }))}>Unlock writes</button>
    {:else if res.confirm}
      <span class="warn row gap"><Icon name="warning" size={14} />{res.confirm}</span>
      <button class="btn sm" onclick={() => session.run({ force: true, sql: session.ranSQL })}>Run anyway</button>
    {:else if res.error}
      <span class="danger row gap ellipsis selectable" title={res.error}><Icon name="error" size={14} />{res.error}</span>
    {:else}
      <span class="ok">{ws ? 'ok' : 'OK'}</span>
      {#if session.label}<span class="muted">{session.label}</span>{/if}
      {#if res.columns}
        <span>{plural(res.rows?.length ?? 0, 'row')}{res.truncated ? ' · more available' : ''}</span>
      {/if}
      {#if res.message}<span class="muted">{res.message}</span>{/if}
      <span class="muted">{fmtMs(res.ms)}</span>
    {/if}
    <span class="grow"></span>
    {#if hasGrid && res?.id}
      <button class="act" onclick={() => exportAs('csv')}>{ws ? 'csv' : 'CSV'}</button>
      <button class="act" onclick={() => exportAs('json')}>{ws ? 'json' : 'JSON'}</button>
      <button class="act" onclick={() => exportAs('sql')}>{ws ? 'sql' : 'SQL'}</button>
    {/if}
  </div>

  <div class="results">
    {#if hasGrid && res}
      <Grid columns={res.columns ?? []} rows={res.rows ?? []} bind:selected={session.selected} />
    {:else}
      <div class="empty">{res && !res.error && !res.confirm && !res.locked ? 'Statement executed.' : res ? 'Nothing was run.' : 'Results appear here.'}</div>
    {/if}
  </div>
</div>

<style>
  .qp { display: flex; flex-direction: column; height: 100%; min-height: 0; min-width: 0; }
  .tb { height: 34px; flex: none; display: flex; align-items: center; gap: 12px; padding: 0 12px; border-bottom: 1px solid var(--line); color: var(--muted); }
  .qp:not(.ws) .tb { height: 42px; gap: 8px; padding: 0 12px; }
  .act { display: inline-flex; align-items: center; gap: 4px; color: var(--muted); height: 26px; padding: 0 2px; }
  .qp:not(.ws) .act { padding: 0 8px; border-radius: var(--r); font-weight: 500; font-size: 12.5px; }
  .act:hover { color: var(--text); }
  .qp:not(.ws) .act:hover { background: var(--hover); }
  .act.run { color: var(--accent-text); }
  .qp:not(.ws) .act.run { background: var(--accent); color: var(--on-accent); padding: 0 12px 0 8px; }
  .qp:not(.ws) .act.run:hover { background: var(--accent); filter: brightness(1.05); }
  .act.stop { color: var(--danger); }
  .sep { width: 1px; height: 14px; background: var(--line-strong); }
  .lockhint { display: inline-flex; align-items: center; gap: 4px; font-size: 12px; }
  .limit { background: transparent; border: 0; color: var(--faint); font: inherit; font-size: 12px; cursor: pointer; }
  .editor { flex: none; min-height: 72px; }
  .resize { height: 5px; margin-top: -3px; flex: none; cursor: row-resize; position: relative; z-index: 3; border-bottom: 1px solid var(--line); }
  .resize:hover { background: var(--accent-soft); }
  .status { height: 30px; flex: none; display: flex; align-items: center; gap: 12px; padding: 0 12px; border-bottom: 1px solid var(--line); color: var(--muted); font-size: 12px; min-width: 0; }
  .status .gap { gap: 6px; }
  .results { flex: 1; min-height: 0; }
</style>
