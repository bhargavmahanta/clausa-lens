import type { ResetResult } from "../../lib/contracts";

export function ResetDialog({
  onCancel,
  onConfirm,
  status,
}: {
  onCancel: () => void;
  onConfirm: () => void;
  status: "confirming" | "submitting";
}) {
  const submitting = status === "submitting";
  return (
    <div className="reset-dialog-backdrop">
      <section
        aria-describedby="reset-dialog-description"
        aria-labelledby="reset-dialog-title"
        aria-modal="true"
        className="reset-dialog"
        role="dialog"
      >
        <p className="panel-kicker">Destructive demo action</p>
        <h2 id="reset-dialog-title">Confirm demo reset</h2>
        <p id="reset-dialog-description">
          Clear captured incidents, replay runs, replay-only ledger effects, and restore the 350 ms fault configuration?
        </p>
        <div className="reset-dialog__actions">
          <button disabled={submitting} onClick={onCancel} type="button">Cancel</button>
          <button disabled={submitting} onClick={onConfirm} type="button">
            {submitting ? "Resetting…" : "Reset demo"}
          </button>
        </div>
      </section>
    </div>
  );
}

export function ResetReceipt({ result }: { result: ResetResult }) {
  return (
    <section className="reset-receipt" aria-live="polite" aria-labelledby="reset-receipt-title">
      <h2 id="reset-receipt-title">Demo reset complete</h2>
      <code>{result.reset_id}</code>
      <p>{result.configured_latency_ms} ms payment latency restored · Deduplication disabled</p>
      <dl>
        <div><dt>Incidents cleared</dt><dd>{result.cleared_incident_count}</dd></div>
        <div><dt>Runs cleared</dt><dd>{result.cleared_run_count}</dd></div>
        <div><dt>Ledger effects cleared</dt><dd>{result.cleared_ledger_count}</dd></div>
      </dl>
    </section>
  );
}
