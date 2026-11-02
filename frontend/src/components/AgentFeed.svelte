<script lang="ts">
  import { app } from '../lib/store.svelte'
  import { clock, fmtN, fmtMs, plural } from '../lib/format'

  let { limit = 40 }: { limit?: number } = $props()

  const items = $derived(app.log.slice(0, limit))
  const name = (id: string) => app.conn(id)?.name ?? id
  const one = (s: string) => s.replace(/\s+/g, ' ').trim()

  function outcome(e: (typeof app.log)[number]) {
    switch (e.status) {
      case 'allowed':
        return { t: e.tool === 'list_tables' || e.tool === 'list_connections' || e.tool === 'describe_table' ? `${plural(e.rows, 'item')} · ${fmtMs(e.ms)}` : `${plural(e.rows, 'row')} · ${fmtMs(e.ms)}`, c: 'var(--ok)' }
      case 'truncated':
        return { t: `${fmtN(e.rows)} rows · capped`, c: 'var(--warn)' }
      case 'approved':
        return { t: `approved · ${plural(e.rows, 'row')} changed`, c: 'var(--ok)' }
      case 'denied':
        return { t: 'denied · ' + e.note.toLowerCase(), c: 'var(--danger)' }
      case 'blocked':
        return { t: 'blocked · ' + e.note.replace(/^.*not in the allowlist.*$/, 'not in allowlist').toLowerCase(), c: 'var(--danger)' }
      default:
        return { t: 'error · ' + e.note, c: 'var(--danger)' }
    }
  }
</script>

<div class="feed">
  <div class="hd faint">
    <span class="dot" style="background:{app.mcp.running ? 'var(--ok)' : 'var(--line-strong)'};width:6px;height:6px"></span>
    {app.layout === 'workspace' ? 'agent feed' : 'Agent feed'} · {app.mcp.running ? 'live' : 'server off'}
  </div>
  <div class="list">
    {#each app.queue as q (q.id)}
      <button class="it" onclick={() => (app.queueOpen = true)}>
        <span class="faint ellipsis">{clock(q.at)} · {q.client.toLowerCase()} · execute · {name(q.conn)}</span>
        <span class="muted ellipsis">{one(q.sql)}</span>
        <span class="accent">queued for approval</span>
      </button>
    {/each}
    {#each items as e (e.id)}
      {@const o = outcome(e)}
      <button class="it" onclick={() => app.openView('log', e.sql ? one(e.sql).slice(0, 40) : '')}>
        <span class="faint ellipsis">{clock(e.at)} · {e.client.toLowerCase()} · {e.tool}{e.conn ? ' · ' + name(e.conn) : ''}</span>
        {#if e.sql}<span class="muted ellipsis">{one(e.sql)}</span>{/if}
        <span class="ellipsis" style="color:{o.c}">{o.t}</span>
      </button>
    {:else}
      {#if !app.queue.length}
        <div class="none faint">No agent calls yet. Connect a client from the MCP server view.</div>
      {/if}
    {/each}
  </div>
</div>

<style>
  .feed { display: flex; flex-direction: column; min-height: 0; flex: 1; }
  .hd { padding: 10px 14px 6px; display: flex; align-items: center; gap: 8px; flex: none; }
  .list { overflow: auto; flex: 1; min-height: 0; font-size: 11.5px; }
  .it { width: 100%; padding: 6px 14px; display: flex; flex-direction: column; gap: 2px; border-top: 1px solid var(--line); text-align: left; min-width: 0; }
  .it:hover { background: var(--hover); }
  .it > span { display: block; max-width: 100%; }
  .none { padding: 10px 14px; line-height: 1.5; }
</style>
