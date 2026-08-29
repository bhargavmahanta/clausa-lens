import { describe, expect, it } from "vitest";

describe("overview carousel orbit", () => {
  it("defines the four evidence objects in workflow order", async () => {
    const { overviewCarouselStages } = await import(
      "../../src/features/overview/carousel"
    );

    expect(overviewCarouselStages).toEqual([
      "incident",
      "capsule",
      "replay",
      "diff",
    ]);
  });

  it("places the selected stage at the front and its opposite behind", async () => {
    const { getOrbitTransform, getStageTargetAngle } = await import(
      "../../src/features/overview/carousel"
    );
    const angle = getStageTargetAngle(2, 4);
    const selected = getOrbitTransform(2, angle, 4);
    const opposite = getOrbitTransform(0, angle, 4);

    expect(selected.frontness).toBeCloseTo(1, 5);
    expect(selected.x).toBeCloseTo(0, 5);
    expect(selected.z).toBeGreaterThan(opposite.z);
    expect(selected.scale).toBeGreaterThan(opposite.scale);
    expect(selected.blur).toBeLessThan(opposite.blur);
  });

  it("uses the shortest curved rotation across the wrap boundary", async () => {
    const { getShortestOrbitDelta } = await import(
      "../../src/features/overview/carousel"
    );

    expect(
      getShortestOrbitDelta(Math.PI * 0.95, -Math.PI * 0.95),
    ).toBeCloseTo(Math.PI * 0.1, 5);
    expect(
      getShortestOrbitDelta(-Math.PI * 0.95, Math.PI * 0.95),
    ).toBeCloseTo(-Math.PI * 0.1, 5);
  });

  it("changes position continuously between two orbit samples", async () => {
    const { getOrbitTransform } = await import(
      "../../src/features/overview/carousel"
    );
    const start = getOrbitTransform(1, 0, 4);
    const middle = getOrbitTransform(1, Math.PI / 8, 4);
    const end = getOrbitTransform(1, Math.PI / 4, 4);

    expect(middle.x).not.toBe(start.x);
    expect(middle.x).not.toBe(end.x);
    expect(middle.z).toBeGreaterThan(Math.min(start.z, end.z));
    expect(middle.z).toBeLessThan(Math.max(start.z, end.z));
  });

  it("lifts the side cards while dropping the rear card along the curved surface", async () => {
    const { getOrbitTransform } = await import(
      "../../src/features/overview/carousel"
    );
    const front = getOrbitTransform(0, 0, 4);
    const right = getOrbitTransform(1, 0, 4);
    const rear = getOrbitTransform(2, 0, 4);
    const left = getOrbitTransform(3, 0, 4);

    expect(right.y).toBeLessThan(front.y - 140);
    expect(left.y).toBeLessThan(front.y - 140);
    expect(right.rotateZ).toBeLessThan(0);
    expect(left.rotateZ).toBeGreaterThan(0);
    expect(rear.y).toBeGreaterThan(front.y + 300);
  });

  it("identifies the nearest front stage throughout wrap-around", async () => {
    const { getFocusedStageIndex, getStageTargetAngle } = await import(
      "../../src/features/overview/carousel"
    );

    for (let index = 0; index < 4; index += 1) {
      expect(getFocusedStageIndex(getStageTargetAngle(index, 4), 4)).toBe(index);
    }
  });
});
