const docsNav = [
  {
    group: 'Release',
    items: [
      { title: 'Overview', path: 'README.md', icon: '🌈', fallbacks: ['../README.md'] },
      { title: 'Brand kit', path: 'BRAND.md', icon: '🎨' },
      { title: 'Charts', path: 'CHARTS.md', icon: '📊' },
      { title: 'Release notes', path: 'RELEASE_NOTES.md', icon: '🎉', fallbacks: ['../RELEASE_NOTES.md'] },
      { title: 'Changelog', path: 'CHANGELOG.md', icon: '🗓️', fallbacks: ['../CHANGELOG.md'] },
      { title: 'GitHub Pages', path: 'GITHUB_PAGES.md', icon: '🚀' }
    ]
  },
  {
    group: 'API and Operations',
    items: [
      { title: 'API', path: 'API.md', icon: '🔌' },
      { title: 'OpenAPI YAML', path: 'openapi.yaml', icon: '🧾', fallbacks: ['../api/openapi.yaml'], format: 'code' },
      { title: 'Configuration', path: 'CONFIGURATION.md', icon: '⚙️' },
      { title: 'Production deployment', path: 'PRODUCTION_DEPLOYMENT.md', icon: '🚢' },
      { title: 'Operations', path: 'OPERATIONS.md', icon: '🧰' },
      { title: 'Backup and restore', path: 'BACKUP_RESTORE.md', icon: '💾' }
    ]
  },
  {
    group: 'Architecture',
    items: [
      { title: 'Architecture', path: 'ARCHITECTURE.md', icon: '🏗️' },
      { title: 'State machines', path: 'STATE_MACHINES.md', icon: '🔀' },
      { title: 'Scheduler', path: 'SCHEDULER.md', icon: '🧭' },
      { title: 'Reconciler', path: 'RECONCILER.md', icon: '🔁' },
      { title: 'Rollouts', path: 'ROLLOUTS.md', icon: '🚀' },
      { title: 'HA control plane', path: 'HA_CONTROL_PLANE_DESIGN.md', icon: '👑' }
    ]
  },
  {
    group: 'Runtime',
    items: [
      { title: 'Agent', path: 'AGENT.md', icon: '🤖' },
      { title: 'Service spec', path: 'SERVICE_SPEC.md', icon: '📦' },
      { title: 'Resources', path: 'RESOURCES.md', icon: '🧮' },
      { title: 'Health checks', path: 'HEALTHCHECKS.md', icon: '🫀' },
      { title: 'Logs', path: 'LOGS.md', icon: '📜' },
      { title: 'Events', path: 'EVENTS.md', icon: '⚡' }
    ]
  },
  {
    group: 'Networking and Security',
    items: [
      { title: 'Networking', path: 'NETWORKING.md', icon: '🕸️' },
      { title: 'Service discovery', path: 'SERVICE_DISCOVERY.md', icon: '🔎' },
      { title: 'Traefik', path: 'TRAEFIK.md', icon: '🚦' },
      { title: 'Secrets', path: 'SECRETS.md', icon: '🔐' },
      { title: 'Registries', path: 'REGISTRIES.md', icon: '🗃️' },
      { title: 'Security', path: 'SECURITY.md', icon: '🛡️' },
      { title: 'Security review', path: 'SECURITY_REVIEW.md', icon: '🧪' }
    ]
  },
  {
    group: 'Reliability',
    items: [
      { title: 'Observability', path: 'OBSERVABILITY.md', icon: '📈' },
      { title: 'Reliability', path: 'RELIABILITY.md', icon: '🪢' },
      { title: 'Autoscaling', path: 'AUTOSCALING.md', icon: '📊' },
      { title: 'Autoscaling design', path: 'AUTOSCALING_DESIGN.md', icon: '🧠' },
      { title: 'Load testing', path: 'LOAD_TESTING.md', icon: '🏋️' },
      { title: 'Chaos testing', path: 'CHAOS_TESTING.md', icon: '🌪️' },
      { title: 'Development', path: 'DEVELOPMENT.md', icon: '🛠️' }
    ]
  }
]

const allDocs = docsNav.flatMap((section) => section.items.map((item) => ({ ...item, group: section.group })))
const pagesBaseURL = 'https://alekpopovic.github.io/orch/'
const nav = document.querySelector('#nav')
const content = document.querySelector('#doc-content')
const meta = document.querySelector('#doc-meta')
const toc = document.querySelector('#toc')
const tocPanel = document.querySelector('#toc-panel')
const readingProgress = document.querySelector('#reading-progress')
const search = document.querySelector('#doc-search')
const sidebar = document.querySelector('#sidebar')
const backdrop = document.querySelector('#sidebar-backdrop')
const openButton = document.querySelector('#sidebar-open')
const closeButton = document.querySelector('#sidebar-close')
const themeSelect = document.querySelector('#theme-select')
const themeSelectMobile = document.querySelector('#theme-select-mobile')
const themeMedia = window.matchMedia('(prefers-color-scheme: dark)')

function applyTheme(mode) {
  const selected = mode || localStorage.getItem('orch-docs-theme') || 'auto'
  const shouldUseDark = selected === 'dark' || (selected === 'auto' && themeMedia.matches)
  document.documentElement.classList.toggle('dark', shouldUseDark)
  themeSelect.value = selected
  themeSelectMobile.value = selected
}

function setTheme(mode) {
  localStorage.setItem('orch-docs-theme', mode)
  applyTheme(mode)
  if (content.querySelector('.mermaid')) {
    loadDoc()
  }
}

function normalizeRoute(route) {
  return routeParts(route).path
}

function routeParts(route) {
  const raw = decodeURIComponent((route || '').replace(/^#/, '')).trim()
  if (!raw) return { path: 'README.md', anchor: '' }
  const [path, ...anchorParts] = raw.split('#')
  return { path: path || 'README.md', anchor: anchorParts.join('#') }
}

function pageURL(path) {
  return `${pagesBaseURL}#${encodeURIComponent(path)}`
}

function activeDoc() {
  const route = normalizeRoute(window.location.hash)
  return allDocs.find((item) => item.path === route) || allDocs.find((item) => item.path.endsWith(route)) || allDocs[0]
}

function renderNav(filter = '') {
  const normalizedFilter = filter.trim().toLowerCase()
  const current = activeDoc().path
  nav.innerHTML = docsNav
    .map((section) => {
      const items = section.items.filter((item) => {
        if (!normalizedFilter) return true
        return `${section.group} ${item.title} ${item.path}`.toLowerCase().includes(normalizedFilter)
      })
      if (!items.length) return ''
      return `
        <section>
          <h2 class="px-3 text-xs font-bold uppercase tracking-[0.22em] text-slate-400 dark:text-slate-500">${escapeHTML(section.group)}</h2>
          <div class="mt-2 space-y-1">
            ${items.map((item) => `
              <a class="nav-link ${item.path === current ? 'active' : ''}" href="${pageURL(item.path)}">
                <span class="grid h-6 w-6 place-items-center rounded-lg bg-slate-100 text-sm dark:bg-slate-900">${escapeHTML(item.icon || '•')}</span>
                <span>${escapeHTML(item.title)}</span>
              </a>
            `).join('')}
          </div>
        </section>
      `
    })
    .join('')
}

async function fetchFirst(paths) {
  const attempts = Array.from(new Set(paths))
  let lastError
  for (const path of attempts) {
    try {
      const response = await fetch(path, { cache: 'no-cache' })
      if (response.ok) {
        return { path, text: await response.text() }
      }
      lastError = new Error(`${response.status} ${response.statusText}`)
    } catch (error) {
      lastError = error
    }
  }
  throw lastError || new Error('No document paths configured')
}

async function loadDoc() {
  const doc = activeDoc()
  const route = routeParts(window.location.hash)
  renderNav(search.value)
  meta.innerHTML = `
    <span class="rounded-full bg-slate-100 px-3 py-1 dark:bg-slate-900">${escapeHTML(doc.group)}</span>
    <span>${escapeHTML(doc.path)}</span>
    <a class="doc-action" href="${pageURL(doc.path)}">↗ GitHub Pages</a>
    <button class="doc-action" type="button" data-copy-page-link="${pageURL(doc.path)}">⛓ Copy link</button>
  `
  content.innerHTML = `<div class="animate-pulse space-y-4"><div class="h-8 w-2/3 rounded bg-slate-200 dark:bg-slate-800"></div><div class="h-4 w-full rounded bg-slate-200 dark:bg-slate-800"></div><div class="h-4 w-5/6 rounded bg-slate-200 dark:bg-slate-800"></div></div>`
  try {
    const paths = [doc.path, ...(doc.fallbacks || [])]
    const loaded = await fetchFirst(paths)
    document.title = `${doc.title} · orch docs`
    if (doc.format === 'code') {
      content.innerHTML = `<h1>${escapeHTML(doc.title)}</h1><pre><code>${escapeHTML(loaded.text)}</code></pre>`
    } else if (window.marked) {
      marked.setOptions({ gfm: true, headerIds: true, mangle: false })
      content.innerHTML = marked.parse(loaded.text)
    } else {
      content.innerHTML = `<pre><code>${escapeHTML(loaded.text)}</code></pre>`
    }
    wireContentLinks()
    wireCopyLink()
    renderTOC()
    await renderMermaidDiagrams()
    updateReadingProgress()
    closeSidebar()
    if (route.anchor) {
      document.getElementById(route.anchor)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    } else {
      window.scrollTo({ top: 0, behavior: 'smooth' })
    }
  } catch (error) {
    content.innerHTML = `
      <h1>Document unavailable</h1>
      <p>Could not load <code>${escapeHTML(doc.path)}</code>.</p>
      <pre><code>${escapeHTML(error.message || String(error))}</code></pre>
    `
  }
}

function wireCopyLink() {
  meta.querySelector('[data-copy-page-link]')?.addEventListener('click', async (event) => {
    const button = event.currentTarget
    const link = button.getAttribute('data-copy-page-link')
    try {
      await navigator.clipboard.writeText(link)
      button.textContent = '✅ Copied'
      setTimeout(() => {
        button.textContent = '⛓ Copy link'
      }, 1600)
    } catch {
      window.prompt('Copy GitHub Pages link', link)
    }
  })
}

function renderTOC() {
  const headings = Array.from(content.querySelectorAll('h2, h3'))
  if (!headings.length) {
    tocPanel.classList.add('xl:hidden')
    toc.innerHTML = ''
    return
  }
  tocPanel.classList.remove('xl:hidden')
  toc.innerHTML = headings
    .map((heading) => {
      if (!heading.id) {
        heading.id = slugify(heading.textContent)
      }
      const depth = heading.tagName === 'H3' ? 'depth-3' : 'depth-2'
      return `<a class="toc-link ${depth}" href="${pageURL(activeDoc().path)}#${encodeURIComponent(heading.id)}">${escapeHTML(heading.textContent)}</a>`
    })
    .join('')
}

async function renderMermaidDiagrams() {
  if (!window.mermaid) return
  const blocks = Array.from(content.querySelectorAll('pre code.language-mermaid'))
  if (!blocks.length) return
  mermaid.initialize({
    startOnLoad: false,
    securityLevel: 'strict',
    theme: document.documentElement.classList.contains('dark') ? 'dark' : 'default',
    themeVariables: {
      primaryColor: '#dbeafe',
      primaryBorderColor: '#2374e1',
      primaryTextColor: '#0f172a',
      lineColor: '#64748b',
      secondaryColor: '#ede9fe',
      tertiaryColor: '#ecfeff'
    }
  })
  blocks.forEach((block) => {
    const diagram = document.createElement('div')
    diagram.className = 'mermaid'
    diagram.textContent = block.textContent
    block.closest('pre').replaceWith(diagram)
  })
  await mermaid.run({ nodes: content.querySelectorAll('.mermaid') })
}

function wireContentLinks() {
  content.querySelectorAll('a[href]').forEach((link) => {
    const href = link.getAttribute('href') || ''
    if (href.startsWith('http') || href.startsWith('#') || href.startsWith('mailto:')) return
    const cleaned = href.replace(/^\.\//, '').replace(/^docs\//, '').replace(/^api\//, '')
    const target = allDocs.find((item) => item.path === cleaned || item.path === cleaned.split('#')[0])
    if (target) {
      const anchor = cleaned.includes('#') ? `#${cleaned.split('#').slice(1).join('#')}` : ''
      link.setAttribute('href', `${pageURL(target.path)}${anchor}`)
    }
  })
}

function slugify(value) {
  return String(value)
    .toLowerCase()
    .trim()
    .replace(/[`'"().,:/]/g, '')
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-')
}

function updateReadingProgress() {
  const max = document.documentElement.scrollHeight - window.innerHeight
  const percent = max > 0 ? Math.min(100, Math.max(0, (window.scrollY / max) * 100)) : 0
  readingProgress.style.width = `${percent}%`
}

function escapeHTML(value) {
  return String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#039;')
}

function openSidebar() {
  sidebar.classList.remove('-translate-x-full')
  backdrop.classList.remove('hidden')
}

function closeSidebar() {
  sidebar.classList.add('-translate-x-full')
  backdrop.classList.add('hidden')
}

themeSelect.addEventListener('change', (event) => setTheme(event.target.value))
themeSelectMobile.addEventListener('change', (event) => setTheme(event.target.value))
themeMedia.addEventListener('change', () => applyTheme())
search.addEventListener('input', (event) => renderNav(event.target.value))
window.addEventListener('hashchange', loadDoc)
window.addEventListener('scroll', updateReadingProgress, { passive: true })
window.addEventListener('resize', updateReadingProgress)
openButton.addEventListener('click', openSidebar)
closeButton.addEventListener('click', closeSidebar)
backdrop.addEventListener('click', closeSidebar)

applyTheme()
renderNav()
loadDoc()
