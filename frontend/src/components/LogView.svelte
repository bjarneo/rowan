<script lang="ts">
  import { app } from '../lib/store.svelte'
  import { clock, ago, connColor, fmtN, fmtMs, initials } from '../lib/format'
  import { highlight } from '../lib/sql'

  let { q: initial = '' }: { q?: string } = $props()

  let filter = $state<'all' | 'reads' | 'writes' | 'blocked'>('all')
  let client = $state('all')
  let q = $state('')
  let open = $state<string | null>(null)
  $effect.pre(() => {
    q = initial
  })

  const READS = ['query', 'list_tables', 'describe_table', 'list_connections', 'explain']
  const clients = $derived([...new Set(app.log.map((l) => l.client))].sort())
  const list = $derived.by(() => {
    const s = q.trim().toLowerCase()
    return app.log.filter(
      (l) =>
        (filter === 'all' || (filter === 'reads' && READS.includes(l.tool)) || (filter === 'writes' && l.tool === 'execute') || (filter === 'blocked' && (l.status === 'blocked' || l.status === 'denied'))) &&
        (client === 'all' || l.client === client) &&
        (!s || `${l.sql} ${app.conn(l.conn)?.name ?? l.conn} ${l.client} ${l.tool}`.toLowerCase().includes(s)),
    )
  })
  const shown = $derived(list.slice(0, 400))
  const ST: Record<string, [string, string, string]> = {
    allowed: ['var(--muted)', 'var(--sunken)', 'Allowed'],
    truncated: ['var(--warn)', 'var(--warn-soft)', 'Truncated'],
    approved: ['var(--ok)', 'var(--ok-soft)', 'Approved'],
    denied: ['var(--danger)', 'var(--danger-soft)', 'Denied'],
    blocked: ['var(--danger)', 'var(--danger-soft)', 'Blocked'],
    error: ['var(--danger)', 'var(--danger-soft)', 'Error'],
  }
</script>

<div class="lv">
  <div class="bar">
    <h1>Activity log</h1>
    <div class="seg">
      {#each [['all', 'All'], ['reads', 'Reads'], ['writes', 'Writes'], ['blocked', 'Blocked']] as [k, l]}
        <button class:on={filter === k} onclick={() => (filter = k as typeof filter)}>{l}</button>
      {/each}
    </div>
    <select class="input" style="width:160px;height:28px" bind:value={client}>
      <option value="all">All clients</option>
      {#each clients as c}<option value={c}>{c}</option>{/each}
    </select>
    <input class="input" style="width:240px;height:28px" placeholder="Filter by SQL, table or client" bind:value={q} />
    <span class="grow"></span>
    <span class="faint">{fmtN(list.length)} events{list.length > shown.length ? ', newest 400 shown' : ''}</span>
  </div>
  <div class="tbl">
    <div class="tr th">
      <span>Time</span><span>Client</span><span>Tool</span><span>Connection</span><span>Statement</span><span class="num">Rows</span><span class="num">Time</span><span>Outcome</span>
    </div>
    {#each shown as l (l.id)}
      {@const st = ST[l.status] ?? ST.allowed}
      {@const c = app.conn(l.conn)}
      <button class="tr" class:open={open === l.id} onclick={() => (open = open === l.id ? null : l.id)}>
        <span class="mono" title={ago(l.at, app.now)}>{clock(l.at)}</span>
        <span class="row" style="gap:6px"><span class="badge">{initials(l.client)}</span><span class="ellipsis">{l.client}</span></span>
        <span class="mono">{l.tool}</span>
        <span class="row ellipsis" style="gap:6px">{#if c}<span class="dot" style="background:{connColor(c.color)}"></span>{/if}{c?.name ?? (l.conn || '—')}</span>
        <span class="mono ellipsis muted">{l.sql || '—'}</span>
        <span class="num mono">{l.status === 'blocked' || l.status === 'denied' ? '—' : fmtN(l.rows)}</span>
        <span class="num mono faint">{fmtMs(l.ms)}</span>
        <span><span class="chip" style="color:{st[0]};background:{st[1]}">{st[2]}</span></span>
      </button>
      {#if open === l.id}
        <div class="detail">
          {#if l.reason}<div><span class="faint">Reason</span>{l.reason}</div>{/if}
          <div><span class="faint">Outcome</span>{l.note || 'Allowed by the connection policy.'}</div>
          {#if l.sql}<pre class="selectable">{@html highlight(l.sql)}</pre>{/if}
        </div>
      {/if}
    {:else}
      <div class="empty" style="height:auto;padding:48px">No agent calls match.</div>
    {/each}
  </div>
</div>

<style>
  .lv { display: flex; flex-direction: column; height: 100%; min-height: 0; }
  .bar { display: flex; align-items: center; gap: 10px; padding: 14px 20px; border-bottom: 1px solid var(--line); flex: none; }
  h1 { margin: 0 8px 0 0; font: 600 16px var(--sans); }
  :global(:root[data-layout='workspace']) h1 { font: 600 13px var(--mono); }
  .tbl { flex: 1; overflow: auto; }
  .tr { display: grid; grid-template-columns: 76px 150px 116px 140px minmax(0, 1fr) 70px 76px 100px; gap: 12px; align-items: center; min-height: 36px; padding: 0 20px; border-bottom: 1px solid var(--line); width: 100%; text-align: left; font-size: 12.5px; }
  .tr:not(.th):hover { background: var(--hover); }
  .tr.open { background: var(--sunken); }
  .th { position: sticky; top: 0; background: var(--panel); color: var(--faint); font-weight: 500; font-size: 12px; z-index: 1; min-height: 32px; }
  .num { text-align: right; }
  .badge { width: 20px; height: 20px; border-radius: 5px; display: grid; place-items: center; background: var(--raised); border: 1px solid var(--line); font: 600 9px var(--mono); color: var(--muted); flex: none; }
  :global(:root[data-layout='workspace']) .badge { border-radius: 0; }
  .detail { padding: 10px 20px 14px 108px; background: var(--sunken); border-bottom: 1px solid var(--line); display: flex; flex-direction: column; gap: 6px; font-size: 12.5px; }
  .detail > div { display: grid; grid-template-columns: 80px 1fr; }
  pre { margin: 4px 0 0; padding: 8px 10px; background: var(--bg); border: 1px solid var(--line); border-radius: var(--r); font: 400 12px/1.55 var(--mono); white-space: pre-wrap; }
  pre :global(.k) { color: var(--syn-kw); }
  pre :global(.s) { color: var(--syn-str); }
  pre :global(.n) { color: var(--syn-num); }
</style>
