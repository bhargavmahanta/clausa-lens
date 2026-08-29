package checkout

import (
	"context"
	"testing"

	"github.com/causalens/causalens/internal/contracts"
)

var fixedTime = "2026-08-29T10:32:01.000Z"

func ev(eventType contracts.EventType, attempt, seq int, eventID string, opKind contracts.OperationKind, attrs map[string]any) contracts.ExecutionEvent {
	if attrs == nil {
		attrs = map[string]any{}
	}
	return contracts.ExecutionEvent{
		SchemaVersion:      contracts.ContractVersion,
		EventID:            eventID,
		ExecutionID:        "exec-1",
		TraceID:            "trace-1",
		Component:          contracts.ComponentRef{Name: "x", Instance: "x-1"},
		Operation:          contracts.OperationRef{Name: "op", Kind: opKind},
		EventType:          eventType,
		Attempt:            attempt,
		LogicalOperationID: "checkout-8271",
		OccurredAt:         fixedTime,
		Sequence:           seq,
		Status:             contracts.EventRunning,
		Attributes:         attrs,
	}
}

func goldenEvidence() []contracts.ExecutionEvent {
	return []contracts.ExecutionEvent{
		ev(contracts.EventStart, 1, 0, "evt-payment-1-start", contracts.OperationDependency, map[string]any{"configured_latency_ms": 350}),
		ev(contracts.EventTimeout, 1, 1, "evt-timeout", contracts.OperationControl, map[string]any{"checkout_timeout_ms": 200}),
		ev(contracts.EventRetry, 2, 2, "evt-retry", contracts.OperationControl, nil),
		ev(contracts.EventStart, 2, 3, "evt-payment-2-start", contracts.OperationDependency, map[string]any{"configured_latency_ms": 350}),
		ev(contracts.EventEffect, 1, 4, "evt-ledger-1", contracts.OperationSideEffect, map[string]any{"effect_id": "8271-1", "effect_committed": true}),
		ev(contracts.EventEffect, 2, 5, "evt-ledger-2", contracts.OperationSideEffect, map[string]any{"effect_id": "8271-2", "effect_committed": true}),
	}
}

func TestDetectIncident_MatchesGoldenEvidence(t *testing.T) {
	p := New()
	result, err := p.DetectIncident(context.Background(), goldenEvidence())
	if err != nil {
		t.Fatalf("DetectIncident: %v", err)
	}
	if !result.Matched {
		t.Fatalf("expected match, got %+v", result)
	}
	if result.Oracle.ID != "duplicate_ledger_effect" || result.Oracle.Version != "1.0.0" {
		t.Fatalf("unexpected oracle ref %+v", result.Oracle)
	}
	if result.EffectSummary.PaymentAttemptCount != 2 || result.EffectSummary.LedgerCommitCount != 2 {
		t.Fatalf("EffectSummary = %+v, want {2 2}", result.EffectSummary)
	}
	want := []string{"evt-timeout", "evt-retry", "evt-ledger-1", "evt-ledger-2"}
	if len(result.RequiredEvidenceEventIDs) != len(want) {
		t.Fatalf("RequiredEvidenceEventIDs = %v, want %v", result.RequiredEvidenceEventIDs, want)
	}
	for i, id := range want {
		if result.RequiredEvidenceEventIDs[i] != id {
			t.Fatalf("RequiredEvidenceEventIDs[%d] = %q, want %q", i, result.RequiredEvidenceEventIDs[i], id)
		}
	}
}

func TestDetectIncident_MissingTimeoutDoesNotMatch(t *testing.T) {
	p := New()
	events := []contracts.ExecutionEvent{
		ev(contracts.EventStart, 1, 0, "evt-payment-1-start", contracts.OperationDependency, nil),
		ev(contracts.EventEffect, 1, 1, "evt-ledger-1", contracts.OperationSideEffect, map[string]any{"effect_id": "8271-1", "effect_committed": true}),
	}
	result, err := p.DetectIncident(context.Background(), events)
	if err != nil {
		t.Fatalf("DetectIncident: %v", err)
	}
	if result.Matched {
		t.Fatalf("expected no match without a timeout event, got %+v", result)
	}
	if len(result.RequiredEvidenceEventIDs) == 0 {
		t.Fatalf("required_evidence_event_ids must be non-empty even for a non-match")
	}
}

func TestDetectIncident_SingleEffectDoesNotMatch(t *testing.T) {
	p := New()
	events := goldenEvidence()[:5] // drop the second committed ledger effect
	result, err := p.DetectIncident(context.Background(), events)
	if err != nil {
		t.Fatalf("DetectIncident: %v", err)
	}
	if result.Matched {
		t.Fatalf("expected no match with only one committed effect, got %+v", result)
	}
	if result.EffectSummary.LedgerCommitCount != 1 {
		t.Fatalf("LedgerCommitCount = %d, want 1", result.EffectSummary.LedgerCommitCount)
	}
}

func TestDetectIncident_NoEventsIsAnError(t *testing.T) {
	p := New()
	if _, err := p.DetectIncident(context.Background(), nil); err == nil {
		t.Fatalf("expected an error for empty evidence")
	}
}

func TestDetectIncident_DeterministicForSameInput(t *testing.T) {
	p := New()
	events := goldenEvidence()
	first, err := p.DetectIncident(context.Background(), events)
	if err != nil {
		t.Fatalf("DetectIncident: %v", err)
	}
	second, err := p.DetectIncident(context.Background(), events)
	if err != nil {
		t.Fatalf("DetectIncident: %v", err)
	}
	if first.Matched != second.Matched || first.Explanation != second.Explanation {
		t.Fatalf("expected deterministic output, got %+v vs %+v", first, second)
	}
}

// TestEvaluateOutcome_WhatIfShapeIsNotMatched covers the P0 what-if replay:
// payment completes before the timeout, so there is no timeout, no retry,
// and only one committed ledger effect.
func TestEvaluateOutcome_WhatIfShapeIsNotMatched(t *testing.T) {
	p := New()
	events := []contracts.ExecutionEvent{
		ev(contracts.EventStart, 1, 0, "evt-whatif-payment-start", contracts.OperationDependency, map[string]any{"configured_latency_ms": 50}),
		ev(contracts.EventComplete, 1, 1, "evt-whatif-payment-complete", contracts.OperationDependency, nil),
		ev(contracts.EventEffect, 1, 2, "evt-whatif-ledger-1", contracts.OperationSideEffect, map[string]any{"effect_id": "8271-1", "effect_committed": true}),
	}
	result, err := p.EvaluateOutcome(context.Background(), contracts.ReplayExecution{Events: events})
	if err != nil {
		t.Fatalf("EvaluateOutcome: %v", err)
	}
	if result.Matched {
		t.Fatalf("expected no match for the what-if shape, got %+v", result)
	}
	if result.EffectSummary.PaymentAttemptCount != 1 || result.EffectSummary.LedgerCommitCount != 1 {
		t.Fatalf("EffectSummary = %+v, want {1 1}", result.EffectSummary)
	}
}

func TestEvaluateOutcome_WorksWithZeroValueGraph(t *testing.T) {
	// replay.Evaluate calls EvaluateOutcome with only Run and Events set
	// (ReplayExecution.Graph stays zero-value); the pack must not depend on
	// Graph.
	p := New()
	_, err := p.EvaluateOutcome(context.Background(), contracts.ReplayExecution{
		Run:    contracts.ReplayRun{RunID: "run-1"},
		Events: goldenEvidence(),
	})
	if err != nil {
		t.Fatalf("EvaluateOutcome: %v", err)
	}
}
