import type { ReplayDiff } from "../../lib/contracts";
import { buildDiffView } from "./diff-view";

function signed(value: number) {
  if (value === 0) return "0";
  return value < 0 ? `−${Math.abs(value)}` : `+${value}`;
}

export function ReplayDiffPanel({ diff }: { diff: ReplayDiff }) {
  const view = buildDiffView(diff);

  return (
    <section className="replay-diff" aria-labelledby="replay-diff-title">
      <header className="workflow-card__header">
        <div>
          <p className="panel-kicker">Evidence comparison</p>
          <h2 id="replay-diff-title">Replay Diff</h2>
        </div>
        <code>{view.diffId}</code>
      </header>

      <div className="diff-effect-grid">
        <article>
          <span>Baseline</span>
          <strong>{view.effects.baseline.paymentAttemptCount} attempts</strong>
          <small>{view.effects.baseline.ledgerCommitCount} ledger commits</small>
        </article>
        <article aria-label="Effect delta">
          <span>Comparison − baseline</span>
          <strong>{signed(view.effects.delta.paymentAttemptCount)} attempts</strong>
          <small>{signed(view.effects.delta.ledgerCommitCount)} ledger commits</small>
        </article>
        <article>
          <span>What-if</span>
          <strong>{view.effects.comparison.paymentAttemptCount} attempt</strong>
          <small>{view.effects.comparison.ledgerCommitCount} ledger commit</small>
        </article>
      </div>

      <div className="oracle-comparison" aria-label="Failure oracle comparison">
        <article>
          <span>Baseline oracle</span>
          <strong>{view.oracles.baselineMatched ? "MATCHED" : "NOT MATCHED"}</strong>
          <p>{view.oracles.baselineExplanation}</p>
        </article>
        <article>
          <span>What-if oracle</span>
          <strong>{view.oracles.comparisonMatched ? "MATCHED" : "NOT MATCHED"}</strong>
          <p>{view.oracles.comparisonExplanation}</p>
        </article>
      </div>

      <section className="first-divergence" aria-labelledby="first-divergence-title">
        <h3 id="first-divergence-title">First meaningful divergence</h3>
        {view.firstDivergence ? (
          <div>
            <code>{view.firstDivergence.rule}</code>
            <dl>
              <div>
                <dt>Baseline #{view.firstDivergence.baselineTimelineIndex}</dt>
                <dd>{view.firstDivergence.baselineEventId ?? "No event reference"}</dd>
                <dd>{String(view.firstDivergence.baselineValue)}</dd>
              </div>
              <div>
                <dt>What-if #{view.firstDivergence.comparisonTimelineIndex}</dt>
                <dd>{view.firstDivergence.comparisonEventId ?? "No event reference"}</dd>
                <dd>{String(view.firstDivergence.comparisonValue)}</dd>
              </div>
            </dl>
          </div>
        ) : (
          <p>No first meaningful divergence was provided by the API.</p>
        )}
      </section>

      <p className="evidence-note">{view.evidenceSummary}</p>
      {view.limitations.length > 0 ? (
        <div className="diff-limitations">
          <h3>Limitations</h3>
          <ul>{view.limitations.map((item) => <li key={item}>{item}</li>)}</ul>
        </div>
      ) : null}
    </section>
  );
}
