import type {
  ApiEnvelope,
  AuthResult,
  BindingCompleteRequest,
  CommandAccepted,
  CommandStatusView,
  EdgeHistoryList,
  EdgeNotificationList,
  EdgeTaskList,
  EmployeeExceptionReportRequest,
  EmployeeTaskCommandRequest,
  RealtimeTicket,
  PasswordLoginRequest,
  ProviderExchangeRequest,
  SessionView,
  TaskChangedNotification,
} from "@flight/contracts";
import type { SessionApi, SessionSnapshot, SessionStore } from "@flight/auth";

export class MiniappHttpError extends Error {
  constructor(readonly status: number, readonly body: unknown) { super("miniapp request failed"); }
}

export class MiniappEdgeGateway implements SessionApi {
  constructor(private readonly baseUrl: string, private readonly getAccessToken: () => string | undefined) {}

  passwordLogin(payload: PasswordLoginRequest): Promise<ApiEnvelope<AuthResult>> { return this.request("/api/v1/auth/password/login", "POST", payload, false); }
  exchange(payload: ProviderExchangeRequest): Promise<ApiEnvelope<AuthResult>> { return this.request("/api/v1/auth/exchange", "POST", payload, false); }
  completeBinding(payload: BindingCompleteRequest): Promise<ApiEnvelope<AuthResult>> { return this.request("/api/v1/auth/bindings/complete", "POST", payload, false); }
  refresh(refreshToken: string): Promise<ApiEnvelope<AuthResult>> { return this.request("/api/v1/auth/refresh", "POST", { refresh_token: refreshToken }, false); }
  me(): Promise<ApiEnvelope<SessionView>> { return this.request("/api/v1/auth/me", "GET", undefined, true); }
  logout(): Promise<void> { return this.request("/api/v1/auth/logout", "POST", undefined, true).then(() => undefined); }
  listTasks(): Promise<EdgeTaskList> { return this.request("/api/v1/tasks", "GET", undefined, true); }
  listHistory(status?: "completed" | "cancelled" | "all", page = 1, pageSize = 20): Promise<ApiEnvelope<EdgeHistoryList>> { const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) }); if (status) params.set("status", status); return this.request(`/api/v1/history?${params.toString()}`, "GET", undefined, true); }
  listNotifications(status?: "unread" | "read", page = 1, pageSize = 20): Promise<ApiEnvelope<EdgeNotificationList>> { const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) }); if (status) params.set("status", status); return this.request(`/api/v1/notifications?${params.toString()}`, "GET", undefined, true); }
  markNotificationRead(publicID: string): Promise<ApiEnvelope<{ public_id: string; status: "read" }>> { return this.request(`/api/v1/notifications/${encodeURIComponent(publicID)}/read`, "POST", undefined, true); }
  receiveTask(taskPublicID: string, payload: EmployeeTaskCommandRequest): Promise<CommandAccepted> { return this.request(`/api/v1/tasks/${encodeURIComponent(taskPublicID)}/received`, "POST", payload, true); }
  startTask(taskPublicID: string, payload: EmployeeTaskCommandRequest): Promise<CommandAccepted> { return this.request(`/api/v1/tasks/${encodeURIComponent(taskPublicID)}/start`, "POST", payload, true); }
  acceptTask(taskPublicID: string, payload: EmployeeTaskCommandRequest): Promise<CommandAccepted> { return this.startTask(taskPublicID, payload); }
  completeTask(taskPublicID: string, payload: EmployeeTaskCommandRequest): Promise<CommandAccepted> { return this.request(`/api/v1/tasks/${encodeURIComponent(taskPublicID)}/complete`, "POST", payload, true); }
  reportException(taskPublicID: string, payload: EmployeeExceptionReportRequest): Promise<CommandAccepted> { return this.request(`/api/v1/tasks/${encodeURIComponent(taskPublicID)}/exceptions`, "POST", payload, true); }
  commandStatus(commandID: string): Promise<ApiEnvelope<CommandStatusView>> { return this.request(`/api/v1/commands/${encodeURIComponent(commandID)}`, "GET", undefined, true); }
  realtimeTicket(): Promise<ApiEnvelope<RealtimeTicket>> { return this.request("/api/v1/realtime/ticket", "POST", undefined, true); }

  private request<T>(path: string, method: "GET" | "POST", data: unknown, authenticated: boolean): Promise<T> {
    return new Promise((resolve, reject) => {
      const header: Record<string, string> = { Accept: "application/json", "Content-Type": "application/json; charset=utf-8" };
      const token = authenticated ? this.getAccessToken() : undefined;
      if (token) header.Authorization = `Bearer ${token}`;
      wx.request({ url: `${this.baseUrl.replace(/\/$/, "")}${path}`, method, data, header, success: (response) => { if (response.statusCode < 200 || response.statusCode >= 300) reject(new MiniappHttpError(response.statusCode, response.data)); else resolve(response.data as T); }, fail: reject });
    });
  }
}

export type MiniappRealtimeState = "idle" | "connecting" | "open" | "retrying" | "closed" | "unauthorized";

// The socket only carries a refresh hint. Every page re-reads the complete
// Edge snapshot, so a dropped native message cannot become a business fact.
export class MiniappRealtimeClient {
  private socket?: MiniappSocketTask;
  private timer?: ReturnType<typeof setTimeout>;
  private generation = 0;
  private attempt = 0;
  private running = false;
  private state: MiniappRealtimeState = "idle";
  private onTaskChanged: () => void = () => undefined;
  private onStateChange?: (state: MiniappRealtimeState) => void;
  private onUnauthorized?: () => void;

  constructor(private readonly gateway: MiniappEdgeGateway, private readonly baseUrl: string) {}

  start(onTaskChanged: () => void, onStateChange?: (state: MiniappRealtimeState) => void, onUnauthorized?: () => void): void {
    if (this.running) return;
    this.onTaskChanged = onTaskChanged;
    this.onStateChange = onStateChange;
    this.onUnauthorized = onUnauthorized;
    this.running = true;
    this.attempt = 0;
    void this.connect();
  }

  stop(): void {
    if (!this.running && !this.socket) return;
    this.running = false;
    this.generation += 1;
    if (this.timer !== undefined) clearTimeout(this.timer);
    this.timer = undefined;
    this.socket?.close({ code: 1000, reason: "page hidden" });
    this.socket = undefined;
    this.setState("closed");
  }

  getState(): MiniappRealtimeState { return this.state; }

  private async connect(): Promise<void> {
    if (!this.running) return;
    const generation = ++this.generation;
    this.setState("connecting");
    let ticket: RealtimeTicket;
    try {
      ticket = (await this.gateway.realtimeTicket()).data;
    } catch (error) {
      if (error instanceof MiniappHttpError && error.status === 401) {
        this.running = false;
        this.setState("unauthorized");
        this.onUnauthorized?.();
        return;
      }
      this.scheduleReconnect();
      return;
    }
    if (!this.running || generation !== this.generation) return;
    const socket = wx.connectSocket({ url: this.toWebSocketURL(), protocols: [ticket.protocol, ticket.ticket_protocol] });
    this.socket = socket;
    socket.onOpen(() => {
      if (!this.running || generation !== this.generation || this.socket !== socket) { socket.close(); return; }
      this.attempt = 0;
      this.setState("open");
    });
    socket.onMessage((event) => {
      if (!this.running || this.socket !== socket || typeof event.data !== "string") return;
      this.handleMessage(socket, event.data);
    });
    socket.onError(() => { if (this.socket === socket) socket.close(); });
    socket.onClose(() => {
      if (this.socket !== socket) return;
      this.socket = undefined;
      if (this.running) this.scheduleReconnect();
    });
  }

  private scheduleReconnect(): void {
    if (!this.running || this.timer !== undefined) return;
    this.attempt += 1;
    const delay = Math.min(30_000, 1_000 * (2 ** Math.min(this.attempt - 1, 5)));
    this.setState("retrying");
    this.timer = setTimeout(() => { this.timer = undefined; void this.connect(); }, delay);
  }

  private handleMessage(socket: MiniappSocketTask, raw: string): void {
    let message: unknown;
    try { message = JSON.parse(raw); } catch { return; }
    if (!message || typeof message !== "object") return;
    const value = message as Partial<TaskChangedNotification> & { type?: string };
    const messageType = (message as { type?: string }).type;
    if (messageType === "ping") { socket.send({ data: JSON.stringify({ type: "pong" }) }); return; }
    if (value.type === "task_changed" && typeof value.notification_id === "string" && typeof value.task_public_id === "string") this.onTaskChanged();
  }

  private toWebSocketURL(): string { return `${this.baseUrl.replace(/^http:/, "ws:").replace(/^https:/, "wss:").replace(/\/$/, "")}/api/v1/ws/native`; }
  private setState(state: MiniappRealtimeState): void { this.state = state; this.onStateChange?.(state); }
}

export class MiniappSessionStore implements SessionStore {
  constructor(private readonly key = "flight.edge.employee-miniapp.session") {}
  read(): SessionSnapshot | undefined { return wx.getStorageSync(this.key) as SessionSnapshot | undefined; }
  write(snapshot: SessionSnapshot): void { wx.setStorageSync(this.key, snapshot); }
  clear(): void { wx.removeStorageSync(this.key); }
}
