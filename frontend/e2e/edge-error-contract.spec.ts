import { expect, test } from "@playwright/test";

const edgeBaseURL = process.env.E2E_EDGE_API_URL ?? "http://127.0.0.1:8082";
const employeeNo = process.env.E2E_LIFECYCLE_EMPLOYEE_NO ?? process.env.E2E_EMPLOYEE_NO;
const password = process.env.E2E_LIFECYCLE_EMPLOYEE_PASSWORD ?? process.env.E2E_EMPLOYEE_PASSWORD;

test("Edge 对 401/403/404/409 返回稳定 UTF-8 错误契约", async ({ request }) => {
  test.skip(!employeeNo || !password, "需要设置员工账号环境变量");

  const unauthenticated = await request.get(edgeBaseURL + "/api/v1/tasks");
  await expectApiError(unauthenticated, 401, "unauthenticated");

  const login = await request.post(edgeBaseURL + "/api/v1/auth/password/login", { data: utf8JSON({ employee_no: employeeNo, password, client: "employee-web" }) });
  expect(login.status()).toBe(200);
  const loginBody = await login.json() as { data: { access_token: string; principal: { public_id: string } } };
  const accessToken = loginBody.data.access_token;
  const authHeaders = { Authorization: "Bearer " + accessToken };

  const missingCommand = await request.get(edgeBaseURL + "/api/v1/commands/00000000-0000-4000-8000-000000000404", { headers: authHeaders });
  await expectApiError(missingCommand, 404, "command_not_found");

  const forbidden = await request.post(edgeBaseURL + "/api/v1/commands", {
    headers: { ...authHeaders, "Content-Type": "application/json; charset=utf-8" },
    data: utf8JSON(commandEnvelope("00000000-0000-4000-8000-000000000403", "different-employee", "payload-403")),
  });
  await expectApiError(forbidden, 403, "forbidden");

  const commandID = crypto.randomUUID();
  const first = await request.post(edgeBaseURL + "/api/v1/commands", {
    headers: { ...authHeaders, "Content-Type": "application/json; charset=utf-8" },
    data: utf8JSON(commandEnvelope(commandID, loginBody.data.principal.public_id, "payload-409-a")),
  });
  expect(first.status()).toBe(202);

  const conflict = await request.post(edgeBaseURL + "/api/v1/commands", {
    headers: { ...authHeaders, "Content-Type": "application/json; charset=utf-8" },
    data: utf8JSON(commandEnvelope(commandID, loginBody.data.principal.public_id, "payload-409-b")),
  });
  await expectApiError(conflict, 409, "command_id_conflict");
});

async function expectApiError(response: { status(): number; headers(): Record<string, string>; json(): Promise<unknown> }, status: number, code: string): Promise<void> {
  expect(response.status()).toBe(status);
  expect(response.headers()["content-type"]).toContain("application/json");
  expect(response.headers()["content-type"]).toContain("utf-8");
  expect(response.json()).resolves.toMatchObject({ code });
}

function utf8JSON(value: unknown): Buffer {
  return Buffer.from(JSON.stringify(value), "utf8");
}

function commandEnvelope(commandID: string, actorPublicID: string, marker: string): Record<string, unknown> {
  return {
    command_id: commandID,
    command_type: "employee_accept_task.v1",
    schema_version: 1,
    actor_public_id: actorPublicID,
    aggregate_id: "00000000-0000-4000-8000-000000000001",
    occurred_at: new Date().toISOString(),
    trace_id: crypto.randomUUID(),
    payload: { marker },
  };
}
