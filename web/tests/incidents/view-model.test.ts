import { describe, expect, it } from "vitest";

import type { IncidentDetailResponse } from "../../src/lib/contracts";
import { goldenIncidentDetail } from "../fixtures/golden-contracts";

describe("incident evidence view model", () => {
  it("preserves API timeline order and keeps attempts and effects distinct", async () => {
    const { buildIncidentView } = await import("../../src/features/incidents/view-model");
    const view = buildIncidentView(
      goldenIncidentDetail as unknown as IncidentDetailResponse,
    );

    expect(view.timeline.map((entry) => entry.event.event_id)).toEqual([
      "evt-gateway-start",
      "evt-checkout-start",
      "evt-payment-1-start",
      "evt-timeout",
      "evt-retry",
      "evt-payment-2-start",
      "evt-ledger-1",
      "evt-ledger-2",
    ]);
    expect(view.timeline.filter((entry) => entry.event.operation.name === "authorize").map((entry) => entry.event.attempt)).toEqual([1, 2]);
    expect(view.timeline.filter((entry) => entry.event.event_type === "EFFECT").map((entry) => entry.event.attributes.effect_id)).toEqual([
      "ledger-effect-1",
      "ledger-effect-2",
    ]);
  });

  it("separates structural edges from incident-owned oracle evidence", async () => {
    const { buildIncidentView } = await import("../../src/features/incidents/view-model");
    const view = buildIncidentView(
      goldenIncidentDetail as unknown as IncidentDetailResponse,
    );

    expect(view.structuralEdges[3]).toMatchObject({
      type: "RETRY",
      fromEventId: "evt-timeout",
      toEventId: "evt-retry",
    });
    expect(view.evidenceEvents.map((event) => event.event_id)).toEqual([
      "evt-timeout",
      "evt-retry",
      "evt-ledger-1",
      "evt-ledger-2",
    ]);
    expect(view.componentPath).toEqual(["gateway", "checkout", "payment", "ledger"]);
    expect(view.requestId).toBe("checkout-8271");
  });
});
