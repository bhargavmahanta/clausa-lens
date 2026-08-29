const workflowSteps = [
  { id: "capture", label: "Capture" },
  { id: "trace", label: "Trace" },
  { id: "capsule", label: "Capsule" },
  { id: "replay", label: "Replay" },
  { id: "what-if", label: "What-if" },
  { id: "diff", label: "Diff" },
] as const;

export type WorkflowStep = (typeof workflowSteps)[number]["id"];

export type WorkflowProgressProps = {
  currentStep: WorkflowStep;
  completedSteps?: readonly WorkflowStep[];
};

export function WorkflowProgress({
  currentStep,
  completedSteps = [],
}: WorkflowProgressProps) {
  const completed = new Set(completedSteps);

  return (
    <nav className="workflow-progress" aria-label="Investigation progress">
      <ol>
        {workflowSteps.map((step, index) => {
          const state = completed.has(step.id)
            ? "complete"
            : step.id === currentStep
              ? "current"
              : "upcoming";

          return (
            <li key={step.id} data-state={state}>
              <span className="workflow-progress__index" aria-hidden="true">
                {String(index + 1).padStart(2, "0")}
              </span>
              <span className="workflow-progress__label" aria-current={state === "current" ? "step" : undefined}>
                {step.label}
              </span>
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
