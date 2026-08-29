"use client";

import { gsap } from "gsap";
import { useReducedMotion } from "motion/react";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type FocusEvent,
  type KeyboardEvent,
  type PointerEvent,
} from "react";

import {
  desktopOrbitGeometry,
  getFocusedStageIndex,
  getOrbitTransform,
  getShortestOrbitDelta,
  getStageTargetAngle,
  nextCarouselIndex,
  normalizeOrbitAngle,
  previousCarouselIndex,
  type OrbitGeometry,
  type OverviewCarouselStage,
} from "./carousel";
import {
  OverviewCard,
  type CarouselStageChip,
  type CarouselStageContent,
  type CarouselStageMetric,
} from "./overview-card";

export type { CarouselStageChip, CarouselStageContent, CarouselStageMetric };

export type OverviewCarouselProps = {
  stages: CarouselStageContent[];
  initialStage?: OverviewCarouselStage;
  autoRotateMs?: number;
};

type DragState = {
  pointerId: number;
  startX: number;
  startAngle: number;
  moved: boolean;
};

function getResponsiveGeometry(width: number): OrbitGeometry {
  if (width <= 720) {
    return {
      ...desktopOrbitGeometry,
      radiusX: Math.max(300, width * 0.86),
      radiusY: 96,
      depth: 360,
      minScale: 0.68,
      maxBlur: 3,
    };
  }

  if (width <= 980) {
    return {
      ...desktopOrbitGeometry,
      radiusX: Math.min(430, width * 0.48),
      radiusY: 132,
      depth: 440,
      minScale: 0.61,
    };
  }

  return desktopOrbitGeometry;
}

export function OverviewCarousel({
  stages,
  initialStage = "incident",
  autoRotateMs = 7000,
}: OverviewCarouselProps) {
  const initialIndex = Math.max(
    0,
    stages.findIndex((content) => content.stage === initialStage),
  );
  const [activeStage, setActiveStage] =
    useState<OverviewCarouselStage>(initialStage);
  const [manuallyPaused, setManuallyPaused] = useState(false);
  const [engaged, setEngaged] = useState(false);
  const reduceMotion = Boolean(useReducedMotion());
  const sceneRef = useRef<HTMLDivElement>(null);
  const cardRefs = useRef<Array<HTMLElement | null>>([]);
  const orbit = useRef({ angle: getStageTargetAngle(initialIndex, stages.length) });
  const animation = useRef<gsap.core.Tween | null>(null);
  const drag = useRef<DragState | null>(null);

  const applyTransforms = useCallback(() => {
    const geometry = getResponsiveGeometry(sceneRef.current?.clientWidth ?? 1180);

    cardRefs.current.forEach((card, index) => {
      if (!card) return;
      const value = getOrbitTransform(
        index,
        orbit.current.angle,
        stages.length,
        geometry,
      );
      gsap.set(card, {
        x: value.x,
        y: value.y,
        z: value.z,
        scale: value.scale,
        rotateX: value.rotateX,
        rotateY: value.rotateY,
        rotateZ: value.rotateZ,
        opacity: value.opacity,
        filter: `blur(${value.blur}px)`,
        zIndex: value.zIndex,
      });
    });
  }, [stages.length]);

  const promote = useCallback(
    (stage: OverviewCarouselStage) => {
      const index = stages.findIndex((content) => content.stage === stage);
      if (index < 0) return;
      const target = getStageTargetAngle(index, stages.length);
      const destination =
        orbit.current.angle +
        getShortestOrbitDelta(orbit.current.angle, target);

      animation.current?.kill();
      setActiveStage(stage);

      if (reduceMotion) {
        orbit.current.angle = normalizeOrbitAngle(destination);
        applyTransforms();
        cardRefs.current[index]?.focus();
        return;
      }

      animation.current = gsap.to(orbit.current, {
        angle: destination,
        duration: 0.92,
        ease: "power3.inOut",
        overwrite: true,
        onUpdate: applyTransforms,
        onComplete: () => {
          orbit.current.angle = normalizeOrbitAngle(destination);
          applyTransforms();
          cardRefs.current[index]?.focus();
        },
      });
    },
    [applyTransforms, reduceMotion, stages],
  );

  const rotateBy = useCallback(
    (direction: 1 | -1) => {
      const currentIndex = Math.max(
        0,
        stages.findIndex((content) => content.stage === activeStage),
      );
      const nextIndex =
        direction === 1
          ? nextCarouselIndex(currentIndex, stages.length)
          : previousCarouselIndex(currentIndex, stages.length);
      promote(stages[nextIndex].stage);
    },
    [activeStage, promote, stages],
  );

  useLayoutEffect(() => {
    const scene = sceneRef.current;
    if (!scene) return;

    const context = gsap.context(applyTransforms, scene);
    const observer = new ResizeObserver(applyTransforms);
    observer.observe(scene);

    return () => {
      animation.current?.kill();
      observer.disconnect();
      context.revert();
    };
  }, [applyTransforms]);

  useEffect(() => {
    if (manuallyPaused || engaged || reduceMotion || stages.length < 2) return;

    const timer = window.setInterval(() => rotateBy(1), autoRotateMs);
    return () => window.clearInterval(timer);
  }, [autoRotateMs, engaged, manuallyPaused, reduceMotion, rotateBy, stages.length]);

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === "ArrowRight") {
      event.preventDefault();
      rotateBy(1);
    } else if (event.key === "ArrowLeft") {
      event.preventDefault();
      rotateBy(-1);
    }
  }

  function handleFocus(event: FocusEvent<HTMLDivElement>) {
    if (event.currentTarget.contains(event.target)) setEngaged(true);
  }

  function handleBlur(event: FocusEvent<HTMLDivElement>) {
    if (!event.currentTarget.contains(event.relatedTarget)) setEngaged(false);
  }

  function handlePointerDown(event: PointerEvent<HTMLDivElement>) {
    if (event.button !== 0) return;
    drag.current = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startAngle: orbit.current.angle,
      moved: false,
    };
    animation.current?.kill();
    event.currentTarget.setPointerCapture(event.pointerId);
    setEngaged(true);
  }

  function handlePointerMove(event: PointerEvent<HTMLDivElement>) {
    const currentDrag = drag.current;
    if (!currentDrag || currentDrag.pointerId !== event.pointerId || reduceMotion) return;
    const delta = event.clientX - currentDrag.startX;
    currentDrag.moved ||= Math.abs(delta) > 6;
    orbit.current.angle = currentDrag.startAngle + delta * 0.0045;
    applyTransforms();
  }

  function handlePointerEnd(event: PointerEvent<HTMLDivElement>) {
    const currentDrag = drag.current;
    if (!currentDrag || currentDrag.pointerId !== event.pointerId) return;
    const delta = event.clientX - currentDrag.startX;
    drag.current = null;

    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }

    if (reduceMotion || Math.abs(delta) < 48) {
      promote(activeStage);
    } else {
      const focusedIndex = getFocusedStageIndex(orbit.current.angle, stages.length);
      promote(stages[focusedIndex].stage);
    }
  }

  return (
    <section
      aria-label="Workflow evidence orbit"
      className="overview-hero"
      onBlur={handleBlur}
      onFocus={handleFocus}
      onMouseEnter={() => setEngaged(true)}
      onMouseLeave={() => setEngaged(false)}
    >
      <div className="overview-hero__heading">
        <div>
          <p className="eyebrow">Incident evidence / controlled replay</p>
          <h1>See the failure. Change one thing.</h1>
        </div>
        <p>
          Rotate through the complete investigation, from captured trace to
          first meaningful divergence.
        </p>
      </div>

      <div className="overview-hero__controls" aria-label="Orbit controls">
        <button
          aria-label="Rotate orbit backward"
          onClick={() => rotateBy(-1)}
          type="button"
        >
          Previous
        </button>
        <button
          aria-label={
            manuallyPaused
              ? "Resume automatic rotation"
              : "Pause automatic rotation"
          }
          aria-pressed={manuallyPaused}
          onClick={() => setManuallyPaused((value) => !value)}
          type="button"
        >
          {manuallyPaused ? "Resume" : "Pause"}
        </button>
        <button
          aria-label="Rotate orbit forward"
          onClick={() => rotateBy(1)}
          type="button"
        >
          Next
        </button>
      </div>

      <p className="sr-only" role="status" aria-live="polite">
        {stages.find((content) => content.stage === activeStage)?.title ?? ""} in focus
      </p>

      <div
        aria-keyshortcuts="ArrowLeft ArrowRight"
        aria-roledescription="3D card orbit"
        className="orbit-stage"
        onKeyDown={handleKeyDown}
        onPointerCancel={handlePointerEnd}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerEnd}
        ref={sceneRef}
        role="group"
        tabIndex={0}
      >
        <div className="orbit-stage__axis" aria-hidden="true" />
        {stages.map((content, index) => {
          const isFront = content.stage === activeStage;
          return (
            <OverviewCard
              articleRef={(node) => {
                cardRefs.current[index] = node;
              }}
              content={content}
              isFront={isFront}
              key={content.stage}
              onPromote={() => promote(content.stage)}
            />
          );
        })}
      </div>

      <div className="orbit-progress" aria-label="Orbit position">
        {stages.map((content, index) => (
          <button
            aria-label={`Show ${content.title}`}
            aria-pressed={content.stage === activeStage}
            key={content.stage}
            onClick={() => promote(content.stage)}
            type="button"
          >
            <span>{String(index + 1).padStart(2, "0")}</span>
            <span>{content.title}</span>
          </button>
        ))}
      </div>
    </section>
  );
}
