import type {
  IsolationEvidence,
  ReplayCapsule,
  ReplayDiff,
  ReplayRun,
  ResetResult,
} from "../../lib/contracts";

export const developmentCapsule = {
  schema_version: "1.0",
  capsule_id: "cap-8271",
  created_at: "2026-08-29T10:33:00Z",
  source: {
    incident_id: "inc-8271",
    trace_id: "trace-8271",
    execution_id: "exec-original-8271",
    capture_environment: "DEMO",
    captured_at: "2026-08-29T10:32:01.561Z",
  },
  system_pack: {
    id: "checkout_duplicate_effect",
    version: "1.0.0",
    interface_version: "1.0",
  },
  trigger: {
    request_or_message: {
      method: "POST",
      path: "/checkout",
      body: { checkout_id: "checkout-8271", amount_minor: 4999, currency: "INR" },
    },
    sanitized_headers: { "content-type": "application/json" },
  },
  event_ids: ["evt-timeout", "evt-retry", "evt-ledger-1", "evt-ledger-2"],
  graph_id: "graph-8271",
  state_fixtures: [
    {
      fixture_id: "state-ledger-empty",
      kind: "POSTGRES_ROWSET",
      content_ref: "fixture://golden/ledger-empty-v1",
      content_digest: "b".repeat(64),
      sanitization_status: "PASS",
      reset_strategy: "TRUNCATE_AND_LOAD",
    },
  ],
  dependency_fixtures: [
    {
      fixture_id: "dependency-payment-350ms",
      dependency: "payment_simulator",
      request_match: { logical_operation_id: "checkout-8271" },
      response: { status: "APPROVED" },
      latency_ms: 350,
      failure_mode: "NONE",
      invocation_limit: 2,
    },
  ],
  timing_policy: { clock_tolerance_ms: 5, timeout_ms: 200 },
  replay_plan: {
    entrypoint: "gateway.checkout",
    required_components: ["gateway", "checkout", "payment", "ledger"],
    fixture_load_order: ["state-ledger-empty", "dependency-payment-350ms"],
    reset_strategy: "GOLDEN_RESET_V1",
  },
  failure_oracle: {
    id: "duplicate_ledger_effect",
    version: "1.0.0",
    expected_match: true,
    expected_effect_summary: { payment_attempt_count: 2, ledger_commit_count: 2 },
  },
  allowed_interventions: [
    { type: "PAYMENT_LATENCY", value_type: "INTEGER", unit: "ms", minimum: 0, maximum: 5000 },
  ],
  safety: {
    policy_version: "1.0",
    sanitization_status: "PASS",
    blocked_destinations: ["production-databases", "public-internet", "real-payment-provider"],
    allowed_destinations: ["payment-simulator", "replay-postgres"],
    credential_profile: "replay-only",
  },
  integrity: { algorithm: "SHA-256", digest: "a".repeat(64) },
} satisfies ReplayCapsule;

const baselineCore = {
  schema_version: "1.0",
  run_id: "run-base-8271",
  execution_id: "exec-replay-base-8271",
  capsule_id: "cap-8271",
  capsule_hash: "a".repeat(64),
  run_type: "BASELINE",
  trial_number: 1,
} as const;

const baselineIsolation: IsolationEvidence = {
  policy_version: "1.0",
  verdict: "PASS",
  runtime_namespace: "replay-run-base-8271",
  network_policy: "PASS",
  credential_profile: "replay-only",
  datastore_destinations: ["postgres://replay/ledger_run_base_8271"],
  simulator_interactions: [
    {
      dependency: "payment_simulator",
      destination: "http://payment-simulator:8080",
      operation: "authorize",
      result: "SIMULATED",
    },
  ],
  denied_interactions: [],
  teardown_result: "PASS",
};

export const developmentBaselineSnapshots: ReplayRun[] = [
  { ...baselineCore, status: "CREATED", observed_event_ids: [] },
  { ...baselineCore, status: "VALIDATING", observed_event_ids: [] },
  {
    ...baselineCore,
    status: "RUNNING",
    started_at: "2026-08-29T10:34:00Z",
    observed_event_ids: ["evt-replay-payment-start"],
  },
  {
    ...baselineCore,
    status: "COMPLETED",
    outcome: "REPRODUCED",
    started_at: "2026-08-29T10:34:00Z",
    completed_at: "2026-08-29T10:34:00.561Z",
    observed_event_ids: [
      "evt-replay-timeout",
      "evt-replay-retry",
      "evt-replay-ledger-1",
      "evt-replay-ledger-2",
    ],
    effect_summary: { payment_attempt_count: 2, ledger_commit_count: 2 },
    failure_oracle_result: {
      oracle: { id: "duplicate_ledger_effect", version: "1.0.0" },
      matched: true,
      effect_summary: { payment_attempt_count: 2, ledger_commit_count: 2 },
      required_evidence_event_ids: [
        "evt-replay-timeout",
        "evt-replay-retry",
        "evt-replay-ledger-1",
        "evt-replay-ledger-2",
      ],
      explanation: "Baseline reproduced the timeout-driven duplicate ledger effect.",
    },
    isolation_evidence: baselineIsolation,
  },
];

const whatIfCore = {
  schema_version: "1.0",
  run_id: "run-whatif-8271",
  execution_id: "exec-replay-whatif-8271",
  capsule_id: "cap-8271",
  capsule_hash: "a".repeat(64),
  run_type: "WHAT_IF",
  baseline_run_id: "run-base-8271",
  intervention: { type: "PAYMENT_LATENCY", from: 350, to: 50, unit: "ms" },
  trial_number: 1,
} as const;

export const developmentWhatIfSnapshots: ReplayRun[] = [
  { ...whatIfCore, status: "CREATED", observed_event_ids: [] },
  { ...whatIfCore, status: "VALIDATING", observed_event_ids: [] },
  {
    ...whatIfCore,
    status: "RUNNING",
    started_at: "2026-08-29T10:35:00Z",
    observed_event_ids: ["evt-whatif-payment-start"],
  },
  {
    ...whatIfCore,
    status: "COMPLETED",
    outcome: "MITIGATED",
    started_at: "2026-08-29T10:35:00Z",
    completed_at: "2026-08-29T10:35:00.067Z",
    observed_event_ids: ["evt-whatif-payment-complete", "evt-whatif-ledger-1"],
    effect_summary: { payment_attempt_count: 1, ledger_commit_count: 1 },
    failure_oracle_result: {
      oracle: { id: "duplicate_ledger_effect", version: "1.0.0" },
      matched: false,
      effect_summary: { payment_attempt_count: 1, ledger_commit_count: 1 },
      required_evidence_event_ids: ["evt-whatif-payment-complete", "evt-whatif-ledger-1"],
      explanation: "Payment completed before timeout; no retry or duplicate effect occurred.",
    },
    isolation_evidence: {
      ...baselineIsolation,
      runtime_namespace: "replay-run-whatif-8271",
      datastore_destinations: ["postgres://replay/ledger_run_whatif_8271"],
    },
  },
];

export const developmentReplayDiff = {
  schema_version: "1.0",
  diff_id: "diff-8271",
  baseline_run_id: "run-base-8271",
  comparison_run_id: "run-whatif-8271",
  alignment_version: "1.0",
  intervention: { type: "PAYMENT_LATENCY", from: 350, to: 50, unit: "ms" },
  baseline_oracle_result: developmentBaselineSnapshots[3].failure_oracle_result!,
  comparison_oracle_result: developmentWhatIfSnapshots[3].failure_oracle_result!,
  matched_events: [
    { baseline_event_id: "evt-replay-ledger-1", comparison_event_id: "evt-whatif-ledger-1" },
  ],
  added_event_ids: [],
  removed_event_ids: ["evt-replay-timeout", "evt-replay-retry", "evt-replay-ledger-2"],
  changed_events: [],
  first_meaningful_divergence: {
    baseline_event_id: "evt-replay-timeout",
    comparison_event_id: "evt-whatif-payment-complete",
    rule: "PAYMENT_COMPLETES_BEFORE_TIMEOUT",
    baseline_value: "TIMEOUT",
    comparison_value: "SUCCESS",
    baseline_timeline_index: 3,
    comparison_timeline_index: 3,
  },
  baseline_effect_summary: { payment_attempt_count: 2, ledger_commit_count: 2 },
  comparison_effect_summary: { payment_attempt_count: 1, ledger_commit_count: 1 },
  effect_delta: { payment_attempt_count: -1, ledger_commit_count: -1 },
  evidence_summary: "Lower payment latency completed before checkout timeout; retry and duplicate effect disappeared.",
  limitations: ["Applies to checkout_duplicate_effect pack v1.0.0 fixtures."],
} satisfies ReplayDiff;

export const developmentResetResult = {
  schema_version: "1.0",
  reset_id: "reset-1",
  status: "COMPLETED",
  cleared_incident_count: 1,
  cleared_run_count: 2,
  cleared_ledger_count: 3,
  fixture_version: "1.0.0",
  configured_latency_ms: 350,
  deduplication_enabled: false,
  next_logical_operation_id: "checkout-8271",
} satisfies ResetResult;
