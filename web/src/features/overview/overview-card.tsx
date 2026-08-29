import type { Ref } from "react";

import type { OverviewCarouselStage } from "./carousel";

export type CarouselStageChip = {
  label: string;
  tone: "pass" | "neutral" | "warning" | "fail";
};

export type CarouselStageMetric = {
  label: string;
  value: string;
};

export type CarouselStageContent = {
  stage: OverviewCarouselStage;
  eyebrow: string;
  title: string;
  description: string;
  summary: string;
  href: string;
  actionLabel: string;
  statusChip?: CarouselStageChip;
  metrics?: CarouselStageMetric[];
};

type OverviewCardProps = {
  articleRef: Ref<HTMLElement>;
  content: CarouselStageContent;
  isFront: boolean;
  onPromote: () => void;
};

export function OverviewCard({
  articleRef,
  content,
  isFront,
  onPromote,
}: OverviewCardProps) {
  return (
    <article
      aria-current={isFront ? "true" : undefined}
      aria-label={`${content.title} evidence card`}
      className="orbit-card"
      data-front={isFront ? "true" : "false"}
      data-stage={content.stage}
      ref={articleRef}
      tabIndex={isFront ? -1 : undefined}
    >
      {!isFront ? (
        <button
          aria-label={`Bring ${content.title} forward`}
          className="orbit-card__promote"
          onClick={onPromote}
          type="button"
        />
      ) : null}

      <div className="orbit-card__inner" inert={isFront ? undefined : true}>
        <header className="orbit-card__header">
          <span className="orbit-card__index" aria-hidden="true">
            {String(content.stage === "incident" ? 1 : content.stage === "capsule" ? 2 : content.stage === "replay" ? 3 : 4).padStart(2, "0")}
          </span>
          <div>
            <p className="orbit-card__eyebrow">{content.eyebrow}</p>
            <h2>{content.title}</h2>
            <p className="orbit-card__description">{content.description}</p>
          </div>
          {content.statusChip ? (
            <span className="status-chip" data-status={content.statusChip.tone}>
              {content.statusChip.label}
            </span>
          ) : null}
        </header>

        <div className="orbit-card__signal" aria-hidden="true">
          <span />
          <span />
          <span />
        </div>

        <p className="orbit-card__summary">{content.summary}</p>

        {content.metrics?.length ? (
          <dl className="orbit-card__metrics">
            {content.metrics.map((metric) => (
              <div key={metric.label}>
                <dt>{metric.label}</dt>
                <dd>{metric.value}</dd>
              </div>
            ))}
          </dl>
        ) : (
          <div className="orbit-card__empty-metrics" aria-hidden="true">
            <span />
            <span />
            <span />
          </div>
        )}

        <a className="orbit-card__action" href={content.href}>
          <span>{content.actionLabel}</span>
          <span aria-hidden="true">Open</span>
        </a>
      </div>
    </article>
  );
}
