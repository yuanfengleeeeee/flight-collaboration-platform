import type { AuthResult, Provider, ProviderExchangeRequest } from "@flight/contracts";
import { miniappSession } from "./session";

export type LoginCodeProvider = () => Promise<string>;

/**
 * Personal WeChat mini-program login uses a one-time wx.login code. The code
 * is sent to Edge and then Core verifies it with WeChat; AppSecret never
 * crosses into this package or the mini-program runtime.
 */
export function personalWechatLoginCode(): Promise<string> {
  return new Promise((resolve, reject) => {
    wx.login({
      success: (result) => {
        if (result.code) {
          resolve(result.code);
          return;
        }
        reject(new Error(result.errMsg || "微信登录凭证为空"));
      },
      fail: reject,
    });
  });
}

/**
 * Exchange is deliberately provider-code based. For a WeCom H5 entry, pass
 * the OAuth callback code as `loginCode`; for a WeCom-native mini-program
 * entry, inject the code provider supplied by that platform runtime.
 */
export async function exchangeProviderCode(provider: Provider, loginCode: string): Promise<AuthResult> {
  const payload: ProviderExchangeRequest = { provider, provider_code: loginCode, client: "employee-miniapp" };
  return miniappSession.exchange(payload);
}

/**
 * First-use binding is a two-code flow: password establishes the employee
 * account and returns a one-time binding ticket, then a fresh platform code
 * binds the current WeChat/WeCom identity to that Staff.
 */
export async function passwordLoginAndBind(provider: Provider, employeeNo: string, password: string, getCode: LoginCodeProvider = personalWechatLoginCode): Promise<AuthResult> {
  const passwordResult = await miniappSession.passwordLogin({
    employee_no: employeeNo,
    password,
    client: "employee-miniapp",
    provider,
  });
  if (passwordResult.state === "authenticated") return passwordResult;
  if (!passwordResult.binding_ticket) throw new Error("绑定票据为空");
  const code = await getCode();
  return miniappSession.completeBinding({
    binding_ticket: passwordResult.binding_ticket,
    provider,
    provider_code: code,
    client: "employee-miniapp",
  });
}
