import { describe, expect, it, vi } from "vitest";
import type { AuthResult } from "@flight/contracts";
import { MemorySessionStore, SessionManager, type SessionApi, type SessionSnapshot } from "./index";

const principal = { public_id: "staff-1", type: "human" as const, role: "staff" };

function sessionApi(overrides: Partial<SessionApi> = {}): SessionApi {
  return {
    passwordLogin: vi.fn(),
    exchange: vi.fn(),
    completeBinding: vi.fn(),
    refresh: vi.fn(),
    me: vi.fn(),
    logout: vi.fn(),
    ...overrides,
  };
}

function authenticated(accessToken: string, refreshToken: string): { data: AuthResult } {
  return { data: { state: "authenticated", access_token: accessToken, refresh_token: refreshToken, principal, session_public_id: "session-2", expires_in: 900 } };
}

describe("SessionManager", () => {
  it("refreshes an expired access token during session restore", async () => {
    const stored: SessionSnapshot = { accessToken: "expired", refreshToken: "refresh-1", principal, sessionPublicID: "session-1" };
    const store = new MemorySessionStore();
    store.write(stored);
    const me = vi.fn<SessionApi["me"]>().mockRejectedValue(new Error("access token expired"));
    const refresh = vi.fn<SessionApi["refresh"]>().mockResolvedValue(authenticated("access-2", "refresh-2"));
    const manager = new SessionManager(sessionApi({ me, refresh }), store);

    const snapshot = await manager.restore();

    expect(refresh).toHaveBeenCalledWith("refresh-1");
    expect(snapshot?.accessToken).toBe("access-2");
    expect(manager.getSnapshot()?.refreshToken).toBe("refresh-2");
    expect(store.read()?.sessionPublicID).toBe("session-2");
  });

  it("clears a session when both access and refresh recovery fail", async () => {
    const store = new MemorySessionStore();
    store.write({ accessToken: "expired", refreshToken: "refresh-1", principal });
    const manager = new SessionManager(sessionApi({
      me: vi.fn<SessionApi["me"]>().mockRejectedValue(new Error("expired")),
      refresh: vi.fn<SessionApi["refresh"]>().mockRejectedValue(new Error("refresh revoked")),
    }), store);

    await expect(manager.restore()).resolves.toBeUndefined();

    expect(manager.getSnapshot()).toBeUndefined();
    expect(store.read()).toBeUndefined();
  });

  it("keeps a pending binding ticket separate from an authenticated session", async () => {
    const binding: AuthResult = { state: "binding_required", binding_ticket: "binding-1", principal };
    const manager = new SessionManager(sessionApi({ passwordLogin: vi.fn().mockResolvedValue({ data: binding }) }), new MemorySessionStore());

    await manager.passwordLogin({ employee_no: "E-001", password: "secret", client: "employee-web" });

    expect(manager.getPendingBinding()).toEqual({ ticket: "binding-1", principal });
    expect(manager.getSnapshot()).toBeUndefined();
  });
});
