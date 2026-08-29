import { describe, expect, it } from "vitest";

import type { ReplayCapsule } from "../../src/lib/contracts";
import { capsule } from "../fixtures/golden-contracts";

describe("capsule view model", () => {
  it("surfaces the frozen capsule facts without inventing evidence", async () => {
    const { buildCapsuleView } = await import("../../src/features/replay/capsule-view");
    const view = buildCapsuleView(capsule as unknown as ReplayCapsule);

    expect(view.capsuleId).toBe("cap-8271");
    expect(view.pack).toEqual({ id: "checkout_duplicate_effect", version: "1.0.0", interfaceVersion: "1.0" });
    expect(view.triggerSummary).toBe("POST /checkout");
    expect(view.sanitizedHeaders).toEqual({ "content-type": "application/json" });
    expect(view.source).toMatchObject({
      incidentId: "inc-8271",
      traceId: "trace-8271",
      executionId: "exec-original-8271",
      captureEnvironment: "DEMO",
    });
    expect(view.eventIds).toEqual(["evt-timeout", "evt-retry", "evt-ledger-1", "evt-ledger-2"]);
    expect(view.graphId).toBe("graph-8271");
  });

  it("presents fixtures, timing, plan, and oracle expectations exactly as returned", async () => {
    const { buildCapsuleView } = await import("../../src/features/replay/capsule-view");
    const view = buildCapsuleView(capsule as unknown as ReplayCapsule);

    expect(view.stateFixtures).toHaveLength(1);
    expect(view.stateFixtures[0]).toMatchObject({
      fixtureId: "state-ledger-empty",
      resetStrategy: "TRUNCATE_AND_LOAD",
      sanitizationStatus: "PASS",
    });
    expect(view.dependencyFixtures).toHaveLength(1);
    expect(view.dependencyFixtures[0]).toMatchObject({
      dependency: "payment_simulator",
      latencyMS: 350,
      failureMode: "NONE",
      invocationLimit: 2,
    });
    expect(view.timing).toEqual({ clockToleranceMS: 5, timeoutMS: 200 });
    expect(view.plan).toEqual({
      entrypoint: "gateway.checkout",
      requiredComponents: ["gateway", "checkout", "payment", "ledger"],
      fixtureLoadOrder: ["state-ledger-empty", "dependency-payment-350ms"],
      resetStrategy: "GOLDEN_RESET_V1",
    });
    expect(view.oracle).toMatchObject({
      id: "duplicate_ledger_effect",
      expectedMatch: true,
      expectedEffectSummary: { paymentAttemptCount: 2, ledgerCommitCount: 2 },
    });
  });

  it("passes safety policy and integrity digest through verbatim", async () => {
    const { buildCapsuleView } = await import("../../src/features/replay/capsule-view");
    const view = buildCapsuleView(capsule as unknown as ReplayCapsule);

    expect(view.safety).toEqual({
      policyVersion: "1.0",
      sanitizationStatus: "PASS",
      blockedDestinations: ["production-databases", "public-internet", "real-payment-provider"],
      allowedDestinations: ["payment-simulator", "replay-postgres"],
      credentialProfile: "replay-only",
    });
    expect(view.integrity).toEqual({ algorithm: "SHA-256", digest: "a".repeat(64) });
    expect(view.allowedInterventions).toEqual([
      { type: "PAYMENT_LATENCY", valueType: "INTEGER", unit: "ms", minimum: 0, maximum: 5000 },
    ]);
  });
});
