import { SessionManager } from "@flight/auth";
import { MiniappEdgeGateway, MiniappRealtimeClient, MiniappSessionStore, MiniappTaskCommandStore } from "../platform/edge-gateway";
import { edgeBaseUrl } from "./runtime-config";

const sessionRuntime: { manager?: SessionManager } = {};
const gateway = new MiniappEdgeGateway(edgeBaseUrl, () => sessionRuntime.manager?.getAccessToken());
const manager = new SessionManager(gateway, new MiniappSessionStore());
const realtime = new MiniappRealtimeClient(gateway, edgeBaseUrl);
const taskCommandStore = new MiniappTaskCommandStore("flight.edge.employee-miniapp.task-command");
sessionRuntime.manager = manager;

export { gateway as edgeGateway, manager as miniappSession, realtime as miniappRealtime, taskCommandStore };
