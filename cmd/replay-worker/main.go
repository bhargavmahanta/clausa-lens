// Command replay-worker drives ReplayRuns to terminal states by executing each
// validated capsule against replay-only services and recording the newly
// observed replay events. It selects the System Pack from PACK_IMPL using the
// same pack-agnostic registry as the Core API, so no scenario logic lives here.
//
// Worker model (smallest durable P0): a single worker polling loop claims one
// run at a time by atomically advancing CREATED -> VALIDATING -> RUNNING using
// the store's lifecycle transition (which is the mutual-exclusion primitive:
// only one worker can win the CREATED -> VALIDATING advance). Replaying never
// re-runs an already-claimed run. On startup it reclaims runs stranded in
// VALIDATING/RUNNING by a previous worker (marking them FAILED) so a crashed
// worker cannot cause duplicate side effects. There is no queue, no Kafka, no
// manager UI, and no hidden manual database edits.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/causalens/causalens/internal/capsule"
	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/core"
	"github.com/causalens/causalens/internal/packregistry"
	"github.com/causalens/causalens/internal/replay"
)

// workerStore is the persistence seam the worker needs. It is satisfied by both
// core.Store and core.PostgresRepository.
type workerStore interface {
	GetRun(context.Context, string) (contracts.ReplayRun, error)
	PutRun(context.Context, contracts.ReplayRun) error
	TransitionRun(context.Context, contracts.ReplayRunStatus, contracts.ReplayRun) error
	EventsForRun(context.Context, contracts.ReplayRun) ([]contracts.ExecutionEvent, error)
	GetCapsule(context.Context, string) (contracts.ReplayCapsule, error)
	IngestEvent(context.Context, contracts.ExecutionEvent) error
	ListRuns(context.Context, contracts.ReplayRunStatus) ([]contracts.ReplayRun, error)
	ClaimRun(context.Context, string, time.Duration) (contracts.ReplayRun, error)
	RenewLease(context.Context, string, time.Duration) error
	ReclaimExpiredRuns(context.Context, time.Duration) ([]contracts.ReplayRun, error)
}

// runStore wraps workerStore in the replay.RunStore/EventLoader seams the
// internal/replay helpers consume, so the worker never reimplements lifecycle
// transitions.
type runStore struct{ workerStore }

func (s runStore) GetRun(ctx context.Context, id string) (contracts.ReplayRun, error) {
	return s.workerStore.GetRun(ctx, id)
}
func (s runStore) PutRun(ctx context.Context, r contracts.ReplayRun) error {
	return s.workerStore.PutRun(ctx, r)
}
func (s runStore) TransitionRun(ctx context.Context, from contracts.ReplayRunStatus, r contracts.ReplayRun) error {
	return s.workerStore.TransitionRun(ctx, from, r)
}
func (s runStore) EventsForRun(ctx context.Context, r contracts.ReplayRun) ([]contracts.ExecutionEvent, error) {
	return s.workerStore.EventsForRun(ctx, r)
}

// processRun advances one run from its current status to a terminal state and
// persists the result. It claims a run at CREATED atomically (locking it against
// a concurrent worker), executes its capsule through replay.Execute, records the
// newly observed replay events, derives the outcome from the pack's oracle, and
// finalizes with real isolation evidence. It refuses to run without a pack or a
// runner, skips already-terminal runs, and never falls back to source evidence.
func processRun(ctx context.Context, store workerStore, pack contracts.SystemPack, runner replay.Runner, runID string, lease time.Duration) (contracts.ReplayRun, error) {
	if pack == nil {
		return contracts.ReplayRun{}, fmt.Errorf("replay worker: no System Pack wired (set PACK_IMPL)")
	}
	if runner == nil {
		return contracts.ReplayRun{}, fmt.Errorf("replay worker: no runner configured")
	}
	run, err := store.GetRun(ctx, runID)
	if err != nil {
		return contracts.ReplayRun{}, fmt.Errorf("replay worker: get run: %w", err)
	}
	if isTerminal(run.Status) {
		return run, nil
	}

	if run.Status == contracts.ReplayRunCreated {
		claimed, err := store.ClaimRun(ctx, runID, lease)
		if err != nil {
			// If another worker already claimed it, skip (never re-run).
			if current, getErr := store.GetRun(ctx, runID); getErr == nil && current.Status != contracts.ReplayRunCreated {
				return current, nil
			}
			return run, err
		}
		run = claimed
	}
	if run.Status == contracts.ReplayRunValidating {
		if run.RunType == contracts.RunTypeWhatIf {
			if err := authorizeBaseline(ctx, store, run); err != nil {
				return finalizeBlocked(ctx, store, run, contracts.InterventionInvalid, err.Error())
			}
		}
		run, err = replay.AdvanceRun(ctx, runStore{store}, run, contracts.ReplayRunRunning)
		if err != nil {
			return run, err
		}
		// Runtime has started: renew the lease so a running execution is not
		// mistaken for an orphan by a surviving worker.
		_ = store.RenewLease(ctx, run.RunID, lease)
	}
	if run.Status != contracts.ReplayRunRunning {
		return run, fmt.Errorf("replay worker: run %q is in unexpected status %s", run.RunID, run.Status)
	}

	cap, err := store.GetCapsule(ctx, run.CapsuleID)
	if err != nil {
		return finalizeFailed(ctx, store, run, contracts.FixtureMissing, fmt.Sprintf("replay worker: capsule %s not found", run.CapsuleID))
	}
	if !capsule.VerifyDigest(cap) {
		return finalizeBlocked(ctx, store, run, contracts.IntegrityMismatch, "replay worker: capsule integrity digest mismatch")
	}

	plan := cap.ReplayPlan
	latency := fixtureLatency(cap)
	if run.RunType == contracts.RunTypeWhatIf && run.Intervention != nil {
		applied, err := pack.ApplyIntervention(ctx, plan, *run.Intervention)
		if err != nil {
			return finalizeBlocked(ctx, store, run, contracts.InterventionInvalid, "replay worker: invalid intervention: "+err.Error())
		}
		plan = applied
		latency = run.Intervention.To
	}

	result, err := replay.Execute(ctx, replay.ExecutionConfig{
		Pack:      pack,
		Capsule:   cap,
		Run:       run,
		Plan:      plan,
		Fixtures:  contracts.FixtureSet{StateFixtures: cap.StateFixtures, DependencyFixtures: cap.DependencyFixtures},
		Runner:    runner,
		LatencyMS: latency,
	})
	if err != nil {
		return finalizeExecuteError(ctx, store, run, err)
	}

	for _, event := range result.Events {
		if err := store.IngestEvent(ctx, event); err != nil {
			return finalizeFailed(ctx, store, run, contracts.InternalFailure, "replay worker: persist replay event: "+err.Error())
		}
	}

	oracle, err := pack.EvaluateOutcome(ctx, contracts.ReplayExecution{Run: run, Events: result.Events})
	if err != nil {
		return finalizeFailed(ctx, store, run, contracts.OracleUnavailable, "replay worker: outcome evaluation failed: "+err.Error())
	}
	summary := oracle.EffectSummary
	completed, err := replay.FinalizeRun(ctx, runStore{store}, run, replay.OutcomeFor(run.RunType, oracle.Matched), &summary, &oracle, &result.Isolation, nil)
	if err != nil {
		return run, fmt.Errorf("replay worker: finalize run: %w", err)
	}
	return completed, nil
}

// authorizeBaseline enforces the frozen baseline gate: a what-if run must
// reference a baseline with status COMPLETED, outcome REPRODUCED, and passing
// isolation evidence, with a matching capsule hash/provenance.
func authorizeBaseline(ctx context.Context, store workerStore, run contracts.ReplayRun) error {
	baseline, err := store.GetRun(ctx, run.BaselineRunID)
	if err != nil {
		return fmt.Errorf("baseline %s not found", run.BaselineRunID)
	}
	passed := baseline.RunType == contracts.RunTypeBaseline &&
		baseline.Status == contracts.ReplayRunCompleted &&
		baseline.Outcome == contracts.ReplayOutcomeReproduced &&
		baseline.IsolationEvidence != nil &&
		baseline.IsolationEvidence.Verdict == contracts.VerdictPass &&
		baseline.CapsuleHash == run.CapsuleHash
	if !passed {
		return fmt.Errorf("baseline %s is not an eligible COMPLETED/REPRODUCED baseline with passing isolation", run.BaselineRunID)
	}
	return nil
}

// finalizeExecuteError maps a replay.Execute error to a terminal run per the
// frozen error codes: contract/safety invalidation -> BLOCKED; runtime/fixture
// or absence of observed events -> FAILED.
func finalizeExecuteError(ctx context.Context, store workerStore, run contracts.ReplayRun, err error) (contracts.ReplayRun, error) {
	switch {
	case errors.Is(err, replay.ErrInvalidDigest):
		return finalizeBlocked(ctx, store, run, contracts.IntegrityMismatch, err.Error())
	case errors.Is(err, replay.ErrPackValidation):
		return finalizeBlocked(ctx, store, run, contracts.SanitizationFailed, err.Error())
	case errors.Is(err, replay.ErrUnsafeReplay):
		return finalizeBlocked(ctx, store, run, contracts.DestinationBlocked, err.Error())
	case errors.Is(err, replay.ErrInvalidIntervention):
		return finalizeBlocked(ctx, store, run, contracts.InterventionInvalid, err.Error())
	case errors.Is(err, replay.ErrTeardown):
		return finalizeBlocked(ctx, store, run, contracts.IsolationViolation, err.Error())
	case errors.Is(err, replay.ErrReplayRunID):
		return finalizeBlocked(ctx, store, run, contracts.IntegrityMismatch, err.Error())
	default:
		return finalizeFailed(ctx, store, run, contracts.InternalFailure, err.Error())
	}
}

func finalizeBlocked(ctx context.Context, store workerStore, run contracts.ReplayRun, code contracts.ErrorCode, message string) (contracts.ReplayRun, error) {
	return finalizeTerminal(ctx, store, run, contracts.ReplayRunBlocked, code, message)
}

func finalizeFailed(ctx context.Context, store workerStore, run contracts.ReplayRun, code contracts.ErrorCode, message string) (contracts.ReplayRun, error) {
	return finalizeTerminal(ctx, store, run, contracts.ReplayRunFailed, code, message)
}

func finalizeTerminal(ctx context.Context, store workerStore, run contracts.ReplayRun, status contracts.ReplayRunStatus, code contracts.ErrorCode, message string) (contracts.ReplayRun, error) {
	target := run
	target.Status = status
	target.Outcome = ""
	target.Error = &contracts.RunError{Code: code, Message: message, Retryable: false, Details: map[string]any{"run_id": run.RunID}}
	target.CompletedAt = nowRFC3339()
	if err := target.Validate(); err != nil {
		return run, fmt.Errorf("replay worker: %s finalize invalid: %w", status, err)
	}
	if isTerminal(run.Status) {
		return target, nil
	}
	if err := store.TransitionRun(ctx, run.Status, target); err != nil {
		return target, err
	}
	return target, nil
}

// runPoll cycles until ctx is cancelled: reclaim expired/stranded runs, then
// claim and process one CREATED run per iteration.
func runPoll(ctx context.Context, store workerStore, pack contracts.SystemPack, runner replay.Runner, lease time.Duration, sleep time.Duration) {
	for {
		if ctx.Err() != nil {
			return
		}
		reclaim, err := reclaimOrphans(ctx, store, lease)
		if err != nil {
			log.Printf("replay worker: reclaim: %v", err)
		}
		_ = reclaim

		runs, err := store.ListRuns(ctx, contracts.ReplayRunCreated)
		if err != nil {
			log.Printf("replay worker: list created: %v", err)
		}
		claimed := false
		for _, run := range runs {
			outcome, err := processRun(ctx, store, pack, runner, run.RunID, lease)
			if err != nil {
				log.Printf("replay worker: run %s: %v", run.RunID, err)
				continue
			}
			if outcome.Status != contracts.ReplayRunCreated {
				log.Printf("replay worker: run %s -> %s (outcome %s)", outcome.RunID, outcome.Status, outcome.Outcome)
			}
			claimed = true
			break
		}
		if !claimed && sleep > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(sleep):
			}
		}
	}
}

// reclaimOrphans fails runs stranded in VALIDATING/RUNNING whose lease expired
// (or that never had one), so a crashed worker's run is recovered as FAILED and
// never re-executed into duplicate side effects. Recovery is fail-only.
func reclaimOrphans(ctx context.Context, store workerStore, lease time.Duration) ([]string, error) {
	expired, err := store.ReclaimExpiredRuns(ctx, lease)
	if err != nil {
		return nil, err
	}
	var stranded []string
	for _, run := range expired {
		if _, err := replay.FinalizeOrphan(ctx, runStore{store}, run); err != nil {
			continue
		}
		stranded = append(stranded, run.RunID)
	}
	return stranded, nil
}

func isTerminal(status contracts.ReplayRunStatus) bool {
	return status == contracts.ReplayRunCompleted || status == contracts.ReplayRunFailed || status == contracts.ReplayRunBlocked
}

func fixtureLatency(capsule contracts.ReplayCapsule) int {
	if len(capsule.DependencyFixtures) > 0 {
		return capsule.DependencyFixtures[0].LatencyMS
	}
	return 0
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func main() {
	runID := flag.String("run-id", "", "id of a single run to drive to a terminal state (optional; omit for polling mode)")
	interval := flag.Duration("interval", 2*time.Second, "poll interval in polling mode")
	lease := flag.Duration("lease", leaseDefault(), "orphan lease time for reclaiming stranded runs")
	flag.Parse()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	db, err := core.OpenPostgres(databaseURL)
	if err != nil {
		log.Fatal("failed to initialize database")
	}
	defer db.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatal("database is unavailable")
	}

	store := core.NewPostgresRepository(db)
	pack := packregistry.Resolve(os.Getenv("PACK_IMPL"))
	runner := newDemoRunner()

	if *runID != "" {
		result, err := processRun(ctx, store, pack, runner, *runID, *lease)
		if err != nil {
			log.Fatalf("replay worker: %v", err)
		}
		log.Printf("replay worker: run %s -> %s (outcome %s)", result.RunID, result.Status, result.Outcome)
		return
	}
	runPoll(ctx, store, pack, runner, *lease, *interval)
}

// leaseDefault returns the lease default from REPLAY_LEASE, falling back to the
// frozen 5m default, so the Compose REPLAY_LEASE setting is actually honored.
func leaseDefault() time.Duration {
	raw := os.Getenv("REPLAY_LEASE")
	if raw == "" {
		return 5 * time.Minute
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		log.Printf("replay worker: invalid REPLAY_LEASE %q; using 5m", raw)
		return 5 * time.Minute
	}
	return d
}
