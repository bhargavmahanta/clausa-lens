import type { ReplayRun } from "../../lib/contracts";

type EvidenceTone = "neutral" | "info" | "positive" | "warning" | "failure" | "muted";

function outcomeTone(outcome: ReplayRun["outcome"]): EvidenceTone {
  if (outcome === "MITIGATED") return "positive";
  if (outcome === "REPRODUCED" || outcome === "UNCHANGED") return "failure";
  if (outcome === "NOT_REPRODUCED" || outcome === "INCONCLUSIVE") return "warning";
  return "muted";
}

export type RunEvidenceSummaryProps = {
  run: ReplayRun;
};

export function RunEvidenceSummary({ run }: RunEvidenceSummaryProps) {
  const isolation = run.isolation_evidence?.verdict ?? "NOT AVAILABLE";
  const oracle = run.failure_oracle_result
    ? run.failure_oracle_result.matched
      ? "MATCHED"
      : "NOT MATCHED"
    : "NOT EVALUATED";
  const outcomeLabel = run.run_type === "BASELINE" ? "Baseline outcome" : "What-if outcome";

  const rows: Array<{ label: string; value: string; tone: EvidenceTone }> = [
    { label: "Replay execution", value: run.status, tone: run.status === "RUNNING" ? "info" : "neutral" },
    { label: "Isolation", value: isolation, tone: isolation === "PASS" ? "positive" : isolation === "FAIL" ? "failure" : "muted" },
    { label: "Failure oracle", value: oracle, tone: oracle === "MATCHED" ? "failure" : oracle === "NOT MATCHED" ? "positive" : "muted" },
    { label: outcomeLabel, value: run.outcome ?? "NOT ASSIGNED", tone: outcomeTone(run.outcome) },
  ];

  return (
    <dl className="evidence-summary" aria-label={`${run.run_type === "BASELINE" ? "Baseline" : "What-if"} replay evidence`}>
      {rows.map((row) => (
        <div className="evidence-summary__row" key={row.label}>
          <dt>{row.label}</dt>
          <dd data-tone={row.tone}>{row.value}</dd>
        </div>
      ))}
    </dl>
  );
}
