package packregistry

import (
	"context"
	"testing"

	"github.com/causalens/causalens/internal/capsule"
	"github.com/causalens/causalens/internal/contracts"
)

func devIncident() contracts.Incident {
	return contracts.Incident{
		SchemaVersion:      contracts.ContractVersion,
		IncidentID:         "inc-1",
		Status:             contracts.IncidentReady,
		FailureOracle:      contracts.FailureOracleRef{ID: "duplicate_ledger_effect", Version: "1.0.0"},
		SystemPack:         contracts.SystemPackRef{ID: "checkout_duplicate_effect", Version: "1.0.0", InterfaceVersion: contracts.ContractVersion},
		TraceID:            "trace-1",
		ExecutionID:        "exec-1",
		DetectedAt:         "2026-08-29T10:32:01.561Z",
		Summary:            "duplicate effect",
		EvidenceEventIDs:   []string{"e1"},
		GraphID:            "graph-1",
		SanitizationStatus: contracts.SanitizationPass,
	}
}

func devEvents() []contracts.ExecutionEvent {
	return []contracts.ExecutionEvent{{
		SchemaVersion: contracts.ContractVersion, EventID: "e1", ExecutionID: "exec-1", TraceID: "trace-1",
		Component: contracts.ComponentRef{Name: "payment", Instance: "i"}, Operation: contracts.OperationRef{Name: "authorize", Kind: contracts.OperationDependency},
		EventType: contracts.EventStart, Attempt: 1, LogicalOperationID: "checkout-1",
		OccurredAt: "2026-08-29T10:32:01Z", Sequence: 0, Status: contracts.EventRunning, Attributes: map[string]any{"configured_latency_ms": 350},
	}}
}

func TestDevPackDescriptorIsHonest(t *testing.T) {
	d := NewDevPack().Descriptor()
	if d.ID == "checkout_duplicate_effect" && d.Version == "1.0.0" {
		t.Fatal("dev pack must not masquerade as the real checkout pack id/version")
	}
	if d.ID != "checkout_duplicate_effect_dev" || d.Version != "0.0.0-dev" {
		t.Fatalf("unexpected dev descriptor: %+v", d)
	}
}

func TestDevPackCompilesValidCapsule(t *testing.T) {
	pack := NewDevPack()
	incident := devIncident()
	events := devEvents()
	source := contracts.CapsuleSource{IncidentID: incident.IncidentID, TraceID: incident.TraceID, ExecutionID: incident.ExecutionID, CaptureEnvironment: contracts.CaptureDemo, CapturedAt: incident.DetectedAt}
	trigger := contracts.Trigger{RequestOrMessage: map[string]any{"method": "POST"}, SanitizedHeaders: map[string]string{"content-type": "application/json"}}

	created, err := capsule.Compile(pack, incident.SystemPack, incident, events, source, trigger, "cap-1", "graph-1", "2026-08-29T10:33:00Z")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if err := created.Validate(); err != nil {
		t.Fatalf("capsule fails Validate: %v", err)
	}
	if !capsule.VerifyDigest(created) {
		t.Fatal("capsule digest mismatch")
	}
	if len(created.StateFixtures) != 1 || len(created.DependencyFixtures) != 1 {
		t.Fatalf("fixtures: %d state, %d dep", len(created.StateFixtures), len(created.DependencyFixtures))
	}
}

func TestDevPackEvaluateOutcomePerRunType(t *testing.T) {
	pack := NewDevPack()
	ctx := context.Background()

	baseline := contracts.ReplayExecution{Run: contracts.ReplayRun{RunType: contracts.RunTypeBaseline}}
	res, err := pack.EvaluateOutcome(ctx, baseline)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Matched || res.EffectSummary != (contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2}) {
		t.Fatalf("baseline oracle: %+v", res)
	}

	whatIf := contracts.ReplayExecution{Run: contracts.ReplayRun{RunType: contracts.RunTypeWhatIf}}
	res, err = pack.EvaluateOutcome(ctx, whatIf)
	if err != nil {
		t.Fatal(err)
	}
	if res.Matched || res.EffectSummary != (contracts.EffectSummary{PaymentAttemptCount: 1, LedgerCommitCount: 1}) {
		t.Fatalf("what-if oracle: %+v", res)
	}
}

func TestDevPackAlignProducesValidDiffSets(t *testing.T) {
	pack := NewDevPack()
	baseEvents := []contracts.ExecutionEvent{devEvent("be1", "FAILED", 350, 0), devEvent("be2", "TIMEOUT", 350, 1)}
	compEvents := []contracts.ExecutionEvent{devEvent("ce1", "SUCCESS", 50, 0)}
	baseline := contracts.ReplayExecution{Run: contracts.ReplayRun{RunID: "run-base", RunType: contracts.RunTypeBaseline, Outcome: contracts.ReplayOutcomeReproduced}, Events: baseEvents}
	comparison := contracts.ReplayExecution{Run: contracts.ReplayRun{RunID: "run-comp", RunType: contracts.RunTypeWhatIf, BaselineRunID: "run-base", Outcome: contracts.ReplayOutcomeMitigated, Intervention: &contracts.Intervention{Type: contracts.InterventionPaymentLatency, From: 350, To: 50, Unit: contracts.InterventionUnitMilliseconds}}, Events: compEvents}

	matched, added, removed, changes, err := pack.Align(context.Background(), "diff-1", baseline, comparison)
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != 1 || matched[0].BaselineEventID != "be1" || matched[0].ComparisonEventID != "ce1" {
		t.Fatalf("matched: %+v", matched)
	}
	if len(removed) != 1 || removed[0] != "be2" {
		t.Fatalf("removed: %+v", removed)
	}
	if len(added) != 0 {
		t.Fatalf("added: %+v", added)
	}
	if len(changes) == 0 || changes[0].Field != "status" {
		t.Fatalf("changes: %+v", changes)
	}

	diff, err := pack.Compare(context.Background(), "diff-1", baseline, comparison)
	if err != nil {
		t.Fatal(err)
	}
	if err := diff.Validate(); err != nil {
		t.Fatalf("compare diff fails Validate: %v", err)
	}
}

func TestDevPackNormalizeUnsupported(t *testing.T) {
	if _, err := NewDevPack().Normalize(context.Background(), contracts.RawEvidence{}); err == nil {
		t.Fatal("Normalize should be unsupported for the dev pack")
	}
}

func devEvent(id, status string, latencyMS, sequence int) contracts.ExecutionEvent {
	return contracts.ExecutionEvent{
		SchemaVersion: contracts.ContractVersion, EventID: id, ExecutionID: "exec", TraceID: "trace",
		Component: contracts.ComponentRef{Name: "payment", Instance: "i"}, Operation: contracts.OperationRef{Name: "authorize", Kind: contracts.OperationDependency},
		EventType: contracts.EventComplete, Attempt: 1, LogicalOperationID: "checkout-1",
		OccurredAt: "2026-08-29T10:32:01Z", Sequence: sequence, Status: contracts.EventStatus(status),
		DurationMS: intPtr(1), Attributes: map[string]any{"configured_latency_ms": latencyMS},
	}
}

func intPtr(v int) *int { return &v }
