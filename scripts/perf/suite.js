// The UI steps of the benchmark. bench.mjs runs this file as the body of an
// async function in the page, in Chromium or in WebKitGTK. It returns one
// entry per step: the time until the frame after the step completes.
const out = []
const $ = (s) => document.querySelector(s)
const $$ = (s) => [...document.querySelectorAll(s)]
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
const nodes = () => document.getElementsByTagName('*').length

/** Resolves after the frame that follows cond() turning true. */
const wait = (cond, t0, ms = 120000) =>
  new Promise((res, rej) => {
    const tick = () => {
      if (cond()) return requestAnimationFrame(() => setTimeout(() => res(performance.now() - t0)))
      if (performance.now() - t0 > ms) return rej(new Error('timed out'))
      requestAnimationFrame(tick)
    }
    tick()
  })

async function step(name, act, cond = () => true, t0 = performance.now()) {
  try {
    await act()
    out.push({ step: name, ms: await wait(cond, t0), note: `${nodes()} nodes` })
  } catch (e) {
    const status = $('.status')?.textContent.trim().replace(/\s+/g, ' ')
    out.push({ step: name, ms: -1, note: `failed: ${e.message} · status: ${status}` })
  }
}

/** Runs act n times and records the mean and the worst time. */
async function repeat(name, n, act) {
  let sum = 0
  let worst = 0
  try {
    for (let i = 0; i < n; i++) {
      const t0 = performance.now()
      await act(i)
      const ms = await wait(() => true, t0)
      sum += ms
      worst = Math.max(worst, ms)
      await sleep(30)
    }
  } catch (e) {
    out.push({ step: name, ms: -1, note: `failed: ${e.message}` })
    return
  }
  out.push({ step: name, ms: sum / n, note: `mean of ${n} · worst ${worst.toFixed(0)} ms` })
}

const typeInto = (name, sel, text) =>
  repeat(name, text.length, (i) => {
    const el = $(sel)
    el.focus()
    document.execCommand('insertText', false, text[i])
  })

const frames = (n = 2) => new Promise((r) => { const f = () => (--n ? requestAnimationFrame(f) : r()); requestAnimationFrame(f) })
const setFilter = async (v) => {
  const el = $('.tfilter')
  el.focus()
  el.select()
  if (v) document.execCommand('insertText', false, v)
  else document.execCommand('delete')
  await frames()
}
/** The sidebar renders only the rows in view, so filter it to find a table. */
const tableBtn = async (name) => {
  await setFilter(name)
  return $$('.tables .tr').find((e) => e.title === name)
}
// The sidebar marks the table when its result arrives.
const openTable = async (name) => {
  const btn = await tableBtn(name)
  await step(`open ${name} from the sidebar`, () => btn.click(), () => $('.tr.on')?.title === name && $('.status .ok') && $$('.grid .r').length > 5)
  await setFilter('')
}

// ---- Start ----
// performance.now() counts from the start of the navigation.
await step('load the page until the sidebar shows tables', () => {}, () => $$('.tables .tr').length > 20, 0)
await sleep(400)

await typeInto('type "order" in the table filter', '.tfilter', 'order')
await step('clear the table filter', () => {
  const el = $('.tfilter')
  el.select()
  document.execCommand('delete')
}, () => $$('.tables .tr').length > 20 && !$('.tfilter').value)
await sleep(300)
await repeat('scroll the sidebar', 20, (i) => {
  $('.tables .tl').scrollTop = (i + 1) * 900
})

await openTable('orders')
await sleep(400)
await openTable('wide_metrics')
await sleep(400)
await repeat('scroll the wide grid down', 30, (i) => {
  $('.grid').scrollTop = (i + 1) * 84
})
await repeat('scroll the wide grid right', 10, (i) => {
  $('.grid').scrollLeft = (i + 1) * 600
})
await step('select a row in the wide grid', () => $$('.grid .r')[4].querySelector('.c[data-j]:not([data-j="-1"])')?.click() ?? $$('.grid .r')[4].click(), () => $('.grid .r.sel'))
await repeat('ArrowDown in the wide grid', 15, () => {
  $('.grid').dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
})
await sleep(400)

// Raise the row limit to 10,000, then open the largest table.
$('select.limit').value = '10000'
$('select.limit').dispatchEvent(new Event('change', { bubbles: true }))
await sleep(400)
{
  const btn = await tableBtn('order_items')
  await step('open order_items at 10,000 rows', () => btn.click(), () => $('.tr.on')?.title === 'order_items' && $('.status')?.textContent.includes('10,000'))
  await setFilter('')
}
await sleep(300)
await repeat('scroll the 10,000-row grid', 20, (i) => {
  $('.grid').scrollTop = (i + 1) * 4000
})
await step('select a row in order_items', () => $$('.grid .r')[3].querySelector('.c').click(), () => $('.grid .r.sel'))
$('select.limit').value = '1000'
$('select.limit').dispatchEvent(new Event('change', { bubbles: true }))
await sleep(300)

// The editor. Focus loads the columns for completion.
await step('focus the editor', () => {
  const ta = $('.editor textarea')
  ta.focus()
  ta.select()
  document.execCommand('delete')
})
await sleep(2000)
await typeInto('type a query with completion', '.editor textarea', 'SELECT * FROM orders o WHERE o.st')

// A script of 3,000 lines, as when a migration is pasted.
{
  const line = (i) => `UPDATE orders SET status = 'shipped', note = 'batch ${i}' /* row ${i} */ WHERE id = ${i} AND total_cents > 100;`
  const big = Array.from({ length: 3000 }, (_, i) => line(i)).join('\n')
  const paste = (text) => {
    const ta = $('.editor textarea')
    ta.focus()
    ta.select()
    const dt = new DataTransfer()
    dt.setData('text/plain', text)
    ta.dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }))
  }
  const prev = $('.editor textarea').value
  await step('paste a script of 3,000 lines', () => paste(big), () => $('.editor textarea').value.length === big.length && $('.editor pre').textContent.includes('batch 0'))
  await step('undo the paste', () => {
    $('.editor textarea').dispatchEvent(new KeyboardEvent('keydown', { key: 'z', ctrlKey: true, bubbles: true, cancelable: true }))
  }, () => $('.editor textarea').value === prev)
  await step('redo the paste', () => {
    $('.editor textarea').dispatchEvent(new KeyboardEvent('keydown', { key: 'z', ctrlKey: true, shiftKey: true, bubbles: true, cancelable: true }))
  }, () => $('.editor textarea').value.length === big.length)
  await sleep(300)
  await typeInto('type at the end of the 3,000-line script', '.editor textarea', ' -- note')
  await repeat('scroll the 3,000-line script', 20, (i) => {
    const ta = $('.editor textarea')
    ta.scrollTop = (i + 1) * 2000
    ta.dispatchEvent(new Event('scroll'))
  })
  const ta = $('.editor textarea')
  ta.focus()
  ta.select()
  document.execCommand('delete')
  await sleep(300)
}

// The command layout.
await step('switch to the command layout', () => $('button[title="Switch to the command layout"]').click(), () => $('.cs input'))
await sleep(500)
await typeInto('type "orders refunded" in search', '.cs input', 'orders refunded')
await step('wait for the search preview', () => {}, () => $$('.mini .mr').length > 2)
return out
