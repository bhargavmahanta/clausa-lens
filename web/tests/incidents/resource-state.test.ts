import { describe, expect, it } from "vitest";

import { CausaLensApiError, ContractDecodeError } from "../../src/lib/api";
import type { Incident } from "../../src/lib/contracts";
import { incident } from "../fixtures/golden-contracts";

describe("incident resource state", () => {
  it("selects a ready incident and keeps empty, detected, and blocked collections explicit", async () => {
    const { classifyIncidentCollection } = await import(
      "../../src/features/incidents/incident-resource"
    );
    const ready = incident as unknown as Incident;
    const detected = { ...ready, status: "DETECTED", graph_id: undefined } as Incident;
    const blocked = {
      ...ready,
      status: "BLOCKED",
      graph_id: undefined,
      sanitization_status: "FAIL",
      block_reason: {
        code: "SANITIZATION_FAILED",
        path: "trigger.body",
        message: "Sanitization failed.",
      },
    } as Incident;

    expect(classifyIncidentCollection([])).toEqual({ status: "empty" });
    expect(classifyIncidentCollection([detected])).toMatchObject({ status: "pending" });
    expect(classifyIncidentCollection([blocked])).toMatchObject({ status: "blocked" });
    expect(classifyIncidentCollection([detected, ready])).toEqual({
      status: "selected",
      incidentId: "inc-8271",
    });
  });

  it("distinguishes unsupported resources from stable API failures", async () => {
    const { toIncidentDashboardError } = await import(
      "../../src/features/incidents/incident-resource"
    );
    const decodeError = new ContractDecodeError("IncidentDetailResponse", []);
    const apiError = new CausaLensApiError(422, {
      code: "SANITIZATION_FAILED",
      message: "Incident detail is blocked.",
      retryable: false,
      details: {},
    });

    expect(toIncidentDashboardError(decodeError)).toMatchObject({
      status: "unsupported",
    });
    expect(toIncidentDashboardError(apiError)).toEqual({
      status: "error",
      message: "Incident detail is blocked.",
      code: "SANITIZATION_FAILED",
    });
  });
});
