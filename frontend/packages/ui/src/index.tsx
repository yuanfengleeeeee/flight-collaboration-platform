import type { CommandStatus, TaskStatus } from "@flight/contracts";
import { commandStatusLabel, taskStatusLabel } from "@flight/task-domain";
import type { ReactElement, ReactNode } from "react";

export function StatusBadge({ status }: { status: TaskStatus }): ReactElement {
  return <span className={`status-badge status-${status}`}><span aria-hidden="true">●</span>{taskStatusLabel[status]}</span>;
}

export function CommandBadge({ status }: { status: CommandStatus }): ReactElement {
  return <span className={`command-badge command-${status}`}><span aria-hidden="true">●</span>{commandStatusLabel[status]}</span>;
}

export function Notice({ tone = "neutral", children }: { tone?: "neutral" | "error" | "success"; children: ReactNode }): ReactElement {
  return <div className={`notice notice-${tone}`} role={tone === "error" ? "alert" : "status"}>{children}</div>;
}
