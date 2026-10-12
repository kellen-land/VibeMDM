#!/usr/bin/env node
// Guardrail for the console's UI rules (docs/ui-guidelines.md). Fails when a page builds its
// own version of something the shared components provide. A line can opt out with a
// trailing comment `ui-allow: <rule>` and a reason.
import { readdirSync, readFileSync } from 'node:fs'
import { join, relative } from 'node:path'

const root = new URL('../src/', import.meta.url).pathname
const rules = [
  {
    id: 'save-button',
    why: 'Saving an object goes through ObjectBar (one Save per object); sections and dialogs use a plain "Save" button with data-shortcut="save".',
    test: (line, file) => /icon=\{<Save\b/.test(line) && !file.startsWith('components/editor/'),
  },
  {
    id: 'raw-table',
    why: 'Lists use the Table component (or the IndexPage template).',
    test: (line, file) => /<table[\s>]/.test(line) && file.startsWith('pages/'),
  },
  {
    id: 'browser-confirm',
    why: 'Confirm with ConfirmDialog or ObjectBar, never window.confirm/alert.',
    test: (line) => /\bwindow\.(confirm|alert)\(/.test(line),
  },
  {
    id: 'title-case-new',
    why: 'Buttons and titles are sentence case ("New policy", not "New Policy").',
    test: (line) => /(newLabel=|>\s*)['"]?New [A-Z][a-z]+/.test(line) && !/New (Mac|Apple|Jamf|Installomator|SCEP|LAPS|ACME|DDM|MDM|API|VPN|Wi-Fi)\b/.test(line),
  },
  {
    id: 'delete-header-button',
    why: 'Delete lives in the ObjectBar (view mode) or in bulk actions, not in page headers.',
    test: (line) => /variant="danger-outline"[^>]*icon=\{<Trash2/.test(line),
  },
]

const files = []
const walk = (d) => {
  for (const e of readdirSync(d, { withFileTypes: true })) {
    const p = join(d, e.name)
    if (e.isDirectory()) walk(p)
    else if (e.isFile() && p.endsWith('.tsx')) files.push(p)
  }
}
walk(root)

let problems = 0
for (const f of files) {
  const rel = relative(root, f)
  readFileSync(f, 'utf8')
    .split('\n')
    .forEach((line, i) => {
      for (const r of rules) {
        if (r.test(line, rel) && !line.includes(`ui-allow: ${r.id}`)) {
          console.log(`src/${rel}:${i + 1}: [${r.id}] ${r.why}\n    ${line.trim()}`)
          problems++
        }
      }
    })
}
if (problems) {
  console.log(`\n${problems} UI rule ${problems === 1 ? 'problem' : 'problems'} (see docs/ui-guidelines.md).`)
  process.exit(1)
}
console.log(`UI rules: ${files.length} files OK.`)
