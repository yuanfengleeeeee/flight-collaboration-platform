import { canComplete, canReceive, canStart, commandStatusLabel, createClientID, projectionStatus, taskStatusLabel } from "@flight/task-domain";
import type { CommandStatus, EdgeTaskList, EdgeTaskProjection } from "@flight/contracts";
import { edgeGateway, isMiniappUnauthorized, miniappRealtime, relaunchMiniappLogin, restoreMiniappSession, taskCommandStore } from "../../src/app/session";
import { MiniappHttpError } from "../../src/platform/edge-gateway";
import type { MiniappTaskCommandAction, MiniappTaskCommandReceipt } from "../../src/platform/edge-gateway";

let pollingCommandID = "";
let pollingTimer: ReturnType<typeof setTimeout> | undefined;

Page({
  data: { task: undefined as EdgeTaskProjection | undefined, taskID: "", statusLabel: "", loading: false, error: "", notice: "", commandID: "", commandStatus: "" as CommandStatus | "", commandStatusLabel: "", canReceive: false, canStart: false, canComplete: false, realtimeState: "idle", projectionLagState: "unknown", projectionLagSeconds: 0, projectionRevision: 0 },

  onLoad(query: { id?: string }) {
    this.setData({ taskID: query.id || "" });
  },

  onShow() {
    void this.prepare();
  },

  onHide() { miniappRealtime.stop(); stopCommandPolling(); },

  async prepare() {
    if (!(await restoreMiniappSession())) {
      relaunchMiniappLogin();
      return;
    }
    miniappRealtime.start(() => { void this.loadTask(this.data.taskID); }, (state) => this.setData({ realtimeState: state }), () => { void this.prepare(); });
    if (this.data.taskID) this.resumeStoredCommand(this.data.taskID);
  },

  async loadTask(taskID: string, confirmedCommand?: MiniappTaskCommandReceipt) {
    if (!taskID) return;
    this.setData({ loading: true, error: "" });
    try {
      let response = await edgeGateway.listTasks();
      if (confirmedCommand) response = await waitForCommandProjection(taskID, confirmedCommand, response);
      const task = response.items.find((item) => item.public_id === taskID);
      if (!task) throw new Error("任务已不在当前员工 Projection 中，可能已被重新分配或撤回");
      const status = projectionStatus(task);
      const receiptStatus = task.receipt_status || "pending";
      this.setData({ task, statusLabel: taskStatusLabel[status], canReceive: canReceive(status, receiptStatus), canStart: canStart(status, receiptStatus), canComplete: canComplete(status), projectionLagState: response.projection_lag_state, projectionLagSeconds: response.projection_lag_seconds, projectionRevision: response.projection_revision, loading: false });
      this.resumeStoredCommand(taskID);
    } catch (error) {
      if (isMiniappUnauthorized(error)) {
        relaunchMiniappLogin();
        return;
      }
      this.setData({ loading: false, error: miniappErrorMessage(error, "任务详情读取失败") });
    }
  },

  receive() { this.sendCommand("receive"); },
  start() { this.sendCommand("start"); },
  complete() { this.sendCommand("complete"); },

  resumeStoredCommand(taskID: string) {
    const stored = taskCommandStore.read(taskID);
    if (!stored) return;
    if (stored.status === "confirmed" || stored.status === "failed") {
      taskCommandStore.clear(stored.id);
      this.setData({ commandID: "", commandStatus: "", commandStatusLabel: "" });
      return;
    }
    this.setData({ commandID: stored.id, commandStatus: stored.status || "pending", commandStatusLabel: commandStatusLabel[stored.status || "pending"] });
    this.pollCommand(stored.id, taskID);
  },

  sendCommand(action: MiniappTaskCommandAction) {
    const task = this.data.task;
    if (!task || !task.assignment_public_id || this.data.loading || this.data.commandID) return;
    this.setData({ loading: true, error: "", notice: "" });
    const stored = taskCommandStore.read(task.public_id);
    const reusable = stored && stored.action === action && stored.assignmentPublicID === task.assignment_public_id && stored.expectedSyncVersion === task.sync_version && stored.status !== "confirmed" && stored.status !== "failed";
    const draft: MiniappTaskCommandReceipt = reusable && stored ? stored : { id: createClientID(), action, taskPublicID: task.public_id, assignmentPublicID: task.assignment_public_id, expectedSyncVersion: task.sync_version, status: "pending" };
    taskCommandStore.write(draft);
    this.setData({ commandID: draft.id, commandStatus: "pending", commandStatusLabel: commandStatusLabel.pending });
    const payload = { command_id: draft.id, assignment_public_id: draft.assignmentPublicID, expected_sync_version: draft.expectedSyncVersion };
    (action === "receive" ? edgeGateway.receiveTask(task.public_id, payload) : action === "start" ? edgeGateway.startTask(task.public_id, payload) : edgeGateway.completeTask(task.public_id, payload)).then((accepted) => {
      taskCommandStore.write({ ...draft, status: accepted.status });
      this.setData({ loading: false, notice: `命令已持久化到 Edge（${commandStatusLabel[accepted.status]}），最终结果以刷新后的任务快照为准。` });
      this.pollCommand(draft.id, task.public_id);
    }).catch((error) => {
      this.setData({ loading: false, error: miniappErrorMessage(error, "操作已提交，但同步状态尚未确认") });
    });
  },

  pollCommand(commandID: string, taskID: string) {
    if (pollingCommandID === commandID) return;
    stopCommandPolling();
    pollingCommandID = commandID;
    const poll = async (): Promise<void> => {
      if (pollingCommandID !== commandID) return;
      try {
        const response = await edgeGateway.commandStatus(commandID);
        if (pollingCommandID !== commandID) return;
        const status = response.data.status;
        const current = taskCommandStore.read(taskID);
        if (current?.id === commandID) taskCommandStore.write({ ...current, status });
        this.setData({ commandID, commandStatus: status, commandStatusLabel: commandStatusLabel[status] });
        if (status === "confirmed" || status === "failed") {
          taskCommandStore.clear(commandID);
          pollingCommandID = "";
          this.setData({ commandID: "", commandStatus: "", commandStatusLabel: "" });
          if (status === "failed") this.setData({ error: response.data.error_code || "command_failed" });
          else if (current) await this.loadTask(taskID, current);
          else await this.loadTask(taskID);
          return;
        }
        pollingTimer = setTimeout(() => void poll(), 2000);
      } catch (error) {
        this.setData({ error: miniappErrorMessage(error, "暂时无法查询命令状态，系统会继续重试") });
        pollingTimer = setTimeout(() => void poll(), 5000);
      }
    };
    void poll();
  },

  reload() { this.loadTask(this.data.taskID); },
});

const projectionRefreshAttempts = 10;
const projectionRefreshDelayMS = 500;

async function waitForCommandProjection(taskID: string, command: MiniappTaskCommandReceipt, initial: EdgeTaskList): Promise<EdgeTaskList> {
  let response = initial;
  for (let attempt = 0; attempt < projectionRefreshAttempts; attempt += 1) {
    const task = response.items.find((item) => item.public_id === taskID);
    if (task && projectionMatchesCommand(task, command)) return response;
    await new Promise((resolve) => setTimeout(resolve, projectionRefreshDelayMS));
    response = await edgeGateway.listTasks();
  }
  return response;
}

function projectionMatchesCommand(task: EdgeTaskProjection, command: MiniappTaskCommandReceipt): boolean {
  if (task.sync_version <= command.expectedSyncVersion) return false;
  const status = projectionStatus(task);
  if (command.action === "receive") return task.receipt_status === "received";
  if (command.action === "start") return status === "in_progress";
  return status === "completed";
}

function stopCommandPolling(): void {
  pollingCommandID = "";
  if (pollingTimer !== undefined) clearTimeout(pollingTimer);
  pollingTimer = undefined;
}

function miniappErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof MiniappHttpError && error.body && typeof error.body === "object") {
    const body = error.body as { code?: unknown; message?: unknown; request_id?: unknown };
    if (typeof body.message === "string") return `${typeof body.code === "string" ? `${body.code} · ` : ""}${body.message}${typeof body.request_id === "string" ? `（request_id: ${body.request_id}）` : ""}`;
  }
  return error instanceof Error ? error.message : fallback;
}
