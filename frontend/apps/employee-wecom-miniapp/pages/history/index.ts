import { edgeGateway, isMiniappUnauthorized, relaunchMiniappLogin, restoreMiniappSession } from "../../src/app/session";
import type { EdgeTaskProjection } from "@flight/contracts";

let historyLoadPromise: Promise<void> | undefined;

Page({
  data: { items: [] as EdgeTaskProjection[], loading: true, error: "", page: 1, pageSize: 20, total: 0 },

  onShow() { void this.prepare(); },

  async prepare() {
    if (!(await restoreMiniappSession())) {
      relaunchMiniappLogin();
      return;
    }
    void this.loadHistory(this.data.page);
  },

  async loadHistory(page = 1) {
    if (historyLoadPromise) return historyLoadPromise;
    historyLoadPromise = this.refreshHistory(page).finally(() => { historyLoadPromise = undefined; });
    return historyLoadPromise;
  },

  async refreshHistory(page = 1) {
    this.setData({ loading: true, error: "" });
    try {
      const response = await edgeGateway.listHistory(undefined, page, this.data.pageSize);
      this.setData({ items: response.data.items, page: response.data.page, total: response.data.total, loading: false });
    } catch (error) {
      if (isMiniappUnauthorized(error)) {
        relaunchMiniappLogin();
        return;
      }
      this.setData({ loading: false, error: error instanceof Error ? error.message : "历史任务读取失败" });
    }
  },

  previousPage() { if (this.data.page > 1) this.loadHistory(this.data.page - 1); },

  nextPage() { if (this.data.page * this.data.pageSize < this.data.total) this.loadHistory(this.data.page + 1); },

  reload() { void this.loadHistory(this.data.page); },

  openTask(event: { currentTarget: { dataset: { id: string } } }) {
    wx.navigateTo({ url: `/pages/task-detail/index?id=${encodeURIComponent(event.currentTarget.dataset.id)}` });
  },
});
