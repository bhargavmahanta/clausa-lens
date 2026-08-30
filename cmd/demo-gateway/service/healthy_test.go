package service

import (
	"context"
	"errors"
	"testing"

	checkoutsvc "github.com/causalens/causalens/cmd/demo-checkout/service"
	"github.com/causalens/causalens/internal/capture"
	"github.com/causalens/causalens/internal/contracts"
)

type stubLatencyController struct {
	calls []int
	err   error
}

func (s *stubLatencyController) SetLatency(_ context.Context, latencyMs int) error {
	s.calls = append(s.calls, latencyMs)
	return s.err
}

func TestService_Checkout_HealthyControlDoesNotConsumeDeterministicSeed(t *testing.T) {
	sink := capture.NewInMemorySink()
	recorder := capture.NewRecorder(contracts.ComponentRef{Name: "gateway", Instance: "gateway-1"}, capture.NewIDGenerator(1), sink)
	ids := capture.NewIDGenerator(capture.DefaultCheckoutSeed)
	checkout := &stubCheckout{result: checkoutsvc.Result{Attempts: 1}}
	latency := &stubLatencyController{}
	svc := New(checkout, ids, recorder).WithPaymentLatencyControl(latency)

	result, err := svc.Checkout(context.Background(), Request{Scenario: ScenarioHealthy})
	if err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if result.LogicalOperationID != "checkout-"+ControlCheckoutID {
		t.Fatalf("LogicalOperationID = %q, want checkout-%s", result.LogicalOperationID, ControlCheckoutID)
	}
	if result.Attempts != 1 {
		t.Fatalf("Attempts = %d, want 1", result.Attempts)
	}
	if got := ids.PeekNextLogicalOperationID(); got != "checkout-8271" {
		t.Fatalf("healthy control consumed the golden seed: next = %q, want checkout-8271", got)
	}
}

func TestService_Checkout_HealthyControlSetsAndRestoresLatency(t *testing.T) {
	sink := capture.NewInMemorySink()
	recorder := capture.NewRecorder(contracts.ComponentRef{Name: "gateway", Instance: "gateway-1"}, capture.NewIDGenerator(1), sink)
	ids := capture.NewIDGenerator(capture.DefaultCheckoutSeed)
	checkout := &stubCheckout{result: checkoutsvc.Result{Attempts: 1}}
	latency := &stubLatencyController{}
	svc := New(checkout, ids, recorder).WithPaymentLatencyControl(latency)

	if _, err := svc.Checkout(context.Background(), Request{Scenario: ScenarioHealthy}); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if len(latency.calls) != 2 {
		t.Fatalf("expected set+restore latency calls, got %v", latency.calls)
	}
	if latency.calls[0] != HealthyPaymentLatencyMs {
		t.Fatalf("first latency call = %d, want %d", latency.calls[0], HealthyPaymentLatencyMs)
	}
	if latency.calls[1] != GoldenPaymentLatencyMs {
		t.Fatalf("restore latency call = %d, want %d", latency.calls[1], GoldenPaymentLatencyMs)
	}
}

func TestService_Checkout_FaultedScenarioNeverTouchesPaymentLatency(t *testing.T) {
	sink := capture.NewInMemorySink()
	recorder := capture.NewRecorder(contracts.ComponentRef{Name: "gateway", Instance: "gateway-1"}, capture.NewIDGenerator(1), sink)
	ids := capture.NewIDGenerator(capture.DefaultCheckoutSeed)
	checkout := &stubCheckout{result: checkoutsvc.Result{Attempts: 2}}
	latency := &stubLatencyController{}
	svc := New(checkout, ids, recorder).WithPaymentLatencyControl(latency)

	if _, err := svc.Checkout(context.Background(), Request{}); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if len(latency.calls) != 0 {
		t.Fatalf("faulted scenario must not touch payment latency, got %v", latency.calls)
	}
}

func TestService_Checkout_HealthyControlFailsClosedWhenLatencyUnsettable(t *testing.T) {
	sink := capture.NewInMemorySink()
	recorder := capture.NewRecorder(contracts.ComponentRef{Name: "gateway", Instance: "gateway-1"}, capture.NewIDGenerator(1), sink)
	ids := capture.NewIDGenerator(capture.DefaultCheckoutSeed)
	checkout := &stubCheckout{result: checkoutsvc.Result{Attempts: 1}}
	latency := &stubLatencyController{err: errors.New("payment unreachable")}
	svc := New(checkout, ids, recorder).WithPaymentLatencyControl(latency)

	if _, err := svc.Checkout(context.Background(), Request{Scenario: ScenarioHealthy}); err == nil {
		t.Fatalf("expected error when payment latency cannot be set, got nil")
	}
	if checkout.lastReq.LogicalOperationID != "" {
		t.Fatalf("checkout must not run when the control latency cannot be established")
	}
}

func TestService_Checkout_HealthyControlFailsClosedWithoutController(t *testing.T) {
	sink := capture.NewInMemorySink()
	recorder := capture.NewRecorder(contracts.ComponentRef{Name: "gateway", Instance: "gateway-1"}, capture.NewIDGenerator(1), sink)
	ids := capture.NewIDGenerator(capture.DefaultCheckoutSeed)
	checkout := &stubCheckout{result: checkoutsvc.Result{Attempts: 1}}
	svc := New(checkout, ids, recorder)

	if _, err := svc.Checkout(context.Background(), Request{Scenario: ScenarioHealthy}); err == nil {
		t.Fatalf("expected error when no payment latency controller is wired, got nil")
	}
}
