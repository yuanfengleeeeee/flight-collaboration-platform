import { SessionManager } from "@flight/auth";
import { MiniappEdgeGateway, MiniappSessionStore } from "../platform/edge-gateway";

const sessionRuntime: { manager?: SessionManager } = {};
const gateway = new MiniappEdgeGateway("https://edge.example.invalid", () => sessionRuntime.manager?.getAccessToken());
const manager = new SessionManager(gateway, new MiniappSessionStore());
sessionRuntime.manager = manager;

export { gateway as edgeGateway, manager as miniappSession };
