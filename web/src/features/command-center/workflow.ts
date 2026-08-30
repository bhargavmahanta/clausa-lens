import type { CausaLensClient } from "../../lib/api";
import type {
  CreateRunRequest,
  Incident,
  ReplayCapsule,
  ReplayDiff,
  ReplayRun,
  ResetResult,
} from "../../lib/contracts";
import {
  pollForIncident,
  requestDemoCheckout,
  requestHealthyControlCheckout,
} from "../demo/trigger";
import { pollRunUntilTerminal } from "../replay/run-tracker";
import { isActiveRunStatus } from "../replay/workflow";
import {
  createGenerationToken,
  type CommandCenterReplayAction,
  type FrontendError,
  type ReplayGenerationToken,
} from "../replay/command-center-state";

export type DemoTriggerState =
  | { status: "idle" }
  | { status: "starting" }
  | { status: "waiting" }
  | { status: "failed"; error: FrontendError };

export type HealthyControlResult =
  | { status: "silent"; attempts: number; traceId: string }
  | { status: "unexpected-incident"; attempts: number; traceId: string }
  | { status: "failed"; error: FrontendError };

export type CommandCenterWorkflowDeps = {
  client: Pick<
    CausaLensClient,
    "createCapsule" | "createRun" | "getRun" | "createDiff" | "resetDemo" | "listIncidents"
  >;
  dispatch: (action: CommandCenterReplayAction) => void;
  onDemoState: (state: DemoTriggerState) => void;
  onIncidentDetected: (incident: Incident) => void;
  toFrontendError: (error: unknown) => FrontendError;
  fetchImpl?: typeof fetch;
  pollIntervalMs?: number;
  pollMaxAttempts?: number;
};

export type CommandCenterWorkflow = {
  token: ReplayGenerationToken;
  invalidate: () => void;
  notifyIncidentChanged: () => void;
  compileCapsule: (incidentId: string | undefined) => Promise<void>;
  startBaseline: (capsuleId: string) => Promise<void>;
  startWhatIf: (capsuleId: string, request: CreateRunRequest) => Promise<void>;
  createDiff: (request: { baseline_run_id: string; comparison_run_id: string }) => Promise<void>;
  confirmReset: () => Promise<ResetResult | undefined>;
  startFaultedCheckout: () => Promise<void>;
  runHealthyControl: () => Promise<HealthyControlResult>;
};

export function createCommandCenterWorkflow(
  deps: CommandCenterWorkflowDeps,
): CommandCenterWorkflow {
  const {
    client,
    dispatch,
    onDemoState,
    onIncidentDetected,
    toFrontendError,
    fetchImpl = fetch,
    pollIntervalMs = 500,
    pollMaxAttempts = 60,
  } = deps;
  const token = createGenerationToken();

  function compileCapsule(incidentId: string | undefined): Promise<void> {
    if (!incidentId) return Promise.resolve();
    const isCurrent = token.issue();
    dispatch({ type: "resourceLoading", resource: "capsule" });
    return (async () => {
      try {
        const value: ReplayCapsule = await client.createCapsule(incidentId);
        if (isCurrent()) dispatch({ type: "resourceReady", resource: "capsule", value });
      } catch (error) {
        if (isCurrent()) {
          dispatch({ type: "resourceFailed", resource: "capsule", error: toFrontendError(error) });
        }
      }
    })();
  }

  async function monitorRun(resource: "baseline" | "whatIf", run: ReplayRun, isCurrent: () => boolean) {
    if (!isActiveRunStatus(run)) return;
    const observed = await pollRunUntilTerminal({
      getRun: client.getRun,
      runId: run.run_id,
      intervalMs: pollIntervalMs,
      isCancelled: () => !isCurrent(),
      onProgress: (value) => {
        if (isCurrent()) dispatch({ type: "resourceReady", resource, value });
      },
    });
    if (observed && isCurrent()) {
      dispatch({ type: "resourceReady", resource, value: observed });
    }
  }

  async function startBaseline(capsuleId: string): Promise<void> {
    const isCurrent = token.issue();
    dispatch({ type: "resourceLoading", resource: "baseline" });
    try {
      const value: ReplayRun = await client.createRun(capsuleId, { run_type: "BASELINE" });
      if (!isCurrent()) return;
      dispatch({ type: "resourceReady", resource: "baseline", value });
      await monitorRun("baseline", value, isCurrent);
    } catch (error) {
      if (isCurrent()) {
        dispatch({ type: "resourceFailed", resource: "baseline", error: toFrontendError(error) });
      }
    }
  }

  async function startWhatIf(capsuleId: string, request: CreateRunRequest): Promise<void> {
    const isCurrent = token.issue();
    dispatch({ type: "resourceLoading", resource: "whatIf" });
    try {
      const value: ReplayRun = await client.createRun(capsuleId, request);
      if (!isCurrent()) return;
      dispatch({ type: "resourceReady", resource: "whatIf", value });
      await monitorRun("whatIf", value, isCurrent);
    } catch (error) {
      if (isCurrent()) {
        dispatch({ type: "resourceFailed", resource: "whatIf", error: toFrontendError(error) });
      }
    }
  }

  async function createDiff(request: {
    baseline_run_id: string;
    comparison_run_id: string;
  }): Promise<void> {
    const isCurrent = token.issue();
    dispatch({ type: "resourceLoading", resource: "diff" });
    try {
      const value: ReplayDiff = await client.createDiff(request);
      if (isCurrent()) dispatch({ type: "resourceReady", resource: "diff", value });
    } catch (error) {
      if (isCurrent()) {
        dispatch({ type: "resourceFailed", resource: "diff", error: toFrontendError(error) });
      }
    }
  }

  async function confirmReset(): Promise<ResetResult | undefined> {
    token.invalidate();
    dispatch({ type: "resetStarted" });
    try {
      const result = await client.resetDemo({ scenario_id: "checkout_duplicate_effect" });
      dispatch({ type: "resetSucceeded", result });
      return result;
    } catch (error) {
      dispatch({ type: "resetFailed", error: toFrontendError(error) });
      return undefined;
    }
  }

  async function startFaultedCheckout(): Promise<void> {
    const isCurrent = token.issue();
    onDemoState({ status: "starting" });
    try {
      const trace = await requestDemoCheckout(fetchImpl);
      if (!isCurrent()) {
        onDemoState({ status: "idle" });
        return;
      }
      onDemoState({ status: "waiting" });
      const incident = await pollForIncident({
        listIncidents: (query) => client.listIncidents(query),
        trace,
        intervalMs: pollIntervalMs,
        maxAttempts: pollMaxAttempts,
        isCancelled: () => !isCurrent(),
      });
      if (!isCurrent()) {
        onDemoState({ status: "idle" });
        return;
      }
      if (!incident) {
        onDemoState({
          status: "failed",
          error: {
            code: "ORACLE_UNAVAILABLE",
            message: "No incident was detected for the faulted checkout in time.",
            retryable: true,
          },
        });
        return;
      }
      onDemoState({ status: "idle" });
      token.invalidate();
      onIncidentDetected(incident);
    } catch (error) {
      if (isCurrent()) {
        onDemoState({ status: "failed", error: toFrontendError(error) });
      } else {
        onDemoState({ status: "idle" });
      }
    }
  }

  async function runHealthyControl(): Promise<HealthyControlResult> {
    try {
      const outcome = await requestHealthyControlCheckout(fetchImpl);
      const incident = await pollForIncident({
        listIncidents: (query) => client.listIncidents(query),
        trace: outcome,
        intervalMs: pollIntervalMs,
        maxAttempts: pollMaxAttempts,
      });
      if (incident) {
        return {
          status: "unexpected-incident",
          attempts: outcome.attempts,
          traceId: outcome.traceId,
        };
      }
      return { status: "silent", attempts: outcome.attempts, traceId: outcome.traceId };
    } catch (error) {
      return { status: "failed", error: toFrontendError(error) };
    }
  }

  return {
    token,
    invalidate: () => token.invalidate(),
    notifyIncidentChanged: () => token.invalidate(),
    compileCapsule,
    startBaseline,
    startWhatIf,
    createDiff,
    confirmReset,
    startFaultedCheckout,
    runHealthyControl,
  };
}
