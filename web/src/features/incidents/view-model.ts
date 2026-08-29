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

export type ExecutionGraphNodeView = {
  event: ExecutionEvent;
  timelineIndex: number;
};

export type IncidentEvidenceView = {
  requestId: string | undefined;
  componentPath: string[];
  timeline: IncidentTimelineEntry[];
  graphNodes: ExecutionGraphNodeView[];
  structuralEdges: StructuralEdge[];
  evidenceEvents: ExecutionEvent[];
};

export function buildIncidentView(
  detail: IncidentDetailResponse,
): IncidentEvidenceView {
  const eventById = new Map(
    detail.events.map((event) => [event.event_id, event]),
  );
  const graphNodes = detail.graph.nodes
    .flatMap((node) => {
      const event = eventById.get(node.event_id);
      return event ? [{ event, timelineIndex: node.timeline_index }] : [];
    })
    .sort((left, right) => left.timelineIndex - right.timelineIndex);
  const componentPath = graphNodes.reduce<string[]>((components, { event }) => {
    if (!components.includes(event.component.name)) {
      components.push(event.component.name);
    }
    return components;
  }, []);

  return {
    requestId: graphNodes[0]?.event.logical_operation_id,
    componentPath,
    timeline: graphNodes,
    graphNodes,
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
