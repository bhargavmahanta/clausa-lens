import {
  createDiffRequestSchema,
  createRunRequestSchema,
  resetRequestSchema,
  type APIErrorResponse,
  type ReplayRun,
} from "../../lib/contracts";
import { developmentIncidentDetail, developmentIncidentList } from "../incidents/development-fixture";
import {
  developmentBaselineSnapshots,
  developmentCapsule,
  developmentReplayDiff,
  developmentResetResult,
  developmentWhatIfSnapshots,
} from "./development-fixture";

export type DevelopmentApiResult = { body: unknown; status: number };

type StoredRun = { snapshots: ReplayRun[]; index: number };

const runs = new Map<string, StoredRun>();
let incidentsVisible = true;
let diffCreated = false;

function errorResult(
  status: number,
  code: APIErrorResponse["error"]["code"],
  message: string,
  details: Record<string, unknown> = {},
): DevelopmentApiResult {
  return { body: { error: { code, message, retryable: false, details } }, status };
}

export function unavailableFixture(): DevelopmentApiResult {
  return errorResult(
    404,
    "FIXTURE_MISSING",
    "Development fixtures are unavailable outside development.",
  );
}

export function listDevelopmentIncidents(): DevelopmentApiResult {
  return {
    body: incidentsVisible ? developmentIncidentList : { items: [] },
    status: 200,
  };
}

export function getDevelopmentIncident(incidentId: string): DevelopmentApiResult {
  if (!incidentsVisible || incidentId !== developmentIncidentDetail.incident.incident_id) {
    return errorResult(404, "FIXTURE_MISSING", `No development fixture exists for incident ${incidentId}.`);
  }
  return { body: developmentIncidentDetail, status: 200 };
}

export function compileDevelopmentCapsule(incidentId: string): DevelopmentApiResult {
  if (!incidentsVisible || incidentId !== developmentCapsule.source.incident_id) {
    return errorResult(404, "FIXTURE_MISSING", `No development fixture exists for incident ${incidentId}.`);
  }
  return { body: developmentCapsule, status: 201 };
}

function terminalRun(runId: string): ReplayRun | undefined {
  const stored = runs.get(runId);
  const run = stored?.snapshots[stored.index];
  return run?.status === "COMPLETED" ? run : undefined;
}

export function createDevelopmentRun(capsuleId: string, body: unknown): DevelopmentApiResult {
  if (capsuleId !== developmentCapsule.capsule_id) {
    return errorResult(404, "FIXTURE_MISSING", `No development fixture exists for capsule ${capsuleId}.`);
  }

  const decoded = createRunRequestSchema.safeParse(body);
  if (!decoded.success) {
    const isWhatIf = typeof body === "object" && body !== null && "run_type" in body && body.run_type === "WHAT_IF";
    return errorResult(
      400,
      isWhatIf ? "INTERVENTION_INVALID" : "SCHEMA_INVALID",
      isWhatIf ? "The what-if intervention must be PAYMENT_LATENCY from 350 ms to 50 ms." : "The run request does not match the frozen contract.",
      isWhatIf ? { field: "intervention" } : { resource: "CreateRunRequest" },
    );
  }

  if (decoded.data.run_type === "BASELINE") {
    runs.set("run-base-8271", { snapshots: developmentBaselineSnapshots, index: 0 });
    return { body: developmentBaselineSnapshots[0], status: 202 };
  }

  const baseline = terminalRun(decoded.data.baseline_run_id!);
  if (
    !baseline ||
    baseline.outcome !== "REPRODUCED" ||
    baseline.isolation_evidence?.verdict !== "PASS" ||
    baseline.capsule_hash !== developmentCapsule.integrity.digest
  ) {
    return errorResult(409, "INTERVENTION_INVALID", "A safely reproduced matching baseline is required.");
  }

  runs.set("run-whatif-8271", { snapshots: developmentWhatIfSnapshots, index: 0 });
  return { body: developmentWhatIfSnapshots[0], status: 202 };
}

export function getDevelopmentRun(runId: string): DevelopmentApiResult {
  const stored = runs.get(runId);
  if (!stored) {
    return errorResult(404, "FIXTURE_MISSING", `No development fixture exists for run ${runId}.`);
  }
  stored.index = Math.min(stored.index + 1, stored.snapshots.length - 1);
  return { body: stored.snapshots[stored.index], status: 200 };
}

export function createDevelopmentDiff(body: unknown): DevelopmentApiResult {
  const decoded = createDiffRequestSchema.safeParse(body);
  if (!decoded.success) {
    return errorResult(400, "SCHEMA_INVALID", "The diff request does not match the frozen contract.");
  }
  const baseline = terminalRun(decoded.data.baseline_run_id);
  const comparison = terminalRun(decoded.data.comparison_run_id);
  if (!baseline || !comparison || baseline.outcome !== "REPRODUCED" || comparison.outcome !== "MITIGATED") {
    return errorResult(409, "INTERVENTION_INVALID", "Completed baseline and what-if runs are required.");
  }
  diffCreated = true;
  return { body: developmentReplayDiff, status: 201 };
}

export function getDevelopmentDiff(diffId: string): DevelopmentApiResult {
  if (!diffCreated || diffId !== developmentReplayDiff.diff_id) {
    return errorResult(404, "FIXTURE_MISSING", `No development fixture exists for diff ${diffId}.`);
  }
  return { body: developmentReplayDiff, status: 200 };
}

export function resetDevelopmentDemo(body: unknown): DevelopmentApiResult {
  const decoded = resetRequestSchema.safeParse(body);
  if (!decoded.success) {
    return errorResult(400, "SCHEMA_INVALID", "The reset request does not match the frozen contract.");
  }
  runs.clear();
  diffCreated = false;
  incidentsVisible = false;
  return { body: developmentResetResult, status: 200 };
}
