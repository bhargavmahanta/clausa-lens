import { describe, expect, it } from "vitest";

import {
  baselineRun,
  capsule,
  capturedEvent,
  graph,
  incident,
  replayDiff,
  retryEvent,
} from "../fixtures/golden-contracts";

describe("frozen contract conformance regressions", () => {
  it("requires UTC timestamps while retaining nanosecond precision", async () => {
    const { executionEventSchema } = await import("../../src/lib/contracts");

    expect(
      executionEventSchema.safeParse({
        ...capturedEvent,
        occurred_at: "2026-08-29T16:02:01.015+05:30",
      }).success,
    ).toBe(false);
    expect(
      executionEventSchema.safeParse({
        ...capturedEvent,
        occurred_at: "2026-08-29T10:32:01.015123456Z",
      }).success,
    ).toBe(true);
  });

  it("uses the SemVer grammar and allows contract strings to be empty", async () => {
    const { incidentSchema } = await import("../../src/lib/contracts");

    expect(
      incidentSchema.safeParse({
        ...incident,
        summary: "",
        system_pack: { ...incident.system_pack, version: "1.0.0-01" },
      }).success,
    ).toBe(false);
    expect(incidentSchema.safeParse({ ...incident, summary: "" }).success).toBe(true);
  });

  it("rejects null where lifecycle fields must be omitted", async () => {
    const { createRunRequestSchema } = await import("../../src/lib/contracts");

    expect(
      createRunRequestSchema.safeParse({
        run_type: "BASELINE",
        baseline_run_id: null,
        intervention: null,
      }).success,
    ).toBe(false);
  });

  it("rejects graph cycles and hard edges that contradict timeline order", async () => {
    const { executionGraphSchema } = await import("../../src/lib/contracts");
    const cyclicGraph = {
      ...graph,
      edges: [
        { edge_id: "edge-a", from_event_id: "evt-payment-1-start", to_event_id: "evt-retry", type: "RETRY" },
        { edge_id: "edge-b", from_event_id: "evt-retry", to_event_id: "evt-payment-1-start", type: "DEPENDENCY" },
      ],
    };

    expect(executionGraphSchema.safeParse(cyclicGraph).success).toBe(false);
  });

  it("freezes intervention bounds and isolation-denial semantics", async () => {
    const { interventionSpecSchema, isolationEvidenceSchema } = await import("../../src/lib/contracts");

    expect(
      interventionSpecSchema.safeParse({
        type: "PAYMENT_LATENCY",
        value_type: "INTEGER",
        unit: "ms",
        minimum: 1,
        maximum: 5000,
      }).success,
    ).toBe(false);
    expect(
      isolationEvidenceSchema.safeParse({
        ...baselineRun.isolation_evidence,
        simulator_interactions: [
          {
            dependency: "payment_simulator",
            destination: "http://payment-simulator:8080",
            operation: "authorize",
            result: "DENIED",
          },
        ],
      }).success,
    ).toBe(false);
  });

  it("requires the capsule replay plan to load every fixture exactly once", async () => {
    const { replayCapsuleSchema } = await import("../../src/lib/contracts");

    expect(
      replayCapsuleSchema.safeParse({
        ...capsule,
        replay_plan: {
          ...capsule.replay_plan,
          fixture_load_order: ["state-ledger-empty", "unknown-fixture"],
        },
      }).success,
    ).toBe(false);
  });

  it("rejects replay outcomes that contradict oracle evidence", async () => {
    const { replayRunSchema } = await import("../../src/lib/contracts");

    expect(
      replayRunSchema.safeParse({
        ...baselineRun,
        failure_oracle_result: { ...baselineRun.failure_oracle_result, matched: false },
      }).success,
    ).toBe(false);
    expect(replayRunSchema.safeParse({ ...baselineRun, started_at: undefined }).success).toBe(false);
  });

  it("requires isolation evidence when an isolation policy blocks a run", async () => {
    const { replayRunSchema } = await import("../../src/lib/contracts");
    const blockedRun = {
      ...baselineRun,
      status: "BLOCKED",
      outcome: undefined,
      isolation_evidence: undefined,
      error: {
        code: "ISOLATION_VIOLATION",
        message: "Replay isolation failed.",
        retryable: false,
        details: {},
      },
    };

    expect(replayRunSchema.safeParse(blockedRun).success).toBe(false);
  });

  it("verifies ReplayDiff arithmetic and reproduced-baseline evidence", async () => {
    const { replayDiffSchema } = await import("../../src/lib/contracts");

    expect(
      replayDiffSchema.safeParse({
        ...replayDiff,
        effect_delta: { payment_attempt_count: 1, ledger_commit_count: 1 },
      }).success,
    ).toBe(false);
    expect(
      replayDiffSchema.safeParse({
        ...replayDiff,
        baseline_oracle_result: { ...replayDiff.baseline_oracle_result, matched: false },
      }).success,
    ).toBe(false);
  });

  it("requires incident events to follow graph timeline order", async () => {
    const { incidentDetailResponseSchema } = await import("../../src/lib/contracts");
    const response = { incident, graph, events: [retryEvent, capturedEvent] };

    expect(incidentDetailResponseSchema.safeParse(response).success).toBe(false);
    expect(incidentDetailResponseSchema.safeParse({ ...response, events: [capturedEvent, retryEvent] }).success).toBe(true);
  });
});
