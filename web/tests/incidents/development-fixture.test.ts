import { describe, expect, it } from "vitest";

import {
  incidentDetailResponseSchema,
  incidentListResponseSchema,
} from "../../src/lib/contracts";
import {
  developmentIncidentDetail,
  developmentIncidentList,
} from "../../src/features/incidents/development-fixture";

describe("development incident fixture", () => {
  it("is a frozen-contract-valid list and ordered detail resource", () => {
    const list = incidentListResponseSchema.parse(developmentIncidentList);
    const detail = incidentDetailResponseSchema.parse(developmentIncidentDetail);

    expect(list.items).toHaveLength(1);
    expect(list.items[0].incident_id).toBe(detail.incident.incident_id);
    expect(detail.events.map((event) => event.event_type)).toEqual([
      "START",
      "START",
      "START",
      "TIMEOUT",
      "RETRY",
      "START",
      "EFFECT",
      "EFFECT",
    ]);
  });
});
