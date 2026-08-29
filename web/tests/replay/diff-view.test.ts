import { describe, expect, it } from "vitest";

import type { ReplayDiff } from "../../src/lib/contracts";
import { replayDiff } from "../fixtures/golden-contracts";

describe("replay diff view model", () => {
  it("projects API effect counts, oracle comparison, and first divergence verbatim", async () => {
    const { buildDiffView } = await import("../../src/features/replay/diff-view");
    const view = buildDiffView(replayDiff as unknown as ReplayDiff);

    expect(view.effects).toEqual({
      baseline: { paymentAttemptCount: 2, ledgerCommitCount: 2 },
      comparison: { paymentAttemptCount: 1, ledgerCommitCount: 1 },
      delta: { paymentAttemptCount: -1, ledgerCommitCount: -1 },
    });
    expect(view.oracles).toEqual({
      baselineMatched: true,
      comparisonMatched: false,
      baselineExplanation: "Baseline reproduced the timeout-driven duplicate ledger effect.",
      comparisonExplanation: "Payment completed before timeout; no duplicate effect occurred.",
    });
    expect(view.firstDivergence).toEqual({
      rule: "PAYMENT_COMPLETES_BEFORE_TIMEOUT",
      baselineEventId: "evt-replay-timeout",
      comparisonEventId: "evt-whatif-payment-complete",
      baselineValue: "TIMEOUT",
      comparisonValue: "SUCCESS",
      baselineTimelineIndex: 3,
      comparisonTimelineIndex: 3,
    });
    expect(view.removedEventCount).toBe(3);
    expect(view.addedEventCount).toBe(0);
  });

  it("does not invent a first divergence when the API omits it", async () => {
    const { buildDiffView } = await import("../../src/features/replay/diff-view");
    const view = buildDiffView({
      ...replayDiff,
      first_meaningful_divergence: undefined,
    } as unknown as ReplayDiff);

    expect(view.firstDivergence).toBeUndefined();
  });
});
