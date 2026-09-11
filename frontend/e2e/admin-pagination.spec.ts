import { expect, test } from "@playwright/test";

const coreBaseURL = process.env.E2E_CORE_API_URL ?? "http://127.0.0.1:8081";
const adminBaseURL = process.env.E2E_ADMIN_BASE_URL ?? "http://127.0.0.1:4174";

const paginatedResources = [
  "/api/v1/areas?include_disabled=true",
  "/api/v1/teams?include_disabled=true",
  "/api/v1/positions?include_disabled=true",
  "/api/v1/capabilities?include_disabled=true",
  "/api/v1/personnel",
  "/api/v1/personnel/status",
  "/api/v1/templates?include_disabled=true",
  "/api/v1/flights?include_terminal=true",
  "/api/v1/tasks",
  "/api/v1/assignments",
  "/api/v1/exceptions",
  "/api/v1/events",
  "/api/v1/audit",
  "/api/v1/admin/identities?include_disabled=true",
] as const;

test("管理端大数据接口保持统一分页契约", async ({ request }) => {
  test.skip(process.env.E2E_LARGE_DATA !== "true", "设置 E2E_LARGE_DATA=true 后运行 100+ 条数据分页验收");

  for (const resource of paginatedResources) {
    const first = await request.get(withPagination(resource, 1), { headers: actorHeaders() });
    await expectPaginated(first, resource, 1);
    const firstBody = await first.json() as PaginatedBody;
    if (firstBody.data.total <= firstBody.data.page_size) continue;

    const second = await request.get(withPagination(resource, 2), { headers: actorHeaders() });
    await expectPaginated(second, resource, 2);
    const secondBody = await second.json() as PaginatedBody;
    expect(secondBody.data.items.map((item) => item.public_id)).not.toEqual(firstBody.data.items.map((item) => item.public_id));
  }
});

test("管理端任务工作台按服务端页码切换大数据", async ({ page, request }) => {
  test.skip(process.env.E2E_LARGE_DATA !== "true", "设置 E2E_LARGE_DATA=true 后运行管理端翻页验收");
  test.skip(process.env.E2E_ADMIN_UI !== "true", "设置 E2E_ADMIN_UI=true 后运行管理端浏览器翻页验收");

  const probe = await request.get(withPagination("/api/v1/tasks", 1), { headers: actorHeaders() });
  await expectPaginated(probe, "/api/v1/tasks", 1);
  const probeBody = await probe.json() as PaginatedBody;
  test.skip(probeBody.data.total <= 20, "当前任务数据不足 21 条，无法验证第二页");

  const firstPage = page.waitForResponse((response) => response.url().includes("/core-api/api/v1/tasks?page=1&page_size=20"));
  await page.goto(`${adminBaseURL}/tasks`);
  await firstPage;
  await expect(page.locator(".list-pager")).toContainText(`共 ${probeBody.data.total} 条`);

  const nextPage = page.waitForResponse((response) => response.url().includes("/core-api/api/v1/tasks?page=2&page_size=20"));
  await page.locator(".list-pager button", { hasText: "下一页" }).click();
  await nextPage;
  await expect(page.locator(".list-pager")).toContainText("第 2 /");

  const lastPageNumber = Math.ceil(probeBody.data.total / 20);
  const lastPage = page.waitForResponse((response) => response.url().includes(`/core-api/api/v1/tasks?page=${lastPageNumber}&page_size=20`));
  await page.locator(".list-pager button", { hasText: "末页" }).click();
  await lastPage;
  await expect(page.locator(".list-pager")).toContainText(`第 ${lastPageNumber} /`);

  const firstPageAgain = page.waitForResponse((response) => response.url().includes("/core-api/api/v1/tasks?page=1&page_size=20"));
  await page.locator(".list-pager button", { hasText: "首页" }).click();
  await firstPageAgain;
  await expect(page.locator(".list-pager")).toContainText("第 1 /");
});

type PaginatedBody = {
  data: {
    items: Array<{ public_id?: string; id?: number }>;
    page: number;
    page_size: number;
    total: number;
  };
};

function withPagination(resource: string, page: number): string {
  return `${coreBaseURL}${resource}${resource.includes("?") ? "&" : "?"}page=${page}&page_size=20`;
}

function actorHeaders(): Record<string, string> {
  return {
    Accept: "application/json",
    "X-Actor-Type": "human",
    "X-Actor-Public-ID": "e2e-manager",
    "X-Actor-Roles": "admin",
    "X-Actor-Global": "true",
  };
}

async function expectPaginated(response: { status(): number; headers(): Record<string, string>; json(): Promise<unknown> }, resource: string, page: number): Promise<void> {
  expect(response.status(), resource).toBe(200);
  expect(response.headers()["content-type"], resource).toContain("application/json");
  expect(response.headers()["content-type"], resource).toContain("utf-8");
  const body = await response.json() as PaginatedBody;
  expect(body.data.page, resource).toBe(page);
  expect(body.data.page_size, resource).toBe(20);
  expect(body.data.total, resource).toBeGreaterThanOrEqual(0);
  expect(Array.isArray(body.data.items), resource).toBe(true);
}
