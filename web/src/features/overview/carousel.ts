export const overviewCarouselStages = [
  "incident",
  "capsule",
  "replay",
  "diff",
] as const;

export type OverviewCarouselStage = (typeof overviewCarouselStages)[number];

export type OrbitGeometry = {
  radiusX: number;
  sideLift: number;
  backDrop: number;
  depth: number;
  tilt: number;
  minScale: number;
  maxBlur: number;
};

export type OrbitTransform = {
  x: number;
  y: number;
  z: number;
  scale: number;
  rotateX: number;
  rotateY: number;
  rotateZ: number;
  opacity: number;
  blur: number;
  zIndex: number;
  frontness: number;
};

export const desktopOrbitGeometry: OrbitGeometry = {
  radiusX: 520,
  sideLift: 170,
  backDrop: 360,
  depth: 520,
  tilt: -0.02,
  minScale: 0.6,
  maxBlur: 2.4,
};

const fullTurn = Math.PI * 2;

export function normalizeOrbitAngle(angle: number): number {
  return ((angle + Math.PI) % fullTurn + fullTurn) % fullTurn - Math.PI;
}

export function getShortestOrbitDelta(
  currentAngle: number,
  targetAngle: number,
): number {
  return normalizeOrbitAngle(targetAngle - currentAngle);
}

export function getStageTargetAngle(
  stageIndex: number,
  count: number = overviewCarouselStages.length,
): number {
  return normalizeOrbitAngle(-(stageIndex * fullTurn) / count);
}

export function getFocusedStageIndex(
  orbitAngle: number,
  count: number = overviewCarouselStages.length,
): number {
  let focused = 0;
  let closest = Number.POSITIVE_INFINITY;

  for (let index = 0; index < count; index += 1) {
    const distance = Math.abs(
      normalizeOrbitAngle(orbitAngle + (index * fullTurn) / count),
    );
    if (distance < closest) {
      focused = index;
      closest = distance;
    }
  }

  return focused;
}

export function getOrbitTransform(
  stageIndex: number,
  orbitAngle: number,
  count: number = overviewCarouselStages.length,
  geometry: OrbitGeometry = desktopOrbitGeometry,
): OrbitTransform {
  const theta = normalizeOrbitAngle(
    orbitAngle + (stageIndex * fullTurn) / count,
  );
  const rawX = Math.sin(theta) * geometry.radiusX;
  const curveProgress = 1 - Math.cos(theta);
  const curveQuadratic = geometry.backDrop / 2 + geometry.sideLift;
  const curveLinear = -geometry.sideLift - curveQuadratic;
  const rawY =
    curveLinear * curveProgress +
    curveQuadratic * curveProgress * curveProgress;
  const tiltCos = Math.cos(geometry.tilt);
  const tiltSin = Math.sin(geometry.tilt);
  const frontness = (Math.cos(theta) + 1) / 2;

  return {
    x: rawX * tiltCos - rawY * tiltSin,
    y: rawX * tiltSin + rawY * tiltCos,
    z: (Math.cos(theta) - 1) * geometry.depth,
    scale: geometry.minScale + (1 - geometry.minScale) * frontness,
    rotateX: (1 - frontness) * 8,
    rotateY: Math.sin(theta) * -24,
    rotateZ: Math.sin(theta) * -14,
    opacity: 0.48 + frontness * 0.52,
    blur: geometry.maxBlur * (1 - frontness),
    zIndex: Math.round(frontness * 100),
    frontness,
  };
}

export function nextCarouselIndex(current: number, count: number): number {
  return (current + 1) % count;
}

export function previousCarouselIndex(current: number, count: number): number {
  return (current - 1 + count) % count;
}
