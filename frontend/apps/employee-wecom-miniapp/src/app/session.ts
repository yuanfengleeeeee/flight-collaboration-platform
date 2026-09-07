import { SessionManager } from "@flight/auth";
import { MiniappEdgeGateway, MiniappRealtimeClient, MiniappSessionStore } from "../platform/edge-gateway";
import { edgeBaseUrl } from "./runtime-config";

const sessionRuntime: { manager?: SessionManager } = {};
const gateway = new MiniappEdgeGateway(edgeBaseUrl, () => sessionRuntime.manager?.getAccessToken());
const manager = new SessionManager(gateway, new MiniappSessionStore());
sessionRuntime.manager = manager;
const realtime = new MiniappRealtimeClient(gateway, edgeBaseUrl);

export { gateway as edgeGateway, manager as miniappSession, realtime as miniappRealtime };
