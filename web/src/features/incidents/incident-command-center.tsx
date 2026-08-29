"use client";

import { useEffect, useMemo, useState } from "react";

import { createCausaLensClient } from "../../lib/api";
import type { Incident } from "../../lib/contracts";
import { IncidentDashboard, type IncidentDashboardState } from "./incident-dashboard";
import {
  classifyIncidentCollection,
  toIncidentDashboardError,
} from "./incident-resource";

const defaultApiBaseUrl = "http://localhost:8080";

export function IncidentCommandCenter() {
  const client = useMemo(
    () =>
      createCausaLensClient({
        baseUrl: process.env.NEXT_PUBLIC_CAUSALENS_API_URL ?? defaultApiBaseUrl,
      }),
    [],
  );
  const [incidents, setIncidents] = useState<Incident[]>([]);
  const [selectedIncidentId, setSelectedIncidentId] = useState<string>();
  const [state, setState] = useState<IncidentDashboardState>({ status: "loading" });
  const [nextCursor, setNextCursor] = useState<string>();
  const [requestVersion, setRequestVersion] = useState(0);

  useEffect(() => {
    let cancelled = false;

    async function loadIncidentCollection() {
      try {
        const response = await client.listIncidents({ limit: 20 });
        if (cancelled) return;

        setIncidents(response.items);
        setNextCursor(response.next_cursor);
        const classification = classifyIncidentCollection(response.items);
        if (classification.status === "selected") {
          setSelectedIncidentId(classification.incidentId);
          return;
        }
        setState(classification);
      } catch (error) {
        if (!cancelled) setState(toIncidentDashboardError(error));
      }
    }

    void loadIncidentCollection();
    return () => {
      cancelled = true;
    };
  }, [client, requestVersion]);

  useEffect(() => {
    if (!selectedIncidentId) return;
    let cancelled = false;

    async function loadIncidentDetail() {
      try {
        const detail = await client.getIncident(selectedIncidentId as string);
        if (!cancelled) {
          setState({ status: "ready", incidents, detail, nextCursor });
        }
      } catch (error) {
        if (!cancelled) setState(toIncidentDashboardError(error));
      }
    }

    void loadIncidentDetail();
    return () => {
      cancelled = true;
    };
  }, [client, incidents, nextCursor, selectedIncidentId]);

  function selectIncident(incidentId: string) {
    const selected = incidents.find((item) => item.incident_id === incidentId);
    if (!selected) return;
    if (selected.status === "BLOCKED") {
      setSelectedIncidentId(undefined);
      setState({ status: "blocked", incident: selected });
      return;
    }
    if (selected.status === "DETECTED") {
      setSelectedIncidentId(undefined);
      setState({ status: "pending", incident: selected });
      return;
    }
    setState({ status: "loading" });
    setSelectedIncidentId(incidentId);
  }

  function retryRequest() {
    setState({ status: "loading" });
    setSelectedIncidentId(undefined);
    setRequestVersion((version) => version + 1);
  }

  return (
    <IncidentDashboard
      availableIncidents={incidents}
      onRetry={retryRequest}
      onSelectIncident={selectIncident}
      state={state}
    />
  );
}
