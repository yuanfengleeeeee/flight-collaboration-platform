import { expect, test } from "@playwright/test";

const roleMatrixEnabled = process.env.E2E_ROLE_MATRIX === "true";
const roleURLs = {
  admin: process.env.E2E_ADMIN_ROLE_URL ?? "http://127.0.0.1:4176",
  manager: process.env.E2E_MANAGER_ROLE_URL ?? "http://127.0.0.1:4174",
  leader: process.env.E2E_LEADER_ROLE_URL ?? "http://127.0.0.1:4178",
} as const;

const allModules = [
  "运行总览", "航班运行", "任务工作台", "任务模板", "任务分配", "任务历史",
  "组织与班组", "人员档案", "岗位与能力", "人员状态", "规则配置", "事件与通知",
  "异常处理", "审计日志", "报表统计", "用户与角色", "权限范围", "开发诊断",
] as const;

test("管理端角色矩阵控制导航、直接路由和主数据写入口", async ({ browser }) => {
  test.skip(!roleMatrixEnabled, "设置 E2E_ROLE_MATRIX=true，并提供三个角色的临时 Web 实例后运行");

  const expectations = {
    admin: { visible: allModules, hidden: [], scope: "全部团队 · 全部区域" },
    manager: { visible: allModules.filter((item) => item !== "用户与角色"), hidden: ["用户与角色"], scope: "全部团队 · 全部区域" },
    leader: {
      visible: allModules.filter((item) => !["审计日志", "用户与角色", "开发诊断"].includes(item)),
      hidden: ["审计日志", "用户与角色", "开发诊断"],
      scope: "当前团队 / 区域",
    },
  } as const;

  for (const role of ["admin", "manager", "leader"] as const) {
    const page = await browser.newPage();
    await page.goto(`${roleURLs[role]}/tasks`);
    await expect(page.locator(".scope-card")).toContainText(expectations[role].scope);
    const navigation = page.getByRole("navigation", { name: "管理端工作区" });
    for (const label of expectations[role].visible) await expect(navigation.getByRole("link", { name: label, exact: true })).toBeVisible();
    for (const label of expectations[role].hidden) await expect(navigation.getByRole("link", { name: label, exact: true })).toHaveCount(0);

    await page.goto(`${roleURLs[role]}/organization`);
    if (role === "admin") {
      await expect(page.getByText("新增运行区域", { exact: true })).toBeVisible();
      await expect(page.getByText("新增班组", { exact: true })).toBeVisible();
      await expect(page.getByRole("button", { name: "编辑", exact: true }).first()).toBeVisible();
    } else {
      await expect(page.getByText("新增运行区域", { exact: true })).toHaveCount(0);
      await expect(page.getByText("新增班组", { exact: true })).toHaveCount(0);
      await expect(page.getByRole("button", { name: "编辑", exact: true })).toHaveCount(0);
    }

    if (role === "admin") {
      await page.goto(`${roleURLs.admin}/users-roles`);
      await expect(page.getByRole("heading", { name: "用户与角色", exact: true })).toBeVisible();
      await expect(page.getByRole("button", { name: "创建身份", exact: true })).toBeVisible();
      await expect(page.locator("body")).not.toBeEmpty();
    } else {
      await page.goto(`${roleURLs[role]}/users-roles`);
      await expect(page.getByRole("heading", { name: "当前角色没有访问权限", exact: true })).toBeVisible();
    }

    if (role === "leader") {
      await page.goto(`${roleURLs.leader}/personnel`);
      await expect(page.getByRole("heading", { name: "人员档案", exact: true })).toBeVisible();
      await expect(page.getByRole("alert")).toHaveCount(0);
      await expect(page.getByText("新增员工 / 保障人员", { exact: true })).toHaveCount(0);
      await page.goto(`${roleURLs.leader}/audit`);
      await expect(page.getByRole("heading", { name: "当前角色没有访问权限", exact: true })).toBeVisible();
      await page.goto(`${roleURLs.leader}/diagnostics`);
      await expect(page.getByRole("heading", { name: "当前角色没有访问权限", exact: true })).toBeVisible();
    }

    await page.close();
  }
});
