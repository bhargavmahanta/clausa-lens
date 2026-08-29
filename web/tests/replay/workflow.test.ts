import { describe, expect, it } from "vitest";

import type { ReplayCapsule, ReplayRun } from "../../src/lib/contracts";
import { baselineRun, capsule, whatIfRun } from "../fixtures/golden-contracts";

const readyCapsule = capsule as unknown as ReplayCapsule;
const completedBaseline = baselineRun as unknown as ReplayRun;

function activeRun(status: "CREATED" | "VALIDATING" | "RUNNING"): ReplayRun {
  return {
    ...completedBaseline,
    status,
    outcome: undefined,
    completed_at: undefined,
    effect_summary: undefined,
    failure_oracle_result: undefined,
    isolation_evidence: undefined,
    started_at: undefined,
  } as ReplayRun;
}

async function workflow() {
  return import("../../src/features/replay/workflow");
}

describe("replay workflow state machine", () => {
  it("compiles a capsule and only then accepts a baseline run", async () => {
    const { reduceReplayWorkflow } = await workflow();

    expect(reduceReplayWorkflow({ phase: "idle" }, { type: "runStarted", run: activeRun("CREATED") })).toMatchObject({
      phase: "idle",
    });

    let state = reduceReplayWorkflow({ phase: "idle" }, { type: "compileStarted" });
    expect(state).toMatchObject({ phase: "compiling" });

    state = reduceReplayWorkflow(state, { type: "capsuleCompiled", capsule: readyCapsule });
    expect(state).toMatchObject({ phase: "capsuleReady" });

    state = reduceReplayWorkflow(state, { type: "runStarted", run: activeRun("CREATED") });
    expect(state).toMatchObject({ phase: "active" });
  });

  it("keeps the run in an active phase for CREATED, VALIDATING, and RUNNING statuses", async () => {
    const { reduceReplayWorkflow, isActiveRunPhase } = await workflow();

    for (const status of ["CREATED", "VALIDATING", "RUNNING"] as const) {
      let state = reduceReplayWorkflow({ phase: "idle" }, { type: "compileStarted" });
      state = reduceReplayWorkflow(state, { type: "capsuleCompiled", capsule: readyCapsule });
      state = reduceReplayWorkflow(state, { type: "runStarted", run: activeRun(status) });
      expect(isActiveRunPhase(state)).toBe(true);
    }
  });

  it("ignores duplicate run starts while a run is already active or terminal", async () => {
    const { reduceReplayWorkflow } = await workflow();

    let state = reduceReplayWorkflow({ phase: "idle" }, { type: "compileStarted" });
    state = reduceReplayWorkflow(state, { type: "capsuleCompiled", capsule: readyCapsule });
    state = reduceReplayWorkflow(state, { type: "runStarted", run: activeRun("RUNNING") });
    const duplicate = reduceReplayWorkflow(state, { type: "runStarted", run: activeRun("CREATED") });
    expect(duplicate).toEqual(state);

    const finished = reduceReplayWorkflow(state, { type: "runUpdated", run: completedBaseline });
    expect(finished).toMatchObject({ phase: "terminal" });
    const duplicateAfterTerminal = reduceReplayWorkflow(finished, { type: "runStarted", run: activeRun("CREATED") });
    expect(duplicateAfterTerminal).toEqual(finished);
  });

  it("ignores stale run updates that belong to a different run id", async () => {
    const { reduceReplayWorkflow } = await workflow();

    let state = reduceReplayWorkflow({ phase: "idle" }, { type: "compileStarted" });
    state = reduceReplayWorkflow(state, { type: "capsuleCompiled", capsule: readyCapsule });
    state = reduceReplayWorkflow(state, { type: "runStarted", run: activeRun("RUNNING") });

    const stale = reduceReplayWorkflow(state, { type: "runUpdated", run: whatIfRun as unknown as ReplayRun });
    expect(stale).toEqual(state);
  });

  it("moves an active run to terminal with its error preserved for BLOCKED and FAILED", async () => {
    const { reduceReplayWorkflow } = await workflow();

    let state = reduceReplayWorkflow({ phase: "idle" }, { type: "compileStarted" });
    state = reduceReplayWorkflow(state, { type: "capsuleCompiled", capsule: readyCapsule });
    state = reduceReplayWorkflow(state, { type: "runStarted", run: activeRun("VALIDATING") });

    const blocked = reduceReplayWorkflow(state, {
      type: "runUpdated",
      run: {
        ...completedBaseline,
        status: "BLOCKED",
        outcome: undefined,
        completed_at: "2026-08-29T10:34:01Z",
        error: {
          code: "ISOLATION_VIOLATION",
          message: "Replay attempted a blocked destination.",
          retryable: false,
          details: {},
        },
      } as ReplayRun,
    });

    expect(blocked).toMatchObject({ phase: "terminal" });
    if (blocked.phase === "terminal") {
      expect(blocked.run.error?.code).toBe("ISOLATION_VIOLATION");
      expect(blocked.run.outcome).toBeUndefined();
    }
  });

  it("records compile and poll failures without discarding the capsule evidence", async () => {
    const { reduceReplayWorkflow } = await workflow();

    let state = reduceReplayWorkflow({ phase: "idle" }, { type: "compileStarted" });
    state = reduceReplayWorkflow(state, { type: "failed", message: "Core API unavailable.", code: "INTERNAL_FAILURE" });
    expect(state).toMatchObject({ phase: "error", message: "Core API unavailable." });

    state = reduceReplayWorkflow({ phase: "idle" }, { type: "compileStarted" });
    state = reduceReplayWorkflow(state, { type: "capsuleCompiled", capsule: readyCapsule });
    state = reduceReplayWorkflow(state, { type: "failed", message: "Monitoring failed." });
    expect(state).toMatchObject({ phase: "error", capsule: readyCapsule });

    state = reduceReplayWorkflow(state, { type: "reset" });
    expect(state).toMatchObject({ phase: "idle" });
  });
});

describe("what-if authorization gate", () => {
  it("unlocks only for a completed, reproduced baseline with passing isolation", async () => {
    const { deriveWhatIfGate } = await workflow();

    expect(deriveWhatIfGate(completedBaseline)).toEqual({ unlocked: true });
  });

  it("stays locked before, during, and after an unsuccessful baseline", async () => {
    const { deriveWhatIfGate } = await workflow();

    expect(deriveWhatIfGate(undefined).unlocked).toBe(false);

    const running = deriveWhatIfGate(activeRun("RUNNING"));
    expect(running).toMatchObject({ unlocked: false });
    expect(running.unlocked ? "" : running.reason).toMatch(/RUNNING/);

    const notReproduced = deriveWhatIfGate({
      ...completedBaseline,
      outcome: "NOT_REPRODUCED",
      failure_oracle_result: {
        ...completedBaseline.failure_oracle_result!,
        matched: false,
      },
    } as ReplayRun);
    expect(notReproduced).toMatchObject({ unlocked: false });
    expect(notReproduced.unlocked ? "" : notReproduced.reason).toMatch(/NOT_REPRODUCED/);

    const isolationFailed = deriveWhatIfGate({
      ...completedBaseline,
      isolation_evidence: { ...completedBaseline.isolation_evidence!, verdict: "FAIL" },
    } as ReplayRun);
    expect(isolationFailed).toMatchObject({ unlocked: false });

    const noIsolation = deriveWhatIfGate({
      ...completedBaseline,
      isolation_evidence: undefined,
    } as ReplayRun);
    expect(noIsolation).toMatchObject({ unlocked: false });

    const blocked = deriveWhatIfGate({
      ...completedBaseline,
      status: "BLOCKED",
      outcome: undefined,
      error: { code: "INTEGRITY_MISMATCH", message: "Digest mismatch.", retryable: false, details: {} },
    } as ReplayRun);
    expect(blocked).toMatchObject({ unlocked: false });
  });

  it("builds only the frozen PAYMENT_LATENCY 350 ms to 50 ms request", async () => {
    const { buildWhatIfRequest } = await workflow();

    expect(buildWhatIfRequest(completedBaseline)).toEqual({
      run_type: "WHAT_IF",
      baseline_run_id: "run-base-8271",
      intervention: { type: "PAYMENT_LATENCY", from: 350, to: 50, unit: "ms" },
    });
    expect(buildWhatIfRequest(activeRun("RUNNING"))).toBeUndefined();
    expect(buildWhatIfRequest({ ...completedBaseline, outcome: "NOT_REPRODUCED" } as ReplayRun)).toBeUndefined();
  });
});
