package checkout

import (
	"context"
	"testing"

	"github.com/causalens/causalens/internal/capsule"
	"github.com/causalens/causalens/internal/contracts"
)

// TestPack_CompilesThroughRealCapsuleCompiler proves the pack works end to
// end against Bhargav's real, unmodified capsule.Compile -- not a
// reimplementation of his logic. A pack-produced capsule must pass his
// integrity, schema, and pack-validation gates.
func TestPack_CompilesThroughRealCapsuleCompiler(t *testing.T) {
	p := New()
	incident := goldenIncident()
	events := goldenEvidence()

	c, err := capsule.Compile(p, p.Descriptor(), incident, events,
		contracts.CapsuleSource{
			IncidentID: incident.IncidentID, TraceID: incident.TraceID, ExecutionID: incident.ExecutionID,
			CaptureEnvironment: contracts.CaptureDemo, CapturedAt: incident.DetectedAt,
		},
		contracts.Trigger{RequestOrMessage: map[string]any{}, SanitizedHeaders: map[string]string{}},
		"cap-8271", incident.GraphID, fixedTime,
	)
	if err != nil {
		t.Fatalf("capsule.Compile: %v", err)
	}

	if err := c.Validate(); err != nil {
		t.Fatalf("compiled capsule fails core contract validation: %v", err)
	}
	if !capsule.VerifyDigest(c) {
		t.Fatalf("compiled capsule integrity digest does not match its content")
	}
	if issues := p.ValidateCapsule(context.Background(), c); len(issues) != 0 {
		t.Fatalf("compiled capsule fails the pack's own validation: %+v", issues)
	}
	if c.ReplayPlan.Entrypoint != "gateway.checkout" {
		t.Fatalf("Entrypoint = %q, want gateway.checkout", c.ReplayPlan.Entrypoint)
	}
	if c.FailureOracle.ExpectedEffectSummary != (contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2}) {
		t.Fatalf("unexpected FailureOracle.ExpectedEffectSummary: %+v", c.FailureOracle.ExpectedEffectSummary)
	}
	if len(c.AllowedInterventions) != 1 || c.AllowedInterventions[0].Type != contracts.InterventionPaymentLatency {
		t.Fatalf("unexpected AllowedInterventions: %+v", c.AllowedInterventions)
	}
}

// TestPack_RecompilingFromTheSameEvidenceIsDeterministic proves that
// compiling the same incident and evidence twice produces the same
// integrity digest, matching "every method returns deterministic output for
// the same validated input" in CONTRACTS.md.
func TestPack_RecompilingFromTheSameEvidenceIsDeterministic(t *testing.T) {
	p := New()
	incident := goldenIncident()
	events := goldenEvidence()
	source := contracts.CapsuleSource{
		IncidentID: incident.IncidentID, TraceID: incident.TraceID, ExecutionID: incident.ExecutionID,
		CaptureEnvironment: contracts.CaptureDemo, CapturedAt: incident.DetectedAt,
	}
	trigger := contracts.Trigger{RequestOrMessage: map[string]any{}, SanitizedHeaders: map[string]string{}}

	first, err := capsule.Compile(p, p.Descriptor(), incident, events, source, trigger, "cap-8271", incident.GraphID, fixedTime)
	if err != nil {
		t.Fatalf("first Compile: %v", err)
	}
	second, err := capsule.Compile(p, p.Descriptor(), incident, events, source, trigger, "cap-8271", incident.GraphID, fixedTime)
	if err != nil {
		t.Fatalf("second Compile: %v", err)
	}
	if first.Integrity.Digest != second.Integrity.Digest {
		t.Fatalf("expected identical digests for identical input, got %q vs %q", first.Integrity.Digest, second.Integrity.Digest)
	}
}
