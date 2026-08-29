import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

describe("overview carousel hero", () => {
  it("renders all five workflow stages with exactly one active front card", async () => {
    const { OverviewCarousel } = await import("../../src/features/overview/overview-carousel");
    const markup = renderToStaticMarkup(
      <OverviewCarousel
        stages={[
          {
            stage: "capture",
            title: "Capture",
            summary: "inc-8271 · READY",
            description: "What failure was detected",
            statusChip: { label: "READY", tone: "pass" },
          },
          {
            stage: "trace",
            title: "Trace",
            summary: "Gateway → Checkout → Payment → Ledger",
            description: "How the request moved through the system",
          },
          {
            stage: "replay",
            title: "Replay",
            summary: "Baseline not started",
            description: "Capsule, baseline, and what-if status",
            statusChip: { label: "IDLE", tone: "neutral" },
          },
          {
            stage: "diff",
            title: "Diff",
            summary: "−1 attempt · −1 ledger commit",
            description: "What changed between baseline and what-if",
          },
          {
            stage: "overview",
            title: "Overview",
            summary: "2 of 4 stages complete",
            description: "Workflow progress and isolation status",
          },
        ]}
      />,
    );

    expect(markup).toContain('aria-label="Workflow overview carousel"');
    expect(markup.match(/data-stage=/g)).toHaveLength(5);
    expect(markup.match(/aria-current="true"/g)).toHaveLength(1);
    expect(markup).toContain("inc-8271 · READY");
    expect(markup).toContain("Gateway → Checkout → Payment → Ledger");
    expect(markup).toContain("−1 attempt · −1 ledger commit");
    expect(markup).toContain("2 of 4 stages complete");
    expect(markup).toContain("Open Capture");
    expect(markup).toContain("Open Replay");
    expect(markup).toContain("Open Diff");
    expect(markup).not.toContain("Open Trace");
  });

  it("announces the active stage through a polite live region", async () => {
    const { OverviewCarousel } = await import("../../src/features/overview/overview-carousel");
    const markup = renderToStaticMarkup(
      <OverviewCarousel
        initialStage="replay"
        stages={[
          { stage: "capture", title: "Capture", summary: "s1", description: "d1" },
          { stage: "trace", title: "Trace", summary: "s2", description: "d2" },
          { stage: "replay", title: "Replay", summary: "s3", description: "d3" },
          { stage: "diff", title: "Diff", summary: "s4", description: "d4" },
          { stage: "overview", title: "Overview", summary: "s5", description: "d5" },
        ]}
      />,
    );

    expect(markup).toContain('aria-live="polite"');
    expect(markup).toContain("Replay in focus");
    expect(markup.match(/data-placement="front"/g)).toHaveLength(1);
  });

  it("provides rotation controls for keyboard users", async () => {
    const { OverviewCarousel } = await import("../../src/features/overview/overview-carousel");
    const markup = renderToStaticMarkup(
      <OverviewCarousel
        stages={[
          { stage: "capture", title: "Capture", summary: "s1", description: "d1" },
          { stage: "trace", title: "Trace", summary: "s2", description: "d2" },
          { stage: "replay", title: "Replay", summary: "s3", description: "d3" },
          { stage: "diff", title: "Diff", summary: "s4", description: "d4" },
          { stage: "overview", title: "Overview", summary: "s5", description: "d5" },
        ]}
      />,
    );

    expect(markup).toContain('aria-label="Rotate carousel backward"');
    expect(markup).toContain('aria-label="Rotate carousel forward"');
    expect(markup).toContain('aria-label="Pause auto-rotation"');
  });
});
