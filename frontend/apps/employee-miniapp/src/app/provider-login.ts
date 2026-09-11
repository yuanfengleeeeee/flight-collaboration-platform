import type { AuthResult, ProviderExchangeRequest } from "@flight/contracts";
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
 * Exchange is deliberately provider-code based. The personal WeChat client
 * sends only its one-time wx.login code; the enterprise WeChat client has its
 * own adapter and client identifier.
 */
export async function exchangeProviderCode(loginCode: string): Promise<AuthResult> {
  const payload: ProviderExchangeRequest = { provider: "personal_wechat", provider_code: loginCode, client: "employee-miniapp" };
  return miniappSession.exchange(payload);
}

/**
 * First-use binding is a two-code flow: password establishes the employee
 * account and returns a one-time binding ticket, then a fresh personal WeChat
 * code binds the current identity to that Staff.
 */
export async function passwordLoginAndBind(employeeNo: string, password: string, getCode: LoginCodeProvider = personalWechatLoginCode): Promise<AuthResult> {
  const passwordResult = await miniappSession.passwordLogin({
    employee_no: employeeNo,
    password,
    client: "employee-miniapp",
    provider: "personal_wechat",
  });
  if (passwordResult.state === "authenticated") return passwordResult;
  if (!passwordResult.binding_ticket) throw new Error("绑定票据为空");
  const code = await getCode();
  return miniappSession.completeBinding({
    binding_ticket: passwordResult.binding_ticket,
    provider: "personal_wechat",
    provider_code: code,
    client: "employee-miniapp",
  });
}
