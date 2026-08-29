import type { CreateRunRequest, ReplayCapsule, ReplayRun } from "../../lib/contracts";

export type ReplayWorkflowState =
  | { phase: "idle" }
  | { phase: "compiling" }
  | { phase: "capsuleReady"; capsule: ReplayCapsule }
  | { phase: "active"; capsule: ReplayCapsule; run: ReplayRun }
  | { phase: "terminal"; capsule: ReplayCapsule; run: ReplayRun }
  | { phase: "error"; message: string; code?: string; capsule?: ReplayCapsule };

export type ReplayWorkflowAction =
  | { type: "compileStarted" }
  | { type: "capsuleCompiled"; capsule: ReplayCapsule }
  | { type: "runStarted"; run: ReplayRun }
  | { type: "runUpdated"; run: ReplayRun }
  | { type: "failed"; message: string; code?: string }
  | { type: "reset" };

const activeStatuses = new Set<ReplayRun["status"]>(["CREATED", "VALIDATING", "RUNNING"]);

export function isActiveRunStatus(run: ReplayRun): boolean {
  return activeStatuses.has(run.status);
}

export function isActiveRunPhase(state: ReplayWorkflowState): boolean {
  return state.phase === "active" && isActiveRunStatus(state.run);
}

export function reduceReplayWorkflow(
  state: ReplayWorkflowState,
  action: ReplayWorkflowAction,
): ReplayWorkflowState {
  switch (action.type) {
    case "compileStarted":
      if (state.phase === "compiling") return state;
      return { phase: "compiling" };
    case "capsuleCompiled":
      if (state.phase !== "compiling") return state;
      return { phase: "capsuleReady", capsule: action.capsule };
    case "runStarted": {
      if (state.phase !== "capsuleReady") return state;
      if (!isActiveRunStatus(action.run)) {
        return { phase: "terminal", capsule: state.capsule, run: action.run };
      }
      return { phase: "active", capsule: state.capsule, run: action.run };
    }
    case "runUpdated": {
      if (state.phase !== "active") return state;
      if (state.run.run_id !== action.run.run_id) return state;
      if (isActiveRunStatus(action.run)) {
        return { phase: "active", capsule: state.capsule, run: action.run };
      }
      return { phase: "terminal", capsule: state.capsule, run: action.run };
    }
    case "failed": {
      if (state.phase === "terminal") return state;
      if (state.phase === "active" && isActiveRunStatus(state.run)) {
        return { phase: "error", message: action.message, code: action.code, capsule: state.capsule };
      }
      if (state.phase === "capsuleReady" || state.phase === "active") {
        return { phase: "error", message: action.message, code: action.code, capsule: state.capsule };
      }
      return { phase: "error", message: action.message, code: action.code };
    }
    case "reset":
      return { phase: "idle" };
    default:
      return state;
  }
}

export type WhatIfGate =
  | { unlocked: true }
  | { unlocked: false; reason: string };

export function deriveWhatIfGate(run: ReplayRun | undefined): WhatIfGate {
  if (!run) {
    return { unlocked: false, reason: "No baseline run exists yet." };
  }
  if (run.status !== "COMPLETED") {
    return {
      unlocked: false,
      reason: `Baseline run is ${run.status}; what-if unlocks only after a completed baseline.`,
    };
  }
  if (run.outcome !== "REPRODUCED") {
    return {
      unlocked: false,
      reason: `Baseline outcome is ${run.outcome}; experimentation stops without a reproduced failure.`,
    };
  }
  if (run.isolation_evidence?.verdict !== "PASS") {
    return { unlocked: false, reason: "Baseline isolation evidence is not passing." };
  }
  return { unlocked: true };
}

export function buildWhatIfRequest(run: ReplayRun | undefined): CreateRunRequest | undefined {
  if (!run || !deriveWhatIfGate(run).unlocked) return undefined;
  return {
    run_type: "WHAT_IF",
    baseline_run_id: run.run_id,
    intervention: { type: "PAYMENT_LATENCY", from: 350, to: 50, unit: "ms" },
  };
}
