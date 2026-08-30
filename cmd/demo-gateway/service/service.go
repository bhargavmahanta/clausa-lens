// Package service implements the demo Gateway: the ENTRYPOINT for the
// golden checkout scenario. It allocates (or accepts) the trace_id,
// execution_id, and logical_operation_id for one checkout and forwards the
// request to Checkout, per docs/DEMO_SCENARIO.md.
package service

import (
	"context"
	"fmt"
	"log"
	"strings"

	checkoutsvc "github.com/causalens/causalens/cmd/demo-checkout/service"
	"github.com/causalens/causalens/internal/capture"
	"github.com/causalens/causalens/internal/contracts"
)

// CheckoutClient is the checkout dependency Gateway calls. The demo binary
// wires this to an HTTP client (cmd/demo-checkout/service.Client); tests
// may substitute a stub.
type CheckoutClient interface {
	Process(ctx context.Context, req checkoutsvc.Request) (checkoutsvc.Result, error)
}

// Request is the inbound POST /checkout payload. CheckoutID is optional:
// when absent, Gateway allocates the next deterministic identifier from its
// IDGenerator, matching the reset contract's requirement that a fresh reset
// reports a deterministic next_logical_operation_id. Scenario selects the
// run shape: "" (or "faulted") runs the golden duplicate-effect scenario;
// "healthy" runs the control scenario whose payment completes before the
// checkout timeout, so the failure oracle must stay silent.
type Request struct {
	CheckoutID string
	Scenario   string
}

// Result reports the identifiers Gateway allocated for this checkout and how
// many payment attempts it ultimately took.
type Result struct {
	TraceID            string
	ExecutionID        string
	LogicalOperationID string
	Attempts           int
}

// ScenarioHealthy requests the healthy control run; any other value keeps
// the golden faulted behavior.
const ScenarioHealthy = "healthy"

// ControlCheckoutID is the fixed identifier for healthy-control runs. It is
// explicit so control runs never consume the deterministic golden seed and
// the faulted demo keeps reporting checkout-8271 after a reset.
const ControlCheckoutID = "ctrl-1"

// HealthyPaymentLatencyMs and GoldenPaymentLatencyMs are the per-attempt
// payment latencies for the control and golden scenarios. 50 ms completes
// before Checkout's 200 ms timeout; 350 ms is the frozen golden fault
// configuration (Payment's DefaultLatencyMs).
const (
	HealthyPaymentLatencyMs = 50
	GoldenPaymentLatencyMs  = 350
)

// PaymentLatencyController reconfigures the Payment simulator's configured
// latency for the healthy-control scenario. The demo binary wires this to
// the payment service's HTTP client; tests may substitute a stub.
type PaymentLatencyController interface {
	SetLatency(ctx context.Context, latencyMs int) error
}

// Service is the demo Gateway. It is safe for concurrent use.
type Service struct {
	checkout CheckoutClient
	ids      *capture.IDGenerator
	recorder *capture.Recorder
	payment  PaymentLatencyController
}

// New returns a Gateway that allocates identifiers from ids and forwards
// checkout requests through checkout.
func New(checkout CheckoutClient, ids *capture.IDGenerator, recorder *capture.Recorder) *Service {
	return &Service{checkout: checkout, ids: ids, recorder: recorder}
}

// WithPaymentLatencyControl attaches the Payment latency controller used by
// the healthy-control scenario.
func (s *Service) WithPaymentLatencyControl(payment PaymentLatencyController) *Service {
	s.payment = payment
	return s
}

// Reset restores deterministic identifier allocation, matching the reset
// contract in CONTRACTS.md.
func (s *Service) Reset() { s.ids.Reset() }

// NextLogicalOperationID reports the logical_operation_id the next unscoped
// checkout request will receive, matching
// ResetResult.next_logical_operation_id.
func (s *Service) NextLogicalOperationID() string { return s.ids.PeekNextLogicalOperationID() }

// Checkout runs one golden checkout request end to end: it allocates
// identifiers, records the ENTRYPOINT event, forwards to Checkout, and
// records the completion. Healthy-control requests temporarily lower the
// Payment latency and always restore the golden configuration afterwards.
func (s *Service) Checkout(ctx context.Context, req Request) (Result, error) {
	checkoutID := normalizeCheckoutID(req.CheckoutID)
	if checkoutID == "" && req.Scenario == ScenarioHealthy {
		checkoutID = ControlCheckoutID
	}
	if checkoutID == "" {
		seq := s.ids.NextCheckoutSeq()
		checkoutID = fmt.Sprintf("%d", seq)
	}
	traceID := "trace-" + checkoutID
	executionID := "exec-original-" + checkoutID
	logicalOperationID := "checkout-" + checkoutID

	if req.Scenario == ScenarioHealthy {
		if s.payment == nil {
			return Result{}, fmt.Errorf("gateway: healthy control requires a payment latency controller")
		}
		if err := s.payment.SetLatency(ctx, HealthyPaymentLatencyMs); err != nil {
			return Result{}, fmt.Errorf("gateway: healthy control: set payment latency: %w", err)
		}
		defer func() {
			if err := s.payment.SetLatency(ctx, GoldenPaymentLatencyMs); err != nil {
				log.Printf("gateway: healthy control: restore payment latency: %v", err)
			}
		}()
	}

	start, err := s.recorder.Record(ctx, capture.RecordInput{
		ExecutionID:        executionID,
		TraceID:            traceID,
		LogicalOperationID: logicalOperationID,
		Attempt:            1,
		Operation:          contracts.OperationRef{Name: "checkout", Kind: contracts.OperationEntrypoint},
		EventType:          contracts.EventStart,
		Status:             contracts.EventRunning,
	})
	if err != nil {
		return Result{}, fmt.Errorf("gateway: record entrypoint start event: %w", err)
	}

	checkoutResult, err := s.checkout.Process(ctx, checkoutsvc.Request{
		ExecutionID:        executionID,
		TraceID:            traceID,
		LogicalOperationID: logicalOperationID,
		ParentEventID:      start.EventID,
		CheckoutID:         checkoutID,
	})
	if err != nil {
		return Result{}, fmt.Errorf("gateway: checkout: %w", err)
	}

	if _, err := s.recorder.Record(ctx, capture.RecordInput{
		ExecutionID:        executionID,
		TraceID:            traceID,
		LogicalOperationID: logicalOperationID,
		ParentEventID:      start.EventID,
		Attempt:            checkoutResult.Attempts,
		Operation:          contracts.OperationRef{Name: "checkout", Kind: contracts.OperationEntrypoint},
		EventType:          contracts.EventComplete,
		Status:             contracts.EventSuccess,
	}); err != nil {
		return Result{}, fmt.Errorf("gateway: record entrypoint complete event: %w", err)
	}

	return Result{
		TraceID:            traceID,
		ExecutionID:        executionID,
		LogicalOperationID: logicalOperationID,
		Attempts:           checkoutResult.Attempts,
	}, nil
}

// normalizeCheckoutID canonicalizes a caller-supplied checkout_id to the bare
// suffix Gateway uses to derive trace_id, execution_id, and
// logical_operation_id. Callers may send either the raw suffix ("8271") or
// the frozen contract's logical_operation_id-shaped value ("checkout-8271");
// both must resolve to the same identity. An empty result (including an
// input that is exactly the "checkout-" prefix) falls through to
// deterministic allocation.
func normalizeCheckoutID(raw string) string {
	return strings.TrimPrefix(raw, "checkout-")
}
