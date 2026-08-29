import { z } from "zod";

const contractString = z.string();
const identifierString = z.string().min(1);
const nonNegativeInteger = z.number().int().nonnegative();
const optionalIdentifier = identifierString.optional();
const timestampSchema = z.string().datetime({ offset: false });
const semverSchema = z
  .string()
  .regex(
    /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$/,
  );
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
    name: contractString,
    instance: contractString,
  })
  .strict();

export const operationRefSchema = z
  .object({
    name: contractString,
    kind: operationKindSchema,
  })
  .strict();

export const systemPackRefSchema = z
  .object({
    id: identifierString,
    version: semverSchema,
    interface_version: z.literal("1.0"),
  })
  .strict();

export const failureOracleRefSchema = z
  .object({
    id: identifierString,
    version: semverSchema,
  })
  .strict();

const eventAttributesSchema = z
  .object({
    configured_latency_ms: nonNegativeInteger.optional(),
    checkout_timeout_ms: z.number().int().min(1).optional(),
    effect_id: identifierString.optional(),
    effect_committed: z.boolean().optional(),
    dependency_name: contractString.optional(),
  })
  .strict();

export const executionEventSchema = z
  .object({
    schema_version: z.literal("1.0"),
    event_id: identifierString,
    execution_id: identifierString,
    trace_id: identifierString,
    parent_event_id: identifierString.optional(),
    replay_run_id: identifierString.optional(),
    component: componentRefSchema,
    operation: operationRefSchema,
    event_type: eventTypeSchema,
    attempt: z.number().int().min(1),
    logical_operation_id: identifierString,
    occurred_at: timestampSchema,
    sequence: nonNegativeInteger,
    duration_ms: nonNegativeInteger.optional(),
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
    message: contractString,
  })
  .strict();

export const runErrorSchema = z
  .object({
    code: errorCodeSchema,
    message: contractString,
    retryable: z.boolean(),
    details: jsonObjectSchema,
  })
  .strict();

export const incidentStatusSchema = z.enum(["DETECTED", "READY", "BLOCKED"]);

export const incidentSchema = z
  .object({
    schema_version: z.literal("1.0"),
    incident_id: identifierString,
    status: incidentStatusSchema,
    block_reason: validationIssueSchema.optional(),
    failure_oracle: failureOracleRefSchema,
    system_pack: systemPackRefSchema,
    trace_id: identifierString,
    execution_id: identifierString,
    detected_at: timestampSchema,
    summary: contractString,
    evidence_event_ids: z.array(identifierString).min(1),
    graph_id: optionalIdentifier,
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
    event_id: identifierString,
    timeline_index: nonNegativeInteger,
  })
  .strict();

const graphEdgeSchema = z
  .object({
    edge_id: identifierString,
    from_event_id: identifierString,
    to_event_id: identifierString,
    type: graphEdgeTypeSchema,
  })
  .strict()
  .refine((edge) => edge.from_event_id !== edge.to_event_id, {
    message: "Graph edges cannot reference the same source and target event",
  });

export const executionGraphSchema = z
  .object({
    schema_version: z.literal("1.0"),
    graph_id: identifierString,
    incident_id: identifierString,
    ordering_policy_version: z.literal("1.0"),
    nodes: z.array(graphNodeSchema),
    edges: z.array(graphEdgeSchema),
  })
  .strict()
  .superRefine((graph, context) => {
    const eventIds = new Set(graph.nodes.map((node) => node.event_id));
    const timelineIndexByEventId = new Map(
      graph.nodes.map((node) => [node.event_id, node.timeline_index]),
    );
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

      const fromIndex = timelineIndexByEventId.get(edge.from_event_id);
      const toIndex = timelineIndexByEventId.get(edge.to_event_id);
      if (
        edge.type !== "TEMPORAL" &&
        fromIndex !== undefined &&
        toIndex !== undefined &&
        fromIndex >= toIndex
      ) {
        context.addIssue({
          code: "custom",
          message: "Hard ordering edges must move forward in timeline order",
          path: ["edges", index],
        });
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
    required_evidence_event_ids: z.array(identifierString),
    explanation: contractString,
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
    minimum: z.literal(0),
    maximum: z.literal(5000),
  })
  .strict();

const stateFixtureSchema = z
  .object({
    fixture_id: identifierString,
    kind: z.literal("POSTGRES_ROWSET"),
    content_ref: contractString,
    content_digest: sha256Schema,
    sanitization_status: z.literal("PASS"),
    reset_strategy: z.literal("TRUNCATE_AND_LOAD"),
  })
  .strict();

const dependencyFixtureSchema = z
  .object({
    fixture_id: identifierString,
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
    capsule_id: identifierString,
    created_at: timestampSchema,
    source: z
      .object({
        incident_id: identifierString,
        trace_id: identifierString,
        execution_id: identifierString,
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
    event_ids: z.array(identifierString).min(1),
    graph_id: identifierString,
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
        fixture_load_order: z.array(identifierString),
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
        blocked_destinations: z.array(contractString).min(1),
        allowed_destinations: z.array(contractString),
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
  .strict()
  .superRefine((capsule, context) => {
    const fixtureIds = [
      ...capsule.state_fixtures.map((fixture) => fixture.fixture_id),
      ...capsule.dependency_fixtures.map((fixture) => fixture.fixture_id),
    ];
    const fixtureIdSet = new Set(fixtureIds);
    const loadOrderSet = new Set(capsule.replay_plan.fixture_load_order);

    if (
      fixtureIdSet.size !== fixtureIds.length ||
      loadOrderSet.size !== capsule.replay_plan.fixture_load_order.length ||
      fixtureIdSet.size !== loadOrderSet.size ||
      [...fixtureIdSet].some((fixtureId) => !loadOrderSet.has(fixtureId))
    ) {
      context.addIssue({
        code: "custom",
        message: "fixture_load_order must reference every capsule fixture exactly once",
        path: ["replay_plan", "fixture_load_order"],
      });
    }
  });

export const dependencyInteractionSchema = z
  .object({
    dependency: contractString,
    destination: contractString,
    operation: contractString,
    result: z.enum(["SIMULATED", "ALLOWED", "DENIED"]),
  })
  .strict();

export const isolationEvidenceSchema = z
  .object({
    policy_version: z.literal("1.0"),
    verdict: z.enum(["PASS", "FAIL"]),
    runtime_namespace: contractString,
    network_policy: z.enum(["PASS", "FAIL"]),
    credential_profile: z.literal("replay-only"),
    datastore_destinations: z.array(contractString),
    simulator_interactions: z.array(dependencyInteractionSchema),
    denied_interactions: z.array(dependencyInteractionSchema),
    teardown_result: z.enum(["PASS", "FAIL"]),
  })
  .strict()
  .superRefine((evidence, context) => {
    const shouldFail =
      evidence.network_policy === "FAIL" ||
      evidence.teardown_result === "FAIL" ||
      evidence.denied_interactions.length > 0 ||
      evidence.simulator_interactions.some((interaction) => interaction.result === "DENIED");
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
    run_id: identifierString,
    execution_id: identifierString,
    capsule_id: identifierString,
    capsule_hash: sha256Schema,
    run_type: z.enum(["BASELINE", "WHAT_IF"]),
    baseline_run_id: optionalIdentifier,
    intervention: interventionSchema.optional(),
    trial_number: z.number().int().min(1),
    status: replayRunStatusSchema,
    outcome: replayOutcomeSchema.optional(),
    started_at: timestampSchema.optional(),
    completed_at: timestampSchema.optional(),
    observed_event_ids: z.array(identifierString),
    effect_summary: effectSummarySchema.optional(),
    failure_oracle_result: oracleResultSchema.optional(),
    isolation_evidence: isolationEvidenceSchema.optional(),
    error: runErrorSchema.optional(),
  })
  .strict()
  .superRefine((run, context) => {
    const terminal = ["COMPLETED", "FAILED", "BLOCKED"].includes(run.status);
    if (terminal && !run.completed_at) {
      context.addIssue({ code: "custom", message: "Terminal runs require completed_at", path: ["completed_at"] });
    }

    if (run.status === "COMPLETED") {
      if (
        !run.started_at ||
        !run.outcome ||
        !run.effect_summary ||
        !run.failure_oracle_result ||
        run.isolation_evidence?.verdict !== "PASS"
      ) {
        context.addIssue({ code: "custom", message: "COMPLETED runs require outcome, evidence, and passing isolation" });
      }
    } else if (run.outcome !== undefined) {
      context.addIssue({ code: "custom", message: "Only COMPLETED runs may include outcome", path: ["outcome"] });
    }

    const requiresError = run.status === "FAILED" || run.status === "BLOCKED";
    if (requiresError !== Boolean(run.error)) {
      context.addIssue({ code: "custom", message: "FAILED and BLOCKED runs require error; other statuses forbid it" });
    }

    if (
      run.status === "BLOCKED" &&
      (run.error?.code === "ISOLATION_VIOLATION" || run.error?.code === "DESTINATION_BLOCKED") &&
      !run.isolation_evidence
    ) {
      context.addIssue({
        code: "custom",
        message: "Isolation-related BLOCKED runs require isolation_evidence",
        path: ["isolation_evidence"],
      });
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

    const oracleMatched = run.failure_oracle_result?.matched;
    if (
      (run.outcome === "REPRODUCED" || run.outcome === "UNCHANGED") &&
      oracleMatched !== true
    ) {
      context.addIssue({
        code: "custom",
        message: `${run.outcome} requires a matching failure oracle`,
        path: ["failure_oracle_result", "matched"],
      });
    }
    if (
      (run.outcome === "NOT_REPRODUCED" || run.outcome === "MITIGATED") &&
      oracleMatched !== false
    ) {
      context.addIssue({
        code: "custom",
        message: `${run.outcome} requires a non-matching failure oracle`,
        path: ["failure_oracle_result", "matched"],
      });
    }
  });

const eventAlignmentSchema = z
  .object({
    baseline_event_id: identifierString,
    comparison_event_id: identifierString,
  })
  .strict();

const eventChangeSchema = z
  .object({
    baseline_event_id: identifierString,
    comparison_event_id: identifierString,
    field: contractString,
    baseline_value: z.json(),
    comparison_value: z.json(),
  })
  .strict();

const firstDivergenceSchema = z
  .object({
    baseline_event_id: optionalIdentifier,
    comparison_event_id: optionalIdentifier,
    rule: contractString,
    baseline_value: z.json(),
    comparison_value: z.json(),
    baseline_timeline_index: nonNegativeInteger,
    comparison_timeline_index: nonNegativeInteger,
  })
  .strict();

export const replayDiffSchema = z
  .object({
    schema_version: z.literal("1.0"),
    diff_id: identifierString,
    baseline_run_id: identifierString,
    comparison_run_id: identifierString,
    alignment_version: z.literal("1.0"),
    intervention: interventionSchema,
    baseline_oracle_result: oracleResultSchema,
    comparison_oracle_result: oracleResultSchema,
    matched_events: z.array(eventAlignmentSchema),
    added_event_ids: z.array(identifierString),
    removed_event_ids: z.array(identifierString),
    changed_events: z.array(eventChangeSchema),
    first_meaningful_divergence: firstDivergenceSchema.optional(),
    baseline_effect_summary: effectSummarySchema,
    comparison_effect_summary: effectSummarySchema,
    effect_delta: effectDeltaSchema,
    evidence_summary: contractString,
    limitations: z.array(contractString),
  })
  .strict()
  .superRefine((diff, context) => {
    if (!diff.baseline_oracle_result.matched) {
      context.addIssue({
        code: "custom",
        message: "ReplayDiff requires a reproduced baseline oracle result",
        path: ["baseline_oracle_result", "matched"],
      });
    }

    const expectedPaymentDelta =
      diff.comparison_effect_summary.payment_attempt_count -
      diff.baseline_effect_summary.payment_attempt_count;
    const expectedLedgerDelta =
      diff.comparison_effect_summary.ledger_commit_count -
      diff.baseline_effect_summary.ledger_commit_count;
    if (diff.effect_delta.payment_attempt_count !== expectedPaymentDelta) {
      context.addIssue({
        code: "custom",
        message: "payment_attempt_count delta must be comparison minus baseline",
        path: ["effect_delta", "payment_attempt_count"],
      });
    }
    if (diff.effect_delta.ledger_commit_count !== expectedLedgerDelta) {
      context.addIssue({
        code: "custom",
        message: "ledger_commit_count delta must be comparison minus baseline",
        path: ["effect_delta", "ledger_commit_count"],
      });
    }
  });

export const resetRequestSchema = z
  .object({ scenario_id: z.literal("checkout_duplicate_effect") })
  .strict();

export const resetResultSchema = z
  .object({
    schema_version: z.literal("1.0"),
    reset_id: identifierString,
    status: z.enum(["COMPLETED", "FAILED"]),
    cleared_incident_count: nonNegativeInteger,
    cleared_run_count: nonNegativeInteger,
    cleared_ledger_count: nonNegativeInteger,
    fixture_version: z.literal("1.0.0"),
    configured_latency_ms: z.literal(350),
    deduplication_enabled: z.literal(false),
    next_logical_operation_id: z.literal("checkout-8271"),
    error: runErrorSchema.optional(),
  })
  .strict()
  .superRefine((result, context) => {
    if ((result.status === "FAILED") !== Boolean(result.error)) {
      context.addIssue({ code: "custom", message: "Only failed resets require an error", path: ["error"] });
    }
  });

export const acceptedEventResponseSchema = z
  .object({ event_id: identifierString, status: z.literal("ACCEPTED") })
  .strict();

export const incidentListResponseSchema = z
  .object({
    items: z.array(incidentSchema),
    next_cursor: optionalIdentifier,
  })
  .strict();

export const incidentListQuerySchema = z
  .object({
    status: incidentStatusSchema.optional(),
    cursor: optionalIdentifier,
    limit: z.number().int().min(1).max(100).default(20),
  })
  .strict();

export const incidentDetailResponseSchema = z
  .object({
    incident: incidentSchema,
    graph: executionGraphSchema,
    events: z.array(executionEventSchema),
  })
  .strict()
  .superRefine((detail, context) => {
    const timelineEventIds = [...detail.graph.nodes]
      .sort((left, right) => left.timeline_index - right.timeline_index)
      .map((node) => node.event_id);
    const responseEventIds = detail.events.map((event) => event.event_id);

    if (
      timelineEventIds.length !== responseEventIds.length ||
      timelineEventIds.some((eventId, index) => eventId !== responseEventIds[index])
    ) {
      context.addIssue({
        code: "custom",
        message: "Incident events must match graph nodes in timeline order",
        path: ["events"],
      });
    }

    const eventIds = new Set(responseEventIds);
    detail.incident.evidence_event_ids.forEach((eventId, index) => {
      if (!eventIds.has(eventId)) {
        context.addIssue({
          code: "custom",
          message: "Incident evidence references must resolve to detail events",
          path: ["incident", "evidence_event_ids", index],
        });
      }
    });

    if (detail.graph.incident_id !== detail.incident.incident_id) {
      context.addIssue({
        code: "custom",
        message: "Incident graph must belong to the detail incident",
        path: ["graph", "incident_id"],
      });
    }

    detail.events.forEach((event, index) => {
      if (event.trace_id !== detail.incident.trace_id || event.execution_id !== detail.incident.execution_id) {
        context.addIssue({
          code: "custom",
          message: "Incident events must belong to the incident trace and execution",
          path: ["events", index],
        });
      }
    });
  });

export const createRunRequestSchema = z
  .object({
    run_type: z.enum(["BASELINE", "WHAT_IF"]),
    baseline_run_id: optionalIdentifier,
    intervention: interventionSchema.optional(),
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
    baseline_run_id: identifierString,
    comparison_run_id: identifierString,
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
