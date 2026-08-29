import { describe, expect, it } from "vitest";

import {
  baselineRun,
  capsule,
  capturedEvent,
  goldenIncidentDetail,
  incident,
  replayDiff,
  resetResult,
} from "../fixtures/golden-contracts";

type FetchCall = {
  input: string | URL | Request;
  init?: RequestInit;
};

describe("CausaLens API client", () => {
  it("requests and decodes a replay run through the configured boundary", async () => {
    const apiModule = (await import("../../src/lib/api")) as Record<string, unknown>;
    expect(apiModule.createCausaLensClient).toBeDefined();
    if (typeof apiModule.createCausaLensClient !== "function") return;

    const calls: FetchCall[] = [];
    const fetchImpl = async (input: string | URL | Request, init?: RequestInit) => {
      calls.push({ input, init });
      return Response.json(baselineRun);
    };
    const client = apiModule.createCausaLensClient({
      baseUrl: "http://core-api.test",
      fetchImpl,
    }) as { getRun: (runId: string) => Promise<{ run_id: string; outcome?: string | null }> };

    const run = await client.getRun("run-base-8271");

    expect(run.run_id).toBe("run-base-8271");
    expect(run.outcome).toBe("REPRODUCED");
    expect(String(calls[0]?.input)).toBe("http://core-api.test/v1/runs/run-base-8271");
    expect(calls[0]?.init?.headers).toEqual({ Accept: "application/json" });
  });

  it("preserves the stable API error code and HTTP status", async () => {
    const { CausaLensApiError, createCausaLensClient } = await import("../../src/lib/api");
    const fetchImpl = async () =>
      Response.json(
        {
          error: {
            code: "INTEGRITY_MISMATCH",
            message: "Replay Capsule digest does not match its canonical content.",
            retryable: false,
            details: { capsule_id: "cap-8271" },
          },
        },
        { status: 422 },
      );
    const client = createCausaLensClient({ baseUrl: "http://core-api.test", fetchImpl });

    await expect(client.getRun("run-base-8271")).rejects.toMatchObject({
      name: CausaLensApiError?.name ?? "CausaLensApiError",
      status: 422,
      code: "INTEGRITY_MISMATCH",
      retryable: false,
      details: { capsule_id: "cap-8271" },
    });
  });

  it("covers the frozen frontend API surface through one client", async () => {
    const apiModule = await import("../../src/lib/api");
    const calls: FetchCall[] = [];
    const fetchImpl = async (input: string | URL | Request, init?: RequestInit) => {
      calls.push({ input, init });
      const url = new URL(String(input));

      if (url.pathname === "/v1/events") return Response.json({ event_id: capturedEvent.event_id, status: "ACCEPTED" }, { status: 202 });
      if (url.pathname === "/v1/incidents") return Response.json({ items: [incident] });
      if (url.pathname === "/v1/incidents/inc-8271") {
        return Response.json(goldenIncidentDetail);
      }
      if (url.pathname === "/v1/incidents/inc-8271/capsules") return Response.json(capsule, { status: 201 });
      if (url.pathname === "/v1/capsules/cap-8271/runs") return Response.json(baselineRun, { status: 202 });
      if (url.pathname === "/v1/diffs") return Response.json(replayDiff, { status: 201 });
      if (url.pathname === "/v1/diffs/diff-8271") return Response.json(replayDiff);
      if (url.pathname === "/v1/demo/reset") return Response.json(resetResult);

      return Response.json({ error: { code: "INTERNAL_FAILURE", message: "Unexpected path", retryable: false, details: {} } }, { status: 500 });
    };
    const client = apiModule.createCausaLensClient({ baseUrl: "http://core-api.test/", fetchImpl }) as Record<
      string,
      (...arguments_: never[]) => Promise<unknown>
    >;

    const expectedMethods = [
      "acceptEvent",
      "listIncidents",
      "getIncident",
      "createCapsule",
      "createRun",
      "getRun",
      "createDiff",
      "getDiff",
      "resetDemo",
    ];
    expect(Object.keys(client).sort()).toEqual(expectedMethods.sort());
    if (expectedMethods.some((method) => typeof client[method] !== "function")) return;

    await client.acceptEvent(capturedEvent as never);
    await client.listIncidents({ status: "READY", limit: 10 } as never);
    await client.getIncident("inc-8271" as never);
    await client.createCapsule("inc-8271" as never);
    await client.createRun("cap-8271" as never, { run_type: "BASELINE" } as never);
    await client.createDiff({ baseline_run_id: "run-base-8271", comparison_run_id: "run-whatif-8271" } as never);
    await client.getDiff("diff-8271" as never);
    await client.resetDemo({ scenario_id: "checkout_duplicate_effect" } as never);

    expect(String(calls[1]?.input)).toBe("http://core-api.test/v1/incidents?status=READY&limit=10");
    expect(calls[0]?.init?.method).toBe("POST");
    expect(calls[4]?.init?.body).toBe(JSON.stringify({ run_type: "BASELINE" }));
    expect(calls[7]?.init?.body).toBe(JSON.stringify({ scenario_id: "checkout_duplicate_effect" }));
  });

  it("rejects an invalid what-if request before making a network call", async () => {
    const { createCausaLensClient } = await import("../../src/lib/api");
    let networkCalls = 0;
    const fetchImpl = async () => {
      networkCalls += 1;
      return Response.json(baselineRun);
    };
    const client = createCausaLensClient({ baseUrl: "http://core-api.test", fetchImpl });

    await expect(
      client.createRun("cap-8271", { run_type: "WHAT_IF" } as never),
    ).rejects.toMatchObject({
      name: "ContractValidationError",
      resource: "CreateRunRequest",
    });
    expect(networkCalls).toBe(0);
  });

  it("rejects a success response with the wrong endpoint status", async () => {
    const { createCausaLensClient } = await import("../../src/lib/api");
    const client = createCausaLensClient({
      baseUrl: "http://core-api.test",
      fetchImpl: async () => Response.json({ event_id: capturedEvent.event_id, status: "ACCEPTED" }),
    });

    await expect(client.acceptEvent(capturedEvent)).rejects.toMatchObject({
      name: "ProtocolError",
      expectedStatus: 202,
      receivedStatus: 200,
    });
  });
});
