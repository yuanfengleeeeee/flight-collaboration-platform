import { miniappSession } from "../../src/app/session";

Page({
  data: { loading: false },
  async logout() {
    this.setData({ loading: true });
    try { await miniappSession.logout(); } finally { this.setData({ loading: false }); }
    wx.reLaunch({ url: "/pages/login/index" });
  },
});
