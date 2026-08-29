package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/packregistry"
	"github.com/causalens/causalens/internal/replay"
)

var workerDigest = strings.Repeat("a", 64)

type fakeWorkerStore struct {
	runs     map[string]contracts.ReplayRun
	capsules map[string]contracts.ReplayCapsule
	err      error
}

func (f *fakeWorkerStore) GetRun(_ context.Context, id string) (contracts.ReplayRun, error) {
	if f.err != nil {
		return contracts.ReplayRun{}, f.err
	}
	run, ok := f.runs[id]
	if !ok {
		return contracts.ReplayRun{}, errors.New("not found")
	}
	return run, nil
}
func (f *fakeWorkerStore) PutRun(_ context.Context, run contracts.ReplayRun) error {
	if f.err != nil {
		return f.err
	}
	f.runs[run.RunID] = run
	return nil
}
func (f *fakeWorkerStore) TransitionRun(_ context.Context, from contracts.ReplayRunStatus, run contracts.ReplayRun) error {
	if f.err != nil {
		return f.err
	}
	current, ok := f.runs[run.RunID]
	if !ok || current.Status != from {
		return errors.New("invalid lifecycle")
	}
	f.runs[run.RunID] = run
	return nil
}
func (f *fakeWorkerStore) EventsForRun(_ context.Context, run contracts.ReplayRun) ([]contracts.ExecutionEvent, error) {
	if f.err != nil {
		return nil, f.err
	}
	return nil, nil
}
func (f *fakeWorkerStore) GetCapsule(_ context.Context, id string) (contracts.ReplayCapsule, error) {
	if f.err != nil {
		return contracts.ReplayCapsule{}, f.err
	}
	c, ok := f.capsules[id]
	if !ok {
		return contracts.ReplayCapsule{}, errors.New("capsule not found")
	}
	return c, nil
}
func (f *fakeWorkerStore) IngestEvent(_ context.Context, _ contracts.ExecutionEvent) error {
	if f.err != nil {
		return f.err
	}
	return nil
}
func (f *fakeWorkerStore) ListRuns(_ context.Context, status contracts.ReplayRunStatus) ([]contracts.ReplayRun, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []contracts.ReplayRun
	for _, run := range f.runs {
		if run.Status == status {
			out = append(out, run)
		}
	}
	return out, nil
}

func workerBaseline(runID string) contracts.ReplayRun {
	return contracts.ReplayRun{SchemaVersion: contracts.ContractVersion, RunID: runID, ExecutionID: "exec-base", CapsuleID: "cap-1", CapsuleHash: workerDigest, RunType: contracts.RunTypeBaseline, TrialNumber: 1, Status: contracts.ReplayRunCreated, ObservedEventIDs: []string{}}
}
func workerWhatIf(runID string) contracts.ReplayRun {
	return contracts.ReplayRun{SchemaVersion: contracts.ContractVersion, RunID: runID, ExecutionID: "exec-whatif", CapsuleID: "cap-1", CapsuleHash: workerDigest, RunType: contracts.RunTypeWhatIf, BaselineRunID: "run-base", Intervention: &contracts.Intervention{Type: contracts.InterventionPaymentLatency, From: 350, To: 50, Unit: contracts.InterventionUnitMilliseconds}, TrialNumber: 1, Status: contracts.ReplayRunCreated, ObservedEventIDs: []string{}}
}

func validCapsule() contracts.ReplayCapsule {
	return contracts.ReplayCapsule{
		SchemaVersion: contracts.ContractVersion, CapsuleID: "cap-1",
		Integrity: contracts.Integrity{Algorithm: contracts.IntegritySHA256, Digest: workerDigest},
	}
}

type okRunner struct{}

func (okRunner) Run(context.Context, replay.RunnerConfig) (replay.RunResult, error) {
	return replay.RunResult{}, nil
}

func TestProcessRunRequiresPack(t *testing.T) {
	store := &fakeWorkerStore{runs: map[string]contracts.ReplayRun{"run-base": workerBaseline("run-base")}}
	if _, err := processRun(context.Background(), store, nil, okRunner{}, "run-base"); err == nil {
		t.Fatal("expected an error when no pack is wired")
	}
}

func TestProcessRunRequiresRunner(t *testing.T) {
	store := &fakeWorkerStore{runs: map[string]contracts.ReplayRun{"run-base": workerBaseline("run-base")}}
	if _, err := processRun(context.Background(), store, packregistry.NewDevPack(), nil, "run-base"); err == nil {
		t.Fatal("expected an error when no runner is configured")
	}
}

func TestProcessRunUnknownRunFails(t *testing.T) {
	store := &fakeWorkerStore{runs: map[string]contracts.ReplayRun{}}
	if _, err := processRun(context.Background(), store, packregistry.NewDevPack(), okRunner{}, "missing"); err == nil {
		t.Fatal("expected an error for an unknown run")
	}
}

func TestProcessRunSkipsTerminal(t *testing.T) {
	done := workerBaseline("run-base")
	done.Status = contracts.ReplayRunCompleted
	done.Outcome = contracts.ReplayOutcomeReproduced
	store := &fakeWorkerStore{runs: map[string]contracts.ReplayRun{"run-base": done}}
	result, err := processRun(context.Background(), store, packregistry.NewDevPack(), okRunner{}, "run-base")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != contracts.ReplayRunCompleted {
		t.Fatalf("terminal run should be returned unchanged: %s", result.Status)
	}
}

func TestReclaimOrphansFailsStranded(t *testing.T) {
	run := workerBaseline("run-base")
	run.Status = contracts.ReplayRunRunning
	run.StartedAt = "2020-01-01T00:00:00Z"
	store := &fakeWorkerStore{runs: map[string]contracts.ReplayRun{"run-base": run}}
	reclaimed, err := reclaimOrphans(context.Background(), store, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(reclaimed) != 1 {
		t.Fatalf("expected 1 reclaimed run, got %d", len(reclaimed))
	}
	got := store.runs["run-base"]
	if got.Status != contracts.ReplayRunFailed || got.Error == nil {
		t.Fatalf("stranded run should be FAILED: %+v", got)
	}
}

// TestReclaimOrphansSkipsFreshClaim verifies a run a concurrent worker just
// claimed (VALIDATING, no StartedAt) is never reclaimed under a positive lease,
// so two workers cannot both fail/execute the same run.
func TestReclaimOrphansSkipsFreshClaim(t *testing.T) {
	run := workerBaseline("run-base")
	run.Status = contracts.ReplayRunValidating
	store := &fakeWorkerStore{runs: map[string]contracts.ReplayRun{"run-base": run}}
	reclaimed, err := reclaimOrphans(context.Background(), store, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(reclaimed) != 0 {
		t.Fatalf("fresh VALIDATING claim must not be reclaimed, got %d", len(reclaimed))
	}
	if store.runs["run-base"].Status != contracts.ReplayRunValidating {
		t.Fatalf("run should remain VALIDATING: %+v", store.runs["run-base"])
	}
}
