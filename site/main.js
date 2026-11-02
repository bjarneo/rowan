// Rowan landing page.
// Finds the repository from the page address, links the download for the
// visitor's OS, and runs the layout tabs, the copy buttons, the video control
// and the section highlight in the title bar.
(() => {
  'use strict'

  const root = document.documentElement
  root.classList.add('js')
  const $ = (id) => document.getElementById(id)
  const reduceMotion = matchMedia('(prefers-reduced-motion: reduce)')

  // ---- Repository ----
  // The page lives at {owner}.github.io/{repo}/. On another host, the
  // data-repo attribute on <html> can give "owner/repo".

  function detectRepo() {
    const host = /^([a-z0-9][a-z0-9-]{0,38})\.github\.io$/.exec(location.hostname.toLowerCase())
    if (host) {
      const first = location.pathname.split('/').filter(Boolean)[0] || ''
      if (/^[A-Za-z0-9._-]{1,100}$/.test(first) && !/\.html?$/i.test(first)) {
        return { owner: host[1], repo: first }
      }
    }
    const attr = /^([A-Za-z0-9][A-Za-z0-9-]{0,38})\/([A-Za-z0-9._-]{1,100})$/.exec((root.dataset.repo || '').trim())
    return attr ? { owner: attr[1], repo: attr[2] } : null
  }

  const gh = detectRepo()
  const base = gh ? `https://github.com/${gh.owner}/${gh.repo}` : ''
  const links = gh
    ? {
        repo: base,
        releases: `${base}/releases`,
        docs: `${base}/tree/main/docs`,
        changelog: `${base}/blob/main/CHANGELOG.md`,
      }
    : null

  for (const a of document.querySelectorAll('a[data-link]')) {
    const url = links && links[a.dataset.link]
    if (url) {
      a.href = url
      a.hidden = false
    } else {
      a.removeAttribute('href')
    }
  }
  if (gh) {
    for (const el of document.querySelectorAll('[data-needs-repo]')) el.hidden = false
    $('clone-cmd').textContent = `git clone ${base}.git`
    $('cd-cmd').textContent = `cd ${gh.repo}`
    $('clone-line').hidden = false
    $('cd-line').hidden = false
  }

  // ---- Download ----

  const OS = {
    linux: { label: 'Linux', match: (n) => n.includes('linux') && n.includes('amd64') },
    macos: { label: 'macOS', match: (n) => n.includes('darwin') || n.includes('macos') },
    // Windows has an installer and a zip. The installer comes first.
    windows: { label: 'Windows', match: (n) => n.includes('windows'), prefer: (n) => n.includes('installer') },
  }
  // Checksums and signatures are not the build itself.
  const SKIP = /\.(sha256|sha512|sha1|md5|sig|asc|pem|sbom|spdx|json|txt)$|checksum|sha256sums/i

  function detectOS() {
    const uad = navigator.userAgentData
    const platform = ((uad && uad.platform) || navigator.platform || '').toLowerCase()
    const ua = navigator.userAgent.toLowerCase()
    if ((uad && uad.mobile) || /android|iphone|ipad|ipod/.test(ua)) return null
    if (platform.startsWith('mac') || ua.includes('macintosh')) {
      // iPadOS reports a Mac platform, but it has a touch screen.
      return navigator.maxTouchPoints > 1 ? null : 'macos'
    }
    if (platform.startsWith('win') || ua.includes('windows')) return 'windows'
    if (/linux|x11|cros/.test(`${platform} ${ua}`)) return 'linux'
    return null
  }

  const safeUrl = (u) => (typeof u === 'string' && u.startsWith('https://github.com/') ? u : null)
  function pick(assets, key) {
    const { match, prefer } = OS[key]
    const builds = assets.filter((a) => !SKIP.test(a.name) && match(a.name.toLowerCase()))
    return (prefer && builds.find((a) => prefer(a.name.toLowerCase()))) || builds[0]
  }
  const megabytes = (n) => (n > 0 ? `${(n / 1048576).toFixed(1)} MB` : '')

  function el(tag, props, text) {
    const node = Object.assign(document.createElement(tag), props)
    if (text) node.textContent = text
    return node
  }

  async function latestRelease() {
    const key = `rowan-release:${gh.owner}/${gh.repo}`
    try {
      const cached = JSON.parse(sessionStorage.getItem(key) || 'null')
      if (cached && Date.now() - cached.at < 30 * 60 * 1000) return cached.release
    } catch {}
    const ctl = new AbortController()
    const timer = setTimeout(() => ctl.abort(), 8000)
    try {
      const res = await fetch(`https://api.github.com/repos/${gh.owner}/${gh.repo}/releases/latest`, {
        headers: { Accept: 'application/vnd.github+json' },
        signal: ctl.signal,
      })
      if (!res.ok) throw new Error(`GitHub API returned ${res.status}`)
      const data = await res.json()
      const release = {
        tag: String(data.tag_name || ''),
        url: safeUrl(data.html_url) || links.releases,
        assets: (Array.isArray(data.assets) ? data.assets : [])
          .map((a) => ({ name: String(a.name || ''), url: safeUrl(a.browser_download_url), size: Number(a.size) || 0 }))
          .filter((a) => a.url && a.name),
      }
      try {
        sessionStorage.setItem(key, JSON.stringify({ at: Date.now(), release }))
      } catch {}
      return release
    } finally {
      clearTimeout(timer)
    }
  }

  function showRelease(release, os) {
    const download = $('download')
    const meta = $('download-meta')
    const alt = $('download-alt')

    if (release.tag) {
      $('bar-tag').textContent = release.tag
      $('bar-version').href = release.url
      $('bar-version').hidden = false
    }

    const mine = os ? pick(release.assets, os) : null
    if (mine) {
      download.href = mine.url
      const rest = [release.tag, megabytes(mine.size)].filter(Boolean)
      meta.replaceChildren(el('span', { className: 'tag' }, mine.name), rest.length ? ` · ${rest.join(' · ')}` : '')
    } else {
      download.href = release.url
      meta.textContent = os
        ? `${release.tag || 'The latest release'} has no ${OS[os].label} build. Open the release page to see the builds.`
        : 'Rowan is a desktop app for Linux, macOS and Windows.'
    }

    const others = Object.keys(OS)
      .filter((key) => !mine || key !== os)
      .map((key) => [OS[key].label, pick(release.assets, key)])
      .filter(([, asset]) => asset)
    const parts = [...others.map(([label, asset]) => el('a', { href: asset.url }, label)), el('a', { href: links.releases }, 'all releases')]
    alt.replaceChildren(document.createTextNode(mine ? 'Other builds: ' : 'Builds: '))
    parts.forEach((part, i) => {
      if (i) alt.append(', ')
      alt.append(part)
    })
    alt.append('.')
    alt.hidden = false
  }

  if (gh) {
    const os = detectOS()
    $('download').href = links.releases
    $('download').classList.add('is-download')
    $('download-label').textContent = os ? `Download for ${OS[os].label}` : 'Download Rowan'
    $('download-secondary').hidden = false
    latestRelease().then(
      (release) => showRelease(release, os),
      () => {
        // The button keeps the link to the releases page.
      },
    )
  }

  // ---- Screenshots ----
  // A missing file shows the empty frame instead of a broken image icon.

  for (const img of document.querySelectorAll('.frame img')) {
    const frame = img.closest('.frame')
    img.addEventListener('error', () => frame.classList.add('missing'))
    img.addEventListener('load', () => frame.classList.remove('missing'))
    if (img.complete && img.naturalWidth === 0) frame.classList.add('missing')
  }

  // ---- Promo video ----

  const video = $('promo')
  const toggle = $('promo-toggle')
  if (video && toggle) {
    const label = $('promo-toggle-label')
    let userPlayed = false
    let userPaused = false
    let offscreen = false

    const sync = () => {
      toggle.dataset.state = video.paused ? 'paused' : 'playing'
      label.textContent = video.paused ? 'Play the video' : 'Pause the video'
    }
    const ready = () => {
      toggle.hidden = false
      sync()
    }

    if (reduceMotion.matches) {
      video.autoplay = false
      video.removeAttribute('autoplay')
      video.pause()
    }
    video.addEventListener('play', () => {
      if (reduceMotion.matches && !userPlayed) video.pause()
      sync()
    })
    video.addEventListener('pause', sync)
    video.addEventListener('loadedmetadata', ready)
    video.addEventListener('error', () => {
      toggle.hidden = true
    })
    if (video.readyState >= 1) ready()

    toggle.addEventListener('click', () => {
      if (video.paused) {
        userPlayed = true
        userPaused = false
        video.play().catch(() => {})
      } else {
        userPaused = true
        video.pause()
      }
    })

    // Stop the video when it is out of view, and start it again on return.
    if ('IntersectionObserver' in window) {
      new IntersectionObserver(([entry]) => {
        if (!entry.isIntersecting && !video.paused) {
          offscreen = true
          video.pause()
        } else if (entry.isIntersecting && offscreen && !userPaused) {
          offscreen = false
          video.play().catch(() => {})
        }
      }).observe(video)
    }
  }

  // ---- Layout tabs ----

  const tabs = [...document.querySelectorAll('#layout-tabs [role="tab"]')]
  function selectTab(tab, focus) {
    for (const t of tabs) {
      const on = t === tab
      t.setAttribute('aria-selected', String(on))
      t.tabIndex = on ? 0 : -1
      $(t.getAttribute('aria-controls')).hidden = !on
    }
    if (focus) tab.focus()
  }
  tabs.forEach((tab, i) => {
    const panel = $(tab.getAttribute('aria-controls'))
    panel.setAttribute('role', 'tabpanel')
    panel.setAttribute('aria-labelledby', tab.id)
    panel.tabIndex = 0
    tab.addEventListener('click', () => selectTab(tab))
    tab.addEventListener('keydown', (e) => {
      const keys = { ArrowRight: i + 1, ArrowLeft: i - 1, Home: 0, End: tabs.length - 1 }
      if (!(e.key in keys)) return
      e.preventDefault()
      selectTab(tabs[(keys[e.key] + tabs.length) % tabs.length], true)
    })
  })
  if (tabs.length) selectTab(tabs[0])

  // ---- Copy buttons ----

  const status = $('copy-status')
  for (const btn of document.querySelectorAll('button.copy[data-copy]')) {
    const pre = $(btn.dataset.copy)
    const label = btn.querySelector('.copy-label')
    let timer = 0
    btn.hidden = false
    btn.addEventListener('click', async () => {
      const text = [...pre.querySelectorAll('.cmd')]
        .filter((cmd) => !cmd.closest('[hidden]'))
        .map((cmd) => cmd.textContent)
        .join('\n')
      try {
        await navigator.clipboard.writeText(text)
        btn.dataset.state = 'done'
        label.textContent = 'copied'
        status.textContent = 'Copied to the clipboard.'
      } catch {
        const range = document.createRange()
        range.selectNodeContents(pre)
        const sel = getSelection()
        sel.removeAllRanges()
        sel.addRange(range)
        status.textContent = 'The browser blocked the clipboard. The text is selected. Press Ctrl+C to copy it.'
      }
      clearTimeout(timer)
      timer = setTimeout(() => {
        delete btn.dataset.state
        label.textContent = 'copy'
        status.textContent = ''
      }, 2000)
    })
  }

  // ---- Section highlight in the title bar ----

  const nav = document.querySelector('.bar-nav')
  const navLinks = [...nav.querySelectorAll('a[href^="#"]')]
  const byId = new Map(navLinks.map((a) => [a.getAttribute('href').slice(1), a]))
  if ('IntersectionObserver' in window) {
    const spy = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (!entry.isIntersecting) continue
          for (const a of navLinks) a.removeAttribute('aria-current')
          const a = byId.get(entry.target.id)
          if (!a) continue
          a.setAttribute('aria-current', 'true')
          const left = a.offsetLeft - nav.offsetLeft
          if (left < nav.scrollLeft || left + a.offsetWidth > nav.scrollLeft + nav.clientWidth) {
            nav.scrollTo({ left: left - 24, behavior: reduceMotion.matches ? 'auto' : 'smooth' })
          }
        }
      },
      { rootMargin: '-45% 0px -50% 0px' },
    )
    for (const id of ['top', ...byId.keys()]) {
      const section = $(id)
      if (section) spy.observe(section)
    }
  }
})()
