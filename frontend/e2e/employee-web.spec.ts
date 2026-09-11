import { expect, test, type Page } from "@playwright/test";

const employeeNo = process.env.E2E_EMPLOYEE_NO;
const password = process.env.E2E_EMPLOYEE_PASSWORD;
const expectedFlight = process.env.E2E_EXPECTED_FLIGHT ?? "FL-LIVE-001";
const expectedTaskName = process.env.E2E_EXPECTED_TASK_NAME ?? "联调到达保障任务";
const expectedAreaName = process.env.E2E_EXPECTED_AREA_NAME ?? "联调运行区";

test("员工 Web 可以登录并读取 UTF-8 Edge Projection", async ({ page }) => {
  test.skip(!employeeNo || !password, "需要设置 E2E_EMPLOYEE_NO 和 E2E_EMPLOYEE_PASSWORD");

  const realtimeReady = waitForRealtimeReady(page);
  await page.goto("/login");
  await expect(page.getByRole("heading", { name: "进入我的任务", exact: true })).toBeVisible();
  await page.getByLabel("工号", { exact: true }).fill(employeeNo!);
  await page.getByLabel("密码", { exact: true }).fill(password!);
  await page.getByRole("button", { name: "登录并查看任务", exact: true }).click();

  await expect(page).toHaveURL(/\/tasks$/);
  await expect(page.getByRole("banner")).toContainText("staff");
  await expect(page.getByRole("status")).toContainText("已连接 Edge");
  await expect(page.getByRole("heading", { name: "我的任务", exact: true })).toBeVisible();
  await expect(page.getByText(expectedFlight, { exact: true })).toBeVisible();
  const expectedTaskCard = page.locator(".employee-task-card").filter({ hasText: expectedFlight });
  await expect(expectedTaskCard.getByRole("heading", { name: expectedTaskName, exact: true })).toBeVisible();
  await expect(expectedTaskCard.getByText(new RegExp(`${escapeRegExp(expectedAreaName)} · 计划`))).toBeVisible();
  await realtimeReady;

  await page.reload();
  await expect(page).toHaveURL(/\/tasks$/);
  await expect(page.getByRole("heading", { name: "我的任务", exact: true })).toBeVisible();
  await expect(page.locator(".employee-task-card").filter({ hasText: expectedFlight }).getByRole("heading", { name: expectedTaskName, exact: true })).toBeVisible();
});

function waitForRealtimeReady(page: Page): Promise<void> {
  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => reject(new Error("员工 Web 未在 10 秒内收到 WebSocket ready 帧")), 10_000);
    page.on("websocket", (socket) => {
      if (!socket.url().includes("/api/v1/ws")) return;
      socket.on("framereceived", (data) => {
        const frame = decodeWebSocketFrame(data.payload);
        if (!frame.includes('"type":"ready"')) return;
        clearTimeout(timeout);
        resolve();
      });
    });
  });
}

function decodeWebSocketFrame(data: unknown): string {
  if (typeof data === "string") return data;
  if (data instanceof Uint8Array) return new TextDecoder("utf-8").decode(data);
  if (typeof data === "object" && data !== null && "data" in data) {
    const bytes = (data as { data?: unknown }).data;
    if (Array.isArray(bytes)) return new TextDecoder("utf-8").decode(Uint8Array.from(bytes.filter((value): value is number => typeof value === "number")));
  }
  return String(data);
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
