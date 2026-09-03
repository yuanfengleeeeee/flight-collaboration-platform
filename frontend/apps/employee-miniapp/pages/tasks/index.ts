import { edgeGateway } from "../../src/app/session";
import type { EdgeTaskProjection } from "@flight/contracts";

Page({
  data: { tasks: [] as EdgeTaskProjection[], loading: true, error: "" },

  onShow() { this.loadTasks(); },

  async loadTasks() {
    this.setData({ loading: true, error: "" });
    try {
      const response = await edgeGateway.listTasks();
      this.setData({ tasks: response.items, loading: false });
    } catch (error) {
      this.setData({ loading: false, error: error instanceof Error ? error.message : "任务同步失败" });
    }
  },

  openTask(event: { currentTarget: { dataset: { id: string } } }) {
    wx.navigateTo({ url: `/pages/task-detail/index?id=${encodeURIComponent(event.currentTarget.dataset.id)}` });
  },
});
