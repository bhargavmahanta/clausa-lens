import { describe, expect, it } from "vitest";

const validCapturedEvent = {
  schema_version: "1.0",
  event_id: "evt-payment-1-start",
  execution_id: "exec-original-8271",
  trace_id: "trace-8271",
  parent_event_id: "evt-checkout-start",
  component: { name: "payment", instance: "payment-1" },
  operation: { name: "authorize", kind: "DEPENDENCY" },
  event_type: "START",
  attempt: 1,
  logical_operation_id: "checkout-8271",
  occurred_at: "2026-08-29T10:32:01.015Z",
  sequence: 1,
  status: "RUNNING",
  attributes: { configured_latency_ms: 350 },
};

describe("ExecutionEvent v1.0", () => {
  it("accepts a complete captured execution event", async () => {
    const contracts = (await import("../../src/lib/contracts")) as {
      executionEventSchema?: {
        safeParse: (value: unknown) => { success: boolean };
      };
    };

    expect(contracts.executionEventSchema).toBeDefined();
    if (!contracts.executionEventSchema) return;

    expect(contracts.executionEventSchema.safeParse(validCapturedEvent).success).toBe(true);
  });

  it("rejects unknown closed-enum values", async () => {
    const { executionEventSchema } = await import("../../src/lib/contracts");
    const unsupportedEvent = { ...validCapturedEvent, status: "QUEUED" };

    expect(executionEventSchema.safeParse(unsupportedEvent).success).toBe(false);
  });

  it("rejects unknown top-level fields", async () => {
    const { executionEventSchema } = await import("../../src/lib/contracts");
    const eventWithUnknownField = { ...validCapturedEvent, raw_log: "not canonical evidence" };

    expect(executionEventSchema.safeParse(eventWithUnknownField).success).toBe(false);
  });
});
