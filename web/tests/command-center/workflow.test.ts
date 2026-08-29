import { describe, expect, it, vi } from "vitest";

import type { CausaLensClient } from "../../src/lib/api";
import type { Incident, ReplayCapsule, ReplayDiff, ReplayRun } from "../../src/lib/contracts";
import { baselineRun, capsule, incident, replayDiff } from "../fixtures/golden-contracts";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function flush(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

const activeRun = {
  ...baselineRun,
  status: "CREATED",
  outcome: undefined,
  effect_summary: undefined,
  failure_oracle_result: undefined,
  isolation_evidence: undefined,
  started_at: undefined,
  completed_at: undefined,
} as unknown as ReplayRun;

async function setup() {
  const { createCommandCenterWorkflow } = await import(
    "../../src/features/command-center/workflow"
  );
  const dispatchLog: string[] = [];
  const demoLog: unknown[] = [];
  const detected: Incident[] = [];

  const capsuleGate = deferred<ReplayCapsule>();
  const runGate = deferred<ReplayRun>();
  const diffGate = deferred<ReplayDiff>();
  const listGate = deferred<{ items: Incident[] }>();
  let resolveCheckout!: (response: Response) => void;
  const checkoutGate = new Promise<Response>((resolve) => {
    resolveCheckout = resolve;
  });

  const client = {
    createCapsule: vi.fn(() => capsuleGate.promise),
    createRun: vi.fn(() => runGate.promise),
    getRun: vi.fn(),
    createDiff: vi.fn(() => diffGate.promise),
    resetDemo: vi.fn(),
    listIncidents: vi.fn(() => listGate.promise),
  } as unknown as Pick<
    CausaLensClient,
    "createCapsule" | "createRun" | "getRun" | "createDiff" | "resetDemo" | "listIncidents"
  >;

  const workflow = createCommandCenterWorkflow({
    client,
    dispatch: (action) => dispatchLog.push(`${action.type}:${"resource" in action ? action.resource : ""}`),
    onDemoState: (state) => demoLog.push(state),
    onIncidentDetected: (value) => detected.push(value),
    toFrontendError: (error) => ({
      message: error instanceof Error ? error.message : "failed",
      retryable: false,
    }),
    fetchImpl: (() => checkoutGate) as typeof fetch,
    pollIntervalMs: 0,
    pollMaxAttempts: 5,
  });

  return {
    workflow,
    client,
    dispatchLog,
    demoLog,
    detected,
    gates: {
      capsule: capsuleGate,
      run: runGate,
      diff: diffGate,
      list: listGate,
      resolveCheckout,
    },
  };
}

describe("generation-guarded command center workflow", () => {
  it("dispatches a ready capsule when no invalidation happens", async () => {
    const { workflow, dispatchLog, gates } = await setup();
    const running = workflow.compileCapsule("inc-8271");
    gates.capsule.resolve(capsule as unknown as ReplayCapsule);
    await running;

    expect(dispatchLog).toContain("resourceReady:capsule");
  });

  it("reset during checkout creation prevents polling and auto-selection", async () => {
    const { workflow, client, detected, demoLog, gates } = await setup();
    const running = workflow.startFaultedCheckout();

    workflow.invalidate();
    gates.resolveCheckout(
      new Response(
        JSON.stringify({ trace_id: "trace-8271", execution_id: "exec-original-8271" }),
        { status: 200 },
      ),
    );
    await running;
    await flush();

    expect(client.listIncidents).not.toHaveBeenCalled();
    expect(detected).toEqual([]);
    expect(demoLog.at(-1)).toEqual({ status: "idle" });
  });

  it("reset during capsule creation ignores the late capsule", async () => {
    const { workflow, dispatchLog, gates } = await setup();
    const running = workflow.compileCapsule("inc-8271");

    workflow.invalidate();
    gates.capsule.resolve(capsule as unknown as ReplayCapsule);
    await running;
    await flush();

    expect(dispatchLog).toContain("resourceLoading:capsule");
    expect(dispatchLog).not.toContain("resourceReady:capsule");
  });

  it("reset during run creation ignores the late run and never polls", async () => {
    const { workflow, client, dispatchLog, gates } = await setup();
    const running = workflow.startBaseline("cap-8271");

    workflow.invalidate();
    gates.run.resolve(activeRun);
    await running;
    await flush();

    expect(dispatchLog).toContain("resourceLoading:baseline");
    expect(dispatchLog).not.toContain("resourceReady:baseline");
    expect(client.getRun).not.toHaveBeenCalled();
  });

  it("reset during diff creation ignores the late diff", async () => {
    const { workflow, dispatchLog, gates } = await setup();
    const running = workflow.createDiff({
      baseline_run_id: "run-base-8271",
      comparison_run_id: "run-whatif-8271",
    });

    workflow.invalidate();
    gates.diff.resolve(replayDiff as unknown as ReplayDiff);
    await running;
    await flush();

    expect(dispatchLog).toContain("resourceLoading:diff");
    expect(dispatchLog).not.toContain("resourceReady:diff");
  });

  it("incident change during an in-flight request ignores the old response", async () => {
    const { workflow, dispatchLog, gates } = await setup();
    const running = workflow.compileCapsule("inc-8271");

    workflow.notifyIncidentChanged();
    gates.capsule.resolve(capsule as unknown as ReplayCapsule);
    await running;
    await flush();

    expect(dispatchLog).not.toContain("resourceReady:capsule");
  });

  it("cancelled demo trigger returns to an actionable idle state", async () => {
    const { workflow, detected, demoLog, gates } = await setup();
    const running = workflow.startFaultedCheckout();

    gates.resolveCheckout(
      new Response(
        JSON.stringify({ trace_id: "trace-8271", execution_id: "exec-original-8271" }),
        { status: 200 },
      ),
    );
    await flush();
    expect(demoLog).toContainEqual({ status: "waiting" });

    workflow.invalidate();
    gates.list.resolve({ items: [] });
    await running;
    await flush();

    expect(detected).toEqual([]);
    expect(demoLog.at(-1)).toEqual({ status: "idle" });
  });

  it("auto-selects the detected incident when the trigger completes", async () => {
    const { workflow, detected, demoLog, gates } = await setup();
    const running = workflow.startFaultedCheckout();

    gates.resolveCheckout(
      new Response(
        JSON.stringify({ trace_id: "trace-8271", execution_id: "exec-original-8271" }),
        { status: 200 },
      ),
    );
    await flush();
    gates.list.resolve({ items: [incident as unknown as Incident] });
    await running;
    await flush();

    expect(detected).toEqual([incident as unknown as Incident]);
    expect(demoLog.at(-1)).toEqual({ status: "idle" });
  });
});
