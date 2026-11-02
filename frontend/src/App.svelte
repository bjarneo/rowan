<script lang="ts">
  import { onMount } from 'svelte'
  import { app } from './lib/store.svelte'
  import { HUES } from './lib/format'
  import CommandLayout from './layouts/CommandLayout.svelte'
  import WorkspaceLayout from './layouts/WorkspaceLayout.svelte'
  import CommandSurface from './components/CommandSurface.svelte'
  import QueueDrawer from './components/QueueDrawer.svelte'
  import UnlockDialog from './components/UnlockDialog.svelte'
  import ConnectionDialog from './components/ConnectionDialog.svelte'
  import Toast from './components/Toast.svelte'

  onMount(() => {
    app.init()
  })

  $effect(() => {
    const root = document.documentElement
    root.dataset.theme = app.theme
    if (!app.config) return
    root.dataset.layout = app.layout
    root.dataset.density = app.config.app.density
    root.style.setProperty('--acc-h', String(HUES[app.config.app.accent] ?? 290))
  })

  function onKey(e: KeyboardEvent) {
    if (!app.ready) return
    const mod = e.metaKey || e.ctrlKey
    if (mod && e.key.toLowerCase() === 'k') {
      e.preventDefault()
      if (app.layout === 'command' && app.view.kind === 'home' && !app.palette) {
        document.querySelector<HTMLInputElement>('.cs input')?.focus()
        return
      }
      app.palette = !app.palette
      return
    }
    if (e.key === 'Escape') {
      if (app.connDialog) app.connDialog = null
      else if (app.unlockReq) app.unlockReq = null
      else if (app.palette) app.palette = false
      else if (app.queueOpen) app.queueOpen = false
      else if (app.layout === 'command' && app.view.kind !== 'home') app.home()
      else return
      e.preventDefault()
    }
  }
</script>

<svelte:window onkeydown={onKey} />

{#if app.bootError}
  <div class="empty"><div><p>Rowan could not start.</p><p class="danger selectable">{app.bootError}</p></div></div>
{:else if !app.ready}
  <div class="empty faint">Loading…</div>
{:else}
  {#if app.layout === 'workspace'}
    <WorkspaceLayout />
  {:else}
    <CommandLayout />
  {/if}

  {#if app.palette}
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="scrim" onclick={() => (app.palette = false)}></div>
    <div class="palette"><CommandSurface overlay /></div>
  {/if}
  {#if app.queueOpen}<QueueDrawer />{/if}
  {#if app.unlockReq}<UnlockDialog />{/if}
  {#if app.connDialog}<ConnectionDialog />{/if}
  <Toast />
{/if}

<style>
  .palette { position: fixed; z-index: 41; top: 10vh; left: 50%; transform: translateX(-50%); }
</style>
