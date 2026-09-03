import type {
  ApiEnvelope,
  ApiErrorBody,
  AuthResult,
  BindingCompleteRequest,
  CommandAccepted,
  CommandStatusView,
  CoreTask,
  CoreAssignmentList,
  CorePersonnelList,
  CoreTaskList,
  EdgeTaskList,
  EmployeeTaskCommandRequest,
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

  async listTasks(query: TaskListQuery = {}, signal?: AbortSignal): Promise<ApiEnvelope<CoreTaskList>> {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) if (value !== undefined && value !== "") params.set(key, String(value));
    const suffix = params.toString() ? `?${params.toString()}` : "";
    return this.http.request<ApiEnvelope<CoreTaskList>>(`/api/v1/tasks${suffix}`, { signal });
  }

  getTask(publicID: string, signal?: AbortSignal): Promise<ApiEnvelope<CoreTask>> {
    return this.http.request<ApiEnvelope<CoreTask>>(`/api/v1/tasks/${encodeURIComponent(publicID)}`, { signal });
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

  realtimeTicket(signal?: AbortSignal): Promise<ApiEnvelope<RealtimeTicket>> {
    return this.http.request<ApiEnvelope<RealtimeTicket>>("/api/v1/realtime/ticket", { method: "POST", signal });
  }

  acceptTask(publicID: string, payload: EmployeeTaskCommandRequest): Promise<CommandAccepted> {
    return this.http.request<CommandAccepted>(`/api/v1/tasks/${encodeURIComponent(publicID)}/accept`, { method: "POST", body: JSON.stringify(payload) });
  }

  completeTask(publicID: string, payload: EmployeeTaskCommandRequest): Promise<CommandAccepted> {
    return this.http.request<CommandAccepted>(`/api/v1/tasks/${encodeURIComponent(publicID)}/complete`, { method: "POST", body: JSON.stringify(payload) });
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
