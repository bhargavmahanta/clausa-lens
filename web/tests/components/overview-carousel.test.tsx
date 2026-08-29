import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { CarouselStageContent } from "../../src/features/overview/overview-carousel";

const stages: CarouselStageContent[] = [
  {
    stage: "incident",
    eyebrow: "Captured evidence",
    title: "Incident / Trace",
    description: "Follow the request path",
    summary: "Gateway → Checkout → Payment → Ledger",
    href: "#incident-workspace",
    actionLabel: "Inspect incident",
    statusChip: { label: "SELECTED", tone: "pass" },
    metrics: [
      { label: "Oracle", value: "Pending" },
      { label: "Services", value: "4" },
    ],
  },
  {
    stage: "capsule",
    eyebrow: "Replay artifact",
    title: "Replay Capsule",
    description: "Integrity and isolation",
    summary: "Awaiting capsule compilation",
    href: "#replay-lab",
    actionLabel: "Open capsule",
  },
  {
    stage: "replay",
    eyebrow: "Controlled execution",
    title: "Replay",
    description: "Baseline and what-if",
    summary: "Baseline not started",
    href: "#replay-lab",
    actionLabel: "Open replay lab",
  },
  {
    stage: "diff",
    eyebrow: "Evidence delta",
    title: "Diff",
    description: "First meaningful divergence",
    summary: "Awaiting both runs",
    href: "#replay-lab",
    actionLabel: "Inspect diff",
  },
];

describe("overview carousel hero", () => {
  it("renders four evidence cards with one accessible front card", async () => {
    const { OverviewCarousel } = await import(
      "../../src/features/overview/overview-carousel"
    );
    const markup = renderToStaticMarkup(<OverviewCarousel stages={stages} />);

    expect(markup).toContain('aria-label="Workflow evidence orbit"');
    expect(markup).toContain('aria-roledescription="3D card orbit"');
    expect(markup.match(/data-stage=/g)).toHaveLength(4);
    expect(markup.match(/aria-current="true"/g)).toHaveLength(1);
    expect(markup.match(/data-front="true"/g)).toHaveLength(1);
    expect(markup).toContain("Incident / Trace");
    expect(markup).toContain("Replay Capsule");
    expect(markup).toContain("Gateway → Checkout → Payment → Ledger");
    expect(markup).toContain("Bring Replay forward");
    expect(markup).not.toContain("Bring Incident / Trace forward");
  });

  it("announces a requested initial stage and exposes its full action", async () => {
    const { OverviewCarousel } = await import(
      "../../src/features/overview/overview-carousel"
    );
    const markup = renderToStaticMarkup(
      <OverviewCarousel initialStage="replay" stages={stages} />,
    );

    expect(markup).toContain('aria-live="polite"');
    expect(markup).toContain("Replay in focus");
    expect(markup).toContain('href="#replay-lab"');
    expect(markup).toContain("Open replay lab");
    expect(markup.match(/data-front="true"/g)).toHaveLength(1);
  });

  it("provides pause and directional controls for non-pointer input", async () => {
    const { OverviewCarousel } = await import(
      "../../src/features/overview/overview-carousel"
    );
    const markup = renderToStaticMarkup(<OverviewCarousel stages={stages} />);

    expect(markup).toContain('aria-label="Rotate orbit backward"');
    expect(markup).toContain('aria-label="Rotate orbit forward"');
    expect(markup).toContain('aria-label="Pause automatic rotation"');
    expect(markup).toContain('aria-keyshortcuts="ArrowLeft ArrowRight"');
  });
});
