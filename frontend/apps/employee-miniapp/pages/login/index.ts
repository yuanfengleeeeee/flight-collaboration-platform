import { exchangeProviderCode, passwordLoginAndBind, personalWechatLoginCode } from "../../src/app/provider-login";
import { miniappSession } from "../../src/app/session";

Page({
  data: { employeeNo: "", password: "", error: "", loading: false, passwordLoading: false },

  inputValue(event: { currentTarget: { dataset: { field: "employeeNo" | "password" } }; detail: { value: string } }) {
    this.setData({ [event.currentTarget.dataset.field]: event.detail.value, error: "" });
  },

  async login() {
    this.setData({ loading: true, error: "" });
    try {
      await passwordLoginAndBind(this.data.employeeNo, this.data.password);
      wx.reLaunch({ url: "/pages/tasks/index" });
    } catch (error) {
      this.setData({ error: error instanceof Error ? error.message : "登录失败，请稍后重试" });
    } finally {
      this.setData({ loading: false });
    }
  },

  async quickLogin() {
    this.setData({ loading: true, error: "" });
    try {
      await exchangeProviderCode(await personalWechatLoginCode());
      wx.reLaunch({ url: "/pages/tasks/index" });
    } catch (error) {
      this.setData({ error: error instanceof Error ? error.message : "个人微信快捷登录失败，请先使用工号密码绑定" });
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
