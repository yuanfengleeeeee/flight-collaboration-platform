import { projectionStatus, taskStatusLabel } from "@flight/task-domain";
import { edgeGateway, miniappRealtime } from "../../src/app/session";
import type { EdgeTaskProjection } from "@flight/contracts";

Page({
  data: { tasks: [] as (EdgeTaskProjection & { status_label: string })[], loading: true, error: "", realtimeState: "idle" },

  onShow() {
    this.loadTasks();
    miniappRealtime.start(() => this.loadTasks(), (state) => this.setData({ realtimeState: state }), () => wx.reLaunch({ url: "/pages/login/index" }));
  },

  onHide() { miniappRealtime.stop(); },

  async loadTasks() {
    this.setData({ loading: true, error: "" });
    try {
      const response = await edgeGateway.listTasks();
      const tasks = response.items.map((task) => ({ ...task, status_label: taskStatusLabel[projectionStatus(task)] }));
      this.setData({ tasks, loading: false });
    } catch (error) {
      this.setData({ loading: false, error: error instanceof Error ? error.message : "任务同步失败" });
    }
  },

  openTask(event: { currentTarget: { dataset: { id: string } } }) {
    wx.navigateTo({ url: `/pages/task-detail/index?id=${encodeURIComponent(event.currentTarget.dataset.id)}` });
  },
});
