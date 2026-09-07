import { ApiClient, CoreApiClient } from "@flight/api-client";

const env = import.meta.env;
const accessTokenKey = "flight.core.admin.access-token";
const adminSsoStateKey = "flight.core.admin.sso-state";
const adminSsoReturnKey = "flight.core.admin.sso-return";

export const adminRuntime = {
  coreBaseUrl: env.VITE_CORE_API_BASE_URL ?? "/core-api",
  adminSsoStartUrl: env.VITE_ADMIN_SSO_START_URL ?? "",
  adminSsoExchangeUrl: env.VITE_ADMIN_SSO_EXCHANGE_URL ?? "",
  devActorEnabled: env.VITE_ENABLE_DEV_ACTOR === "true",
  devActorRole: env.VITE_DEV_ACTOR_ROLE ?? "manager",
};

export function readAdminAccessToken(): string {
  try {
    return window.localStorage.getItem(accessTokenKey) ?? env.VITE_ADMIN_ACCESS_TOKEN ?? "";
  } catch {
    return env.VITE_ADMIN_ACCESS_TOKEN ?? "";
  }
}

export function saveAdminAccessToken(value: string): void {
  window.localStorage.setItem(accessTokenKey, value.trim());
}

export function clearAdminAccessToken(): void {
  window.localStorage.removeItem(accessTokenKey);
}

export function adminSsoEnabled(): boolean {
  return Boolean(adminRuntime.adminSsoStartUrl && adminRuntime.adminSsoExchangeUrl);
}

export function beginAdminSso(returnPath = "/tasks"): void {
  if (!adminSsoEnabled()) throw new Error("管理端 SSO 尚未配置");
  const state = createSsoState();
  const callbackURL = `${window.location.origin}/sso/callback`;
  window.sessionStorage.setItem(adminSsoStateKey, state);
  window.sessionStorage.setItem(adminSsoReturnKey, safeReturnPath(returnPath));
  const url = new URL(adminRuntime.adminSsoStartUrl, window.location.origin);
  url.searchParams.set("redirect_uri", callbackURL);
  url.searchParams.set("state", state);
  window.location.assign(url.toString());
}

export function adminSsoReturnPath(): string {
  const value = window.sessionStorage.getItem(adminSsoReturnKey) ?? "/tasks";
  return safeReturnPath(value);
}

export async function exchangeAdminSsoCode(code: string, state: string): Promise<void> {
  if (!adminSsoEnabled()) throw new Error("管理端 SSO 尚未配置");
  const expectedState = window.sessionStorage.getItem(adminSsoStateKey);
  if (!expectedState || expectedState !== state) throw new Error("SSO 状态校验失败，请重新登录");
  const response = await fetch(adminRuntime.adminSsoExchangeUrl, {
    method: "POST",
    credentials: "include",
    headers: { Accept: "application/json", "Content-Type": "application/json; charset=utf-8" },
    body: JSON.stringify({ code, state, redirect_uri: `${window.location.origin}/sso/callback` }),
  });
  const raw = await response.text();
  let payload: unknown;
  try { payload = raw ? JSON.parse(raw) : undefined; } catch { throw new Error("SSO 服务端返回了无法识别的数据"); }
  if (!response.ok) throw new Error(readSsoMessage(payload, "SSO 登录失败"));
  const result = readSsoResult(payload);
  if (!result.access_token) throw new Error("SSO 响应没有 Core 会话令牌");
  saveAdminAccessToken(result.access_token);
  window.sessionStorage.removeItem(adminSsoStateKey);
  window.sessionStorage.removeItem(adminSsoReturnKey);
}

function createSsoState(): string {
  const bytes = new Uint8Array(18);
  window.crypto.getRandomValues(bytes);
  return Array.from(bytes, (value) => value.toString(16).padStart(2, "0")).join("");
}

function safeReturnPath(value: string): string {
  return value.startsWith("/") && !value.startsWith("//") ? value : "/tasks";
}

function readSsoResult(payload: unknown): { access_token?: string } {
  if (!payload || typeof payload !== "object") return {};
  const value = payload as { access_token?: unknown; data?: { access_token?: unknown } };
  if (typeof value.access_token === "string") return { access_token: value.access_token };
  if (value.data && typeof value.data.access_token === "string") return { access_token: value.data.access_token };
  return {};
}

function readSsoMessage(payload: unknown, fallback: string): string {
  if (!payload || typeof payload !== "object") return fallback;
  const value = payload as { message?: unknown; data?: { message?: unknown } };
  if (typeof value.message === "string") return value.message;
  if (value.data && typeof value.data.message === "string") return value.data.message;
  return fallback;
}

function devActorHeaders(): Record<string, string> {
  if (!adminRuntime.devActorEnabled) return {};
  return {
    "X-Actor-Type": "human",
    "X-Actor-Public-ID": env.VITE_DEV_ACTOR_PUBLIC_ID ?? "dev-manager",
    "X-Actor-Roles": adminRuntime.devActorRole,
    "X-Actor-Global": adminRuntime.devActorRole === "manager" || adminRuntime.devActorRole === "admin" || adminRuntime.devActorRole === "supervisor" ? "true" : "false",
    "X-Actor-Team-IDs": env.VITE_DEV_ACTOR_TEAM_IDS ?? "",
    "X-Actor-Area-IDs": env.VITE_DEV_ACTOR_AREA_IDS ?? "",
  };
}

export const coreApi = new CoreApiClient(new ApiClient({
  baseUrl: adminRuntime.coreBaseUrl,
  getAccessToken: readAdminAccessToken,
  extraHeaders: devActorHeaders,
}));

export function hasAdminAccess(): boolean {
  return Boolean(readAdminAccessToken() || adminRuntime.devActorEnabled);
}
