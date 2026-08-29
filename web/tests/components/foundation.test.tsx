import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { baselineRun } from "../fixtures/golden-contracts";

describe("Command Center foundation", () => {
  it("provides a keyboard-visible skip link and labelled main landmark", async () => {
    const { CommandCenterShell } = await import("../../src/components/system");
    const markup = renderToStaticMarkup(<CommandCenterShell><p>Evidence</p></CommandCenterShell>);

    expect(markup).toContain('class="skip-link"');
    expect(markup).toContain('href="#main-content"');
    expect(markup).toContain('<main id="main-content" tabindex="-1">');
  });

  it("renders a truthful investigation workflow without marking future steps complete", async () => {
    const workflow = (await import("../../src/components/workflow")) as Record<string, unknown>;
    expect(workflow.WorkflowProgress).toBeDefined();
    if (typeof workflow.WorkflowProgress !== "function") return;

    const WorkflowProgress = workflow.WorkflowProgress as (properties: {
      currentStep: string;
      completedSteps: string[];
    }) => React.ReactNode;
    const markup = renderToStaticMarkup(
      <WorkflowProgress currentStep="capsule" completedSteps={["capture", "trace"]} />,
    );

    expect(markup).toContain("Capture");
    expect(markup).toContain("Trace");
    expect(markup).toContain("Capsule");
    expect(markup).toContain("Replay");
    expect(markup).toContain("What-if");
    expect(markup).toContain("Diff");
    expect(markup).toContain('aria-current="step"');
    expect(markup.match(/data-state="complete"/g)).toHaveLength(2);
    expect(markup.match(/data-state="upcoming"/g)).toHaveLength(3);
  });

  it("separates run status, isolation, oracle, and outcome evidence", async () => {
    const workflow = (await import("../../src/components/workflow")) as Record<string, unknown>;
    expect(workflow.RunEvidenceSummary).toBeDefined();
    if (typeof workflow.RunEvidenceSummary !== "function") return;

    const RunEvidenceSummary = workflow.RunEvidenceSummary as (properties: {
      run: typeof baselineRun;
    }) => React.ReactNode;
    const markup = renderToStaticMarkup(<RunEvidenceSummary run={baselineRun} />);

    expect(markup).toContain("Replay execution");
    expect(markup).toContain("COMPLETED");
    expect(markup).toContain("Isolation");
    expect(markup).toContain("PASS");
    expect(markup).toContain("Failure oracle");
    expect(markup).toContain("MATCHED");
    expect(markup).toContain("Baseline outcome");
    expect(markup).toContain("REPRODUCED");
  });

  it("keeps a machine-readable error code visible in blocked states", async () => {
    const system = (await import("../../src/components/system")) as Record<string, unknown>;
    expect(system.StatePanel).toBeDefined();
    if (typeof system.StatePanel !== "function") return;

    const StatePanel = system.StatePanel as (properties: {
      state: string;
      title: string;
      message: string;
      code: string;
    }) => React.ReactNode;
    const markup = renderToStaticMarkup(
      <StatePanel
        state="blocked"
        title="Replay blocked"
        message="The capsule integrity check did not pass."
        code="INTEGRITY_MISMATCH"
      />,
    );

    expect(markup).toContain('role="alert"');
    expect(markup).toContain("Replay blocked");
    expect(markup).toContain("INTEGRITY_MISMATCH");
  });
});
