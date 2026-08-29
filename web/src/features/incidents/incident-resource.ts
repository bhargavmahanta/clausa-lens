import {
  CausaLensApiError,
  ContractDecodeError,
  ProtocolError,
} from "../../lib/api";
import type { Incident } from "../../lib/contracts";
import type { IncidentDashboardState } from "./incident-dashboard";

export type IncidentCollectionClassification =
  | { status: "empty" }
  | { status: "pending"; incident: Incident }
  | { status: "blocked"; incident: Incident }
  | { status: "selected"; incidentId: string };

export function classifyIncidentCollection(
  incidents: Incident[],
): IncidentCollectionClassification {
  if (incidents.length === 0) {
    return { status: "empty" };
  }

  const readyIncident = incidents.find((item) => item.status === "READY");
  if (readyIncident) {
    return { status: "selected", incidentId: readyIncident.incident_id };
  }

  const pendingIncident = incidents.find((item) => item.status === "DETECTED");
  if (pendingIncident) {
    return { status: "pending", incident: pendingIncident };
  }

  return { status: "blocked", incident: incidents[0] };
}

export function toIncidentDashboardError(error: unknown): IncidentDashboardState {
  if (error instanceof ContractDecodeError) {
    return {
      status: "unsupported",
      message: `${error.message} ${error.issues.length} contract issue${error.issues.length === 1 ? "" : "s"} reported.`,
    };
  }

  if (error instanceof CausaLensApiError) {
    return {
      status: "error",
      message: error.message,
      code: error.code,
    };
  }

  if (error instanceof ProtocolError) {
    return {
      status: "error",
      message: error.message,
      code: "HTTP_PROTOCOL",
    };
  }

  return {
    status: "error",
    message: error instanceof Error ? error.message : "The incident request failed unexpectedly.",
  };
}
