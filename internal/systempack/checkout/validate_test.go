package checkout

import (
	"context"
	"testing"

	"github.com/causalens/causalens/internal/contracts"
)

func goldenCapsuleFixtures() contracts.FixtureSet {
	return contracts.FixtureSet{
		StateFixtures: []contracts.StateFixture{{
			FixtureID: "state-ledger-empty", Kind: contracts.StateFixturePostgresRowset,
			ContentRef: "fixture://golden/ledger-empty-v1", ContentDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			SanitizationStatus: contracts.SanitizationPass, ResetStrategy: contracts.FixtureTruncateAndLoad,
		}},
		DependencyFixtures: []contracts.DependencyFixture{{
			FixtureID: "dependency-payment-350ms", Dependency: contracts.DependencyPaymentSimulator,
			RequestMatch: map[string]any{"logical_operation_id": "checkout-8271"}, Response: map[string]any{"status": "APPROVED"},
			LatencyMS: 350, FailureMode: contracts.FailureModeNone, InvocationLimit: 2,
		}},
	}
}

func goldenCapsule() contracts.ReplayCapsule {
	fixtures := goldenCapsuleFixtures()
	return contracts.ReplayCapsule{
		SchemaVersion: contracts.ContractVersion,
		CapsuleID:     "cap-8271",
		CreatedAt:     fixedTime,
		Source: contracts.CapsuleSource{
			IncidentID: "inc-8271", TraceID: "trace-8271", ExecutionID: "exec-original-8271",
			CaptureEnvironment: contracts.CaptureDemo, CapturedAt: fixedTime,
		},
		SystemPack:         New().Descriptor(),
		Trigger:            contracts.Trigger{RequestOrMessage: map[string]any{}, SanitizedHeaders: map[string]string{}},
		EventIDs:           []string{"evt-timeout", "evt-retry", "evt-ledger-1", "evt-ledger-2"},
		GraphID:            "graph-8271",
		StateFixtures:      fixtures.StateFixtures,
		DependencyFixtures: fixtures.DependencyFixtures,
		TimingPolicy:       contracts.TimingPolicy{ClockToleranceMS: 5, TimeoutMS: 200},
		ReplayPlan: contracts.ReplayPlan{
			Entrypoint: "gateway.checkout", RequiredComponents: []string{"gateway", "checkout", "payment", "ledger"},
			FixtureLoadOrder: []string{"state-ledger-empty", "dependency-payment-350ms"}, ResetStrategy: contracts.ReplayResetGoldenV1,
		},
		FailureOracle: contracts.FailureOracleSpec{
			ID: "duplicate_ledger_effect", Version: "1.0.0", ExpectedMatch: true,
			ExpectedEffectSummary: contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2},
		},
		AllowedInterventions: New().AllowedInterventions(),
		Safety: contracts.SafetyPolicy{
			PolicyVersion: contracts.ContractVersion, SanitizationStatus: contracts.SanitizationPass,
			BlockedDestinations: []string{"production-databases", "public-internet", "real-payment-provider"},
			AllowedDestinations: []string{"payment-simulator", "replay-postgres"}, CredentialProfile: contracts.CredentialReplayOnly,
		},
		Integrity: contracts.Integrity{Algorithm: contracts.IntegritySHA256, Digest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}
}

func TestValidateCapsule_GoldenCapsuleHasNoIssues(t *testing.T) {
	p := New()
	issues := p.ValidateCapsule(context.Background(), goldenCapsule())
	if len(issues) != 0 {
		t.Fatalf("expected no issues for a golden capsule, got %+v", issues)
	}
}

func TestValidateCapsule_RejectsWrongBaselineLatency(t *testing.T) {
	p := New()
	c := goldenCapsule()
	c.DependencyFixtures[0].LatencyMS = 999
	issues := p.ValidateCapsule(context.Background(), c)
	if len(issues) == 0 {
		t.Fatalf("expected an issue for a baseline capsule whose payment latency is not 350ms")
	}
}

func TestValidateCapsule_RejectsWrongInvocationLimit(t *testing.T) {
	p := New()
	c := goldenCapsule()
	c.DependencyFixtures[0].InvocationLimit = 99
	issues := p.ValidateCapsule(context.Background(), c)
	if len(issues) == 0 {
		t.Fatalf("expected an issue for a baseline capsule whose invocation_limit is not 2")
	}
}

func TestValidateCapsule_RejectsMissingLedgerStateFixture(t *testing.T) {
	p := New()
	c := goldenCapsule()
	c.StateFixtures = nil
	issues := p.ValidateCapsule(context.Background(), c)
	if len(issues) == 0 {
		t.Fatalf("expected an issue for a capsule missing the state-ledger-empty fixture")
	}
}
