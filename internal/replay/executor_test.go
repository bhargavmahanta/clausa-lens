package replay

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/causalens/causalens/internal/capsule"
	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/packregistry"
)

func makeCapsule(t *testing.T) contracts.ReplayCapsule {
	t.Helper()
	c := contracts.ReplayCapsule{
		SchemaVersion:        contracts.ContractVersion,
		CapsuleID:            "cap-1",
		CreatedAt:            "2026-08-29T10:33:00Z",
		Source:               contracts.CapsuleSource{IncidentID: "inc-1", TraceID: "trace-1", ExecutionID: "exec-1", CaptureEnvironment: contracts.CaptureDemo, CapturedAt: "2026-08-29T10:32:01.561Z"},
		SystemPack:           contracts.SystemPackRef{ID: "checkout_duplicate_effect_dev", Version: "0.0.0-dev", InterfaceVersion: contracts.ContractVersion},
		Trigger:              contracts.Trigger{RequestOrMessage: map[string]any{"method": "POST"}, SanitizedHeaders: map[string]string{"content-type": "application/json"}},
		EventIDs:             []string{"e1"},
		GraphID:              "graph-1",
		StateFixtures:        []contracts.StateFixture{{FixtureID: "state-ledger-empty", Kind: contracts.StateFixturePostgresRowset, ContentRef: "fixture://golden/ledger-empty-v1", ContentDigest: strings.Repeat("b", 64), SanitizationStatus: contracts.SanitizationPass, ResetStrategy: contracts.FixtureTruncateAndLoad}},
		DependencyFixtures:   []contracts.DependencyFixture{{FixtureID: "dependency-payment-350ms", Dependency: contracts.DependencyPaymentSimulator, RequestMatch: map[string]any{"logical_operation_id": "checkout-1"}, Response: map[string]any{"status": "APPROVED"}, LatencyMS: 350, FailureMode: contracts.FailureModeNone, InvocationLimit: 2}},
		TimingPolicy:         contracts.TimingPolicy{ClockToleranceMS: 5, TimeoutMS: 200},
		ReplayPlan:           contracts.ReplayPlan{Entrypoint: "gateway.checkout", RequiredComponents: []string{"gateway", "checkout", "payment", "ledger"}, FixtureLoadOrder: []string{"state-ledger-empty", "dependency-payment-350ms"}, ResetStrategy: contracts.ReplayResetGoldenV1},
		FailureOracle:        contracts.FailureOracleSpec{ID: "duplicate_ledger_effect", Version: "1.0.0", ExpectedMatch: true, ExpectedEffectSummary: contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2}},
		AllowedInterventions: []contracts.InterventionSpec{{Type: contracts.InterventionPaymentLatency, ValueType: contracts.InterventionValueInteger, Unit: contracts.InterventionUnitMilliseconds, Minimum: 0, Maximum: 5000}},
		Safety:               capsule.SafePolicy(),
	}
	c.Integrity.Algorithm = contracts.IntegritySHA256
	digest, err := capsule.ComputeDigest(c)
	if err != nil {
		t.Fatalf("compute digest: %v", err)
	}
	c.Integrity.Digest = digest
	if !capsule.VerifyDigest(c) {
		t.Fatal("test capsule digest should verify")
	}
	return c
}

type fakeRunner struct {
	result RunResult
	err    error
}

func (f fakeRunner) Run(context.Context, RunnerConfig) (RunResult, error) { return f.result, f.err }

func baselineRun(runID string) contracts.ReplayRun {
	return contracts.ReplayRun{SchemaVersion: contracts.ContractVersion, RunID: runID, ExecutionID: "e", CapsuleID: "cap-1", CapsuleHash: strings.Repeat("a", 64), RunType: contracts.RunTypeBaseline, TrialNumber: 1, Status: contracts.ReplayRunRunning}
}

func mkWhatIfRun(runID string) contracts.ReplayRun {
	run := baselineRun(runID)
	run.RunType = contracts.RunTypeWhatIf
	run.BaselineRunID = "run-base"
	run.Intervention = &contracts.Intervention{Type: contracts.InterventionPaymentLatency, From: 350, To: 50, Unit: contracts.InterventionUnitMilliseconds}
	return run
}

func oneEvent(runID string) []contracts.ExecutionEvent {
	return []contracts.ExecutionEvent{{SchemaVersion: contracts.ContractVersion, EventID: "rt-1", ExecutionID: "e", TraceID: "trace-1", ReplayRunID: runID, Component: contracts.ComponentRef{Name: "payment", Instance: "payment-1"}, Operation: contracts.OperationRef{Name: "authorize", Kind: contracts.OperationDependency}, EventType: contracts.EventStart, Attempt: 1, LogicalOperationID: "checkout-1", OccurredAt: "2026-08-29T10:32:01Z", Status: contracts.EventRunning, Attributes: map[string]any{}}}
}

func validRunResult(runID string) RunResult {
	events := oneEvent(runID)
	return RunResult{Events: events, Isolation: IsolationFor("replay-run-"+runID, true)}
}

func TestExecuteRejectsBadDigest(t *testing.T) {
	capsule := makeCapsule(t)
	capsule.Integrity.Digest = strings.Repeat("0", 64)
	_, err := Execute(context.Background(), ExecutionConfig{Pack: nil, Capsule: capsule, Run: baselineRun("run-base"), Plan: capsule.ReplayPlan, Fixtures: contracts.FixtureSet{}, Runner: fakeRunner{}, LatencyMS: 350})
	if !errors.Is(err, ErrInvalidDigest) {
		t.Fatalf("expected ErrInvalidDigest, got %v", err)
	}
}

func TestExecuteRejectsBaselineWithIntervention(t *testing.T) {
	capsule := makeCapsule(t)
	run := baselineRun("run-base")
	run.Intervention = &contracts.Intervention{Type: contracts.InterventionPaymentLatency, From: 350, To: 50, Unit: contracts.InterventionUnitMilliseconds}
	_, err := Execute(context.Background(), ExecutionConfig{Pack: packregistry.NewDevPack(), Capsule: capsule, Run: run, Plan: capsule.ReplayPlan, Fixtures: contracts.FixtureSet{}, Runner: fakeRunner{}, LatencyMS: 350})
	if !errors.Is(err, ErrInvalidIntervention) {
		t.Fatalf("expected ErrInvalidIntervention, got %v", err)
	}
}

func TestExecuteRejectsWhatIfWithoutBaseline(t *testing.T) {
	capsule := makeCapsule(t)
	run := mkWhatIfRun("run-whatif")
	run.BaselineRunID = ""
	_, err := Execute(context.Background(), ExecutionConfig{Pack: packregistry.NewDevPack(), Capsule: capsule, Run: run, Plan: capsule.ReplayPlan, Fixtures: contracts.FixtureSet{}, Runner: fakeRunner{}, LatencyMS: 50})
	if !errors.Is(err, ErrInvalidIntervention) {
		t.Fatalf("expected ErrInvalidIntervention, got %v", err)
	}
}

func TestExecuteRejectsNoReplayEvents(t *testing.T) {
	capsule := makeCapsule(t)
	result := RunResult{Isolation: IsolationFor("replay-run-run-base", true)}
	_, err := Execute(context.Background(), ExecutionConfig{Pack: packregistry.NewDevPack(), Capsule: capsule, Run: baselineRun("run-base"), Plan: capsule.ReplayPlan, Fixtures: contracts.FixtureSet{}, Runner: fakeRunner{result: result}, LatencyMS: 350})
	if !errors.Is(err, ErrNoReplayEvents) {
		t.Fatalf("expected ErrNoReplayEvents, got %v", err)
	}
}

func TestExecuteRejectsTeardownFailure(t *testing.T) {
	capsule := makeCapsule(t)
	result := RunResult{Events: oneEvent("run-base"), Isolation: IsolationFor("replay-run-run-base", false), Teardown: true}
	_, err := Execute(context.Background(), ExecutionConfig{Pack: packregistry.NewDevPack(), Capsule: capsule, Run: baselineRun("run-base"), Plan: capsule.ReplayPlan, Fixtures: contracts.FixtureSet{}, Runner: fakeRunner{result: result}, LatencyMS: 350})
	if !errors.Is(err, ErrTeardown) {
		t.Fatalf("expected ErrTeardown, got %v", err)
	}
}

func TestExecuteRejectsMissingReplayRunID(t *testing.T) {
	capsule := makeCapsule(t)
	events := oneEvent("run-base")
	events[0].ReplayRunID = ""
	result := RunResult{Events: events, Isolation: IsolationFor("replay-run-run-base", true)}
	_, err := Execute(context.Background(), ExecutionConfig{Pack: packregistry.NewDevPack(), Capsule: capsule, Run: baselineRun("run-base"), Plan: capsule.ReplayPlan, Fixtures: contracts.FixtureSet{}, Runner: fakeRunner{result: result}, LatencyMS: 350})
	if !errors.Is(err, ErrReplayRunID) {
		t.Fatalf("expected ErrReplayRunID, got %v", err)
	}
}

func TestExecuteSucceedsWithRealIsolation(t *testing.T) {
	capsule := makeCapsule(t)
	run := baselineRun("run-base")
	result, err := Execute(context.Background(), ExecutionConfig{Pack: packregistry.NewDevPack(), Capsule: capsule, Run: run, Plan: capsule.ReplayPlan, Fixtures: contracts.FixtureSet{}, Runner: fakeRunner{result: validRunResult(run.RunID)}, LatencyMS: 350})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 1 || result.Events[0].ReplayRunID != "run-base" {
		t.Fatalf("execution events: %+v", result.Events)
	}
	if err := ValidateIsolation(result.Isolation); err != nil {
		t.Fatalf("real isolation invalid: %v", err)
	}
}
