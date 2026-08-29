package service

import (
	"context"
	"testing"

	"github.com/causalens/causalens/internal/capture"
	"github.com/causalens/causalens/internal/contracts"
)

func newTestService(t *testing.T) (*Service, *capture.InMemorySink) {
	t.Helper()
	sink := capture.NewInMemorySink()
	recorder := capture.NewRecorder(contracts.ComponentRef{Name: "ledger", Instance: "ledger-1"}, capture.NewIDGenerator(1), sink)
	return New(recorder), sink
}

func TestService_CommitCreatesDistinctEffectsPerAttempt(t *testing.T) {
	svc, sink := newTestService(t)
	ctx := context.Background()

	r1, err := svc.Commit(ctx, CommitRequest{
		ExecutionID: "exec-1", TraceID: "trace-1", LogicalOperationID: "checkout-1",
		CheckoutID: "8271", Attempt: 1,
	})
	if err != nil {
		t.Fatalf("Commit attempt 1: %v", err)
	}
	r2, err := svc.Commit(ctx, CommitRequest{
		ExecutionID: "exec-1", TraceID: "trace-1", LogicalOperationID: "checkout-1",
		CheckoutID: "8271", Attempt: 2,
	})
	if err != nil {
		t.Fatalf("Commit attempt 2: %v", err)
	}

	if r1.EffectID == r2.EffectID {
		t.Fatalf("expected distinct effect ids, got %q for both attempts", r1.EffectID)
	}
	if !r1.Committed || !r2.Committed {
		t.Fatalf("expected both attempts committed with deduplication disabled, got %v and %v", r1.Committed, r2.Committed)
	}
	if got := svc.CommittedEffectCount(); got != 2 {
		t.Fatalf("CommittedEffectCount() = %d, want 2", got)
	}

	events := sink.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 captured events, got %d", len(events))
	}
	for _, ev := range events {
		if err := ev.Validate(); err != nil {
			t.Fatalf("captured event fails contract validation: %v", err)
		}
		if ev.EventType != contracts.EventEffect {
			t.Fatalf("expected EFFECT event_type, got %s", ev.EventType)
		}
		if ev.Attributes["effect_committed"] != true {
			t.Fatalf("expected effect_committed=true, got %v", ev.Attributes["effect_committed"])
		}
	}
}

func TestService_DeduplicationBlocksSecondEffect(t *testing.T) {
	svc, _ := newTestService(t)
	svc.SetDeduplicationEnabled(true)
	ctx := context.Background()

	r1, err := svc.Commit(ctx, CommitRequest{
		ExecutionID: "exec-1", TraceID: "trace-1", LogicalOperationID: "checkout-1",
		CheckoutID: "8271", Attempt: 1,
	})
	if err != nil {
		t.Fatalf("Commit attempt 1: %v", err)
	}
	r2, err := svc.Commit(ctx, CommitRequest{
		ExecutionID: "exec-1", TraceID: "trace-1", LogicalOperationID: "checkout-1",
		CheckoutID: "8271", Attempt: 2,
	})
	if err != nil {
		t.Fatalf("Commit attempt 2: %v", err)
	}
	if !r1.Committed || r2.Committed {
		t.Fatalf("expected first attempt committed and second blocked, got %v and %v", r1.Committed, r2.Committed)
	}
	if got := svc.CommittedEffectCount(); got != 1 {
		t.Fatalf("CommittedEffectCount() = %d, want 1", got)
	}
}

func TestService_ResetRestoresDefaultsAndClearsState(t *testing.T) {
	svc, _ := newTestService(t)
	svc.SetDeduplicationEnabled(true)
	ctx := context.Background()
	if _, err := svc.Commit(ctx, CommitRequest{
		ExecutionID: "exec-1", TraceID: "trace-1", LogicalOperationID: "checkout-1",
		CheckoutID: "8271", Attempt: 1,
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	svc.Reset()

	if svc.DeduplicationEnabled() != DefaultDeduplicationEnabled {
		t.Fatalf("Reset did not restore default deduplication policy")
	}
	if got := svc.CommittedEffectCount(); got != 0 {
		t.Fatalf("Reset did not clear state, count = %d", got)
	}
}

func TestEffectKey_IsCheckoutIDPlusAttempt(t *testing.T) {
	if got := EffectKey("8271", 2); got != "8271-2" {
		t.Fatalf("EffectKey(8271, 2) = %q, want %q", got, "8271-2")
	}
}
