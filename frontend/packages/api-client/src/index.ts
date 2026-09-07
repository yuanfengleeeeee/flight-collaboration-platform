import type {
  ApiEnvelope,
  ApiErrorBody,
  AuthResult,
  BindingCompleteRequest,
  CommandAccepted,
  CommandStatusView,
  CoreTask,
  CoreTaskChangeRequest,
  CoreTaskChangeRequestList,
  CoreTaskHistory,
  CoreAssignmentList,
  CoreAdminIdentity,
  CoreAdminIdentityList,
  CorePositionList,
  CorePosition,
  CoreCapabilityList,
  CoreCapability,
  CoreArea,
  CoreAreaList,
  CoreFlightList,
  CoreExceptionList,
  CoreReportOverview,
  CorePersonnelStatusList,
  CorePersonnelStatusHistoryList,
  CoreEventList,
  CoreAuditList,
  CoreScopeView,
  CoreDiagnostics,
  CorePersonnel,
  CorePersonnelList,
  CoreTemplate,
  CoreTemplateList,
  CoreTeam,
  CoreTeamList,
  CoreTaskList,
  EdgeTaskList,
  EdgeHistoryList,
  EdgeNotificationList,
  EmployeeTaskCommandRequest,
  EmployeeExceptionReportRequest,
  PasswordLoginRequest,
  ProviderExchangeRequest,
  RealtimeTicket,
  SessionView,
  TaskChangedNotification,
  TaskCancellationResult,
  TaskConfirmationResult,
  TaskListQuery,
} from "@flight/contracts";

export interface ApiClientOptions {
  baseUrl: string;
  getAccessToken?: () => string | undefined;
  extraHeaders?: () => Record<string, string>;
  onUnauthorized?: () => void;
  timeoutMs?: number;
}

export class ApiClientError extends Error {
  readonly status: number;
  readonly body: ApiErrorBody;

  constructor(status: number, body: ApiErrorBody) {
    super(body.message || `API request failed with ${status}`);
    this.name = "ApiClientError";
    this.status = status;
    this.body = body;
  }
}

export class ApiClient {
  private readonly baseUrl: string;
  private readonly getAccessToken?: () => string | undefined;
  private readonly extraHeaders?: () => Record<string, string>;
  private readonly onUnauthorized?: () => void;
  private readonly timeoutMs: number;

  constructor(options: ApiClientOptions) {
    this.baseUrl = options.baseUrl.replace(/\/$/, "");
    this.getAccessToken = options.getAccessToken;
    this.extraHeaders = options.extraHeaders;
    this.onUnauthorized = options.onUnauthorized;
    this.timeoutMs = options.timeoutMs ?? 15_000;
  }

  async request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const headers = new Headers(init.headers);
    headers.set("Accept", "application/json");
    if (init.body !== undefined) headers.set("Content-Type", "application/json; charset=utf-8");
    const token = this.getAccessToken?.();
    if (token) headers.set("Authorization", `Bearer ${token}`);
    for (const [key, value] of Object.entries(this.extraHeaders?.() ?? {})) headers.set(key, value);

    const controller = new AbortController();
    const timeout = globalThis.setTimeout(() => controller.abort(), this.timeoutMs);
    const signal = init.signal;
    const abort = () => controller.abort();
    if (signal?.aborted) controller.abort();
    signal?.addEventListener("abort", abort, { once: true });
    try {
      const response = await fetch(`${this.baseUrl}${path}`, { ...init, headers, signal: controller.signal });
      const raw = await response.text();
      const payload: unknown = raw ? JSON.parse(raw) : undefined;
      if (!response.ok) {
        const body = isApiErrorBody(payload) ? payload : { code: "request_failed", message: `请求失败（${response.status}）` };
        if (response.status === 401) this.onUnauthorized?.();
        throw new ApiClientError(response.status, body);
      }
      return payload as T;
    } catch (error) {
      if (error instanceof SyntaxError) throw new ApiClientError(502, { code: "invalid_json", message: "服务端返回了无法识别的数据" });
      throw error;
    } finally {
      globalThis.clearTimeout(timeout);
      signal?.removeEventListener("abort", abort);
    }
  }
}

export class CoreApiClient {
  constructor(private readonly http: ApiClient) {}

  async listAreas(query: { q?: string; include_disabled?: boolean; page?: number; page_size?: number } = {}, signal?: AbortSignal): Promise<ApiEnvelope<CoreAreaList>> {
    const params = new URLSearchParams();
    if (query.q) params.set("q", query.q);
    if (query.include_disabled) params.set("include_disabled", "true");
    if (query.page !== undefined) params.set("page", String(query.page));
    if (query.page_size !== undefined) params.set("page_size", String(query.page_size));
    const suffix = params.toString() ? `?${params.toString()}` : "";
    return this.http.request<ApiEnvelope<CoreAreaList>>(`/api/v1/areas${suffix}`, { signal });
  }

  createArea(payload: { code: string; name: string; enabled?: boolean }): Promise<ApiEnvelope<CoreArea>> {
    return this.http.request<ApiEnvelope<CoreArea>>("/api/v1/areas", { method: "POST", body: JSON.stringify(payload) });
  }

  updateArea(publicID: string, payload: { code?: string; name?: string; enabled?: boolean }): Promise<ApiEnvelope<CoreArea>> {
    return this.http.request<ApiEnvelope<CoreArea>>(`/api/v1/areas/${encodeURIComponent(publicID)}`, { method: "PATCH", body: JSON.stringify(payload) });
  }

  async listTeams(areaPublicID?: string, includeDisabled = false, signal?: AbortSignal, page = 1, pageSize = 20, query = ""): Promise<ApiEnvelope<CoreTeamList>> {
    const params = new URLSearchParams();
    if (areaPublicID) params.set("area_public_id", areaPublicID);
    if (includeDisabled) params.set("include_disabled", "true");
    if (query) params.set("q", query);
    params.set("page", String(page)); params.set("page_size", String(pageSize));
    const suffix = params.toString() ? `?${params.toString()}` : "";
    return this.http.request<ApiEnvelope<CoreTeamList>>(`/api/v1/teams${suffix}`, { signal });
  }

  createTeam(payload: { area_public_id: string; code: string; name: string; enabled?: boolean }): Promise<ApiEnvelope<CoreTeam>> {
    return this.http.request<ApiEnvelope<CoreTeam>>("/api/v1/teams", { method: "POST", body: JSON.stringify(payload) });
  }

  updateTeam(publicID: string, payload: { area_public_id?: string; code?: string; name?: string; enabled?: boolean }): Promise<ApiEnvelope<CoreTeam>> {
    return this.http.request<ApiEnvelope<CoreTeam>>(`/api/v1/teams/${encodeURIComponent(publicID)}`, { method: "PATCH", body: JSON.stringify(payload) });
  }

  createPersonnel(payload: { employee_no: string; display_name: string; team_public_id: string; position_code: string; capability_code?: string; capabilities?: string[]; password: string }): Promise<ApiEnvelope<CorePersonnel>> {
    return this.http.request<ApiEnvelope<CorePersonnel>>("/api/v1/personnel", { method: "POST", body: JSON.stringify(payload) });
  }

  updatePersonnel(publicID: string, payload: { employee_no?: string; display_name?: string; team_public_id?: string; position_code?: string; capability_code?: string; capabilities?: string[]; enabled?: boolean }): Promise<ApiEnvelope<CorePersonnel>> {
    return this.http.request<ApiEnvelope<CorePersonnel>>(`/api/v1/personnel/${encodeURIComponent(publicID)}`, { method: "PATCH", body: JSON.stringify(payload) });
  }

  resetPersonnelPassword(publicID: string, password: string): Promise<void> {
    return this.http.request<void>(`/api/v1/personnel/${encodeURIComponent(publicID)}/password/reset`, { method: "POST", body: JSON.stringify({ password }) });
  }

  async listTemplates(includeDisabled = false, signal?: AbortSignal, page = 1, pageSize = 20): Promise<ApiEnvelope<CoreTemplateList>> {
    const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
    if (includeDisabled) params.set("include_disabled", "true");
    return this.http.request<ApiEnvelope<CoreTemplateList>>(`/api/v1/templates?${params.toString()}`, { signal });
  }

  createTemplate(payload: { name: string; trigger_type?: string; template_version: number; enabled?: boolean; area_public_id: string; team_public_id: string; required_position_code: string; required_capability_code?: string; required_capabilities?: string[]; planned_offset_seconds?: number; default_message: string }): Promise<ApiEnvelope<CoreTemplate>> {
    return this.http.request<ApiEnvelope<CoreTemplate>>("/api/v1/templates", { method: "POST", body: JSON.stringify(payload) });
  }

  updateTemplate(publicID: string, payload: { name?: string; enabled?: boolean; area_public_id?: string; team_public_id?: string; required_position_code?: string; required_capability_code?: string; required_capabilities?: string[]; planned_offset_seconds?: number; default_message?: string }): Promise<ApiEnvelope<CoreTemplate>> {
    return this.http.request<ApiEnvelope<CoreTemplate>>(`/api/v1/templates/${encodeURIComponent(publicID)}`, { method: "PATCH", body: JSON.stringify(payload) });
  }

  async listFlights(query: { operating_date?: string; include_terminal?: boolean; page?: number; page_size?: number } = {}, signal?: AbortSignal): Promise<ApiEnvelope<CoreFlightList>> {
    const params = new URLSearchParams();
    if (query.operating_date) params.set("operating_date", query.operating_date);
    if (query.include_terminal) params.set("include_terminal", "true");
    if (query.page !== undefined) params.set("page", String(query.page));
    if (query.page_size !== undefined) params.set("page_size", String(query.page_size));
    const suffix = params.toString() ? `?${params.toString()}` : "";
    return this.http.request<ApiEnvelope<CoreFlightList>>(`/api/v1/flights${suffix}`, { signal });
  }

  recordFlightArrival(publicID: string, payload: { source_event_id: string; occurred_at: string; actual_arrival_at: string; source: string }): Promise<ApiEnvelope<unknown>> {
    return this.http.request<ApiEnvelope<unknown>>(`/internal/integration/v1/flights/${encodeURIComponent(publicID)}/arrival`, { method: "POST", body: JSON.stringify(payload) });
  }

  async listAdminIdentities(includeDisabled = false, signal?: AbortSignal, page = 1, pageSize = 20): Promise<ApiEnvelope<CoreAdminIdentityList>> {
    const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
    if (includeDisabled) params.set("include_disabled", "true");
    return this.http.request<ApiEnvelope<CoreAdminIdentityList>>(`/api/v1/admin/identities?${params.toString()}`, { signal });
  }

  createAdminIdentity(payload: { provider: string; external_subject: string; display_name: string; role: string; enabled?: boolean; global_scope?: boolean; area_public_ids?: string[]; team_public_ids?: string[]; user_id?: number }): Promise<ApiEnvelope<CoreAdminIdentity>> {
    return this.http.request<ApiEnvelope<CoreAdminIdentity>>("/api/v1/admin/identities", { method: "POST", body: JSON.stringify(payload) });
  }

  updateAdminIdentity(publicID: string, payload: { display_name?: string; role?: string; enabled?: boolean; global_scope?: boolean; area_public_ids?: string[]; team_public_ids?: string[]; user_id?: number }): Promise<ApiEnvelope<CoreAdminIdentity>> {
    return this.http.request<ApiEnvelope<CoreAdminIdentity>>(`/api/v1/admin/identities/${encodeURIComponent(publicID)}`, { method: "PATCH", body: JSON.stringify(payload) });
  }

  async listPositions(query: { include_disabled?: boolean; page?: number; page_size?: number } = {}, signal?: AbortSignal): Promise<ApiEnvelope<CorePositionList>> {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) if (value !== undefined) params.set(key, String(value));
    return this.http.request<ApiEnvelope<CorePositionList>>(`/api/v1/positions?${params.toString()}`, { signal });
  }

  createPosition(payload: { code: string; name: string; description?: string; enabled?: boolean }): Promise<ApiEnvelope<CorePosition>> {
    return this.http.request<ApiEnvelope<CorePosition>>("/api/v1/positions", { method: "POST", body: JSON.stringify(payload) });
  }

  updatePosition(publicID: string, payload: { name?: string; description?: string; enabled?: boolean }): Promise<ApiEnvelope<CorePosition>> {
    return this.http.request<ApiEnvelope<CorePosition>>(`/api/v1/positions/${encodeURIComponent(publicID)}`, { method: "PATCH", body: JSON.stringify(payload) });
  }

  deletePosition(publicID: string): Promise<void> {
    return this.http.request<void>(`/api/v1/positions/${encodeURIComponent(publicID)}`, { method: "DELETE" });
  }

  async listCapabilities(query: { include_disabled?: boolean; page?: number; page_size?: number } = {}, signal?: AbortSignal): Promise<ApiEnvelope<CoreCapabilityList>> {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) if (value !== undefined) params.set(key, String(value));
    return this.http.request<ApiEnvelope<CoreCapabilityList>>(`/api/v1/capabilities?${params.toString()}`, { signal });
  }

  createCapability(payload: { code: string; name: string; description?: string; enabled?: boolean }): Promise<ApiEnvelope<CoreCapability>> {
    return this.http.request<ApiEnvelope<CoreCapability>>("/api/v1/capabilities", { method: "POST", body: JSON.stringify(payload) });
  }

  updateCapability(publicID: string, payload: { name?: string; description?: string; enabled?: boolean }): Promise<ApiEnvelope<CoreCapability>> {
    return this.http.request<ApiEnvelope<CoreCapability>>(`/api/v1/capabilities/${encodeURIComponent(publicID)}`, { method: "PATCH", body: JSON.stringify(payload) });
  }

  deleteCapability(publicID: string): Promise<void> {
    return this.http.request<void>(`/api/v1/capabilities/${encodeURIComponent(publicID)}`, { method: "DELETE" });
  }

  async listTasks(query: TaskListQuery = {}, signal?: AbortSignal): Promise<ApiEnvelope<CoreTaskList>> {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) if (value !== undefined && value !== "") params.set(key, String(value));
    const suffix = params.toString() ? `?${params.toString()}` : "";
    return this.http.request<ApiEnvelope<CoreTaskList>>(`/api/v1/tasks${suffix}`, { signal });
  }

  getTask(publicID: string, signal?: AbortSignal): Promise<ApiEnvelope<CoreTask>> {
    return this.http.request<ApiEnvelope<CoreTask>>(`/api/v1/tasks/${encodeURIComponent(publicID)}`, { signal });
  }

  getTaskHistory(publicID: string, signal?: AbortSignal): Promise<ApiEnvelope<CoreTaskHistory>> {
    return this.http.request<ApiEnvelope<CoreTaskHistory>>(`/api/v1/tasks/${encodeURIComponent(publicID)}/history`, { signal });
  }

  async listPersonnel(query: { work_state?: string; team_public_id?: string; area_public_id?: string; page?: number; page_size?: number } = {}, signal?: AbortSignal): Promise<ApiEnvelope<CorePersonnelList>> {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) if (value !== undefined && value !== "") params.set(key, String(value));
    const suffix = params.toString() ? `?${params.toString()}` : "";
    return this.http.request<ApiEnvelope<CorePersonnelList>>(`/api/v1/personnel${suffix}`, { signal });
  }

  async listAssignments(query: { status?: string; task_public_id?: string; personnel_public_id?: string; page?: number; page_size?: number } = {}, signal?: AbortSignal): Promise<ApiEnvelope<CoreAssignmentList>> {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) if (value !== undefined && value !== "") params.set(key, String(value));
    const suffix = params.toString() ? `?${params.toString()}` : "";
    return this.http.request<ApiEnvelope<CoreAssignmentList>>(`/api/v1/assignments${suffix}`, { signal });
  }

  async listExceptions(query: { status?: string; severity?: string; task_public_id?: string; personnel_public_id?: string; page?: number; page_size?: number } = {}, signal?: AbortSignal): Promise<ApiEnvelope<CoreExceptionList>> {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) if (value !== undefined && value !== "") params.set(key, String(value));
    const suffix = params.toString() ? `?${params.toString()}` : "";
    return this.http.request<ApiEnvelope<CoreExceptionList>>(`/api/v1/exceptions${suffix}`, { signal });
  }

  async listTaskChangeRequests(query: { status?: string; task_public_id?: string; page?: number; page_size?: number } = {}, signal?: AbortSignal): Promise<ApiEnvelope<CoreTaskChangeRequestList>> {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) if (value !== undefined && value !== "") params.set(key, String(value));
    const suffix = params.toString() ? `?${params.toString()}` : "";
    return this.http.request<ApiEnvelope<CoreTaskChangeRequestList>>(`/api/v1/task-change-requests${suffix}`, { signal });
  }

  reviewTaskChangeRequest(publicID: string, payload: { decision: "approve" | "reject"; review_note?: string }): Promise<ApiEnvelope<CoreTaskChangeRequest>> {
    return this.http.request<ApiEnvelope<CoreTaskChangeRequest>>(`/api/v1/task-change-requests/${encodeURIComponent(publicID)}/review`, { method: "PATCH", body: JSON.stringify(payload) });
  }

  updateException(publicID: string, payload: { status: "acknowledged" | "resolved" | "rejected"; resolution_note?: string }): Promise<ApiEnvelope<{ public_id: string; status: string }>> {
    return this.http.request<ApiEnvelope<{ public_id: string; status: string }>>(`/api/v1/exceptions/${encodeURIComponent(publicID)}`, { method: "PATCH", body: JSON.stringify(payload) });
  }

  async getReportOverview(query: { from?: string; to?: string } = {}, signal?: AbortSignal): Promise<ApiEnvelope<CoreReportOverview>> {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) if (value !== undefined && value !== "") params.set(key, value);
    const suffix = params.toString() ? `?${params.toString()}` : "";
    return this.http.request<ApiEnvelope<CoreReportOverview>>(`/api/v1/reports/overview${suffix}`, { signal });
  }

  async listPersonnelStatus(query: { work_state?: string; team_public_id?: string; area_public_id?: string; page?: number; page_size?: number } = {}, signal?: AbortSignal): Promise<ApiEnvelope<CorePersonnelStatusList>> {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) if (value !== undefined && value !== "") params.set(key, String(value));
    return this.http.request<ApiEnvelope<CorePersonnelStatusList>>(`/api/v1/personnel/status?${params.toString()}`, { signal });
  }

  async listPersonnelStatusHistory(publicID: string, query: { page?: number; page_size?: number } = {}, signal?: AbortSignal): Promise<ApiEnvelope<CorePersonnelStatusHistoryList>> {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) if (value !== undefined) params.set(key, String(value));
    return this.http.request<ApiEnvelope<CorePersonnelStatusHistoryList>>(`/api/v1/personnel/${encodeURIComponent(publicID)}/status-history?${params.toString()}`, { signal });
  }

  async listEvents(query: { event_type?: string; status?: string; flight_public_id?: string; from?: string; to?: string; page?: number; page_size?: number } = {}, signal?: AbortSignal): Promise<ApiEnvelope<CoreEventList>> {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) if (value !== undefined && value !== "") params.set(key, String(value));
    return this.http.request<ApiEnvelope<CoreEventList>>(`/api/v1/events?${params.toString()}`, { signal });
  }

  async listAudit(query: { actor_id?: string; action?: string; resource_type?: string; from?: string; to?: string; page?: number; page_size?: number } = {}, signal?: AbortSignal): Promise<ApiEnvelope<CoreAuditList>> {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) if (value !== undefined && value !== "") params.set(key, String(value));
    return this.http.request<ApiEnvelope<CoreAuditList>>(`/api/v1/audit?${params.toString()}`, { signal });
  }

  getScopes(signal?: AbortSignal): Promise<ApiEnvelope<CoreScopeView>> {
    return this.http.request<ApiEnvelope<CoreScopeView>>("/api/v1/scopes", { signal });
  }

  getDiagnostics(signal?: AbortSignal): Promise<ApiEnvelope<CoreDiagnostics>> {
    return this.http.request<ApiEnvelope<CoreDiagnostics>>("/api/v1/diagnostics", { signal });
  }

  confirmTask(publicID: string, payload: { candidate_public_id: string; confirmation_id: string; expected_task_version: number }): Promise<ApiEnvelope<TaskConfirmationResult>> {
    return this.http.request<ApiEnvelope<TaskConfirmationResult>>(`/api/v1/tasks/${encodeURIComponent(publicID)}/confirm`, { method: "POST", body: JSON.stringify(payload) });
  }

  cancelTask(publicID: string, payload: { cancellation_id: string; expected_task_version: number; reason: string }): Promise<ApiEnvelope<TaskCancellationResult>> {
    return this.http.request<ApiEnvelope<TaskCancellationResult>>(`/api/v1/tasks/${encodeURIComponent(publicID)}/cancel`, { method: "POST", body: JSON.stringify(payload) });
  }
}

export class EdgeApiClient {
  constructor(private readonly http: ApiClient) {}

  async passwordLogin(payload: PasswordLoginRequest): Promise<ApiEnvelope<AuthResult>> {
    return this.http.request<ApiEnvelope<AuthResult>>("/api/v1/auth/password/login", { method: "POST", body: JSON.stringify(payload) });
  }

  exchange(payload: ProviderExchangeRequest): Promise<ApiEnvelope<AuthResult>> {
    return this.http.request<ApiEnvelope<AuthResult>>("/api/v1/auth/exchange", { method: "POST", body: JSON.stringify(payload) });
  }

  completeBinding(payload: BindingCompleteRequest): Promise<ApiEnvelope<AuthResult>> {
    return this.http.request<ApiEnvelope<AuthResult>>("/api/v1/auth/bindings/complete", { method: "POST", body: JSON.stringify(payload) });
  }

  refresh(refreshToken: string): Promise<ApiEnvelope<AuthResult>> {
    return this.http.request<ApiEnvelope<AuthResult>>("/api/v1/auth/refresh", { method: "POST", body: JSON.stringify({ refresh_token: refreshToken }) });
  }

  me(signal?: AbortSignal): Promise<ApiEnvelope<SessionView>> {
    return this.http.request<ApiEnvelope<SessionView>>("/api/v1/auth/me", { signal });
  }

  async logout(): Promise<void> {
    await this.http.request<void>("/api/v1/auth/logout", { method: "POST" });
  }

  listTasks(signal?: AbortSignal): Promise<EdgeTaskList> {
    return this.http.request<EdgeTaskList>("/api/v1/tasks", { signal });
  }

  async listHistory(query: { status?: "completed" | "cancelled" | "all"; page?: number; page_size?: number } = {}, signal?: AbortSignal): Promise<ApiEnvelope<EdgeHistoryList>> {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) if (value !== undefined) params.set(key, String(value));
    const suffix = params.toString() ? `?${params.toString()}` : "";
    return this.http.request<ApiEnvelope<EdgeHistoryList>>(`/api/v1/history${suffix}`, { signal });
  }

  async listNotifications(query: { status?: "unread" | "read"; page?: number; page_size?: number } = {}, signal?: AbortSignal): Promise<ApiEnvelope<EdgeNotificationList>> {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) if (value !== undefined) params.set(key, String(value));
    const suffix = params.toString() ? `?${params.toString()}` : "";
    return this.http.request<ApiEnvelope<EdgeNotificationList>>(`/api/v1/notifications${suffix}`, { signal });
  }

  markNotificationRead(publicID: string): Promise<ApiEnvelope<{ public_id: string; status: "read" }>> {
    return this.http.request<ApiEnvelope<{ public_id: string; status: "read" }>>(`/api/v1/notifications/${encodeURIComponent(publicID)}/read`, { method: "POST" });
  }

  realtimeTicket(signal?: AbortSignal): Promise<ApiEnvelope<RealtimeTicket>> {
    return this.http.request<ApiEnvelope<RealtimeTicket>>("/api/v1/realtime/ticket", { method: "POST", signal });
  }

  receiveTask(publicID: string, payload: EmployeeTaskCommandRequest): Promise<CommandAccepted> {
    return this.http.request<CommandAccepted>(`/api/v1/tasks/${encodeURIComponent(publicID)}/received`, { method: "POST", body: JSON.stringify(payload) });
  }

  startTask(publicID: string, payload: EmployeeTaskCommandRequest): Promise<CommandAccepted> {
    return this.http.request<CommandAccepted>(`/api/v1/tasks/${encodeURIComponent(publicID)}/start`, { method: "POST", body: JSON.stringify(payload) });
  }

  /** @deprecated Use receiveTask for acknowledgement or startTask for execution. */
  acceptTask(publicID: string, payload: EmployeeTaskCommandRequest): Promise<CommandAccepted> {
    return this.http.request<CommandAccepted>(`/api/v1/tasks/${encodeURIComponent(publicID)}/accept`, { method: "POST", body: JSON.stringify(payload) });
  }

  completeTask(publicID: string, payload: EmployeeTaskCommandRequest): Promise<CommandAccepted> {
    return this.http.request<CommandAccepted>(`/api/v1/tasks/${encodeURIComponent(publicID)}/complete`, { method: "POST", body: JSON.stringify(payload) });
  }

  reportException(publicID: string, payload: EmployeeExceptionReportRequest): Promise<CommandAccepted> {
    return this.http.request<CommandAccepted>(`/api/v1/tasks/${encodeURIComponent(publicID)}/exceptions`, { method: "POST", body: JSON.stringify(payload) });
  }

  commandStatus(commandID: string, signal?: AbortSignal): Promise<ApiEnvelope<CommandStatusView>> {
    return this.http.request<ApiEnvelope<CommandStatusView>>(`/api/v1/commands/${encodeURIComponent(commandID)}`, { signal });
  }
}

export type RealtimeState = "idle" | "connecting" | "open" | "retrying" | "closed" | "unauthorized";

export interface RealtimeSocket {
  onopen: ((event: unknown) => void) | null;
  onmessage: ((event: { data: unknown }) => void) | null;
  onerror: ((event: unknown) => void) | null;
  onclose: ((event: unknown) => void) | null;
  send(data: string): void;
  close(): void;
}

export interface RealtimeClientOptions {
  webSocketUrl: string;
  getTicket: (signal?: AbortSignal) => Promise<ApiEnvelope<RealtimeTicket>>;
  onTaskChanged: (notification: TaskChangedNotification) => void;
  onConnected?: () => void;
  onStateChange?: (state: RealtimeState) => void;
  onUnauthorized?: () => void;
  reconnectBaseMs?: number;
  reconnectMaxMs?: number;
  webSocketFactory?: (url: string, protocols: string[]) => RealtimeSocket;
  random?: () => number;
}

export class RealtimeClient {
  private readonly webSocketUrl: string;
  private readonly getTicket: RealtimeClientOptions["getTicket"];
  private readonly onTaskChanged: RealtimeClientOptions["onTaskChanged"];
  private readonly onConnected?: RealtimeClientOptions["onConnected"];
  private readonly onStateChange?: RealtimeClientOptions["onStateChange"];
  private readonly onUnauthorized?: RealtimeClientOptions["onUnauthorized"];
  private readonly reconnectBaseMs: number;
  private readonly reconnectMaxMs: number;
  private readonly webSocketFactory: NonNullable<RealtimeClientOptions["webSocketFactory"]>;
  private readonly random: () => number;
  private readonly seenNotificationIDs = new Set<string>();
  private socket?: RealtimeSocket;
  private reconnectTimer?: ReturnType<typeof globalThis.setTimeout>;
  private generation = 0;
  private reconnectAttempt = 0;
  private running = false;
  private state: RealtimeState = "idle";

  constructor(options: RealtimeClientOptions) {
    this.webSocketUrl = options.webSocketUrl;
    this.getTicket = options.getTicket;
    this.onTaskChanged = options.onTaskChanged;
    this.onConnected = options.onConnected;
    this.onStateChange = options.onStateChange;
    this.onUnauthorized = options.onUnauthorized;
    this.reconnectBaseMs = Math.max(100, options.reconnectBaseMs ?? 1_000);
    this.reconnectMaxMs = Math.max(this.reconnectBaseMs, options.reconnectMaxMs ?? 30_000);
    this.webSocketFactory = options.webSocketFactory ?? ((url, protocols) => new WebSocket(url, protocols) as unknown as RealtimeSocket);
    this.random = options.random ?? Math.random;
  }

  start(): void {
    if (this.running) return;
    this.running = true;
    this.reconnectAttempt = 0;
    this.connect();
  }

  stop(): void {
    if (!this.running && !this.socket) return;
    this.running = false;
    this.generation += 1;
    if (this.reconnectTimer !== undefined) {
      globalThis.clearTimeout(this.reconnectTimer);
      this.reconnectTimer = undefined;
    }
    const socket = this.socket;
    this.socket = undefined;
    socket?.close();
    this.setState("closed");
  }

  getState(): RealtimeState { return this.state; }

  private async connect(): Promise<void> {
    if (!this.running) return;
    const generation = ++this.generation;
    this.setState("connecting");
    let ticket: RealtimeTicket;
    try {
      ticket = (await this.getTicket()).data;
    } catch (error) {
      if (!this.running || generation !== this.generation) return;
      if (error instanceof ApiClientError && error.status === 401) {
        this.setState("unauthorized");
        this.onUnauthorized?.();
        this.running = false;
        return;
      }
      this.scheduleReconnect();
      return;
    }
    if (!this.running || generation !== this.generation) return;

    let socket: RealtimeSocket;
    try {
      socket = this.webSocketFactory(this.webSocketUrl, [ticket.protocol, ticket.ticket_protocol]);
    } catch {
      this.scheduleReconnect();
      return;
    }
    this.socket = socket;
    socket.onopen = () => {
      if (!this.running || generation !== this.generation || this.socket !== socket) {
        socket.close();
        return;
      }
      this.reconnectAttempt = 0;
      this.setState("open");
      this.onConnected?.();
    };
    socket.onmessage = (event) => {
      if (this.socket !== socket || !this.running) return;
      this.handleMessage(socket, event.data);
    };
    socket.onerror = () => {
      if (this.socket === socket) socket.close();
    };
    socket.onclose = () => {
      if (this.socket !== socket) return;
      this.socket = undefined;
      if (this.running) this.scheduleReconnect();
    };
  }

  private scheduleReconnect(): void {
    if (!this.running || this.reconnectTimer !== undefined) return;
    this.reconnectAttempt += 1;
    const exponential = Math.min(this.reconnectMaxMs, this.reconnectBaseMs * (2 ** Math.min(this.reconnectAttempt - 1, 12)));
    const jittered = Math.max(0, Math.round(exponential * (0.5 + this.random() * 0.5)));
    this.setState("retrying");
    this.reconnectTimer = globalThis.setTimeout(() => {
      this.reconnectTimer = undefined;
      void this.connect();
    }, jittered);
  }

  private handleMessage(socket: RealtimeSocket, raw: unknown): void {
    if (typeof raw !== "string") return;
    let message: unknown;
    try {
      message = JSON.parse(raw);
    } catch {
      return;
    }
    if (!isRecord(message) || typeof message.type !== "string") return;
    if (message.type === "ping") {
      try {
        socket.send(JSON.stringify({ type: "pong" }));
      } catch {
        socket.close();
      }
      return;
    }
    if (message.type !== "task_changed" || !isTaskChangedNotification(message)) return;
    if (this.seenNotificationIDs.has(message.notification_id)) return;
    this.seenNotificationIDs.add(message.notification_id);
    if (this.seenNotificationIDs.size > 256) {
      const oldest = this.seenNotificationIDs.values().next().value;
      if (oldest) this.seenNotificationIDs.delete(oldest);
    }
    this.onTaskChanged(message);
  }

  private setState(state: RealtimeState): void {
    this.state = state;
    this.onStateChange?.(state);
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isTaskChangedNotification(value: unknown): value is TaskChangedNotification {
  if (!isRecord(value)) return false;
  return value.type === "task_changed" && typeof value.notification_id === "string" && value.notification_id !== "" && typeof value.task_public_id === "string" && value.task_public_id !== "" && typeof value.sync_version === "number" && Number.isInteger(value.sync_version) && value.sync_version > 0 && typeof value.reason === "string" && value.reason !== "" && typeof value.issued_at === "string" && value.issued_at !== "";
}

function isApiErrorBody(value: unknown): value is ApiErrorBody {
  return typeof value === "object" && value !== null && typeof (value as { code?: unknown }).code === "string" && typeof (value as { message?: unknown }).message === "string";
}
