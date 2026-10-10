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
}

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

// ---- webhooks ----

export function useWebhooks(enabled = true) {
  return useQuery({ queryKey: qk.webhooks, queryFn: () => api.get<WebhookList>('/api/v1/webhooks'), enabled })
}

export function useWebhookDeliveries(enabled = true) {
  return useQuery({ queryKey: qk.webhookDeliveries, queryFn: () => api.get<ListResponse<WebhookDelivery>>('/api/v1/webhooks/deliveries'), enabled, refetchInterval: 15_000 })
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
