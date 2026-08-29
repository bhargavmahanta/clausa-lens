"use client";

import { useCallback, useMemo, useReducer, useRef, useState } from "react";

import { StatePanel } from "../../components/system";
import {
  CausaLensApiError,
  ContractDecodeError,
  ProtocolError,
  createCausaLensClient,
} from "../../lib/api";
import type { Incident } from "../../lib/contracts";
import {
  IncidentCommandCenter,
  incidentDataSource,
  type SelectionRequest,
} from "../incidents/incident-command-center";
import { buildWhatIfRequest } from "../replay/workflow";
import {
  ReplayWorkspace,
  ResetDialog,
  ResetReceipt,
  initialCommandCenterReplayState,
  isActiveRunStatus,
  reduceCommandCenterReplay,
  type FrontendError,
  type ReplayWorkflowState,
} from "../replay";
import {
  createCommandCenterWorkflow,
  type DemoTriggerState,
} from "./workflow";

function toFrontendError(error: unknown): FrontendError {
  if (error instanceof CausaLensApiError) {
    return { code: error.code, message: error.message, retryable: error.retryable };
  }
  if (error instanceof ContractDecodeError) {
    return {
      code: "SCHEMA_INVALID",
      message: `${error.message} ${error.issues.length} contract issue${error.issues.length === 1 ? "" : "s"} reported.`,
      retryable: false,
    };
  }
  if (error instanceof ProtocolError) {
    return { code: "SCHEMA_INVALID", message: error.message, retryable: false };
  }
  return {
    code: "INTERNAL_FAILURE",
    message: error instanceof Error ? error.message : "The workflow request failed unexpectedly.",
    retryable: false,
  };
}

function toWorkspaceState(
  state: ReturnType<typeof reduceCommandCenterReplay>,
): ReplayWorkflowState {
  if (state.capsule.status === "idle") return { phase: "idle" };
  if (state.capsule.status === "loading") return { phase: "compiling" };
  if (state.capsule.status === "error") {
    return { phase: "error", ...state.capsule.error };
  }
  const capsule = state.capsule.value;
  if (state.baseline.status === "error") {
    return { phase: "error", capsule, ...state.baseline.error };
  }
  if (state.baseline.status !== "ready") return { phase: "capsuleReady", capsule };
  return isActiveRunStatus(state.baseline.value)
    ? { phase: "active", capsule, run: state.baseline.value }
    : { phase: "terminal", capsule, run: state.baseline.value };
}

export function CommandCenter() {
  const client = useMemo(
    () => createCausaLensClient({ baseUrl: incidentDataSource.baseUrl }),
    [],
  );
  const [selectedIncidentId, setSelectedIncidentId] = useState<string>();
  const [incidentVersion, setIncidentVersion] = useState(0);
  const [state, dispatch] = useReducer(
    reduceCommandCenterReplay,
    initialCommandCenterReplayState,
  );
  const [selectionRequest, setSelectionRequest] = useState<SelectionRequest>();
  const [demo, setDemo] = useState<DemoTriggerState>({ status: "idle" });
  const selectedIncidentRef = useRef<string | undefined>(undefined);

  const handleIncidentDetected = useCallback((incident: Incident) => {
    dispatch({ type: "incidentChanged" });
    setSelectionRequest({ incidentId: incident.incident_id, nonce: Date.now() });
  }, []);

  const workflow = useMemo(
    () =>
      createCommandCenterWorkflow({
        client,
        dispatch,
        onDemoState: setDemo,
        onIncidentDetected: handleIncidentDetected,
        toFrontendError,
      }),
    [client, handleIncidentDetected],
  );

  const handleSelectionChange = useCallback(
    (incidentId: string | undefined) => {
      if (selectedIncidentRef.current !== incidentId) {
        selectedIncidentRef.current = incidentId;
        workflow.invalidate();
        dispatch({ type: "incidentChanged" });
        setDemo((current) =>
          current.status === "starting" || current.status === "waiting"
            ? { status: "idle" }
            : current,
        );
      }
      setSelectedIncidentId(incidentId);
    },
    [workflow],
  );

  function compileCapsule() {
    void workflow.compileCapsule(selectedIncidentId);
  }

  function startBaseline() {
    if (state.capsule.status !== "ready") return;
    void workflow.startBaseline(state.capsule.value.capsule_id);
  }

  function startWhatIf() {
    if (state.capsule.status !== "ready" || state.baseline.status !== "ready") return;
    const request = buildWhatIfRequest(state.baseline.value);
    if (!request) return;
    void workflow.startWhatIf(state.capsule.value.capsule_id, request);
  }

  function createDiff() {
    if (state.baseline.status !== "ready" || state.whatIf.status !== "ready") return;
    void workflow.createDiff({
      baseline_run_id: state.baseline.value.run_id,
      comparison_run_id: state.whatIf.value.run_id,
    });
  }

  async function confirmReset() {
    workflow.invalidate();
    setDemo({ status: "idle" });
    try {
      const result = await workflow.confirmReset();
      if (result) {
        selectedIncidentRef.current = undefined;
        setSelectedIncidentId(undefined);
        setIncidentVersion((version) => version + 1);
      }
    } catch {
      // confirmReset maps failures itself; this guard keeps the UI alive.
    }
  }

  const workspaceState = toWorkspaceState(state);
  const whatIf = state.whatIf.status === "ready" ? state.whatIf.value : undefined;
  const diff = state.diff.status === "ready" ? state.diff.value : undefined;
  const demoPending = demo.status === "starting" || demo.status === "waiting";

  return (
    <>
      <section className="workspace-intro" aria-labelledby="workspace-title">
        <div><p className="eyebrow">Capture → Trace → Replay → Diff</p><h1 id="workspace-title">Incident analysis</h1></div>
        <p>Contract-decoded evidence from capture through first divergence.</p>
      </section>

      <section className="demo-trigger" aria-labelledby="demo-trigger-title">
        <div>
          <p className="panel-kicker">Judge control</p>
          <h2 id="demo-trigger-title">Golden scenario</h2>
          <p className="evidence-note">Triggers the fixed faulted checkout and opens the detected incident automatically.</p>
        </div>
        <button
          aria-live="polite"
          disabled={demoPending}
          onClick={() => void workflow.startFaultedCheckout()}
          type="button"
        >
          {demo.status === "starting"
            ? "Starting faulted checkout…"
            : demo.status === "waiting"
              ? "Waiting for incident detection…"
              : "Start Faulted Checkout"}
        </button>
      </section>
      {demo.status === "failed" ? (
        <StatePanel
          state="error"
          title="Faulted checkout trigger failed"
          message={demo.error.message}
          code={demo.error.code}
          action={<button onClick={() => setDemo({ status: "idle" })} type="button">Dismiss</button>}
        />
      ) : null}

      <IncidentCommandCenter
        key={incidentVersion}
        onSelectionChange={handleSelectionChange}
        selectionRequest={selectionRequest}
      />

      <ReplayWorkspace
        baselineLoading={state.baseline.status === "loading"}
        diff={diff}
        diffLoading={state.diff.status === "loading"}
        hasSelectedIncident={Boolean(selectedIncidentId)}
        onCompile={compileCapsule}
        onCreateDiff={createDiff}
        onStartBaseline={startBaseline}
        onStartWhatIf={startWhatIf}
        state={workspaceState}
        whatIfLoading={state.whatIf.status === "loading"}
        whatIfRun={whatIf}
      />

      {state.whatIf.status === "error" ? <StatePanel state="error" title="What-if replay unavailable" message={state.whatIf.error.message} code={state.whatIf.error.code} /> : null}
      {state.diff.status === "error" ? <StatePanel state="error" title="Replay Diff unavailable" message={state.diff.error.message} code={state.diff.error.code} /> : null}

      <section className="reset-control" aria-labelledby="reset-control-title">
        <div><p className="panel-kicker">Deterministic scenario</p><h2 id="reset-control-title">Reset demo workflow</h2></div>
        <button onClick={() => void confirmReset()} type="button">Reset demo workflow</button>
      </section>
      {state.reset.status === "confirming" || state.reset.status === "submitting" ? (
        <ResetDialog
          onCancel={() => dispatch({ type: "resetConfirmationClosed" })}
          onConfirm={() => void confirmReset()}
          status={state.reset.status}
        />
      ) : null}
      {state.reset.status === "completed" ? <ResetReceipt result={state.reset.result} /> : null}
      {state.reset.status === "failed" ? <StatePanel state="failed" title="Demo reset failed" message={state.reset.error.message} code={state.reset.error.code} /> : null}
    </>
  );
}
