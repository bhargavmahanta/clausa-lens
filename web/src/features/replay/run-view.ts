import type { ReplayRun } from "../../lib/contracts";

export type RunStatusRow = {
  label: string;
  value: string;
  tone: "completed" | "active" | "pass" | "fail" | "match" | "no-match" | "reproduced" | "not-reproduced" | "mitigated" | "neutral" | "blocked" | "failed";
};

export type RunView = {
  runId: string;
  runType: ReplayRun["run_type"];
  status: ReplayRun["status"];
  lifecycleActive: boolean;
  rows: RunStatusRow[];
  observedEventCount: number;
  error?: { code: string; message: string; retryable: boolean };
  isolation?: {
    verdict: string;
    networkPolicy: string;
    runtimeNamespace: string;
    credentialProfile: string;
    datastoreDestinations: string[];
    simulatorInteractions: ReplayRun["isolation_evidence"] extends undefined ? never[] : NonNullable<ReplayRun["isolation_evidence"]>["simulator_interactions"];
    deniedInteractions: NonNullable<ReplayRun["isolation_evidence"]>["denied_interactions"];
    teardownResult: string;
  };
  effectSummary?: { paymentAttemptCount: number; ledgerCommitCount: number };
  oracle?: { matched: boolean; explanation: string; requiredEvidenceCount: number };
  timing?: { startedAt?: string; completedAt?: string };
};

const activeStatuses = new Set<ReplayRun["status"]>(["CREATED", "VALIDATING", "RUNNING"]);

function executionTone(status: ReplayRun["status"]): RunStatusRow["tone"] {
  if (status === "COMPLETED") return "completed";
  if (status === "BLOCKED") return "blocked";
  if (status === "FAILED") return "failed";
  return "active";
}

function outcomeRow(run: ReplayRun): RunStatusRow | undefined {
  if (!run.outcome) return undefined;
  const tone: RunStatusRow["tone"] =
    run.outcome === "REPRODUCED"
      ? "reproduced"
      : run.outcome === "NOT_REPRODUCED"
        ? "not-reproduced"
        : run.outcome === "MITIGATED"
          ? "mitigated"
          : "neutral";
  return {
    label: run.run_type === "BASELINE" ? "Baseline outcome" : "What-if outcome",
    value: run.outcome,
    tone,
  };
}

export function buildRunView(run: ReplayRun): RunView {
  const rows: RunStatusRow[] = [
    { label: "Replay execution", value: run.status, tone: executionTone(run.status) },
  ];

  if (run.isolation_evidence) {
    rows.push({
      label: "Isolation",
      value: run.isolation_evidence.verdict,
      tone: run.isolation_evidence.verdict === "PASS" ? "pass" : "fail",
    });
  }
  if (run.failure_oracle_result) {
    rows.push({
      label: "Failure oracle",
      value: run.failure_oracle_result.matched ? "MATCHED" : "NOT MATCHED",
      tone: run.failure_oracle_result.matched ? "match" : "no-match",
    });
  }
  const outcome = outcomeRow(run);
  if (outcome) rows.push(outcome);

  return {
    runId: run.run_id,
    runType: run.run_type,
    status: run.status,
    lifecycleActive: activeStatuses.has(run.status),
    rows,
    observedEventCount: run.observed_event_ids.length,
    error: run.error
      ? { code: run.error.code, message: run.error.message, retryable: run.error.retryable }
      : undefined,
    isolation: run.isolation_evidence
      ? {
          verdict: run.isolation_evidence.verdict,
          networkPolicy: run.isolation_evidence.network_policy,
          runtimeNamespace: run.isolation_evidence.runtime_namespace,
          credentialProfile: run.isolation_evidence.credential_profile,
          datastoreDestinations: run.isolation_evidence.datastore_destinations,
          simulatorInteractions: run.isolation_evidence.simulator_interactions,
          deniedInteractions: run.isolation_evidence.denied_interactions,
          teardownResult: run.isolation_evidence.teardown_result,
        }
      : undefined,
    effectSummary: run.effect_summary
      ? {
          paymentAttemptCount: run.effect_summary.payment_attempt_count,
          ledgerCommitCount: run.effect_summary.ledger_commit_count,
        }
      : undefined,
    oracle: run.failure_oracle_result
      ? {
          matched: run.failure_oracle_result.matched,
          explanation: run.failure_oracle_result.explanation,
          requiredEvidenceCount: run.failure_oracle_result.required_evidence_event_ids.length,
        }
      : undefined,
    timing: { startedAt: run.started_at, completedAt: run.completed_at },
  };
}
