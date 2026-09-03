import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient, ApiClientError } from "./index";

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
});
