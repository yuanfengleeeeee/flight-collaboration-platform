export { edgeGateway, miniappSession } from "./session";
export { enterpriseWechatLoginCode, passwordLoginAndBind, quickLogin } from "./provider-login";

export const employeeWeComMiniappContract = {
  client: "employee-wecom-miniapp" as const,
  provider: "wecom" as const,
  edgeOnly: true,
  projectionIsReadOnlySource: true,
};
