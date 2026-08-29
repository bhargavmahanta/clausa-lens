import { describe, expect, it, vi } from "vitest";

import {
  apiErrorResponseSchema,
  replayCapsuleSchema,
  replayDiffSchema,
  replayRunSchema,
  resetResultSchema,
} from "../../src/lib/contracts";

vi.stubEnv("NODE_ENV", "development");

const routeContext = <Name extends string>(name: Name, value: string) => ({
  params: Promise.resolve({ [name]: value } as Record<Name, string>),
});

function jsonRequest(url: string, body: unknown) {
  return new Request(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

describe("development replay fixture routes", () => {
  it("compiles a contract-valid capsule with POST and HTTP 201", async () => {
    const { POST } = await import(
      "../../src/app/api/dev/v1/incidents/[incidentId]/capsules/route"
    );

    const response = await POST(
      new Request("http://localhost/api/dev/v1/incidents/inc-8271/capsules", { method: "POST" }),
      routeContext("incidentId", "inc-8271"),
    );

    expect(response.status).toBe(201);
    expect(replayCapsuleSchema.parse(await response.json()).capsule_id).toBe("cap-8271");
  });

  it("creates and polls a baseline run without returning an early outcome", async () => {
    const { POST } = await import(
      "../../src/app/api/dev/v1/capsules/[capsuleId]/runs/route"
    );
    const { GET } = await import("../../src/app/api/dev/v1/runs/[runId]/route");
    const response = await POST(
      jsonRequest("http://localhost/api/dev/v1/capsules/cap-8271/runs", { run_type: "BASELINE" }),
      routeContext("capsuleId", "cap-8271"),
    );
    const created = replayRunSchema.parse(await response.json());

    expect(response.status).toBe(202);
    expect(created.status).toBe("CREATED");
    expect(created.outcome).toBeUndefined();

    const validating = replayRunSchema.parse(
      await (await GET(new Request("http://localhost"), routeContext("runId", created.run_id))).json(),
    );
    const running = replayRunSchema.parse(
      await (await GET(new Request("http://localhost"), routeContext("runId", created.run_id))).json(),
    );
    const completed = replayRunSchema.parse(
      await (await GET(new Request("http://localhost"), routeContext("runId", created.run_id))).json(),
    );

    expect([validating.status, running.status, completed.status]).toEqual([
      "VALIDATING",
      "RUNNING",
      "COMPLETED",
    ]);
    expect(completed.outcome).toBe("REPRODUCED");
  });

  it("rejects every what-if except PAYMENT_LATENCY 350 ms to 50 ms", async () => {
    const { POST } = await import(
      "../../src/app/api/dev/v1/capsules/[capsuleId]/runs/route"
    );
    const response = await POST(
      jsonRequest("http://localhost/api/dev/v1/capsules/cap-8271/runs", {
        run_type: "WHAT_IF",
        baseline_run_id: "run-base-8271",
        intervention: { type: "PAYMENT_LATENCY", from: 350, to: 51, unit: "ms" },
      }),
      routeContext("capsuleId", "cap-8271"),
    );
    const body = apiErrorResponseSchema.parse(await response.json());

    expect(response.status).toBe(400);
    expect(body.error.code).toBe("INTERVENTION_INVALID");
    expect(body.error.details).toEqual({ field: "intervention" });
  });

  it("creates the exact what-if and diff resources after the reproduced baseline", async () => {
    const { POST: createRun } = await import(
      "../../src/app/api/dev/v1/capsules/[capsuleId]/runs/route"
    );
    const { GET: getRun } = await import("../../src/app/api/dev/v1/runs/[runId]/route");
    const { POST: createDiff } = await import("../../src/app/api/dev/v1/diffs/route");

    const whatIfResponse = await createRun(
      jsonRequest("http://localhost/api/dev/v1/capsules/cap-8271/runs", {
        run_type: "WHAT_IF",
        baseline_run_id: "run-base-8271",
        intervention: { type: "PAYMENT_LATENCY", from: 350, to: 50, unit: "ms" },
      }),
      routeContext("capsuleId", "cap-8271"),
    );
    const created = replayRunSchema.parse(await whatIfResponse.json());
    await getRun(new Request("http://localhost"), routeContext("runId", created.run_id));
    await getRun(new Request("http://localhost"), routeContext("runId", created.run_id));
    const completed = replayRunSchema.parse(
      await (await getRun(new Request("http://localhost"), routeContext("runId", created.run_id))).json(),
    );
    const diffResponse = await createDiff(
      jsonRequest("http://localhost/api/dev/v1/diffs", {
        baseline_run_id: "run-base-8271",
        comparison_run_id: completed.run_id,
      }),
    );
    const diff = replayDiffSchema.parse(await diffResponse.json());

    expect(whatIfResponse.status).toBe(202);
    expect(completed.outcome).toBe("MITIGATED");
    expect(diffResponse.status).toBe(201);
    expect(diff.effect_delta).toEqual({ payment_attempt_count: -1, ledger_commit_count: -1 });
    expect(diff.first_meaningful_divergence?.rule).toBe("PAYMENT_COMPLETES_BEFORE_TIMEOUT");
  });

  it("returns a frozen error for a missing run", async () => {
    const { GET } = await import("../../src/app/api/dev/v1/runs/[runId]/route");
    const response = await GET(
      new Request("http://localhost/api/dev/v1/runs/run-missing"),
      routeContext("runId", "run-missing"),
    );
    const body = apiErrorResponseSchema.parse(await response.json());

    expect(response.status).toBe(404);
    expect(body.error.code).toBe("FIXTURE_MISSING");
    expect(body.error.message).toContain("run-missing");
  });

  it("resets the mock scenario with the exact request and clears incidents", async () => {
    const { POST } = await import("../../src/app/api/dev/v1/demo/reset/route");
    const { GET: listIncidents } = await import("../../src/app/api/dev/v1/incidents/route");
    const response = await POST(
      jsonRequest("http://localhost/api/dev/v1/demo/reset", {
        scenario_id: "checkout_duplicate_effect",
      }),
    );
    const result = resetResultSchema.parse(await response.json());
    const incidentList = await (await listIncidents()).json();

    expect(response.status).toBe(200);
    expect(result.status).toBe("COMPLETED");
    expect(result.configured_latency_ms).toBe(350);
    expect(result.deduplication_enabled).toBe(false);
    expect(incidentList.items).toEqual([]);
  });
});
