"use client";

import { motion, useReducedMotion } from "motion/react";
import { useMemo, useRef, useState } from "react";

import { StatePanel } from "../../components/system";
import type {
  Incident,
  IncidentDetailResponse,
} from "../../lib/contracts";
import {
  getPanelPlacement,
  investigationPanels,
  type InvestigationPanel,
  type PanelPlacement,
} from "./card-stack";
import { buildIncidentView } from "./view-model";

export type IncidentDashboardState =
  | { status: "loading" }
  | { status: "empty" }
  | { status: "pending"; incident: Incident }
  | { status: "blocked"; incident: Incident }
  | { status: "unsupported"; message: string }
  | { status: "error"; message: string; code?: string }
  | {
      status: "ready";
      incidents: Incident[];
      detail: IncidentDetailResponse;
      nextCursor?: string;
    };

export type IncidentDashboardProps = {
  state: IncidentDashboardState;
  initialPanel?: InvestigationPanel;
  availableIncidents?: Incident[];
  onRetry?: () => void;
  onSelectIncident?: (incidentId: string) => void;
};

const panelLabels: Record<InvestigationPanel, string> = {
  incident: "Incident",
  trace: "Trace",
  timeline: "Timeline",
  evidence: "Evidence",
};

const placementMotion: Record<
  PanelPlacement,
  { x: string; y: string; scale: number; rotateX: number; rotateY: number; rotateZ: number; opacity: number; zIndex: number }
> = {
  active: { x: "0%", y: "0%", scale: 1, rotateX: 0, rotateY: 0, rotateZ: 0, opacity: 1, zIndex: 4 },
  right: { x: "68%", y: "-8%", scale: 0.73, rotateX: 2, rotateY: -27, rotateZ: 5, opacity: 0.76, zIndex: 2 },
  rear: { x: "0%", y: "-19%", scale: 0.68, rotateX: 8, rotateY: 0, rotateZ: 0, opacity: 0.48, zIndex: 1 },
  left: { x: "-68%", y: "7%", scale: 0.75, rotateX: 1, rotateY: 28, rotateZ: -6, opacity: 0.72, zIndex: 3 },
};

function formatTimestamp(timestamp: string) {
  return timestamp.slice(11, 23);
}

function IncidentPanel({
  incidents,
  hasMoreIncidents,
  selectedIncident,
  onSelectIncident,
}: {
  incidents: Incident[];
  hasMoreIncidents: boolean;
  selectedIncident: Incident;
  onSelectIncident?: (incidentId: string) => void;
}) {
  return (
    <div className="investigation-panel investigation-panel--incident">
      <header className="panel-header">
        <div>
          <p className="panel-kicker">Capture queue</p>
          <h2>{selectedIncident.incident_id}</h2>
        </div>
        <span className="status-chip" data-status={selectedIncident.status.toLowerCase()}>
          {selectedIncident.status}
        </span>
      </header>

      <p className="incident-summary">{selectedIncident.summary}</p>

      <div className="incident-list" aria-label="Captured incidents">
        {incidents.map((item) => (
          <button
            className="incident-list__item"
            data-selected={item.incident_id === selectedIncident.incident_id}
            disabled={item.incident_id === selectedIncident.incident_id}
            key={item.incident_id}
            onClick={() => onSelectIncident?.(item.incident_id)}
            type="button"
          >
            <span>{item.incident_id}</span>
            <small>{item.detected_at.slice(11, 23)} UTC</small>
          </button>
        ))}
      </div>

      {hasMoreIncidents ? (
        <p className="incident-list__notice">Additional incidents are available beyond this page.</p>
      ) : null}

      <dl className="compact-facts">
        <div><dt>System Pack</dt><dd>{selectedIncident.system_pack.id}</dd></div>
        <div><dt>Sanitization</dt><dd>{selectedIncident.sanitization_status}</dd></div>
        <div><dt>Evidence refs</dt><dd>{selectedIncident.evidence_event_ids.length}</dd></div>
      </dl>
    </div>
  );
}

function TracePanel({ detail }: { detail: IncidentDetailResponse }) {
  const view = useMemo(() => buildIncidentView(detail), [detail]);

  return (
    <div className="investigation-panel investigation-panel--trace">
      <header className="panel-header">
        <div>
          <p className="panel-kicker">Incident detail</p>
          <h2>Trace</h2>
        </div>
        <span className="panel-sequence">01 / 04</span>
      </header>

      <dl className="identifier-grid">
        <div><dt>Request</dt><dd>{view.requestId ?? "Unavailable"}</dd></div>
        <div><dt>Trace ID</dt><dd>{detail.incident.trace_id}</dd></div>
        <div><dt>Execution ID</dt><dd>{detail.incident.execution_id}</dd></div>
      </dl>

      <div className="component-path" aria-label="Components in timeline order">
        {view.componentPath.map((component, index) => (
          <div className="component-node" key={component}>
            <span>{String(index + 1).padStart(2, "0")}</span>
            <strong>{component}</strong>
          </div>
        ))}
      </div>

      <section className="edge-register" aria-labelledby="edge-register-title">
        <div className="panel-subheading">
          <h3 id="edge-register-title">Observed structure</h3>
          <span>{view.structuralEdges.length} edges</span>
        </div>
        <p className="evidence-note">Edges describe recorded ordering and relationships; they are not automatic causal claims.</p>
        <ul>
          {view.structuralEdges.map((edge) => (
            <li key={edge.edgeId}>
              <code>{edge.type}</code>
              <span>{edge.fromEventId}</span>
              <i aria-hidden="true">→</i>
              <span>{edge.toEventId}</span>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}

function TimelinePanel({ detail }: { detail: IncidentDetailResponse }) {
  const view = useMemo(() => buildIncidentView(detail), [detail]);

  return (
    <div className="investigation-panel investigation-panel--timeline">
      <header className="panel-header">
        <div>
          <p className="panel-kicker">Deterministic order</p>
          <h2>Timeline</h2>
        </div>
        <span className="panel-sequence">{view.timeline.length} events</span>
      </header>

      <ol className="event-timeline">
        {view.timeline.map(({ event, timelineIndex }) => (
          <li data-event-status={event.status.toLowerCase()} key={event.event_id}>
            <div className="timeline-time">
              <span>{formatTimestamp(event.occurred_at)}</span>
              <small>#{String(timelineIndex).padStart(2, "0")}</small>
            </div>
            <span className="timeline-rail" aria-hidden="true" />
            <div className="timeline-event">
              <div>
                <strong>{event.operation.name}</strong>
                <span>{event.component.name}</span>
              </div>
              <div className="timeline-meta">
                <span>Attempt {event.attempt}</span>
                <code>{event.event_type}</code>
              </div>
              {event.attributes.effect_id ? (
                <small className="effect-id">{event.attributes.effect_id}</small>
              ) : null}
            </div>
          </li>
        ))}
      </ol>
    </div>
  );
}

function EvidencePanel({ detail }: { detail: IncidentDetailResponse }) {
  const view = useMemo(() => buildIncidentView(detail), [detail]);

  return (
    <div className="investigation-panel investigation-panel--evidence">
      <header className="panel-header">
        <div>
          <p className="panel-kicker">Failure evidence</p>
          <h2>Oracle register</h2>
        </div>
        <span className="status-chip" data-status="ready">REFERENCED</span>
      </header>

      <div className="oracle-identity">
        <span>Failure oracle</span>
        <strong>{detail.incident.failure_oracle.id}</strong>
        <code>v{detail.incident.failure_oracle.version}</code>
      </div>

      <section className="evidence-events" aria-labelledby="evidence-events-title">
        <div className="panel-subheading">
          <h3 id="evidence-events-title">Incident-owned evidence</h3>
          <span>{view.evidenceEvents.length} references</span>
        </div>
        <ul>
          {view.evidenceEvents.map((event) => (
            <li key={event.event_id}>
              <span data-event-type={event.event_type.toLowerCase()}>{event.event_type}</span>
              <div><strong>{event.operation.name}</strong><code>{event.event_id}</code></div>
            </li>
          ))}
        </ul>
      </section>

      <section className="interpretation-card" aria-labelledby="interpretation-title">
        <span>API-provided interpretation</span>
        <h3 id="interpretation-title">{detail.incident.summary}</h3>
        <p>The interface shows the evidence backing this incident summary without upgrading structural edges into causal conclusions.</p>
      </section>
    </div>
  );
}

function RecoveryActions({
  availableIncidents,
  currentIncidentId,
  onRetry,
  onSelectIncident,
}: {
  availableIncidents: Incident[];
  currentIncidentId?: string;
  onRetry?: () => void;
  onSelectIncident?: (incidentId: string) => void;
}) {
  const alternatives = availableIncidents.filter(
    (incident) => incident.incident_id !== currentIncidentId,
  );

  if (alternatives.length === 0 && !onRetry) return null;

  return (
    <div className="incident-recovery">
      {alternatives.map((incident) => (
        <button
          key={incident.incident_id}
          onClick={() => onSelectIncident?.(incident.incident_id)}
          type="button"
        >
          Open {incident.incident_id}
        </button>
      ))}
      {onRetry ? <button onClick={onRetry} type="button">Retry request</button> : null}
    </div>
  );
}

function DashboardState({
  availableIncidents,
  onRetry,
  onSelectIncident,
  state,
}: {
  availableIncidents: Incident[];
  onRetry?: () => void;
  onSelectIncident?: (incidentId: string) => void;
  state: Exclude<IncidentDashboardState, { status: "ready" }>;
}) {
  const recovery = (
    <RecoveryActions
      availableIncidents={availableIncidents}
      currentIncidentId={"incident" in state ? state.incident.incident_id : undefined}
      onRetry={onRetry}
      onSelectIncident={onSelectIncident}
    />
  );

  if (state.status === "loading") {
    return <StatePanel state="loading" title="Loading incident evidence" message="Requesting the frozen incident list and ordered detail resource from the Core API." />;
  }
  if (state.status === "empty") {
    return <StatePanel state="empty" title="No captured incidents" message="The Core API returned an empty incident collection. Trigger the golden checkout failure to begin." />;
  }
  if (state.status === "pending") {
    return <StatePanel action={recovery} state="loading" title="Incident graph pending" message={`${state.incident.incident_id} is detected, but its ordered graph is not ready yet.`} />;
  }
  if (state.status === "blocked") {
    return (
      <StatePanel
        state="blocked"
        title="Incident blocked"
        message={state.incident.block_reason?.message ?? "The incident cannot advance to an execution graph."}
        code={state.incident.block_reason?.code}
        action={recovery}
      />
    );
  }
  if (state.status === "unsupported") {
    return <StatePanel action={recovery} state="unsupported" title="Unsupported API resource" message={state.message} code="UNSUPPORTED_CONTRACT" />;
  }
  return <StatePanel action={recovery} state="error" title="Incident evidence unavailable" message={state.message} code={state.code} />;
}

export function IncidentDashboard({
  availableIncidents = [],
  state,
  initialPanel = "trace",
  onRetry,
  onSelectIncident,
}: IncidentDashboardProps) {
  const [activePanel, setActivePanel] = useState<InvestigationPanel>(initialPanel);
  const cardRefs = useRef<Partial<Record<InvestigationPanel, HTMLElement | null>>>({});
  const reduceMotion = useReducedMotion();

  function promotePanel(panel: InvestigationPanel) {
    setActivePanel(panel);
    requestAnimationFrame(() => cardRefs.current[panel]?.focus());
  }

  if (state.status !== "ready") {
    return (
      <section className="incident-dashboard-state" aria-live="polite">
        <DashboardState
          availableIncidents={availableIncidents}
          onRetry={onRetry}
          onSelectIncident={onSelectIncident}
          state={state}
        />
      </section>
    );
  }

  const panelContent: Record<InvestigationPanel, React.ReactNode> = {
    incident: (
      <IncidentPanel
        incidents={state.incidents}
        hasMoreIncidents={Boolean(state.nextCursor)}
        onSelectIncident={onSelectIncident}
        selectedIncident={state.detail.incident}
      />
    ),
    trace: <TracePanel detail={state.detail} />,
    timeline: <TimelinePanel detail={state.detail} />,
    evidence: <EvidencePanel detail={state.detail} />,
  };

  return (
    <section className="incident-dashboard" aria-label="Incident Command Center">
      <nav className="panel-switcher" aria-label="Investigation sections">
        {investigationPanels.map((panel) => (
          <button
            aria-current={panel === activePanel ? "page" : undefined}
            key={panel}
            onClick={() => setActivePanel(panel)}
            type="button"
          >
            <span>{String(investigationPanels.indexOf(panel) + 1).padStart(2, "0")}</span>
            {panelLabels[panel]}
          </button>
        ))}
      </nav>

      <div className="card-stage">
        {investigationPanels.map((panel) => {
          const placement = getPanelPlacement(activePanel, panel);
          const isActive = placement === "active";
          const target = placementMotion[placement];
          const motionTarget = reduceMotion
            ? { opacity: target.opacity, zIndex: target.zIndex }
            : target;

          return (
            <motion.article
              animate={motionTarget}
              aria-current={isActive ? "true" : undefined}
              aria-labelledby={`panel-${panel}-title`}
              className="glass-card"
              data-panel={panel}
              data-placement={placement}
              initial={false}
              key={panel}
              ref={(node) => {
                cardRefs.current[panel] = node;
              }}
              tabIndex={isActive ? -1 : undefined}
              transition={reduceMotion ? { duration: 0 } : { type: "spring", stiffness: 190, damping: 24, mass: 0.85 }}
            >
              <span className="sr-only" id={`panel-${panel}-title`}>{panelLabels[panel]} section</span>
              <div className="glass-card__content" inert={isActive ? undefined : true}>
                {panelContent[panel]}
              </div>
              {!isActive ? (
                <button
                  className="glass-card__promote"
                  onClick={() => promotePanel(panel)}
                  type="button"
                >
                  <span>Open {panelLabels[panel]} section</span>
                </button>
              ) : null}
            </motion.article>
          );
        })}
      </div>
    </section>
  );
}
