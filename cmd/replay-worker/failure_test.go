package main

import (
	"context"
	"testing"
	"time"

	"github.com/causalens/causalens/internal/contracts"
	checkout "github.com/causalens/causalens/internal/systempack/checkout"
)

func workerFailureStore(cap contracts.ReplayCapsule) *fakeWorkerStore {
	return &fakeWorkerStore{
		runs:     map[string]contracts.ReplayRun{},
		capsules: map[string]contracts.ReplayCapsule{"cap-1": cap},
	}
}

func reproducedBaseline(runID string) contracts.ReplayRun {
	run := workerBaseline(runID)
	run.Status = contracts.ReplayRunCompleted
	run.Outcome = contracts.ReplayOutcomeReproduced
	run.StartedAt = "2026-08-29T10:34:00Z"
	run.CompletedAt = "2026-08-29T10:34:01Z"
	run.EffectSummary = &contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2}
	run.FailureOracleResult = &contracts.OracleResult{Oracle: contracts.FailureOracleRef{ID: checkout.OracleID, Version: "1.0.0"}, Matched: true, EffectSummary: contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2}, RequiredEvidenceEventIDs: []string{"e"}, Explanation: "reproduced"}
	run.IsolationEvidence = &contracts.IsolationEvidence{PolicyVersion: contracts.ContractVersion, Verdict: contracts.VerdictPass, RuntimeNamespace: "ns", NetworkPolicy: contracts.VerdictPass, CredentialProfile: contracts.CredentialReplayOnly, DatastoreDestinations: []string{"postgres://replay/ledger"}, SimulatorInteractions: []contracts.DependencyInteraction{}, DeniedInteractions: []contracts.DependencyInteraction{}, TeardownResult: contracts.VerdictPass}
	return run
}

// TestWorkerWhatIfWithoutEligibleBaseline verifies the frozen baseline gate: a
// what-if whose baseline is not COMPLETED/REPRODUCED with passing isolation is
// BLOCKED, never COMPLETED.
func TestWorkerWhatIfWithoutEligibleBaseline(t *testing.T) {
	ctx := context.Background()
	cap := compileCapsule(t)
	store := workerFailureStore(cap)
	base := workerBaseline("run-base") // CREATED, not COMPLETED
	store.runs["run-base"] = base
	run := workerWhatIf("run-whatif")
	store.runs["run-whatif"] = run

	result, err := processRun(ctx, store, checkout.New(), newDemoRunner(), "run-whatif", 5*time.Minute)
	if err != nil {
		t.Fatalf("processRun: %v", err)
	}
	if result.Status != contracts.ReplayRunBlocked {
		t.Fatalf("expected BLOCKED, got %s", result.Status)
	}
	if result.Outcome != "" || result.Error == nil {
		t.Fatalf("blocked run must have no outcome and an error: %+v", result)
	}
}

// TestWorkerBadCapsuleDigestBlock verifies a capsule whose integrity digest no
// longer matches its content is BLOCKED before any replay runs.
func TestWorkerBadCapsuleDigestBlock(t *testing.T) {
	ctx := context.Background()
	cap := compileCapsule(t)
	cap.Integrity.Digest = "0000000000000000000000000000000000000000000000000000000000000000"
	store := workerFailureStore(cap)
	store.runs["run-base"] = workerBaseline("run-base")

	result, err := processRun(ctx, store, checkout.New(), newDemoRunner(), "run-base", 5*time.Minute)
	if err != nil {
		t.Fatalf("processRun: %v", err)
	}
	if result.Status != contracts.ReplayRunBlocked || result.Error == nil || result.Error.Code != contracts.IntegrityMismatch {
		t.Fatalf("expected INTEGRITY_MISMATCH blocked, got %+v", result)
	}
}

// TestWorkerMissingCapsuleFails verifies a run referencing a nonexistent capsule
// never reaches COMPLETED.
func TestWorkerMissingCapsuleFails(t *testing.T) {
	ctx := context.Background()
	store := &fakeWorkerStore{runs: map[string]contracts.ReplayRun{"run-base": workerBaseline("run-base")}}
	result, err := processRun(ctx, store, checkout.New(), newDemoRunner(), "run-base", 5*time.Minute)
	if err != nil {
		t.Fatalf("processRun: %v", err)
	}
	if result.Status != contracts.ReplayRunFailed {
		t.Fatalf("expected FAILED, got %s", result.Status)
	}
}
