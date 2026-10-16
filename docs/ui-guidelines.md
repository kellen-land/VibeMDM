# Console UI guidelines

These rules keep every screen of the VibeMDM console the same: the same page shapes, the same way of saving, the
same words. The workflow follows Jamf Pro (view → Edit → Save, Jamf's keyboard shortcuts); the page structure and
ergonomics follow Shopify Polaris (a small set of page templates and building blocks). The owner is sensitive to
clutter: keep screens calm, one primary action per screen, and fold rare options away.

`npm run check:ui` (in `web/`) flags the most common deviations. Run it with the other gates.

## Page templates

Every page is one of these. Build new pages from the shared components; don't hand-roll headers, toolbars or tables.

| Template | Component | Used for |
| --- | --- | --- |
| **Index** | `components/templates/IndexPage` | Every list of objects: policies, profiles, scripts, groups, searches, apps, … |
| **Details** | `PageHeader` + `Tabs` + `DetailsLayout`/`SummaryCard` + `ObjectBar` | One object: its tabs, its settings and a summary column |
| **Settings** | Settings section nav + `ObjectBar noDone` | Singleton configuration pages |
| **Homepage** | `MetricsCard`, `CalloutCard` | The dashboard |

### Index

- Title, the page description, and **one** primary action: **New *noun*** (sentence case). Other page actions are
  secondary buttons before it.
- Search ("Search *plural*…"), a few filters only where they help (state, category, type), sortable columns.
- A row opens the object's Details page. There are no per-row Delete buttons: select rows for bulk actions (Delete,
  Enable/Disable where it applies), or delete from the Details page.
- Empty state: "No *plural* yet" and one sentence on what they are for, with the New action.

### Details

- Breadcrumb, title, then tabs. The first (General) tab puts the settings in the main column and a **Summary** card in
  the side column: status, scope, key counts, last changed.
- The object opens **read-only**. The bar at the bottom right (`ObjectBar`) holds History, Clone, Delete, any
  object-specific actions (Run now, Export, …), Done and **Edit**.
- **Edit** unlocks every tab; the bar shows the unsaved-changes dot, **Cancel** and **Save**. One Save for the whole
  object, never a Save per tab or card.
- Delete asks for a second click in the bar. Clone opens an unsaved "Copy of …".
- Device records are the exception, as in Jamf: each section (User and Location, Extension Attributes) has its own
  Edit / Cancel / Save.

### Settings

- Same as Details without Done, Delete or Clone: Edit, then one Save for the page.

### Dialogs

- Small forms (users, webhooks, tokens, network segments) stay in dialogs with **Cancel** and one primary button at
  the bottom (`data-shortcut="cancel"` / `"save"`). Destructive confirmations use `ConfirmDialog`.

## Feedback

- **Saved:** the bar turns green with a drawn check ("Saved"), a line sweeps across it and the page's cards pulse once
  as the object locks read-only; a success notification slides in at the top right. Saves outside the bar (device
  record sections) call `useCelebrate()`. All of it is skipped with *Reduce motion*.
- **Failed:** a notification with the reason, and the form stays in edit mode. Validation problems are shown on the
  field (and Save jumps to the tab that has them).
- **Problems with the page itself** (not found, no permission, missing setup): an `Alert` banner on the page.
- Never `window.confirm` or `alert`.

## Keyboard

Jamf Pro's shortcuts, with Control on a Mac and Alt elsewhere (see the manual, §1.7). Buttons opt in with
`data-shortcut="save|edit|cancel|delete|clone|new|history|logs|view|back"`; tabs with `shortcut: 'logs' | 'view'`.

## Words

- Sentence case everywhere: "New policy", "Save", "Run now".
- Verbs mean one thing: **Create** (new object), **Save** (changes), **Delete** (the object is gone), **Remove** (take
  it off devices or out of a list; the object stays), **Deploy** / **Redeploy** (send to devices).
- Descriptions say what something does for the admin, in one sentence, without jargon where a plain word works.
- Name objects the same way in lists, breadcrumbs, dialogs and notifications ("policy", "configuration profile",
  "smart group").

## Building blocks

`MetricsCard` (one figure, optional link), `CalloutCard` (one next step; at most one on a page), `SummaryCard`
(label/value rows), `EmptyState`, `Table`, `Badge`, `Alert`. Prefer these over one-off cards.

## Accessibility

The console targets WCAG 2.2 AA. Build on the shared components, which carry most of it:

- **Focus:** every focusable element shows the accent outline (`focus-ring`, or the base style). Never remove an
  outline without replacing it. Pages start with "Skip to content", and the browser title follows the page's `h1`
  (`PageHeader`).
- **Colour:** body text uses `ink` or `ink-muted`; `ink-faint` is for secondary text and still meets 4.5:1. Status
  text uses `good`/`warning`/`serious`/`critical`, which are theme-aware; white text on a status fill uses
  `good-strong`/`critical-strong`. Form control outlines use `line-control` (3:1). Never let colour be the only signal.
- **Keyboard:** anything clickable is a `<button>` or a link, never a `div`, `span` or `tr` with `onClick`. Composite
  widgets (tabs, menus, comboboxes) follow the ARIA Authoring Practices pattern for their role.
- **Names:** every control has a visible label or an `aria-label` that says which object it acts on ("Enable
  Weekly cache cleanup", not "Toggle").

`.github/workflows/build.yml` runs an accessibility scan on every PR (`tests/a11y`): axe-core on every console page in
light and dark theme, plus a check that each page has its own title. A problem that is not in
`tests/a11y/baseline.json` fails the build. When you fix a known problem, re-run the scan with `--update-baseline`
and commit the smaller baseline; never add entries by hand.
