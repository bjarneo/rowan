<script lang="ts">
  // A virtualized result grid. It renders only the rows in view, so large
  // results scroll at full frame rate. Rows are not copied: sorting keeps an
  // index array.
  import type { Column, Cell } from '../lib/types'
  import { colWidth } from '../lib/format'
  import { api } from '../lib/api'
  import { app } from '../lib/store.svelte'
  import Icon from './Icon.svelte'

  let {
    columns,
    rows,
    selected = $bindable(-1),
    mono = true,
    gutter = true,
    sortable = true,
    compact = false,
    emptyText = 'No rows',
  }: {
    columns: Column[]
    rows: Cell[][]
    selected?: number
    mono?: boolean
    gutter?: boolean
    sortable?: boolean
    compact?: boolean
    emptyText?: string
  } = $props()

  let viewport: HTMLDivElement
  let scrollTop = $state(0)
  let height = $state(400)
  let sort = $state<{ col: number; dir: 1 | -1 } | null>(null)
  let selCol = $state(-1)

  const rowH = $derived(compact ? 26 : app.layout === 'workspace' ? (app.config?.app.density === 'compact' ? 24 : 28) : app.config?.app.density === 'compact' ? 26 : 30)
  const charW = $derived(mono ? 7.6 : 7.1)
  const widths = $derived(columns.map((c, j) => colWidth(c.name, c.type, sample(j), charW)))
  const gutterW = $derived(gutter ? Math.max(44, String(rows.length).length * 8 + 22) : 0)
  const template = $derived((gutter ? gutterW + 'px ' : '') + widths.map((w) => w + 'px').join(' ') + ' minmax(0,1fr)')
  const totalW = $derived(gutterW + widths.reduce((a, b) => a + b, 0))

  // Reset sorting when a new result arrives.
  let lastRows: Cell[][] | null = null
  $effect.pre(() => {
    if (rows !== lastRows) {
      lastRows = rows
      sort = null
      selCol = -1
      if (viewport) viewport.scrollTop = 0
      scrollTop = 0
    }
  })

  function* sample(j: number) {
    const n = Math.min(rows.length, 80)
    for (let i = 0; i < n; i++) yield rows[i][j]
  }

  const order = $derived.by(() => {
    const idx = Array.from({ length: rows.length }, (_, i) => i)
    if (!sort) return idx
    const { col, dir } = sort
    const num = columns[col]?.numeric
    idx.sort((a, b) => {
      const x = rows[a][col]
      const y = rows[b][col]
      if (x == null) return y == null ? 0 : 1
      if (y == null) return -1
      if (num) return (Number(x) - Number(y)) * dir
      return x < y ? -dir : x > y ? dir : 0
    })
    return idx
  })

  const overscan = 10
  const start = $derived(Math.max(0, Math.floor(scrollTop / rowH) - overscan))
  const end = $derived(Math.min(order.length, Math.ceil((scrollTop + height) / rowH) + overscan))
  const visible = $derived(order.slice(start, end))

  function onScroll() {
    scrollTop = viewport.scrollTop
  }

  function toggleSort(j: number) {
    if (!sortable) return
    sort = !sort || sort.col !== j ? { col: j, dir: 1 } : sort.dir === 1 ? { col: j, dir: -1 } : null
  }

  function onCellClick(e: MouseEvent) {
    const cell = (e.target as HTMLElement).closest<HTMLElement>('[data-j]')
    const row = cell?.parentElement
    if (!cell || !row?.dataset.i) return
    selected = +row.dataset.i
    selCol = +cell.dataset.j!
  }

  function onKey(e: KeyboardEvent) {
    if (!rows.length) return
    const pos = order.indexOf(selected)
    let next = pos
    if (e.key === 'ArrowDown') next = Math.min(order.length - 1, pos + 1)
    else if (e.key === 'ArrowUp') next = Math.max(0, pos - 1)
    else if (e.key === 'PageDown') next = Math.min(order.length - 1, pos + Math.floor(height / rowH))
    else if (e.key === 'PageUp') next = Math.max(0, pos - Math.floor(height / rowH))
    else if (e.key === 'ArrowRight') { selCol = Math.min(columns.length - 1, selCol + 1); e.preventDefault(); return }
    else if (e.key === 'ArrowLeft') { selCol = Math.max(0, selCol - 1); e.preventDefault(); return }
    else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'c') {
      const v = selected >= 0 ? (selCol >= 0 ? rows[selected][selCol] : rows[selected].map((x) => x ?? 'NULL').join('\t')) : null
      if (v != null) {
        api.copy(v)
        app.toast(selCol >= 0 ? 'Copied the cell' : 'Copied the row')
      }
      e.preventDefault()
      return
    } else return
    e.preventDefault()
    selected = order[next]
    const top = next * rowH
    if (top < viewport.scrollTop + rowH) viewport.scrollTop = Math.max(0, top - rowH)
    else if (top + rowH * 2 > viewport.scrollTop + height) viewport.scrollTop = top + rowH * 2 - height
  }
</script>

<div
  class="grid"
  class:mono
  bind:this={viewport}
  bind:clientHeight={height}
  onscroll={onScroll}
  onkeydown={onKey}
  tabindex="0"
  role="grid"
  style="--rh:{rowH}px"
>
  <div class="head" style="grid-template-columns:{template};min-width:{totalW}px" role="row">
    {#if gutter}<span class="g"></span>{/if}
    {#each columns as c, j}
      <button class="h" class:num={c.numeric} onclick={() => toggleSort(j)} title={c.type} role="columnheader">
        <span class="ellipsis">{c.name}</span>
        {#if sort?.col === j}<Icon name={sort.dir > 0 ? 'arrow_upward' : 'arrow_downward'} size={13} />{/if}
      </button>
    {/each}
  </div>
  {#if rows.length === 0}
    <div class="none">{emptyText}</div>
  {:else}
    <div style="height:{order.length * rowH}px;min-width:{totalW}px;position:relative">
      <!-- One delegated handler for every cell keeps the row markup light. -->
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div style="position:absolute;left:0;right:0;top:{start * rowH}px" onclick={onCellClick}>
        {#each visible as i}
          <div class="r" class:sel={i === selected} style="grid-template-columns:{template}" role="row" data-i={i}>
            {#if gutter}<span class="g" data-j="-1">{i + 1}</span>{/if}
            {#each rows[i] as v, j}
              <span class="c" class:num={columns[j].numeric} class:null={v === null} class:cur={i === selected && j === selCol} data-j={j} role="gridcell">{v === null ? 'NULL' : v}</span>
            {/each}
          </div>
        {/each}
      </div>
    </div>
  {/if}
</div>

<style>
  .grid { position: relative; overflow: auto; height: 100%; outline: none; font: 400 12.5px var(--sans); contain: strict; }
  .grid.mono { font-family: var(--mono); font-size: 12.5px; }
  .head {
    position: sticky; top: 0; z-index: 2; display: grid; height: var(--rh); align-items: stretch;
    background: var(--panel); border-bottom: 1px solid var(--line); color: var(--faint);
  }
  .h { display: flex; align-items: center; gap: 4px; padding: 0 10px; min-width: 0; color: var(--muted); font: 500 12px var(--sans); }
  :global(:root[data-layout='workspace']) .grid.mono .h { font: 400 12px var(--mono); color: var(--faint); }
  .h:hover { color: var(--text); background: var(--hover); }
  .h.num { justify-content: flex-end; }
  .r { display: grid; height: var(--rh); align-items: center; border-bottom: 1px solid var(--line); }
  .r:hover { background: var(--panel); }
  .r.sel { background: var(--accent-soft); box-shadow: inset 2px 0 0 var(--accent); }
  .g { text-align: right; padding-right: 12px; color: var(--faint); font: 400 11.5px var(--mono); height: 100%; line-height: var(--rh); }
  .c { padding: 0 10px; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; height: 100%; line-height: var(--rh); user-select: text; -webkit-user-select: text; }
  .c.num { text-align: right; font-variant-numeric: tabular-nums; }
  .c.null { color: var(--faint); }
  .c.cur { box-shadow: inset 0 0 0 1.5px var(--accent); }
  .none { padding: 18px 14px; color: var(--faint); }
</style>
