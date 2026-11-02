<script lang="ts">
  // The search surface of the command layout (2a). The first word picks a
  // table. Other words become filters, and the preview runs live.
  import { untrack } from 'svelte'
  import { app } from '../lib/store.svelte'
  import { api } from '../lib/api'
  import { fuzzy, matchWords, fitColumns, fmtK, fmtN, ENGINE, connColor, MOD, isToday, plural, ago } from '../lib/format'
  import { looksLikeSQL, looksLikeWrite, highlight } from '../lib/sql'
  import type { PreviewResult, RunResult, TableSummary, Cell } from '../lib/types'
  import Icon from './Icon.svelte'

  let { overlay = false }: { overlay?: boolean } = $props()

  type Item = {
    key: string
    group: string
    icon: string
    label: string
    mono?: boolean
    hint?: string
    kind: 'table' | 'snippet' | 'sql' | 'command' | 'conn'
    table?: TableSummary
    rest?: string
    sql?: string
    desc?: string
    run: (asQuery?: boolean) => void
  }

  let q = $state('')
  let idx = $state(0)
  let input: HTMLInputElement
  let preview = $state.raw<PreviewResult | null>(null)
  let previewSQL = $state.raw<RunResult | null>(null)
  let loading = $state(false)
  let timer = 0
  let token = 0

  const cid = $derived(app.activeConn)
  const cn = $derived(app.active)
  const tables = $derived(app.tables(cid))
  const trimmed = $derived(q.trim())
  const first = $derived(trimmed.split(/\s+/)[0] ?? '')
  const rest = $derived(trimmed.slice(first.length).trim())
  const sqlish = $derived(looksLikeSQL(trimmed))

  const tableHits = $derived.by(() => {
    if (sqlish) return []
    if (!first) return tables.slice(0, 6)
    return tables
      .map((t) => ({ t, s: fuzzy(first, t.name) }))
      .filter((x) => x.s >= 0)
      .sort((a, b) => b.s - a.s)
      .slice(0, 6)
      .map((x) => x.t)
  })
  const top = $derived(tableHits[0])

  const commands = $derived.by(() => {
    const c = cn
    const unl = c ? app.unlocked(c.id) : false
    const todayFor = (t: string) => app.log.filter((e) => isToday(e.at) && e.sql.toLowerCase().includes(t.toLowerCase())).length
    const list: Omit<Item, 'group' | 'key'>[] = []
    if (top && first && c) {
      list.push({ kind: 'command', icon: 'receipt_long', label: `Agent activity on ${top.name}`, hint: `${todayFor(top.name)} today`, desc: `Agent calls whose SQL names ${top.name}.`, run: () => app.openView('log', top.name) })
    }
    list.push(
      { kind: 'command', icon: 'terminal', label: 'New query', hint: c?.name, desc: 'Open an empty editor on the current connection.', run: () => c && app.openQuery(c.id, '') },
      { kind: 'command', icon: 'inbox', label: 'Approval queue', hint: `${app.queue.length} waiting`, desc: 'Writes from agents that wait for you.', run: () => { app.palette = false; app.queueOpen = true } },
      { kind: 'command', icon: unl ? 'lock' : 'lock_open', label: c ? (unl ? `Relock writes on ${c.name}` : `Unlock writes on ${c.name}`) : 'Unlock writes', desc: 'Every connection opens read-only. Unlock writes for a while, or until relaunch.', run: () => c && (unl ? app.relock(c.id) : app.requireWrite(c.id)) },
      { kind: 'command', icon: 'hub', label: 'MCP server', hint: app.mcp.running ? 'running' : 'off', desc: 'Clients, agent access per connection, and setup snippets.', run: () => app.openView('mcp') },
      { kind: 'command', icon: 'receipt_long', label: 'Activity log', hint: `${app.log.filter((e) => isToday(e.at)).length} today`, desc: 'Every agent call with its outcome.', run: () => app.openView('log') },
      { kind: 'command', icon: 'add', label: 'New connection', desc: 'Add PostgreSQL, MySQL or SQLite. Passwords go to the system keychain.', run: () => { app.palette = false; app.connDialog = { id: null } } },
    )
    if (c) list.push({ kind: 'command', icon: 'edit', label: `Edit ${c.name}`, desc: 'Change host, credentials, color and production flag.', run: () => { app.palette = false; app.connDialog = { id: c.id } } })
    if (c) list.push({ kind: 'command', icon: 'refresh', label: `Reload tables of ${c.name}`, desc: 'Read the table list and row counts again.', run: () => { app.loadSchema(c.id, true); app.toast('Reloading tables') } })
    list.push(
      { kind: 'command', icon: 'space_dashboard', label: app.layout === 'command' ? 'Switch to workspace layout' : 'Switch to command layout', desc: 'Command layout opens everything from search. Workspace layout keeps tabs, tables, editor and inspector on screen.', run: () => app.toggleLayout() },
      { kind: 'command', icon: 'contrast', label: 'Toggle light and dark', run: () => app.toggleTheme() },
      { kind: 'command', icon: 'settings', label: 'Settings', run: () => app.openView('settings') },
    )
    return list
  })

  const items = $derived.by(() => {
    const out: Item[] = []
    const c = cn
    if (c) {
      tableHits.forEach((t, i) => {
        const r = i === 0 ? rest : ''
        const filtered = i === 0 && r && preview?.table === t.name && (preview.interp.filters?.length ?? 0) > 0
        out.push({
          key: 't:' + t.schema + '.' + t.name, group: `Tables in ${c.name}`, kind: 'table', icon: t.kind === 'view' ? 'visibility' : 'table_chart',
          label: t.name, mono: true, hint: filtered ? 'filtered' : t.kind === 'view' ? 'view' : (t.estimate ? '~' : '') + fmtK(t.rows), table: t, rest: r,
          run: (asQuery) => openTable(t, r, asQuery),
        })
      })
    }
    const snips = app.config.snippets ?? []
    const sh = trimmed ? snips.map((s) => ({ s, sc: matchWords(trimmed, s.name) })).filter((x) => x.sc >= 0).sort((a, b) => b.sc - a.sc).slice(0, 4).map((x) => x.s) : snips.slice(0, 3)
    for (const s of sh) out.push({ key: 's:' + s.id, group: 'Saved queries', kind: 'snippet', icon: 'bookmark', label: s.name, sql: s.sql, run: () => c && app.openQuery(c.id, s.sql) })
    if (c && sqlish) {
      out.push({ key: 'sql', group: 'Run as SQL', kind: 'sql', icon: 'terminal', label: trimmed, mono: true, sql: trimmed, run: () => app.openQuery(c.id, trimmed, !looksLikeWrite(trimmed)) })
    } else if (c && top && rest && preview?.table === top.name && preview.sql) {
      const sql = preview.sql
      out.push({ key: 'sql', group: 'Run as SQL', kind: 'sql', icon: 'terminal', label: sql, mono: true, sql, run: () => app.openQuery(c.id, sql, true) })
    }
    const cmds = trimmed ? commands.map((x) => ({ x, s: matchWords(trimmed, x.label) })).filter((y) => y.s >= 0).sort((a, b) => b.s - a.s).map((y) => y.x) : commands.slice(0, 4)
    const keepFirst = trimmed && top && first ? commands.slice(0, 1) : []
    const seen = new Set<string>()
    for (const x of [...keepFirst, ...cmds].slice(0, 6)) {
      if (seen.has(x.label)) continue
      seen.add(x.label)
      out.push({ ...x, key: 'c:' + x.label, group: 'Commands' })
    }
    const ch = trimmed ? app.conns.filter((x) => x.id !== cid && matchWords(trimmed, x.name) >= 0) : []
    for (const x of ch.slice(0, 4)) out.push({ key: 'n:' + x.id, group: 'Connections', kind: 'conn', icon: 'database', label: x.name, hint: ENGINE[x.engine].label, run: () => { app.setActive(x.id); app.palette = false; q = '' } })
    return out
  })

  const groups = $derived.by(() => {
    const g: { title: string; items: { it: Item; i: number }[] }[] = []
    items.forEach((it, i) => {
      const last = g[g.length - 1]
      if (last && last.title === it.group) last.items.push({ it, i })
      else g.push({ title: it.group, items: [{ it, i }] })
    })
    return g
  })

  const current = $derived(items[Math.min(idx, items.length - 1)])

  $effect(() => {
    // Keep the selection in range when the list changes.
    if (idx >= items.length) idx = Math.max(0, items.length - 1)
  })

  // Live preview of the selected item. The effect keys on plain strings so
  // that a new preview result does not start another preview.
  const previewKey = $derived(current ? `${cid}|${current.kind}|${current.key}|${current.rest ?? ''}|${current.kind === 'sql' && sqlish ? current.sql : ''}` : '')

  $effect(() => {
    const key = previewKey
    clearTimeout(timer)
    const my = ++token
    if (!key) return
    untrack(() => {
      const it = current!
      const c = cid
      if (it.kind === 'table' && it.table) {
        const t = it.table
        const r = it.rest ?? ''
        loading = true
        timer = window.setTimeout(async () => {
          const p = await api.preview({ conn: c, schema: t.schema, table: t.name, rest: r, limit: 8 })
          if (my !== token || p.canceled) return
          preview = p
          loading = false
        }, 90)
      } else if (it.kind === 'sql' && it.sql && sqlish) {
        const sql = it.sql
        if (looksLikeWrite(sql)) {
          previewSQL = { write: true } as RunResult
          return
        }
        loading = true
        timer = window.setTimeout(async () => {
          const r = await api.previewSQL(c, sql)
          if (my !== token) return
          previewSQL = r
          loading = false
        }, 260)
      }
    })
  })

  $effect(() => {
    if (overlay || app.view.kind === 'home') queueMicrotask(() => input?.focus())
  })

  function openTable(t: TableSummary, r: string, asQuery?: boolean) {
    const c = cid
    let sql = ''
    if (preview && preview.table === t.name && preview.schema === t.schema && r) sql = preview.sql
    if (asQuery) {
      app.openQuery(c, (sql || `SELECT * FROM ${t.name}`) + ';', true)
    } else {
      app.openTable(c, t.schema, t.name, { rest: r, sql })
    }
    q = ''
  }

  function onKey(e: KeyboardEvent) {
    const n = items.length
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      idx = (idx + 1) % Math.max(n, 1)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      idx = (idx - 1 + n) % Math.max(n, 1)
    } else if (e.key === 'Enter') {
      e.preventDefault()
      current?.run(e.metaKey || e.ctrlKey)
    } else if (e.key === 'Tab' && !e.shiftKey) {
      // Write the reading of the filters into the input so the user can edit it.
      e.preventDefault()
      if (top && preview?.table === top.name && preview.interp.canonical) {
        const extra = preview.interp.unresolved?.join(' ') ?? ''
        q = `${top.name} ${preview.interp.canonical}${extra ? ' ' + extra : ''} `
      } else if (current?.kind === 'table') {
        q = current.label + ' '
      }
    } else if (e.key === 'Escape') {
      if (q) {
        e.stopPropagation()
        q = ''
      }
    }
  }

  function onInput() {
    idx = 0
  }

  const pv = $derived(current?.kind === 'table' && preview && current.table && preview.table === current.table.name ? preview : null)
  const pvRows = $derived<Cell[][]>(pv?.rows ?? [])
  // Show the primary key and the filtered columns first, so the filter is visible.
  const pvOrder = $derived.by(() => {
    const cols = pv?.columns ?? []
    const want = new Set(pv?.interp.filters?.map((f) => f.column) ?? [])
    const rank = (j: number) => (j === 0 ? 0 : want.has(cols[j].name) ? 1 : 2)
    return cols.map((_, j) => j).sort((a, b) => rank(a) - rank(b) || a - b)
  })
  const pvFit = $derived(fitColumns(pvOrder.map((j) => pv!.columns![j]), pvRows.map((r) => pvOrder.map((j) => r[j])), 470))
  const sqFit = $derived(fitColumns(previewSQL?.columns ?? [], previewSQL?.rows ?? [], 470))
  const recents = $derived(app.recents.slice(0, 4))

  function openRecent(r: (typeof app.recents)[number]) {
    if (!app.conn(r.conn)) return
    if (r.kind === 'table' && r.table) app.openTable(r.conn, r.schema ?? '', r.table)
    else if (r.kind === 'query') app.openQuery(r.conn, r.sql ?? '')
    else if (r.kind === 'view' && r.view) app.openView(r.view as 'log' | 'mcp' | 'settings')
  }

  function where(p: PreviewResult) {
    const parts: string[] = []
    if (p.where) parts.push('where ' + p.where)
    if (p.matched >= 0) parts.push(`${p.matchedCapped ? fmtN(p.matched) + '+' : fmtN(p.matched)} of ${p.totalEstimate ? '~' : ''}${fmtN(p.total)} rows`)
    return parts.join(' · ')
  }
</script>

<div class="cs" class:overlay>
  <div class="box">
    <div class="in">
      <Icon name="search" size={22} fill="var(--faint)" />
      <input
        bind:this={input}
        bind:value={q}
        oninput={onInput}
        onkeydown={onKey}
        placeholder={cn ? 'Search tables, saved queries, SQL and commands' : 'Add a connection to start'}
        spellcheck="false"
        autocomplete="off"
      />
      {#if overlay}<span class="kbd">esc</span>{:else}<span class="kbd">{MOD}K</span>{/if}
    </div>
    <div class="split">
      <div class="list">
        {#each groups as g}
          <div class="gh">{g.title}</div>
          {#each g.items as { it, i } (it.key)}
            <button class="it" class:on={i === idx} onmousemove={() => (idx = i)} onclick={() => it.run()}>
              <Icon name={it.icon} size={17} fill="var(--muted)" />
              <span class="lbl ellipsis" class:mono={it.mono} class:sqlish={it.kind === 'sql'}>{it.label}</span>
              {#if it.hint}<span class="hint">{it.hint}</span>{/if}
            </button>
          {/each}
        {:else}
          <div class="none">{cn ? 'No matches.' : 'No connections yet.'}</div>
        {/each}
      </div>
      <div class="pv">
        {#if !current}
          <div class="empty">
            {#if !cn}
              <div>
                <p>Rowan has no connections.</p>
                <button class="btn primary" onclick={() => (app.connDialog = { id: null })}><Icon name="add" size={16} />New connection</button>
              </div>
            {:else}
              Type to search.
            {/if}
          </div>
        {:else if current.kind === 'table'}
          <div class="ph">
            <span class="mono strong">{current.label}</span>
            <span class="faint ellipsis">{pv ? where(pv) : loading ? 'Loading…' : ''}</span>
            <span class="grow"></span>
            <span class="keys">↵ open · {MOD}↵ query</span>
          </div>
          {#if pv?.error}
            <div class="perr danger selectable">{pv.error}</div>
          {:else if pv}
            <div class="mini" style="--tpl:{pvFit.template}">
              <div class="mr head">{#each pvFit.cols as c}<span class:num={c.numeric}>{c.name}</span>{/each}</div>
              {#each pvRows as r}
                <div class="mr">{#each pvFit.cols as c, j}{@const v = r[pvOrder[j]]}<span class:num={c.numeric} class:null={v === null} class="ellipsis">{v ?? 'NULL'}</span>{/each}</div>
              {:else}
                <div class="mr nomatch faint">No rows match.</div>
              {/each}
            </div>
          {/if}
          <span class="grow"></span>
          {#if pv?.interp.note}
            <div class="note"><Icon name="auto_awesome" size={15} />{pv.interp.note}</div>
          {:else if pv?.interp.filters?.length}
            <div class="note"><Icon name="filter_alt" size={15} />Filtered on {pv.interp.filters.map((f) => f.column).join(' and ')}. Press tab to edit the filters.</div>
          {:else}
            <div class="note faint"><span>Add words to filter: a value such as <span class="mono">refunded</span>, <span class="mono">column:value</span> or <span class="mono">total_cents&gt;5000</span>.</span></div>
          {/if}
        {:else if current.kind === 'snippet' || (current.kind === 'sql' && !sqlish)}
          <div class="ph"><span class="strong">{current.kind === 'snippet' ? current.label : 'Run as SQL'}</span><span class="grow"></span><span class="keys">↵ open in the editor</span></div>
          <pre class="code selectable">{@html highlight(current.sql ?? '')}</pre>
        {:else if current.kind === 'sql'}
          <div class="ph"><span class="strong">Run as SQL</span><span class="faint">{previewSQL?.columns ? plural(previewSQL.rows?.length ?? 0, 'row') + (previewSQL.truncated ? ', first 12' : '') : ''}</span><span class="grow"></span><span class="keys">↵ open and run</span></div>
          {#if previewSQL?.write}
            <div class="note warn"><Icon name="lock" size={15} />Writes run from the editor, after you unlock the connection. ↵ opens it there.</div>
          {:else if previewSQL?.error}
            <div class="perr danger selectable">{previewSQL.error}</div>
          {:else if previewSQL?.columns}
            <div class="mini" style="--tpl:{sqFit.template}">
              <div class="mr head">{#each sqFit.cols as c}<span class:num={c.numeric}>{c.name}</span>{/each}</div>
              {#each (previewSQL.rows ?? []).slice(0, 8) as r}
                <div class="mr">{#each sqFit.cols as c, j}<span class:num={c.numeric} class:null={r[j] === null} class="ellipsis">{r[j] ?? 'NULL'}</span>{/each}</div>
              {/each}
            </div>
          {:else}
            <div class="note faint">{loading ? 'Running a read-only preview…' : ''}</div>
          {/if}
        {:else if current.kind === 'conn'}
          {@const x = app.conns.find((c) => 'n:' + c.id === current.key)}
          {#if x}
            <div class="ph"><span class="dot" style="background:{connColor(x.color)}"></span><span class="strong">{x.name}</span><span class="grow"></span><span class="keys">↵ switch</span></div>
            <div class="conninfo">
              <div><span class="faint">Engine</span>{ENGINE[x.engine].label}</div>
              <div><span class="faint">{x.engine === 'sqlite' ? 'File' : 'Host'}</span><span class="mono">{x.engine === 'sqlite' ? x.file : `${x.host}:${x.port}`}</span></div>
              {#if x.engine !== 'sqlite'}<div><span class="faint">Database</span><span class="mono">{x.database}</span></div>{/if}
              <div><span class="faint">Agents</span>{x.mcp.exposed ? (x.mcp.access === 'read-write' ? 'Read and write' : 'Read-only') : 'Hidden'}</div>
              {#if x.production}<div><span class="faint">Production</span>Unlock asks for the name</div>{/if}
            </div>
          {/if}
        {:else}
          <div class="ph"><span class="strong">{current.label}</span><span class="grow"></span><span class="keys">↵ run</span></div>
          {#if current.desc}<p class="desc muted">{current.desc}</p>{/if}
        {/if}
      </div>
    </div>
  </div>
  {#if !overlay && recents.length}
    <div class="recent">
      <span class="label">Recently opened</span>
      <div class="rg">
        {#each recents as r}
          <button onclick={() => openRecent(r)}>
            <div class:mono={r.kind === 'table'} class="rt ellipsis">{r.title}</div>
            <div class="faint">{r.kind === 'table' ? app.conn(r.conn)?.name ?? r.conn : r.kind === 'query' ? 'Query' : 'View'} · {ago(r.at, app.now)}</div>
          </button>
        {/each}
      </div>
    </div>
  {/if}
</div>

<style>
  .cs { width: 880px; max-width: calc(100vw - 48px); display: flex; flex-direction: column; gap: 40px; }
  .box { background: var(--raised); border: 1px solid var(--line); border-radius: var(--r-xl); box-shadow: var(--shadow); overflow: hidden; }
  .in { height: 62px; display: flex; align-items: center; gap: 12px; padding: 0 20px; border-bottom: 1px solid var(--line); }
  .in input { flex: 1; min-width: 0; border: 0; outline: none; background: transparent; font: 400 19px var(--sans); caret-color: var(--accent); }
  :global(:root[data-layout='workspace']) .in input { font: 400 17px var(--mono); }
  .split { display: grid; grid-template-columns: 330px minmax(0, 1fr); height: 410px; }
  .list { border-right: 1px solid var(--line); padding: 8px; display: flex; flex-direction: column; gap: 1px; overflow: auto; }
  .gh { padding: 8px 10px 4px; font: 500 11.5px var(--sans); color: var(--faint); }
  .gh:not(:first-child) { padding-top: 10px; }
  .it { display: flex; align-items: center; gap: 10px; min-height: 36px; padding: 6px 10px; border-radius: 8px; text-align: left; }
  :global(:root[data-layout='workspace']) .it { border-radius: 0; }
  .it.on { background: var(--hover); }
  .lbl { flex: 1; font-size: 13px; }
  .lbl.mono { font: 400 13px var(--mono); }
  .it.on .lbl.mono { font-weight: 500; }
  .lbl.sqlish { font: 400 12px/1.45 var(--mono); color: var(--muted); white-space: normal; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; }
  .hint { font: 400 11.5px var(--mono); color: var(--faint); white-space: nowrap; }
  .none { padding: 14px 10px; color: var(--faint); }
  .pv { display: flex; flex-direction: column; min-width: 0; min-height: 0; }
  .ph { display: flex; align-items: baseline; gap: 10px; padding: 14px 18px 10px; min-width: 0; }
  .ph .dot { align-self: center; }
  .strong { font-weight: 600; font-size: 14px; }
  .mono.strong { font: 600 14px var(--mono); }
  .keys { font: 400 11.5px var(--mono); color: var(--faint); white-space: nowrap; }
  .mini { margin: 0 18px; border: 1px solid var(--line); border-radius: 8px; overflow: hidden; font: 400 12px var(--mono); }
  :global(:root[data-layout='workspace']) .mini { border-radius: 0; }
  .mr { display: grid; grid-template-columns: var(--tpl); gap: 10px; height: 30px; align-items: center; padding: 0 12px; border-bottom: 1px solid var(--line); }
  .mr:last-child { border-bottom: 0; }
  .mr.head { background: var(--panel); font: 500 12px var(--sans); color: var(--text); }
  .mr span { min-width: 0; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .mr .num { text-align: right; }
  .mr .null { color: var(--faint); }
  .mr.nomatch { display: block; line-height: 30px; }
  .note { display: flex; align-items: center; gap: 8px; padding: 12px 18px; border-top: 1px solid var(--line); font-size: 12px; color: var(--muted); }
  .note.warn { color: var(--warn); border-top: 0; }
  .perr { padding: 4px 18px; font-size: 12.5px; }
  .code { margin: 0 18px; padding: 12px 14px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); font: 400 12.5px/1.6 var(--mono); white-space: pre-wrap; overflow: auto; max-height: 320px; }
  .code :global(.k) { color: var(--syn-kw); }
  .code :global(.s) { color: var(--syn-str); }
  .code :global(.n) { color: var(--syn-num); }
  .code :global(.f) { color: var(--syn-fn); }
  .code :global(.c) { color: var(--syn-com); }
  .desc { padding: 0 18px; margin: 0; line-height: 1.5; max-width: 460px; }
  .conninfo { padding: 0 18px; display: flex; flex-direction: column; gap: 8px; }
  .conninfo > div { display: grid; grid-template-columns: 100px 1fr; }
  .empty p { margin: 0 0 12px; }
  .recent { display: flex; flex-direction: column; gap: 8px; }
  .rg { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 24px; font-size: 12.5px; }
  .rg button { text-align: left; min-width: 0; padding: 4px 0; }
  .rg button:hover .rt { color: var(--accent-text); }
  .rt { font-weight: 500; }
  .rt.mono { font: 500 13px var(--mono); }
  .cs.overlay { gap: 0; }
</style>
