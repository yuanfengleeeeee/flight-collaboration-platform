import type {
  ApiEnvelope,
  AuthResult,
  BindingCompleteRequest,
  CommandAccepted,
  CommandStatusView,
  EdgeTaskList,
  EmployeeTaskCommandRequest,
  PasswordLoginRequest,
  ProviderExchangeRequest,
  SessionView,
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
  acceptTask(taskPublicID: string, payload: EmployeeTaskCommandRequest): Promise<CommandAccepted> { return this.request(`/api/v1/tasks/${encodeURIComponent(taskPublicID)}/accept`, "POST", payload, true); }
  completeTask(taskPublicID: string, payload: EmployeeTaskCommandRequest): Promise<CommandAccepted> { return this.request(`/api/v1/tasks/${encodeURIComponent(taskPublicID)}/complete`, "POST", payload, true); }
  commandStatus(commandID: string): Promise<ApiEnvelope<CommandStatusView>> { return this.request(`/api/v1/commands/${encodeURIComponent(commandID)}`, "GET", undefined, true); }

  private request<T>(path: string, method: "GET" | "POST", data: unknown, authenticated: boolean): Promise<T> {
    return new Promise((resolve, reject) => {
      const header: Record<string, string> = { Accept: "application/json", "Content-Type": "application/json; charset=utf-8" };
      const token = authenticated ? this.getAccessToken() : undefined;
      if (token) header.Authorization = `Bearer ${token}`;
      wx.request({ url: `${this.baseUrl.replace(/\/$/, "")}${path}`, method, data, header, success: (response) => { if (response.statusCode < 200 || response.statusCode >= 300) reject(new MiniappHttpError(response.statusCode, response.data)); else resolve(response.data as T); }, fail: reject });
    });
  }
}

export class MiniappSessionStore implements SessionStore {
  constructor(private readonly key = "flight.edge.employee-miniapp.session") {}
  read(): SessionSnapshot | undefined { return wx.getStorageSync(this.key) as SessionSnapshot | undefined; }
  write(snapshot: SessionSnapshot): void { wx.setStorageSync(this.key, snapshot); }
  clear(): void { wx.removeStorageSync(this.key); }
}
