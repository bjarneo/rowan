<script lang="ts">
  import { app } from '../lib/store.svelte'
  import { api } from '../lib/api'
  import { ENGINE, connColor } from '../lib/format'
  import Icon from './Icon.svelte'

  const req = $derived(app.unlockReq)
  const c = $derived(req ? app.conn(req.conn) : undefined)
  const need = $derived(!!c?.production && app.config.safety.confirmProductionUnlock)
  const defMin = (() => {
    const r = app.config.safety.relockAfter
    if (r === 'relaunch') return 0
    const m = parseInt(r, 10)
    return Number.isFinite(m) && m > 0 ? m : 15
  })()
  let minutes = $state(defMin)
  let typed = $state('')
  let err = $state('')
  let nameInput = $state<HTMLInputElement>()
  let goBtn = $state<HTMLButtonElement>()
  $effect(() => {
    ;(nameInput ?? goBtn)?.focus()
  })
  const ok = $derived(!need || typed.trim() === c?.name)

  async function go() {
    // Read the request before it is cleared: the derived values go away with it.
    const conn = c
    const then = req?.then
    if (!ok || !conn) return
    try {
      await api.unlock(conn.id, minutes, typed)
    } catch (e) {
      err = String(e)
      return
    }
    app.unlockReq = null
    app.toast(`Writes unlocked on ${conn.name}`)
    then?.()
  }

  function close() {
    app.unlockReq = null
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="scrim" onclick={close}></div>
{#if c}
  <div class="dlg" role="dialog" aria-modal="true" aria-label="Unlock writes">
    <div class="hd">
      <span class="ic"><Icon name="lock_open" size={18} /></span>
      <div>
        <div class="t">Unlock writes on {c.name}</div>
        <div class="muted row" style="gap:6px"><span class="dot" style="background:{connColor(c.color)}"></span>{ENGINE[c.engine].label}{c.production ? ' · production' : ''}</div>
      </div>
    </div>
    <p class="muted">Rowan opens every connection read-only. While writes are unlocked, the editor can run INSERT, UPDATE, DELETE and DDL on this connection.</p>
    <div class="lbl">Keep writes unlocked for</div>
    <div class="seg">
      {#each [[5, '5 min'], [15, '15 min'], [30, '30 min'], [60, '1 hour'], [0, 'Until relaunch']] as [m, l]}
        <button class:on={minutes === m} onclick={() => (minutes = m as number)}>{l}</button>
      {/each}
    </div>
    {#if need}
      <div class="lbl">To unlock a production connection, type <span class="mono strong">{c.name}</span></div>
      <input class="input mono" bind:this={nameInput} bind:value={typed} onkeydown={(e) => e.key === 'Enter' && go()} placeholder={c.name} />
    {/if}
    {#if err}<div class="danger">{err}</div>{/if}
    <div class="ft">
      <button class="btn" onclick={close}>Cancel</button>
      <button class="btn go" bind:this={goBtn} disabled={!ok} onclick={go}><Icon name="lock_open" size={15} />Unlock writes</button>
    </div>
  </div>
{/if}

<style>
  .dlg { position: fixed; z-index: 41; top: 18vh; left: 50%; transform: translateX(-50%); width: 460px; background: var(--raised); border: 1px solid var(--line); border-radius: var(--r-xl); box-shadow: var(--shadow); padding: 20px; display: flex; flex-direction: column; gap: 12px; }
  .hd { display: flex; gap: 12px; align-items: center; }
  .ic { width: 34px; height: 34px; border-radius: 9px; display: grid; place-items: center; background: var(--warn-soft); color: var(--warn); }
  :global(:root[data-layout='workspace']) .ic { border-radius: 0; }
  .t { font-weight: 600; font-size: 14.5px; }
  p { margin: 0; line-height: 1.5; font-size: 12.5px; }
  .lbl { font-size: 12px; color: var(--muted); margin-top: 4px; }
  .strong { font-weight: 600; color: var(--text); }
  .ft { display: flex; justify-content: flex-end; gap: 8px; margin-top: 6px; }
  .go { background: var(--warn-soft); border-color: transparent; color: var(--warn); }
</style>
