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
  /** Bootstrap accounts must choose a new password; every other API call answers 403 until they do. */
  must_change_password?: boolean
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
  /** "" | "custom:<uuid>" | "builtin:<domain>" when the profile was built from a JSON schema form. */
  schema_source?: string
  schema_values?: Record<string, unknown> | null
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
  /** Installomator steps; run after packages and before the run command. */
  apps: PolicyApp[] | null
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
  /** Set instead of computer_id for iPhone and iPad commands. */
  mobile_device_id?: string
  /** The device's name (computer or mobile device). */
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
  filevault_auto_remediate: boolean
  /** True when VIBEMDM_ENCRYPTION_KEY is set and secrets are sealed at rest (read-only). */
  encryption_at_rest: boolean
  /** Days log entries are kept in the database (1–3650). */
  log_retention_days: number
  /** Write daily gzipped NDJSON archives before old entries are pruned. */
  log_archive_enabled: boolean
  /** Expose the MCP server for AI agents at /mcp. */
  mcp_enabled: boolean
  /** Let admin API tokens call the MCP write tools. */
  mcp_allow_writes: boolean
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
  log_retention_days?: number
  log_archive_enabled?: boolean
  mcp_enabled?: boolean
  mcp_allow_writes?: boolean
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
  /** Apple Configurator "Enroll in MDM Server" URL (admin only). */
  configurator_url?: string
}

export interface InvitationCreated {
  invitation: EnrollmentInvitation
  token: string
  enroll_url: string
  install_command: string
  configurator_url: string
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
  filevault: FileVaultStatus | null
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
  options: InstallomatorOptions
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
  /** Custom labels as "custom:<key>". */
  custom_labels: string[] | null
  choices: InstallomatorChoices
  source: string
  license_url: string
}

export interface InstallomatorChoices {
  notify: string[]
  blocking_process_action: string[]
  reopen: string[]
  ignore_app_store_apps: string[]
  type: string[]
}

/** Runtime options for an Installomator run (model.InstallomatorOptions). Empty string = Installomator default. */
export interface InstallomatorOptions {
  notify: string
  blocking_process_action: string
  reopen: string
  ignore_app_store_apps: string
  force_install: boolean
}

export const emptyInstallomatorOptions = (): InstallomatorOptions => ({ notify: '', blocking_process_action: '', reopen: '', ignore_app_store_apps: '', force_install: false })

/** An Installomator step in a policy. `label` is a shipped label or "custom:<key>". */
export interface PolicyApp {
  label: string
  options: InstallomatorOptions
}

/** Admin-defined Installomator label (model.CustomLabel). */
export interface CustomLabel {
  id: string
  key: string
  name: string
  type: string
  download_url: string
  expected_team_id: string
  package_id: string
  app_name: string
  archive_name: string
  version_key: string
  app_new_version: string
  blocking_processes: string[] | null
  description: string
  created_at: string
  updated_at: string
}

/** POST /apps/installomator-preview: either `label` or `custom`, plus options. */
export interface InstallomatorPreviewRequest {
  label?: string
  custom?: Partial<CustomLabel>
  options: InstallomatorOptions
}

export interface InstallomatorPreview {
  args: string[]
  command: string
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

// ---- FileVault escrow (internal/model FileVault*, internal/api/filevault.go) ----

/** A computer's escrow state. Never contains the key. */
export interface FileVaultStatus {
  escrowed: boolean
  fingerprint: string
  escrowed_at: string | null
  rotated_count: number
  source: string
  /** Deployment status of the escrow profile on this Mac (pending | installed | failed | removing | removed | ""). */
  escrow_profile_status: string
}

export interface FileVaultRow {
  computer_id: string
  computer_name: string
  serial_number: string
  filevault_enabled: boolean | null
  mdm_enrolled: boolean
  agent_enrolled: boolean
  escrowed: boolean
  fingerprint: string
  escrowed_at: string | null
  rotated_count: number
  escrow_profile_status: string
}

export interface FileVaultSummary {
  total: number
  escrowed: number
  missing_key: number
  not_encrypted: number
  unknown: number
}

export interface BuiltinProfileView {
  deployed: boolean
  profile?: ConfigProfile
  status: Record<string, number>
  /** Enforcement profile only: the currently deployed options (absent when no enforcement profile exists). */
  options?: FileVaultEnforceOptions
}

export interface FileVaultOverview {
  summary: FileVaultSummary
  computers: FileVaultRow[] | null
  auto_remediate: boolean
  escrow_profile: BuiltinProfileView
  enforce_profile: BuiltinProfileView
  escrow_certificate?: { subject: string; not_after: string }
}

/** escrow.EnforceOptions has no json tags, so Go serializes the field names verbatim. */
export interface FileVaultEnforceOptions {
  DeferAtLogout: boolean
  DeferDontAskAtLogout: boolean
  /** -1 = unlimited deferrals, 0..9999 */
  MaxBypassAttempts: number
  ShowRecoveryKey: boolean
}

export interface FileVaultUpdate {
  escrow?: { enabled: boolean; scope: Scope }
  enforce?: { enabled: boolean; scope: Scope; options: FileVaultEnforceOptions }
  auto_remediate?: boolean
}

export interface FileVaultReveal {
  key: string
  status: FileVaultStatus
}

// ---- MDM command inspector (internal/api/inspector.go) ----

export type CommandEventName = 'queued' | 'push_sent' | 'push_failed' | 'connected' | 'delivered' | 'not_now' | 'acknowledged' | 'error' | 'cancelled' | string

export interface CommandEvent {
  id: number
  command_uuid: string
  enrollment_id: string
  event: CommandEventName
  description: string
  details: Record<string, unknown> | null
  request_type: string
  computer_id: string
  computer_name: string
  at: string
}

export interface CommandInspection {
  command: MDMCommand
  summary: string
  result_summary: string
  events: CommandEvent[] | null
  redacted: boolean
  can_reveal: boolean
}

export interface CommandSearchHit {
  command_uuid: string
  request_type: string
  computer_id: string
  computer_name: string
  status: string
  /** command | result */
  matched_in: string
  snippet: string
  created_at: string
}

// ---- Logs & diagnostics (internal/api/logs.go, internal/store/logs.go) ----

export type LogSource = 'server' | 'agent'
export type LogLevel = 'debug' | 'info' | 'warn' | 'error'

export interface LogEntry {
  id: number
  source: LogSource
  computer_id?: string
  computer_name?: string
  at: string
  /** Upper-case as stored: DEBUG | INFO | WARN | ERROR. */
  level: string
  component: string
  msg: string
  attrs: Record<string, unknown> | null
}

/** Filters shared by GET /logs and GET /logs/export. */
export interface LogQuery {
  source: LogSource
  /** Minimum level. */
  level?: LogLevel
  component?: string
  computer_id?: string
  q?: string
  /** RFC3339 */
  from?: string
  /** RFC3339 (exclusive) */
  to?: string
}

/** GET /logs. Newest first, or oldest first when `since_id` is set. */
export interface LogPage {
  items: LogEntry[]
  last_id: number
  next_before_id?: number
}

export interface LogArchive {
  name: string
  size_bytes: number
  modified_at: string
}

export type DiagnosticStatus = 'requested' | 'uploaded' | 'failed'

export interface DiagnosticBundle {
  id: string
  computer_id: string
  computer_name: string
  status: DiagnosticStatus | string
  filename: string
  size_bytes: number
  sha256: string
  error: string
  requested_by: string
  created_at: string
  uploaded_at: string | null
}

// ---- Software updates / Declarative Device Management (internal/api/updates.go, internal/ddm) ----

export type UpdatePlanKind = 'enforcement' | 'settings'
export type UpdateAction = 'Allowed' | 'AlwaysOn' | 'AlwaysOff'

/** ddm.Settings: every field is optional; omitted means "leave the macOS default". */
export interface UpdateSettings {
  notifications?: boolean
  allow_standard_user_os_updates?: boolean
  /** 1–90 */
  major_deferral_days?: number
  minor_deferral_days?: number
  system_deferral_days?: number
  auto_download?: UpdateAction
  auto_install_os_updates?: UpdateAction
  auto_install_security_updates?: UpdateAction
  rapid_security_response?: boolean
  rapid_security_response_rollback?: boolean
}

/** model.UpdatePlan */
export interface UpdatePlan {
  id: string
  name: string
  enabled: boolean
  kind: UpdatePlanKind
  target_os_version: string
  target_build_version: string
  /** "YYYY-MM-DDTHH:MM:SS" in the Mac's local time. */
  target_local_datetime: string
  details_url: string
  settings: UpdateSettings | null
  scope: Scope
  created_at: string
  updated_at: string
}

export type UpdateState = 'up_to_date' | 'pending' | 'failed' | 'not_targeted' | 'no_ddm' | 'unsupported'

/** api.updateRow */
export interface UpdateRow {
  computer_id: string
  computer_name: string
  os_version: string
  os_build: string
  mdm_enrolled: boolean
  enforcement_plan: string
  target_os_version: string
  deadline: string
  settings_plan: string
  state: UpdateState
  install_state: string
  pending_version?: Record<string, unknown>
  failure_reason?: Record<string, unknown>
  declarations_in_sync: boolean
  last_status_at: string | null
}

export interface UpdatesOverview {
  computers: UpdateRow[] | null
  /** Only states with at least one Mac are present. */
  counts: Partial<Record<UpdateState, number>>
}

export interface CatalogRelease {
  version: string
  release_date: string
  update_name: string
}

export interface CatalogVersion {
  name: string
  latest_version: string
  latest_build: string
  release_date: string
  actively_exploited_cves: string[] | null
  recent_releases: CatalogRelease[] | null
}

export interface UpdatesCatalog {
  versions: CatalogVersion[]
  source: string
  fetched_at: string
}

/** ddm.Declaration (no json renames: Apple's field names). */
export interface DDMDeclaration {
  Type: string
  Identifier: string
  ServerToken: string
  Payload: Record<string, unknown> | null
}

/** model.DDMState */
export interface DDMState {
  computer_id: string
  served_token: string
  sync_sent_token: string
  sync_sent_at: string | null
  last_sync_at: string | null
  last_status_at: string | null
  status: Record<string, unknown> | null
  declarations_status: Record<string, unknown> | null
  errors: unknown[] | null
}

export interface DDMReport {
  at: string
  report: Record<string, unknown>
}

export interface ComputerDDM {
  summary: UpdateRow
  declarations: DDMDeclaration[] | null
  declarations_token: string
  state: DDMState | null
  recent_reports: DDMReport[] | null
}

// ---- Health: misconfiguration detection & self-healing (internal/api/health.go, internal/health) ----

export type HealthSeverity = 'critical' | 'warning' | 'info'
/** "pending" is internal (grace period) and never listed by default. */
export type HealthFindingState = 'pending' | 'open' | 'acknowledged' | 'snoozed' | 'resolved'
export type HealthObjectType = 'server' | 'computer' | 'policy' | 'profile' | 'app' | 'webhook' | 'plan'
export type HealthCategory = 'platform' | 'fleet' | 'configuration' | 'security'

/** model.HealthRemediation */
export interface HealthRemediation {
  id: string
  label: string
  description: string
  /** May run automatically when auto-heal is on for the check. */
  safe: boolean
  /** The server can perform it (POST remediate); otherwise it is guidance, optionally with an in-app `url`. */
  runnable: boolean
  url?: string
}

/** model.HealthFinding */
export interface HealthFinding {
  id: string
  check_id: string
  check_title?: string
  category?: HealthCategory | string
  object_type: HealthObjectType | string
  object_id: string
  object_name: string
  severity: HealthSeverity
  summary: string
  detail: string
  facts: Record<string, unknown> | null
  remediations: HealthRemediation[] | null
  state: HealthFindingState
  snoozed_until: string | null
  acknowledged_by: string
  first_seen: string
  last_seen: string
  resolved_at: string | null
  occurrences: number
  heal_attempts: number
  last_heal_at: string | null
  last_heal_error: string
}

export type HealthEventName =
  | 'opened'
  | 'reopened'
  | 'resolved'
  | 'escalated'
  | 'auto_heal'
  | 'heal_failed'
  | 'acknowledged'
  | 'snoozed'
  | 'unsnoozed'
  | 'open'
  | 'remediated'
  | 'remediation_failed'

/** model.HealthEvent */
export interface HealthEvent {
  id: number
  finding_id: string
  check_id: string
  event: HealthEventName | string
  actor: string
  message: string
  at: string
}

/** health.CheckInfo */
export interface HealthCheckInfo {
  id: string
  title: string
  category: HealthCategory | string
  description: string
  grace_seconds: number
  auto_heal_default: boolean
  enabled: boolean
  /** Effective setting (override or default). */
  auto_heal: boolean
  supports_heal: boolean
  last_run_error?: string
}

/** api.healthGroup. `severity` is the worst open severity, or "acknowledged" when nothing in the group is open. */
export interface HealthGroup {
  check_id: string
  title: string
  category: HealthCategory | string
  severity: HealthSeverity | 'acknowledged'
  count: number
  open: number
  /** Up to three findings. */
  samples: HealthFinding[]
  first_seen: string
}

/** GET /health/summary (also returned by POST /health/run). Snoozed findings are excluded from groups. */
export interface HealthSummary {
  /** critical/warning/info count state=open only. */
  counts: { critical: number; warning: number; info: number; acknowledged: number; snoozed: number }
  groups: HealthGroup[]
  last_run: string | null
  checks_enabled: number
  checks_total: number
  check_errors: { check_id: string; title: string; error: string }[]
}

export interface HealthFindingDetail {
  finding: HealthFinding
  events: HealthEvent[] | null
  check_description: string
}

export interface HealthFindingQuery {
  /** Comma-separated; default open,acknowledged,snoozed. */
  state?: string
  severity?: string
  check?: string
  object_type?: string
  object_id?: string
  limit?: number
}

export type HealthBulkAction = 'acknowledge' | 'snooze' | 'reopen' | 'remediate'

export interface HealthBulkResult {
  succeeded: number
  failed: Record<string, string>
}

export interface HealthRemediateResult {
  resolved: boolean
  message: string
}

// ---- Compliance (mSCP) ----

export interface ComplianceCatalogBaseline {
  key: string
  title: string
  description: string
  rules: number
  scannable: number
  with_profile: number
}

export interface ComplianceCatalog {
  source: string
  release: string
  platform: string
  license: string
  baselines: ComplianceCatalogBaseline[]
}

export type ComplianceSeverity = '' | 'low' | 'medium' | 'high'

export interface ComplianceODVHint {
  datatype: string
  description: string
  min?: number
  max?: number
}

/** api.ruleView */
export interface ComplianceRule {
  id: string
  title: string
  section: string
  discussion: string
  tags: string[] | null
  severity: ComplianceSeverity | string
  /** Empty when the rule can be scanned automatically. */
  manual_reason: string
  has_fix: boolean
  has_profile: boolean
  platforms: string[] | null
  arch?: string
  in_benchmark: boolean
  odv?: { hint: ComplianceODVHint; default: unknown }
  references?: Record<string, unknown>
  check?: string
  fix?: string
  expected?: string
}

/** model.ComplianceBaseline */
export interface ComplianceBaseline {
  id: string
  name: string
  description: string
  benchmark: string
  enabled: boolean
  scope: Scope
  extra_rules: string[] | null
  /** rule id → reason */
  exclusions: Record<string, string> | null
  odv_overrides: Record<string, unknown> | null
  scan_interval_hours: number
  auto_remediate: boolean
  deploy_profile: boolean
  profile_id: string | null
  created_at: string
  updated_at: string
}

export interface ComplianceBaselineRow extends ComplianceBaseline {
  /** Computers with at least one result. */
  computers: number
  compliant: number
  /** Average percent passing, 0–100. */
  score: number
}

export interface ComplianceComputerSummary {
  computer_id: string
  computer_name: string
  serial_number: string
  os_version: string
  passed: number
  failed: number
  errors: number
  remediated: number
  total: number
  /** Percent passing (remediated counts as passing), 0–100. */
  score: number
  last_scan_at: string | null
  last_scan_status: string
}

export interface ComplianceReportRule extends ComplianceRule {
  rule_id: string
  passed: number
  failed: number
  errors: number
  remediated: number
  drifted: number
  /** Exemption reason, or "" when enforced. */
  excluded: string
}

export interface ComplianceReport {
  baseline: ComplianceBaseline
  release: string
  benchmark_title: string
  summary: { targeted: number; scanned: number; compliant: number; score: number; failing_rules: number; drifted: number; remediated: number }
  computers: ComplianceComputerSummary[] | null
  rules: ComplianceReportRule[] | null
}

export type ComplianceRuleStatus = 'pass' | 'fail' | 'error' | 'remediated' | 'not_scanned' | 'excluded' | 'manual' | 'not_applicable'

export interface ComplianceComputerRule {
  id: string
  title: string
  section: string
  severity: string
  status: ComplianceRuleStatus | string
  previous_status?: string
  actual?: string
  expected?: string
  detail?: string
  checked_at?: string
  has_fix: boolean
}

/** model.ComplianceScan */
export interface ComplianceScan {
  id: string
  baseline_id: string
  computer_id: string
  computer_name?: string
  trigger: string
  remediate: boolean
  status: 'pending' | 'completed' | 'failed' | 'expired' | string
  os_version: string
  passed: number
  failed: number
  remediated: number
  errors: number
  total: number
  duration_ms: number
  error: string
  requested_at: string
  completed_at: string | null
}

export interface ComplianceComputerDetail {
  baseline: ComplianceBaseline
  computer: { id: string; name: string; os_version: string; agent_enrolled: boolean }
  rules: ComplianceComputerRule[] | null
  scans: ComplianceScan[] | null
}

export interface ComplianceScanResult {
  queued: number
  skipped: Record<string, string> | null
}

export interface ComplianceProfilePreview {
  payload: string
  rules: string[] | null
  profile_id: string | null
  deployed: boolean
}

export interface ComputerComplianceRow {
  baseline: ComplianceBaseline
  in_scope: boolean
  summary: Omit<ComplianceComputerSummary, 'computer_id' | 'computer_name' | 'serial_number' | 'os_version'> | null
}

// ---- Application & Custom Settings: JSON schema profiles (internal/api/schemas.go, internal/schema) ----

/** One node of a Jamf-style "external application" schema (a json-editor subset of JSON Schema). */
export interface SchemaNode {
  title?: string
  description?: string
  type?: string | string[]
  properties?: Record<string, SchemaNode>
  items?: SchemaNode
  enum?: unknown[]
  default?: unknown
  minimum?: number
  maximum?: number
  minLength?: number
  maxLength?: number
  minItems?: number
  maxItems?: number
  pattern?: string
  /** VibeMDM extension: "data" marks a base64 string delivered as plist <data>. */
  format?: string
  required?: string[]
  property_order?: number
  propertyOrder?: number
  anyOf?: SchemaNode[]
  oneOf?: SchemaNode[]
  links?: { rel?: string; href: string }[]
  options?: {
    enum_titles?: string[]
    infoText?: string
    inputAttributes?: { placeholder?: string; [k: string]: unknown }
    remove_empty_properties?: boolean
    [k: string]: unknown
  }
}

export interface JSONSchemaDoc extends SchemaNode {
  __preferencedomain?: string
  __version?: string
}

/** api.schemaView. Built-in ids look like "builtin:com.macjutsu.super"; custom ids are UUIDs. */
export interface CustomSchemaSummary {
  id: string
  name: string
  preference_domain: string
  builtin: boolean
  title: string
  description: string
  properties: number
  profiles: number
  updated_at?: string
}

export interface CustomSchemaDetail extends CustomSchemaSummary {
  schema: JSONSchemaDoc
}

/** Validation problem at a path like "key", "parent.child" or "list[0].key". */
export interface SchemaFieldError {
  path: string
  message: string
}

export interface SchemaPreview {
  values: Record<string, unknown>
  errors: SchemaFieldError[]
  payload: string
}

export type SchemaValues = Record<string, unknown>

// ---- Software updates → super (internal/api/super.go) ----

export type SuperAuthMode = 'none' | 'local' | 'service'

export interface SuperConfig {
  enabled: boolean
  scope: Scope
  auth_mode: SuperAuthMode
  auth_account: string
  has_password: boolean
  profile_id: string | null
  version: string
  updated_at: string
}

export interface SuperView {
  config: SuperConfig
  values: SchemaValues
  schema_id: string
  version: string
  script_url: string
  sha256: string
  license: string
  project: string
}

export interface SuperSaveRequest {
  enabled: boolean
  scope: Scope
  auth_mode: SuperAuthMode
  auth_account: string
  /** Omit to keep the stored password; "" clears it. */
  auth_password?: string
  values: SchemaValues
}

export type SuperState = 'current' | 'outdated' | 'missing'

/** The super.plist keys the agent reports (all optional, strings as super writes them). */
export interface SuperReport {
  SuperVersion?: string
  SuperStatus?: string
  NextAutoLaunch?: string
  DeadlineCounterSoft?: string | number
  DeadlineCounterHard?: string | number
  DeadlineCounterFocus?: string | number
  WorkflowTarget?: string
  LastSuccessfulCheckDate?: string
  [k: string]: unknown
}

export interface SuperStatusRow {
  computer_id: string
  computer_name: string
  os_version: string
  agent_recent: boolean
  super: SuperReport | null
  last_inventory_at: string | null
  state: SuperState
}

export interface SuperStatus {
  summary: { targeted: number; current: number; outdated: number; missing: number }
  computers: SuperStatusRow[]
  version: string
  enabled: boolean
}

export interface SuperInstallResult {
  queued: number
  skipped?: Record<string, string> | null
}

// ---- Identity → XCreds (internal/api/identity.go) ----

export interface XCredsView {
  enabled: boolean
  scope: Scope
  /** LicenseFile is always "" here; check has_license. */
  values: SchemaValues
  schema_id: string
  profile_id: string | null
  app: { id: string; enabled: boolean; status: Record<string, number> | null } | null
  has_license?: boolean
  license: string
  project: string
}

export interface XCredsSaveRequest {
  enabled: boolean
  scope: Scope
  values: SchemaValues
  install_app: boolean
}

// ---- White-glove onboarding (swiftDialog) ----

/** Inventory fields an onboarding form may collect; the server lists the allowed keys with labels. */
export type OnboardingFieldKey = 'computer_name' | 'asset_tag' | 'building' | 'department' | 'room' | 'assigned_user' | 'assigned_real_name' | 'assigned_email' | 'assigned_position'

export interface OnboardingField {
  key: OnboardingFieldKey | string
  label: string
  required: boolean
  placeholder: string
  /** When set, the field is shown as a picker instead of a text box. */
  options: string[] | null
  regex: string
  regex_error: string
}

export type OnboardingStepType = 'app' | 'policy' | 'script' | 'recon' | 'wait'

export interface OnboardingStep {
  type: OnboardingStepType
  title: string
  subtitle: string
  icon: string
  app_id?: string
  policy_id?: string
  script_id?: string
  seconds?: number
  continue_on_error: boolean
}

export type OnboardingFinalAction = 'none' | 'restart' | 'logout' | 'shutdown'

export interface OnboardingConfig {
  title: string
  /** Markdown is allowed. */
  message: string
  /** Path, URL or SF Symbol ("SF=laptopcomputer"). */
  icon: string
  banner: string
  blur_screen: boolean
  fields: OnboardingField[] | null
  /** e.g. "MAC-{serial}"; a collected computer_name field overrides it. */
  computer_name_template: string
  steps: OnboardingStep[] | null
  final_action: OnboardingFinalAction | ''
  final_message: string
}

/** model.OnboardingWorkflow */
export interface OnboardingWorkflow {
  id: string
  name: string
  enabled: boolean
  /** Lower wins when several workflows are in scope (default 100). */
  priority: number
  scope: Scope
  config: OnboardingConfig
  created_at: string
  updated_at: string
}

export interface OnboardingWorkflowList {
  items: OnboardingWorkflow[] | null
  /** key → label */
  field_keys: Record<string, string>
}

export type OnboardingRunStatus = 'queued' | 'running' | 'completed' | 'failed'
export type OnboardingStepStatus = 'success' | 'failed' | 'skipped'

export interface OnboardingStepResult {
  title: string
  type: string
  status: OnboardingStepStatus | string
  detail: string
  duration_ms: number
}

/** model.OnboardingRun */
export interface OnboardingRun {
  id: string
  workflow_id: string
  workflow_name?: string
  computer_id: string
  computer_name?: string
  status: OnboardingRunStatus | string
  user_entry: Record<string, string> | null
  steps: OnboardingStepResult[] | null
  log?: string
  created_at: string
  started_at: string | null
  completed_at: string | null
}

export interface OnboardingRunResult {
  queued: number
  /** computer name → reason */
  skipped: Record<string, string> | null
}

// ---- LAPS: local administrator password (internal/model LAPS*, internal/api/laps.go) ----

export interface LAPSConfig {
  enabled: boolean
  scope: Scope
  account_name: string
  create_account: boolean
  /** Hide the account from the login window and Users & Groups. */
  hide_account: boolean
  /** "jamf": A-Z and 2-9 in dash-separated groups of five (like Jamf Pro); "complex": every character class. */
  password_format: 'jamf' | 'complex'
  password_length: number
  /** Rotate passwords that were never viewed after rotation_days (Jamf: autoRotateEnabled). */
  auto_rotate: boolean
  rotation_days: number
  /** Minutes after a view before the password rotates (Jamf: passwordRotationTime). Always on. */
  rotate_after_use_minutes: number
}

export type LAPSRotationStatus = 'ok' | 'due' | 'pending' | 'missing' | 'failing' | 'out_of_scope'

/** One Mac's LAPS state. Never contains a password. */
export interface LAPSStatus {
  in_scope: boolean
  has_password: boolean
  account: string
  fingerprint: string
  escrowed_at: string | null
  expires_at: string | null
  rotated_count: number
  pending: boolean
  pending_at: string | null
  last_viewed_at: string | null
  last_viewed_by: string
  rotate_after: string | null
  rotate_requested: boolean
  last_error: string
  last_error_at: string | null
  agent_enrolled: boolean
  computer_id?: string
  computer_name?: string
  serial_number?: string
  agent_last_seen?: string | null
  rotation_status?: LAPSRotationStatus
  guid: string
  /** Set while an administrator-chosen password waits to be applied. */
  password_set_by?: string
}

export interface LAPSView {
  viewed_by: string
  reason: string
  viewed_at: string
}

/** One password a Mac has had (never includes the password in the console API). */
export interface LAPSHistoryEntry {
  account: string
  guid: string
  fingerprint: string
  created_at: string
  retired_at: string | null
  views: LAPSView[]
}

export type LAPSEventType = 'CREATED' | 'ROTATED' | 'VIEWED' | 'PENDING' | 'SET' | 'ERROR'

export interface LAPSEvent {
  id: number
  account: string
  event_type: LAPSEventType
  actor: string
  detail: string
  at: string
}

export interface LAPSHistory {
  events: LAPSEvent[]
  passwords: LAPSHistoryEntry[]
}

export interface LAPSPendingRotation {
  computer_id: string
  computer_name: string
  account: string
  guid: string
  reason: 'viewed' | 'requested' | 'set' | 'expired' | 'staged' | 'missing'
  since: string
  due_at: string | null
}

export interface LAPSSummary {
  in_scope: number
  ok: number
  due: number
  pending: number
  missing: number
  failing: number
}

export interface LAPSOverview {
  config: LAPSConfig
  computers: LAPSStatus[] | null
  summary: LAPSSummary
}

export interface LAPSReveal {
  account: string
  password: string
  /** Staged but never confirmed by the agent; may be the live password. */
  unconfirmed_password?: string
  status: LAPSStatus
}

// ---- Automatic agent install through MDM (internal/agentinstall, internal/api/agentinstall.go) ----

export interface AgentPackage {
  file_name: string
  version: string
  size: number
  sha256: string
  md5_chunk: number
  md5s: string[]
  signed_by: string
  uploaded_by: string
  uploaded_at: string
}

export interface AgentPackageStatus {
  auto_install: boolean
  package: AgentPackage | null
  ca_fingerprint?: string
}

export interface AgentInstallAttempt {
  id: number
  computer_id: string
  command_uuid: string
  status: 'sent' | 'installed' | 'failed'
  detail: string
  created_by: string
  created_at: string
  updated_at: string
}

// ---- Mobile devices (internal/model MobileDevice*, internal/api/mobile.go) ----

/** An iPhone, iPad, iPod touch or Apple TV. Kept apart from computers, as in Jamf Pro. */
export interface MobileDevice {
  id: string
  udid: string
  serial_number: string
  name: string
  device_type: 'iPhone' | 'iPad' | 'iPod' | 'AppleTV' | ''
  model: string
  model_name: string
  product_name: string
  os_version: string
  os_build: string
  capacity_gb: number | null
  available_gb: number | null
  /** 0..1 */
  battery_level: number | null
  wifi_mac: string
  bluetooth_mac: string
  imei: string
  meid: string
  phone_number: string
  carrier: string
  cellular_technology: string
  is_roaming: boolean | null
  is_supervised: boolean | null
  is_shared_ipad: boolean | null
  activation_lock_enabled: boolean | null
  find_my_enabled: boolean | null
  lost_mode_enabled: boolean | null
  passcode_present: boolean | null
  passcode_compliant: boolean | null
  hardware_encryption: number | null
  last_backup_at: string | null
  mdm_enrolled: boolean
  mdm_enrolled_at: string | null
  mdm_last_checkin: string | null
  mdm_unenrolled_at: string | null
  device_info: Record<string, unknown>
  security_info: Record<string, unknown>
  last_inventory_at: string | null
  asset_tag: string
  site: string
  department: string
  building: string
  room: string
  assigned_user: string
  assigned_real_name: string
  assigned_email: string
  notes: string
  created_at: string
  updated_at: string
}

export interface MobileDeviceApp {
  name: string
  version: string
  bundle_id: string
  managed: boolean
  managed_status: string
}

export interface MobileDeviceDetail extends MobileDevice {
  apps: MobileDeviceApp[]
  profiles: ComputerProfile[]
  groups: ComputerGroup[]
  extension_attributes: EAValue[]
}

/** Admin-entered custom field for mobile devices (no agent on iOS). */
export interface MobileExtensionAttribute {
  id: string
  name: string
  description: string
  data_type: 'string' | 'integer' | 'date'
  input_type: 'text' | 'popup'
  popup_choices: string[]
  created_at: string
  updated_at: string
}

export interface MobileGroupPreview {
  count: number
  mobile_devices: MobileDevice[]
}

export type MobileDeviceListParams = ComputerListParams

export interface MobileSearchRunResult {
  search: AdvancedSearch
  count: number
  mobile_devices: MobileDevice[]
}
