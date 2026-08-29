import { afterEach, describe, expect, it, vi } from "vitest";

const routeContext = (path: string[]) => ({
  params: Promise.resolve({ path } as { path: string[] }),
});

describe("same-origin Core API proxy", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  async function proxyGet(pathSegments: string[], search = "") {
    const { GET } = await import("../../src/app/v1/[...path]/route");
    return GET(
      new Request(`http://localhost:3000/v1/${pathSegments.join("/")}${search}`, {
        headers: { Accept: "application/json" },
      }),
      routeContext(pathSegments),
    );
  }

  it("forwards same-origin requests to the single configured upstream", async () => {
    vi.stubEnv("CAUSALENS_CORE_API_URL", "http://core-api:8080");
    const upstreamFetch = vi.fn(async () =>
      new Response(JSON.stringify({ items: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", upstreamFetch);

    const response = await proxyGet(["incidents"], "?limit=20");
    const body = await response.json();

    expect(upstreamFetch).toHaveBeenCalledTimes(1);
    const [calledUrl, calledInit] = upstreamFetch.mock.calls[0] as unknown as [string, RequestInit];
    expect(calledUrl).toBe("http://core-api:8080/v1/incidents?limit=20");
    expect(calledInit.method).toBe("GET");
    expect(response.status).toBe(200);
    expect(body).toEqual({ items: [] });
  });

  it("forwards POST bodies to the configured upstream", async () => {
    vi.stubEnv("CAUSALENS_CORE_API_URL", "http://core-api:8080");
    const upstreamFetch = vi.fn(async () =>
      new Response(JSON.stringify({ event_id: "evt-1", status: "ACCEPTED" }), { status: 202 }),
    );
    vi.stubGlobal("fetch", upstreamFetch);

    const { POST } = await import("../../src/app/v1/[...path]/route");
    const response = await POST(
      new Request("http://localhost:3000/v1/events", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ event_id: "evt-1" }),
      }),
      routeContext(["events"]),
    );
    const [, calledInit] = upstreamFetch.mock.calls[0] as unknown as [string, RequestInit];

    expect(calledInit.method).toBe("POST");
    expect(JSON.parse(String(calledInit.body))).toEqual({ event_id: "evt-1" });
    expect(response.status).toBe(202);
  });

  it("fails closed with a frozen error when no upstream is configured", async () => {
    vi.stubEnv("CAUSALENS_CORE_API_URL", "");
    const upstreamFetch = vi.fn();
    vi.stubGlobal("fetch", upstreamFetch);

    const response = await proxyGet(["incidents"]);
    const body = await response.json();

    expect(response.status).toBe(503);
    expect(body.error.code).toBe("INTERNAL_FAILURE");
    expect(body.error.message).toMatch(/CAUSALENS_CORE_API_URL/);
    expect(upstreamFetch).not.toHaveBeenCalled();
  });

  it("rejects proxy paths that attempt to escape the fixed upstream", async () => {
    vi.stubEnv("CAUSALENS_CORE_API_URL", "http://core-api:8080");
    const upstreamFetch = vi.fn();
    vi.stubGlobal("fetch", upstreamFetch);

    for (const segments of [["..", "admin"], ["http:", "evil.test"]]) {
      const response = await proxyGet(segments);
      expect(response.status).toBe(400);
      const body = await response.json();
      expect(body.error.code).toBe("INTERNAL_FAILURE");
    }
    expect(upstreamFetch).not.toHaveBeenCalled();
  });

  it("returns a frozen 502 error when the upstream is unreachable", async () => {
    vi.stubEnv("CAUSALENS_CORE_API_URL", "http://core-api:8080");
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new Error("connection refused");
      }),
    );

    const response = await proxyGet(["incidents"]);
    const body = await response.json();

    expect(response.status).toBe(502);
    expect(body.error.code).toBe("INTERNAL_FAILURE");
  });
});
