// Automated accessibility check of the VibeMDM console.
//
// Against a running server (fresh database), it signs in, fills the console with simulated Macs and
// iPhones and a few groups, scripts and policies, then runs axe-core (WCAG 2.0–2.2 A/AA plus best
// practice) on every console route in light and dark theme, and checks that every page has its own
// title. Results are compared with baseline.json: a rule failing on a route where it did not fail
// before fails the run. Fixed entries are reported so the baseline can be tightened.
//
//   node scan.mjs --base http://127.0.0.1:8080 --admin-password-file pw [--seed] [--update-baseline]
//
// Seeding needs VIBEMDM_SIM (path to vibemdm-sim) and A11Y_PUSH_CERT / A11Y_PUSH_KEY (any
// certificate whose subject has a UID; pushes are never sent).
import { chromium } from 'playwright'
import { AxeBuilder } from '@axe-core/playwright'
import { spawn } from 'node:child_process'
import { appendFileSync, readFileSync, writeFileSync } from 'node:fs'
import { randomBytes } from 'node:crypto'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const arg = (name, def) => {
  const i = process.argv.indexOf(name)
  return i > 0 ? process.argv[i + 1] : def
}
const flag = (name) => process.argv.includes(name)
const BASE = arg('--base', 'http://127.0.0.1:8080').replace(/\/$/, '')
const BASELINE = join(here, 'baseline.json')
const REPORT = arg('--report', join(here, 'a11y-report.json'))
const TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice']

// ---- console API (cookie session) ----
let cookie = ''
async function api(method, path, body) {
  const res = await fetch(BASE + '/api/v1' + path, {
    method,
    headers: { 'Content-Type': 'application/json', 'X-Requested-With': 'XMLHttpRequest', ...(cookie && { Cookie: cookie }) },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const set = res.headers.getSetCookie?.() ?? []
  if (set.length) cookie = set.map((c) => c.split(';')[0]).join('; ')
  const text = await res.text()
  if (!res.ok) throw new Error(`${method} ${path}: ${res.status} ${text.slice(0, 200)}`)
  return text ? JSON.parse(text) : {}
}
const items = (r) => r.items ?? r

async function signIn() {
  let password = readFileSync(arg('--admin-password-file'), 'utf8').trim()
  await api('POST', '/auth/login', { username: 'admin', password })
  const me = await api('GET', '/auth/me')
  if (me.must_change_password) {
    const next = randomBytes(18).toString('base64url')
    await api('POST', '/auth/password', { current_password: password, new_password: next })
    password = next
    // Keep the new password, so the scan can run again against the same server.
    writeFileSync(arg('--admin-password-file'), password)
    await api('POST', '/auth/login', { username: 'admin', password })
  }
  return password
}

async function seed() {
  const sim = process.env.VIBEMDM_SIM
  await api('PUT', '/settings/apns', { cert_pem: readFileSync(process.env.A11Y_PUSH_CERT, 'utf8'), key_pem: readFileSync(process.env.A11Y_PUSH_KEY, 'utf8') })
  const settings = await api('GET', '/settings')
  await api('PUT', '/settings', { ...settings, org_name: 'Northwind Studio', remote_shell_enabled: true })
  const challenge = (await api('GET', '/settings/scep')).challenge
  const token = (await api('GET', '/settings')).agent_enrollment_secret
  const run = (args) => spawn(sim, ['-server', BASE, ...args], { stdio: 'ignore', detached: true }).unref()
  run(['-n', '8', '-prefix', 'NW-MAC', '-checkin', '15s', '-poll', '5s', '-scep-challenge', challenge, '-token', token])
  run(['-n', '3', '-platform', 'iphone', '-prefix', 'NW-IPHONE', '-scep-challenge', challenge])
  const c = (field, value) => ({ and_or: 'and', field, operator: 'is', value, open_paren: false, close_paren: false })
  await api('POST', '/groups', { name: 'Apple silicon', description: 'M-series Macs', is_smart: true, criteria: [c('architecture', 'arm64')] })
  await api('POST', '/groups', { name: 'FileVault off', description: 'Macs that still need encryption', is_smart: true, criteria: [c('filevault_enabled', 'false')] })
  const script = await api('POST', '/scripts', { name: 'Clear user caches', category: 'Maintenance', info: '', notes: '', priority: 'after', parameter_labels: ['Prefix'], contents: '#!/bin/zsh\necho ok\n', os_requirements: '' })
  const sel = () => ({ computer_ids: [], group_ids: [], buildings: [], departments: [], network_segments: [], users: [] })
  await api('POST', '/policies', {
    name: 'Weekly cache cleanup', enabled: true, category: 'Maintenance',
    triggers: { startup: false, login: false, checkin: true, enrollment_complete: false, network_change: false, custom: '' },
    frequency: 'Once every week', scope: { all_computers: true, targets: sel(), limitations: {}, exclusions: sel() },
    scripts: [{ script_id: script.id, priority: 'after', parameters: [] }], packages: [], apps: [], accounts: [], run_command: '',
    update_inventory: false, reboot: {}, user_interaction: {}, self_service: {},
  })
  // Give the simulated devices time to enroll and report inventory.
  for (let i = 0; i < 60; i++) {
    const n = items(await api('GET', '/computers')).length + items(await api('GET', '/mobile-devices')).length
    if (n >= 11) break
    await new Promise((r) => setTimeout(r, 1000))
  }
}

// ---- routes ----
// Every static route in App.tsx, plus one record of each kind (dynamic segments by template).
function staticRoutes() {
  const app = readFileSync(join(here, '../../web/src/App.tsx'), 'utf8')
  const paths = [...app.matchAll(/path="(\/[^"]*)"/g)].map((m) => m[1])
  return [...new Set(paths)].filter((p) => !p.includes(':') && !p.includes('*') && p !== '/login')
}
async function recordRoutes() {
  const first = async (path) => items(await api('GET', path))[0]?.id
  const out = []
  const add = async (template, list) => {
    const id = await first(list)
    if (id) out.push({ template, path: template.replace(':id', id) })
  }
  await add('/computers/:id', '/computers')
  await add('/mobile-devices/:id', '/mobile-devices')
  await add('/policies/:id', '/policies')
  await add('/groups/:id', '/groups')
  await add('/scripts/:id', '/scripts')
  out.push({ template: '/settings/users', path: '/settings/users' })
  return out
}

// ---- scan ----
async function scan(password) {
  const routes = [...staticRoutes().map((p) => ({ template: p, path: p })), ...(await recordRoutes())]
  const browser = await chromium.launch()
  const found = new Map() // "rule route" -> { rule, route, impact, help, themes:Set, nodes, example }
  const titles = new Map()
  const note = (rule, route, impact, help, theme, nodes, example) => {
    const k = `${rule} ${route}`
    const f = found.get(k) ?? { rule, route, impact, help, themes: new Set(), nodes: 0, example }
    f.themes.add(theme)
    f.nodes += nodes
    found.set(k, f)
  }
  const audit = async (page, route, theme) => {
    const r = await new AxeBuilder({ page }).withTags(TAGS).analyze()
    for (const v of r.violations) note(v.id, route, v.impact, v.help, theme, v.nodes.length, v.nodes[0]?.target?.join(' '))
  }
  const settle = async (page) => {
    await page.waitForLoadState('load')
    await page.waitForTimeout(1200)
  }
  for (const theme of ['light', 'dark']) {
    const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, colorScheme: theme, bypassCSP: true })
    await ctx.addInitScript((t) => localStorage.setItem('vibemdm-theme-preference', t), theme)
    const page = await ctx.newPage()
    if (theme === 'light') {
      for (const p of ['/login', '/enroll']) {
        await page.goto(BASE + p)
        await settle(page)
        await audit(page, p, theme)
      }
    }
    const res = await page.request.post(BASE + '/api/v1/auth/login', { data: { username: 'admin', password }, headers: { 'X-Requested-With': 'XMLHttpRequest' } })
    if (!res.ok()) throw new Error(`browser sign-in failed: ${res.status()}`)
    for (const r of routes) {
      await page.goto(BASE + r.path)
      await settle(page)
      await audit(page, r.template, theme)
      if (theme === 'light') titles.set(r.template, await page.title())
    }
    await ctx.close()
  }
  await browser.close()
  // Custom check: every page has its own browser title (WCAG 2.4.2).
  for (const [route, title] of titles) {
    if (!title || title === 'VibeMDM') note('vibemdm-page-title', route, 'serious', 'Page has a title of its own', 'light', 1, title)
  }
  return { routes: routes.length, found: [...found.values()].map((f) => ({ ...f, themes: [...f.themes].sort() })) }
}

// ---- compare with the baseline ----
function compare(result) {
  let baseline = []
  try {
    baseline = JSON.parse(readFileSync(BASELINE, 'utf8')).known
  } catch {
    // no baseline yet: everything is new
  }
  const key = (f) => `${f.rule} ${f.route}`
  const known = new Set(baseline.map(key))
  const now = new Set(result.found.map(key))
  return {
    added: result.found.filter((f) => !known.has(key(f))),
    fixed: baseline.filter((b) => !now.has(key(b))),
  }
}

function summary(result, diff) {
  const byImpact = {}
  for (const f of result.found) byImpact[f.impact] = (byImpact[f.impact] ?? 0) + 1
  const lines = [
    `## Accessibility check`,
    '',
    `${result.routes} console routes plus login and enrollment, light and dark theme, axe-core WCAG 2.2 AA.`,
    '',
    `Known issues still open: ${result.found.length - diff.added.length} (${Object.entries(byImpact).map(([k, v]) => `${v} ${k}`).join(', ') || 'none'}).`,
    '',
  ]
  if (diff.added.length) {
    lines.push(`### ❌ New problems (${diff.added.length})`, '', '| Rule | Route | Impact | Themes | Example |', '| --- | --- | --- | --- | --- |')
    for (const f of diff.added) lines.push(`| \`${f.rule}\` ${f.help} | \`${f.route}\` | ${f.impact} | ${f.themes.join(', ')} | \`${(f.example ?? '').slice(0, 80)}\` |`)
    lines.push('')
  } else {
    lines.push('No new accessibility problems.', '')
  }
  if (diff.fixed.length) {
    lines.push(`### ✅ Fixed since the baseline (${diff.fixed.length})`, '', 'Run `npm run scan -- … --update-baseline` and commit `tests/a11y/baseline.json` to lock these in.', '')
    for (const f of diff.fixed) lines.push(`- \`${f.rule}\` on \`${f.route}\``)
  }
  return lines.join('\n')
}

const password = await signIn()
if (flag('--seed')) await seed()
const result = await scan(password)
const diff = compare(result)
writeFileSync(REPORT, JSON.stringify({ ...result, ...diff }, null, 1))
const md = summary(result, diff)
console.log(md)
if (process.env.GITHUB_STEP_SUMMARY) appendFileSync(process.env.GITHUB_STEP_SUMMARY, md + '\n')
if (flag('--update-baseline')) {
  const known = result.found.map(({ rule, route, impact, help }) => ({ rule, route, impact, help })).sort((a, b) => (a.rule + a.route).localeCompare(b.rule + b.route))
  writeFileSync(BASELINE, JSON.stringify({ note: 'Known accessibility problems, removed as they are fixed. Never add entries by hand: fix new problems instead.', known }, null, 1) + '\n')
  console.log(`\nbaseline.json updated: ${known.length} known entries`)
} else if (diff.added.length) {
  process.exitCode = 1
}
