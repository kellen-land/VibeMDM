import { useEffect, useState , useCallback } from 'react'
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import {
  Activity,
  AppWindow,
  Ban,
  ChevronDown,
  ChevronUp,
  FileCode,
  Fingerprint,
  HandHelping,
  HeartPulse,
  KeyRound,
  LockKeyhole,
  UserLock,
  Laptop,
  LayoutDashboard,
  ListChecks,
  LogOut,
  Menu,
  Monitor,
  Moon,
  Package,
  Puzzle,
  RefreshCcwDot,
  ScrollText,
  QrCode,
  Search,
  Settings,
  Smartphone,
  ShieldCheck,
  ShieldHalf,
  Sun,
  Terminal,
  Users,
  X,
  type LucideIcon,
  Keyboard,
  PanelLeftClose,
  PanelLeftOpen,
} from 'lucide-react'
import { useAuth } from '../../auth/AuthContext'
import { useTheme } from '../../theme/ThemeContext'
import { cn } from '../../lib/cn'
import { Badge } from '../ui/Badge'
import { Dropdown } from '../ui/Dropdown'
import { ShortcutHandler, ShortcutHelp } from '../editor/Shortcuts'
import { pageKey } from '../../lib/pageKey'
import { hasModifier, modLabel } from '../../lib/shortcuts'
import { EditGuardProvider } from '../editor/EditGuard'
import { SaveFeedbackProvider } from '../editor/SaveFeedback'
import { ChangePasswordDialog } from './ChangePasswordDialog'
import { useToast } from '../ui/Toast'
import { errorMessage } from '../../api/client'
import { useHealthSummary } from '../../api/queries'

interface NavItem {
  to: string
  label: string
  icon: LucideIcon
  end?: boolean
  badge?: 'health'
}
/** A group without a heading is always shown; others fold away to keep the sidebar calm. */
const navGroups: { heading: string; items: NavItem[] }[] = [
  {
    heading: '',
    items: [
      { to: '/', label: 'Dashboard', icon: LayoutDashboard, end: true },
      { to: '/health', label: 'Health', icon: HeartPulse, badge: 'health' },
    ],
  },
  {
    heading: 'Computers',
    items: [
      { to: '/computers', label: 'Computers', icon: Laptop },
      { to: '/groups', label: 'Computer groups', icon: Users },
      { to: '/searches', label: 'Searches', icon: Search },
    ],
  },
  {
    heading: 'Mobile Devices',
    items: [
      { to: '/mobile-devices', label: 'Mobile devices', icon: Smartphone },
      { to: '/mobile-groups', label: 'Mobile device groups', icon: Users },
      { to: '/mobile-searches', label: 'Searches', icon: Search },
      { to: '/mobile-profiles', label: 'Configuration profiles', icon: ShieldCheck },
      { to: '/mobile-enrollment', label: 'Enrollment', icon: QrCode },
    ],
  },
  {
    heading: 'Management',
    items: [
      { to: '/policies', label: 'Policies', icon: ListChecks },
      { to: '/scripts', label: 'Scripts', icon: Terminal },
      { to: '/profiles', label: 'Configuration profiles', icon: ShieldCheck },
      { to: '/updates', label: 'Software updates', icon: RefreshCcwDot },
      { to: '/packages', label: 'Packages', icon: Package },
      { to: '/apps', label: 'Mac Apps', icon: AppWindow },
      { to: '/extension-attributes', label: 'Extension attributes', icon: Puzzle },
    ],
  },
  {
    heading: 'Security',
    items: [
      { to: '/filevault', label: 'FileVault', icon: LockKeyhole },
      { to: '/laps', label: 'Local admin passwords', icon: UserLock },
      { to: '/compliance', label: 'Compliance', icon: ShieldHalf },
      { to: '/identity', label: 'Identity', icon: Fingerprint },
      { to: '/restricted-software', label: 'Restricted software', icon: Ban },
    ],
  },
  {
    heading: 'Operations',
    items: [
      { to: '/activity', label: 'Activity', icon: Activity },
      { to: '/logs', label: 'Logs', icon: ScrollText },
      { to: '/enrollment', label: 'Enrollment', icon: QrCode },
      { to: '/onboarding', label: 'Onboarding', icon: HandHelping },
    ],
  },
  { heading: '', items: [{ to: '/settings', label: 'Settings', icon: Settings }] },
]

/** Open critical + warning findings; hidden when zero so a healthy system stays quiet. */
function HealthNavBadge() {
  const q = useHealthSummary()
  const c = q.data?.counts
  const n = c ? c.critical + c.warning : 0
  if (!c || n === 0) return null
  const label = [c.critical && `${c.critical} critical`, c.warning && `${c.warning} ${c.warning === 1 ? 'warning' : 'warnings'}`].filter(Boolean).join(', ')
  return (
    <span
      className={cn(
        'ml-auto rounded px-1.5 text-[11px] font-semibold leading-4 tabular-nums',
        c.critical ? 'bg-critical/15 text-[#b22424] dark:text-[#f28b8b]' : 'bg-warning/15 text-[#8a5a0a] dark:text-[#f0c070]',
      )}
      title={label}
      aria-label={`${label} open`}
    >
      {n}
    </span>
  )
}

/** The rail shows open findings as a dot on the Health icon. */
function HealthNavDot() {
  const q = useHealthSummary()
  const c = q.data?.counts
  if (!c || c.critical + c.warning === 0) return null
  return <span className={cn('absolute left-[26px] top-1.5 size-2 rounded-full ring-2 ring-surface', c.critical ? 'bg-critical' : 'bg-warning')} aria-label={`${c.critical + c.warning} open findings`} />
}

/** Collapse the sidebar to an icon rail, or expand it again (⌃[ / Alt+[). */
function RailToggle({ rail, onToggle }: { rail: boolean; onToggle: () => void }) {
  const label = `${rail ? 'Expand' : 'Collapse'} sidebar (${modLabel}[)`
  return (
    <button type="button" onClick={onToggle} aria-label={label} title={label} className="inline-flex size-8 items-center justify-center rounded-md border border-line text-ink-muted hover:bg-line/40 hover:text-ink focus-ring">
      {rail ? <PanelLeftOpen className="size-4" /> : <PanelLeftClose className="size-4" />}
    </button>
  )
}

const prefLabel = { system: 'System', light: 'Light', dark: 'Dark' } as const
const nextPref = { system: 'light', light: 'dark', dark: 'system' } as const

/** Cycles System → Light → Dark. The icon shows the current choice; System follows the OS live. */
function ThemeToggle({ className }: { className?: string }) {
  const { theme, preference, cycle } = useTheme()
  const label = `Theme: ${prefLabel[preference]}${preference === 'system' ? ` (${theme})` : ''}. Switch to ${prefLabel[nextPref[preference]]}`
  return (
    <button
      type="button"
      onClick={cycle}
      aria-label={label}
      title={label}
      className={cn('inline-flex size-8 items-center justify-center rounded-md border border-line text-ink-muted hover:text-ink hover:bg-line/40 focus-ring', className)}
    >
      {preference === 'system' ? <Monitor className="size-4" /> : preference === 'light' ? <Sun className="size-4" /> : <Moon className="size-4" />}
    </button>
  )
}

export function AppShell() {
  const { user, status, logout } = useAuth()
  const [mobileOpen, setMobileOpen] = useState(false)
  const [pwOpen, setPwOpen] = useState(false)
  const [keysOpen, setKeysOpen] = useState(false)
  const loc = useLocation()
  const navigate = useNavigate()
  const toast = useToast()
  useEffect(() => setMobileOpen(false), [loc.pathname])

  // Desktop sidebar: full width or a slim icon rail. Remembered per browser.
  const [rail, setRail] = useState(() => {
    try {
      return localStorage.getItem('vibemdm.nav.rail') === '1'
    } catch {
      return false
    }
  })
  const toggleRail = useCallback(() => {
    setRail((r) => {
      try {
        localStorage.setItem('vibemdm.nav.rail', r ? '0' : '1')
      } catch {
        /* storage unavailable: applies to this visit */
      }
      return !r
    })
  }, [])
  // ⌃[ on a Mac, Alt+[ elsewhere (the shortcut modifier), like the other console shortcuts.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.code === 'BracketLeft' && hasModifier(e) && !e.shiftKey) {
        e.preventDefault()
        toggleRail()
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [toggleRail])

  // Headed groups fold away; the one holding the current page opens itself. Choices persist per browser.
  const [openGroups, setOpenGroups] = useState<Record<string, boolean>>(() => {
    try {
      return JSON.parse(localStorage.getItem('vibemdm.nav') ?? '{}')
    } catch {
      return {}
    }
  })
  useEffect(() => {
    const active = navGroups.find((g) => g.heading && g.items.some((n) => (n.end ? loc.pathname === n.to : loc.pathname === n.to || loc.pathname.startsWith(n.to + '/'))))
    if (active && !openGroups[active.heading]) setOpenGroups((o) => ({ ...o, [active.heading]: true }))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [loc.pathname])
  useEffect(() => {
    try {
      localStorage.setItem('vibemdm.nav', JSON.stringify(openGroups))
    } catch {
      /* storage unavailable: folding still works for this visit */
    }
  }, [openGroups])

  const doLogout = async () => {
    try {
      await logout()
      navigate('/login', { replace: true })
    } catch (e) {
      toast.error('Sign out failed', errorMessage(e))
    }
  }

  const sidebar = (compact: boolean) => (
    <div className="flex h-full flex-col overflow-hidden bg-surface">
      <div className="flex items-center gap-2.5 px-4 h-14 border-b border-line">
        <div className="flex size-8 items-center justify-center rounded-md bg-accent-500/15 text-accent-400 ring-1 ring-accent-500/30">
          <FileCode className="size-[18px]" aria-hidden="true" />
        </div>
        <div className={cn('min-w-0 whitespace-nowrap transition-opacity duration-200 motion-reduce:transition-none', compact && 'pointer-events-none opacity-0')}>
          <p className="truncate text-[13px] font-semibold text-ink leading-4 tracking-tight">{status?.org_name || 'VibeMDM'}</p>
          <p className="truncate text-[10px] uppercase tracking-[0.12em] text-ink-faint leading-4">Device Management</p>
        </div>
        <button type="button" className="ml-auto lg:hidden text-ink-muted" onClick={() => setMobileOpen(false)} aria-label="Close menu">
          <X className="size-5" />
        </button>
      </div>
      <nav className={cn('flex-1 overflow-y-auto overflow-x-hidden px-2.5 py-3', compact ? 'space-y-2' : 'space-y-4')}>
        {navGroups.map((g, gi) => {
          // In the rail every group is shown (icons only); folding is for the full sidebar.
          const open = compact || !g.heading || !!openGroups[g.heading]
          return (
            <div key={g.heading || `top-${gi}`}>
              {g.heading &&
                (compact ? (
                  <div className="mx-2 mb-2 h-px bg-line" aria-hidden="true" />
                ) : (
                  <button
                    type="button"
                    onClick={() => setOpenGroups((o) => ({ ...o, [g.heading]: !o[g.heading] }))}
                    aria-expanded={open}
                    className="flex w-full items-center justify-between whitespace-nowrap rounded px-2 pb-1 micro-label text-ink-faint hover:text-ink-muted focus-ring"
                  >
                    {g.heading}
                    <ChevronDown className={cn('size-3.5 transition-transform duration-300 ease-out motion-reduce:transition-none', !open && '-rotate-90')} aria-hidden="true" />
                  </button>
                ))}
              {/* Folds with a height animation (grid rows 0fr ↔ 1fr) and a fade. */}
              <div
                className={cn(
                  'grid transition-[grid-template-rows,opacity] duration-300 ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none',
                  open ? 'grid-rows-[1fr] opacity-100' : 'grid-rows-[0fr] opacity-0',
                )}
                inert={!open}
              >
                <div className="min-h-0 space-y-0.5 overflow-hidden">
                  {g.items.map((n) => (
                    <NavLink
                      key={n.to}
                      to={n.to}
                      end={n.end}
                      title={compact ? n.label : undefined}
                      aria-label={compact ? n.label : undefined}
                      className={({ isActive }) =>
                        cn(
                          'group relative flex items-center gap-2.5 rounded-md pl-3 pr-2.5 py-[7px] text-[13px] font-medium transition-colors',
                          isActive ? 'bg-accent-500/12 text-ink' : 'text-ink-muted hover:bg-line/40 hover:text-ink',
                        )
                      }
                    >
                      {({ isActive }) => (
                        <>
                          <span className={cn('absolute left-0 top-1/2 -translate-y-1/2 h-4 w-0.5 rounded-full bg-accent-400 transition-opacity', isActive ? 'opacity-100' : 'opacity-0')} aria-hidden="true" />
                          <n.icon className={cn('size-4 shrink-0', isActive ? 'text-accent-400' : 'text-ink-faint group-hover:text-ink-muted')} aria-hidden="true" />
                          <span className={cn('truncate whitespace-nowrap transition-[opacity,transform] duration-200 motion-reduce:transition-none', compact && '-translate-x-1 opacity-0')}>{n.label}</span>
                          {n.badge === 'health' && (compact ? <HealthNavDot /> : <HealthNavBadge />)}
                        </>
                      )}
                    </NavLink>
                  ))}
                </div>
              </div>
            </div>
          )
        })}
      </nav>
      <div className="border-t border-line p-2.5">
        {compact ? (
          <div className="flex flex-col items-center gap-2">
            <button type="button" onClick={() => setPwOpen(true)} className="flex size-8 items-center justify-center rounded-md bg-line text-[11px] font-semibold uppercase text-ink hover:ring-1 hover:ring-line-strong focus-ring" title={`${user?.display_name || user?.username} · change password`}>
              {(user?.display_name || user?.username || '?').slice(0, 2)}
            </button>
            <ThemeToggle />
            <RailToggle rail onToggle={toggleRail} />
          </div>
        ) : (
        <>
        <Dropdown
          align="up"
          className="w-full"
          menuClassName="w-full"
          trigger={
            <button type="button" className="flex w-full items-center gap-2.5 rounded-md px-2 py-1.5 text-left hover:bg-line/40 focus-ring">
              <div className="flex size-7 items-center justify-center rounded-md bg-line text-[11px] font-semibold uppercase text-ink">
                {(user?.display_name || user?.username || '?').slice(0, 2)}
              </div>
              <div className="min-w-0 flex-1">
                <p className="truncate text-[13px] font-medium text-ink leading-4">{user?.display_name || user?.username}</p>
                <div className="flex items-center gap-1.5 mt-0.5">
                  <p className="truncate text-[11px] text-ink-faint">{user?.username}</p>
                  <Badge tone={user?.role === 'admin' ? 'accent' : 'neutral'} className="px-1 py-0 text-[10px]">
                    {user?.role}
                  </Badge>
                </div>
              </div>
              <ChevronUp className="size-4 text-ink-faint" />
            </button>
          }
          items={[
            { label: 'Change password', icon: <KeyRound />, onSelect: () => setPwOpen(true) },
            { label: 'Keyboard shortcuts', icon: <Keyboard />, onSelect: () => setKeysOpen(true) },
            { separator: true, label: '' },
            { label: 'Sign out', icon: <LogOut />, onSelect: doLogout },
          ]}
        />
        <div className="mt-2 flex items-center justify-between gap-1.5 px-1">
          <span className="mr-auto whitespace-nowrap text-[10px] text-ink-faint tabular-nums">{status?.version ? `v${status.version}` : ''}</span>
          <ThemeToggle />
          {!mobileOpen && <RailToggle rail={false} onToggle={toggleRail} />}
        </div>
        </>
        )}
      </div>
    </div>
  )

  return (
    <div className="flex min-h-full">
      <aside
        className={cn(
          'hidden lg:flex fixed inset-y-0 left-0 flex-col border-r border-line z-30 transition-[width] duration-300 ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none',
          rail ? 'w-[60px]' : 'w-60',
        )}
      >
        {sidebar(rail)}
      </aside>
      {/* Phone drawer: slides in over a fading backdrop. */}
      <div className={cn('fixed inset-0 z-40 lg:hidden', !mobileOpen && 'pointer-events-none')} aria-hidden={!mobileOpen}>
        <div className={cn('absolute inset-0 bg-black/60 backdrop-blur-sm transition-opacity duration-300 motion-reduce:transition-none', mobileOpen ? 'opacity-100' : 'opacity-0')} onClick={() => setMobileOpen(false)} />
        <aside
          className={cn(
            'absolute inset-y-0 left-0 w-64 border-r border-line shadow-2xl transition-transform duration-300 ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none',
            mobileOpen ? 'translate-x-0' : '-translate-x-full',
          )}
          inert={!mobileOpen}
        >
          {sidebar(false)}
        </aside>
      </div>
      <div className={cn('flex min-w-0 flex-1 flex-col transition-[padding] duration-300 ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none', rail ? 'lg:pl-[60px]' : 'lg:pl-60')}>
        <header className="sticky top-0 z-20 flex h-13 items-center gap-3 border-b border-line bg-surface/90 backdrop-blur px-4 py-2.5 lg:hidden">
          <button type="button" onClick={() => setMobileOpen(true)} className="text-ink-muted" aria-label="Open menu">
            <Menu className="size-5" />
          </button>
          <span className="text-[13px] font-semibold">{status?.org_name || 'VibeMDM'}</span>
          <ThemeToggle className="ml-auto" />
        </header>
        <main className="flex-1 px-4 py-6 sm:px-6 lg:px-8 lg:py-7 max-w-[1440px] w-full mx-auto">
          {/* Keyed by page: every object page (including /x/new after /x/1, or a Clone) starts fresh. */}
          <EditGuardProvider>
            <SaveFeedbackProvider>
              <Outlet key={pageKey(loc.pathname)} />
            </SaveFeedbackProvider>
          </EditGuardProvider>
        </main>
      </div>
      <ChangePasswordDialog open={pwOpen} onClose={() => setPwOpen(false)} />
      <ShortcutHandler />
      <ShortcutHelp open={keysOpen} onClose={() => setKeysOpen(false)} />
    </div>
  )
}
