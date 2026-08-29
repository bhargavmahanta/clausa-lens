import { afterEach, describe, expect, it, vi } from "vitest";

describe("judge demo trigger route", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("calls the configured gateway with the fixed golden checkout payload", async () => {
    vi.stubEnv("CAUSALENS_GATEWAY_URL", "http://demo-gateway:8080/");
    const gatewayFetch = vi.fn(async () =>
      new Response(
        JSON.stringify({
          trace_id: "trace-8271",
          execution_id: "exec-original-8271",
          logical_operation_id: "checkout-8271",
          attempts: 2,
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", gatewayFetch);

    const { POST } = await import("../../src/app/api/demo/checkout/route");
    const response = await POST();
    const body = await response.json();

    expect(gatewayFetch).toHaveBeenCalledTimes(1);
    const [calledUrl, calledInit] = gatewayFetch.mock.calls[0] as unknown as [string, RequestInit];
    expect(calledUrl).toBe("http://demo-gateway:8080/checkout");
    expect(calledInit.method).toBe("POST");
    expect(JSON.parse(String(calledInit.body))).toEqual({
      checkout_id: "checkout-8271",
      amount_minor: 4999,
      currency: "INR",
    });
    expect(response.status).toBe(200);
    expect(body.trace_id).toBe("trace-8271");
    expect(body.execution_id).toBe("exec-original-8271");
  });

  it("fails closed when no gateway URL is configured", async () => {
    vi.stubEnv("CAUSALENS_GATEWAY_URL", "");
    const gatewayFetch = vi.fn();
    vi.stubGlobal("fetch", gatewayFetch);

    const { POST } = await import("../../src/app/api/demo/checkout/route");
    const response = await POST();
    const body = await response.json();

    expect(response.status).toBe(503);
    expect(body.error.code).toBe("INTERNAL_FAILURE");
    expect(body.error.message).toMatch(/CAUSALENS_GATEWAY_URL/);
    expect(gatewayFetch).not.toHaveBeenCalled();
  });

  it("returns a frozen 502 when the gateway rejects or responds with an unusable shape", async () => {
    vi.stubEnv("CAUSALENS_GATEWAY_URL", "http://demo-gateway:8080");
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("server unavailable", { status: 500 })),
    );

    const { POST } = await import("../../src/app/api/demo/checkout/route");
    const rejected = await POST();
    expect(rejected.status).toBe(502);
    expect((await rejected.json()).error.code).toBe("INTERNAL_FAILURE");

    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(JSON.stringify({ unexpected: true }), { status: 200 })),
    );
    const malformed = await POST();
    expect(malformed.status).toBe(502);
    expect((await malformed.json()).error.code).toBe("INTERNAL_FAILURE");
  });
});
