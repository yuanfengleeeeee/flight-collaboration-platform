import { expect, test } from "@playwright/test";

const coreBaseURL = process.env.E2E_CORE_API_URL ?? "http://127.0.0.1:8081";
const taskPublicID = process.env.E2E_SCOPE_TASK_ID;
const teamID = process.env.E2E_SCOPE_TEAM_ID ?? "1";
const areaID = process.env.E2E_SCOPE_AREA_ID ?? "1";

test("主任看到全部任务，队长只看到自己团队和区域的任务", async ({ request }) => {
  test.skip(!taskPublicID, "需要设置 E2E_SCOPE_TASK_ID");

  const manager = await request.get(coreBaseURL + "/api/v1/tasks?page=1&page_size=50", { headers: actorHeaders("manager", true) });
  expect(manager.status()).toBe(200);
  expect(manager.headers()["content-type"]).toContain("application/json");
  expect(manager.headers()["content-type"]).toContain("utf-8");
  const managerBody = await manager.json() as { data: { items: Array<{ public_id: string }>; total: number } };
  expect(managerBody.data.items.some((item) => item.public_id === taskPublicID)).toBe(true);

  const scopedLeader = await request.get(coreBaseURL + "/api/v1/tasks?page=1&page_size=50", { headers: actorHeaders("leader", false, teamID, areaID) });
  expect(scopedLeader.status()).toBe(200);
  const scopedLeaderBody = await scopedLeader.json() as { data: { items: Array<{ public_id: string }>; total: number } };
  expect(scopedLeaderBody.data.items.some((item) => item.public_id === taskPublicID)).toBe(true);

  const otherLeader = await request.get(coreBaseURL + "/api/v1/tasks?page=1&page_size=50", { headers: actorHeaders("leader", false, "999999", "999999") });
  expect(otherLeader.status()).toBe(200);
  const otherLeaderBody = await otherLeader.json() as { data: { items: Array<{ public_id: string }>; total: number } };
  expect(otherLeaderBody.data.items).toHaveLength(0);
  expect(otherLeaderBody.data.total).toBe(0);

  const hiddenDetail = await request.get(coreBaseURL + "/api/v1/tasks/" + taskPublicID, { headers: actorHeaders("leader", false, "999999", "999999") });
  expect(hiddenDetail.status()).toBe(404);
});

function actorHeaders(role: string, global: boolean, team = "", area = ""): Record<string, string> {
  return {
    "X-Actor-Type": "human",
    "X-Actor-Public-ID": "e2e-" + role,
    "X-Actor-Roles": role,
    "X-Actor-Global": String(global),
    "X-Actor-Team-IDs": team,
    "X-Actor-Area-IDs": area,
  };
}
