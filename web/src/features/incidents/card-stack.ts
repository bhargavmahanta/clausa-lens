export const investigationPanels = [
  "incident",
  "trace",
  "timeline",
  "evidence",
] as const;

export type InvestigationPanel = (typeof investigationPanels)[number];
export type PanelPlacement = "active" | "right" | "rear" | "left";

const placements: readonly PanelPlacement[] = ["active", "right", "rear", "left"];

export function getPanelPlacement(
  activePanel: InvestigationPanel,
  panel: InvestigationPanel,
): PanelPlacement {
  const activeIndex = investigationPanels.indexOf(activePanel);
  const panelIndex = investigationPanels.indexOf(panel);
  const relativeIndex = (panelIndex - activeIndex + investigationPanels.length) % investigationPanels.length;

  return placements[relativeIndex] ?? "rear";
}
