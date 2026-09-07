import { canComplete, canReceive, canStart, createClientID, projectionStatus, taskStatusLabel } from "@flight/task-domain";
import { edgeGateway, miniappRealtime } from "../../src/app/session";
import type { EdgeTaskProjection } from "@flight/contracts";

Page({
  data: { task: undefined as EdgeTaskProjection | undefined, taskID: "", statusLabel: "", loading: false, error: "", notice: "", canReceive: false, canStart: false, canComplete: false, realtimeState: "idle" },

  onLoad(query: { id?: string }) {
    this.setData({ taskID: query.id || "" });
    this.loadTask(query.id || "");
  },

  onShow() { miniappRealtime.start(() => this.loadTask(this.data.taskID), (state) => this.setData({ realtimeState: state }), () => wx.reLaunch({ url: "/pages/login/index" })); },
  onHide() { miniappRealtime.stop(); },

  async loadTask(taskID: string) {
    try {
      const response = await edgeGateway.listTasks();
      const task = response.items.find((item) => item.public_id === taskID);
      if (!task) throw new Error("任务已不在当前员工 Projection 中");
      const status = projectionStatus(task);
      const receiptStatus = task.receipt_status || "pending";
      this.setData({ task, statusLabel: taskStatusLabel[status], canReceive: canReceive(status, receiptStatus), canStart: canStart(status, receiptStatus), canComplete: canComplete(status) });
    } catch (error) {
      this.setData({ error: error instanceof Error ? error.message : "任务详情读取失败" });
    }
  },

  async receive() { this.sendCommand("receive"); },
  async start() { this.sendCommand("start"); },
  async complete() { this.sendCommand("complete"); },

  async sendCommand(action: "receive" | "start" | "complete") {
    const task = this.data.task;
    if (!task || !task.assignment_public_id) return;
    this.setData({ loading: true, error: "", notice: "" });
    const payload = { command_id: createClientID(), assignment_public_id: task.assignment_public_id, expected_sync_version: task.sync_version };
    try {
      const accepted = action === "receive" ? await edgeGateway.receiveTask(task.public_id, payload) : action === "start" ? await edgeGateway.startTask(task.public_id, payload) : await edgeGateway.completeTask(task.public_id, payload);
      this.setData({ notice: `命令已保存：${accepted.status}。最终任务状态以刷新后的 Projection 为准。` });
      await this.loadTask(task.public_id);
    } catch (error) {
      this.setData({ error: error instanceof Error ? error.message : "操作已提交，但状态尚未确认" });
    } finally {
      this.setData({ loading: false });
    }
  },

  reload() { this.loadTask(this.data.taskID); },
});
