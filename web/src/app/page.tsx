import { CommandCenterShell, StatePanel } from "../components/system";
import { WorkflowProgress } from "../components/workflow";

export default function HomePage() {
  return (
    <CommandCenterShell>
      <section className="hero" aria-labelledby="hero-title">
        <div className="hero__copy">
          <p className="eyebrow">Evidence-first incident replay</p>
          <h1 id="hero-title">Make distributed incidents replayable.</h1>
          <p className="hero__lede">
            Capture one distributed failure, reproduce it inside an isolated runtime,
            change one approved condition, and inspect the first meaningful divergence.
          </p>
        </div>
        <div className="hero__contract" aria-label="Frozen frontend contract">
          <span>Frontend foundation</span>
          <strong>Next.js 16.2.11</strong>
          <code>API v1.0</code>
        </div>
      </section>

      <section className="workflow-section" aria-labelledby="workflow-title">
        <div className="section-heading">
          <div>
            <p className="eyebrow">Golden investigation</p>
            <h2 id="workflow-title">Capture to Replay Diff</h2>
          </div>
          <p>Every completed step will be backed by a decoded Core API resource.</p>
        </div>
        <WorkflowProgress currentStep="capture" />
      </section>

      <section className="foundation-grid" aria-label="Command Center readiness">
        <StatePanel
          state="loading"
          title="Waiting for Core API"
          message="No incident, isolation, replay, or outcome state is displayed until a valid v1.0 resource is received."
        />

        <article className="evidence-card">
          <p className="eyebrow">Frozen scenario contract</p>
          <h2>Checkout duplicate effect</h2>
          <dl className="contract-facts">
            <div>
              <dt>Payment latency</dt>
              <dd>350 ms</dd>
            </div>
            <div>
              <dt>Checkout timeout</dt>
              <dd>200 ms</dd>
            </div>
            <div>
              <dt>Approved what-if</dt>
              <dd>50 ms</dd>
            </div>
          </dl>
          <p className="evidence-card__note">
            These are contract values, not a claim about current runtime health.
          </p>
        </article>
      </section>

      <section className="principles" aria-labelledby="principles-title">
        <div>
          <p className="eyebrow">Rendering discipline</p>
          <h2 id="principles-title">The interface presents evidence. The API owns truth.</h2>
        </div>
        <ul>
          <li>Closed enums and unknown fields are rejected.</li>
          <li>Blocked and failed runs never receive an outcome.</li>
          <li>Isolation, oracle, lifecycle, and outcome stay visibly separate.</li>
        </ul>
      </section>
    </CommandCenterShell>
  );
}
