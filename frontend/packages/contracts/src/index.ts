export type Role = "admin" | "manager" | "leader" | "staff";
export type Provider = "personal_wechat" | "wecom";
export type ClientKind = "employee-web" | "employee-miniapp";

export type TaskStatus =
  | "awaiting_confirmation"
  | "assigned"
  | "in_progress"
  | "completed"
  | "cancelled";

export type CandidateStatus = "proposed" | "selected" | "rejected" | "invalidated";
export type AssignmentStatus = "confirmed" | "accepted" | "completed" | "cancelled";
export type CommandStatus = "pending" | "syncing" | "confirmed" | "failed";

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
  confirmation_id: string;
  confirmed_by_public_id: string;
  confirmed_at: string;
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
  confirmation_id: string;
  confirmed_by_public_id: string;
  confirmed_at: string;
  accepted_at?: string;
  completed_at?: string;
  cancelled_at?: string;
  cancel_reason?: string;
  planned_at: string;
}

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

export interface CoreTaskList {
  items: CoreTask[];
  page: number;
  page_size: number;
  total: number;
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
  return value === "awaiting_confirmation" || value === "assigned" || value === "in_progress" || value === "completed" || value === "cancelled";
}

export function normalizeTaskStatus(value: string | undefined): TaskStatus {
  return isTaskStatus(value) ? value : "awaiting_confirmation";
}
