import { describe, expect, it } from "vitest";

describe("overview carousel ring", () => {
  it("places exactly one card in front for five workflow stages", async () => {
    const { getCarouselPlacement, overviewCarouselStages } = await import(
      "../../src/features/overview/carousel"
    );

    expect(overviewCarouselStages).toEqual(["capture", "trace", "replay", "diff", "overview"]);

    for (const active of overviewCarouselStages) {
      const placements = overviewCarouselStages.map((stage) =>
        getCarouselPlacement(active, stage),
      );
      expect(placements.filter((placement) => placement === "front")).toHaveLength(1);
      expect(placements.filter((placement) => placement === "behindLeft")).toHaveLength(1);
      expect(placements.filter((placement) => placement === "behindRight")).toHaveLength(1);
      expect(placements.filter((placement) => placement === "left")).toHaveLength(1);
      expect(placements.filter((placement) => placement === "right")).toHaveLength(1);
    }
  });

  it("advances the ring deterministically for auto-rotation", async () => {
    const { nextCarouselIndex, overviewCarouselStages } = await import(
      "../../src/features/overview/carousel"
    );

    expect(nextCarouselIndex(0)).toBe(1);
    expect(nextCarouselIndex(3)).toBe(4);
    expect(nextCarouselIndex(4)).toBe(0);

    let index = 1;
    const visited: number[] = [];
    for (let step = 0; step < overviewCarouselStages.length; step += 1) {
      visited.push(index);
      index = nextCarouselIndex(index);
    }
    expect(index).toBe(1);
    expect(visited).toEqual([1, 2, 3, 4, 0]);
  });

  it("derives the diagonal ring geometry from the relative position", async () => {
    const { getCarouselTransform } = await import("../../src/features/overview/carousel");

    const front = getCarouselTransform("front");
    expect(front).toMatchObject({ x: "0%", y: "0%", z: 0, scale: 1, rotateY: 0 });

    const left = getCarouselTransform("left");
    expect(left.z).toBeLessThan(0);
    expect(left.rotateY).toBeGreaterThan(0);
    expect(left.scale).toBeLessThan(1);

    const right = getCarouselTransform("right");
    expect(right.z).toBeLessThan(0);
    expect(right.rotateY).toBeLessThan(0);
    expect(right.scale).toBeLessThan(1);

    const behind = getCarouselTransform("behindLeft");
    expect(behind.z).toBeLessThan(left.z);
    expect(behind.opacity).toBeLessThan(1);

    expect(getCarouselTransform("behindRight")).toMatchObject({
      z: behind.z,
      opacity: behind.opacity,
    });
  });
});
