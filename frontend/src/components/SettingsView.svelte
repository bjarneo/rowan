<script lang="ts">
  import { app } from '../lib/store.svelte'
  import { HUES } from '../lib/format'
  import type { AppSettings } from '../lib/types'

  const a = $derived(app.config.app)
  const s = $derived(app.config.safety)
  const seg = <K extends keyof AppSettings>(k: K, v: AppSettings[K]) => () => app.saveApp({ [k]: v } as Partial<AppSettings>)
</script>

<div class="sv">
  <div class="inner">
    <h1>Settings</h1>
    <p class="muted">Rowan saves every change to <span class="mono">{app.configPath}</span>.</p>

    <section class="card">
      <div class="sh">Display</div>
      <div class="r"><span>Layout</span><div class="seg">
        <button class:on={a.layout === 'command'} onclick={seg('layout', 'command')}>Command</button>
        <button class:on={a.layout === 'workspace'} onclick={seg('layout', 'workspace')}>Workspace</button>
      </div><span class="faint">Command opens everything from one search surface. Workspace keeps tabs, tables, the editor and the inspector on screen.</span></div>
      <div class="r"><span>Theme</span><div class="seg">
        {#each ['system', 'light', 'dark'] as t}<button class:on={a.theme === t} onclick={seg('theme', t as AppSettings['theme'])}>{t[0].toUpperCase() + t.slice(1)}</button>{/each}
      </div><span></span></div>
      <div class="r"><span>Accent</span><div class="row" style="gap:10px">
        {#each Object.entries(HUES) as [k, h]}
          <button class="sw" class:on={a.accent === k} style="--c:oklch(0.6 0.15 {h})" onclick={seg('accent', k as AppSettings['accent'])} aria-label={k}></button>
        {/each}
      </div><span></span></div>
      <div class="r"><span>Density</span><div class="seg">
        <button class:on={a.density === 'compact'} onclick={seg('density', 'compact')}>Compact</button>
        <button class:on={a.density === 'balanced'} onclick={seg('density', 'balanced')}>Balanced</button>
      </div><span></span></div>
    </section>

    <section class="card">
      <div class="sh">Queries</div>
      <div class="r"><span>Row limit</span>
        <select class="input" style="width:140px" value={a.rowLimit} onchange={(e) => app.saveApp({ rowLimit: +e.currentTarget.value })}>
          {#each [100, 500, 1000, 5000, 10000, 50000] as n}<option value={n}>{n.toLocaleString('en-US')} rows</option>{/each}
        </select><span class="faint">Rowan stops reading after this many rows and cancels the rest on the server.</span></div>
      <div class="r"><span>Statement timeout</span>
        <select class="input" style="width:140px" value={s.statementTimeout} onchange={(e) => app.saveSafety({ statementTimeout: e.currentTarget.value })}>
          {#each [['15s', '15 seconds'], ['30s', '30 seconds'], ['60s', '1 minute'], ['5m', '5 minutes'], ['30m', '30 minutes']] as [v, l]}<option value={v}>{l}</option>{/each}
        </select><span></span></div>
    </section>

    <section class="card">
      <div class="sh">Safety</div>
      <div class="r"><span>Relock writes after</span>
        <select class="input" style="width:140px" value={s.relockAfter} onchange={(e) => app.saveSafety({ relockAfter: e.currentTarget.value })}>
          {#each [['5m', '5 minutes'], ['15m', '15 minutes'], ['30m', '30 minutes'], ['60m', '1 hour'], ['relaunch', 'Relaunch']] as [v, l]}<option value={v}>{l}</option>{/each}
        </select><span class="faint">The default in the unlock prompt. Every connection opens read-only.</span></div>
      <div class="r"><span>Confirm production unlock</span>
        <button class="switch" class:on={s.confirmProductionUnlock} onclick={() => app.saveSafety({ confirmProductionUnlock: !s.confirmProductionUnlock })} aria-label="Confirm production unlock"></button>
        <span class="faint">Type the connection name to unlock a production connection.</span></div>
      <div class="r"><span>Confirm destructive statements</span>
        <button class="switch" class:on={s.confirmDestructive} onclick={() => app.saveSafety({ confirmDestructive: !s.confirmDestructive })} aria-label="Confirm destructive statements"></button>
        <span class="faint">Ask before DROP, TRUNCATE, and DELETE or UPDATE without WHERE.</span></div>
    </section>
  </div>
</div>

<style>
  .sv { height: 100%; overflow: auto; }
  .inner { max-width: 820px; margin: 0 auto; padding: 28px 32px 48px; display: flex; flex-direction: column; gap: 14px; }
  h1 { margin: 0; font: 600 20px/1.25 var(--sans); letter-spacing: -0.015em; }
  :global(:root[data-layout='workspace']) h1 { font: 600 15px var(--mono); letter-spacing: 0; }
  p { margin: 0 0 6px; }
  .sh { padding: 12px 16px; border-bottom: 1px solid var(--line); font-weight: 500; }
  .r { display: grid; grid-template-columns: 210px auto 1fr; gap: 16px; align-items: center; padding: 12px 16px; border-top: 1px solid var(--line); font-size: 12.5px; }
  .sh + .r { border-top: 0; }
  .sw { width: 20px; height: 20px; border-radius: 50%; background: var(--c); }
  .sw.on { box-shadow: 0 0 0 2px var(--raised), 0 0 0 3.5px var(--c); }
  :global(:root[data-layout='workspace']) .sw { border-radius: 0; }
</style>
