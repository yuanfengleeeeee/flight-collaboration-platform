import type { AuthResult, ProviderExchangeRequest } from "@flight/contracts";
import { miniappSession } from "./session";

function getWeComLoginCode(): Promise<string> {
  return new Promise((resolve, reject) => {
    const login = wx.qy?.login;
    if (!login) {
      reject(new Error("当前运行环境不是企业微信小程序"));
      return;
    }
    login({
      success: (result) => {
        if (result.code) {
          resolve(result.code);
          return;
        }
        reject(new Error(result.errMsg || "企业微信登录凭证为空"));
      },
      fail: reject,
    });
  });
}

export function enterpriseWechatLoginCode(): Promise<string> {
  return getWeComLoginCode();
}

export async function quickLogin(): Promise<AuthResult> {
  const payload: ProviderExchangeRequest = {
    provider: "wecom",
    provider_code: await getWeComLoginCode(),
    client: "employee-wecom-miniapp",
  };
  return miniappSession.exchange(payload);
}

export async function passwordLoginAndBind(employeeNo: string, password: string): Promise<AuthResult> {
  const passwordResult = await miniappSession.passwordLogin({
    employee_no: employeeNo,
    password,
    client: "employee-wecom-miniapp",
    provider: "wecom",
  });
  if (passwordResult.state === "authenticated") return passwordResult;
  if (!passwordResult.binding_ticket) throw new Error("绑定票据为空");
  return miniappSession.completeBinding({
    binding_ticket: passwordResult.binding_ticket,
    provider: "wecom",
    provider_code: await getWeComLoginCode(),
    client: "employee-wecom-miniapp",
  });
}
