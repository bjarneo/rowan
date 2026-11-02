<script lang="ts">
  import { app } from '../lib/store.svelte'
  import { api } from '../lib/api'
  import { CC, ENGINE } from '../lib/format'
  import type { Connection } from '../lib/types'
  import Icon from './Icon.svelte'

  const id = app.connDialog?.id ?? null
  const existing = id ? app.conn(id) : undefined

  const blank: Connection = {
    id: '', name: '', engine: 'postgres', host: 'localhost', port: 5432, database: '', user: '', file: '',
    sslMode: 'prefer', sslRootCert: '', color: 'violet', production: false, hasPassword: false,
    ssh: { enabled: false, host: '', port: 22, user: '', key: '~/.ssh/id_ed25519' },
    mcp: { exposed: false, access: 'read-only', approveWrites: true, rowLimit: 500, tables: null },
  }
  let f = $state<Connection>(existing ? structuredClone($state.snapshot(existing) as Connection) : blank)
  let password = $state('')
  let pwChanged = $state(!existing)
  let test = $state<{ st: 'run' | 'ok' | 'fail'; msg: string } | null>(null)
  let saving = $state(false)
  let confirmDelete = $state(false)

  const sslModes = $derived(
    f.engine === 'mysql' ? ['disabled', 'preferred', 'required', 'verify-ca', 'verify-identity'] : ['disable', 'allow', 'prefer', 'require', 'verify-ca', 'verify-full'],
  )

  function setEngine(e: Connection['engine']) {
    f.engine = e
    f.port = ENGINE[e].port
    f.sslMode = e === 'mysql' ? 'preferred' : 'prefer'
    test = null
  }

  const form = () => ({ ...($state.snapshot(f) as Connection), password, passwordChanged: pwChanged })

  async function runTest() {
    test = { st: 'run', msg: 'Connecting…' }
    const r = await api.testConnection(form())
    test = { st: r.ok ? 'ok' : 'fail', msg: r.message }
  }

  async function save() {
    if (!f.name.trim()) {
      test = { st: 'fail', msg: 'Give the connection a name.' }
      return
    }
    saving = true
    try {
      const c = await api.saveConnection(form())
      const conns = app.conns.some((x) => x.id === c.id) ? app.conns.map((x) => (x.id === c.id ? c : x)) : [...app.conns, c]
      app.config = { ...app.config, connections: conns }
      app.connDialog = null
      app.setActive(c.id)
      app.loadSchema(c.id, true)
      app.toast(`Saved ${c.name} to config.toml`)
    } catch (e) {
      test = { st: 'fail', msg: String(e) }
    } finally {
      saving = false
    }
  }

  async function remove() {
    if (!existing) return
    if (!confirmDelete) {
      confirmDelete = true
      return
    }
    await api.deleteConnection(existing.id)
    const conns = app.conns.filter((x) => x.id !== existing.id)
    app.config = { ...app.config, connections: conns }
    if (app.activeConn === existing.id) app.setActive(conns[0]?.id ?? '')
    app.connDialog = null
    app.toast(`Removed ${existing.name}`)
  }

  async function choose(field: 'file' | 'sslRootCert' | 'sshKey') {
    const p = await api.chooseFile(field === 'file' ? 'Choose a SQLite database' : field === 'sshKey' ? 'Choose an SSH key' : 'Choose a CA certificate')
    if (!p) return
    if (field === 'sshKey') f.ssh.key = p
    else f[field] = p
    if (field === 'file' && !f.name) f.name = p.split('/').pop() ?? ''
  }

  function close() {
    app.connDialog = null
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="scrim" onclick={close}></div>
<div class="dlg" role="dialog" aria-modal="true" aria-label="Connection">
  <div class="hd">
    <div>
      <div class="t">{existing ? existing.name : 'New connection'}</div>
      <div class="muted">{existing ? (f.engine === 'sqlite' ? 'Changes go to config.toml.' : 'Changes go to config.toml. The password stays in the system keychain.') : 'Saved connections open read-only. Passwords go to the system keychain.'}</div>
    </div>
    <span class="grow"></span>
    <button class="icon-btn" onclick={close}><Icon name="close" size={18} /></button>
  </div>
  <div class="body">
    <label class="fld wide"><span>Name</span><input class="input" bind:value={f.name} placeholder="acme-prod" /></label>
    <div class="fld wide">
      <span>Engine</span>
      <div class="engines">
        {#each ['postgres', 'mysql', 'sqlite'] as e}
          <button class="eng" class:on={f.engine === e} onclick={() => setEngine(e as Connection['engine'])}>
            <Icon name="database" size={18} />{ENGINE[e].label}
          </button>
        {/each}
      </div>
    </div>
    {#if f.engine === 'sqlite'}
      <div class="fld wide">
        <span>Database file</span>
        <div class="row" style="gap:8px">
          <input class="input mono" bind:value={f.file} placeholder="~/Projects/app/data.sqlite" />
          <button class="btn" onclick={() => choose('file')}><Icon name="folder_open" size={15} />Choose</button>
        </div>
      </div>
    {:else}
      <label class="fld"><span>Host</span><input class="input mono" bind:value={f.host} placeholder="localhost" /></label>
      <label class="fld sm"><span>Port</span><input class="input mono" type="number" bind:value={f.port} /></label>
      <label class="fld"><span>Database</span><input class="input mono" bind:value={f.database} /></label>
      <label class="fld"><span>User</span><input class="input mono" bind:value={f.user} /></label>
      <label class="fld">
        <span>Password</span>
        <input class="input mono" type="password" value={password} placeholder={existing?.hasPassword && !pwChanged ? 'Saved in the keychain' : ''} oninput={(e) => { password = e.currentTarget.value; pwChanged = true }} />
      </label>
      <label class="fld"><span>SSL mode</span>
        <select class="input" bind:value={f.sslMode}>{#each sslModes as m}<option value={m}>{m}</option>{/each}</select>
      </label>
      {#if /verify/.test(f.sslMode)}
        <div class="fld wide">
          <span>CA certificate</span>
          <div class="row" style="gap:8px"><input class="input mono" bind:value={f.sslRootCert} placeholder="~/.rowan/certs/ca.pem" /><button class="btn" onclick={() => choose('sslRootCert')}>Choose</button></div>
        </div>
      {/if}
      <div class="fld wide toggle">
        <button class="switch" class:on={f.ssh.enabled} onclick={() => (f.ssh.enabled = !f.ssh.enabled)} aria-label="SSH tunnel"></button>
        <span>Connect through an SSH bastion. Rowan checks the host key in <span class="mono">~/.ssh/known_hosts</span> and uses ssh-agent.</span>
      </div>
      {#if f.ssh.enabled}
        <label class="fld"><span>SSH host</span><input class="input mono" bind:value={f.ssh.host} placeholder="bastion.example.com" /></label>
        <label class="fld sm"><span>Port</span><input class="input mono" type="number" bind:value={f.ssh.port} /></label>
        <label class="fld"><span>SSH user</span><input class="input mono" bind:value={f.ssh.user} /></label>
        <div class="fld">
          <span>Key file</span>
          <div class="row" style="gap:8px"><input class="input mono" bind:value={f.ssh.key} /><button class="btn" onclick={() => choose('sshKey')}>Choose</button></div>
        </div>
      {/if}
    {/if}
    <div class="fld wide">
      <span>Color</span>
      <div class="colors">
        {#each Object.entries(CC) as [k, v]}
          <button class="sw" class:on={f.color === k} style="background:{v};--c:{v}" onclick={() => (f.color = k)} aria-label={k}></button>
        {/each}
      </div>
    </div>
    <div class="fld wide toggle">
      <button class="switch" class:on={f.production} onclick={() => (f.production = !f.production)} aria-label="Production"></button>
      <span><b>Production.</b> Unlocking writes asks you to type the name. Agent writes always need your approval.</span>
    </div>
    <div class="fld wide toggle">
      <button class="switch" class:on={f.mcp.exposed} onclick={() => (f.mcp.exposed = !f.mcp.exposed)} aria-label="Share with agents"></button>
      <span><b>Share with agents.</b> MCP clients can see this connection. {existing ? 'Set access, approvals and tables in the MCP server view.' : 'They get read-only access until you change the policy in the MCP server view.'}</span>
    </div>
  </div>
  {#if test}
    <div class="test {test.st}">
      <Icon name={test.st === 'ok' ? 'check_circle' : test.st === 'fail' ? 'error' : 'hourglass_top'} size={16} />
      <span class="selectable">{test.msg}</span>
    </div>
  {/if}
  <div class="ft">
    {#if existing}
      <button class="btn danger" onclick={remove}><Icon name="delete" size={15} />{confirmDelete ? 'Click again to remove' : 'Remove'}</button>
    {/if}
    <span class="grow"></span>
    <button class="btn" onclick={runTest} disabled={test?.st === 'run'}>Test connection</button>
    <button class="btn primary" onclick={save} disabled={saving}>Save</button>
  </div>
</div>

<style>
  .dlg { position: fixed; z-index: 41; top: 6vh; left: 50%; transform: translateX(-50%); width: 640px; max-height: 88vh; display: flex; flex-direction: column; background: var(--raised); border: 1px solid var(--line); border-radius: var(--r-xl); box-shadow: var(--shadow); }
  .hd { display: flex; align-items: flex-start; gap: 12px; padding: 18px 16px 14px 20px; border-bottom: 1px solid var(--line); }
  .t { font-weight: 600; font-size: 15px; margin-bottom: 3px; }
  .hd .muted { font-size: 12.5px; }
  .body { padding: 16px 20px; overflow: auto; display: grid; grid-template-columns: 1fr 1fr; gap: 12px 14px; }
  .fld { display: flex; flex-direction: column; gap: 5px; min-width: 0; }
  .fld > span:first-child { font-size: 12px; color: var(--muted); }
  .fld.wide { grid-column: 1 / -1; }
  .fld.toggle { flex-direction: row; align-items: flex-start; gap: 10px; font-size: 12.5px; line-height: 1.45; color: var(--muted); }
  .fld.toggle > span:first-child { color: var(--muted); }
  .fld.toggle b { color: var(--text); font-weight: 500; }
  .fld.toggle .switch { margin-top: 1px; }
  .engines { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; }
  .eng { display: flex; align-items: center; gap: 8px; height: 40px; padding: 0 12px; border: 1px solid var(--line); border-radius: var(--r-lg); background: var(--raised); font-weight: 500; }
  .eng.on { border-color: var(--accent); background: var(--accent-soft); color: var(--accent-text); }
  .colors { display: flex; gap: 10px; }
  .sw { width: 20px; height: 20px; border-radius: 50%; }
  .sw.on { box-shadow: 0 0 0 2px var(--raised), 0 0 0 3.5px var(--c); }
  :global(:root[data-layout='workspace']) .sw { border-radius: 0; }
  .test { margin: 0 20px 4px; padding: 9px 12px; border-radius: var(--r); display: flex; gap: 8px; align-items: center; font-size: 12.5px; }
  .test.ok { background: var(--ok-soft); color: var(--ok); }
  .test.fail { background: var(--danger-soft); color: var(--danger); }
  .test.run { background: var(--sunken); color: var(--muted); }
  .ft { display: flex; gap: 8px; padding: 14px 20px; border-top: 1px solid var(--line); }
</style>
