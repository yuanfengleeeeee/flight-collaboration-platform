import { ApiClient, EdgeApiClient, RealtimeClient } from "@flight/api-client";
import type { RealtimeState } from "@flight/api-client";
import { BrowserSessionStore, SessionManager } from "@flight/auth";

const env = import.meta.env;
const edgeBaseUrl = env.VITE_EDGE_API_BASE_URL ?? "/edge-api";
let restorePromise: Promise<unknown> | undefined;
const sessionRuntime: { manager?: SessionManager } = {};

const edgeHttp = new ApiClient({
  baseUrl: edgeBaseUrl,
  getAccessToken: () => sessionRuntime.manager?.getAccessToken(),
});

export const edgeApi = new EdgeApiClient(edgeHttp);
const sessionManager = new SessionManager(edgeApi, new BrowserSessionStore("flight.edge.employee-web.session"));
sessionRuntime.manager = sessionManager;

export const employeeRealtime = new RealtimeClient({
  webSocketUrl: toWebSocketURL(edgeBaseUrl),
  getTicket: (signal) => edgeApi.realtimeTicket(signal),
  onConnected: () => window.dispatchEvent(new Event("flight:realtime-connected")),
  onTaskChanged: (notification) => window.dispatchEvent(new CustomEvent("flight:task-changed", { detail: notification })),
  onStateChange: (state: RealtimeState) => window.dispatchEvent(new CustomEvent("flight:realtime-state", { detail: state })),
  onUnauthorized: () => window.dispatchEvent(new Event("flight:realtime-unauthorized")),
});

export { sessionManager };

export function restoreEmployeeSession(): Promise<unknown> {
  return restorePromise ??= sessionManager.restore();
}

function toWebSocketURL(baseUrl: string): string {
  if (baseUrl.startsWith("/")) {
    return `${window.location.protocol === "https:" ? "wss:" : "ws:"}//${window.location.host}${baseUrl.replace(/\/$/, "")}/api/v1/ws`;
  }
  return `${baseUrl.replace(/^http:/, "ws:").replace(/^https:/, "wss:").replace(/\/$/, "")}/api/v1/ws`;
}
