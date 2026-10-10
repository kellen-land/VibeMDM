// Types mirroring the Go JSON contract (internal/model/model.go, internal/store/*.go, internal/api/*.go).

export interface ListResponse<T> {
  items: T[]
  total: number
}

export interface ApiErrorBody {
  error: string
}

export interface OkResponse {
  ok: boolean
}

export type Role = 'admin' | 'viewer'

export interface AdminUser {
  id: string
  username: string
  display_name: string
  email: string
  role: Role
  enabled: boolean
  created_at: string
  last_login_at: string | null
  via?: string
}

export interface PublicStatus {
  org_name: string
  version: string
  time: string
}

/** One side of a scope (targets or exclusions). Buildings/departments are names; network segments are IDs; users are usernames. */
export interface ScopeSelection {
  computer_ids: string[]
  group_ids: string[]
  buildings: string[]
  departments: string[]
  network_segments: string[]
  users: string[]
}

export interface ScopeLimitations {
  users: string[]
  network_segments: string[]
}

/** Jamf-style scope: in scope = target AND passes every limitation AND NOT excluded. */
export interface Scope {
  all_computers: boolean
  targets: ScopeSelection
  limitations: ScopeLimitations
  exclusions: ScopeSelection
}

export const emptySelection = (): ScopeSelection => ({ computer_ids: [], group_ids: [], buildings: [], departments: [], network_segments: [], users: [] })

export const emptyScope = (): Scope => ({
  all_computers: false,
  targets: emptySelection(),
  limitations: { users: [], network_segments: [] },
  exclusions: emptySelection(),
})

/** Fill in any missing arrays so older/partial server payloads are safe to render and send. */
export function normalizeScope(s: Partial<Scope> | null | undefined): Scope {
  const sel = (v: Partial<ScopeSelection> | undefined): ScopeSelection => ({ ...emptySelection(), ...(v ?? {}) })
  return {
    all_computers: !!s?.all_computers,
    targets: sel(s?.targets),
    limitations: { users: s?.limitations?.users ?? [], network_segments: s?.limitations?.network_segments ?? [] },
    exclusions: sel(s?.exclusions),
  }
}

export interface ScopePreview {
  count: number
  computers: Computer[]
}

export interface Building {
  id: string
  name: string
  computers: number
  created_at: string
  updated_at: string
}

export interface Department {
  id: string
  name: string
  computers: number
  created_at: string
  updated_at: string
}

export interface NetworkSegment {
  id: string
  name: string
  starting_address: string
  ending_address: string
  assign_building: string
  assign_department: string
  computers: number
  created_at: string
  updated_at: string
}

export interface Computer {
  id: string
  udid: string
  serial_number: string
  name: string
  hostname: string
  local_hostname: string
  model: string
  model_name: string
  model_identifier: string
  product_name: string
  os_version: string
  os_build: string
  architecture: string
  cpu: string
  cpu_cores: number | null
  memory_mb: number | null
  disk_total_mb: number | null
  disk_free_mb: number | null
  ip_address: string
  mac_address: string
  last_user: string
  last_user_full_name: string
  filevault_enabled: boolean | null
  sip_enabled: boolean | null
  firewall_enabled: boolean | null
  gatekeeper_enabled: boolean | null
  secure_boot_level: string
  is_supervised: boolean | null
  activation_lock_enabled: boolean | null
  bootstrap_token_escrowed: boolean | null
  battery_percent: number | null
  uptime_seconds: number | null
  mdm_enrolled: boolean
  mdm_enrolled_at: string | null
  mdm_last_checkin: string | null
  mdm_unenrolled_at: string | null
  mdm_profile_identifier: string
  agent_enrolled: boolean
  agent_enrolled_at: string | null
  agent_last_checkin: string | null
  agent_version: string
  mdm_device_info: Record<string, unknown> | null
  security_info: Record<string, unknown> | null
  hardware: Record<string, unknown> | null
  inventory?: Record<string, unknown> | null
  last_inventory_at: string | null
  asset_tag: string
  site: string
  department: string
  building: string
  room: string
  assigned_user: string
  assigned_real_name: string
  assigned_email: string
  assigned_phone: string
  assigned_position: string
  notes: string
  created_at: string
  updated_at: string
}

export interface ComputerApp {
  name: string
  version: string
  bundle_id: string
  path: string
  source: string
}

export interface ComputerProfile {
  identifier: string
  uuid: string
  display_name: string
  organization: string
  is_managed: boolean
  source: string
}

export interface LocalUser {
  username: string
  uid: number | null
  real_name: string
  home: string
  is_admin: boolean
  secure_token: boolean | null
}

export interface EAValue {
  ea_id: string
  name: string
  data_type: string
  value: string
  updated_at: string
}

export type EADataType = 'string' | 'integer' | 'date'
export type EAInputType = 'script' | 'text'

export interface ExtensionAttribute {
  id: string
  name: string
  description: string
  data_type: EADataType
  input_type: EAInputType
  script: string
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface Criterion {
  and_or: 'and' | 'or'
  field: string
  operator: string
  value: string
  open_paren: boolean
  close_paren: boolean
}

export interface ComputerGroup {
  id: string
  name: string
  description: string
  is_smart: boolean
  criteria: Criterion[] | null
  member_count: number
  created_at: string
  updated_at: string
}

export type CriterionFieldType = 'string' | 'number' | 'bool' | 'date' | 'version' | 'group' | 'ea'

export interface CriterionField {
  key: string
  label: string
  type: CriterionFieldType
  operators: string[]
}

export interface GroupPreview {
  count: number
  computers: Computer[]
}

export type ScriptPriority = 'before' | 'after'

export interface Script {
  id: string
  name: string
  category: string
  info: string
  notes: string
  priority: ScriptPriority
  parameter_labels: string[] | null
  contents: string
  os_requirements: string
  created_at: string
  updated_at: string
}

export interface Package {
  id: string
  name: string
  filename: string
  category: string
  info: string
  notes: string
  size_bytes: number
  sha256: string
  priority: number
  created_at: string
  updated_at: string
}

export interface ConfigProfile {
  id: string
  name: string
  description: string
  category: string
  identifier: string
  uuid: string
  level: 'device' | 'user' | ''
  payload?: string
  scope: Scope
  created_at: string
  updated_at: string
}

export interface ConfigProfileWithStatus extends ConfigProfile {
  status: Record<string, number>
}

export type DeploymentStatus = 'pending' | 'installed' | 'failed' | 'removing' | 'removed'

export interface ProfileDeployment {
  profile_id: string
  profile_name?: string
  computer_id: string
  computer_name?: string
  status: DeploymentStatus
  command_uuid: string
  error: string
  updated_at: string
}

export interface PolicyTriggers {
  startup: boolean
  login: boolean
  checkin: boolean
  enrollment_complete: boolean
  network_change: boolean
  custom: string
}

export interface PolicyScript {
  script_id: string
  priority: ScriptPriority
  parameters: string[]
}

export interface PolicyPackage {
  package_id: string
  action: string
}

export interface PolicyReboot {
  enabled: boolean
  message: string
  delay_minutes: number
}

export interface PolicyUserInteraction {
  start_message: string
  complete_message: string
}

export interface PolicySelfService {
  enabled: boolean
  display_name: string
  description: string
  button_text: string
}

export type PolicyFrequency = 'ongoing' | 'once_per_computer' | 'once_per_day' | 'once_per_week' | 'once_per_month'

export interface Policy {
  id: string
  name: string
  enabled: boolean
  category: string
  triggers: PolicyTriggers
  frequency: PolicyFrequency
  scope: Scope
  scripts: PolicyScript[] | null
  packages: PolicyPackage[] | null
  run_command: string
  update_inventory: boolean
  reboot: PolicyReboot
  user_interaction: PolicyUserInteraction
  self_service: PolicySelfService
  created_at: string
  updated_at: string
}

export interface PolicyWithStatus extends Policy {
  status: Record<string, number>
}

export interface PolicyStep {
  type: string
  name: string
  status: string
  exit_code: number
  output: string
  duration_ms: number
}

export type PolicyLogStatus = 'pending' | 'running' | 'completed' | 'failed'

export interface PolicyLog {
  id: string
  policy_id: string
  policy_name?: string
  computer_id: string
  computer_name?: string
  trigger: string
  status: PolicyLogStatus
  started_at: string | null
  completed_at: string | null
  log: string
  steps: PolicyStep[] | null
  created_at: string
}

export type TaskStatus = 'queued' | 'sent' | 'completed' | 'failed' | 'cancelled'

export interface AgentTaskResult {
  status?: string
  exit_code?: number
  stdout?: string
  stderr?: string
  error?: string
  started_at?: string
  completed_at?: string
  [k: string]: unknown
}

export interface AgentTask {
  id: string
  computer_id: string
  computer_name?: string
  type: string
  payload: Record<string, unknown> | null
  status: TaskStatus
  result: AgentTaskResult | null
  created_by: string
  created_at: string
  sent_at: string | null
  completed_at: string | null
}

export type MDMCommandStatus = 'Pending' | 'Acknowledged' | 'Error' | 'NotNow' | 'CommandFormatError' | string

export interface MDMCommand {
  command_uuid: string
  enrollment_id: string
  computer_id: string
  computer_name?: string
  request_type: string
  kind: string
  status: MDMCommandStatus
  active: boolean
  created_at: string
  result_updated_at: string | null
  command?: string
  result?: string
  created_by: string
}

export interface APIToken {
  id: string
  name: string
  token_prefix: string
  role: Role
  created_at: string
  last_used_at: string | null
  expires_at: string | null
}

export interface EnrollmentInvitation {
  id: string
  name: string
  token_prefix: string
  enabled: boolean
  max_uses: number | null
  uses: number
  expires_at: string | null
  created_at: string
}

export interface AuditEntry {
  id: number
  at: string
  actor: string
  action: string
  target_type: string
  target_id: string
  details: Record<string, unknown> | null
}

export interface SettingsView {
  org_name: string
  server_url: string
  agent_checkin_interval_seconds: number
  agent_enrollment_secret?: string
  enrollment_profile_name: string
  enrollment_profile_identifier: string
  open_enrollment: boolean
  inventory_interval_hours: number
  signing_configured: boolean
  signing_cert_subject?: string
  remote_shell_enabled: boolean
  remote_shell_idle_timeout_secs: number
  version: string
}

export interface SettingsUpdate {
  org_name?: string
  server_url?: string
  agent_checkin_interval_seconds?: number
  enrollment_profile_name?: string
  enrollment_profile_identifier?: string
  open_enrollment?: boolean
  inventory_interval_hours?: number
  signing_cert_pem?: string
  signing_key_pem?: string
  clear_signing?: boolean
  remote_shell_enabled?: boolean
  remote_shell_idle_timeout_secs?: number
}

export interface APNsCert {
  topic: string
  subject: string
  not_after: string
  not_before: string
  expired: boolean
}

export interface APNsStatus {
  configured: boolean
  certificates: APNsCert[]
  pending_request?: { csr_pem: string; created_at: string }
}

export interface APNsRequestResponse {
  csr_pem: string
  submitted: boolean
  submit_error: string
}

export interface SCEPStatus {
  subject: string
  not_after: string
  not_before: string
  issued: number
  active: number
  ca_pem: string
  challenge?: string
}

export interface EnrollmentStatus {
  server_url: string
  apns_configured: boolean
  topics: string[] | null
  scep_ok: boolean
  agent_binaries: Record<string, boolean>
  open_enrollment: boolean
  invitations: number
  issues: string[]
  enroll_url: string
  install_command?: string
}

export interface InvitationCreated {
  invitation: EnrollmentInvitation
  token: string
  enroll_url: string
  install_command: string
}

export interface Count {
  label: string
  count: number
}

export interface DashboardStats {
  computers: number
  mdm_enrolled: number
  agent_enrolled: number
  agents_online: number
  checked_in_24h: number
  not_seen_30d: number
  filevault_enabled: number
  filevault_disabled: number
  policies: number
  policies_enabled: number
  scripts: number
  profiles: number
  groups: number
  pending_commands: number
  pending_tasks: number
  failed_policies_7d: number
  completed_policies_7d: number
  os_versions: Count[]
  models: Count[]
  recent_checkins: Computer[]
  recent_policy_logs: PolicyLog[]
  recent_tasks: AgentTask[]
}

export interface ComputerDetail extends Computer {
  apps: ComputerApp[] | null
  profiles: ComputerProfile[] | null
  local_users: LocalUser[] | null
  extension_attributes: EAValue[] | null
  groups: ComputerGroup[]
  profile_deployments: ProfileDeployment[] | null
  agent_online: boolean
}

export interface ComputerEdit {
  name?: string
  asset_tag?: string
  site?: string
  department?: string
  building?: string
  room?: string
  assigned_user?: string
  assigned_real_name?: string
  assigned_email?: string
  assigned_phone?: string
  assigned_position?: string
  notes?: string
}

export type MDMCommandType =
  | 'DeviceInformation'
  | 'SecurityInfo'
  | 'ProfileList'
  | 'InstalledApplicationList'
  | 'CertificateList'
  | 'DeviceLock'
  | 'EraseDevice'
  | 'RestartDevice'
  | 'ShutDownDevice'
  | 'EnableRemoteDesktop'
  | 'DisableRemoteDesktop'
  | 'InstallEnterpriseApplication'
  | 'AvailableOSUpdates'
  | 'ScheduleOSUpdate'
  | 'SetRecoveryLock'
  | 'DeviceConfigured'
  | 'InstallProfile'
  | 'RemoveProfile'
  | 'RemoveMDMProfile'
  | 'Raw'

export interface SendCommandRequest {
  type: MDMCommandType
  pin?: string
  message?: string
  phone?: string
  profile_id?: string
  identifier?: string
  manifest_url?: string
  product_key?: string
  install_action?: string
  current_password?: string
  new_password?: string
  plist?: string
}

export interface SendCommandResponse {
  ok: boolean
  command_uuid?: string
  request_type?: string
  push_error?: string
}

export type AgentTaskType =
  | 'run_script'
  | 'run_command'
  | 'run_policy'
  | 'inventory'
  | 'custom_trigger'
  | 'restart'
  | 'shutdown'
  | 'logout_user'
  | 'notify'

export interface CreateTaskRequest {
  type: AgentTaskType
  script_id?: string
  parameters?: string[]
  command?: string
  policy_id?: string
  trigger?: string
  message?: string
}

export interface RunTargetsRequest {
  computer_ids: string[]
  group_ids: string[]
  parameters?: string[]
  scoped?: boolean
}

export interface RunResult {
  queued: number
  skipped: string[]
}

export interface CheckinResponse {
  ok: boolean
  agent_online: boolean
}

export interface UpdateInventoryResponse {
  ok: boolean
  mdm_error: string
  agent_error: string
}

export interface ComputerListParams {
  q?: string
  group_id?: string
  sort?: string
  order?: 'asc' | 'desc'
  limit?: number
  offset?: number
  ids?: string[]
}


export type ShellSessionStatus = 'pending' | 'connecting' | 'active' | 'closed' | 'error' | string

export interface ShellSession {
  id: string
  computer_id: string
  computer_name: string
  actor: string
  actor_ip: string
  status: ShellSessionStatus
  bytes_in: number
  bytes_out: number
  started_at: string
  connected_at: string | null
  ended_at: string | null
}

export interface ShellSessionDetail {
  session: ShellSession
  transcript: string
}

export type RestrictedMatchType = 'exact' | 'contains' | 'regex'

export interface RestrictedSoftware {
  id: string
  name: string
  process_name: string
  match_type: RestrictedMatchType
  kill_process: boolean
  notify_user: boolean
  display_message: string
  enabled: boolean
  scope: Scope
  created_at: string
  updated_at: string
}

export interface RestrictedEvent {
  id: number
  restricted_id: string
  computer_id: string
  computer_name?: string
  rule_name: string
  process: string
  username: string
  killed: boolean
  at: string
}

export interface AdvancedSearch {
  id: string
  name: string
  criteria: Criterion[] | null
  display_fields: string[] | null
  created_at: string
  updated_at: string
}

/** Display/export field option from GET /advanced-searches/fields (note the capitalized keys). */
export interface SearchField {
  Key: string
  Label: string
}

export interface SearchRunResult {
  search: AdvancedSearch
  count: number
  computers: Computer[]
}

export type AppUpdateBehavior = 'install_once' | 'keep_updated'

export interface MacApp {
  id: string
  name: string
  label: string
  category: string
  description: string
  update_behavior: AppUpdateBehavior
  enabled: boolean
  scope: Scope
  self_service: PolicySelfService
  created_at: string
  updated_at: string
}

export interface MacAppWithStatus extends MacApp {
  status: Record<string, number>
}

export interface AppLabels {
  labels: string[]
  source: string
  license_url: string
}

export type AppInstallStatus = 'pending' | 'running' | 'completed' | 'failed' | string

export interface AppInstallLog {
  id: string
  app_id: string
  app_name?: string
  computer_id: string
  computer_name?: string
  status: AppInstallStatus
  trigger: string
  log: string
  created_at: string
  completed_at: string | null
}

export interface Webhook {
  id: string
  name: string
  url: string
  secret?: string
  enabled: boolean
  events: string[]
  created_at: string
  updated_at: string
}

export interface WebhookList {
  items: Webhook[]
  total: number
  available_events: string[]
}

export interface WebhookRequest {
  name: string
  url: string
  /** Omit the key entirely on update to keep the existing secret. */
  secret?: string
  enabled: boolean
  events: string[]
}

export interface WebhookDelivery {
  id: number
  webhook_id: string
  webhook_name: string
  event: string
  status: number
  error: string
  attempts: number
  at: string
}
