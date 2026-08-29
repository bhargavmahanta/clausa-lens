import { describe, expect, it } from "vitest";

import { resolveIncidentDataSource } from "../../src/features/incidents/data-source";

describe("incident data source", () => {
  it("uses the isolated fixture endpoint only in development", () => {
    expect(resolveIncidentDataSource({ isDevelopment: true })).toEqual({
      baseUrl: "/api/dev",
      mode: "fixture",
    });
  });

  it("uses the same-origin server proxy in production", () => {
    expect(resolveIncidentDataSource({ isDevelopment: false })).toEqual({
      baseUrl: "/v1",
      mode: "core",
    });
  });

  it("never returns a browser-direct cross-origin base URL", () => {
    for (const isDevelopment of [true, false]) {
      const source = resolveIncidentDataSource({ isDevelopment });
      expect(source.baseUrl.startsWith("http://")).toBe(false);
      expect(source.baseUrl.startsWith("https://")).toBe(false);
    }
  });
});
