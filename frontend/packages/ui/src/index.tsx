import type { CommandStatus, TaskStatus } from "@flight/contracts";
import { commandStatusLabel, taskStatusLabel } from "@flight/task-domain";
import { useEffect } from "react";
import type { ReactElement, ReactNode } from "react";

export function StatusBadge({ status }: { status: TaskStatus | string }): ReactElement {
  const label = taskStatusLabel[status as TaskStatus] ?? status;
  return <span className={`status-badge status-${status}`}><span aria-hidden="true">●</span>{label}</span>;
}

export function CommandBadge({ status }: { status: CommandStatus }): ReactElement {
  return <span className={`command-badge command-${status}`}><span aria-hidden="true">●</span>{commandStatusLabel[status]}</span>;
}

export function Notice({ tone = "neutral", children }: { tone?: "neutral" | "error" | "success"; children: ReactNode }): ReactElement {
  return <div className={`notice notice-${tone}`} role={tone === "error" ? "alert" : "status"}>{children}</div>;
}

export function ListPager({ page, pageSize, total, onPageChange, disabled = false }: { page: number; pageSize: number; total: number; onPageChange: (page: number) => void; disabled?: boolean }): ReactElement {
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const safePage = Math.min(Math.max(page, 1), totalPages);
  useEffect(() => {
    if (page !== safePage) onPageChange(safePage);
  }, [onPageChange, page, safePage]);
  return <footer className="list-pager" aria-label="分页">
    <span aria-live="polite">第 {safePage} / {totalPages} 页 · 共 {total} 条</span>
    <span className="list-pager__actions">
      <button type="button" disabled={disabled || safePage <= 1} onClick={() => onPageChange(1)}>首页</button>
      <button type="button" disabled={disabled || safePage <= 1} onClick={() => onPageChange(safePage - 1)}>上一页</button>
      <button type="button" disabled={disabled || safePage >= totalPages} onClick={() => onPageChange(safePage + 1)}>下一页</button>
      <button type="button" disabled={disabled || safePage >= totalPages} onClick={() => onPageChange(totalPages)}>末页</button>
    </span>
  </footer>;
}
