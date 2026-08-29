import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import type { ReplayDiff, ResetResult } from "../../src/lib/contracts";
import { replayDiff, resetResult } from "../fixtures/golden-contracts";

vi.stubEnv("NODE_ENV", "development");

describe("command center workflow components", () => {
  it("renders one unified incident-to-diff workflow without fixture values before responses arrive", async () => {
    const { CommandCenter } = await import("../../src/features/command-center");
    const markup = renderToStaticMarkup(<CommandCenter />);

    expect(markup).toContain("Incident analysis");
    expect(markup).toContain("Controlled replay");
    expect(markup).toContain("Reset demo workflow");
    expect(markup).toContain("Development fixture preview");
    expect(markup).toContain("Incident / Trace");
    expect(markup).toContain("Replay Capsule");
    expect(markup).toContain("Baseline and what-if");
    expect(markup).toContain("First meaningful divergence");
    expect(markup).toContain("No incident selected");
    expect(markup).toContain("Awaiting capsule compilation");
    expect(markup.match(/data-stage=/g)).toHaveLength(4);
    expect(markup).not.toContain('data-stage="overview"');
    expect(markup).not.toContain('data-stage="capture"');
    expect(markup).not.toContain("cap-8271");
    expect(markup).not.toContain("REPRODUCED");
    expect(markup).not.toContain("MITIGATED");
  });

  it("renders effect counts, oracle comparison, and API first divergence", async () => {
    const { ReplayDiffPanel } = await import("../../src/features/replay/replay-diff");
    const markup = renderToStaticMarkup(
      <ReplayDiffPanel diff={replayDiff as unknown as ReplayDiff} />,
    );

    expect(markup).toContain("Replay Diff");
    expect(markup).toContain("2 attempts");
    expect(markup).toContain("1 attempt");
    expect(markup).toContain("−1");
    expect(markup).toContain("MATCHED");
    expect(markup).toContain("NOT MATCHED");
    expect(markup).toContain("PAYMENT_COMPLETES_BEFORE_TIMEOUT");
    expect(markup).toContain("evt-replay-timeout");
    expect(markup).toContain("evt-whatif-payment-complete");
    expect(markup).toContain("Applies to checkout_duplicate_effect pack v1.0.0 fixtures.");
  });

  it("renders no fabricated first divergence when the API omits it", async () => {
    const { ReplayDiffPanel } = await import("../../src/features/replay/replay-diff");
    const markup = renderToStaticMarkup(
      <ReplayDiffPanel
        diff={{ ...replayDiff, first_meaningful_divergence: undefined } as unknown as ReplayDiff}
      />,
    );

    expect(markup).toContain("No first meaningful divergence was provided by the API.");
    expect(markup).not.toContain("PAYMENT_COMPLETES_BEFORE_TIMEOUT");
  });

  it("uses a labelled confirmation dialog before reset and presents the reset receipt", async () => {
    const { ResetDialog, ResetReceipt } = await import("../../src/features/replay/reset-dialog");
    const dialog = renderToStaticMarkup(
      <ResetDialog onCancel={() => undefined} onConfirm={() => undefined} status="confirming" />,
    );
    const receipt = renderToStaticMarkup(
      <ResetReceipt result={resetResult as unknown as ResetResult} />,
    );

    expect(dialog).toContain('role="dialog"');
    expect(dialog).toContain('aria-modal="true"');
    expect(dialog).toContain("Confirm demo reset");
    expect(dialog).toContain("Cancel");
    expect(dialog).toContain("Reset demo");
    expect(receipt).toContain("reset-1");
    expect(receipt).toContain("350 ms");
    expect(receipt).toContain("Deduplication disabled");
  });
});
