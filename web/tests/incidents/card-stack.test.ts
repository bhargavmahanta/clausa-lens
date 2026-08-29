import { describe, expect, it } from "vitest";

describe("investigation card stack", () => {
  it("promotes the selected panel and assigns every background depth once", async () => {
    const { getPanelPlacement, investigationPanels } = await import(
      "../../src/features/incidents/card-stack"
    );

    expect(
      investigationPanels.map((panel) => getPanelPlacement("trace", panel)),
    ).toEqual(["left", "active", "right", "rear"]);
    expect(getPanelPlacement("evidence", "evidence")).toBe("active");
  });
});
