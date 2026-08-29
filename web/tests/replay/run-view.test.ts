import { describe, expect, it } from "vitest";

import type { ReplayRun } from "../../src/lib/contracts";
import { baselineRun, whatIfRun } from "../fixtures/golden-contracts";

const completedBaseline = baselineRun as unknown as ReplayRun;

function runOf(status: ReplayRun["status"]): ReplayRun {
  const terminal = ["COMPLETED", "FAILED", "BLOCKED"].includes(status);
  return {
    ...completedBaseline,
    status,
    outcome: terminal && status === "COMPLETED" ? completedBaseline.outcome : undefined,
    effect_summary: status === "COMPLETED" ? completedBaseline.effect_summary : undefined,
    failure_oracle_result: status === "COMPLETED" ? completedBaseline.failure_oracle_result : undefined,
    isolation_evidence: status === "COMPLETED" ? completedBaseline.isolation_evidence : undefined,
    error:
      status === "BLOCKED"
        ? { code: "ISOLATION_VIOLATION", message: "Blocked destination contacted.", retryable: false, details: {} }
        : status === "FAILED"
          ? { code: "INTERNAL_FAILURE", message: "Simulator failed.", retryable: true, details: {} }
          : undefined,
    completed_at: terminal ? "2026-08-29T10:34:01Z" : undefined,
    started_at: status === "CREATED" ? undefined : "2026-08-29T10:34:00Z",
  } as ReplayRun;
}

async function runViewModule() {
  return import("../../src/features/replay/run-view");
}

describe("run view model", () => {
  it("separates replay execution, isolation, oracle, and outcome into distinct rows", async () => {
    const { buildRunView } = await runViewModule();
    const view = buildRunView(completedBaseline);

    expect(view.rows).toEqual([
      { label: "Replay execution", value: "COMPLETED", tone: "completed" },
      { label: "Isolation", value: "PASS", tone: "pass" },
      { label: "Failure oracle", value: "MATCHED", tone: "match" },
      { label: "Baseline outcome", value: "REPRODUCED", tone: "reproduced" },
    ]);
  });

  it("omits evidence rows the API has not provided for active runs", async () => {
    const { buildRunView } = await runViewModule();
    const view = buildRunView(runOf("RUNNING"));

    expect(view.rows).toEqual([{ label: "Replay execution", value: "RUNNING", tone: "active" }]);
    expect(view.lifecycleActive).toBe(true);
    expect(view.isolation).toBeUndefined();
    expect(view.effectSummary).toBeUndefined();
  });

  it("carries errors for BLOCKED and FAILED runs without an outcome", async () => {
    const { buildRunView } = await runViewModule();

    const blocked = buildRunView(runOf("BLOCKED"));
    expect(blocked.rows.some((row) => row.label === "Baseline outcome")).toBe(false);
    expect(blocked.error).toMatchObject({ code: "ISOLATION_VIOLATION" });
    expect(blocked.lifecycleActive).toBe(false);

    const failed = buildRunView(runOf("FAILED"));
    expect(failed.error).toMatchObject({ code: "INTERNAL_FAILURE", retryable: true });
  });

  it("exposes isolation interaction evidence when the API provides it", async () => {
    const { buildRunView } = await runViewModule();
    const view = buildRunView(completedBaseline);

    expect(view.isolation).toMatchObject({
      verdict: "PASS",
      networkPolicy: "PASS",
      runtimeNamespace: "replay-run-base-8271",
      credentialProfile: "replay-only",
      teardownResult: "PASS",
    });
    expect(view.isolation?.simulatorInteractions).toEqual([
      {
        dependency: "payment_simulator",
        destination: "http://payment-simulator:8080",
        operation: "authorize",
        result: "SIMULATED",
      },
    ]);
    expect(view.isolation?.deniedInteractions).toEqual([]);
    expect(view.effectSummary).toEqual({ paymentAttemptCount: 2, ledgerCommitCount: 2 });
    expect(view.oracle?.matched).toBe(true);
    expect(view.oracle?.explanation).toContain("reproduced");
  });

  it("labels what-if outcomes separately from baseline outcomes", async () => {
    const { buildRunView } = await runViewModule();
    const view = buildRunView(whatIfRun as unknown as ReplayRun);

    expect(view.rows).toContainEqual({
      label: "What-if outcome",
      value: "MITIGATED",
      tone: "mitigated",
    });
    expect(view.rows.some((row) => row.label === "Baseline outcome")).toBe(false);
  });
});
