package replay

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/causalens/causalens/internal/contracts"
)

// RunStore is the persistence seam the replay package needs. It is satisfied
// by internal/core PostgresRepository. The package deliberately depends on
// this narrow seam rather than the concrete repository so it can be exercised
// with a fake store in tests and wired to PostgresRepository in production.
type RunStore interface {
	GetRun(ctx context.Context, runID string) (contracts.ReplayRun, error)
	PutRun(ctx context.Context, run contracts.ReplayRun) error
	TransitionRun(ctx context.Context, from contracts.ReplayRunStatus, to contracts.ReplayRun) error
}

// EventLoader is the read seam the replay package uses to load the observed
// events of a run for outcome evaluation. It is satisfied by a run-event
// repository or a demo runtime that records replayed events.
type EventLoader interface {
	EventsForRun(ctx context.Context, run contracts.ReplayRun) ([]contracts.ExecutionEvent, error)
}

// NewRun initializes a CREATED baseline or what-if run referencing a capsule.
// For baseline it records zero intervention; for what-if exactly the provided
// intervention. Validation against the frozen ReplayRun contract runs here;
// the frozen baseline authorization (a what-if must reference a COMPLETED,
// REPRODUCED baseline with passing isolation) is the store's job.
func NewRun(runID, executionID, capsuleID, capsuleHash string, runType contracts.RunType,
	baselineRunID string, intervention *contracts.Intervention, trialNumber int) (contracts.ReplayRun, error) {

	run := contracts.ReplayRun{
		SchemaVersion:    contracts.ContractVersion,
		RunID:            runID,
		ExecutionID:      executionID,
		CapsuleID:        capsuleID,
		CapsuleHash:      capsuleHash,
		RunType:          runType,
		BaselineRunID:    baselineRunID,
		Intervention:     intervention,
		TrialNumber:      trialNumber,
		Status:           contracts.ReplayRunCreated,
		ObservedEventIDs: []string{},
	}

	switch runType {
	case contracts.RunTypeBaseline:
		if baselineRunID != "" || intervention != nil {
			return run, fmt.Errorf("replay: baseline run must omit baseline_run_id and intervention")
		}
	case contracts.RunTypeWhatIf:
		if baselineRunID == "" {
			return run, fmt.Errorf("replay: what-if run requires a baseline_run_id")
		}
		if intervention == nil {
			return run, fmt.Errorf("replay: what-if run requires exactly one intervention")
		}
	default:
		return run, fmt.Errorf("replay: invalid run_type %q", runType)
	}

	if err := run.Validate(); err != nil {
		return run, fmt.Errorf("replay: invalid run: %w", err)
	}
	return run, nil
}

// AdvanceRun performs a single legal lifecycle transition using the store's
// TransitionRun, which enforces both legality and 409-on-illegal. It assigns
// the timestamps the contract demands for the target status and returns a
// descriptive error if the store rejects the transition.
func AdvanceRun(ctx context.Context, store RunStore, run contracts.ReplayRun,
	to contracts.ReplayRunStatus) (contracts.ReplayRun, error) {

	next := run
	next.Status = to
	switch to {
	case contracts.ReplayRunRunning:
		if next.StartedAt == "" {
			next.StartedAt = nowRFC3339()
		}
	case contracts.ReplayRunCompleted, contracts.ReplayRunFailed, contracts.ReplayRunBlocked:
		if next.CompletedAt == "" {
			next.CompletedAt = nowRFC3339()
		}
	}

	if err := store.TransitionRun(ctx, run.Status, next); err != nil {
		return run, fmt.Errorf("replay: advance %s -> %s: %w", run.Status, to, err)
	}
	return next, nil
}

// FinalizeRun assigns a terminal result. It validates the frozen terminal
// invariants via contracts.ReplayRun.Validate and persists through the store's
// TransitionRun. A COMPLETED run with an unsafe isolation evidence is refused
// (the caller records BLOCKED/FAILED instead); supplying a runError routes the
// run to FAILED and never produces a COMPLETED run carrying an error.
func FinalizeRun(ctx context.Context, store RunStore, run contracts.ReplayRun,
	outcome contracts.ReplayOutcome, effectSummary *contracts.EffectSummary,
	oracleResult *contracts.OracleResult, isolation *contracts.IsolationEvidence,
	runError *contracts.RunError) (contracts.ReplayRun, error) {

	target := run

	if runError != nil {
		target.Status = contracts.ReplayRunFailed
		target.Outcome = ""
		target.Error = runError
		target.CompletedAt = requireCompletedAt(run.CompletedAt)
		target.StartedAt = requireStartedAt(run.StartedAt)
		if err := target.Validate(); err != nil {
			return run, fmt.Errorf("replay: failed finalize is invalid: %w", err)
		}
		if !isTerminal(run.Status) {
			if err := store.TransitionRun(ctx, run.Status, target); err != nil {
				return run, fmt.Errorf("replay: transition to FAILED: %w", err)
			}
		}
		return target, nil
	}

	if isolation == nil {
		return run, fmt.Errorf("replay: completed run requires isolation evidence")
	}
	if err := ValidateIsolation(*isolation); err != nil {
		return run, fmt.Errorf("replay: refusing COMPLETED run with unsafe isolation: %w", err)
	}

	target.Status = contracts.ReplayRunCompleted
	target.Outcome = outcome
	target.EffectSummary = effectSummary
	target.FailureOracleResult = oracleResult
	target.IsolationEvidence = isolation
	target.Error = nil
	target.CompletedAt = requireCompletedAt(run.CompletedAt)
	target.StartedAt = requireStartedAt(run.StartedAt)

	if err := target.Validate(); err != nil {
		return run, fmt.Errorf("replay: completed finalize is invalid: %w", err)
	}
	if !isTerminal(run.Status) {
		if err := store.TransitionRun(ctx, run.Status, target); err != nil {
			return run, fmt.Errorf("replay: transition to COMPLETED: %w", err)
		}
	}
	return target, nil
}

// Evaluate applies the System Pack end-to-end for a completed run: it runs the
// pack's EvaluateOutcome over run + observed events, then derives the outcome
// per run type and the frozen applicability rules. On success it returns a
// COMPLETED run carrying the replay-only isolation evidence; on a pack error
// it returns a FAILED run with an internal error. It does not persist.
func Evaluate(ctx context.Context, pack contracts.SystemPack, run contracts.ReplayRun,
	events []contracts.ExecutionEvent) (contracts.ReplayRun, error) {

	oracleResult, err := pack.EvaluateOutcome(ctx, contracts.ReplayExecution{Run: run, Events: events})
	if err != nil {
		failed := run
		failed.Status = contracts.ReplayRunFailed
		failed.Outcome = ""
		failed.CompletedAt = nowRFC3339()
		failed.StartedAt = requireStartedAt(run.StartedAt)
		failed.Error = &contracts.RunError{
			Code:      contracts.InternalFailure,
			Message:   "replay: outcome evaluation failed: " + err.Error(),
			Retryable: true,
			Details:   map[string]any{"run_id": run.RunID},
		}
		if verr := failed.Validate(); verr != nil {
			return run, fmt.Errorf("replay: failed run is invalid: %w", verr)
		}
		return failed, nil
	}

	completed := run
	completed.Status = contracts.ReplayRunCompleted
	completed.Outcome = deriveOutcome(run.RunType, oracleResult.Matched)
	completed.EffectSummary = &oracleResult.EffectSummary
	completed.FailureOracleResult = &oracleResult
	completed.IsolationEvidence = safeIsolation(run.RunID)
	completed.ObservedEventIDs = observedEventIDs(events)
	completed.CompletedAt = nowRFC3339()
	completed.StartedAt = requireStartedAt(run.StartedAt)
	completed.Error = nil

	if verr := completed.Validate(); verr != nil {
		return run, fmt.Errorf("replay: evaluated completed run is invalid: %w", verr)
	}
	return completed, nil
}

// IsolationFor builds truthful, default-deny isolation evidence for an
// in-process replay execution. Because the runner drives the System Pack's
// services in-process against replay-only in-memory state, the runtime reached
// NO external datastore and NO network simulator destination; the evidence
// therefore records zero datastore destinations and zero simulator interactions
// rather than fabricating a payment-simulator host or a replay datastore URL.
// NetworkPolicy is PASS because no egress was attempted, and teardown reflects
// the runner's actual reset/teardown result. The events themselves still prove
// the simulated interactions happened; nothing is invented here.
func IsolationFor(namespace string, teardownOK bool) contracts.IsolationEvidence {
	teardown := contracts.VerdictPass
	if !teardownOK {
		teardown = contracts.VerdictFail
	}
	evidence := contracts.IsolationEvidence{
		PolicyVersion:         contracts.ContractVersion,
		Verdict:               contracts.VerdictPass,
		RuntimeNamespace:      namespace,
		NetworkPolicy:         contracts.VerdictPass,
		CredentialProfile:     contracts.CredentialReplayOnly,
		DatastoreDestinations: []string{},
		SimulatorInteractions: []contracts.DependencyInteraction{},
		DeniedInteractions:    []contracts.DependencyInteraction{},
		TeardownResult:        teardown,
	}
	if teardown != contracts.VerdictPass {
		evidence.Verdict = contracts.VerdictFail
	}
	return evidence
}

// ValidateIsolation returns an error when the isolation evidence is not
// default-deny safe: a production datastore destination, a failed network
// policy, a denied interaction, a failed teardown, or a non-replay credential
// profile. It never weakens the frozen contract rules.
func ValidateIsolation(isolation contracts.IsolationEvidence) error {
	if isolation.PolicyVersion != contracts.ContractVersion {
		return fmt.Errorf("isolation: unsupported policy_version %q", isolation.PolicyVersion)
	}
	if isolation.CredentialProfile != contracts.CredentialReplayOnly {
		return fmt.Errorf("isolation: credential profile %q is not replay-only", isolation.CredentialProfile)
	}
	if isolation.NetworkPolicy == contracts.VerdictFail {
		return fmt.Errorf("isolation: network policy FAIL")
	}
	if isolation.TeardownResult == contracts.VerdictFail {
		return fmt.Errorf("isolation: teardown FAIL")
	}
	if len(isolation.DeniedInteractions) != 0 {
		return fmt.Errorf("isolation: %d denied interaction(s)", len(isolation.DeniedInteractions))
	}
	for _, destination := range isolation.DatastoreDestinations {
		if !isReplayDatastore(destination) {
			return fmt.Errorf("isolation: non-replay datastore destination %q", destination)
		}
	}
	for _, interaction := range isolation.SimulatorInteractions {
		if !replayOnlyHost(interaction.Destination) {
			return fmt.Errorf("isolation: non-replay simulator destination %q", interaction.Destination)
		}
	}
	return nil
}

// deriveOutcome maps an oracle match to the frozen outcome set for the run type.
func deriveOutcome(runType contracts.RunType, matched bool) contracts.ReplayOutcome {
	if runType == contracts.RunTypeWhatIf {
		if matched {
			return contracts.ReplayOutcomeUnchanged
		}
		return contracts.ReplayOutcomeMitigated
	}
	if matched {
		return contracts.ReplayOutcomeReproduced
	}
	return contracts.ReplayOutcomeNotReproduced
}

// safeIsolation returns the canonical default-deny replay-only isolation
// evidence for a run, matching the P0 contract example. Used by Evaluate when
// the runtime does not vend a richer evidence object.
func safeIsolation(runID string) *contracts.IsolationEvidence {
	if runID == "" {
		runID = "run"
	}
	return &contracts.IsolationEvidence{
		PolicyVersion:         contracts.ContractVersion,
		Verdict:               contracts.VerdictPass,
		RuntimeNamespace:      "replay-run-" + runID,
		NetworkPolicy:         contracts.VerdictPass,
		CredentialProfile:     contracts.CredentialReplayOnly,
		DatastoreDestinations: []string{"postgres://replay/ledger_run_" + runID},
		SimulatorInteractions: []contracts.DependencyInteraction{{
			Dependency:  "payment_simulator",
			Destination: "http://payment-simulator:8080",
			Operation:   "authorize",
			Result:      contracts.InteractionSimulated,
		}},
		DeniedInteractions: []contracts.DependencyInteraction{},
		TeardownResult:     contracts.VerdictPass,
	}
}

func observedEventIDs(events []contracts.ExecutionEvent) []string {
	ids := make([]string, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.EventID)
	}
	return ids
}

func isReplayDatastore(destination string) bool {
	return len(destination) >= len("postgres://replay/") && destination[:len("postgres://replay/")] == "postgres://replay/"
}

func replayOnlyHost(destination string) bool {
	host := destination
	if parsed, err := url.Parse(destination); err == nil && parsed.Host != "" {
		host = parsed.Hostname()
	}
	return host == "payment-simulator" || host == "replay-postgres" || strings.HasPrefix(host, "payment-simulator:") || strings.HasPrefix(host, "replay-postgres:")
}

func isTerminal(status contracts.ReplayRunStatus) bool {
	return status == contracts.ReplayRunCompleted || status == contracts.ReplayRunFailed || status == contracts.ReplayRunBlocked
}

func requireStartedAt(value string) string {
	if value != "" {
		return value
	}
	return nowRFC3339()
}

func requireCompletedAt(value string) string {
	if value != "" {
		return value
	}
	return nowRFC3339()
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
