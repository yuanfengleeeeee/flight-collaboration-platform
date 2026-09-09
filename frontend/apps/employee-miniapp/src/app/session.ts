import { SessionManager } from "@flight/auth";
import { MiniappEdgeGateway, MiniappHttpError, MiniappRealtimeClient, MiniappSessionStore, MiniappTaskCommandStore } from "../platform/edge-gateway";
import { edgeBaseUrl } from "./runtime-config";

const sessionRuntime: { manager?: SessionManager } = {};
const gateway = new MiniappEdgeGateway(edgeBaseUrl, () => sessionRuntime.manager?.getAccessToken());
const manager = new SessionManager(gateway, new MiniappSessionStore());
const realtime = new MiniappRealtimeClient(gateway, edgeBaseUrl);
const taskCommandStore = new MiniappTaskCommandStore("flight.edge.employee-miniapp.task-command");
sessionRuntime.manager = manager;

let restorePromise: Promise<boolean> | undefined;
let loginRedirectPending = false;

/**
 * Each mini-program page is bundled as an independent entry, so page startup
 * must validate the persisted Edge session before requesting task data.
 * Keeping one in-flight restore also prevents tab switches from stampeding
 * the auth endpoint and starting the socket before the token is ready.
 */
export function restoreMiniappSession(): Promise<boolean> {
  if (restorePromise) return restorePromise;
  restorePromise = manager.restore().then((snapshot) => Boolean(snapshot), () => false).finally(() => { restorePromise = undefined; });
  return restorePromise;
}

export function isMiniappUnauthorized(error: unknown): boolean {
  return error instanceof MiniappHttpError && error.status === 401;
}

export function relaunchMiniappLogin(): void {
  if (loginRedirectPending) return;
  loginRedirectPending = true;
  wx.reLaunch({ url: "/pages/login/index" });
  setTimeout(() => { loginRedirectPending = false; }, 1000);
}

export { gateway as edgeGateway, manager as miniappSession, realtime as miniappRealtime, taskCommandStore };
