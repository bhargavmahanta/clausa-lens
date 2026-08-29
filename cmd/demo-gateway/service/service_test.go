package service

import (
	"context"
	"testing"

	checkoutsvc "github.com/causalens/causalens/cmd/demo-checkout/service"
	"github.com/causalens/causalens/internal/capture"
	"github.com/causalens/causalens/internal/contracts"
)

type stubCheckout struct {
	lastReq checkoutsvc.Request
	result  checkoutsvc.Result
	err     error
}

func (s *stubCheckout) Process(ctx context.Context, req checkoutsvc.Request) (checkoutsvc.Result, error) {
	s.lastReq = req
	return s.result, s.err
}

func TestService_Checkout_UsesProvidedCheckoutID(t *testing.T) {
	sink := capture.NewInMemorySink()
	recorder := capture.NewRecorder(contracts.ComponentRef{Name: "gateway", Instance: "gateway-1"}, capture.NewIDGenerator(1), sink)
	ids := capture.NewIDGenerator(capture.DefaultCheckoutSeed)
	checkout := &stubCheckout{result: checkoutsvc.Result{Attempts: 2}}
	svc := New(checkout, ids, recorder)

	result, err := svc.Checkout(context.Background(), Request{CheckoutID: "8271"})
	if err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if result.LogicalOperationID != "checkout-8271" {
		t.Fatalf("LogicalOperationID = %q, want checkout-8271", result.LogicalOperationID)
	}
	if result.TraceID != "trace-8271" {
		t.Fatalf("TraceID = %q, want trace-8271", result.TraceID)
	}
	if result.Attempts != 2 {
		t.Fatalf("Attempts = %d, want 2", result.Attempts)
	}
	if checkout.lastReq.LogicalOperationID != result.LogicalOperationID {
		t.Fatalf("checkout did not receive the same logical_operation_id gateway reported")
	}

	events := sink.Events()
	if len(events) != 2 {
		t.Fatalf("expected START and COMPLETE entrypoint events, got %d", len(events))
	}
	for _, ev := range events {
		if err := ev.Validate(); err != nil {
			t.Fatalf("captured event fails contract validation: %v", err)
		}
	}
	if events[0].Operation.Kind != contracts.OperationEntrypoint {
		t.Fatalf("expected ENTRYPOINT operation kind, got %s", events[0].Operation.Kind)
	}
	if events[0].ParentEventID != "" {
		t.Fatalf("root event must have no parent_event_id, got %q", events[0].ParentEventID)
	}
}

func TestService_Checkout_AllocatesDeterministicIDWhenAbsent(t *testing.T) {
	sink := capture.NewInMemorySink()
	recorder := capture.NewRecorder(contracts.ComponentRef{Name: "gateway", Instance: "gateway-1"}, capture.NewIDGenerator(1), sink)
	ids := capture.NewIDGenerator(capture.DefaultCheckoutSeed)
	checkout := &stubCheckout{result: checkoutsvc.Result{Attempts: 1}}
	svc := New(checkout, ids, recorder)

	before := svc.NextLogicalOperationID()
	result, err := svc.Checkout(context.Background(), Request{})
	if err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if result.LogicalOperationID != before {
		t.Fatalf("LogicalOperationID = %q, want deterministic allocation %q", result.LogicalOperationID, before)
	}

	svc.Reset()
	if got := svc.NextLogicalOperationID(); got != before {
		t.Fatalf("after Reset, NextLogicalOperationID() = %q, want %q", got, before)
	}
}
