import { afterEach, describe, expect, it, vi } from "vitest";

import { createCausaLensClient } from "../../src/lib/api";
import { resolveIncidentDataSource } from "../../src/features/incidents/data-source";

function proxyContext(path: string[]) {
  return { params: Promise.resolve({ path } as { path: string[] }) };
}

describe("production request boundary", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("sends exactly one /v1 prefix from the browser through the proxy to Core", async () => {
    vi.stubEnv("CAUSALENS_CORE_API_URL", "http://core-api:8080");
    const browserRequests: string[] = [];
    const upstreamRequests: string[] = [];

    vi.stubGlobal("fetch", async (input: RequestInfo | URL, init?: RequestInit) => {
      const url =
        typeof input === "string" ? input : input instanceof URL ? input.href : input.url;

      if (/^https?:\/\//.test(url)) {
        upstreamRequests.push(url);
        return new Response(JSON.stringify({ items: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }

      browserRequests.push(url);
      const pathname = new URL(`http://site.test${url}`).pathname;
      const segments = pathname.split("/").filter(Boolean).slice(1);
      const { GET } = await import("../../src/app/v1/[...path]/route");
      return GET(
        new Request(`http://site.test${url}`, init),
        proxyContext(segments),
      );
    });

    const source = resolveIncidentDataSource({ isDevelopment: false });
    expect(source.baseUrl).toBe("");

    const client = createCausaLensClient({ baseUrl: source.baseUrl });
    const response = await client.listIncidents({ limit: 100 });

    expect(browserRequests).toEqual(["/v1/incidents?limit=100"]);
    expect(upstreamRequests).toEqual(["http://core-api:8080/v1/incidents?limit=100"]);
    expect(response.items).toEqual([]);
  });
});
