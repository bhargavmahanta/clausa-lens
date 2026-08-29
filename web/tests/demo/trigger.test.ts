import { describe, expect, it } from "vitest";

import type { Incident } from "../../src/lib/contracts";
import { incident } from "../fixtures/golden-contracts";

describe("judge demo trigger flow", () => {
  it("sends the fixed golden checkout payload", async () => {
    const { goldenCheckoutBody } = await import("../../src/features/demo/trigger");
    expect(goldenCheckoutBody).toEqual({
      checkout_id: "checkout-8271",
      amount_minor: 4999,
      currency: "INR",
    });
  });

  it("resolves the detected incident by trace and execution identity", async () => {
    const { findIncidentByTrace } = await import("../../src/features/demo/trigger");
    const match = incident as unknown as Incident;

    expect(
      findIncidentByTrace([match], {
        traceId: "trace-8271",
        executionId: "exec-original-8271",
      }),
    ).toBe(match);

    expect(
      findIncidentByTrace([match], {
        traceId: "trace-other",
        executionId: "exec-original-8271",
      }),
    ).toBeUndefined();
    expect(findIncidentByTrace([], { traceId: "trace-8271", executionId: "exec-original-8271" })).toBeUndefined();
  });

  it("polls the incident list until the faulted checkout incident appears", async () => {
    const { pollForIncident } = await import("../../src/features/demo/trigger");
    const match = incident as unknown as Incident;
    let calls = 0;

    const detected = await pollForIncident({
      listIncidents: async () => {
        calls += 1;
        return calls < 3 ? { items: [] } : { items: [match] };
      },
      trace: { traceId: "trace-8271", executionId: "exec-original-8271" },
      intervalMs: 0,
    });

    expect(calls).toBe(3);
    expect(detected).toBe(match);
  });

  it("gives up after the attempt budget and honours cancellation", async () => {
    const { pollForIncident } = await import("../../src/features/demo/trigger");
    let calls = 0;

    const exhausted = await pollForIncident({
      listIncidents: async () => {
        calls += 1;
        return { items: [] };
      },
      trace: { traceId: "trace-8271", executionId: "exec-original-8271" },
      intervalMs: 0,
      maxAttempts: 3,
    });
    expect(exhausted).toBeUndefined();
    expect(calls).toBe(3);

    let polls = 0;
    const cancelled = await pollForIncident({
      listIncidents: async () => {
        polls += 1;
        return { items: [] };
      },
      trace: { traceId: "trace-8271", executionId: "exec-original-8271" },
      intervalMs: 0,
      maxAttempts: 10,
      isCancelled: () => polls >= 1,
    });
    expect(cancelled).toBeUndefined();
    expect(polls).toBeLessThan(10);
  });

  it("tolerates transient listing errors while polling", async () => {
    const { pollForIncident } = await import("../../src/features/demo/trigger");
    const match = incident as unknown as Incident;
    let calls = 0;

    const detected = await pollForIncident({
      listIncidents: async () => {
        calls += 1;
        if (calls === 1) throw new Error("transport blip");
        return { items: [match] };
      },
      trace: { traceId: "trace-8271", executionId: "exec-original-8271" },
      intervalMs: 0,
    });

    expect(detected).toBe(match);
  });

  it("maps the demo trigger endpoint onto a trace identity", async () => {
    const { requestDemoCheckout } = await import("../../src/features/demo/trigger");

    const trace = await requestDemoCheckout(async () =>
      new Response(
        JSON.stringify({ trace_id: "trace-8271", execution_id: "exec-original-8271" }),
        { status: 200 },
      ),
    );
    expect(trace).toEqual({ traceId: "trace-8271", executionId: "exec-original-8271" });

    await expect(
      requestDemoCheckout(async () =>
        new Response(
          JSON.stringify({ error: { code: "INTERNAL_FAILURE", message: "gateway down", retryable: false, details: {} } }),
          { status: 503 },
        ),
      ),
    ).rejects.toThrow("gateway down");
  });
});
