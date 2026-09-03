import type { CommandStatus, EdgeTaskProjection, TaskStatus } from "@flight/contracts";

export const taskStatusLabel: Record<TaskStatus, string> = {
  awaiting_confirmation: "待确认",
  assigned: "已分配",
  in_progress: "执行中",
  completed: "已完成",
  cancelled: "已取消",
};

export const commandStatusLabel: Record<CommandStatus, string> = {
  pending: "等待同步",
  syncing: "同步中",
  confirmed: "已同步",
  failed: "同步失败",
};

export function projectionStatus(projection: EdgeTaskProjection): TaskStatus {
  return projection.business_status ?? projection.status ?? "awaiting_confirmation";
}

export function canAccept(status: TaskStatus): boolean { return status === "assigned"; }
export function canComplete(status: TaskStatus): boolean { return status === "in_progress"; }
export function isTerminal(status: TaskStatus): boolean { return status === "completed" || status === "cancelled"; }

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
