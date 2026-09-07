export type Role = "admin" | "manager" | "leader" | "supervisor" | "staff";
export type Provider = "personal_wechat" | "wecom";
export type ClientKind = "employee-web" | "employee-miniapp" | "employee-wecom-miniapp";

export type TaskStatus =
  | "pending_dispatch"
  | "awaiting_confirmation"
  | "assigned"
  | "in_progress"
  | "paused"
  | "completed"
  | "cancelled";

export type CandidateStatus = "proposed" | "selected" | "rejected" | "invalidated";
export type AssignmentStatus = "confirmed" | "accepted" | "completed" | "cancelled";
export type AssignmentReceiptStatus = "pending" | "received";
export type CommandStatus = "pending" | "syncing" | "confirmed" | "failed";
export type ExceptionSeverity = "low" | "medium" | "high" | "critical";
export type ExceptionStatus = "open" | "acknowledged" | "resolved" | "rejected";

export interface ApiEnvelope<T> {
  data: T;
  request_id?: string;
  trace_id?: string;
}

export interface ApiErrorBody {
  code: string;
  message: string;
  request_id?: string;
  trace_id?: string;
}

export interface Principal {
  public_id: string;
  type: "human";
  role: Role | string;
}

export interface AuthResult {
  state: "authenticated" | "binding_required";
  binding_ticket?: string;
  access_token?: string;
  refresh_token?: string;
  token_type?: string;
  expires_in?: number;
  session_public_id?: string;
  principal?: Principal;
}

export interface SessionView {
  session_public_id: string;
  expires_at: string;
  principal: Principal;
}

export interface PasswordLoginRequest {
  employee_no: string;
  password: string;
  client: ClientKind;
  provider?: Provider;
  provider_app?: string;
}

export interface ProviderExchangeRequest {
  provider: Provider;
  provider_code: string;
  client: ClientKind;
  redirect_uri?: string;
}

export interface BindingCompleteRequest extends ProviderExchangeRequest {
  binding_ticket: string;
}

export interface CoreTaskCandidate {
  public_id: string;
  personnel_public_id: string;
  rank: number;
  status: CandidateStatus;
  matched_position_code: string;
  matched_capabilities: string[];
  personnel_work_state_snapshot: string;
  personnel_state_changed_at: string;
  rejection_reason?: string;
  selected_at?: string;
  invalidated_at?: string;
}

export interface CoreTaskAssignment {
  public_id: string;
  candidate_public_id: string;
  personnel_public_id: string;
  status: AssignmentStatus;
  status_version: number;
  receipt_status: AssignmentReceiptStatus;
  confirmation_id: string;
  confirmed_by_public_id: string;
  confirmed_at: string;
  received_at?: string;
}

export interface CoreTask {
  public_id: string;
  flight_public_id: string;
  flight_display_no: string;
  template_public_id: string;
  area_public_id: string;
  team_public_id: string;
  trigger_type: string;
  generation_key: string;
  source_event_id: string;
  template_version: number;
  required_position_code: string;
  required_capabilities: string[];
  name: string;
  message: string;
  planned_at: string;
  status: TaskStatus;
  status_version: number;
  sync_version: number;
  candidates: CoreTaskCandidate[];
  assignment?: CoreTaskAssignment;
}

export interface CorePersonnel {
  public_id: string;
  user_public_id: string;
  employee_no: string;
  display_name: string;
  area_public_id: string;
  area_name?: string;
  team_public_id: string;
  team_name?: string;
  position_code: string;
  capability_code: string;
  capabilities: string[];
  work_state: string;
  status_version: number;
  last_state_changed_at: string;
  unavailable_reason?: string;
  enabled: boolean;
}

export interface CoreAssignment {
  public_id: string;
  task_public_id: string;
  flight_public_id: string;
  flight_display_no: string;
  personnel_public_id: string;
  personnel_employee_no: string;
  personnel_name: string;
  area_public_id: string;
  team_public_id: string;
  status: AssignmentStatus;
  status_version: number;
  receipt_status: AssignmentReceiptStatus;
  confirmation_id: string;
  confirmed_by_public_id: string;
  confirmed_at: string;
  received_at?: string;
  accepted_at?: string;
  completed_at?: string;
  cancelled_at?: string;
  cancel_reason?: string;
  planned_at: string;
}

export interface CoreArea {
  public_id: string;
  code: string;
  name: string;
  enabled: boolean;
}

export interface CoreTeam {
  public_id: string;
  area_public_id: string;
  area_name: string;
  code: string;
  name: string;
  enabled: boolean;
}

export interface CoreTemplate {
  public_id: string;
  name: string;
  trigger_type: string;
  template_version: number;
  enabled: boolean;
  area_public_id: string;
  team_public_id: string;
  required_position_code: string;
  required_capability_code: string;
  required_capabilities: string[];
  planned_offset_seconds: number;
  default_message: string;
}

export interface CoreFlight {
  public_id: string;
  flight_display_no: string;
  source_provider: string;
  external_flight_id?: string;
  operating_date: string;
  scheduled_at: string;
  source_last_synced_at?: string;
  source_state: "fresh" | "stale" | "fallback" | "failed" | string;
  source_last_attempt_at?: string;
  source_last_error?: string;
  actual_arrival_at?: string;
  status: "scheduled" | "arrived" | "departed" | "cancelled" | string;
  status_version: number;
  last_status_changed_at: string;
}

export type TaskChangeAction = "pause" | "reassign" | "reschedule" | "cancel" | "resume";
export type TaskChangeRequestStatus = "pending" | "approved" | "rejected" | "applied" | "failed";

export interface CoreTaskChangeRequest {
  public_id: string;
  task_public_id: string;
  exception_public_id?: string;
  action: TaskChangeAction;
  reason: string;
  target_candidate_public_id?: string;
  target_planned_at?: string;
  status: TaskChangeRequestStatus;
  requested_by_public_id: string;
  requested_at: string;
  reviewed_by_public_id?: string;
  reviewed_at?: string;
  review_note?: string;
  applied_at?: string;
  failure_reason?: string;
  request_id: string;
  trace_id: string;
}

export interface CoreTaskChangeRequestList {
  items: CoreTaskChangeRequest[];
  page: number;
  page_size: number;
  total: number;
}

export interface CoreAdminIdentity {
  public_id: string;
  provider: Provider | "oidc" | "development" | string;
  external_subject: string;
  display_name: string;
  role: Exclude<Role, "staff"> | string;
  enabled: boolean;
  global_scope: boolean;
  area_ids: number[];
  team_ids: number[];
  area_public_ids: string[];
  team_public_ids: string[];
  user_id: number;
}

export interface CoreAreaList { items: CoreArea[]; page: number; page_size: number; total: number; }
export interface CoreTeamList { items: CoreTeam[]; page: number; page_size: number; total: number; }
export interface CoreTemplateList { items: CoreTemplate[]; page: number; page_size: number; total: number; }
export interface CoreFlightList { items: CoreFlight[]; page: number; page_size: number; total: number; }
export interface CoreAdminIdentityList { items: CoreAdminIdentity[]; page: number; page_size: number; total: number; }

export interface CorePosition { public_id: string; code: string; name: string; description: string; enabled: boolean; }
export interface CoreCapability { public_id: string; code: string; name: string; description: string; enabled: boolean; }
export interface CorePositionList { items: CorePosition[]; page: number; page_size: number; total: number; }
export interface CoreCapabilityList { items: CoreCapability[]; page: number; page_size: number; total: number; }

export interface CorePersonnelList {
  items: CorePersonnel[];
  page: number;
  page_size: number;
  total: number;
}

export interface CoreAssignmentList {
  items: CoreAssignment[];
  page: number;
  page_size: number;
  total: number;
}

export interface CoreException {
  public_id: string;
  task_public_id: string;
  flight_public_id: string;
  flight_display_no: string;
  assignment_public_id: string;
  personnel_public_id: string;
  personnel_name: string;
  area_public_id: string;
  team_public_id: string;
  category: string;
  severity: ExceptionSeverity | string;
  description: string;
  status: ExceptionStatus | string;
  reported_by_public_id: string;
  reported_at: string;
  resolved_by_public_id?: string;
  resolved_at?: string;
  resolution_note?: string;
}

export interface CoreExceptionList {
  items: CoreException[];
  page: number;
  page_size: number;
  total: number;
}

export interface CoreReportOverview {
  from?: string;
  to?: string;
  task_counts: Record<string, number>;
  flight_counts: Record<string, number>;
  assignment_counts: Record<string, number>;
  personnel_counts: Record<string, number>;
  exception_counts: Record<string, number>;
}

export interface CorePersonnelStatus {
  public_id: string;
  employee_no: string;
  display_name: string;
  area_public_id: string;
  area_name: string;
  team_public_id: string;
  team_name: string;
  position_code: string;
  capability_code: string;
  work_state: string;
  status_version: number;
  last_state_changed_at: string;
  unavailable_reason?: string;
  enabled: boolean;
}
export interface CorePersonnelStatusList { items: CorePersonnelStatus[]; page: number; page_size: number; total: number; }
export interface CorePersonnelStatusHistory {
  public_id: string;
  personnel_public_id: string;
  status_version: number;
  from_state?: string;
  to_state: string;
  reason?: string;
  actor_type: string;
  actor_public_id?: string;
  assignment_id?: number;
  command_id?: string;
  occurred_at: string;
}
export interface CorePersonnelStatusHistoryList { items: CorePersonnelStatusHistory[]; page: number; page_size: number; total: number; }
export interface CoreEvent {
  public_id: string;
  event_type: string;
  status: string;
  aggregate_type: string;
  aggregate_public_id: string;
  flight_public_id?: string;
  flight_display_no?: string;
  source: string;
  occurred_at: string;
  last_error?: string;
}
export interface CoreEventList { items: CoreEvent[]; page: number; page_size: number; total: number; }
export interface CoreAuditEntry {
  id: number;
  actor_type: string;
  actor_id: string;
  action: string;
  resource_type: string;
  resource_id: string;
  result: string;
  request_id: string;
  trace_id: string;
  source_ip: string;
  occurred_at: string;
}
export interface CoreAuditList { items: CoreAuditEntry[]; page: number; page_size: number; total: number; }
export interface CoreScopeView { principal_public_id: string; roles: string[]; global: boolean; area_ids: number[]; team_ids: number[]; }
export interface CoreDiagnostics {
  component: string;
  generated_at: string;
  database_reachable: boolean;
  runtime: { environment: string; redis_enabled: boolean; flight_source_configured: boolean; real_identity_provider: boolean; development_actor_headers: boolean };
  sync: { flight_source_pending: number; flight_source_retry: number; flight_source_failed: number; outbox_pending: number; outbox_failed: number; core_inbox_failed: number };
}

export interface CoreTaskList {
  items: CoreTask[];
  page: number;
  page_size: number;
  total: number;
}

export interface CoreTaskHistoryEvent {
  public_id: string;
  kind: "task" | "assignment" | "personnel" | string;
  status_version: number;
  from_status?: string;
  to_status: string;
  reason?: string;
  actor_type: string;
  actor_public_id?: string;
  command_id?: string;
  source_event_id?: string;
  assignment_public_id?: string;
  personnel_public_id?: string;
  occurred_at: string;
}

export interface CoreTaskHistory {
  task_public_id: string;
  current_status: TaskStatus;
  current_status_version: number;
  events: CoreTaskHistoryEvent[];
}

export interface TaskConfirmationResult {
  result_code: string;
  duplicate: boolean;
  task_public_id: string;
  task_status?: TaskStatus;
  task_version: number;
  candidate_public_id: string;
  assignment_public_id?: string;
  assignment_status?: AssignmentStatus;
  personnel_public_id?: string;
  personnel_work_state?: string;
  sync_version: number;
}

export interface TaskCancellationResult {
  result_code: string;
  duplicate: boolean;
  task_public_id: string;
  task_status?: TaskStatus;
  task_version: number;
  assignment_public_id?: string;
  assignment_status?: AssignmentStatus;
  personnel_public_id?: string;
  personnel_work_state?: string;
  reason?: string;
  sync_version: number;
}

export interface EdgeTaskProjection {
  public_id: string;
  assignment_public_id?: string;
  employee_public_id: string;
  flight_display_no: string;
  task_name: string;
  area_name: string;
  planned_at: string;
  status?: TaskStatus;
  business_status?: TaskStatus;
  receipt_status?: AssignmentReceiptStatus;
  received_at?: string;
  message: string;
  sync_version: number;
  updated_at: string;
}

export interface EdgeTaskList {
  items: EdgeTaskProjection[];
  source: "edge_projection" | string;
  sync_mode: "full_snapshot";
  snapshot_at: string;
  projection_revision: number;
  projection_lag_seconds: number;
  projection_lag_state: "known" | "unknown";
  next_cursor: string | null;
  reset_required: boolean;
  request_id: string;
  trace_id: string;
}

export interface EdgeNotification {
  public_id: string;
  employee_public_id: string;
  title: string;
  message: string;
  status: "unread" | "read" | string;
  sync_version: number;
  updated_at: string;
}

export interface EdgeNotificationList {
  items: EdgeNotification[];
  page: number;
  page_size: number;
  total: number;
}

export interface EdgeHistoryList {
  items: EdgeTaskProjection[];
  page: number;
  page_size: number;
  total: number;
  source: "edge_projection" | string;
}

export interface RealtimeTicket {
  ticket: string;
  protocol: "flight.realtime.v1";
  ticket_protocol: string;
  expires_at: string;
}

export interface TaskChangedNotification {
  type: "task_changed";
  notification_id: string;
  task_public_id: string;
  sync_version: number;
  reason: string;
  issued_at: string;
}

export interface EmployeeTaskCommandRequest {
  command_id: string;
  assignment_public_id: string;
  expected_sync_version: number;
  client_occurred_at?: string;
  note?: string;
}

export interface EmployeeExceptionReportRequest {
  command_id: string;
  assignment_public_id: string;
  expected_sync_version: number;
  category: string;
  severity: ExceptionSeverity;
  description: string;
  client_occurred_at?: string;
  /** Optional requested action; Core still requires manager/admin review before any mutation. */
  change_action?: TaskChangeAction;
  target_candidate_public_id?: string;
  target_planned_at?: string;
}

export interface CommandAccepted {
  command_id: string;
  status: CommandStatus;
  duplicate: boolean;
  request_id?: string;
  trace_id?: string;
}

export interface CommandStatusView {
  command_id: string;
  command_type: string;
  aggregate_id: string;
  status: CommandStatus;
  attempts: number;
  next_attempt_at?: string;
  error_code?: string;
  created_at: string;
  updated_at: string;
}

export interface TaskListQuery {
  status?: TaskStatus;
  flight_public_id?: string;
  page?: number;
  page_size?: number;
}

export function isTaskStatus(value: string | undefined): value is TaskStatus {
  return value === "pending_dispatch" || value === "awaiting_confirmation" || value === "assigned" || value === "in_progress" || value === "paused" || value === "completed" || value === "cancelled";
}

export function normalizeTaskStatus(value: string | undefined): TaskStatus {
  return isTaskStatus(value) ? value : "awaiting_confirmation";
}
