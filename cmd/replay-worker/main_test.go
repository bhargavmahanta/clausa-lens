package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/packregistry"
)

var workerDigest = strings.Repeat("a", 64)

type fakeRunStore struct {
	runs   map[string]contracts.ReplayRun
	events map[string][]contracts.ExecutionEvent
	err    error
}

func (f *fakeRunStore) GetRun(_ context.Context, id string) (contracts.ReplayRun, error) {
	if f.err != nil {
		return contracts.ReplayRun{}, f.err
	}
	run, ok := f.runs[id]
	if !ok {
		return contracts.ReplayRun{}, errors.New("not found")
	}
	return run, nil
}
func (f *fakeRunStore) PutRun(_ context.Context, run contracts.ReplayRun) error {
	if f.err != nil {
		return f.err
	}
	f.runs[run.RunID] = run
	return nil
}
func (f *fakeRunStore) TransitionRun(_ context.Context, from contracts.ReplayRunStatus, run contracts.ReplayRun) error {
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
func (f *fakeRunStore) EventsForRun(_ context.Context, run contracts.ReplayRun) ([]contracts.ExecutionEvent, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.events[run.RunID], nil
}

func workerBaseline(runID string) contracts.ReplayRun {
	return contracts.ReplayRun{SchemaVersion: contracts.ContractVersion, RunID: runID, ExecutionID: "exec-base", CapsuleID: "cap-1", CapsuleHash: workerDigest, RunType: contracts.RunTypeBaseline, TrialNumber: 1, Status: contracts.ReplayRunCreated, ObservedEventIDs: []string{}}
}
func workerWhatIf(runID string) contracts.ReplayRun {
	return contracts.ReplayRun{SchemaVersion: contracts.ContractVersion, RunID: runID, ExecutionID: "exec-whatif", CapsuleID: "cap-1", CapsuleHash: workerDigest, RunType: contracts.RunTypeWhatIf, BaselineRunID: "run-base", Intervention: &contracts.Intervention{Type: contracts.InterventionPaymentLatency, From: 350, To: 50, Unit: contracts.InterventionUnitMilliseconds}, TrialNumber: 1, Status: contracts.ReplayRunCreated, ObservedEventIDs: []string{}}
}

func TestProcessRunBaselineBecomesReproduced(t *testing.T) {
	store := &fakeRunStore{runs: map[string]contracts.ReplayRun{"run-base": workerBaseline("run-base")}}
	result, err := processRun(context.Background(), store, store, packregistry.NewDevPack(), "run-base")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != contracts.ReplayRunCompleted || result.Outcome != contracts.ReplayOutcomeReproduced {
		t.Fatalf("baseline result: status=%s outcome=%s", result.Status, result.Outcome)
	}
	if result.EffectSummary == nil || result.EffectSummary.LedgerCommitCount != 2 || result.IsolationEvidence == nil {
		t.Fatalf("baseline missing result fields: %+v", result)
	}
	if got := store.runs["run-base"].Status; got != contracts.ReplayRunCompleted {
		t.Fatalf("persisted status = %s", got)
	}
}

func TestProcessRunWhatIfBecomesMitigated(t *testing.T) {
	store := &fakeRunStore{
		runs: map[string]contracts.ReplayRun{
			"run-base":   workerBaseline("run-base"),
			"run-whatif": workerWhatIf("run-whatif"),
		},
	}
	result, err := processRun(context.Background(), store, store, packregistry.NewDevPack(), "run-whatif")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != contracts.ReplayRunCompleted || result.Outcome != contracts.ReplayOutcomeMitigated {
		t.Fatalf("what-if result: status=%s outcome=%s", result.Status, result.Outcome)
	}
	if result.EffectSummary == nil || result.EffectSummary.LedgerCommitCount != 1 {
		t.Fatalf("what-if effect summary: %+v", result.EffectSummary)
	}
}

func TestProcessRunSkipsTerminal(t *testing.T) {
	done := workerBaseline("run-base")
	done.Status = contracts.ReplayRunCompleted
	done.Outcome = contracts.ReplayOutcomeReproduced
	store := &fakeRunStore{runs: map[string]contracts.ReplayRun{"run-base": done}}
	result, err := processRun(context.Background(), store, store, packregistry.NewDevPack(), "run-base")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != contracts.ReplayRunCompleted {
		t.Fatalf("terminal run should be returned unchanged: %s", result.Status)
	}
}

func TestProcessRunRequiresPack(t *testing.T) {
	store := &fakeRunStore{runs: map[string]contracts.ReplayRun{"run-base": workerBaseline("run-base")}}
	if _, err := processRun(context.Background(), store, store, nil, "run-base"); err == nil {
		t.Fatal("expected an error when no pack is wired")
	}
}

func TestProcessRunUnknownRunFails(t *testing.T) {
	store := &fakeRunStore{runs: map[string]contracts.ReplayRun{}}
	if _, err := processRun(context.Background(), store, store, packregistry.NewDevPack(), "missing"); err == nil {
		t.Fatal("expected an error for an unknown run")
	}
}
