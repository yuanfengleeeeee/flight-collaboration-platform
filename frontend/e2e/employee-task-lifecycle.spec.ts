import { expect, test } from "@playwright/test";

const employeeNo = process.env.E2E_LIFECYCLE_EMPLOYEE_NO ?? process.env.E2E_EMPLOYEE_NO;
const password = process.env.E2E_LIFECYCLE_EMPLOYEE_PASSWORD ?? process.env.E2E_EMPLOYEE_PASSWORD;
const taskPublicID = process.env.E2E_LIFECYCLE_TASK_ID;

test("员工可以在真实 Projection 上完成 Accept/Complete 并在刷新后恢复终态", async ({ page }) => {
  test.skip(!employeeNo || !password || !taskPublicID, "需要设置 E2E_LIFECYCLE_TASK_ID 与员工账号环境变量");

  await page.goto("/login");
  await page.getByLabel("工号", { exact: true }).fill(employeeNo!);
  await page.getByLabel("密码", { exact: true }).fill(password!);
  await page.getByRole("button", { name: "登录并查看任务", exact: true }).click();
  await expect(page).toHaveURL(/\/tasks$/);

  await page.goto("/tasks/" + taskPublicID);
  await expect(page.getByRole("heading", { name: "任务说明", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "接受任务", exact: true })).toBeVisible();

  await page.getByRole("button", { name: "接受任务", exact: true }).click();
  await expect(page.locator(".command-panel")).toContainText(/等待同步|同步中|已同步/);
  await expect(page.locator(".command-panel")).toContainText("已同步", { timeout: 30_000 });
  await expect(page.locator(".detail-flight-strip .status-badge")).toContainText("执行中", { timeout: 30_000 });

  await page.getByRole("button", { name: "标记完成", exact: true }).click();
  await expect(page.locator(".command-panel")).toContainText("已同步", { timeout: 30_000 });
  await expect(page.locator(".detail-flight-strip .status-badge")).toContainText("已完成", { timeout: 30_000 });

  await page.reload();
  await expect(page).toHaveURL(new RegExp("/tasks/" + escapeRegExp(taskPublicID!)));
  await expect(page.locator(".detail-flight-strip .status-badge")).toContainText("已完成");
  await expect(page.getByText("当前状态只读，最终状态由 Core Projection 决定。", { exact: true })).toBeVisible();
});

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^$()|[\]\\]/g, "\\$&");
}
