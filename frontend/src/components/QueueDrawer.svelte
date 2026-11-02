<script lang="ts">
  import { app } from '../lib/store.svelte'
  import { ago, connColor, initials } from '../lib/format'
  import { highlight } from '../lib/sql'
  import Icon from './Icon.svelte'

  let sel = $state<Record<string, boolean>>({})
  const ids = $derived(app.queue.filter((q) => sel[q.id]).map((q) => q.id))
  const all = $derived(app.queue.length > 0 && ids.length === app.queue.length)

  function toggleAll() {
    sel = all ? {} : Object.fromEntries(app.queue.map((q) => [q.id, true]))
  }
  async function decide(list: string[], ok: boolean) {
    if (!list.length) return
    await app.decide(list, ok)
    const next = { ...sel }
    list.forEach((i) => delete next[i])
    sel = next
  }
  function close() {
    app.queueOpen = false
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="scrim" onclick={close}></div>
<aside class="dr" aria-label="Approval queue">
  <div class="hd">
    <Icon name="inbox" size={18} />
    <span class="t">Approval queue</span>
    <span class="count">{app.queue.length}</span>
    <span class="grow"></span>
    <button class="icon-btn" onclick={close} title="Close (Esc)"><Icon name="close" size={18} /></button>
  </div>
  {#if app.queue.length}
    <div class="bar">
      <button class="chk" class:on={ids.length > 0} onclick={toggleAll} aria-label="Select all">
        {#if all}<Icon name="check" size={13} />{:else if ids.length}<Icon name="remove" size={13} />{/if}
      </button>
      <span class="muted">{ids.length ? `${ids.length} selected` : 'Select all'}</span>
      <span class="grow"></span>
      <button class="btn sm" disabled={!ids.length} onclick={() => decide(ids, false)}>Deny</button>
      <button class="btn sm dark" disabled={!ids.length} onclick={() => decide(ids, true)}>Approve</button>
    </div>
  {/if}
  <div class="list">
    {#each app.queue as q (q.id)}
      {@const c = app.conn(q.conn)}
      <div class="item" class:on={sel[q.id]}>
        <div class="top">
          <button class="chk" class:on={sel[q.id]} onclick={() => (sel = { ...sel, [q.id]: !sel[q.id] })} aria-label="Select">
            {#if sel[q.id]}<Icon name="check" size={13} />{/if}
          </button>
          <span class="badge">{initials(q.client)}</span>
          <span class="strong">{q.client}</span>
          <span class="faint">{ago(q.at, app.now)}</span>
          <span class="grow"></span>
          <span class="row muted" style="gap:6px"><span class="dot" style="background:{connColor(c?.color)}"></span>{c?.name ?? q.conn}</span>
        </div>
        {#if q.reason}<div class="reason">{q.reason}</div>{/if}
        <pre class="sql selectable">{@html highlight(q.sql)}</pre>
        {#if q.impact}<div class="impact"><Icon name="bolt" size={14} />{q.impact}</div>{/if}
        {#if q.changes?.length}
          <div class="diff">
            {#each q.changes as d}
              <div><span class="faint mono">{d.column}</span>{#if d.from}<span class="from">{d.from}</span><span class="faint">→</span>{:else}<span class="faint">+</span>{/if}<span class="to">{d.to}</span></div>
            {/each}
          </div>
        {/if}
        <div class="acts">
          <button class="btn sm" onclick={() => decide([q.id], false)}>Deny</button>
          <button class="btn sm dark" onclick={() => decide([q.id], true)}>Approve</button>
        </div>
      </div>
    {:else}
      <div class="none">
        <Icon name="check_circle" size={28} fill="var(--line-strong)" />
        <div>No writes are waiting.</div>
        <div class="faint">When an agent asks to write to a connection that needs approval, the request shows up here.</div>
      </div>
    {/each}
  </div>
  <div class="ft">
    <button class="link" onclick={() => { close(); app.openView('mcp') }}>Agent policy</button>
    <span class="faint">·</span>
    <button class="link" onclick={() => { close(); app.openView('log') }}>Activity log</button>
  </div>
</aside>

<style>
  .dr { position: fixed; z-index: 41; top: 0; right: 0; bottom: 0; width: 460px; background: var(--raised); border-left: 1px solid var(--line); box-shadow: var(--shadow); display: flex; flex-direction: column; }
  .hd { height: 52px; display: flex; align-items: center; gap: 10px; padding: 0 12px 0 18px; border-bottom: 1px solid var(--line); flex: none; }
  .t { font-weight: 600; font-size: 14px; }
  .count { min-width: 20px; height: 20px; padding: 0 6px; border-radius: 10px; background: var(--accent); color: var(--on-accent); font: 600 11px/20px var(--mono); text-align: center; }
  .bar { display: flex; align-items: center; gap: 10px; padding: 10px 18px; border-bottom: 1px solid var(--line); flex: none; }
  .list { flex: 1; overflow: auto; padding: 12px; display: flex; flex-direction: column; gap: 10px; }
  .item { border: 1px solid var(--line); border-radius: var(--r-lg); padding: 12px; display: flex; flex-direction: column; gap: 8px; background: var(--bg); }
  .item.on { border-color: var(--accent); }
  .top { display: flex; align-items: center; gap: 8px; font-size: 12.5px; }
  .badge { width: 22px; height: 22px; border-radius: 5px; display: grid; place-items: center; background: var(--raised); border: 1px solid var(--line); font: 600 9.5px var(--mono); color: var(--muted); }
  .strong { font-weight: 500; }
  .reason { font-size: 13px; line-height: 1.45; }
  .sql { margin: 0; padding: 8px 10px; background: var(--sunken); border-radius: var(--r); font: 400 12px/1.55 var(--mono); white-space: pre-wrap; word-break: break-word; }
  .sql :global(.k) { color: var(--syn-kw); }
  .sql :global(.s) { color: var(--syn-str); }
  .sql :global(.n) { color: var(--syn-num); }
  .impact { display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--muted); }
  .diff { display: flex; flex-direction: column; gap: 4px; font: 400 12px var(--mono); }
  .diff > div { display: flex; gap: 8px; align-items: baseline; flex-wrap: wrap; }
  .from { color: var(--faint); text-decoration: line-through; }
  .to { padding: 1px 6px; border-radius: 4px; background: var(--warn-soft); }
  .acts { display: flex; justify-content: flex-end; gap: 8px; }
  .chk { width: 16px; height: 16px; border-radius: 4px; border: 1.5px solid var(--line-strong); display: grid; place-items: center; color: var(--on-accent); flex: none; }
  .chk.on { background: var(--accent); border-color: var(--accent); }
  :global(:root[data-layout='workspace']) .chk, :global(:root[data-layout='workspace']) .badge, :global(:root[data-layout='workspace']) .to { border-radius: 0; }
  .none { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 48px 24px; text-align: center; line-height: 1.5; }
  .ft { display: flex; gap: 8px; padding: 12px 18px; border-top: 1px solid var(--line); font-size: 12.5px; flex: none; }
  .link { color: var(--accent-text); }
  .link:hover { text-decoration: underline; }
</style>
