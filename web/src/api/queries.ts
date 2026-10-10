import { useMutation, useQuery, useQueryClient, type UseQueryOptions } from '@tanstack/react-query'
import { api } from './client'
import type {
  AdminUser,
  AdvancedSearch,
  AgentTask,
  APIToken,
  APNsStatus,
  AppInstallLog,
  AppLabels,
  AuditEntry,
  CommandEvent,
  CommandInspection,
  CommandSearchHit,
  CustomLabel,
  InstallomatorPreview,
  InstallomatorPreviewRequest,
  DiagnosticBundle,
  LogArchive,
  LogSource,
  FileVaultOverview,
  Building,
  ComputerDetail,
  Computer,
  ComputerGroup,
  ComputerListParams,
  ConfigProfileWithStatus,
  CriterionField,
  DashboardStats,
  Department,
  EnrollmentInvitation,
  EnrollmentStatus,
  ExtensionAttribute,
  ListResponse,
  MacAppWithStatus,
  MacApp,
  MDMCommand,
  NetworkSegment,
  Package,
  PolicyLog,
  PolicyWithStatus,
  ProfileDeployment,
  RestrictedEvent,
  RestrictedSoftware,
  SCEPStatus,
  Script,
  SearchField,
  SearchRunResult,
  SettingsView,
  ShellSession,
  ShellSessionDetail,
  WebhookDelivery,
  WebhookList,
  UpdatesOverview,
  UpdatePlan,
  UpdatesCatalog,
  ComputerDDM,
  HealthSummary,
  HealthFinding,
  HealthFindingDetail,
  HealthFindingQuery,
  HealthCheckInfo,
  HealthEvent,
  ComplianceCatalog,
  ComplianceRule,
  ComplianceBaseline,
  ComplianceBaselineRow,
  ComplianceReport,
  ComplianceComputerDetail,
  ComplianceScan,
  ComplianceProfilePreview,
  ComputerComplianceRow,
  CustomSchemaSummary,
  CustomSchemaDetail,
  SchemaPreview,
  SchemaValues,
  SuperView,
  SuperStatus,
  XCredsView,
  OnboardingWorkflow,
  OnboardingWorkflowList,
  OnboardingRun,
} from './types'

export const qk = {
  dashboard: ['dashboard'] as const,
  computers: (p?: ComputerListParams) => ['computers', p ?? {}] as const,
  computer: (id: string) => ['computer', id] as const,
  computerCommands: (id: string, p?: object) => ['computer', id, 'commands', p ?? {}] as const,
  computerTasks: (id: string, p?: object) => ['computer', id, 'tasks', p ?? {}] as const,
  computerPolicyLogs: (id: string, p?: object) => ['computer', id, 'policy-logs', p ?? {}] as const,
  groups: ['groups'] as const,
  group: (id: string) => ['group', id] as const,
  groupMembers: (id: string) => ['group', id, 'members'] as const,
  criteriaFields: ['groups', 'criteria-fields'] as const,
  scripts: ['scripts'] as const,
  script: (id: string) => ['script', id] as const,
  packages: ['packages'] as const,
  eas: ['extension-attributes'] as const,
  ea: (id: string) => ['extension-attribute', id] as const,
  policies: ['policies'] as const,
  policy: (id: string) => ['policy', id] as const,
  policyLogs: (id: string, p?: object) => ['policy', id, 'logs', p ?? {}] as const,
  allPolicyLogs: (p?: object) => ['policy-logs', p ?? {}] as const,
  profiles: ['profiles'] as const,
  profile: (id: string) => ['profile', id] as const,
  profileDeployments: (id: string) => ['profile', id, 'deployments'] as const,
  commands: (p?: object) => ['commands', p ?? {}] as const,
  command: (uuid: string) => ['command', uuid] as const,
  tasks: (p?: object) => ['tasks', p ?? {}] as const,
  enrollmentStatus: ['enrollment', 'status'] as const,
  invitations: ['enrollment', 'invitations'] as const,
  settings: ['settings'] as const,
  apns: ['settings', 'apns'] as const,
  scep: ['settings', 'scep'] as const,
  users: ['users'] as const,
  apiTokens: ['api-tokens'] as const,
  audit: (p?: object) => ['audit', p ?? {}] as const,
  shellSessions: (p?: object) => ['shell-sessions', p ?? {}] as const,
  shellSession: (id: string) => ['shell-session', id] as const,
  buildings: ['buildings'] as const,
  departments: ['departments'] as const,
  networkSegments: ['network-segments'] as const,
  restricted: ['restricted-software'] as const,
  restrictedItem: (id: string) => ['restricted-software', id] as const,
  restrictedEvents: (p?: object) => ['restricted-events', p ?? {}] as const,
  searches: ['advanced-searches'] as const,
  search: (id: string) => ['advanced-search', id] as const,
  searchRun: (id: string, p?: object) => ['advanced-search', id, 'run', p ?? {}] as const,
  searchFields: ['advanced-searches', 'fields'] as const,
  apps: ['apps'] as const,
  app: (id: string) => ['app', id] as const,
  appLabels: ['apps', 'labels'] as const,
  appLogs: (id: string, p?: object) => ['app', id, 'logs', p ?? {}] as const,
  allAppLogs: (p?: object) => ['app-logs', p ?? {}] as const,
  webhooks: ['webhooks'] as const,
  webhookDeliveries: ['webhooks', 'deliveries'] as const,
  filevault: ['filevault'] as const,
  commandInspect: (uuid: string, reveal = false) => ['command', uuid, 'inspect', reveal] as const,
  computerTimeline: (id: string, p?: object) => ['computer', id, 'timeline', p ?? {}] as const,
  commandSearch: (p?: object) => ['commands', 'search', p ?? {}] as const,
  logComponents: (source: LogSource) => ['logs', 'components', source] as const,
  logArchives: ['logs', 'archives'] as const,
  customLabels: ['custom-labels'] as const,
  customLabel: (id: string) => ['custom-label', id] as const,
  installomatorPreview: (body: object) => ['installomator-preview', body] as const,
  diagnostics: (computerId: string) => ['computer', computerId, 'diagnostics'] as const,
  updatesOverview: ['updates', 'overview'] as const,
  updatePlans: ['updates', 'plans'] as const,
  updatePlan: (id: string) => ['updates', 'plan', id] as const,
  updatesCatalog: ['updates', 'catalog'] as const,
  computerDDM: (id: string) => ['computer', id, 'ddm'] as const,
  /** Prefix for every health query: invalidate this after any health mutation. */
  health: ['health'] as const,
  healthSummary: ['health', 'summary'] as const,
  healthFindings: (p?: HealthFindingQuery) => ['health', 'findings', p ?? {}] as const,
  healthFinding: (id: string) => ['health', 'finding', id] as const,
  healthChecks: ['health', 'checks'] as const,
  healthEvents: (p?: object) => ['health', 'events', p ?? {}] as const,
  /** Prefix for every compliance query: invalidate this after any baseline mutation or scan. */
  compliance: ['compliance'] as const,
  complianceCatalog: ['compliance', 'catalog'] as const,
  complianceRules: (benchmark: string, all = false) => ['compliance', 'rules', benchmark, all] as const,
  complianceRule: (rule: string, benchmark: string) => ['compliance', 'rule', rule, benchmark] as const,
  complianceBaselines: ['compliance', 'baselines'] as const,
  complianceBaseline: (id: string) => ['compliance', 'baseline', id] as const,
  complianceReport: (id: string) => ['compliance', 'baseline', id, 'report'] as const,
  complianceComputer: (id: string, computerId: string) => ['compliance', 'baseline', id, 'computer', computerId] as const,
  complianceScans: (id: string, p?: object) => ['compliance', 'baseline', id, 'scans', p ?? {}] as const,
  complianceProfile: (id: string) => ['compliance', 'baseline', id, 'profile'] as const,
  computerCompliance: (id: string) => ['computer', id, 'compliance'] as const,
  /** Prefix for every custom-schema query. */
  customSchemas: ['custom-schemas'] as const,
  customSchema: (id: string) => ['custom-schemas', id] as const,
  schemaPreview: (id: string, body: object) => ['custom-schemas', id, 'preview', body] as const,
  /** Prefix for super config + status. */
  superAll: ['super'] as const,
  superConfig: ['super', 'config'] as const,
  superStatus: ['super', 'status'] as const,
  xcreds: ['identity', 'xcreds'] as const,
  /** Prefix for every onboarding query: invalidate this after any workflow mutation or run. */
  onboarding: ['onboarding'] as const,
  onboardingWorkflows: ['onboarding', 'workflows'] as const,
  onboardingWorkflow: (id: string) => ['onboarding', 'workflow', id] as const,
  onboardingRuns: (id: string, p?: object) => ['onboarding', 'workflow', id, 'runs', p ?? {}] as const,
  computerOnboarding: (id: string) => ['onboarding', 'computer', id] as const,
}

/** Path segment for a schema id; built-in ids contain a colon, so always encode. */
export const schemaPath = (id: string) => `/api/v1/custom-schemas/${encodeURIComponent(id)}`

type Opts<T> = Omit<UseQueryOptions<T>, 'queryKey' | 'queryFn'>

export function useDashboard() {
  return useQuery({ queryKey: qk.dashboard, queryFn: () => api.get<DashboardStats>('/api/v1/dashboard'), refetchInterval: 30_000 })
}

export function useComputers(p: ComputerListParams = {}, opts?: Opts<ListResponse<Computer>>) {
  return useQuery({
    queryKey: qk.computers(p),
    queryFn: () => api.get<ListResponse<Computer>>('/api/v1/computers', { query: { ...p, ids: p.ids } }),
    placeholderData: (prev) => prev,
    ...opts,
  })
}

export function useComputer(id: string | undefined) {
  return useQuery({ queryKey: qk.computer(id ?? ''), queryFn: () => api.get<ComputerDetail>(`/api/v1/computers/${id}`), enabled: !!id, refetchInterval: 15_000 })
}

export function useGroups(opts?: Opts<ListResponse<ComputerGroup>>) {
  return useQuery({ queryKey: qk.groups, queryFn: () => api.get<ListResponse<ComputerGroup>>('/api/v1/groups'), ...opts })
}

export function useGroup(id: string | undefined) {
  return useQuery({ queryKey: qk.group(id ?? ''), queryFn: () => api.get<ComputerGroup>(`/api/v1/groups/${id}`), enabled: !!id && id !== 'new' })
}

export function useGroupMembers(id: string | undefined) {
  return useQuery({ queryKey: qk.groupMembers(id ?? ''), queryFn: () => api.get<ListResponse<Computer>>(`/api/v1/groups/${id}/members`), enabled: !!id && id !== 'new' })
}

export function useCriteriaFields() {
  return useQuery({ queryKey: qk.criteriaFields, queryFn: () => api.get<CriterionField[]>('/api/v1/groups/criteria-fields'), staleTime: 60_000 })
}

export function useScripts(opts?: Opts<ListResponse<Script>>) {
  return useQuery({ queryKey: qk.scripts, queryFn: () => api.get<ListResponse<Script>>('/api/v1/scripts'), ...opts })
}

export function useScript(id: string | undefined) {
  return useQuery({ queryKey: qk.script(id ?? ''), queryFn: () => api.get<Script>(`/api/v1/scripts/${id}`), enabled: !!id && id !== 'new' })
}

export function usePackages(opts?: Opts<ListResponse<Package>>) {
  return useQuery({ queryKey: qk.packages, queryFn: () => api.get<ListResponse<Package>>('/api/v1/packages'), ...opts })
}

export function useExtensionAttributes() {
  return useQuery({ queryKey: qk.eas, queryFn: () => api.get<ListResponse<ExtensionAttribute>>('/api/v1/extension-attributes') })
}

export function useExtensionAttribute(id: string | undefined) {
  return useQuery({ queryKey: qk.ea(id ?? ''), queryFn: () => api.get<ExtensionAttribute>(`/api/v1/extension-attributes/${id}`), enabled: !!id && id !== 'new' })
}

export function usePolicies(opts?: Opts<ListResponse<PolicyWithStatus>>) {
  return useQuery({ queryKey: qk.policies, queryFn: () => api.get<ListResponse<PolicyWithStatus>>('/api/v1/policies'), ...opts })
}

export function usePolicy(id: string | undefined) {
  return useQuery({ queryKey: qk.policy(id ?? ''), queryFn: () => api.get<PolicyWithStatus>(`/api/v1/policies/${id}`), enabled: !!id && id !== 'new' })
}

export function usePolicyLogs(id: string, p: { status?: string; limit?: number; offset?: number }) {
  return useQuery({ queryKey: qk.policyLogs(id, p), queryFn: () => api.get<ListResponse<PolicyLog>>(`/api/v1/policies/${id}/logs`, { query: p }), placeholderData: (prev) => prev })
}

export function useAllPolicyLogs(p: { status?: string; limit?: number; offset?: number }) {
  return useQuery({ queryKey: qk.allPolicyLogs(p), queryFn: () => api.get<ListResponse<PolicyLog>>('/api/v1/policy-logs', { query: p }), placeholderData: (prev) => prev, refetchInterval: 15_000 })
}

export function useProfiles(opts?: Opts<ListResponse<ConfigProfileWithStatus>>) {
  return useQuery({ queryKey: qk.profiles, queryFn: () => api.get<ListResponse<ConfigProfileWithStatus>>('/api/v1/profiles'), ...opts })
}

export function useProfile(id: string | undefined) {
  return useQuery({ queryKey: qk.profile(id ?? ''), queryFn: () => api.get<ConfigProfileWithStatus>(`/api/v1/profiles/${id}`), enabled: !!id && id !== 'new' })
}

export function useProfileDeployments(id: string | undefined) {
  return useQuery({ queryKey: qk.profileDeployments(id ?? ''), queryFn: () => api.get<ListResponse<ProfileDeployment>>(`/api/v1/profiles/${id}/deployments`), enabled: !!id && id !== 'new', refetchInterval: 15_000 })
}

export function useCommands(p: { computer_id?: string; status?: string; limit?: number; offset?: number }) {
  return useQuery({ queryKey: qk.commands(p), queryFn: () => api.get<ListResponse<MDMCommand>>('/api/v1/commands', { query: p }), placeholderData: (prev) => prev, refetchInterval: 15_000 })
}

export function useCommand(uuid: string | undefined) {
  return useQuery({ queryKey: qk.command(uuid ?? ''), queryFn: () => api.get<MDMCommand>(`/api/v1/commands/${uuid}`), enabled: !!uuid })
}

/** Full command story. Revealed (raw) payloads are never cached: gcTime 0 drops them as soon as the inspector closes. */
export function useCommandInspection(uuid: string | undefined, reveal = false) {
  return useQuery({
    queryKey: qk.commandInspect(uuid ?? '', reveal),
    queryFn: () => api.get<CommandInspection>(`/api/v1/commands/${uuid}/inspect`, { query: { reveal: reveal ? 1 : undefined } }),
    enabled: !!uuid,
    refetchInterval: reveal ? false : 15_000,
    ...(reveal ? { gcTime: 0, staleTime: Infinity } : {}),
  })
}

export function useComputerTimeline(id: string, p: { limit?: number; offset?: number }) {
  return useQuery({ queryKey: qk.computerTimeline(id, p), queryFn: () => api.get<ListResponse<CommandEvent>>(`/api/v1/computers/${id}/timeline`, { query: p }), placeholderData: (prev) => prev, refetchInterval: 15_000 })
}

export function useCommandSearch(p: { q: string; computer_id?: string }) {
  return useQuery({
    queryKey: qk.commandSearch(p),
    queryFn: () => api.get<ListResponse<CommandSearchHit>>('/api/v1/commands/search', { query: p }),
    enabled: p.q.trim().length >= 2,
    placeholderData: (prev) => prev,
  })
}

export function useFileVault() {
  return useQuery({ queryKey: qk.filevault, queryFn: () => api.get<FileVaultOverview>('/api/v1/filevault'), refetchInterval: 30_000 })
}

export function useTasks(p: { computer_id?: string; status?: string; type?: string; limit?: number; offset?: number }) {
  return useQuery({ queryKey: qk.tasks(p), queryFn: () => api.get<ListResponse<AgentTask>>('/api/v1/tasks', { query: p }), placeholderData: (prev) => prev, refetchInterval: 15_000 })
}

export function useEnrollmentStatus() {
  return useQuery({ queryKey: qk.enrollmentStatus, queryFn: () => api.get<EnrollmentStatus>('/api/v1/enrollment/status'), staleTime: 30_000 })
}

export function useInvitations() {
  return useQuery({ queryKey: qk.invitations, queryFn: () => api.get<ListResponse<EnrollmentInvitation>>('/api/v1/enrollment/invitations') })
}

export function useSettings() {
  return useQuery({ queryKey: qk.settings, queryFn: () => api.get<SettingsView>('/api/v1/settings') })
}

export function useAPNs() {
  return useQuery({ queryKey: qk.apns, queryFn: () => api.get<APNsStatus>('/api/v1/settings/apns') })
}

export function useSCEP() {
  return useQuery({ queryKey: qk.scep, queryFn: () => api.get<SCEPStatus>('/api/v1/settings/scep') })
}

export function useUsers(enabled = true) {
  return useQuery({ queryKey: qk.users, queryFn: () => api.get<ListResponse<AdminUser>>('/api/v1/users'), enabled })
}

export function useAPITokens(enabled = true) {
  return useQuery({ queryKey: qk.apiTokens, queryFn: () => api.get<ListResponse<APIToken>>('/api/v1/api-tokens'), enabled })
}

export function useAudit(p: { limit?: number; offset?: number }, enabled = true) {
  return useQuery({ queryKey: qk.audit(p), queryFn: () => api.get<ListResponse<AuditEntry>>('/api/v1/audit', { query: p }), enabled, placeholderData: (prev) => prev })
}

export function useShellSessions(p: { computer_id?: string; limit?: number; offset?: number }, enabled = true) {
  return useQuery({ queryKey: qk.shellSessions(p), queryFn: () => api.get<ListResponse<ShellSession>>('/api/v1/shell-sessions', { query: p }), enabled, placeholderData: (prev) => prev, refetchInterval: 15_000 })
}

export function useShellSession(id: string | undefined) {
  return useQuery({ queryKey: qk.shellSession(id ?? ''), queryFn: () => api.get<ShellSessionDetail>(`/api/v1/shell-sessions/${id}`), enabled: !!id })
}

// ---- network organization ----

export function useBuildings(opts?: Opts<ListResponse<Building>>) {
  return useQuery({ queryKey: qk.buildings, queryFn: () => api.get<ListResponse<Building>>('/api/v1/buildings'), staleTime: 30_000, ...opts })
}

export function useDepartments(opts?: Opts<ListResponse<Department>>) {
  return useQuery({ queryKey: qk.departments, queryFn: () => api.get<ListResponse<Department>>('/api/v1/departments'), staleTime: 30_000, ...opts })
}

export function useNetworkSegments(opts?: Opts<ListResponse<NetworkSegment>>) {
  return useQuery({ queryKey: qk.networkSegments, queryFn: () => api.get<ListResponse<NetworkSegment>>('/api/v1/network-segments'), staleTime: 30_000, ...opts })
}

// ---- restricted software ----

export function useRestrictedSoftware(opts?: Opts<ListResponse<RestrictedSoftware>>) {
  return useQuery({ queryKey: qk.restricted, queryFn: () => api.get<ListResponse<RestrictedSoftware>>('/api/v1/restricted-software'), ...opts })
}

export function useRestrictedSoftwareItem(id: string | undefined) {
  return useQuery({ queryKey: qk.restrictedItem(id ?? ''), queryFn: () => api.get<RestrictedSoftware>(`/api/v1/restricted-software/${id}`), enabled: !!id && id !== 'new' })
}

export function useRestrictedEvents(p: { computer_id?: string; limit?: number; offset?: number }) {
  return useQuery({ queryKey: qk.restrictedEvents(p), queryFn: () => api.get<ListResponse<RestrictedEvent>>('/api/v1/restricted-software/events', { query: p }), placeholderData: (prev) => prev, refetchInterval: 15_000 })
}

// ---- advanced searches ----

export function useAdvancedSearches(opts?: Opts<ListResponse<AdvancedSearch>>) {
  return useQuery({ queryKey: qk.searches, queryFn: () => api.get<ListResponse<AdvancedSearch>>('/api/v1/advanced-searches'), ...opts })
}

export function useAdvancedSearch(id: string | undefined) {
  return useQuery({ queryKey: qk.search(id ?? ''), queryFn: () => api.get<AdvancedSearch>(`/api/v1/advanced-searches/${id}`), enabled: !!id && id !== 'new' })
}

export function useAdvancedSearchRun(id: string | undefined, p: { limit?: number } = {}, enabled = true) {
  return useQuery({ queryKey: qk.searchRun(id ?? '', p), queryFn: () => api.get<SearchRunResult>(`/api/v1/advanced-searches/${id}/run`, { query: p }), enabled: enabled && !!id && id !== 'new', placeholderData: (prev) => prev })
}

export function useSearchFields() {
  return useQuery({ queryKey: qk.searchFields, queryFn: () => api.get<SearchField[]>('/api/v1/advanced-searches/fields'), staleTime: 300_000 })
}

// ---- mac apps ----

export function useApps(opts?: Opts<ListResponse<MacAppWithStatus>>) {
  return useQuery({ queryKey: qk.apps, queryFn: () => api.get<ListResponse<MacAppWithStatus>>('/api/v1/apps'), ...opts })
}

export function useApp(id: string | undefined) {
  return useQuery({ queryKey: qk.app(id ?? ''), queryFn: () => api.get<MacApp>(`/api/v1/apps/${id}`), enabled: !!id && id !== 'new' })
}

export function useAppLabels() {
  return useQuery({ queryKey: qk.appLabels, queryFn: () => api.get<AppLabels>('/api/v1/apps/labels'), staleTime: 600_000 })
}

export function useAppLogs(id: string, p: { computer_id?: string; limit?: number; offset?: number }) {
  return useQuery({ queryKey: qk.appLogs(id, p), queryFn: () => api.get<ListResponse<AppInstallLog>>(`/api/v1/apps/${id}/logs`, { query: p }), placeholderData: (prev) => prev, refetchInterval: 15_000 })
}

export function useAllAppLogs(p: { computer_id?: string; limit?: number; offset?: number }) {
  return useQuery({ queryKey: qk.allAppLogs(p), queryFn: () => api.get<ListResponse<AppInstallLog>>('/api/v1/apps/logs', { query: p }), placeholderData: (prev) => prev, refetchInterval: 15_000 })
}

export function useCustomLabels() {
  return useQuery({ queryKey: qk.customLabels, queryFn: () => api.get<ListResponse<CustomLabel>>('/api/v1/custom-labels') })
}

export function useCustomLabel(id: string | undefined) {
  return useQuery({ queryKey: qk.customLabel(id ?? ''), queryFn: () => api.get<CustomLabel>(`/api/v1/custom-labels/${id}`), enabled: !!id && id !== 'new' })
}

/** Exact Installomator command for a label (or custom definition) and options. Pass a debounced body; null disables. */
export function useInstallomatorPreview(body: InstallomatorPreviewRequest | null) {
  return useQuery({
    queryKey: qk.installomatorPreview(body ?? {}),
    queryFn: () => api.post<InstallomatorPreview>('/api/v1/apps/installomator-preview', body),
    enabled: !!body,
    retry: false,
    staleTime: 60_000,
  })
}

// ---- webhooks ----

export function useWebhooks(enabled = true) {
  return useQuery({ queryKey: qk.webhooks, queryFn: () => api.get<WebhookList>('/api/v1/webhooks'), enabled })
}

export function useWebhookDeliveries(enabled = true) {
  return useQuery({ queryKey: qk.webhookDeliveries, queryFn: () => api.get<ListResponse<WebhookDelivery>>('/api/v1/webhooks/deliveries'), enabled, refetchInterval: 15_000 })
}

// ---- logs & diagnostics ----

export function useLogComponents(source: LogSource) {
  return useQuery({ queryKey: qk.logComponents(source), queryFn: () => api.get<string[]>('/api/v1/logs/components', { query: { source } }), staleTime: 60_000 })
}

export function useLogArchives(enabled = true) {
  return useQuery({ queryKey: qk.logArchives, queryFn: () => api.get<LogArchive[]>('/api/v1/logs/archives'), enabled })
}

/** Diagnostic bundles for a computer; polls every 3s while any bundle is still being collected. */
export function useDiagnostics(computerId: string) {
  return useQuery({
    queryKey: qk.diagnostics(computerId),
    queryFn: () => api.get<ListResponse<DiagnosticBundle>>(`/api/v1/computers/${computerId}/diagnostics`),
    refetchInterval: (query) => ((query.state.data?.items ?? []).some((b) => b.status === 'requested') ? 3_000 : false),
  })
}

/** Convenience: a mutation that invalidates the given keys on success. */
export function useInvalidate() {
  const qc = useQueryClient()
  return (...keys: readonly (readonly unknown[])[]) => Promise.all(keys.map((k) => qc.invalidateQueries({ queryKey: k as unknown[] })))
}

export function useApiMutation<TVars, TData = unknown>(fn: (vars: TVars) => Promise<TData>, invalidate: readonly (readonly unknown[])[] = [], onSuccess?: (data: TData, vars: TVars) => void) {
  const qc = useQueryClient()
  return useMutation<TData, Error, TVars>({
    mutationFn: fn,
    onSuccess: async (data, vars) => {
      await Promise.all(invalidate.map((k) => qc.invalidateQueries({ queryKey: k as unknown[] })))
      onSuccess?.(data, vars)
    },
  })
}

// ---- Software updates (DDM) ----

export function useUpdatesOverview() {
  return useQuery({ queryKey: qk.updatesOverview, queryFn: () => api.get<UpdatesOverview>('/api/v1/updates/overview'), refetchInterval: 30_000 })
}

export function useUpdatePlans() {
  return useQuery({ queryKey: qk.updatePlans, queryFn: () => api.get<ListResponse<UpdatePlan>>('/api/v1/updates/plans') })
}

export function useUpdatePlan(id: string | undefined) {
  return useQuery({ queryKey: qk.updatePlan(id ?? ''), queryFn: () => api.get<UpdatePlan>(`/api/v1/updates/plans/${id}`), enabled: !!id })
}

/** SOFA-backed catalog; the server answers 502 when the feed is unreachable, so don't retry hard. */
export function useUpdatesCatalog() {
  return useQuery({ queryKey: qk.updatesCatalog, queryFn: () => api.get<UpdatesCatalog>('/api/v1/updates/catalog'), staleTime: 30 * 60_000, retry: false })
}

export function useComputerDDM(id: string) {
  return useQuery({ queryKey: qk.computerDDM(id), queryFn: () => api.get<ComputerDDM>(`/api/v1/computers/${id}/ddm`), enabled: !!id, refetchInterval: 15_000 })
}

// ---- Health ----

/** Headline counts and grouped issues. Also drives the sidebar badge, so it refreshes every minute. */
export function useHealthSummary() {
  return useQuery({ queryKey: qk.healthSummary, queryFn: () => api.get<HealthSummary>('/api/v1/health/summary'), refetchInterval: 60_000 })
}

export function useHealthFindings(p: HealthFindingQuery, enabled = true) {
  return useQuery({
    queryKey: qk.healthFindings(p),
    queryFn: () => api.get<ListResponse<HealthFinding>>('/api/v1/health/findings', { query: { ...p } }),
    enabled,
    placeholderData: (prev) => prev,
    refetchInterval: 60_000,
  })
}

export function useHealthFinding(id: string | undefined) {
  return useQuery({ queryKey: qk.healthFinding(id ?? ''), queryFn: () => api.get<HealthFindingDetail>(`/api/v1/health/findings/${id}`), enabled: !!id })
}

export function useHealthChecks() {
  return useQuery({ queryKey: qk.healthChecks, queryFn: () => api.get<HealthCheckInfo[]>('/api/v1/health/checks'), staleTime: 30_000 })
}

export function useHealthEvents(p: { limit?: number } = {}) {
  return useQuery({ queryKey: qk.healthEvents(p), queryFn: () => api.get<HealthEvent[]>('/api/v1/health/events', { query: p }), refetchInterval: 60_000 })
}

// ---- Compliance (mSCP) ----

/** The embedded catalog never changes while the server runs. */
export function useComplianceCatalog() {
  return useQuery({ queryKey: qk.complianceCatalog, queryFn: () => api.get<ComplianceCatalog>('/api/v1/compliance/catalog'), staleTime: Infinity })
}

export function useComplianceRules(benchmark: string | undefined, all = false) {
  return useQuery({
    queryKey: qk.complianceRules(benchmark ?? '', all),
    queryFn: () => api.get<ComplianceRule[]>('/api/v1/compliance/catalog/rules', { query: { benchmark, all: all ? 1 : undefined } }),
    enabled: !!benchmark,
    staleTime: Infinity,
  })
}

/** One rule with its check/fix scripts; only fetched when someone opens them. */
export function useComplianceRule(rule: string | undefined, benchmark = '', enabled = true) {
  return useQuery({
    queryKey: qk.complianceRule(rule ?? '', benchmark),
    queryFn: () => api.get<ComplianceRule>(`/api/v1/compliance/catalog/rules/${encodeURIComponent(rule ?? '')}`, { query: { benchmark } }),
    enabled: enabled && !!rule,
    staleTime: Infinity,
  })
}

export function useComplianceBaselines() {
  return useQuery({ queryKey: qk.complianceBaselines, queryFn: () => api.get<ComplianceBaselineRow[]>('/api/v1/compliance/baselines'), refetchInterval: 60_000 })
}

export function useComplianceBaseline(id: string | undefined) {
  return useQuery({ queryKey: qk.complianceBaseline(id ?? ''), queryFn: () => api.get<ComplianceBaseline>(`/api/v1/compliance/baselines/${id}`), enabled: !!id })
}

export function useComplianceReport(id: string | undefined) {
  return useQuery({ queryKey: qk.complianceReport(id ?? ''), queryFn: () => api.get<ComplianceReport>(`/api/v1/compliance/baselines/${id}/report`), enabled: !!id, refetchInterval: 30_000 })
}

export function useComplianceComputer(id: string | undefined, computerId: string | undefined) {
  return useQuery({
    queryKey: qk.complianceComputer(id ?? '', computerId ?? ''),
    queryFn: () => api.get<ComplianceComputerDetail>(`/api/v1/compliance/baselines/${id}/computers/${computerId}`),
    enabled: !!id && !!computerId,
    refetchInterval: (query) => ((query.state.data?.scans ?? []).some((s) => s.status === 'pending') ? 5_000 : 30_000),
  })
}

/** Polls quickly while any scan is still pending. */
export function useComplianceScans(id: string | undefined, p: { computer_id?: string; limit?: number } = {}) {
  return useQuery({
    queryKey: qk.complianceScans(id ?? '', p),
    queryFn: () => api.get<ComplianceScan[]>(`/api/v1/compliance/baselines/${id}/scans`, { query: p }),
    enabled: !!id,
    refetchInterval: (query) => ((query.state.data ?? []).some((s) => s.status === 'pending') ? 5_000 : 30_000),
  })
}

/** 422 means "no rule in this baseline has profile settings"; don't retry it. */
export function useComplianceProfile(id: string | undefined, enabled = true) {
  return useQuery({
    queryKey: qk.complianceProfile(id ?? ''),
    queryFn: () => api.get<ComplianceProfilePreview>(`/api/v1/compliance/baselines/${id}/profile`),
    enabled: enabled && !!id,
    retry: false,
  })
}

export function useComputerCompliance(id: string) {
  return useQuery({ queryKey: qk.computerCompliance(id), queryFn: () => api.get<ComputerComplianceRow[]>(`/api/v1/computers/${id}/compliance`), enabled: !!id, refetchInterval: 30_000 })
}

// ---- Application & Custom Settings (JSON schema profiles) ----

export function useCustomSchemas() {
  return useQuery({ queryKey: qk.customSchemas, queryFn: () => api.get<CustomSchemaSummary[]>('/api/v1/custom-schemas') })
}

/** Built-in schemas never change while the server runs; custom ones only change through this console. */
export function useCustomSchema(id: string | undefined | null) {
  return useQuery({ queryKey: qk.customSchema(id ?? ''), queryFn: () => api.get<CustomSchemaDetail>(schemaPath(id ?? '')), enabled: !!id, staleTime: 5 * 60_000 })
}

/** Live profile preview. Pass a debounced body; null disables. */
export function useSchemaPreview(id: string | undefined | null, body: { name: string; values: SchemaValues } | null) {
  return useQuery({
    queryKey: qk.schemaPreview(id ?? '', body ?? {}),
    queryFn: () => api.post<SchemaPreview>(`${schemaPath(id ?? '')}/preview`, body),
    enabled: !!id && !!body,
    retry: false,
    staleTime: 60_000,
    placeholderData: (prev) => prev,
  })
}

// ---- Software updates → super ----

export function useSuper() {
  return useQuery({ queryKey: qk.superConfig, queryFn: () => api.get<SuperView>('/api/v1/updates/super') })
}

export function useSuperStatus() {
  return useQuery({ queryKey: qk.superStatus, queryFn: () => api.get<SuperStatus>('/api/v1/updates/super/status'), refetchInterval: 30_000 })
}

// ---- Identity → XCreds ----

export function useXCreds() {
  return useQuery({ queryKey: qk.xcreds, queryFn: () => api.get<XCredsView>('/api/v1/identity/xcreds'), refetchInterval: 30_000 })
}

// ---- White-glove onboarding ----

export function useOnboardingWorkflows() {
  return useQuery({ queryKey: qk.onboardingWorkflows, queryFn: () => api.get<OnboardingWorkflowList>('/api/v1/onboarding/workflows') })
}

export function useOnboardingWorkflow(id: string | undefined) {
  return useQuery({ queryKey: qk.onboardingWorkflow(id ?? ''), queryFn: () => api.get<OnboardingWorkflow>(`/api/v1/onboarding/workflows/${id}`), enabled: !!id && id !== 'new' })
}

const runsActive = (runs: OnboardingRun[] | undefined) => (runs ?? []).some((r) => r.status === 'queued' || r.status === 'running')

/** Polls quickly while any run is still queued or running. */
export function useOnboardingRuns(id: string | undefined, p: { computer_id?: string; limit?: number } = {}) {
  return useQuery({
    queryKey: qk.onboardingRuns(id ?? '', p),
    queryFn: () => api.get<OnboardingRun[]>(`/api/v1/onboarding/workflows/${id}/runs`, { query: p }),
    enabled: !!id && id !== 'new',
    refetchInterval: (query) => (runsActive(query.state.data) ? 5_000 : 30_000),
  })
}

export function useComputerOnboarding(id: string) {
  return useQuery({
    queryKey: qk.computerOnboarding(id),
    queryFn: () => api.get<OnboardingRun[]>(`/api/v1/computers/${id}/onboarding`),
    enabled: !!id,
    refetchInterval: (query) => (runsActive(query.state.data) ? 5_000 : 30_000),
  })
}
