import { personalWechatLoginCode, passwordLoginAndBind } from "../../src/app/provider-login";
import { miniappSession } from "../../src/app/session";

Page({
  data: { provider: "personal_wechat", employeeNo: "", password: "", wecomCode: "", error: "", loading: false, passwordLoading: false },

  selectProvider(event: { currentTarget: { dataset: { provider: "personal_wechat" | "wecom" } } }) {
    this.setData({ provider: event.currentTarget.dataset.provider, error: "" });
  },

  inputValue(event: { currentTarget: { dataset: { field: "employeeNo" | "password" | "wecomCode" } }; detail: { value: string } }) {
    this.setData({ [event.currentTarget.dataset.field]: event.detail.value, error: "" });
  },

  async login() {
    const provider = this.data.provider as "personal_wechat" | "wecom";
    const getCode = provider === "personal_wechat" ? personalWechatLoginCode : async () => this.data.wecomCode;
    if (provider === "wecom" && !this.data.wecomCode) {
      this.setData({ error: "请从企业微信 OAuth 回调中取得 code 后再登录" });
      return;
    }
    this.setData({ loading: true, error: "" });
    try {
      await passwordLoginAndBind(provider, this.data.employeeNo, this.data.password, getCode);
      wx.reLaunch({ url: "/pages/tasks/index" });
    } catch (error) {
      this.setData({ error: error instanceof Error ? error.message : "登录失败，请稍后重试" });
    } finally {
      this.setData({ loading: false });
    }
  },

  async passwordOnlyLogin() {
    this.setData({ passwordLoading: true, error: "" });
    try {
      await miniappSession.passwordLogin({ employee_no: this.data.employeeNo, password: this.data.password, client: "employee-miniapp" });
      wx.reLaunch({ url: "/pages/tasks/index" });
    } catch (error) {
      this.setData({ error: error instanceof Error ? error.message : "登录失败，请稍后重试" });
    } finally {
      this.setData({ passwordLoading: false });
    }
  },
});
