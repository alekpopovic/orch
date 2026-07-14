const docsNav = [
  {
    group: 'Release',
    items: [
      { title: 'Overview', path: 'README.md', fallbacks: ['../README.md'] },
      { title: 'Release notes', path: 'RELEASE_NOTES.md', fallbacks: ['../RELEASE_NOTES.md'] },
      { title: 'Changelog', path: 'CHANGELOG.md', fallbacks: ['../CHANGELOG.md'] },
      { title: 'GitHub Pages', path: 'GITHUB_PAGES.md' }
    ]
  },
  {
    group: 'API and Operations',
    items: [
      { title: 'API', path: 'API.md' },
      { title: 'OpenAPI YAML', path: 'openapi.yaml', fallbacks: ['../api/openapi.yaml'], format: 'code' },
      { title: 'Configuration', path: 'CONFIGURATION.md' },
      { title: 'Production deployment', path: 'PRODUCTION_DEPLOYMENT.md' },
      { title: 'Operations', path: 'OPERATIONS.md' },
      { title: 'Backup and restore', path: 'BACKUP_RESTORE.md' }
    ]
  },
  {
    group: 'Architecture',
    items: [
      { title: 'Architecture', path: 'ARCHITECTURE.md' },
      { title: 'State machines', path: 'STATE_MACHINES.md' },
      { title: 'Scheduler', path: 'SCHEDULER.md' },
      { title: 'Reconciler', path: 'RECONCILER.md' },
      { title: 'Rollouts', path: 'ROLLOUTS.md' },
      { title: 'HA control plane', path: 'HA_CONTROL_PLANE_DESIGN.md' }
    ]
  },
  {
    group: 'Runtime',
    items: [
      { title: 'Agent', path: 'AGENT.md' },
      { title: 'Service spec', path: 'SERVICE_SPEC.md' },
      { title: 'Resources', path: 'RESOURCES.md' },
      { title: 'Health checks', path: 'HEALTHCHECKS.md' },
      { title: 'Logs', path: 'LOGS.md' },
      { title: 'Events', path: 'EVENTS.md' }
    ]
  },
  {
    group: 'Networking and Security',
    items: [
      { title: 'Networking', path: 'NETWORKING.md' },
      { title: 'Service discovery', path: 'SERVICE_DISCOVERY.md' },
      { title: 'Traefik', path: 'TRAEFIK.md' },
      { title: 'Secrets', path: 'SECRETS.md' },
      { title: 'Registries', path: 'REGISTRIES.md' },
      { title: 'Security', path: 'SECURITY.md' },
      { title: 'Security review', path: 'SECURITY_REVIEW.md' }
    ]
  },
  {
    group: 'Reliability',
    items: [
      { title: 'Observability', path: 'OBSERVABILITY.md' },
      { title: 'Reliability', path: 'RELIABILITY.md' },
      { title: 'Autoscaling', path: 'AUTOSCALING.md' },
      { title: 'Autoscaling design', path: 'AUTOSCALING_DESIGN.md' },
      { title: 'Load testing', path: 'LOAD_TESTING.md' },
      { title: 'Chaos testing', path: 'CHAOS_TESTING.md' },
      { title: 'Development', path: 'DEVELOPMENT.md' }
    ]
  }
]

const allDocs = docsNav.flatMap((section) => section.items.map((item) => ({ ...item, group: section.group })))
const nav = document.querySelector('#nav')
const content = document.querySelector('#doc-content')
const meta = document.querySelector('#doc-meta')
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
}

function normalizeRoute(route) {
  const raw = decodeURIComponent((route || '').replace(/^#/, '')).trim()
  return raw || 'README.md'
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
              <a class="nav-link ${item.path === current ? 'active' : ''}" href="#${encodeURIComponent(item.path)}">
                <span class="h-1.5 w-1.5 rounded-full bg-current opacity-50"></span>
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
  renderNav(search.value)
  meta.innerHTML = `
    <span class="rounded-full bg-slate-100 px-3 py-1 dark:bg-slate-900">${escapeHTML(doc.group)}</span>
    <span>${escapeHTML(doc.path)}</span>
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
    closeSidebar()
    window.scrollTo({ top: 0, behavior: 'smooth' })
  } catch (error) {
    content.innerHTML = `
      <h1>Document unavailable</h1>
      <p>Could not load <code>${escapeHTML(doc.path)}</code>.</p>
      <pre><code>${escapeHTML(error.message || String(error))}</code></pre>
    `
  }
}

function wireContentLinks() {
  content.querySelectorAll('a[href]').forEach((link) => {
    const href = link.getAttribute('href') || ''
    if (href.startsWith('http') || href.startsWith('#') || href.startsWith('mailto:')) return
    const cleaned = href.replace(/^\.\//, '').replace(/^docs\//, '').replace(/^api\//, '')
    const target = allDocs.find((item) => item.path === cleaned || item.path === cleaned.split('#')[0])
    if (target) {
      link.setAttribute('href', `#${encodeURIComponent(target.path)}`)
    }
  })
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
openButton.addEventListener('click', openSidebar)
closeButton.addEventListener('click', closeSidebar)
backdrop.addEventListener('click', closeSidebar)

applyTheme()
renderNav()
loadDoc()
