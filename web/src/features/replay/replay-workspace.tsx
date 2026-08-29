import { StatePanel } from "../../components/system";
import type { ReplayDiff, ReplayRun } from "../../lib/contracts";
import { buildCapsuleView } from "./capsule-view";
import { ReplayDiffPanel } from "./replay-diff";
import { buildRunView } from "./run-view";
import type { ReplayWorkflowState, WhatIfGate } from "./workflow";
import { deriveWhatIfGate } from "./workflow";

export type ReplayWorkspaceProps = {
  state: ReplayWorkflowState;
  hasSelectedIncident?: boolean;
  gate?: WhatIfGate;
  whatIfRun?: ReplayRun;
  diff?: ReplayDiff;
  baselineLoading?: boolean;
  whatIfLoading?: boolean;
  diffLoading?: boolean;
  onCompile?: () => void;
  onStartBaseline?: () => void;
  onStartWhatIf?: () => void;
  onCreateDiff?: () => void;
  onRetry?: () => void;
};

function CapsuleEvidence({ capsule }: { capsule: Extract<ReplayWorkflowState, { phase: "capsuleReady" | "active" | "terminal" }>["capsule"] }) {
  const view = buildCapsuleView(capsule);
  return (
    <section className="workflow-card capsule-evidence" aria-labelledby="capsule-title">
      <header className="workflow-card__header">
        <div><p className="panel-kicker">Immutable replay evidence</p><h2 id="capsule-title">Replay Capsule</h2></div>
        <code>{view.capsuleId}</code>
      </header>
      <div className="capsule-validation" role="status">
        <span>Capsule validation</span><strong>VALID</strong>
        <small>Decoded against frozen contract 1.0</small>
      </div>
      <dl className="identifier-grid">
        <div><dt>System Pack</dt><dd>{view.pack.id}</dd></div>
        <div><dt>Interface</dt><dd>{view.pack.interfaceVersion}</dd></div>
        <div><dt>Entrypoint</dt><dd>{view.plan.entrypoint}</dd></div>
        <div><dt>Reset strategy</dt><dd>{view.plan.resetStrategy}</dd></div>
        <div><dt>Trigger</dt><dd>{view.triggerSummary}</dd></div>
        <div><dt>Graph</dt><dd>{view.graphId}</dd></div>
      </dl>
      <div className="capsule-evidence-grid">
        <section><h3>State fixtures</h3>{view.stateFixtures.map((fixture) => <code key={fixture.fixtureId}>{fixture.fixtureId} · {fixture.sanitizationStatus}</code>)}</section>
        <section><h3>Dependency fixtures</h3>{view.dependencyFixtures.map((fixture) => <code key={fixture.fixtureId}>{fixture.fixtureId} · {fixture.latencyMS} ms</code>)}</section>
        <section><h3>Oracle expectation</h3><code>{view.oracle.id}</code><p>payment_attempt_count: {view.oracle.expectedEffectSummary.paymentAttemptCount} · ledger_commit_count: {view.oracle.expectedEffectSummary.ledgerCommitCount}</p></section>
        <section><h3>Safety policy</h3><p>Credential profile: {view.safety.credentialProfile}</p>{view.safety.blockedDestinations.map((destination) => <code key={destination}>{destination}</code>)}</section>
      </div>
      <div className="integrity-evidence"><span>Integrity</span><strong>{view.integrity.algorithm}</strong><code>{view.integrity.digest}</code></div>
    </section>
  );
}

function RunEvidence({ run }: { run: ReplayRun }) {
  const view = buildRunView(run);
  const reproduced = run.outcome === "REPRODUCED";
  return (
    <section className="workflow-card run-evidence" aria-labelledby={`${run.run_id}-title`}>
      <header className="workflow-card__header">
        <div><p className="panel-kicker">{view.runType} replay</p><h2 id={`${run.run_id}-title`}>Run evidence</h2></div>
        <code>{view.runId}</code>
      </header>
      <dl className="run-status-grid">
        {view.rows.map((row) => <div data-tone={row.tone} key={row.label}><dt>{row.label}</dt><dd>{row.value}</dd></div>)}
      </dl>
      {view.error ? <StatePanel state={run.status === "BLOCKED" ? "blocked" : "failed"} title={`${run.status} replay`} message={view.error.message} code={view.error.code} /> : null}
      {view.isolation ? (
        <section className="isolation-evidence"><h3>Isolation evidence</h3><dl><div><dt>Runtime namespace</dt><dd>{view.isolation.runtimeNamespace}</dd></div><div><dt>Network policy</dt><dd>{view.isolation.networkPolicy}</dd></div><div><dt>Credential profile</dt><dd>{view.isolation.credentialProfile}</dd></div><div><dt>Teardown</dt><dd>{view.isolation.teardownResult}</dd></div></dl>{view.isolation.simulatorInteractions.map((item) => <p key={`${item.dependency}-${item.operation}`}>{item.dependency} · {item.result}</p>)}</section>
      ) : null}
      {view.effectSummary ? <p className="effect-summary">{view.effectSummary.paymentAttemptCount} payment attempts · {view.effectSummary.ledgerCommitCount} ledger commits</p> : null}
      {view.oracle ? <section className="oracle-result"><h3>{reproduced ? "Failure reproduced" : run.outcome === "MITIGATED" ? "Failure mitigated" : "Oracle result"}</h3><p>{view.oracle.explanation}</p></section> : null}
    </section>
  );
}

export function ReplayWorkspace({
  state,
  hasSelectedIncident = true,
  gate,
  whatIfRun,
  diff,
  baselineLoading,
  whatIfLoading,
  diffLoading,
  onCompile,
  onStartBaseline,
  onStartWhatIf,
  onCreateDiff,
  onRetry,
}: ReplayWorkspaceProps) {
  const capsule = "capsule" in state ? state.capsule : undefined;
  const baselineRun = state.phase === "active" || state.phase === "terminal" ? state.run : undefined;
  const whatIfGate = gate ?? deriveWhatIfGate(baselineRun);
  const baselineDisabled = !capsule || baselineLoading || state.phase === "active" || state.phase === "terminal";

  return (
    <section className="replay-workspace" aria-labelledby="replay-workspace-title">
      <header className="workspace-section-heading">
        <div><p className="eyebrow">Compile → Reproduce → Intervene → Diff</p><h2 id="replay-workspace-title">Controlled replay</h2></div>
        <p>Every value below arrives through the frozen API boundary.</p>
      </header>

      {state.phase === "compiling" ? <StatePanel state="loading" title="Compiling Replay Capsule" message="Validating fixtures, plan, safety policy, and integrity." /> : null}
      {state.phase === "error" ? <StatePanel action={onRetry ? <button onClick={onRetry} type="button">Retry</button> : undefined} state="error" title="Replay workflow unavailable" message={state.message} code={state.code} /> : null}
      {capsule ? <CapsuleEvidence capsule={capsule} /> : null}

      <nav className="workflow-actions" aria-label="Replay workflow actions">
        <button disabled={!hasSelectedIncident || state.phase !== "idle"} onClick={onCompile} type="button">Compile replay capsule</button>
        <button disabled={baselineDisabled} onClick={onStartBaseline} type="button">{baselineLoading ? "Starting baseline…" : "Start baseline replay"}</button>
      </nav>

      {baselineRun ? <RunEvidence run={baselineRun} /> : null}

      <section className="workflow-card intervention-card" aria-labelledby="intervention-title">
        <header className="workflow-card__header"><div><p className="panel-kicker">Approved what-if</p><h2 id="intervention-title">PAYMENT LATENCY</h2></div><code>PAYMENT_LATENCY</code></header>
        <div className="intervention-values"><strong>350 ms</strong><span aria-hidden="true">→</span><strong>50 ms</strong></div>
        <p>All other replay conditions remain unchanged.</p>
        {!whatIfGate.unlocked ? <p className="gate-reason">{whatIfGate.reason}</p> : !whatIfRun ? <p>What-if execution arrives with the next checkpoint.</p> : null}
        <button disabled={!whatIfGate.unlocked || whatIfLoading || Boolean(whatIfRun)} onClick={onStartWhatIf} type="button">{whatIfLoading ? "Starting what-if…" : "Run controlled what-if"}</button>
      </section>

      {whatIfRun ? <RunEvidence run={whatIfRun} /> : null}
      {whatIfRun?.status === "COMPLETED" ? <button className="diff-action" disabled={diffLoading || Boolean(diff)} onClick={onCreateDiff} type="button">{diffLoading ? "Building Replay Diff…" : "Build Replay Diff"}</button> : null}
      {diff ? <ReplayDiffPanel diff={diff} /> : null}
    </section>
  );
}
