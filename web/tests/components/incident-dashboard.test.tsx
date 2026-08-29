import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { Incident, IncidentDetailResponse } from "../../src/lib/contracts";
import { goldenIncidentDetail, incident } from "../fixtures/golden-contracts";

describe("C2 incident dashboard", () => {
  it("renders identifiers, observed graph structure, ordered events, attempts, and oracle references", async () => {
    const { IncidentDashboard } = await import(
      "../../src/features/incidents/incident-dashboard"
    );
    const markup = renderToStaticMarkup(
      <IncidentDashboard
        state={{
          status: "ready",
          incidents: [incident as unknown as Incident],
          detail: goldenIncidentDetail as unknown as IncidentDetailResponse,
          nextCursor: "next-page",
        }}
      />,
    );

    expect(markup).toContain("inc-8271");
    expect(markup).toContain("trace-8271");
    expect(markup).toContain("exec-original-8271");
    expect(markup).toContain("Observed structure");
    expect(markup).toContain("Components in timeline order");
    expect(markup).not.toContain("Component execution path");
    expect(markup).not.toContain("Causal edge");
    expect(markup).toContain("Attempt 1");
    expect(markup).toContain("Attempt 2");
    expect(markup).toContain("ledger-effect-1");
    expect(markup).toContain("ledger-effect-2");
    expect(markup).toContain("duplicate_ledger_effect");
    expect(markup).toContain("Additional incidents are available beyond this page.");
    expect(markup).toContain('disabled=""');
    expect(markup.indexOf("payment-timeout")).toBeLessThan(
      markup.indexOf("retry-payment"),
    );
  });

  it("exposes four keyboard-addressable panels with one active front card", async () => {
    const { IncidentDashboard } = await import(
      "../../src/features/incidents/incident-dashboard"
    );
    const markup = renderToStaticMarkup(
      <IncidentDashboard
        initialPanel="timeline"
        state={{
          status: "ready",
          incidents: [incident as unknown as Incident],
          detail: goldenIncidentDetail as unknown as IncidentDetailResponse,
        }}
      />,
    );

    expect(markup).toContain('aria-label="Investigation sections"');
    expect(markup.match(/data-panel=/g)).toHaveLength(4);
    expect(markup.match(/data-placement="active"/g)).toHaveLength(1);
    expect(markup).toContain('tabindex="-1"');
    expect(markup).toContain('aria-current="true"');
    expect(markup).toContain("Open Incident section");
    expect(markup).toContain("Open Trace section");
    expect(markup).toContain("Open Evidence section");
  });

  it("renders honest loading, empty, blocked, and malformed-resource states", async () => {
    const { IncidentDashboard } = await import(
      "../../src/features/incidents/incident-dashboard"
    );
    const loading = renderToStaticMarkup(
      <IncidentDashboard state={{ status: "loading" }} />,
    );
    const empty = renderToStaticMarkup(
      <IncidentDashboard state={{ status: "empty" }} />,
    );
    const pending = renderToStaticMarkup(
      <IncidentDashboard
        state={{
          status: "pending",
          incident: {
            ...incident,
            status: "DETECTED",
            graph_id: undefined,
          } as unknown as Incident,
        }}
      />,
    );
    const blockedIncident = {
      ...incident,
      incident_id: "inc-blocked",
      status: "BLOCKED",
      graph_id: undefined,
      sanitization_status: "FAIL",
      block_reason: {
        code: "SANITIZATION_FAILED",
        path: "trigger.body",
        message: "Captured input did not pass sanitization.",
      },
    } as unknown as Incident;
    const blocked = renderToStaticMarkup(
      <IncidentDashboard
        availableIncidents={[incident as unknown as Incident, blockedIncident]}
        onRetry={() => undefined}
        onSelectIncident={() => undefined}
        state={{
          status: "blocked",
          incident: blockedIncident,
        }}
      />,
    );
    const unsupported = renderToStaticMarkup(
      <IncidentDashboard
        state={{
          status: "unsupported",
          message: "The API returned an unknown event status.",
        }}
      />,
    );

    expect(loading).toContain("Loading incident evidence");
    expect(empty).toContain("No captured incidents");
    expect(pending).toContain("Incident graph pending");
    expect(blocked).toContain("Incident blocked");
    expect(blocked).toContain("SANITIZATION_FAILED");
    expect(blocked).toContain("Open inc-8271");
    expect(blocked).toContain("Retry request");
    expect(unsupported).toContain("Unsupported API resource");
  });
});
