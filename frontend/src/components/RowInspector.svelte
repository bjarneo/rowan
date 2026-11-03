<script lang="ts">
  import type { RunResult, Inspection, Link, TableInfo } from '../lib/types'
  import { api } from '../lib/api'
  import { app } from '../lib/store.svelte'
  import { fmtN } from '../lib/format'

  let {
    conn,
    result,
    selected,
    onfollow,
  }: { conn: string; result: RunResult | null; selected: number; onfollow: (l: Link, value: string) => void } = $props()

  const ws = $derived(app.layout === 'workspace')
  const row = $derived(result?.rows && selected >= 0 ? result.rows[selected] : null)
  const src = $derived(result?.source ?? null)
  let insp = $state.raw<Inspection | null>(null)
  let info = $state.raw<TableInfo | null>(null)
  let timer = 0
  let token = 0

  const tableCache = new Map<string, Promise<TableInfo>>()
  function tableInfo(c: string, schema: string, table: string) {
    const k = `${c}/${schema}/${table}`
    if (!tableCache.has(k)) tableCache.set(k, api.table(c, schema, table).catch(() => null as unknown as TableInfo))
    return tableCache.get(k)!
  }

  $effect(() => {
    const r = row
    const s = src
    const cols = result?.columns
    clearTimeout(timer)
    insp = null
    if (!r || !s || !cols) {
      info = null
      return
    }
    const my = ++token
    tableInfo(conn, s.schema, s.table).then((t) => {
      if (my === token) info = t
    })
    timer = window.setTimeout(async () => {
      const obj: Record<string, string | null> = {}
      cols.forEach((c, j) => (obj[c.name] = r[j]))
      const res = await api.inspect({ conn, schema: s.schema, table: s.table, row: obj })
      if (my === token) insp = res
    }, 120)
  })

  const pk = $derived(info?.columns.find((c) => c.pk))
  const pkIdx = $derived(pk && result?.columns ? result.columns.findIndex((c) => c.name === pk.name) : -1)
  const title = $derived(
    src && row ? `${ws ? 'row · ' : ''}${src.table}${pkIdx >= 0 ? ` · ${pk!.name} ${row[pkIdx]}` : ''}` : row ? `row ${selected + 1}` : '',
  )

  function copy(v: string | null) {
    api.copy(v ?? 'NULL')
    app.toast('Copied the value')
  }
</script>

<div class="ri">
  {#if !row || !result?.columns}
    <div class="hd faint">{ws ? 'row inspector' : 'Row inspector'}</div>
    <div class="hint faint">Select a row to inspect it.</div>
  {:else}
    <div class="hd faint ellipsis" title={title}>{title}</div>
    <div class="fields selectable">
      {#each result.columns as c, j}
        {@const v = row[j]}
        {@const fk = insp?.fks?.[c.name]}
        <div class="f">
          <span class="k ellipsis" title={c.name}>{c.name}</span>
          {#if fk && v !== null}
            <button class="v link ellipsis" onclick={() => onfollow(fk, v)} title="Open {fk.table}">{v}{fk.label ? ` → ${fk.label}` : ` → ${fk.table}`}</button>
          {:else}
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <span class="v ellipsis" class:null={v === null} title={v ?? 'NULL'} ondblclick={() => copy(v)}>{v === null ? 'NULL' : v}</span>
          {/if}
        </div>
      {/each}
      {#if insp?.refBy?.some((l) => l.count !== 0)}
        <div class="f">
          <span class="k">referenced by</span>
          <span class="v refs">
            <!-- A count of -1 means the column has no index, so Rowan did not count. -->
            {#each insp.refBy.filter((l) => l.count !== 0) as l, i}
              {#if i > 0}<span class="faint sep">·</span>{/if}
              <button class="link" onclick={() => onfollow(l, '')} title={l.count < 0 ? `${l.table}.${l.column} has no index. Rowan did not count the rows.` : `${l.table}.${l.column}`}>{l.count < 0 ? '' : (l.capped ? fmtN(l.count) + '+' : fmtN(l.count)) + ' '}{l.table}</button>
            {/each}
          </span>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .ri { display: flex; flex-direction: column; min-height: 0; }
  .hd { padding: 10px 14px; border-bottom: 1px solid var(--line); flex: none; }
  .hint { padding: 12px 14px; }
  .fields { padding: 6px 0; overflow: auto; }
  .f { display: grid; grid-template-columns: 110px minmax(0, 1fr); gap: 8px; padding: 5px 14px; align-items: baseline; }
  .k { color: var(--faint); }
  .v { min-width: 0; }
  .v.null { color: var(--faint); }
  .link { color: var(--accent-text); text-align: left; }
  .link:hover { text-decoration: underline; }
  .refs { white-space: normal; }
  .sep { margin: 0 6px; }
</style>
