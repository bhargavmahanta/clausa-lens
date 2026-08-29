"use client";

import { useEffect, useMemo, useState } from "react";

import { createCausaLensClient } from "../../lib/api";
import type { Incident } from "../../lib/contracts";
import { IncidentDashboard, type IncidentDashboardState } from "./incident-dashboard";
import {
  classifyIncidentCollection,
  toIncidentDashboardError,
} from "./incident-resource";
import { resolveIncidentDataSource } from "./data-source";

export const incidentDataSource = resolveIncidentDataSource({
  isDevelopment: process.env.NODE_ENV === "development",
});

export type SelectionRequest = {
  incidentId: string;
  nonce: number;
};

export function IncidentCommandCenter({
  onSelectionChange,
  selectionRequest,
}: {
  onSelectionChange?: (incidentId: string | undefined) => void;
  selectionRequest?: SelectionRequest;
}) {
  const client = useMemo(
    () =>
      createCausaLensClient({
        baseUrl: incidentDataSource.baseUrl,
      }),
    [],
  );
  const [incidents, setIncidents] = useState<Incident[]>([]);
  const [selectedIncidentId, setSelectedIncidentId] = useState<string>();
  const [state, setState] = useState<IncidentDashboardState>({ status: "loading" });
  const [nextCursor, setNextCursor] = useState<string>();
  const [requestVersion, setRequestVersion] = useState(0);
  const [pendingSelection, setPendingSelection] = useState<string>();
  const [lastSelectionRequest, setLastSelectionRequest] = useState<SelectionRequest>();

  if (selectionRequest && selectionRequest !== lastSelectionRequest) {
    setLastSelectionRequest(selectionRequest);
    setPendingSelection(selectionRequest.incidentId);
  }

  useEffect(() => {
    let cancelled = false;

    async function loadIncidentCollection() {
      try {
        const response = await client.listIncidents({ limit: 100 });
        if (cancelled) return;

        setIncidents(response.items);
        setNextCursor(response.next_cursor);

        if (pendingSelection) {
          const requested = response.items.find(
            (item) => item.incident_id === pendingSelection && item.status === "READY",
          );
          if (requested) {
            setPendingSelection(undefined);
            setSelectedIncidentId(requested.incident_id);
            return;
          }
        }

        const classification = classifyIncidentCollection(response.items);
        if (classification.status === "selected") {
          setSelectedIncidentId(classification.incidentId);
          return;
        }
        onSelectionChange?.(undefined);
        setState(classification);
      } catch (error) {
        if (!cancelled) setState(toIncidentDashboardError(error));
      }
    }

    void loadIncidentCollection();
    return () => {
      cancelled = true;
    };
  }, [client, onSelectionChange, pendingSelection, requestVersion]);

  useEffect(() => {
    if (!selectedIncidentId) return;
    let cancelled = false;

    async function loadIncidentDetail() {
      try {
        const detail = await client.getIncident(selectedIncidentId as string);
        if (!cancelled) {
          setState({ status: "ready", incidents, detail, nextCursor });
          onSelectionChange?.(detail.incident.incident_id);
        }
      } catch (error) {
        if (!cancelled) setState(toIncidentDashboardError(error));
      }
    }

    void loadIncidentDetail();
    return () => {
      cancelled = true;
    };
  }, [client, incidents, nextCursor, onSelectionChange, selectedIncidentId]);

  function selectIncident(incidentId: string) {
    const selected = incidents.find((item) => item.incident_id === incidentId);
    if (!selected) return;
    if (selected.status === "BLOCKED") {
      onSelectionChange?.(undefined);
      setSelectedIncidentId(undefined);
      setState({ status: "blocked", incident: selected });
      return;
    }
    if (selected.status === "DETECTED") {
      onSelectionChange?.(undefined);
      setSelectedIncidentId(undefined);
      setState({ status: "pending", incident: selected });
      return;
    }
    setState({ status: "loading" });
    setSelectedIncidentId(incidentId);
  }

  function retryRequest() {
    onSelectionChange?.(undefined);
    setState({ status: "loading" });
    setSelectedIncidentId(undefined);
    setRequestVersion((version) => version + 1);
  }

  return (
    <>
      {incidentDataSource.mode === "fixture" ? (
        <p className="fixture-mode-notice" role="status">
          Development fixture preview · Core API not connected
        </p>
      ) : null}
      <IncidentDashboard
        availableIncidents={incidents}
        onRetry={retryRequest}
        onSelectIncident={selectIncident}
        state={state}
      />
    </>
  );
}
