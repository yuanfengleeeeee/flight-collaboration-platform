import { createClientID, suggestedChangeActionForExceptionCategory } from "@flight/task-domain";
import { edgeGateway } from "../../src/app/session";
import type { EdgeTaskProjection, ExceptionSeverity, TaskChangeAction } from "@flight/contracts";

type RequestedChangeAction = "" | TaskChangeAction;
const changeActionOptions = ["仅上报，不申请变更", "申请暂停", "申请重新预分配", "申请调整计划时间", "申请取消", "申请恢复执行"];
const changeActionValues: RequestedChangeAction[] = ["", "pause", "reassign", "reschedule", "cancel", "resume"];

Page({
  data: { tasks: [] as EdgeTaskProjection[], taskID: "", taskIndex: 0, categoryOptions: ["保障冲突（申请重新预分配）", "航班延误（申请调整任务）", "航班取消（申请取消任务）", "突发事件（申请值班经理处置）", "其他现场异常"], categoryIndex: 0, category: "保障冲突（申请重新预分配）", changeActionOptions, changeActionValues, changeActionIndex: 2, changeAction: "reassign" as RequestedChangeAction, targetPlannedAt: "", severity: "medium" as ExceptionSeverity, description: "", loading: true, submitting: false, error: "", notice: "" },

  onShow() { this.loadTasks(); },

  async loadTasks() {
    this.setData({ loading: true, error: "" });
    try {
      const response = await edgeGateway.listTasks();
      const tasks = response.items.filter((task) => Boolean(task.assignment_public_id) && task.business_status !== "cancelled");
      const taskIndex = Math.max(0, tasks.findIndex((task) => task.public_id === this.data.taskID));
      this.setData({ tasks, taskIndex, taskID: tasks[taskIndex]?.public_id || "", loading: false });
    } catch (error) {
      this.setData({ loading: false, error: error instanceof Error ? error.message : "任务读取失败" });
    }
  },

  async submit() {
    const task = this.data.tasks.find((value) => value.public_id === this.data.taskID);
    if (!task?.assignment_public_id || !this.data.category.trim() || !this.data.description.trim() || this.data.submitting || (this.data.changeAction === "reschedule" && !this.data.targetPlannedAt.trim())) return;
    this.setData({ submitting: true, error: "", notice: "" });
    try {
      const response = await edgeGateway.reportException(task.public_id, { command_id: createClientID(), assignment_public_id: task.assignment_public_id, expected_sync_version: task.sync_version, category: this.data.category.trim(), severity: this.data.severity, description: this.data.description.trim(), client_occurred_at: new Date().toISOString(), change_action: this.data.changeAction || undefined, target_planned_at: this.data.changeAction === "reschedule" && this.data.targetPlannedAt ? new Date(this.data.targetPlannedAt).toISOString() : undefined });
      this.setData({ submitting: false, description: "", notice: `异常已保存到 Edge：${response.status}` });
    } catch (error) {
      this.setData({ submitting: false, error: error instanceof Error ? error.message : "异常提交失败" });
    }
  },

  chooseTask(event: { detail: { value: string } }) { const taskIndex = Number(event.detail.value); this.setData({ taskIndex, taskID: this.data.tasks[taskIndex]?.public_id || "" }); },
  chooseSeverity(event: { detail: { value: string } }) { this.setData({ severity: event.detail.value as ExceptionSeverity }); },
  chooseCategory(event: { detail: { value: string } }) { const categoryIndex = Number(event.detail.value); const category = this.data.categoryOptions[categoryIndex] || this.data.categoryOptions[0]; const suggested = suggestedChangeActionForExceptionCategory(category) || ""; const changeActionIndex = Math.max(0, changeActionValues.indexOf(suggested)); this.setData({ categoryIndex, category, changeAction: suggested, changeActionIndex, targetPlannedAt: "" }); },
  chooseChangeAction(event: { detail: { value: string } }) { const changeActionIndex = Number(event.detail.value); const changeAction = changeActionValues[changeActionIndex] || ""; this.setData({ changeActionIndex, changeAction, targetPlannedAt: changeAction === "reschedule" ? this.data.targetPlannedAt : "" }); },
  inputTargetPlannedAt(event: { detail: { value: string } }) { this.setData({ targetPlannedAt: event.detail.value }); },
  inputDescription(event: { detail: { value: string } }) { this.setData({ description: event.detail.value }); },
});
