import { describe, expect, it } from "vitest";
import type { EdgeTaskProjection } from "@flight/contracts";
import { canComplete, canReceive, canStart, createClientID, projectionStatus, suggestedChangeActionForExceptionCategory } from "./index";

const projection: EdgeTaskProjection = {
  public_id: "task-1",
  employee_public_id: "staff-1",
  flight_display_no: "MU2156",
  task_name: "国际航班值机准备",
  area_name: "T2",
  planned_at: "2026-09-02T07:40:00Z",
  business_status: "assigned",
  message: "完成国际航班值机准备",
  sync_version: 1,
  updated_at: "2026-09-02T07:30:00Z",
};

describe("task display domain", () => {
  it("uses business_status as the source of employee action availability", () => {
    expect(projectionStatus(projection)).toBe("assigned");
    expect(canReceive("assigned")).toBe(true);
    expect(canStart("assigned", "pending")).toBe(false);
    expect(canStart("assigned", "received")).toBe(true);
    expect(canComplete("assigned")).toBe(false);
  });

  it("does not make terminal tasks actionable", () => {
    expect(canReceive("completed")).toBe(false);
    expect(canStart("completed")).toBe(false);
    expect(canComplete("cancelled")).toBe(false);
  });

  it("creates UUID-shaped client keys for API idempotency", () => {
    expect(createClientID()).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i);
  });

  it("only suggests safe exception actions from explicit categories", () => {
    expect(suggestedChangeActionForExceptionCategory("保障冲突（申请重新预分配）")).toBe("reassign");
    expect(suggestedChangeActionForExceptionCategory("航班取消（申请取消任务）")).toBe("cancel");
    expect(suggestedChangeActionForExceptionCategory("航班延误（申请调整任务）")).toBeUndefined();
  });
});
