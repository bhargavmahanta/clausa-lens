"use client";

import { motion, useReducedMotion } from "motion/react";
import { useEffect, useRef, useState } from "react";

import {
  getCarouselPlacement,
  getCarouselTransform,
  nextCarouselIndex,
  overviewCarouselStages,
  previousCarouselIndex,
  type OverviewCarouselStage,
} from "./carousel";

export type CarouselStageChip = {
  label: string;
  tone: "pass" | "neutral" | "warning" | "fail";
};

export type CarouselStageContent = {
  stage: OverviewCarouselStage;
  title: string;
  summary: string;
  description: string;
  statusChip?: CarouselStageChip;
  targetId?: string;
};

export type OverviewCarouselProps = {
  stages: CarouselStageContent[];
  initialStage?: OverviewCarouselStage;
  autoRotateMs?: number;
};

export function OverviewCarousel({
  stages,
  initialStage = "trace",
  autoRotateMs = 6000,
}: OverviewCarouselProps) {
  const [activeStage, setActiveStage] = useState<OverviewCarouselStage>(initialStage);
  const [paused, setPaused] = useState(false);
  const reduceMotion = useReducedMotion();
  const cardRefs = useRef<Partial<Record<OverviewCarouselStage, HTMLElement | null>>>({});

  useEffect(() => {
    if (paused || reduceMotion) return;
    const timer = setInterval(() => {
      setActiveStage((current) => {
        const index = overviewCarouselStages.indexOf(current);
        return overviewCarouselStages[nextCarouselIndex(index)];
      });
    }, autoRotateMs);
    return () => clearInterval(timer);
  }, [autoRotateMs, paused, reduceMotion]);

  const activeIndex = overviewCarouselStages.indexOf(activeStage);

  function rotateForward() {
    setActiveStage(overviewCarouselStages[nextCarouselIndex(activeIndex)]);
  }

  function rotateBackward() {
    setActiveStage(overviewCarouselStages[previousCarouselIndex(activeIndex)]);
  }

  function promote(stage: OverviewCarouselStage) {
    setActiveStage(stage);
    requestAnimationFrame(() => cardRefs.current[stage]?.focus());
  }

  const activeContent = stages.find((stage) => stage.stage === activeStage);

  return (
    <section className="overview-hero" aria-label="Workflow overview carousel">
      <div className="overview-hero__controls">
        <button aria-label="Rotate carousel backward" onClick={rotateBackward} type="button">
          ‹
        </button>
        <button
          aria-label={paused ? "Resume auto-rotation" : "Pause auto-rotation"}
          aria-pressed={paused}
          onClick={() => setPaused((value) => !value)}
          type="button"
        >
          {paused ? "▶" : "⏸"}
        </button>
        <button aria-label="Rotate carousel forward" onClick={rotateForward} type="button">
          ›
        </button>
      </div>

      <p className="sr-only" role="status" aria-live="polite">
        {activeContent ? `${activeContent.title} in focus` : ""}
      </p>

      <div className="carousel-stage">
        {stages.map((content) => {
          const placement = getCarouselPlacement(activeStage, content.stage);
          const isFront = placement === "front";
          const transform = getCarouselTransform(placement);
          const motionTarget = reduceMotion
            ? { opacity: transform.opacity, zIndex: transform.zIndex }
            : transform;

          return (
            <motion.article
              animate={motionTarget}
              aria-current={isFront ? "true" : undefined}
              aria-label={`${content.title} stage`}
              className="carousel-card"
              data-placement={placement}
              data-stage={content.stage}
              initial={false}
              key={content.stage}
              ref={(node) => {
                cardRefs.current[content.stage] = node;
              }}
              tabIndex={isFront ? -1 : undefined}
              transition={
                reduceMotion
                  ? { duration: 0 }
                  : { type: "spring", stiffness: 160, damping: 22, mass: 0.9 }
              }
            >
              <div className="carousel-card__content" inert={isFront ? undefined : true}>
                <header>
                  <span className="carousel-card__icon" aria-hidden="true" />
                  <div>
                    <h2>{content.title}</h2>
                    <p>{content.description}</p>
                  </div>
                </header>
                <p className="carousel-card__summary">{content.summary}</p>
                {content.statusChip ? (
                  <span className="status-chip" data-status={content.statusChip.tone}>
                    {content.statusChip.label}
                  </span>
                ) : null}
              </div>
              {!isFront ? (
                <button
                  className="carousel-card__promote"
                  onClick={() => promote(content.stage)}
                  type="button"
                >
                  Open {content.title}
                </button>
              ) : null}
            </motion.article>
          );
        })}
      </div>
    </section>
  );
}
