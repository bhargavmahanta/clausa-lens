import { describe, expect, it } from "vitest";

import {
  baselineRun,
  capsule,
  graph,
  incident,
  replayDiff,
  resetResult,
  whatIfRun,
} from "../fixtures/golden-contracts";

describe("API resource contract v1.0", () => {
  it("decodes the complete golden workflow resources", async () => {
    const contracts = (await import("../../src/lib/contracts")) as Record<string, unknown>;
    const requiredSchemas = [
      "incidentSchema",
      "executionGraphSchema",
      "replayCapsuleSchema",
      "replayRunSchema",
      "replayDiffSchema",
      "resetResultSchema",
    ] as const;

    for (const schemaName of requiredSchemas) {
      expect(contracts[schemaName], `${schemaName} must be exported`).toBeDefined();
    }

    if (requiredSchemas.some((schemaName) => !contracts[schemaName])) return;

    const parse = (schemaName: (typeof requiredSchemas)[number], value: unknown) =>
      (contracts[schemaName] as { safeParse: (input: unknown) => { success: boolean } }).safeParse(value).success;

    expect(parse("incidentSchema", incident)).toBe(true);
    expect(parse("executionGraphSchema", graph)).toBe(true);
    expect(parse("replayCapsuleSchema", capsule)).toBe(true);
    expect(parse("replayRunSchema", baselineRun)).toBe(true);
    expect(parse("replayRunSchema", whatIfRun)).toBe(true);
    expect(parse("replayDiffSchema", replayDiff)).toBe(true);
    expect(parse("resetResultSchema", resetResult)).toBe(true);
  });

  it("rejects replay runs whose lifecycle fields contradict their status", async () => {
    const { replayRunSchema } = await import("../../src/lib/contracts");
    const invalidRuns = [
      { ...baselineRun, status: "RUNNING" },
      { ...baselineRun, isolation_evidence: undefined },
      {
        ...baselineRun,
        intervention: { type: "PAYMENT_LATENCY", from: 350, to: 50, unit: "ms" },
      },
      {
        ...whatIfRun,
        baseline_run_id: undefined,
      },
    ];

    for (const invalidRun of invalidRuns) {
      expect(replayRunSchema.safeParse(invalidRun).success).toBe(false);
    }
  });
});
