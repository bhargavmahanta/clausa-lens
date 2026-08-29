package checkout

import (
	"context"
	"testing"

	"github.com/causalens/causalens/internal/contracts"
)

func realEvent(component, operation string, opKind contracts.OperationKind, eventType contracts.EventType, status contracts.EventStatus, attempt, seq int, eventID string, attrs map[string]any) contracts.ExecutionEvent {
	if attrs == nil {
		attrs = map[string]any{}
	}
	return contracts.ExecutionEvent{
		SchemaVersion:      contracts.ContractVersion,
		EventID:            eventID,
		ExecutionID:        "exec-1",
		TraceID:            "trace-8271",
		Component:          contracts.ComponentRef{Name: component, Instance: component + "-1"},
		Operation:          contracts.OperationRef{Name: operation, Kind: opKind},
		EventType:          eventType,
		Attempt:            attempt,
		LogicalOperationID: "checkout-8271",
		OccurredAt:         fixedTime,
		Sequence:           seq,
		Status:             status,
		Attributes:         attrs,
	}
}

func baselineCompareEvents() []contracts.ExecutionEvent {
	return []contracts.ExecutionEvent{
		realEvent("checkout", "checkout.process", contracts.OperationInternal, contracts.EventStart, contracts.EventRunning, 1, 0, "evt-b-checkout-start", nil),
		realEvent("payment", "authorize", contracts.OperationDependency, contracts.EventStart, contracts.EventRunning, 1, 0, "evt-b-pay-1-start", map[string]any{"configured_latency_ms": 350}),
		realEvent("checkout", "checkout.process", contracts.OperationControl, contracts.EventTimeout, contracts.EventTimedOut, 1, 1, "evt-b-timeout", nil),
		realEvent("checkout", "checkout.process", contracts.OperationControl, contracts.EventRetry, contracts.EventRunning, 2, 2, "evt-b-retry", nil),
		realEvent("payment", "authorize", contracts.OperationDependency, contracts.EventStart, contracts.EventRunning, 2, 1, "evt-b-pay-2-start", map[string]any{"configured_latency_ms": 350}),
		realEvent("ledger", "ledger.commit", contracts.OperationSideEffect, contracts.EventEffect, contracts.EventSuccess, 1, 0, "evt-b-effect-1", map[string]any{"effect_id": "8271-1", "effect_committed": true}),
		realEvent("ledger", "ledger.commit", contracts.OperationSideEffect, contracts.EventEffect, contracts.EventSuccess, 2, 1, "evt-b-effect-2", map[string]any{"effect_id": "8271-2", "effect_committed": true}),
	}
}

func whatIfCompareEvents() []contracts.ExecutionEvent {
	return []contracts.ExecutionEvent{
		realEvent("checkout", "checkout.process", contracts.OperationInternal, contracts.EventStart, contracts.EventRunning, 1, 0, "evt-w-checkout-start", nil),
		realEvent("payment", "authorize", contracts.OperationDependency, contracts.EventStart, contracts.EventRunning, 1, 0, "evt-w-pay-1-start", map[string]any{"configured_latency_ms": 50}),
		realEvent("ledger", "ledger.commit", contracts.OperationSideEffect, contracts.EventEffect, contracts.EventSuccess, 1, 0, "evt-w-effect-1", map[string]any{"effect_id": "8271-1", "effect_committed": true}),
	}
}

func TestAlign_MatchesByComponentOperationTypeLogicalOpAttempt(t *testing.T) {
	p := New()
	matched, added, removed, changed, err := p.Align(context.Background(), "diff-1",
		contracts.ReplayExecution{Events: baselineCompareEvents()},
		contracts.ReplayExecution{Events: whatIfCompareEvents()})
	if err != nil {
		t.Fatalf("Align: %v", err)
	}
	if len(matched) != 3 {
		t.Fatalf("matched = %v, want 3 pairs", matched)
	}
	if len(added) != 0 {
		t.Fatalf("added = %v, want none", added)
	}
	if len(removed) != 4 {
		t.Fatalf("removed = %v, want 4 (timeout, retry, payment attempt 2, effect-2)", removed)
	}
	if len(changed) != 0 {
		t.Fatalf("changed = %v, want none (statuses match)", changed)
	}

	wantRemoved := map[string]bool{"evt-b-timeout": true, "evt-b-retry": true, "evt-b-pay-2-start": true, "evt-b-effect-2": true}
	for _, id := range removed {
		if !wantRemoved[id] {
			t.Fatalf("unexpected removed event %q", id)
		}
	}
}

func TestAlign_DetectsStatusChangeOnMatchedPair(t *testing.T) {
	p := New()
	baseline := []contracts.ExecutionEvent{
		realEvent("payment", "authorize", contracts.OperationDependency, contracts.EventComplete, contracts.EventSuccess, 1, 0, "evt-b-pay-complete", nil),
	}
	comparison := []contracts.ExecutionEvent{
		realEvent("payment", "authorize", contracts.OperationDependency, contracts.EventComplete, contracts.EventFailed, 1, 0, "evt-w-pay-complete", nil),
	}
	_, _, _, changed, err := p.Align(context.Background(), "diff-1",
		contracts.ReplayExecution{Events: baseline}, contracts.ReplayExecution{Events: comparison})
	if err != nil {
		t.Fatalf("Align: %v", err)
	}
	if len(changed) != 1 {
		t.Fatalf("changed = %v, want 1 status change", changed)
	}
	if changed[0].Field != "status" {
		t.Fatalf("changed field = %q, want status", changed[0].Field)
	}
}

func TestCompare_BuildsValidReplayDiffForGoldenWhatIf(t *testing.T) {
	p := New()
	diff, err := p.Compare(context.Background(), "diff-8271",
		contracts.ReplayExecution{
			Run:    contracts.ReplayRun{RunID: "run-base-8271"},
			Events: baselineCompareEvents(),
		},
		contracts.ReplayExecution{
			Run: contracts.ReplayRun{RunID: "run-whatif-8271", Intervention: &contracts.Intervention{
				Type: contracts.InterventionPaymentLatency, From: 350, To: 50, Unit: contracts.InterventionUnitMilliseconds,
			}},
			Events: whatIfCompareEvents(),
		})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if diff.DiffID != "diff-8271" {
		t.Fatalf("DiffID = %q, want diff-8271", diff.DiffID)
	}
	if diff.BaselineRunID != "run-base-8271" || diff.ComparisonRunID != "run-whatif-8271" {
		t.Fatalf("run ids = %q/%q", diff.BaselineRunID, diff.ComparisonRunID)
	}
	if !diff.BaselineOracleResult.Matched {
		t.Fatalf("expected baseline oracle to match")
	}
	if diff.ComparisonOracleResult.Matched {
		t.Fatalf("expected comparison (what-if) oracle to not match")
	}
	if diff.EffectDelta.PaymentAttemptCount >= 0 || diff.EffectDelta.LedgerCommitCount >= 0 {
		t.Fatalf("expected negative effect delta for a mitigated what-if, got %+v", diff.EffectDelta)
	}
	if err := diff.Validate(); err != nil {
		t.Fatalf("assembled ReplayDiff fails contract validation: %v", err)
	}
}
