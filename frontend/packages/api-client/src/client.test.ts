import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient, ApiClientError, CoreApiClient } from "./index";

describe("ApiClient", () => {
  afterEach(() => vi.restoreAllMocks());

  it("declares UTF-8 for JSON request bodies and preserves UTF-8 responses", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ message: "你好" }), {
      status: 200,
      headers: { "Content-Type": "application/json; charset=utf-8" },
    }));
    const client = new ApiClient({ baseUrl: "http://edge.test" });

    const result = await client.request<{ message: string }>("/echo", {
      method: "POST",
      body: JSON.stringify({ message: "你好" }),
    });

    expect(result).toEqual({ message: "你好" });
    const [, init] = fetchMock.mock.calls[0] as [RequestInfo | URL, RequestInit | undefined];
    const headers = new Headers(init?.headers);
    expect(fetchMock).toHaveBeenCalledWith("http://edge.test/echo", expect.anything());
    expect(headers.get("Accept")).toBe("application/json");
    expect(headers.get("Content-Type")).toBe("application/json; charset=utf-8");
  });

  it("maps an API error envelope and invokes the unauthorized hook for 401", async () => {
    const onUnauthorized = vi.fn();
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ code: "session_expired", message: "会话已失效" }), {
      status: 401,
      headers: { "Content-Type": "application/json; charset=utf-8" },
    }));
    const client = new ApiClient({ baseUrl: "http://edge.test", onUnauthorized });

    await expect(client.request("/api/v1/tasks")).rejects.toMatchObject({ status: 401, body: { code: "session_expired" } });
    expect(onUnauthorized).toHaveBeenCalledOnce();
  });

  it("returns a stable invalid-json error when an unavailable service is not JSON", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("upstream unavailable", {
      status: 503,
      headers: { "Content-Type": "text/plain; charset=utf-8" },
    }));
    const client = new ApiClient({ baseUrl: "http://edge.test" });

    const error = await client.request("/api/v1/tasks").catch((value: unknown) => value);

    expect(error).toBeInstanceOf(ApiClientError);
    expect(error).toMatchObject({ status: 502, body: { code: "invalid_json" } });
  });

  it("serializes management filters and write payloads through the Core client", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: { items: [], page: 1, page_size: 20, total: 0 } }), { status: 200, headers: { "Content-Type": "application/json" } }));
    const client = new CoreApiClient(new ApiClient({ baseUrl: "http://core.test", getAccessToken: () => "core-token" }));

    await client.listTeams("area/1", true, undefined, 2, 20, "行李");

    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://core.test/api/v1/teams?area_public_id=area%2F1&include_disabled=true&q=%E8%A1%8C%E6%9D%8E&page=2&page_size=20");
    const [, init] = fetchMock.mock.calls[0] as [RequestInfo | URL, RequestInit | undefined];
    expect(new Headers(init?.headers).get("Authorization")).toBe("Bearer core-token");
  });
});
