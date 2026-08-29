package checkout

import (
	"context"
	"testing"

	"github.com/causalens/causalens/internal/contracts"
)

func goldenIncident() contracts.Incident {
	return contracts.Incident{
		SchemaVersion:      contracts.ContractVersion,
		IncidentID:         "inc-8271",
		Status:             contracts.IncidentReady,
		FailureOracle:      oracleRef(),
		SystemPack:         New().Descriptor(),
		TraceID:            "trace-8271",
		ExecutionID:        "exec-original-8271",
		DetectedAt:         fixedTime,
		Summary:            "Timeout-driven retry committed two ledger effects for checkout-8271.",
		EvidenceEventIDs:   []string{"evt-timeout", "evt-retry", "evt-ledger-1", "evt-ledger-2"},
		GraphID:            "graph-8271",
		SanitizationStatus: contracts.SanitizationPass,
	}
}

func TestExtractFixtures_ProducesValidStateAndDependencyFixtures(t *testing.T) {
	p := New()
	fixtures, err := p.ExtractFixtures(context.Background(), goldenIncident(), goldenEvidence())
	if err != nil {
		t.Fatalf("ExtractFixtures: %v", err)
	}
	if len(fixtures.StateFixtures) == 0 {
		t.Fatalf("expected at least one state fixture")
	}
	if len(fixtures.DependencyFixtures) == 0 {
		t.Fatalf("expected at least one dependency fixture")
	}

	state := fixtures.StateFixtures[0]
	if state.Kind != contracts.StateFixturePostgresRowset {
		t.Fatalf("state fixture kind = %s, want POSTGRES_ROWSET", state.Kind)
	}
	if state.SanitizationStatus != contracts.SanitizationPass {
		t.Fatalf("state fixture sanitization_status = %s, want PASS", state.SanitizationStatus)
	}
	if state.ResetStrategy != contracts.FixtureTruncateAndLoad {
		t.Fatalf("state fixture reset_strategy = %s, want TRUNCATE_AND_LOAD", state.ResetStrategy)
	}
	if len(state.ContentDigest) != 64 {
		t.Fatalf("state fixture content_digest = %q, want 64 hex chars", state.ContentDigest)
	}

	dep := fixtures.DependencyFixtures[0]
	if dep.Dependency != contracts.DependencyPaymentSimulator {
		t.Fatalf("dependency fixture dependency = %s, want payment_simulator", dep.Dependency)
	}
	if dep.LatencyMS != 350 {
		t.Fatalf("dependency fixture latency_ms = %d, want 350 (derived from evidence)", dep.LatencyMS)
	}
	if dep.FailureMode != contracts.FailureModeNone {
		t.Fatalf("dependency fixture failure_mode = %s, want NONE", dep.FailureMode)
	}
	if dep.InvocationLimit != 2 {
		t.Fatalf("dependency fixture invocation_limit = %d, want 2", dep.InvocationLimit)
	}
}

func TestExtractFixtures_DeterministicForSameInput(t *testing.T) {
	p := New()
	first, err := p.ExtractFixtures(context.Background(), goldenIncident(), goldenEvidence())
	if err != nil {
		t.Fatalf("ExtractFixtures: %v", err)
	}
	second, err := p.ExtractFixtures(context.Background(), goldenIncident(), goldenEvidence())
	if err != nil {
		t.Fatalf("ExtractFixtures: %v", err)
	}
	if first.StateFixtures[0].ContentDigest != second.StateFixtures[0].ContentDigest {
		t.Fatalf("expected deterministic content digest")
	}
	if first.DependencyFixtures[0].FixtureID != second.DependencyFixtures[0].FixtureID {
		t.Fatalf("expected deterministic fixture id")
	}
}

func TestBuildReplayPlan_MatchesFrozenGoldenPlan(t *testing.T) {
	p := New()
	fixtures, err := p.ExtractFixtures(context.Background(), goldenIncident(), goldenEvidence())
	if err != nil {
		t.Fatalf("ExtractFixtures: %v", err)
	}
	plan, err := p.BuildReplayPlan(context.Background(), goldenIncident(), fixtures)
	if err != nil {
		t.Fatalf("BuildReplayPlan: %v", err)
	}
	if plan.Entrypoint != "gateway.checkout" {
		t.Fatalf("Entrypoint = %q, want gateway.checkout", plan.Entrypoint)
	}
	want := []string{"gateway", "checkout", "payment", "ledger"}
	if len(plan.RequiredComponents) != len(want) {
		t.Fatalf("RequiredComponents = %v, want %v", plan.RequiredComponents, want)
	}
	for i, c := range want {
		if plan.RequiredComponents[i] != c {
			t.Fatalf("RequiredComponents[%d] = %q, want %q", i, plan.RequiredComponents[i], c)
		}
	}
	if plan.ResetStrategy != contracts.ReplayResetGoldenV1 {
		t.Fatalf("ResetStrategy = %s, want GOLDEN_RESET_V1", plan.ResetStrategy)
	}

	allIDs := map[string]bool{}
	for _, f := range fixtures.StateFixtures {
		allIDs[f.FixtureID] = true
	}
	for _, f := range fixtures.DependencyFixtures {
		allIDs[f.FixtureID] = true
	}
	if plan.FixtureLoadOrder == nil {
		t.Fatalf("FixtureLoadOrder must be non-nil")
	}
	if len(plan.FixtureLoadOrder) != len(allIDs) {
		t.Fatalf("FixtureLoadOrder has %d entries, want exactly %d (one per fixture)", len(plan.FixtureLoadOrder), len(allIDs))
	}
	for _, id := range plan.FixtureLoadOrder {
		if !allIDs[id] {
			t.Fatalf("FixtureLoadOrder references unknown fixture %q", id)
		}
	}
}
