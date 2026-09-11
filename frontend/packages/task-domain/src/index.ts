import type { AssignmentReceiptStatus, AssignmentStatus, CommandStatus, EdgeTaskProjection, TaskChangeAction, TaskChangeRequestStatus, TaskStatus } from "@flight/contracts";

export const taskStatusLabel: Record<TaskStatus, string> = {
  pending_dispatch: "待自动派发",
  awaiting_confirmation: "待确认",
  assigned: "已分配",
  in_progress: "执行中",
  paused: "已暂停",
  completed: "已完成",
  cancelled: "已取消",
};

export const commandStatusLabel: Record<CommandStatus, string> = {
  pending: "等待同步",
  syncing: "同步中",
  confirmed: "已同步",
  failed: "同步失败",
};

export const assignmentStatusLabel: Record<AssignmentStatus, string> = {
  confirmed: "已分配",
  accepted: "历史兼容状态",
  completed: "已完成",
  cancelled: "已取消",
};

export const receiptStatusLabel: Record<AssignmentReceiptStatus, string> = {
  pending: "待确认收件",
  received: "已收到",
};

export const taskChangeActionLabel: Record<TaskChangeAction, string> = {
  pause: "申请暂停",
  reassign: "申请重新分配",
  reschedule: "申请调整计划时间",
  cancel: "申请取消",
  resume: "申请恢复执行",
};

export const taskChangeRequestStatusLabel: Record<TaskChangeRequestStatus, string> = {
  pending: "待审批",
  approved: "已批准",
  rejected: "已驳回",
  applied: "已应用",
  failed: "应用失败",
};

export const flightStatusLabel: Record<string, string> = {
  scheduled: "计划中",
  arrived: "已到达",
  departed: "已离港",
  cancelled: "已取消",
};

export const sourceStateLabel: Record<string, string> = {
  fresh: "来源正常",
  stale: "来源延迟",
  fallback: "使用备份事实",
  failed: "来源失败",
};

export function projectionStatus(projection: EdgeTaskProjection): TaskStatus {
  return projection.business_status ?? projection.status ?? "awaiting_confirmation";
}

export function canReceive(status: TaskStatus, receiptStatus: AssignmentReceiptStatus = "pending"): boolean { return status === "assigned" && receiptStatus !== "received"; }
export function canStart(status: TaskStatus, receiptStatus: AssignmentReceiptStatus = "pending"): boolean { return status === "assigned" && receiptStatus === "received"; }
export function canComplete(status: TaskStatus): boolean { return status === "in_progress"; }
export function isTerminal(status: TaskStatus): boolean { return status === "completed" || status === "cancelled"; }

export function taskReceiptLabel(status: TaskStatus, receiptStatus: AssignmentReceiptStatus = "pending"): string {
  if (status === "assigned") return receiptStatusLabel[receiptStatus];
  if (status === "paused") return "任务已暂停";
  if (status === "in_progress") return "执行中";
  if (status === "completed") return "已完成";
  if (status === "cancelled") return "已取消";
  return taskStatusLabel[status];
}

export function changeActionsForTaskStatus(status: TaskStatus): TaskChangeAction[] {
  if (status === "paused") return ["resume"];
  if (status === "assigned" || status === "in_progress") return ["pause", "reassign", "reschedule", "cancel"];
  return [];
}

export function suggestedChangeActionForExceptionCategory(category: string): TaskChangeAction | undefined {
  if (category.includes("保障冲突")) return "reassign";
  if (category.includes("航班延误")) return "reschedule";
  if (category.includes("航班取消")) return "cancel";
  return undefined;
}

export function createClientID(): string {
  const cryptoValue = globalThis.crypto?.randomUUID?.();
  if (cryptoValue) return cryptoValue;

  // API contracts use UUID-shaped idempotency keys. Keep a standards-shaped
  // fallback for WebViews and miniapp runtimes without crypto.randomUUID.
  const bytes = new Uint8Array(16);
  if (globalThis.crypto?.getRandomValues) globalThis.crypto.getRandomValues(bytes);
  else for (let index = 0; index < bytes.length; index += 1) bytes[index] = Math.floor(Math.random() * 256);
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = [...bytes].map((value) => value.toString(16).padStart(2, "0"));
  return `${hex.slice(0, 4).join("")}-${hex.slice(4, 6).join("")}-${hex.slice(6, 8).join("")}-${hex.slice(8, 10).join("")}-${hex.slice(10).join("")}`;
}
