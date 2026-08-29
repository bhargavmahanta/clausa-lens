import { describe, expect, it } from "vitest";

import type { ReplayCapsule, ReplayDiff, ReplayRun, ResetResult } from "../../src/lib/contracts";
import { baselineRun, capsule, replayDiff, resetResult, whatIfRun } from "../fixtures/golden-contracts";

describe("command center replay state", () => {
  it("clears capsule, runs, diff, and errors after a confirmed successful reset", async () => {
    const { initialCommandCenterReplayState, reduceCommandCenterReplay } = await import(
      "../../src/features/replay/command-center-state"
    );
    let state = reduceCommandCenterReplay(initialCommandCenterReplayState, {
      type: "resourceReady",
      resource: "capsule",
      value: capsule as unknown as ReplayCapsule,
    });
    state = reduceCommandCenterReplay(state, {
      type: "resourceReady",
      resource: "baseline",
      value: baselineRun as unknown as ReplayRun,
    });
    state = reduceCommandCenterReplay(state, {
      type: "resourceReady",
      resource: "whatIf",
      value: whatIfRun as unknown as ReplayRun,
    });
    state = reduceCommandCenterReplay(state, {
      type: "resourceReady",
      resource: "diff",
      value: replayDiff as unknown as ReplayDiff,
    });
    state = reduceCommandCenterReplay(state, { type: "resetConfirmationOpened" });
    state = reduceCommandCenterReplay(state, { type: "resetStarted" });
    state = reduceCommandCenterReplay(state, {
      type: "resetSucceeded",
      result: resetResult as unknown as ResetResult,
    });

    expect(state.capsule.status).toBe("idle");
    expect(state.baseline.status).toBe("idle");
    expect(state.whatIf.status).toBe("idle");
    expect(state.diff.status).toBe("idle");
    expect(state.reset).toMatchObject({ status: "completed", result: { reset_id: "reset-1" } });
  });

  it("preserves evidence and exposes a frozen reset error when reset fails", async () => {
    const { initialCommandCenterReplayState, reduceCommandCenterReplay } = await import(
      "../../src/features/replay/command-center-state"
    );
    let state = reduceCommandCenterReplay(initialCommandCenterReplayState, {
      type: "resourceReady",
      resource: "capsule",
      value: capsule as unknown as ReplayCapsule,
    });
    state = reduceCommandCenterReplay(state, { type: "resetConfirmationOpened" });
    state = reduceCommandCenterReplay(state, { type: "resetStarted" });
    state = reduceCommandCenterReplay(state, {
      type: "resetFailed",
      error: { code: "INTERNAL_FAILURE", message: "Reset failed.", retryable: true },
    });

    expect(state.capsule.status).toBe("ready");
    expect(state.reset).toEqual({
      status: "failed",
      error: { code: "INTERNAL_FAILURE", message: "Reset failed.", retryable: true },
    });
  });

  it("stores an API error without fabricating a resource value", async () => {
    const { initialCommandCenterReplayState, reduceCommandCenterReplay } = await import(
      "../../src/features/replay/command-center-state"
    );
    const state = reduceCommandCenterReplay(initialCommandCenterReplayState, {
      type: "resourceFailed",
      resource: "baseline",
      error: { code: "PACK_UNAVAILABLE", message: "Pack is unavailable.", retryable: false },
    });

    expect(state.baseline).toEqual({
      status: "error",
      error: { code: "PACK_UNAVAILABLE", message: "Pack is unavailable.", retryable: false },
    });
    expect("value" in state.baseline).toBe(false);
  });

  it("clears all replay evidence when the investigated incident changes", async () => {
    const { initialCommandCenterReplayState, reduceCommandCenterReplay } = await import(
      "../../src/features/replay/command-center-state"
    );
    let state = reduceCommandCenterReplay(initialCommandCenterReplayState, {
      type: "resourceReady",
      resource: "capsule",
      value: capsule as unknown as ReplayCapsule,
    });
    state = reduceCommandCenterReplay(state, {
      type: "resourceReady",
      resource: "baseline",
      value: baselineRun as unknown as ReplayRun,
    });
    state = reduceCommandCenterReplay(state, {
      type: "resourceReady",
      resource: "whatIf",
      value: whatIfRun as unknown as ReplayRun,
    });
    state = reduceCommandCenterReplay(state, {
      type: "resourceReady",
      resource: "diff",
      value: replayDiff as unknown as ReplayDiff,
    });

    state = reduceCommandCenterReplay(state, { type: "incidentChanged" });

    expect(state.capsule.status).toBe("idle");
    expect(state.baseline.status).toBe("idle");
    expect(state.whatIf.status).toBe("idle");
    expect(state.diff.status).toBe("idle");
  });

  it("invalidates outstanding pollers through a generation token", async () => {
    const { createGenerationToken } = await import(
      "../../src/features/replay/command-center-state"
    );
    const token = createGenerationToken();

    const firstPoll = token.issue();
    expect(firstPoll()).toBe(true);

    const secondPoll = token.issue();
    expect(secondPoll()).toBe(true);

    token.invalidate();
    expect(firstPoll()).toBe(false);
    expect(secondPoll()).toBe(false);

    const thirdPoll = token.issue();
    expect(thirdPoll()).toBe(true);
  });
});
