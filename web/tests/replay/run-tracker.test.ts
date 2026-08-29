import { describe, expect, it } from "vitest";

import type { ReplayRun } from "../../src/lib/contracts";
import { baselineRun } from "../fixtures/golden-contracts";

const completedBaseline = baselineRun as unknown as ReplayRun;

function activeRun(status: ReplayRun["status"]): ReplayRun {
  return {
    ...completedBaseline,
    status,
    outcome: undefined,
    effect_summary: undefined,
    failure_oracle_result: undefined,
    isolation_evidence: undefined,
    completed_at: undefined,
  } as ReplayRun;
}

describe("run lifecycle poller", () => {
  it("polls until the run reaches a terminal status and reports every observation", async () => {
    const { pollRunUntilTerminal } = await import("../../src/features/replay/run-tracker");

    const observations: ReplayRun[] = [];
    const sequence = [activeRun("CREATED"), activeRun("VALIDATING"), activeRun("RUNNING"), completedBaseline];
    let calls = 0;

    const final = await pollRunUntilTerminal({
      getRun: async () => {
        const run = sequence[Math.min(calls, sequence.length - 1)];
        calls += 1;
        return run;
      },
      onProgress: (run) => observations.push(run),
      intervalMs: 0,
      runId: "run-base-8271",
    });

    expect(calls).toBe(4);
    expect(observations).toHaveLength(4);
    expect(final).toMatchObject({ run_id: "run-base-8271", status: "COMPLETED" });
  });

  it("returns immediately when the first observation is already terminal", async () => {
    const { pollRunUntilTerminal } = await import("../../src/features/replay/run-tracker");

    let calls = 0;
    const final = await pollRunUntilTerminal({
      getRun: async () => {
        calls += 1;
        return completedBaseline;
      },
      intervalMs: 0,
      runId: "run-base-8271",
    });

    expect(calls).toBe(1);
    expect(final?.status).toBe("COMPLETED");
  });

  it("stops polling when cancelled without swallowing the last observation", async () => {
    const { pollRunUntilTerminal } = await import("../../src/features/replay/run-tracker");

    let calls = 0;
    const final = await pollRunUntilTerminal({
      getRun: async () => {
        calls += 1;
        return activeRun("RUNNING");
      },
      isCancelled: () => calls >= 1,
      intervalMs: 0,
      runId: "run-base-8271",
    });
    expect(final).toMatchObject({ status: "RUNNING" });
    expect(calls).toBe(1);
  });

  it("continues past retryable monitoring errors and aborts on terminal non-retryable failures", async () => {
    const { pollRunUntilTerminal } = await import("../../src/features/replay/run-tracker");

    let calls = 0;
    let failures = 0;
    const final = await pollRunUntilTerminal({
      getRun: async () => {
        calls += 1;
        if (calls === 1) {
          failures += 1;
          throw Object.assign(new Error("transport blip"), { retryable: true });
        }
        return completedBaseline;
      },
      onError: (error) => {
        if ((error as { retryable?: boolean }).retryable) {
          failures += 1;
          return true;
        }
        return false;
      },
      intervalMs: 0,
      runId: "run-base-8271",
    });

    expect(calls).toBe(2);
    expect(failures).toBe(2);
    expect(final).toMatchObject({ status: "COMPLETED" });

    let fatalCalls = 0;
    await expect(
      pollRunUntilTerminal({
        getRun: async () => {
          fatalCalls += 1;
          throw Object.assign(new Error("schema drift"), { retryable: false });
        },
        onError: () => false,
        intervalMs: 0,
        runId: "run-base-8271",
      }),
    ).rejects.toThrow("schema drift");
    expect(fatalCalls).toBe(1);
  });
});
