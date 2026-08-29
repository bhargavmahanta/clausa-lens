import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { ReplayCapsule, ReplayRun } from "../../src/lib/contracts";
import type { ReplayWorkspaceProps } from "../../src/features/replay/replay-workspace";
import { baselineRun, capsule } from "../fixtures/golden-contracts";

const readyCapsule = capsule as unknown as ReplayCapsule;
const completedBaseline = baselineRun as unknown as ReplayRun;

function activeRun(status: "CREATED" | "VALIDATING" | "RUNNING"): ReplayRun {
  return {
    ...completedBaseline,
    status,
    outcome: undefined,
    effect_summary: undefined,
    failure_oracle_result: undefined,
    isolation_evidence: undefined,
    started_at: undefined,
    completed_at: undefined,
  } as ReplayRun;
}

async function renderWorkspace(props: ReplayWorkspaceProps) {
  const { ReplayWorkspace } = await import("../../src/features/replay/replay-workspace");
  return renderToStaticMarkup(<ReplayWorkspace {...props} />);
}

describe("C3 replay workspace", () => {
  it("invites capsule compilation while no capsule exists", async () => {
    const markup = await renderWorkspace({ state: { phase: "idle" }, hasSelectedIncident: false });

    expect(markup).toContain("Compile replay capsule");
    expect(markup.match(/disabled=""|disabled/g)?.length).toBeGreaterThan(0);
    expect(markup).not.toContain("cap-8271");
  });

  it("renders the immutable capsule evidence when compilation succeeds", async () => {
    const markup = await renderWorkspace({
      state: { phase: "capsuleReady", capsule: readyCapsule },
    });

    expect(markup).toContain("cap-8271");
    expect(markup).toContain("checkout_duplicate_effect");
    expect(markup).toContain("gateway.checkout");
    expect(markup).toContain("GOLDEN_RESET_V1");
    expect(markup).toContain("state-ledger-empty");
    expect(markup).toContain("dependency-payment-350ms");
    expect(markup).toContain("production-databases");
    expect(markup).toContain("a".repeat(64));
    expect(markup).toContain("payment_attempt_count");
    expect(markup).toContain("Start baseline replay");
  });

  it("shows only real lifecycle statuses for an active run and never an outcome", async () => {
    const markup = await renderWorkspace({
      state: { phase: "active", capsule: readyCapsule, run: activeRun("RUNNING") },
    });

    expect(markup).toContain("RUNNING");
    expect(markup).toContain("run-base-8271");
    expect(markup).not.toContain("REPRODUCED");
    expect(markup).not.toContain("MATCHED");
    expect(markup).not.toContain("Isolation");
    expect(markup).toContain("Start baseline replay");
    expect(markup).toMatch(/disabled/g);
  });

  it("presents reproduction only with separation rows and isolation evidence", async () => {
    const markup = await renderWorkspace({
      state: { phase: "terminal", capsule: readyCapsule, run: completedBaseline },
      gate: { unlocked: true },
    });

    expect(markup).toContain("Replay execution");
    expect(markup).toContain("Isolation");
    expect(markup).toContain("Failure oracle");
    expect(markup).toContain("Baseline outcome");
    expect(markup).toContain("Failure reproduced");
    expect(markup).toContain("payment_simulator");
    expect(markup).toContain("SIMULATED");
    expect(markup).toContain("replay-run-base-8271");
    expect(markup).toContain("reproduced the timeout-driven duplicate ledger effect");
    expect(markup.indexOf("Isolation")).toBeLessThan(markup.indexOf("Failure reproduced"));
  });

  it("keeps BLOCKED runs outcome-free with the frozen error code visible", async () => {
    const blockedRun = {
      ...completedBaseline,
      status: "BLOCKED",
      outcome: undefined,
      effect_summary: undefined,
      failure_oracle_result: undefined,
      isolation_evidence: undefined,
      error: {
        code: "ISOLATION_VIOLATION",
        message: "The replay attempted a blocked destination.",
        retryable: false,
        details: {},
      },
    } as ReplayRun;
    const markup = await renderWorkspace({
      state: { phase: "terminal", capsule: readyCapsule, run: blockedRun },
    });

    expect(markup).toContain("ISOLATION_VIOLATION");
    expect(markup).toContain("blocked destination");
    expect(markup).not.toContain("Baseline outcome");
    expect(markup).not.toContain("REPRODUCED");
    expect(markup).not.toContain("Failure reproduced");
  });

  it("keeps the what-if intervention card locked with an honest reason", async () => {
    const locked = await renderWorkspace({
      state: { phase: "capsuleReady", capsule: readyCapsule },
      gate: { unlocked: false, reason: "Baseline run is RUNNING; what-if unlocks only after a completed baseline." },
    });

    expect(locked).toContain("PAYMENT LATENCY");
    expect(locked).toContain("350 ms");
    expect(locked).toContain("50 ms");
    expect(locked).toContain("All other replay conditions remain unchanged.");
    expect(locked).toContain("Run controlled what-if");
    expect(locked).toContain("Baseline run is RUNNING");
    expect(locked).toMatch(/disabled/g);

    const unlocked = await renderWorkspace({
      state: { phase: "terminal", capsule: readyCapsule, run: completedBaseline },
      gate: { unlocked: true },
    });
    expect(unlocked).toContain("Run controlled what-if");
    expect(unlocked).toContain("What-if execution arrives with the next checkpoint");
  });

  it("renders monitoring errors with recovery instead of fabricating state", async () => {
    const markup = await renderWorkspace({
      state: { phase: "error", message: "Monitoring failed.", capsule: readyCapsule },
      onRetry: () => undefined,
    });

    expect(markup).toContain("Monitoring failed.");
    expect(markup).toContain("Retry");
    expect(markup).not.toContain("REPRODUCED");
    expect(markup).toContain("cap-8271");
  });

  it("keeps PACK_UNAVAILABLE and FAILED errors distinct from successful outcomes", async () => {
    const packUnavailable = await renderWorkspace({
      state: {
        phase: "error",
        message: "The checkout System Pack is unavailable.",
        code: "PACK_UNAVAILABLE",
      },
    });
    const failedRun = {
      ...completedBaseline,
      status: "FAILED",
      outcome: undefined,
      effect_summary: undefined,
      failure_oracle_result: undefined,
      isolation_evidence: undefined,
      error: {
        code: "INTERNAL_FAILURE",
        message: "The replay worker failed.",
        retryable: true,
        details: {},
      },
    } as ReplayRun;
    const failed = await renderWorkspace({
      state: { phase: "terminal", capsule: readyCapsule, run: failedRun },
    });

    expect(packUnavailable).toContain("PACK_UNAVAILABLE");
    expect(packUnavailable).not.toContain("REPRODUCED");
    expect(failed).toContain("FAILED");
    expect(failed).toContain("INTERNAL_FAILURE");
    expect(failed).not.toContain("Baseline outcome");
  });
});
