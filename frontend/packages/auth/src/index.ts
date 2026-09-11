import type {
  AuthResult,
  BindingCompleteRequest,
  PasswordLoginRequest,
  Principal,
  ProviderExchangeRequest,
  SessionView,
} from "@flight/contracts";

export interface SessionSnapshot {
  accessToken: string;
  refreshToken: string;
  principal?: Principal;
  sessionPublicID?: string;
  expiresAt?: string;
}

export interface SessionStore {
  read(): SessionSnapshot | undefined;
  write(snapshot: SessionSnapshot): void;
  clear(): void;
}

export interface SessionApi {
  passwordLogin(payload: PasswordLoginRequest): Promise<{ data: AuthResult }>;
  exchange(payload: ProviderExchangeRequest): Promise<{ data: AuthResult }>;
  completeBinding(payload: BindingCompleteRequest): Promise<{ data: AuthResult }>;
  refresh(refreshToken: string): Promise<{ data: AuthResult }>;
  me(signal?: AbortSignal): Promise<{ data: SessionView }>;
  logout(): Promise<void>;
}

export class BrowserSessionStore implements SessionStore {
  constructor(private readonly key: string) {}

  read(): SessionSnapshot | undefined {
    try {
      const value = window.localStorage.getItem(this.key);
      return value ? (JSON.parse(value) as SessionSnapshot) : undefined;
    } catch {
      return undefined;
    }
  }

  write(snapshot: SessionSnapshot): void {
    window.localStorage.setItem(this.key, JSON.stringify(snapshot));
  }

  clear(): void {
    window.localStorage.removeItem(this.key);
  }
}

export class MemorySessionStore implements SessionStore {
  private value?: SessionSnapshot;

  read(): SessionSnapshot | undefined { return this.value; }
  write(snapshot: SessionSnapshot): void { this.value = snapshot; }
  clear(): void { this.value = undefined; }
}

export class SessionManager {
  private snapshot?: SessionSnapshot;
  private pendingBinding?: { ticket: string; principal?: Principal };
  private readonly listeners = new Set<() => void>();

  constructor(private readonly api: SessionApi, private readonly store: SessionStore) {
    this.snapshot = store.read();
  }

  getSnapshot(): SessionSnapshot | undefined { return this.snapshot; }
  getAccessToken(): string | undefined { return this.snapshot?.accessToken; }
  getPendingBinding(): { ticket: string; principal?: Principal } | undefined { return this.pendingBinding; }

  subscribe(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  async restore(): Promise<SessionSnapshot | undefined> {
    if (!this.snapshot) return undefined;
    try {
      const result = await this.api.me();
      this.snapshot = this.mergeSessionView(result.data);
      this.persist();
      return this.snapshot;
    } catch {
      if (!this.snapshot.refreshToken) {
        this.clear();
        return undefined;
      }
      try {
        return await this.refresh();
      } catch {
        this.clear();
        return undefined;
      }
    }
  }

  async passwordLogin(payload: PasswordLoginRequest): Promise<AuthResult> {
    const result = (await this.api.passwordLogin(payload)).data;
    return this.consumeAuthResult(result);
  }

  async exchange(payload: ProviderExchangeRequest): Promise<AuthResult> {
    const result = (await this.api.exchange(payload)).data;
    return this.consumeAuthResult(result);
  }

  async completeBinding(payload: BindingCompleteRequest): Promise<AuthResult> {
    const result = (await this.api.completeBinding(payload)).data;
    return this.consumeAuthResult(result);
  }

  async refresh(): Promise<SessionSnapshot> {
    if (!this.snapshot?.refreshToken) throw new Error("refresh token is unavailable");
    const result = (await this.api.refresh(this.snapshot.refreshToken)).data;
    this.consumeAuthResult(result);
    if (!this.snapshot) throw new Error("refresh did not issue a session");
    return this.snapshot;
  }

  async logout(): Promise<void> {
    try {
      if (this.snapshot) await this.api.logout();
    } finally {
      this.clear();
    }
  }

  clear(): void {
    this.snapshot = undefined;
    this.pendingBinding = undefined;
    this.store.clear();
    this.emit();
  }

  private consumeAuthResult(result: AuthResult): AuthResult {
    if (result.state === "binding_required") {
      if (result.binding_ticket) this.pendingBinding = { ticket: result.binding_ticket, principal: result.principal };
      this.emit();
      return result;
    }
    if (!result.access_token || !result.refresh_token) throw new Error("authenticated response has no session tokens");
    this.snapshot = {
      accessToken: result.access_token,
      refreshToken: result.refresh_token,
      principal: result.principal,
      sessionPublicID: result.session_public_id,
      expiresAt: result.expires_in ? new Date(Date.now() + result.expires_in * 1000).toISOString() : undefined,
    };
    this.pendingBinding = undefined;
    this.persist();
    return result;
  }

  private mergeSessionView(view: SessionView): SessionSnapshot {
    return { ...this.snapshot!, principal: view.principal, sessionPublicID: view.session_public_id, expiresAt: view.expires_at };
  }

  private persist(): void {
    if (this.snapshot) this.store.write(this.snapshot);
    this.emit();
  }

  private emit(): void { for (const listener of this.listeners) listener(); }
}
