import { afterEach, describe, expect, it, vi } from "vitest";

describe("development fixture routes in production", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("returns the frozen 404 FIXTURE_MISSING error outside development", async () => {
    vi.stubEnv("NODE_ENV", "production");

    const incidents = await import("../../src/app/api/dev/v1/incidents/route");
    const list = await incidents.GET();
    expect(list.status).toBe(404);
    expect((await list.json()).error.code).toBe("FIXTURE_MISSING");

    const detail = await import("../../src/app/api/dev/v1/incidents/[incidentId]/route");
    const single = await detail.GET(
      new Request("http://localhost:3000/api/dev/v1/incidents/inc-8271"),
      { params: Promise.resolve({ incidentId: "inc-8271" }) },
    );
    expect(single.status).toBe(404);
    expect((await single.json()).error.code).toBe("FIXTURE_MISSING");

    const reset = await import("../../src/app/api/dev/v1/demo/reset/route");
    const resetResponse = await reset.POST(
      new Request("http://localhost:3000/api/dev/v1/demo/reset", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ scenario_id: "checkout_duplicate_effect" }),
      }),
    );
    expect(resetResponse.status).toBe(404);
    expect((await resetResponse.json()).error.code).toBe("FIXTURE_MISSING");
  });
});
