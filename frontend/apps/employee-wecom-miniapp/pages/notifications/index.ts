import { edgeGateway } from "../../src/app/session";
import type { EdgeNotification } from "@flight/contracts";

Page({
  data: { items: [] as EdgeNotification[], loading: true, error: "", reading: "", page: 1, pageSize: 20, total: 0 },

  onShow() { this.loadNotifications(this.data.page); },

  async loadNotifications(page = 1) {
    this.setData({ loading: true, error: "" });
    try {
      const response = await edgeGateway.listNotifications(undefined, page, this.data.pageSize);
      this.setData({ items: response.data.items, page: response.data.page, total: response.data.total, loading: false });
    } catch (error) {
      this.setData({ loading: false, error: error instanceof Error ? error.message : "通知读取失败" });
    }
  },

  previousPage() { if (this.data.page > 1) this.loadNotifications(this.data.page - 1); },

  nextPage() { if (this.data.page * this.data.pageSize < this.data.total) this.loadNotifications(this.data.page + 1); },

  async markRead(event: { currentTarget: { dataset: { id: string } } }) {
    const publicID = event.currentTarget.dataset.id;
    if (!publicID || this.data.reading) return;
    this.setData({ reading: publicID, error: "" });
    try {
      await edgeGateway.markNotificationRead(publicID);
      const items = this.data.items.map((item) => item.public_id === publicID ? { ...item, status: "read" as const } : item);
      this.setData({ items, reading: "" });
    } catch (error) {
      this.setData({ reading: "", error: error instanceof Error ? error.message : "通知状态更新失败" });
    }
  },
});
