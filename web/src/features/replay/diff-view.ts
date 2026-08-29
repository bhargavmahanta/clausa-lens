import type { ReplayDiff } from "../../lib/contracts";

export function buildDiffView(diff: ReplayDiff) {
  const divergence = diff.first_meaningful_divergence;

  return {
    diffId: diff.diff_id,
    baselineRunId: diff.baseline_run_id,
    comparisonRunId: diff.comparison_run_id,
    alignmentVersion: diff.alignment_version,
    intervention: diff.intervention,
    effects: {
      baseline: {
        paymentAttemptCount: diff.baseline_effect_summary.payment_attempt_count,
        ledgerCommitCount: diff.baseline_effect_summary.ledger_commit_count,
      },
      comparison: {
        paymentAttemptCount: diff.comparison_effect_summary.payment_attempt_count,
        ledgerCommitCount: diff.comparison_effect_summary.ledger_commit_count,
      },
      delta: {
        paymentAttemptCount: diff.effect_delta.payment_attempt_count,
        ledgerCommitCount: diff.effect_delta.ledger_commit_count,
      },
    },
    oracles: {
      baselineMatched: diff.baseline_oracle_result.matched,
      comparisonMatched: diff.comparison_oracle_result.matched,
      baselineExplanation: diff.baseline_oracle_result.explanation,
      comparisonExplanation: diff.comparison_oracle_result.explanation,
    },
    matchedEventCount: diff.matched_events.length,
    addedEventCount: diff.added_event_ids.length,
    removedEventCount: diff.removed_event_ids.length,
    changedEventCount: diff.changed_events.length,
    firstDivergence: divergence
      ? {
          rule: divergence.rule,
          baselineEventId: divergence.baseline_event_id,
          comparisonEventId: divergence.comparison_event_id,
          baselineValue: divergence.baseline_value,
          comparisonValue: divergence.comparison_value,
          baselineTimelineIndex: divergence.baseline_timeline_index,
          comparisonTimelineIndex: divergence.comparison_timeline_index,
        }
      : undefined,
    evidenceSummary: diff.evidence_summary,
    limitations: diff.limitations,
  };
}

export type DiffView = ReturnType<typeof buildDiffView>;
