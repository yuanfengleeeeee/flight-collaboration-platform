import { describe, expect, it } from "vitest";
import type { EdgeTaskProjection } from "@flight/contracts";
import { canAccept, canComplete, createClientID, projectionStatus } from "./index";

const projection: EdgeTaskProjection = {
  public_id: "task-1",
  employee_public_id: "staff-1",
  flight_display_no: "MU2156",
  task_name: "客舱清洁",
  area_name: "T2",
  planned_at: "2026-09-02T07:40:00Z",
  business_status: "assigned",
  message: "完成客舱清洁",
  sync_version: 1,
  updated_at: "2026-09-02T07:30:00Z",
};

describe("task display domain", () => {
  it("uses business_status as the source of employee action availability", () => {
    expect(projectionStatus(projection)).toBe("assigned");
    expect(canAccept("assigned")).toBe(true);
    expect(canComplete("assigned")).toBe(false);
  });

  it("does not make terminal tasks actionable", () => {
    expect(canAccept("completed")).toBe(false);
    expect(canComplete("cancelled")).toBe(false);
  });

  it("creates UUID-shaped client keys for API idempotency", () => {
    expect(createClientID()).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i);
  });
});
