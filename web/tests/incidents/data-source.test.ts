import { describe, expect, it } from "vitest";

import { resolveIncidentDataSource } from "../../src/features/incidents/data-source";

describe("incident data source", () => {
  it("uses the configured Core API whenever one is provided", () => {
    expect(
      resolveIncidentDataSource({
        configuredBaseUrl: "https://core.example.test/",
        isDevelopment: true,
      }),
    ).toEqual({ baseUrl: "https://core.example.test", mode: "core" });
  });

  it("uses the isolated fixture endpoint only in development", () => {
    expect(
      resolveIncidentDataSource({ isDevelopment: true }),
    ).toEqual({ baseUrl: "/api/dev", mode: "fixture" });

    expect(
      resolveIncidentDataSource({ isDevelopment: false }),
    ).toEqual({ baseUrl: "http://localhost:8080", mode: "core" });
  });
});
