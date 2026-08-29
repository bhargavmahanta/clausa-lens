// @vitest-environment jsdom

import { act, StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  OverviewCarousel,
  type CarouselStageContent,
} from "../../src/features/overview/overview-carousel";

const stages: CarouselStageContent[] = [
  ["incident", "Incident / Trace"],
  ["capsule", "Replay Capsule"],
  ["replay", "Replay"],
  ["diff", "Diff"],
].map(([stage, title]) => ({
  stage: stage as CarouselStageContent["stage"],
  eyebrow: "Evidence",
  title,
  description: "Description",
  summary: "Summary",
  href: "#workspace",
  actionLabel: "Open",
}));

class ResizeObserverStub {
  observe() {}
  disconnect() {}
}

describe("overview carousel client geometry", () => {
  let container: HTMLDivElement;

  beforeEach(() => {
    container = document.createElement("div");
    document.body.appendChild(container);
    vi.stubGlobal("ResizeObserver", ResizeObserverStub);
    vi.stubGlobal(
      "matchMedia",
      vi.fn().mockReturnValue({
        matches: false,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }),
    );
  });

  afterEach(() => {
    container.remove();
    vi.unstubAllGlobals();
  });

  it("applies a distinct 3D transform to every card after mounting", async () => {
    const root = createRoot(container);

    await act(async () => {
      root.render(
        <StrictMode>
          <OverviewCarousel autoRotateMs={60_000} stages={stages} />
        </StrictMode>,
      );
    });

    const transforms = Array.from(
      container.querySelectorAll<HTMLElement>(".orbit-card"),
      (card) => card.style.transform,
    );

    expect(transforms).toHaveLength(4);
    expect(new Set(transforms).size).toBe(4);
    expect(transforms.every(Boolean)).toBe(true);

    await act(async () => root.unmount());
  });
});
