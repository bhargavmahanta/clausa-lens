import type { ReplayCapsule } from "../../lib/contracts";

export function buildCapsuleView(capsule: ReplayCapsule) {
  const request = capsule.trigger.request_or_message;

  return {
    capsuleId: capsule.capsule_id,
    createdAt: capsule.created_at,
    pack: {
      id: capsule.system_pack.id,
      version: capsule.system_pack.version,
      interfaceVersion: capsule.system_pack.interface_version,
    },
    source: {
      incidentId: capsule.source.incident_id,
      traceId: capsule.source.trace_id,
      executionId: capsule.source.execution_id,
      captureEnvironment: capsule.source.capture_environment,
      capturedAt: capsule.source.captured_at,
    },
    triggerSummary:
      typeof request.method === "string" && typeof request.path === "string"
        ? `${request.method} ${request.path}`
        : "Request unavailable",
    sanitizedHeaders: capsule.trigger.sanitized_headers,
    eventIds: capsule.event_ids,
    graphId: capsule.graph_id,
    stateFixtures: capsule.state_fixtures.map((fixture) => ({
      fixtureId: fixture.fixture_id,
      kind: fixture.kind,
      contentRef: fixture.content_ref,
      contentDigest: fixture.content_digest,
      sanitizationStatus: fixture.sanitization_status,
      resetStrategy: fixture.reset_strategy,
    })),
    dependencyFixtures: capsule.dependency_fixtures.map((fixture) => ({
      fixtureId: fixture.fixture_id,
      dependency: fixture.dependency,
      requestMatch: fixture.request_match,
      response: fixture.response,
      latencyMS: fixture.latency_ms,
      failureMode: fixture.failure_mode,
      invocationLimit: fixture.invocation_limit,
    })),
    timing: {
      clockToleranceMS: capsule.timing_policy.clock_tolerance_ms,
      timeoutMS: capsule.timing_policy.timeout_ms,
    },
    plan: {
      entrypoint: capsule.replay_plan.entrypoint,
      requiredComponents: capsule.replay_plan.required_components,
      fixtureLoadOrder: capsule.replay_plan.fixture_load_order,
      resetStrategy: capsule.replay_plan.reset_strategy,
    },
    oracle: {
      id: capsule.failure_oracle.id,
      version: capsule.failure_oracle.version,
      expectedMatch: capsule.failure_oracle.expected_match,
      expectedEffectSummary: {
        paymentAttemptCount: capsule.failure_oracle.expected_effect_summary.payment_attempt_count,
        ledgerCommitCount: capsule.failure_oracle.expected_effect_summary.ledger_commit_count,
      },
    },
    allowedInterventions: capsule.allowed_interventions.map((intervention) => ({
      type: intervention.type,
      valueType: intervention.value_type,
      unit: intervention.unit,
      minimum: intervention.minimum,
      maximum: intervention.maximum,
    })),
    safety: {
      policyVersion: capsule.safety.policy_version,
      sanitizationStatus: capsule.safety.sanitization_status,
      blockedDestinations: capsule.safety.blocked_destinations,
      allowedDestinations: capsule.safety.allowed_destinations,
      credentialProfile: capsule.safety.credential_profile,
    },
    integrity: capsule.integrity,
  };
}

export type CapsuleView = ReturnType<typeof buildCapsuleView>;
