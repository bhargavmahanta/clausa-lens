export type StatePanelState =
  | "loading"
  | "empty"
  | "error"
  | "blocked"
  | "failed"
  | "completed"
  | "unsupported";

export type StatePanelProps = {
  state: StatePanelState;
  title: string;
  message: string;
  code?: string;
  action?: React.ReactNode;
};

const alertStates = new Set<StatePanelState>(["error", "blocked", "failed", "unsupported"]);

export function StatePanel({ state, title, message, code, action }: StatePanelProps) {
  return (
    <section
      className="state-panel"
      data-state={state}
      role={alertStates.has(state) ? "alert" : "status"}
      aria-live={state === "loading" ? "polite" : undefined}
    >
      <span className="state-panel__marker" aria-hidden="true" />
      <div>
        <p className="state-panel__eyebrow">{state}</p>
        <h2>{title}</h2>
        <p>{message}</p>
        {code ? <code>{code}</code> : null}
        {action ? <div className="state-panel__action">{action}</div> : null}
      </div>
    </section>
  );
}
