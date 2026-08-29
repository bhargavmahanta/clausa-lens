export const overviewCarouselStages = [
  "capture",
  "trace",
  "replay",
  "diff",
  "overview",
] as const;

export type OverviewCarouselStage = (typeof overviewCarouselStages)[number];

export type CarouselPlacement = "front" | "right" | "behindRight" | "behindLeft" | "left";

const placementOrder: readonly CarouselPlacement[] = [
  "front",
  "right",
  "behindRight",
  "behindLeft",
  "left",
];

export function getCarouselPlacement(
  activeStage: OverviewCarouselStage,
  stage: OverviewCarouselStage,
): CarouselPlacement {
  const activeIndex = overviewCarouselStages.indexOf(activeStage);
  const stageIndex = overviewCarouselStages.indexOf(stage);
  const relativeIndex =
    (stageIndex - activeIndex + overviewCarouselStages.length) % overviewCarouselStages.length;

  return placementOrder[relativeIndex] ?? "behindRight";
}

export function nextCarouselIndex(
  current: number,
  count: number = overviewCarouselStages.length,
): number {
  return (current + 1) % count;
}

export function previousCarouselIndex(
  current: number,
  count: number = overviewCarouselStages.length,
): number {
  return (current - 1 + count) % count;
}

export type CarouselTransform = {
  x: string;
  y: string;
  z: number;
  scale: number;
  rotateX: number;
  rotateY: number;
  rotateZ: number;
  opacity: number;
  zIndex: number;
};

const transforms: Record<CarouselPlacement, CarouselTransform> = {
  front: { x: "0%", y: "0%", z: 0, scale: 1, rotateX: 0, rotateY: 0, rotateZ: 0, opacity: 1, zIndex: 5 },
  right: { x: "64%", y: "-7%", z: -240, scale: 0.72, rotateX: 2, rotateY: -26, rotateZ: 4, opacity: 0.78, zIndex: 3 },
  behindRight: { x: "26%", y: "-22%", z: -520, scale: 0.58, rotateX: 7, rotateY: -14, rotateZ: 6, opacity: 0.42, zIndex: 1 },
  behindLeft: { x: "-26%", y: "-20%", z: -520, scale: 0.58, rotateX: 7, rotateY: 14, rotateZ: -6, opacity: 0.42, zIndex: 1 },
  left: { x: "-64%", y: "8%", z: -240, scale: 0.72, rotateX: 1, rotateY: 26, rotateZ: -4, opacity: 0.78, zIndex: 4 },
};

export function getCarouselTransform(placement: CarouselPlacement): CarouselTransform {
  return transforms[placement];
}
