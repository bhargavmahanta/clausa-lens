import { afterEach, describe, expect, it, vi } from "vitest";

afterEach(() => vi.unstubAllEnvs());

describe("development fixture routes", () => {
  it("serve contract resources in development", async () => {
    vi.stubEnv("NODE_ENV", "development");
    const { GET: listIncidents } = await import(
      "../../src/app/api/dev/v1/incidents/route"
    );
    const { GET: getIncident } = await import(
      "../../src/app/api/dev/v1/incidents/[incidentId]/route"
    );

    const listResponse = await listIncidents();
    const detailResponse = await getIncident(new Request("http://localhost"), {
      params: Promise.resolve({ incidentId: "inc-8271" }),
    });

    expect(listResponse.status).toBe(200);
    expect(detailResponse.status).toBe(200);
    await expect(listResponse.json()).resolves.toMatchObject({
      items: [{ incident_id: "inc-8271" }],
    });
    await expect(detailResponse.json()).resolves.toMatchObject({
      incident: { incident_id: "inc-8271" },
    });
  });

  it("does not expose fixture resources outside development", async () => {
    vi.stubEnv("NODE_ENV", "production");
    const { GET } = await import("../../src/app/api/dev/v1/incidents/route");

    expect((await GET()).status).toBe(404);
  });
});
