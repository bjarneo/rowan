// The stage holds the app in an iframe and draws the camera, the captions,
// the title cards and the pointer. render(t) computes every frame from the
// time t alone, so the recorder can capture frames at any speed.
;(() => {
  const W = 1920, H = 1080, WIN_W = 1440, WIN_H = 900
  const $ = (id) => document.getElementById(id)
  const ease = {
    linear: (x) => x,
    out: (x) => 1 - Math.pow(1 - x, 3),
    inOut: (x) => (x < 0.5 ? 4 * x * x * x : 1 - Math.pow(-2 * x + 2, 3) / 2),
    spring: (x) => 1 - Math.cos(x * Math.PI * 2.2) * Math.exp(-6 * x),
  }

  // Each actor has numeric properties. A tween moves them over time.
  const actors = {
    // The camera looks at (cx, cy) in window pixels and puts it at (fx, fy) on screen.
    cam: { cx: WIN_W / 2, cy: WIN_H / 2, k: 0.9, fx: W / 2, fy: 505 },
    win: { o: 0, dy: 60, s: 0.96 },
    intro: { o: 0, dy: 24 },
    outro: { o: 0, dy: 24 },
    cur: { x: 900, y: 700, o: 0, press: 0 },
    scrim: { o: 1 },
  }
  const tweens = []
  const caps = [{ t: -1, html: '' }]

  function value(name, key, t) {
    let v = actors[name][key]
    for (const tw of tweens) {
      if (tw.name !== name || !(key in tw.to) || tw.start > t) continue
      const from = tw.from[key] ?? v
      const p = tw.dur <= 0 ? 1 : Math.min(1, (t - tw.start) / tw.dur)
      v = from + (tw.to[key] - from) * ease[tw.ease](p)
    }
    return v
  }
  function props(name, t) {
    const o = {}
    for (const k in actors[name]) o[k] = value(name, k, t)
    return o
  }

  function camMatrix(t) {
    const c = props('cam', t)
    const w = props('win', t)
    const k = c.k * w.s
    return { k, x: c.fx - c.cx * k, y: c.fy - c.cy * k + w.dy, o: w.o, cam: c }
  }

  function captionAt(t) {
    let i = 0
    while (i + 1 < caps.length && caps[i + 1].t <= t) i++
    const cur = caps[i], prev = caps[i - 1]
    const d = t - cur.t
    if (prev && d < 0.2) return { html: prev.html, o: 1 - ease.out(d / 0.2), dy: 0 }
    const p = Math.min(1, Math.max(0, (d - 0.2) / 0.35))
    return { html: cur.html, o: ease.out(p), dy: 18 * (1 - ease.out(p)) }
  }

  let lastCap = null
  window.stage = {
    W, H, WIN_W, WIN_H,
    get doc() { return $('app').contentDocument },
    get win() { return $('app').contentWindow },
    /** Loads the app and resolves when its UI is ready. */
    load(url, ready) {
      return new Promise((res) => {
        const f = $('app')
        f.onload = () => {
          const style = f.contentDocument.createElement('style')
          // Real-time CSS animations would not match the frame clock.
          style.textContent = '*, *::before, *::after { transition: none !important; animation: none !important; caret-color: auto; }'
          f.contentDocument.head.append(style)
          const poll = () => (f.contentDocument.querySelector(ready) ? res(true) : setTimeout(poll, 40))
          poll()
        }
        f.src = url
      })
    },
    set(name, to) { Object.assign(actors[name], to) },
    tween(name, to, start, dur, e = 'inOut') {
      // The start value is the value at the start time, so tweens chain.
      const from = {}
      for (const k in to) from[k] = value(name, k, start)
      tweens.push({ name, to, from, start, dur, ease: e })
    },
    caption(html, t) { caps.push({ t, html }) },
    /** The rect of an element in the app, in window pixels. */
    rect(sel, text) {
      const els = [...this.doc.querySelectorAll(sel)]
      const el = text ? els.find((e) => e.textContent.includes(text)) : els[0]
      if (!el) return null
      const r = el.getBoundingClientRect()
      return { x: r.left, y: r.top, w: r.width, h: r.height }
    },
    /** The camera that fits a rect into the area above the caption. */
    fit(r, { fill = 0.84, max = 2.6, fy = 470, areaH = 800 } = {}) {
      const k = Math.min(max, (W * fill) / r.w, (areaH * fill) / r.h)
      return { cx: r.x + r.w / 2, cy: r.y + r.h / 2, k, fy }
    },
    render(t) {
      const m = camMatrix(t)
      const world = $('world')
      world.style.transform = `translate(${m.x}px, ${m.y}px) scale(${m.k})`
      world.style.opacity = m.o
      for (const id of ['intro', 'outro']) {
        const p = props(id, t)
        const el = $(id)
        el.style.opacity = p.o
        el.style.transform = `translateY(${p.dy}px)`
        el.style.visibility = p.o < 0.005 ? 'hidden' : 'visible'
      }
      const c = props('cur', t)
      const cur = $('cursor')
      cur.style.transform = `translate(${c.x}px, ${c.y}px) scale(${(1 - 0.12 * c.press) / m.k})`
      cur.style.opacity = c.o
      $('ring').style.opacity = c.press * 0.9
      $('ring').style.transform = `scale(${0.6 + 0.6 * c.press})`
      $('scrim').style.opacity = props('scrim', t).o
      const cap = captionAt(t)
      if (cap.html !== lastCap) { $('cap').innerHTML = cap.html; lastCap = cap.html }
      $('cap').style.opacity = cap.o
      $('cap').style.transform = `translateY(${cap.dy}px)`
    },
  }
})()
