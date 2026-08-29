package replay

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/causalens/causalens/internal/capsule"
	"github.com/causalens/causalens/internal/contracts"
)

var (
	// ErrInvalidDigest reports that the capsule integrity digest does not match
	// its canonical content.
	ErrInvalidDigest = errors.New("replay: capsule integrity digest mismatch")
	// ErrPackValidation reports that the System Pack rejected the capsule.
	ErrPackValidation = errors.New("replay: system pack rejected capsule")
	// ErrUnsafeReplay reports that the capsule or its configuration names a
	// production/uncontrolled destination or a production credential profile.
	ErrUnsafeReplay = errors.New("replay: unsafe replay destination or credential")
	// ErrInvalidIntervention reports that the run's intervention is not allowed
	// by the capsule / System Pack, or a baseline run carries one.
	ErrInvalidIntervention = errors.New("replay: invalid intervention")
	// ErrReplayRunID reports that captured replay events omitted the run's
	// replay_run_id, so they cannot be attributed to the run.
	ErrReplayRunID = errors.New("replay: observed events missing replay_run_id")
	// ErrTeardown reports that the replay runtime failed to tear down, which by
	// CONTRACTS.md forces verdict FAIL.
	ErrTeardown = errors.New("replay: replay teardown failed")
	// ErrNoReplayEvents reports that a run produced no observed replay events and
	// therefore cannot reach COMPLETED (no source-event fallback).
	ErrNoReplayEvents = errors.New("replay: run captured no replay events")
)

// RunnerConfig is the replay-only configuration a Runner needs to execute one
// validated run. It is pack-agnostic: the runner sees only the plan and
// fixtures the capsule resolved, never scenario logic or core internals. The
// effective plan already has any approved intervention applied; the runner must
// not apply policy of its own.
type RunnerConfig struct {
	RunID     string
	Namespace string
	Plan      contracts.ReplayPlan
	Fixtures  contracts.FixtureSet
	LatencyMS int
}

// RunResult is what a Runner produces for one execution: the newly observed
// replay events (each carrying the run's replay_run_id), the isolation evidence
// derived from the runner's actual actions, and whether teardown failed.
type RunResult struct {
	Events    []contracts.ExecutionEvent
	Isolation contracts.IsolationEvidence
	Teardown  bool
}

// Runner executes one validated playback. Implementations must only reach
// replay-only services/state, must never contact production or uncontrolled
// destinations, and must capture events with the run's replay_run_id. The
// checked-out System Pack's demo services are a Runner; tests may substitute a
// deterministic fake. A nil Runner produces no events.
type Runner interface {
	Run(ctx context.Context, cfg RunnerConfig) (RunResult, error)
}

// ExecutionConfig carries everything the Execute seam needs for one run. The
// Plan is the effective plan for the run (the capsule plan with the approved
// intervention already applied for a what-if); LatencyMS is the effective
// payment latency (baseline fixture value, or the intervention's target).
type ExecutionConfig struct {
	Pack      contracts.SystemPack
	Capsule   contracts.ReplayCapsule
	Run       contracts.ReplayRun
	Plan      contracts.ReplayPlan
	Fixtures  contracts.FixtureSet
	Runner    Runner
	LatencyMS int
}

// ExecutionResult is the outcome of Execute: the observed replay rows plus the
// runtime's real isolation evidence.
type ExecutionResult struct {
	Events    []contracts.ExecutionEvent
	Isolation contracts.IsolationEvidence
}

// Execute runs one validated replay. It enforces the frozen safety boundary and
// validates the capsule, interventions, and captured events; it returns real
// observed replay events and the runner's isolation evidence. It does not
// persist and never falls back to source evidence: with no Runner, no events
// can be observed.
func Execute(ctx context.Context, cfg ExecutionConfig) (ExecutionResult, error) {
	if !capsule.VerifyDigest(cfg.Capsule) {
		return ExecutionResult{}, ErrInvalidDigest
	}
	if issues := cfg.Pack.ValidateCapsule(ctx, cfg.Capsule); len(issues) > 0 {
		return ExecutionResult{}, fmt.Errorf("%w: %v", ErrPackValidation, issues)
	}
	if err := validateReplaySafety(cfg.Capsule, cfg.Run, cfg.Plan, cfg.Fixtures); err != nil {
		return ExecutionResult{}, err
	}

	if cfg.Runner == nil {
		return ExecutionResult{}, fmt.Errorf("replay: no runner configured")
	}
	namespace := "replay-run-" + cfg.Run.RunID
	result, err := cfg.Runner.Run(ctx, RunnerConfig{
		RunID:     cfg.Run.RunID,
		Namespace: namespace,
		Plan:      cfg.Plan,
		Fixtures:  cfg.Fixtures,
		LatencyMS: cfg.LatencyMS,
	})
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("replay: execute: %w", err)
	}

	if result.Teardown {
		return ExecutionResult{}, fmt.Errorf("%w: run %s", ErrTeardown, cfg.Run.RunID)
	}
	if err := ValidateIsolation(result.Isolation); err != nil {
		return ExecutionResult{}, fmt.Errorf("%w: %v", ErrUnsafeReplay, err)
	}
	if len(result.Events) == 0 {
		return ExecutionResult{}, fmt.Errorf("%w: run %s", ErrNoReplayEvents, cfg.Run.RunID)
	}
	for _, event := range result.Events {
		if event.ReplayRunID != cfg.Run.RunID {
			return ExecutionResult{}, fmt.Errorf("%w: event %s has replay_run_id %q", ErrReplayRunID, event.EventID, event.ReplayRunID)
		}
	}
	return ExecutionResult{Events: result.Events, Isolation: result.Isolation}, nil
}

// validateReplaySafety enforces the default-deny replay boundary: the plan and
// fixtures must reference only allow-listed replay services, the capsule safety
// policy must be replay-only, and the intervention must match the run type and
// the capsule/System Pack's allowed set.
func validateReplaySafety(capsule contracts.ReplayCapsule, run contracts.ReplayRun, plan contracts.ReplayPlan, fixtures contracts.FixtureSet) error {
	if run.RunType == contracts.RunTypeBaseline && run.Intervention != nil {
		return fmt.Errorf("%w: baseline run records an intervention", ErrInvalidIntervention)
	}
	if run.RunType == contracts.RunTypeWhatIf && run.Intervention == nil {
		return fmt.Errorf("%w: what-if run requires exactly one intervention", ErrInvalidIntervention)
	}
	if run.Intervention != nil && !allowedIntervention(capsule, *run.Intervention) {
		return fmt.Errorf("%w: %v", ErrInvalidIntervention, *run.Intervention)
	}
	if run.BaselineRunID == "" && run.RunType == contracts.RunTypeWhatIf {
		return fmt.Errorf("%w: what-if run needs a baseline_run_id", ErrInvalidIntervention)
	}

	if capsule.Safety.CredentialProfile != contracts.CredentialReplayOnly {
		return fmt.Errorf("%w: credential profile %q is not replay-only", ErrUnsafeReplay, capsule.Safety.CredentialProfile)
	}
	if !replayOnlyPlan(plan, fixtures) {
		return fmt.Errorf("%w: replay plan references a non-replay-only component or destination", ErrUnsafeReplay)
	}
	return nil
}

func allowedIntervention(capsule contracts.ReplayCapsule, intervention contracts.Intervention) bool {
	for _, spec := range capsule.AllowedInterventions {
		if spec.Type != intervention.Type || spec.Unit != intervention.Unit || intervention.From < spec.Minimum || intervention.To > spec.Maximum || intervention.From == intervention.To {
			return false
		}
		return true
	}
	return false
}

func replayOnlyPlan(plan contracts.ReplayPlan, fixtures contracts.FixtureSet) bool {
	allowed := map[string]bool{
		"gateway": true, "checkout": true, "payment": true, "ledger": true,
		"payment_simulator": true, "payment-simulator": true, "replay-postgres": true,
	}
	for _, component := range plan.RequiredComponents {
		if !allowed[component] {
			return false
		}
	}
	for _, fixture := range fixtures.DependencyFixtures {
		if fixture.Dependency != contracts.DependencyPaymentSimulator {
			return false
		}
	}
	return true
}

// OutcomeFor derives the frozen completed outcome for a run type and oracle
// match, exposing internal/replay's deriveOutcome to the worker so a completed
// run is always evaluated by the pack's oracle, never hardcoded.
func OutcomeFor(runType contracts.RunType, oracleMatched bool) contracts.ReplayOutcome {
	return deriveOutcome(runType, oracleMatched)
}

// effectiveLatency is removed; the worker supplies the effective latency via
// ExecutionConfig.LatencyMS.

// ReclaimOrphans is a helper the worker uses to fail runs that a previous
// worker left mid-lifecycle (VALIDATING/RUNNING) so they are never re-executed
// into duplicate side effects. It returns the affected run ids.
func ReclaimOrphans(ctx context.Context, store RunStore, run contracts.ReplayRun, lease time.Duration) (bool, error) {
	if run.Status != contracts.ReplayRunValidating && run.Status != contracts.ReplayRunRunning {
		return false, nil
	}
	if lease > 0 && run.StartedAt != "" {
		started, err := time.Parse(time.RFC3339Nano, run.StartedAt)
		if err == nil && time.Since(started) < lease {
			return false, nil
		}
	}
	failed, err := FinalizeOrphan(ctx, store, run)
	if err != nil {
		return false, err
	}
	return failed.Status == contracts.ReplayRunFailed, nil
}

// FinalizeOrphan marks a stranded run FAILED without an outcome, per
// CONTRACTS.md (FAILED requires error; no outcome).
func FinalizeOrphan(ctx context.Context, store RunStore, run contracts.ReplayRun) (contracts.ReplayRun, error) {
	failed := run
	failed.Status = contracts.ReplayRunFailed
	failed.Outcome = ""
	failed.Error = &contracts.RunError{
		Code:      contracts.InternalFailure,
		Message:   "replay worker: previous run was orphaned mid-lifecycle; not re-executed",
		Retryable: false,
		Details:   map[string]any{"run_id": run.RunID},
	}
	failed.CompletedAt = requireCompletedAt(run.CompletedAt)
	if err := failed.Validate(); err != nil {
		return run, fmt.Errorf("replay: orphan finalize invalid: %w", err)
	}
	if isTerminal(run.Status) {
		return failed, nil
	}
	if err := store.TransitionRun(ctx, run.Status, failed); err != nil {
		return run, err
	}
	return failed, nil
}
