import type { CommandStatus } from "@flight/contracts";

export type TaskCommandAction = "receive" | "start" | "complete";

export interface TaskCommandReceipt {
  id: string;
  action: TaskCommandAction;
  taskPublicID: string;
  assignmentPublicID: string;
  expectedSyncVersion: number;
  note?: string;
  status?: CommandStatus;
}

interface StorageLike {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

const storageKey = "flight.edge.employee-web.task-command";

export class TaskCommandStore {
  constructor(private readonly storage: StorageLike | undefined = browserStorage()) {}

  read(taskPublicID?: string): TaskCommandReceipt | undefined {
    if (!this.storage) return undefined;
    try {
      const raw = this.storage.getItem(storageKey);
      if (!raw) return undefined;
      const value: unknown = JSON.parse(raw);
      if (!isTaskCommandReceipt(value) || (taskPublicID && value.taskPublicID !== taskPublicID)) return undefined;
      return value;
    } catch {
      return undefined;
    }
  }

  write(receipt: TaskCommandReceipt): void {
    if (!this.storage) return;
    try {
      this.storage.setItem(storageKey, JSON.stringify(receipt));
    } catch {
      // Storage is an optional recovery hint. It must never block a command.
    }
  }

  clear(commandID?: string): void {
    if (!this.storage) return;
    if (commandID) {
      const current = this.read();
      if (current && current.id !== commandID) return;
    }
    try {
      this.storage.removeItem(storageKey);
    } catch {
      // Best-effort cleanup only; the server remains the source of truth.
    }
  }

  clearForTask(taskPublicID: string): void {
    const current = this.read(taskPublicID);
    if (current) this.clear(current.id);
  }
}

export const taskCommandStore = new TaskCommandStore();

function browserStorage(): StorageLike | undefined {
  try {
    return typeof window === "undefined" ? undefined : window.localStorage;
  } catch {
    return undefined;
  }
}

function isTaskCommandReceipt(value: unknown): value is TaskCommandReceipt {
  if (!value || typeof value !== "object") return false;
  const candidate = value as Partial<TaskCommandReceipt>;
  return typeof candidate.id === "string"
    && (candidate.action === "receive" || candidate.action === "start" || candidate.action === "complete")
    && typeof candidate.taskPublicID === "string"
    && typeof candidate.assignmentPublicID === "string"
    && typeof candidate.expectedSyncVersion === "number"
    && (candidate.status === undefined || isCommandStatus(candidate.status));
}

function isCommandStatus(value: unknown): value is CommandStatus {
  return value === "pending" || value === "syncing" || value === "confirmed" || value === "failed";
}
