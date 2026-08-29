package checkout

import (
	"context"
	"testing"

	"github.com/causalens/causalens/internal/contracts"
)

func TestAllowedInterventions_ExactlyOnePaymentLatencySpec(t *testing.T) {
	p := New()
	specs := p.AllowedInterventions()
	if len(specs) != 1 {
		t.Fatalf("AllowedInterventions() has %d entries, want 1", len(specs))
	}
	want := contracts.InterventionSpec{
		Type: contracts.InterventionPaymentLatency, ValueType: contracts.InterventionValueInteger,
		Unit: contracts.InterventionUnitMilliseconds, Minimum: 0, Maximum: 5000,
	}
	if specs[0] != want {
		t.Fatalf("AllowedInterventions()[0] = %+v, want %+v", specs[0], want)
	}
}

func TestApplyIntervention_ReturnsACopyNotTheSamePlan(t *testing.T) {
	p := New()
	original := contracts.ReplayPlan{
		Entrypoint: "gateway.checkout", RequiredComponents: []string{"gateway", "checkout", "payment", "ledger"},
		FixtureLoadOrder: []string{"state-ledger-empty", "dependency-payment-350ms"}, ResetStrategy: contracts.ReplayResetGoldenV1,
	}
	intervention := contracts.Intervention{Type: contracts.InterventionPaymentLatency, From: 350, To: 50, Unit: contracts.InterventionUnitMilliseconds}

	applied, err := p.ApplyIntervention(context.Background(), original, intervention)
	if err != nil {
		t.Fatalf("ApplyIntervention: %v", err)
	}
	if applied.Entrypoint != original.Entrypoint || applied.ResetStrategy != original.ResetStrategy {
		t.Fatalf("applied plan structurally diverged from original: %+v vs %+v", applied, original)
	}

	// Mutating the returned plan's slice must not affect the original --
	// ApplyIntervention must copy, not mutate, the capsule-derived plan.
	applied.RequiredComponents[0] = "mutated"
	if original.RequiredComponents[0] == "mutated" {
		t.Fatalf("ApplyIntervention mutated the original plan's RequiredComponents slice")
	}
	applied.FixtureLoadOrder[0] = "mutated"
	if original.FixtureLoadOrder[0] == "mutated" {
		t.Fatalf("ApplyIntervention mutated the original plan's FixtureLoadOrder slice")
	}
}

func TestApplyIntervention_RejectsUnsupportedType(t *testing.T) {
	p := New()
	plan := contracts.ReplayPlan{Entrypoint: "gateway.checkout", RequiredComponents: []string{"gateway", "checkout", "payment", "ledger"}, FixtureLoadOrder: []string{}, ResetStrategy: contracts.ReplayResetGoldenV1}
	_, err := p.ApplyIntervention(context.Background(), plan, contracts.Intervention{Type: "SOMETHING_ELSE", From: 1, To: 2, Unit: contracts.InterventionUnitMilliseconds})
	if err == nil {
		t.Fatalf("expected an error for an intervention type outside AllowedInterventions")
	}
}

func TestApplyIntervention_RejectsOutOfRangeValue(t *testing.T) {
	p := New()
	plan := contracts.ReplayPlan{Entrypoint: "gateway.checkout", RequiredComponents: []string{"gateway", "checkout", "payment", "ledger"}, FixtureLoadOrder: []string{}, ResetStrategy: contracts.ReplayResetGoldenV1}
	_, err := p.ApplyIntervention(context.Background(), plan, contracts.Intervention{Type: contracts.InterventionPaymentLatency, From: 350, To: 5001, Unit: contracts.InterventionUnitMilliseconds})
	if err == nil {
		t.Fatalf("expected an error for a value above the allowed maximum")
	}
}
