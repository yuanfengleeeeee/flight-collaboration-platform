import { createClientID } from "@flight/task-domain";
import { edgeGateway } from "../../src/app/session";
import type { EdgeTaskProjection } from "@flight/contracts";

Page({
  data: { task: undefined as EdgeTaskProjection | undefined, taskID: "", loading: false, error: "", notice: "", canAccept: false, canComplete: false },

  onLoad(query: { id?: string }) {
    this.setData({ taskID: query.id || "" });
    this.loadTask(query.id || "");
  },

  async loadTask(taskID: string) {
    try {
      const response = await edgeGateway.listTasks();
      const task = response.items.find((item) => item.public_id === taskID);
      if (!task) throw new Error("任务已不在当前员工 Projection 中");
      this.setData({ task, canAccept: task.business_status === "assigned", canComplete: task.business_status === "in_progress" });
    } catch (error) {
      this.setData({ error: error instanceof Error ? error.message : "任务详情读取失败" });
    }
  },

  async accept() { this.sendCommand("accept"); },
  async complete() { this.sendCommand("complete"); },

  async sendCommand(action: "accept" | "complete") {
    const task = this.data.task;
    if (!task || !task.assignment_public_id) return;
    this.setData({ loading: true, error: "", notice: "" });
    const payload = { command_id: createClientID(), assignment_public_id: task.assignment_public_id, expected_sync_version: task.sync_version };
    try {
      const accepted = action === "accept" ? await edgeGateway.acceptTask(task.public_id, payload) : await edgeGateway.completeTask(task.public_id, payload);
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
