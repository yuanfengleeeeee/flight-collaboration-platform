import { projectionStatus, taskReceiptLabel, taskStatusLabel } from "@flight/task-domain";
import { edgeGateway, isMiniappUnauthorized, miniappRealtime, relaunchMiniappLogin, restoreMiniappSession } from "../../src/app/session";
import type { EdgeTaskProjection } from "@flight/contracts";

let taskLoadPromise: Promise<void> | undefined;

Page({
  data: { tasks: [] as (EdgeTaskProjection & { status_label: string; receipt_label: string })[], loading: true, error: "", realtimeState: "idle", projectionLagState: "unknown", projectionLagSeconds: 0, projectionRevision: 0 },

  onShow() {
    void this.prepare();
  },

  onHide() { miniappRealtime.stop(); },

  async prepare() {
    if (!(await restoreMiniappSession())) {
      relaunchMiniappLogin();
      return;
    }
    void this.loadTasks();
    miniappRealtime.start(() => { void this.loadTasks(); }, (state) => this.setData({ realtimeState: state }), () => { void this.prepare(); });
  },

  async loadTasks() {
    if (taskLoadPromise) return taskLoadPromise;
    taskLoadPromise = this.refreshTasks().finally(() => { taskLoadPromise = undefined; });
    return taskLoadPromise;
  },

  async refreshTasks() {
    this.setData({ loading: true, error: "" });
    try {
      const response = await edgeGateway.listTasks();
      const tasks = response.items.map((task) => ({ ...task, status_label: taskStatusLabel[projectionStatus(task)], receipt_label: taskReceiptLabel(projectionStatus(task), task.receipt_status || "pending") }));
      this.setData({ tasks, projectionLagState: response.projection_lag_state, projectionLagSeconds: response.projection_lag_seconds, projectionRevision: response.projection_revision, loading: false });
    } catch (error) {
      if (isMiniappUnauthorized(error)) {
        relaunchMiniappLogin();
        return;
      }
      this.setData({ loading: false, error: error instanceof Error ? error.message : "任务同步失败" });
    }
  },

  reload() { void this.loadTasks(); },

  openTask(event: { currentTarget: { dataset: { id: string } } }) {
    wx.navigateTo({ url: `/pages/task-detail/index?id=${encodeURIComponent(event.currentTarget.dataset.id)}` });
  },
});
