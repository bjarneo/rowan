<script lang="ts">
  import { app } from '../lib/store.svelte'
</script>

{#if app.toastMsg}
  <div class="toast" role="status">{app.toastMsg}</div>
{/if}

<style>
  .toast {
    position: fixed; z-index: 60; left: 50%; bottom: 44px; transform: translateX(-50%);
    max-width: 640px; padding: 9px 14px; border-radius: var(--r-lg);
    background: var(--text); color: var(--bg); font-size: 12.5px; box-shadow: var(--shadow);
    animation: up 0.16s ease-out;
  }
  @keyframes up { from { opacity: 0; transform: translate(-50%, 6px); } }
</style>
