import { z } from "zod";

const nonEmptyString = z.string().min(1);
const nonNegativeInteger = z.number().int().nonnegative();
const nullableNonEmptyString = nonEmptyString.nullish();
const timestampSchema = z.string().datetime({ offset: true });
const semverSchema = z
  .string()
  .regex(/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/);
const sha256Schema = z.string().regex(/^[a-f0-9]{64}$/);
const jsonObjectSchema = z.record(z.string(), z.json());

export const operationKindSchema = z.enum([
  "ENTRYPOINT",
  "INTERNAL",
  "DEPENDENCY",
  "STATE_CHANGE",
  "SIDE_EFFECT",
  "CONTROL",
]);

export const eventTypeSchema = z.enum([
  "START",
  "COMPLETE",
  "ERROR",
  "TIMEOUT",
  "RETRY",
  "EFFECT",
  "STATE_OBSERVATION",
]);

export const eventStatusSchema = z.enum([
  "RUNNING",
  "SUCCESS",
  "FAILED",
  "TIMEOUT",
  "BLOCKED",
]);

export const componentRefSchema = z
  .object({
    name: nonEmptyString,
    instance: nonEmptyString,
  })
  .strict();

export const operationRefSchema = z
  .object({
    name: nonEmptyString,
    kind: operationKindSchema,
  })
  .strict();

export const systemPackRefSchema = z
  .object({
    id: nonEmptyString,
    version: semverSchema,
    interface_version: z.literal("1.0"),
  })
  .strict();

export const failureOracleRefSchema = z
  .object({
    id: nonEmptyString,
    version: semverSchema,
  })
  .strict();

const eventAttributesSchema = z
  .object({
    configured_latency_ms: nonNegativeInteger.optional(),
    checkout_timeout_ms: z.number().int().min(1).optional(),
    effect_id: nonEmptyString.optional(),
    effect_committed: z.boolean().optional(),
    dependency_name: nonEmptyString.optional(),
  })
  .strict();

export const executionEventSchema = z
  .object({
    schema_version: z.literal("1.0"),
    event_id: nonEmptyString,
    execution_id: nonEmptyString,
    trace_id: nonEmptyString,
    parent_event_id: nonEmptyString.nullish(),
    replay_run_id: nonEmptyString.nullish(),
    component: componentRefSchema,
    operation: operationRefSchema,
    event_type: eventTypeSchema,
    attempt: z.number().int().min(1),
    logical_operation_id: nonEmptyString,
    occurred_at: z.string().datetime({ offset: true }),
    sequence: nonNegativeInteger,
    duration_ms: nonNegativeInteger.nullish(),
    status: eventStatusSchema,
    attributes: eventAttributesSchema,
  })
  .strict();

export type ExecutionEvent = z.infer<typeof executionEventSchema>;

export const errorCodeSchema = z.enum([
  "SCHEMA_INVALID",
  "INTEGRITY_MISMATCH",
  "PACK_UNAVAILABLE",
  "FIXTURE_MISSING",
  "SANITIZATION_FAILED",
  "ISOLATION_VIOLATION",
  "DESTINATION_BLOCKED",
  "GRAPH_CYCLE",
  "ORACLE_UNAVAILABLE",
  "INTERVENTION_INVALID",
  "INTERNAL_FAILURE",
]);

export const validationIssueSchema = z
  .object({
    code: errorCodeSchema,
    path: z.string(),
    message: nonEmptyString,
  })
  .strict();

export const runErrorSchema = z
  .object({
    code: errorCodeSchema,
    message: nonEmptyString,
    retryable: z.boolean(),
    details: jsonObjectSchema,
  })
  .strict();

export const incidentStatusSchema = z.enum(["DETECTED", "READY", "BLOCKED"]);

export const incidentSchema = z
  .object({
    schema_version: z.literal("1.0"),
    incident_id: nonEmptyString,
    status: incidentStatusSchema,
    block_reason: validationIssueSchema.nullish(),
    failure_oracle: failureOracleRefSchema,
    system_pack: systemPackRefSchema,
    trace_id: nonEmptyString,
    execution_id: nonEmptyString,
    detected_at: timestampSchema,
    summary: nonEmptyString,
    evidence_event_ids: z.array(nonEmptyString).min(1),
    graph_id: nullableNonEmptyString,
    sanitization_status: z.enum(["PASS", "FAIL"]),
  })
  .strict()
  .superRefine((incident, context) => {
    if (incident.status === "BLOCKED" && !incident.block_reason) {
      context.addIssue({ code: "custom", message: "BLOCKED incidents require block_reason", path: ["block_reason"] });
    }
    if (incident.status !== "BLOCKED" && incident.block_reason) {
      context.addIssue({ code: "custom", message: "Only BLOCKED incidents may include block_reason", path: ["block_reason"] });
    }
    if (incident.status === "READY" && (!incident.graph_id || incident.sanitization_status !== "PASS")) {
      context.addIssue({ code: "custom", message: "READY incidents require a graph and passing sanitization" });
    }
  });

export const graphEdgeTypeSchema = z.enum([
  "PARENT_CHILD",
  "TEMPORAL",
  "DEPENDENCY",
  "RETRY",
  "STATE",
  "SIDE_EFFECT",
]);

const graphNodeSchema = z
  .object({
    event_id: nonEmptyString,
    timeline_index: nonNegativeInteger,
  })
  .strict();

const graphEdgeSchema = z
  .object({
    edge_id: nonEmptyString,
    from_event_id: nonEmptyString,
    to_event_id: nonEmptyString,
    type: graphEdgeTypeSchema,
  })
  .strict()
  .refine((edge) => edge.from_event_id !== edge.to_event_id, {
    message: "Graph edges cannot reference the same source and target event",
  });

export const executionGraphSchema = z
  .object({
    schema_version: z.literal("1.0"),
    graph_id: nonEmptyString,
    incident_id: nonEmptyString,
    ordering_policy_version: z.literal("1.0"),
    nodes: z.array(graphNodeSchema),
    edges: z.array(graphEdgeSchema),
  })
  .strict()
  .superRefine((graph, context) => {
    const eventIds = new Set(graph.nodes.map((node) => node.event_id));
    const timelineIndexes = new Set<number>();

    graph.nodes.forEach((node, index) => {
      if (timelineIndexes.has(node.timeline_index)) {
        context.addIssue({
          code: "custom",
          message: "timeline_index must be unique",
          path: ["nodes", index, "timeline_index"],
        });
      }
      timelineIndexes.add(node.timeline_index);
    });

    graph.edges.forEach((edge, index) => {
      if (!eventIds.has(edge.from_event_id) || !eventIds.has(edge.to_event_id)) {
        context.addIssue({ code: "custom", message: "Graph edge references a missing node", path: ["edges", index] });
      }
    });
  });

export const effectSummarySchema = z
  .object({
    payment_attempt_count: nonNegativeInteger,
    ledger_commit_count: nonNegativeInteger,
  })
  .strict();

export const effectDeltaSchema = z
  .object({
    payment_attempt_count: z.number().int(),
    ledger_commit_count: z.number().int(),
  })
  .strict();

export const oracleResultSchema = z
  .object({
    oracle: failureOracleRefSchema,
    matched: z.boolean(),
    effect_summary: effectSummarySchema,
    required_evidence_event_ids: z.array(nonEmptyString),
    explanation: nonEmptyString,
  })
  .strict();

export const interventionSchema = z
  .object({
    type: z.literal("PAYMENT_LATENCY"),
    from: z.literal(350),
    to: z.literal(50),
    unit: z.literal("ms"),
  })
  .strict();

export const interventionSpecSchema = z
  .object({
    type: z.literal("PAYMENT_LATENCY"),
    value_type: z.literal("INTEGER"),
    unit: z.literal("ms"),
    minimum: nonNegativeInteger,
    maximum: nonNegativeInteger,
  })
  .strict()
  .refine((specification) => specification.minimum <= specification.maximum, {
    message: "minimum must not exceed maximum",
  });

const stateFixtureSchema = z
  .object({
    fixture_id: nonEmptyString,
    kind: z.literal("POSTGRES_ROWSET"),
    content_ref: nonEmptyString,
    content_digest: sha256Schema,
    sanitization_status: z.literal("PASS"),
    reset_strategy: z.literal("TRUNCATE_AND_LOAD"),
  })
  .strict();

const dependencyFixtureSchema = z
  .object({
    fixture_id: nonEmptyString,
    dependency: z.literal("payment_simulator"),
    request_match: jsonObjectSchema,
    response: jsonObjectSchema,
    latency_ms: nonNegativeInteger,
    failure_mode: z.literal("NONE"),
    invocation_limit: z.number().int().min(1),
  })
  .strict();

export const replayCapsuleSchema = z
  .object({
    schema_version: z.literal("1.0"),
    capsule_id: nonEmptyString,
    created_at: timestampSchema,
    source: z
      .object({
        incident_id: nonEmptyString,
        trace_id: nonEmptyString,
        execution_id: nonEmptyString,
        capture_environment: z.literal("DEMO"),
        captured_at: timestampSchema,
      })
      .strict(),
    system_pack: systemPackRefSchema,
    trigger: z
      .object({
        request_or_message: jsonObjectSchema,
        sanitized_headers: z.record(z.string(), z.string()),
      })
      .strict(),
    event_ids: z.array(nonEmptyString).min(1),
    graph_id: nonEmptyString,
    state_fixtures: z.array(stateFixtureSchema),
    dependency_fixtures: z.array(dependencyFixtureSchema),
    timing_policy: z
      .object({
        clock_tolerance_ms: z.literal(5),
        timeout_ms: z.literal(200),
      })
      .strict(),
    replay_plan: z
      .object({
        entrypoint: z.literal("gateway.checkout"),
        required_components: z.tuple([
          z.literal("gateway"),
          z.literal("checkout"),
          z.literal("payment"),
          z.literal("ledger"),
        ]),
        fixture_load_order: z.array(nonEmptyString),
        reset_strategy: z.literal("GOLDEN_RESET_V1"),
      })
      .strict(),
    failure_oracle: z
      .object({
        id: z.literal("duplicate_ledger_effect"),
        version: z.literal("1.0.0"),
        expected_match: z.literal(true),
        expected_effect_summary: z
          .object({
            payment_attempt_count: z.literal(2),
            ledger_commit_count: z.literal(2),
          })
          .strict(),
      })
      .strict(),
    allowed_interventions: z.array(interventionSpecSchema),
    safety: z
      .object({
        policy_version: z.literal("1.0"),
        sanitization_status: z.literal("PASS"),
        blocked_destinations: z.array(nonEmptyString).min(1),
        allowed_destinations: z.array(nonEmptyString),
        credential_profile: z.literal("replay-only"),
      })
      .strict(),
    integrity: z
      .object({
        algorithm: z.literal("SHA-256"),
        digest: sha256Schema,
      })
      .strict(),
  })
  .strict();

export const dependencyInteractionSchema = z
  .object({
    dependency: nonEmptyString,
    destination: nonEmptyString,
    operation: nonEmptyString,
    result: z.enum(["SIMULATED", "ALLOWED", "DENIED"]),
  })
  .strict();

export const isolationEvidenceSchema = z
  .object({
    policy_version: z.literal("1.0"),
    verdict: z.enum(["PASS", "FAIL"]),
    runtime_namespace: nonEmptyString,
    network_policy: z.enum(["PASS", "FAIL"]),
    credential_profile: z.literal("replay-only"),
    datastore_destinations: z.array(nonEmptyString),
    simulator_interactions: z.array(dependencyInteractionSchema),
    denied_interactions: z.array(dependencyInteractionSchema),
    teardown_result: z.enum(["PASS", "FAIL"]),
  })
  .strict()
  .superRefine((evidence, context) => {
    const shouldFail =
      evidence.network_policy === "FAIL" ||
      evidence.teardown_result === "FAIL" ||
      evidence.denied_interactions.length > 0;
    if (shouldFail && evidence.verdict !== "FAIL") {
      context.addIssue({ code: "custom", message: "Isolation violations require verdict FAIL", path: ["verdict"] });
    }
  });

export const replayRunStatusSchema = z.enum([
  "CREATED",
  "VALIDATING",
  "RUNNING",
  "COMPLETED",
  "FAILED",
  "BLOCKED",
]);

export const replayOutcomeSchema = z.enum([
  "REPRODUCED",
  "NOT_REPRODUCED",
  "MITIGATED",
  "UNCHANGED",
  "INCONCLUSIVE",
]);

export const replayRunSchema = z
  .object({
    schema_version: z.literal("1.0"),
    run_id: nonEmptyString,
    execution_id: nonEmptyString,
    capsule_id: nonEmptyString,
    capsule_hash: sha256Schema,
    run_type: z.enum(["BASELINE", "WHAT_IF"]),
    baseline_run_id: nullableNonEmptyString,
    intervention: interventionSchema.nullish(),
    trial_number: z.number().int().min(1),
    status: replayRunStatusSchema,
    outcome: replayOutcomeSchema.nullish(),
    started_at: timestampSchema.nullish(),
    completed_at: timestampSchema.nullish(),
    observed_event_ids: z.array(nonEmptyString),
    effect_summary: effectSummarySchema.nullish(),
    failure_oracle_result: oracleResultSchema.nullish(),
    isolation_evidence: isolationEvidenceSchema.nullish(),
    error: runErrorSchema.nullish(),
  })
  .strict()
  .superRefine((run, context) => {
    const terminal = ["COMPLETED", "FAILED", "BLOCKED"].includes(run.status);
    if (terminal && !run.completed_at) {
      context.addIssue({ code: "custom", message: "Terminal runs require completed_at", path: ["completed_at"] });
    }

    if (run.status === "COMPLETED") {
      if (!run.outcome || !run.effect_summary || !run.failure_oracle_result || run.isolation_evidence?.verdict !== "PASS") {
        context.addIssue({ code: "custom", message: "COMPLETED runs require outcome, evidence, and passing isolation" });
      }
    } else if (run.outcome) {
      context.addIssue({ code: "custom", message: "Only COMPLETED runs may include outcome", path: ["outcome"] });
    }

    const requiresError = run.status === "FAILED" || run.status === "BLOCKED";
    if (requiresError !== Boolean(run.error)) {
      context.addIssue({ code: "custom", message: "FAILED and BLOCKED runs require error; other statuses forbid it" });
    }

    if (run.run_type === "BASELINE") {
      if (run.baseline_run_id || run.intervention) {
        context.addIssue({ code: "custom", message: "BASELINE runs cannot reference a baseline or intervention" });
      }
      if (run.outcome && !["REPRODUCED", "NOT_REPRODUCED", "INCONCLUSIVE"].includes(run.outcome)) {
        context.addIssue({ code: "custom", message: "Outcome is not valid for a BASELINE run", path: ["outcome"] });
      }
    } else {
      if (!run.baseline_run_id || !run.intervention) {
        context.addIssue({ code: "custom", message: "WHAT_IF runs require baseline_run_id and intervention" });
      }
      if (run.outcome && !["MITIGATED", "UNCHANGED", "INCONCLUSIVE"].includes(run.outcome)) {
        context.addIssue({ code: "custom", message: "Outcome is not valid for a WHAT_IF run", path: ["outcome"] });
      }
    }
  });

const eventAlignmentSchema = z
  .object({
    baseline_event_id: nonEmptyString,
    comparison_event_id: nonEmptyString,
  })
  .strict();

const eventChangeSchema = z
  .object({
    baseline_event_id: nonEmptyString,
    comparison_event_id: nonEmptyString,
    field: nonEmptyString,
    baseline_value: z.json(),
    comparison_value: z.json(),
  })
  .strict();

const firstDivergenceSchema = z
  .object({
    baseline_event_id: nullableNonEmptyString,
    comparison_event_id: nullableNonEmptyString,
    rule: nonEmptyString,
    baseline_value: z.json(),
    comparison_value: z.json(),
    baseline_timeline_index: nonNegativeInteger,
    comparison_timeline_index: nonNegativeInteger,
  })
  .strict();

export const replayDiffSchema = z
  .object({
    schema_version: z.literal("1.0"),
    diff_id: nonEmptyString,
    baseline_run_id: nonEmptyString,
    comparison_run_id: nonEmptyString,
    alignment_version: z.literal("1.0"),
    intervention: interventionSchema,
    baseline_oracle_result: oracleResultSchema,
    comparison_oracle_result: oracleResultSchema,
    matched_events: z.array(eventAlignmentSchema),
    added_event_ids: z.array(nonEmptyString),
    removed_event_ids: z.array(nonEmptyString),
    changed_events: z.array(eventChangeSchema),
    first_meaningful_divergence: firstDivergenceSchema.nullish(),
    baseline_effect_summary: effectSummarySchema,
    comparison_effect_summary: effectSummarySchema,
    effect_delta: effectDeltaSchema,
    evidence_summary: nonEmptyString,
    limitations: z.array(nonEmptyString),
  })
  .strict();

export const resetRequestSchema = z
  .object({ scenario_id: z.literal("checkout_duplicate_effect") })
  .strict();

export const resetResultSchema = z
  .object({
    schema_version: z.literal("1.0"),
    reset_id: nonEmptyString,
    status: z.enum(["COMPLETED", "FAILED"]),
    cleared_incident_count: nonNegativeInteger,
    cleared_run_count: nonNegativeInteger,
    cleared_ledger_count: nonNegativeInteger,
    fixture_version: z.literal("1.0.0"),
    configured_latency_ms: z.literal(350),
    deduplication_enabled: z.literal(false),
    next_logical_operation_id: z.literal("checkout-8271"),
    error: runErrorSchema.nullish(),
  })
  .strict()
  .superRefine((result, context) => {
    if ((result.status === "FAILED") !== Boolean(result.error)) {
      context.addIssue({ code: "custom", message: "Only failed resets require an error", path: ["error"] });
    }
  });

export const acceptedEventResponseSchema = z
  .object({ event_id: nonEmptyString, status: z.literal("ACCEPTED") })
  .strict();

export const incidentListResponseSchema = z
  .object({
    items: z.array(incidentSchema),
    next_cursor: nullableNonEmptyString,
  })
  .strict();

export const incidentListQuerySchema = z
  .object({
    status: incidentStatusSchema.nullish(),
    cursor: nullableNonEmptyString,
    limit: z.number().int().min(1).max(100).default(20),
  })
  .strict();

export const incidentDetailResponseSchema = z
  .object({
    incident: incidentSchema,
    graph: executionGraphSchema,
    events: z.array(executionEventSchema),
  })
  .strict();

export const createRunRequestSchema = z
  .object({
    run_type: z.enum(["BASELINE", "WHAT_IF"]),
    baseline_run_id: nullableNonEmptyString,
    intervention: interventionSchema.nullish(),
  })
  .strict()
  .superRefine((request, context) => {
    if (request.run_type === "BASELINE" && (request.baseline_run_id || request.intervention)) {
      context.addIssue({ code: "custom", message: "BASELINE requests omit baseline_run_id and intervention" });
    }
    if (request.run_type === "WHAT_IF" && (!request.baseline_run_id || !request.intervention)) {
      context.addIssue({ code: "custom", message: "WHAT_IF requests require baseline_run_id and intervention" });
    }
  });

export const createDiffRequestSchema = z
  .object({
    baseline_run_id: nonEmptyString,
    comparison_run_id: nonEmptyString,
  })
  .strict();

export const apiErrorResponseSchema = z.object({ error: runErrorSchema }).strict();

export type Incident = z.infer<typeof incidentSchema>;
export type ExecutionGraph = z.infer<typeof executionGraphSchema>;
export type ReplayCapsule = z.infer<typeof replayCapsuleSchema>;
export type Intervention = z.infer<typeof interventionSchema>;
export type IsolationEvidence = z.infer<typeof isolationEvidenceSchema>;
export type ReplayRun = z.infer<typeof replayRunSchema>;
export type ReplayDiff = z.infer<typeof replayDiffSchema>;
export type ResetRequest = z.infer<typeof resetRequestSchema>;
export type ResetResult = z.infer<typeof resetResultSchema>;
export type CreateRunRequest = z.infer<typeof createRunRequestSchema>;
export type CreateDiffRequest = z.infer<typeof createDiffRequestSchema>;
export type APIErrorResponse = z.infer<typeof apiErrorResponseSchema>;
export type AcceptedEventResponse = z.infer<typeof acceptedEventResponseSchema>;
export type IncidentListResponse = z.infer<typeof incidentListResponseSchema>;
export type IncidentListQuery = z.input<typeof incidentListQuerySchema>;
export type IncidentDetailResponse = z.infer<typeof incidentDetailResponseSchema>;
