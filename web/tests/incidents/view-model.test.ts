import { describe, expect, it } from "vitest";

import type { IncidentDetailResponse } from "../../src/lib/contracts";
import { goldenIncidentDetail } from "../fixtures/golden-contracts";

describe("incident evidence view model", () => {
  it("orders events by graph timeline_index even when the response array is shuffled", async () => {
    const { buildIncidentView } = await import("../../src/features/incidents/view-model");
    const shuffled = {
      ...goldenIncidentDetail,
      events: [...goldenIncidentDetail.events].reverse(),
    } as unknown as IncidentDetailResponse;

    const view = buildIncidentView(shuffled);

    expect(view.timeline.map((entry) => `${entry.timelineIndex}:${entry.event.event_id}`)).toEqual([
      "0:evt-gateway-start",
      "1:evt-checkout-start",
      "2:evt-payment-1-start",
      "3:evt-timeout",
      "4:evt-retry",
      "5:evt-payment-2-start",
      "6:evt-ledger-1",
      "7:evt-ledger-2",
    ]);
    expect(view.componentPath).toEqual(["gateway", "checkout", "payment", "ledger"]);
    expect(view.requestId).toBe("checkout-8271");
  });

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

  it("joins graph nodes to API events without creating nodes or edges", async () => {
    const { buildIncidentView } = await import("../../src/features/incidents/view-model");
    const view = buildIncidentView(
      goldenIncidentDetail as unknown as IncidentDetailResponse,
    );

    expect(view.graphNodes).toHaveLength(8);
    expect(view.graphNodes[0]).toMatchObject({
      timelineIndex: 0,
      event: { event_id: "evt-gateway-start", component: { name: "gateway" } },
    });
    expect(view.structuralEdges).toHaveLength(7);
    expect(view.graphNodes.some((node) => node.event.event_id === "fixture-only-event")).toBe(false);
  });
});
