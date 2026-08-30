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
  OverviewCarousel,
  type CarouselStageContent,
} from "../overview";
import {
  createCommandCenterWorkflow,
  type DemoTriggerState,
  type HealthyControlResult,
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
  const [healthyControl, setHealthyControl] = useState<HealthyControlResult>();
  const [healthyControlRunning, setHealthyControlRunning] = useState(false);
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

  async function runHealthyControl() {
    setHealthyControlRunning(true);
    setHealthyControl(undefined);
    try {
      setHealthyControl(await workflow.runHealthyControl());
    } finally {
      setHealthyControlRunning(false);
    }
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

  const replayStage = state.baseline.status === "ready" && state.baseline.value.status === "COMPLETED";
  const baselineSummary =
    state.baseline.status === "ready"
      ? `Baseline ${state.baseline.value.status}${state.baseline.value.outcome ? ` · ${state.baseline.value.outcome}` : ""}`
      : "Baseline not started";
  const whatIfSummary =
    state.whatIf.status === "ready"
      ? `What-if ${state.whatIf.value.status}${state.whatIf.value.outcome ? ` · ${state.whatIf.value.outcome}` : ""}`
      : "What-if locked until a reproduced baseline";
  const diffSummary = diff
      ? `${diff.effect_delta.payment_attempt_count} attempts · ${diff.effect_delta.ledger_commit_count} ledger commits · oracle ${
          diff.baseline_oracle_result.matched ? "MATCHED" : "NOT MATCHED"
        } → ${diff.comparison_oracle_result.matched ? "MATCHED" : "NOT MATCHED"}`
      : "Awaiting baseline and what-if runs";

  const carouselStages: CarouselStageContent[] = [
    {
      stage: "incident",
      eyebrow: "Captured evidence",
      title: "Incident / Trace",
      summary: selectedIncidentId ?? "No incident selected",
      description: "Follow the request through Gateway → Checkout → Payment → Ledger",
      href: "#incident-workspace",
      actionLabel: "Inspect incident evidence",
      statusChip: selectedIncidentId
        ? { label: "SELECTED", tone: "pass" }
        : { label: "PENDING", tone: "neutral" },
      metrics: [
        { label: "Path", value: "4 services" },
        { label: "Capture", value: selectedIncidentId ? "Detected" : "Waiting" },
      ],
    },
    {
      stage: "capsule",
      eyebrow: "Replay artifact",
      title: "Replay Capsule",
      summary:
        state.capsule.status === "ready"
          ? state.capsule.value.capsule_id
          : state.capsule.status === "loading"
            ? "Compiling capsule from captured evidence"
            : state.capsule.status === "error"
              ? state.capsule.error.message
              : "Awaiting capsule compilation",
      description: "Integrity, fixtures, policy, and isolation readiness",
      href: "#replay-lab",
      actionLabel: "Open capsule workflow",
      statusChip:
        state.capsule.status === "ready"
          ? { label: "READY", tone: "pass" }
          : state.capsule.status === "error"
            ? { label: "BLOCKED", tone: "fail" }
            : state.capsule.status === "loading"
              ? { label: "COMPILING", tone: "warning" }
              : { label: "PENDING", tone: "neutral" },
      metrics: [
        {
          label: "Capsule",
          value: state.capsule.status === "ready" ? "Compiled" : "Not compiled",
        },
        {
          label: "Isolation",
          value:
            state.baseline.status === "ready" && state.baseline.value.isolation_evidence
              ? state.baseline.value.isolation_evidence.verdict
              : "Awaiting replay",
        },
      ],
    },
    {
      stage: "replay",
      eyebrow: "Controlled execution",
      title: "Replay",
      summary: `${baselineSummary} · ${whatIfSummary}`,
      description: "Baseline and what-if",
      href: "#replay-lab",
      actionLabel: "Open replay lab",
      statusChip: replayStage
        ? { label: "REPLAYED", tone: "pass" }
        : { label: "IDLE", tone: "neutral" },
      metrics: [
        {
          label: "Baseline",
          value: state.baseline.status === "ready" ? state.baseline.value.status : "Not started",
        },
        {
          label: "What-if",
          value: state.whatIf.status === "ready" ? state.whatIf.value.status : "Locked",
        },
      ],
    },
    {
      stage: "diff",
      eyebrow: "Evidence delta",
      title: "Diff",
      summary: diffSummary,
      description: "First meaningful divergence",
      href: "#replay-lab",
      actionLabel: "Inspect replay diff",
      statusChip: diff ? { label: "READY", tone: "pass" } : undefined,
      metrics: diff
        ? [
            {
              label: "Ledger commits",
              value: String(diff.effect_delta.ledger_commit_count),
            },
            {
              label: "Oracle",
              value: `${diff.baseline_oracle_result.matched ? "TRUE" : "FALSE"} → ${diff.comparison_oracle_result.matched ? "TRUE" : "FALSE"}`,
            },
          ]
        : [
            { label: "Baseline", value: "Required" },
            { label: "What-if", value: "Required" },
          ],
    },
  ];

  return (
    <>
      <div id="overview-hero">
        <OverviewCarousel stages={carouselStages} />
      </div>

      <section className="workspace-intro" aria-labelledby="workspace-title">
        <div><p className="eyebrow">Capture → Trace → Replay → Diff</p><h2 id="workspace-title">Incident analysis</h2></div>
        <p>Contract-decoded evidence from capture through first divergence.</p>
      </section>

      <section className="demo-trigger" id="demo-trigger" aria-labelledby="demo-trigger-title">
        <div>
          <p className="panel-kicker">Judge control</p>
          <h2 id="demo-trigger-title">Golden scenario</h2>
          <p className="evidence-note">Triggers the fixed faulted checkout and opens the detected incident automatically.</p>
        </div>
        <div className="demo-trigger-actions">
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
          <button
            aria-live="polite"
            disabled={demoPending || healthyControlRunning}
            onClick={() => void runHealthyControl()}
            type="button"
          >
            {healthyControlRunning ? "Running healthy checkout…" : "Run healthy checkout (control)"}
          </button>
        </div>
        {healthyControl ? (
          <p className="evidence-note" role="status">
            {healthyControl.status === "silent"
              ? `Healthy control ${healthyControl.traceId}: ${healthyControl.attempts} payment attempt, no timeout, no retry — the failure oracle stayed silent and created no incident.`
              : healthyControl.status === "unexpected-incident"
                ? `Unexpected: an incident was detected for healthy control ${healthyControl.traceId}. The oracle must stay silent on a healthy trace — investigate.`
                : `Healthy control failed: ${healthyControl.error.message}`}
          </p>
        ) : null}
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

      <div id="incident-workspace">
        <IncidentCommandCenter
          key={incidentVersion}
          onSelectionChange={handleSelectionChange}
          selectionRequest={selectionRequest}
        />
      </div>

      <div id="replay-lab">
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
      </div>

      {state.whatIf.status === "error" ? <StatePanel state="error" title="What-if replay unavailable" message={state.whatIf.error.message} code={state.whatIf.error.code} /> : null}
      {state.diff.status === "error" ? <StatePanel state="error" title="Replay Diff unavailable" message={state.diff.error.message} code={state.diff.error.code} /> : null}

      <section className="reset-control" id="reset-control" aria-labelledby="reset-control-title">
        <div><p className="panel-kicker">Deterministic scenario</p><h2 id="reset-control-title">Reset demo workflow</h2></div>
        <button onClick={() => dispatch({ type: "resetConfirmationOpened" })} type="button">Reset demo workflow</button>
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
