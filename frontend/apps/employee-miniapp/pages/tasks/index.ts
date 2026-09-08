import { projectionStatus, taskReceiptLabel, taskStatusLabel } from "@flight/task-domain";
import { edgeGateway, miniappRealtime } from "../../src/app/session";
import type { EdgeTaskProjection } from "@flight/contracts";

Page({
  data: { tasks: [] as (EdgeTaskProjection & { status_label: string; receipt_label: string })[], loading: true, error: "", realtimeState: "idle", projectionLagState: "unknown", projectionLagSeconds: 0, projectionRevision: 0 },

  onShow() {
    this.loadTasks();
    miniappRealtime.start(() => this.loadTasks(), (state) => this.setData({ realtimeState: state }), () => wx.reLaunch({ url: "/pages/login/index" }));
  },

  onHide() { miniappRealtime.stop(); },

  async loadTasks() {
    this.setData({ loading: true, error: "" });
    try {
      const response = await edgeGateway.listTasks();
      const tasks = response.items.map((task) => ({ ...task, status_label: taskStatusLabel[projectionStatus(task)], receipt_label: taskReceiptLabel(projectionStatus(task), task.receipt_status || "pending") }));
      this.setData({ tasks, projectionLagState: response.projection_lag_state, projectionLagSeconds: response.projection_lag_seconds, projectionRevision: response.projection_revision, loading: false });
    } catch (error) {
      this.setData({ loading: false, error: error instanceof Error ? error.message : "任务同步失败" });
    }
  },

  openTask(event: { currentTarget: { dataset: { id: string } } }) {
    wx.navigateTo({ url: `/pages/task-detail/index?id=${encodeURIComponent(event.currentTarget.dataset.id)}` });
  },
});
