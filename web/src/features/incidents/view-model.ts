import type {
  ExecutionEvent,
  IncidentDetailResponse,
} from "../../lib/contracts";

export type IncidentTimelineEntry = {
  event: ExecutionEvent;
  timelineIndex: number;
};

export type StructuralEdge = {
  edgeId: string;
  fromEventId: string;
  toEventId: string;
  type: IncidentDetailResponse["graph"]["edges"][number]["type"];
};

export type IncidentEvidenceView = {
  requestId: string | undefined;
  componentPath: string[];
  timeline: IncidentTimelineEntry[];
  structuralEdges: StructuralEdge[];
  evidenceEvents: ExecutionEvent[];
};

export function buildIncidentView(
  detail: IncidentDetailResponse,
): IncidentEvidenceView {
  const timelineIndexByEventId = new Map(
    detail.graph.nodes.map((node) => [node.event_id, node.timeline_index]),
  );
  const eventById = new Map(
    detail.events.map((event) => [event.event_id, event]),
  );
  const componentPath = detail.events.reduce<string[]>((components, event) => {
    if (!components.includes(event.component.name)) {
      components.push(event.component.name);
    }
    return components;
  }, []);

  return {
    requestId: detail.events[0]?.logical_operation_id,
    componentPath,
    timeline: detail.events.map((event) => ({
      event,
      timelineIndex: timelineIndexByEventId.get(event.event_id) ?? -1,
    })),
    structuralEdges: detail.graph.edges.map((edge) => ({
      edgeId: edge.edge_id,
      fromEventId: edge.from_event_id,
      toEventId: edge.to_event_id,
      type: edge.type,
    })),
    evidenceEvents: detail.incident.evidence_event_ids.flatMap((eventId) => {
      const event = eventById.get(eventId);
      return event ? [event] : [];
    }),
  };
}
