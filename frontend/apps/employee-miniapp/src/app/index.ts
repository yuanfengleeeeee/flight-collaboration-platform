export { edgeGateway, miniappRealtime, miniappSession } from "./session";
export { exchangeProviderCode, passwordLoginAndBind, personalWechatLoginCode } from "./provider-login";

export const employeeMiniappContract = {
  client: "employee-miniapp" as const,
  provider: "personal_wechat" as const,
  edgeOnly: true,
  projectionIsReadOnlySource: true,
};
