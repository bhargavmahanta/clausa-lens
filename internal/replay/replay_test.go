package replay

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/core"
)

// compile-time guarantee that the concrete persistence layer satisfies the
// narrow RunStore seam (wired at the composition point, not imported here).
var _ RunStore = (*core.PostgresRepository)(nil)

// ---------------------------------------------------------------------------
// fakeStore
// ---------------------------------------------------------------------------

// fakeStore implements RunStore with the same legality, immutability, 409 and
// baseline-authorization semantics as the core PostgresRepository (out of
// scope of this package's implementation). It lets the orchestration logic be
// tested before the real store is wired.
type fakeStore struct {
	mu   sync.Mutex
	runs map[string]contracts.ReplayRun
}

func newFakeStore() *fakeStore {
	return &fakeStore{runs: map[string]contracts.ReplayRun{}}
}

func (s *fakeStore) GetRun(ctx context.Context, runID string) (contracts.ReplayRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return contracts.ReplayRun{}, err
	}
	run, ok := s.runs[runID]
	if !ok {
		return contracts.ReplayRun{}, core.ErrNotFound
	}
	return run, nil
}

func (s *fakeStore) PutRun(ctx context.Context, run contracts.ReplayRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if run.Validate() != nil {
		return core.ErrInvalidLifecycle
	}
	if run.RunType == contracts.RunTypeWhatIf {
		baseline, ok := s.runs[run.BaselineRunID]
		if !ok || baseline.RunType != contracts.RunTypeBaseline || baseline.Status != contracts.ReplayRunCompleted ||
			baseline.Outcome != contracts.ReplayOutcomeReproduced || baseline.IsolationEvidence == nil ||
			baseline.IsolationEvidence.Verdict != contracts.VerdictPass || baseline.CapsuleID != run.CapsuleID ||
			baseline.Intervention != nil {
			return core.ErrInvalidLifecycle
		}
	}
	if _, exists := s.runs[run.RunID]; exists {
		return core.ErrConflict
	}
	s.runs[run.RunID] = run
	return nil
}

func (s *fakeStore) TransitionRun(ctx context.Context, from contracts.ReplayRunStatus, run contracts.ReplayRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if !contracts.CanTransitionReplayRun(from, run.Status) || run.Validate() != nil {
		return core.ErrInvalidLifecycle
	}
	stored, ok := s.runs[run.RunID]
	if !ok || stored.Status != from {
		return core.ErrInvalidLifecycle
	}
	if stored.ExecutionID != run.ExecutionID || stored.CapsuleID != run.CapsuleID || stored.CapsuleHash != run.CapsuleHash ||
		stored.RunType != run.RunType || stored.BaselineRunID != run.BaselineRunID || stored.TrialNumber != run.TrialNumber {
		return core.ErrInvalidLifecycle
	}
	s.runs[run.RunID] = run
	return nil
}

// ---------------------------------------------------------------------------
// fakePack
// ---------------------------------------------------------------------------

type fakePack struct {
	oracle contracts.OracleResult
	err    error
}

func (p *fakePack) Descriptor() contracts.SystemPackRef {
	return contracts.SystemPackRef{ID: "checkout_duplicate_effect", Version: "1.0.0", InterfaceVersion: contracts.ContractVersion}
}
func (p *fakePack) Normalize(context.Context, contracts.RawEvidence) ([]contracts.ExecutionEvent, error) {
	return nil, nil
}
func (p *fakePack) DetectIncident(context.Context, []contracts.ExecutionEvent) (contracts.OracleResult, error) {
	return contracts.OracleResult{}, nil
}
func (p *fakePack) ExtractFixtures(context.Context, contracts.Incident, []contracts.ExecutionEvent) (contracts.FixtureSet, error) {
	return contracts.FixtureSet{}, nil
}
func (p *fakePack) BuildReplayPlan(context.Context, contracts.Incident, contracts.FixtureSet) (contracts.ReplayPlan, error) {
	return contracts.ReplayPlan{}, nil
}
func (p *fakePack) ValidateCapsule(context.Context, contracts.ReplayCapsule) []contracts.ValidationIssue {
	return nil
}
func (p *fakePack) AllowedInterventions() []contracts.InterventionSpec {
	return nil
}
func (p *fakePack) ApplyIntervention(context.Context, contracts.ReplayPlan, contracts.Intervention) (contracts.ReplayPlan, error) {
	return contracts.ReplayPlan{}, nil
}
func (p *fakePack) Compare(context.Context, string, contracts.ReplayExecution, contracts.ReplayExecution) (contracts.ReplayDiff, error) {
	return contracts.ReplayDiff{}, nil
}
func (p *fakePack) EvaluateOutcome(context.Context, contracts.ReplayExecution) (contracts.OracleResult, error) {
	if p.err != nil {
		return contracts.OracleResult{}, p.err
	}
	return p.oracle, nil
}
func (p *fakePack) Labels() contracts.LabelSet { return contracts.LabelSet{} }

func reproducedOracle() contracts.OracleResult {
	return contracts.OracleResult{
		Oracle:                   contracts.FailureOracleRef{ID: "duplicate_ledger_effect", Version: "1.0.0"},
		Matched:                  true,
		EffectSummary:            contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2},
		RequiredEvidenceEventIDs: []string{"evt-timeout", "evt-retry", "evt-ledger-1", "evt-ledger-2"},
		Explanation:              "Baseline reproduced the timeout-driven duplicate ledger effect.",
	}
}

func unmatchedOracle() contracts.OracleResult {
	o := reproducedOracle()
	o.Matched = false
	o.EffectSummary = contracts.EffectSummary{PaymentAttemptCount: 1, LedgerCommitCount: 1}
	o.RequiredEvidenceEventIDs = []string{"evt-whatif-payment-complete", "evt-whatif-ledger-1"}
	o.Explanation = "Payment completed before timeout; no retry or duplicate effect occurred."
	return o
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

const hash64 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func runningBaseline(runID string) contracts.ReplayRun {
	return contracts.ReplayRun{
		SchemaVersion: contracts.ContractVersion,
		RunID:         runID,
		ExecutionID:   "exec-replay-" + runID,
		CapsuleID:     "cap-8271",
		CapsuleHash:   hash64,
		RunType:       contracts.RunTypeBaseline,
		TrialNumber:   1,
		Status:        contracts.ReplayRunRunning,
		StartedAt:     "2026-08-29T10:34:00Z",
	}
}

func whatIfRun(runID, baselineRunID string, intervention *contracts.Intervention) contracts.ReplayRun {
	return contracts.ReplayRun{
		SchemaVersion: contracts.ContractVersion,
		RunID:         runID,
		ExecutionID:   "exec-replay-" + runID,
		CapsuleID:     "cap-8271",
		CapsuleHash:   hash64,
		RunType:       contracts.RunTypeWhatIf,
		BaselineRunID: baselineRunID,
		Intervention:  intervention,
		TrialNumber:   1,
		Status:        contracts.ReplayRunRunning,
		StartedAt:     "2026-08-29T10:35:00Z",
	}
}

func passIsolation(runID string) contracts.IsolationEvidence {
	return contracts.IsolationEvidence{
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

func failIsolation(runID string) contracts.IsolationEvidence {
	evidence := passIsolation(runID)
	evidence.Verdict = contracts.VerdictFail
	evidence.NetworkPolicy = contracts.VerdictFail
	return evidence
}

func validIntervention() *contracts.Intervention {
	return &contracts.Intervention{Type: contracts.InterventionPaymentLatency, From: 350, To: 50, Unit: contracts.InterventionUnitMilliseconds}
}

// ---------------------------------------------------------------------------
// NewRun
// ---------------------------------------------------------------------------

func TestNewRunBaselineZeroInterventionValid(t *testing.T) {
	run, err := NewRun("run-base-8271", "exec-replay-base-8271", "cap-8271", hash64,
		contracts.RunTypeBaseline, "", nil, 1)
	if err != nil {
		t.Fatalf("NewRun: %v", err)
	}
	if run.Status != contracts.ReplayRunCreated {
		t.Fatalf("status = %s, want CREATED", run.Status)
	}
	if run.RunType != contracts.RunTypeBaseline || run.BaselineRunID != "" || run.Intervention != nil {
		t.Fatalf("baseline run shape wrong: %+v", run)
	}
	if err := run.Validate(); err != nil {
		t.Fatalf("created baseline must validate: %v", err)
	}
	if run.ObservedEventIDs == nil {
		t.Fatal("created run must carry a non-nil observed_event_ids slice")
	}
	if len(run.ObservedEventIDs) != 0 {
		t.Fatalf("created run observed_event_ids must be empty, got %d", len(run.ObservedEventIDs))
	}
}

func TestNewRunWhatIfRequiresIntervention(t *testing.T) {
	if _, err := NewRun("run-whatif-8271", "exec-replay-whatif-8271", "cap-8271", hash64,
		contracts.RunTypeWhatIf, "run-base-8271", nil, 1); err == nil {
		t.Fatal("what-if without intervention must be rejected")
	}
}

func TestNewRunWhatIfRequiresBaselineReference(t *testing.T) {
	if _, err := NewRun("run-whatif-8271", "exec-replay-whatif-8271", "cap-8271", hash64,
		contracts.RunTypeWhatIf, "", validIntervention(), 1); err == nil {
		t.Fatal("what-if without baseline reference must be rejected")
	}
}

func TestNewRunBaselineWithInterventionRejected(t *testing.T) {
	if _, err := NewRun("run-base-8271", "exec-replay-base-8271", "cap-8271", hash64,
		contracts.RunTypeBaseline, "", validIntervention(), 1); err == nil {
		t.Fatal("baseline with an intervention must be rejected")
	}
}

func TestNewRunWhatIfWithMismatchedInterventionRejected(t *testing.T) {
	bad := &contracts.Intervention{Type: contracts.InterventionPaymentLatency, From: 350, To: 350, Unit: contracts.InterventionUnitMilliseconds}
	if _, err := NewRun("run-whatif-8271", "exec-replay-whatif-8271", "cap-8271", hash64,
		contracts.RunTypeWhatIf, "run-base-8271", bad, 1); err == nil {
		t.Fatal("what-if with an invalid intervention must be rejected")
	}
}

// ---------------------------------------------------------------------------
// AdvanceRun
// ---------------------------------------------------------------------------

func TestAdvanceRunLegalCreatedToValidating(t *testing.T) {
	store := newFakeStore()
	created, err := NewRun("run-base-8271", "exec-replay-base-8271", "cap-8271", hash64,
		contracts.RunTypeBaseline, "", nil, 1)
	if err != nil {
		t.Fatalf("NewRun: %v", err)
	}
	if err := store.PutRun(context.Background(), created); err != nil {
		t.Fatalf("PutRun created: %v", err)
	}
	next, err := AdvanceRun(context.Background(), store, created, contracts.ReplayRunValidating)
	if err != nil {
		t.Fatalf("AdvanceRun: %v", err)
	}
	if next.Status != contracts.ReplayRunValidating {
		t.Fatalf("status = %s, want VALIDATING", next.Status)
	}
	stored, err := store.GetRun(context.Background(), created.RunID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if stored.Status != contracts.ReplayRunValidating {
		t.Fatalf("stored status = %s, want VALIDATING", stored.Status)
	}
}

func TestAdvanceRunIllegalValidatingToCompleted(t *testing.T) {
	store := newFakeStore()
	created, err := NewRun("run-base-8271", "exec-replay-base-8271", "cap-8271", hash64,
		contracts.RunTypeBaseline, "", nil, 1)
	if err != nil {
		t.Fatalf("NewRun: %v", err)
	}
	if err := store.PutRun(context.Background(), created); err != nil {
		t.Fatalf("PutRun: %v", err)
	}
	validating, err := AdvanceRun(context.Background(), store, created, contracts.ReplayRunValidating)
	if err != nil {
		t.Fatalf("advance to validating: %v", err)
	}
	if _, err := AdvanceRun(context.Background(), store, validating, contracts.ReplayRunCompleted); !errors.Is(err, core.ErrInvalidLifecycle) {
		t.Fatalf("illegal transition error = %v, want ErrInvalidLifecycle", err)
	}
}

// ---------------------------------------------------------------------------
// FinalizeRun
// ---------------------------------------------------------------------------

func TestFinalizeRunCompletesBaselineAndPersists(t *testing.T) {
	store := newFakeStore()
	run := runningBaseline("run-base-8271")
	if err := store.PutRun(context.Background(), run); err != nil {
		t.Fatalf("PutRun: %v", err)
	}
	effect := contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2}
	oracle := reproducedOracle()
	iso := passIsolation(run.RunID)

	completed, err := FinalizeRun(context.Background(), store, run,
		contracts.ReplayOutcomeReproduced, &effect, &oracle, &iso, nil)
	if err != nil {
		t.Fatalf("FinalizeRun: %v", err)
	}
	if completed.Status != contracts.ReplayRunCompleted {
		t.Fatalf("status = %s, want COMPLETED", completed.Status)
	}
	if completed.Outcome != contracts.ReplayOutcomeReproduced {
		t.Fatalf("outcome = %s, want REPRODUCED", completed.Outcome)
	}
	if completed.Error != nil {
		t.Fatalf("completed run must not carry an error, got %v", completed.Error)
	}
	if err := completed.Validate(); err != nil {
		t.Fatalf("completed run must validate: %v", err)
	}
	stored, err := store.GetRun(context.Background(), run.RunID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if stored.Status != contracts.ReplayRunCompleted || stored.Outcome != contracts.ReplayOutcomeReproduced {
		t.Fatalf("store did not persist completed run: %+v", stored)
	}
}

func TestFinalizeRunWithErrorProducesFailedNotCompleted(t *testing.T) {
	store := newFakeStore()
	run := runningBaseline("run-base-8271")
	if err := store.PutRun(context.Background(), run); err != nil {
		t.Fatalf("PutRun: %v", err)
	}
	effect := contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2}
	oracle := reproducedOracle()
	iso := passIsolation(run.RunID)
	runErr := &contracts.RunError{Code: contracts.InternalFailure, Message: "instrumentation crashed", Retryable: true, Details: map[string]any{"run_id": run.RunID}}

	result, err := FinalizeRun(context.Background(), store, run,
		contracts.ReplayOutcomeReproduced, &effect, &oracle, &iso, runErr)
	if err != nil {
		t.Fatalf("FinalizeRun: %v", err)
	}
	if result.Status != contracts.ReplayRunFailed {
		t.Fatalf("status = %s, want FAILED (not COMPLETED)", result.Status)
	}
	if result.Error == nil || result.Error.Code != contracts.InternalFailure {
		t.Fatalf("failed run must carry the internal error, got %+v", result.Error)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("failed run must validate: %v", err)
	}
}

func TestFinalizeRunRefusesUnsafeIsolation(t *testing.T) {
	store := newFakeStore()
	run := runningBaseline("run-base-8271")
	if err := store.PutRun(context.Background(), run); err != nil {
		t.Fatalf("PutRun: %v", err)
	}
	effect := contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2}
	oracle := reproducedOracle()
	iso := failIsolation(run.RunID)

	if _, err := FinalizeRun(context.Background(), store, run,
		contracts.ReplayOutcomeReproduced, &effect, &oracle, &iso, nil); err == nil {
		t.Fatal("FinalizeRun must refuse a COMPLETED run with failed isolation")
	}
	stored, err := store.GetRun(context.Background(), run.RunID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if stored.Status != contracts.ReplayRunRunning {
		t.Fatalf("store must not have transitioned an unsafe run, got %s", stored.Status)
	}
}

// ---------------------------------------------------------------------------
// ValidateIsolation
// ---------------------------------------------------------------------------

func TestValidateIsolationAcceptsCanonicalReplayOnly(t *testing.T) {
	if err := ValidateIsolation(passIsolation("run-base-8271")); err != nil {
		t.Fatalf("canonical replay-only isolation must be accepted: %v", err)
	}
}

func TestValidateIsolationRejectsProductionDatastore(t *testing.T) {
	iso := passIsolation("run-base-8271")
	iso.DatastoreDestinations = []string{"postgres://prod/ledger"}
	if err := ValidateIsolation(iso); err == nil {
		t.Fatal("production datastore destination must be rejected")
	}
}

func TestValidateIsolationRejectsNetworkPolicyFail(t *testing.T) {
	iso := passIsolation("run-base-8271")
	iso.NetworkPolicy = contracts.VerdictFail
	if err := ValidateIsolation(iso); err == nil {
		t.Fatal("failed network policy must be rejected")
	}
}

func TestValidateIsolationRejectsDeniedInteraction(t *testing.T) {
	iso := passIsolation("run-base-8271")
	iso.DeniedInteractions = []contracts.DependencyInteraction{{
		Dependency: "payment_simulator", Destination: "http://payment-simulator:8080",
		Operation: "authorize", Result: contracts.InteractionDenied,
	}}
	if err := ValidateIsolation(iso); err == nil {
		t.Fatal("denied interaction must be rejected")
	}
}

func TestValidateIsolationRejectsFailedTeardown(t *testing.T) {
	iso := passIsolation("run-base-8271")
	iso.TeardownResult = contracts.VerdictFail
	if err := ValidateIsolation(iso); err == nil {
		t.Fatal("failed teardown must be rejected")
	}
}

func TestValidateIsolationRejectsNonReplayCredential(t *testing.T) {
	iso := passIsolation("run-base-8271")
	iso.CredentialProfile = contracts.CredentialProfile("production-credentials")
	if err := ValidateIsolation(iso); err == nil {
		t.Fatal("non-replay credential profile must be rejected")
	}
}

// ---------------------------------------------------------------------------
// Evaluate
// ---------------------------------------------------------------------------

func TestEvaluateProducesCompletedReproducedBaseline(t *testing.T) {
	run := runningBaseline("run-base-8271")
	events := []contracts.ExecutionEvent{validEvent("evt-timeout"), validEvent("evt-retry")}
	pack := &fakePack{oracle: reproducedOracle()}

	result, err := Evaluate(context.Background(), pack, run, events)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Status != contracts.ReplayRunCompleted {
		t.Fatalf("status = %s, want COMPLETED", result.Status)
	}
	if result.Outcome != contracts.ReplayOutcomeReproduced {
		t.Fatalf("outcome = %s, want REPRODUCED", result.Outcome)
	}
	if result.EffectSummary == nil || result.EffectSummary.PaymentAttemptCount != 2 {
		t.Fatalf("effect summary not derived: %+v", result.EffectSummary)
	}
	if result.IsolationEvidence == nil || result.IsolationEvidence.Verdict != contracts.VerdictPass {
		t.Fatalf("completed run must carry replay-only isolation: %+v", result.IsolationEvidence)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("evaluated completed run must pass contract validation: %v", err)
	}
}

func TestEvaluateProducesMitigatedWhatIf(t *testing.T) {
	run := whatIfRun("run-whatif-8271", "run-base-8271", validIntervention())
	events := []contracts.ExecutionEvent{validEvent("evt-whatif-payment-complete"), validEvent("evt-whatif-ledger-1")}
	pack := &fakePack{oracle: unmatchedOracle()}

	result, err := Evaluate(context.Background(), pack, run, events)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Status != contracts.ReplayRunCompleted {
		t.Fatalf("status = %s, want COMPLETED", result.Status)
	}
	if result.Outcome != contracts.ReplayOutcomeMitigated {
		t.Fatalf("outcome = %s, want MITIGATED", result.Outcome)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("evaluated what-if must validate: %v", err)
	}
}

func TestEvaluateProducesFailedRunOnPackError(t *testing.T) {
	run := runningBaseline("run-base-8271")
	events := []contracts.ExecutionEvent{validEvent("evt-timeout")}
	pack := &fakePack{err: errors.New("oracle unavailable")}

	result, err := Evaluate(context.Background(), pack, run, events)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Status != contracts.ReplayRunFailed {
		t.Fatalf("status = %s, want FAILED", result.Status)
	}
	if result.Error == nil || result.Error.Code != contracts.InternalFailure {
		t.Fatalf("run must be FAILED with InternalFailure, got %+v", result.Error)
	}
	if result.Outcome != "" {
		t.Fatalf("failed run must omit outcome, got %q", result.Outcome)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("failed run must validate: %v", err)
	}
}

// ---------------------------------------------------------------------------
// lifecycle demonstration
// ---------------------------------------------------------------------------

func TestBaselineLifecycleCreatedToCompleted(t *testing.T) {
	store := newFakeStore()
	created, err := NewRun("run-base-8271", "exec-replay-base-8271", "cap-8271", hash64,
		contracts.RunTypeBaseline, "", nil, 1)
	if err != nil {
		t.Fatalf("NewRun: %v", err)
	}
	if err := store.PutRun(context.Background(), created); err != nil {
		t.Fatalf("PutRun: %v", err)
	}

	validating, err := AdvanceRun(context.Background(), store, created, contracts.ReplayRunValidating)
	if err != nil {
		t.Fatalf("advance to validating: %v", err)
	}
	running, err := AdvanceRun(context.Background(), store, validating, contracts.ReplayRunRunning)
	if err != nil {
		t.Fatalf("advance to running: %v", err)
	}

	effect := contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2}
	oracle := reproducedOracle()
	iso := passIsolation(running.RunID)
	completed, err := FinalizeRun(context.Background(), store, running,
		contracts.ReplayOutcomeReproduced, &effect, &oracle, &iso, nil)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if completed.Status != contracts.ReplayRunCompleted || completed.Outcome != contracts.ReplayOutcomeReproduced {
		t.Fatalf("lifecycle did not reach COMPLETED(REPRODUCED): %+v", completed)
	}
	if err := completed.Validate(); err != nil {
		t.Fatalf("completed run must validate: %v", err)
	}
}

func TestWhatIfCannotBeCreatedWithoutReproducedBaseline(t *testing.T) {
	store := newFakeStore() // no baseline present
	whatif, err := NewRun("run-whatif-8271", "exec-replay-whatif-8271", "cap-8271", hash64,
		contracts.RunTypeWhatIf, "run-base-8271", validIntervention(), 1)
	if err != nil {
		t.Fatalf("NewRun: %v", err)
	}
	if err := store.PutRun(context.Background(), whatif); !errors.Is(err, core.ErrInvalidLifecycle) {
		t.Fatalf("what-if without a completed reproduced baseline error = %v, want ErrInvalidLifecycle", err)
	}
}

func validEvent(id string) contracts.ExecutionEvent {
	return contracts.ExecutionEvent{
		SchemaVersion:      contracts.ContractVersion,
		EventID:            id,
		ExecutionID:        "exec-replay-base-8271",
		TraceID:            "trace-8271",
		Component:          contracts.ComponentRef{Name: "payment", Instance: "payment-1"},
		Operation:          contracts.OperationRef{Name: "authorize", Kind: contracts.OperationDependency},
		EventType:          contracts.EventComplete,
		Attempt:            1,
		LogicalOperationID: "checkout-8271",
		OccurredAt:         "2026-08-29T10:34:00.5Z",
		Sequence:           1,
		Status:             contracts.EventSuccess,
		Attributes:         map[string]any{},
	}
}

func TestValidationOfInterventionStrings(t *testing.T) {
	// guard against accidental redefinition of frozen identifiers
	if contracts.InterventionPaymentLatency != "PAYMENT_LATENCY" {
		t.Fatal("frozen intervention type changed")
	}
	if strings.Repeat("a", 64) != hash64 {
		t.Fatal("hash helper drift")
	}
}
