export { edgeGateway, miniappSession } from "./session";
export { exchangeProviderCode, passwordLoginAndBind, personalWechatLoginCode } from "./provider-login";

export const employeeMiniappContract = {
  clients: ["personal_wechat", "wecom"] as const,
  edgeOnly: true,
  projectionIsReadOnlySource: true,
};
