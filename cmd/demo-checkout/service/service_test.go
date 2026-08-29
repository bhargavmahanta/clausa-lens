package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	ledgersvc "github.com/causalens/causalens/cmd/demo-ledger/service"
	paymentsvc "github.com/causalens/causalens/cmd/demo-payment/service"
	"github.com/causalens/causalens/internal/capture"
	"github.com/causalens/causalens/internal/contracts"
)

// stubPayment simulates the payment dependency with a fixed, controlled
// delay per call -- never a random jitter -- so tests stay deterministic.
type stubPayment struct {
	mu    sync.Mutex
	delay time.Duration
	calls []int
}

func (s *stubPayment) Authorize(ctx context.Context, req paymentsvc.AuthorizeRequest) (paymentsvc.AuthorizeResult, error) {
	s.mu.Lock()
	s.calls = append(s.calls, req.Attempt)
	s.mu.Unlock()
	time.Sleep(s.delay)
	return paymentsvc.AuthorizeResult{Status: "APPROVED"}, nil
}

func (s *stubPayment) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

// stubLedger simulates the ledger, committing one distinct effect per
// attempt (deduplication disabled), matching the P0 default policy.
type stubLedger struct {
	mu      sync.Mutex
	commits []string
}

func (s *stubLedger) Commit(ctx context.Context, req ledgersvc.CommitRequest) (ledgersvc.CommitResult, error) {
	effectID := fmt.Sprintf("%s-%d", req.CheckoutID, req.Attempt)
	s.mu.Lock()
	s.commits = append(s.commits, effectID)
	s.mu.Unlock()
	return ledgersvc.CommitResult{EffectID: effectID, Committed: true}, nil
}

func (s *stubLedger) commitCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.commits)
}

func newTestService(t *testing.T, paymentDelay time.Duration, timeoutMs int) (*Service, *stubPayment, *stubLedger, *capture.InMemorySink) {
	t.Helper()
	payment := &stubPayment{delay: paymentDelay}
	ledger := &stubLedger{}
	sink := capture.NewInMemorySink()
	recorder := capture.NewRecorder(contracts.ComponentRef{Name: "checkout", Instance: "checkout-1"}, capture.NewIDGenerator(1), sink)
	svc := New(payment, ledger, recorder)
	svc.timeoutMs = timeoutMs
	return svc, payment, ledger, sink
}

func TestService_DefaultConfigMatchesFrozenFaultConfiguration(t *testing.T) {
	if DefaultTimeoutMs != 200 {
		t.Fatalf("DefaultTimeoutMs = %d, want 200 (P0 fixed value)", DefaultTimeoutMs)
	}
	if DefaultMaxAttempts != 2 {
		t.Fatalf("DefaultMaxAttempts = %d, want 2 (P0 fixed value)", DefaultMaxAttempts)
	}
}

// TestService_PaymentFasterThanTimeout_SingleAttempt covers the P0 what-if
// shape: when payment completes before the timeout, checkout makes exactly
// one attempt and one ledger commit, with no timeout or retry event.
func TestService_PaymentFasterThanTimeout_SingleAttempt(t *testing.T) {
	svc, payment, ledger, sink := newTestService(t, 5*time.Millisecond, 40)
	ctx := context.Background()

	result, err := svc.Process(ctx, Request{
		ExecutionID: "exec-1", TraceID: "trace-1", LogicalOperationID: "checkout-1", CheckoutID: "8271",
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if result.Attempts != 1 {
		t.Fatalf("Attempts = %d, want 1", result.Attempts)
	}
	if got := payment.callCount(); got != 1 {
		t.Fatalf("payment call count = %d, want 1", got)
	}
	if got := ledger.commitCount(); got != 1 {
		t.Fatalf("ledger commit count = %d, want 1", got)
	}
	for _, ev := range sink.Events() {
		if ev.EventType == contracts.EventTimeout || ev.EventType == contracts.EventRetry {
			t.Fatalf("unexpected %s event when payment completed before the timeout", ev.EventType)
		}
	}
}

// TestService_PaymentSlowerThanTimeout_TwoAttemptsTwoCommits covers the
// golden baseline shape: checkout crosses its timeout, retries without
// cancelling attempt 1, and both attempts land a ledger effect.
func TestService_PaymentSlowerThanTimeout_TwoAttemptsTwoCommits(t *testing.T) {
	svc, payment, ledger, sink := newTestService(t, 60*time.Millisecond, 20)
	ctx := context.Background()

	result, err := svc.Process(ctx, Request{
		ExecutionID: "exec-1", TraceID: "trace-1", LogicalOperationID: "checkout-1", CheckoutID: "8271",
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if result.Attempts != 2 {
		t.Fatalf("Attempts = %d, want 2", result.Attempts)
	}
	if got := payment.callCount(); got != 2 {
		t.Fatalf("exactly two payment attempts expected, got %d", got)
	}
	if got := ledger.commitCount(); got != 2 {
		t.Fatalf("exactly two ledger commits expected, got %d", got)
	}
	if got := svc.AttemptsObserved(); got != 2 {
		t.Fatalf("AttemptsObserved() = %d, want 2", got)
	}

	events := sink.Events()
	var sawTimeout, sawRetry bool
	var attempts []int
	traceIDs := map[string]bool{}
	logicalIDs := map[string]bool{}
	for _, ev := range events {
		if err := ev.Validate(); err != nil {
			t.Fatalf("captured event fails contract validation: %v", err)
		}
		traceIDs[ev.TraceID] = true
		logicalIDs[ev.LogicalOperationID] = true
		attempts = append(attempts, ev.Attempt)
		switch ev.EventType {
		case contracts.EventTimeout:
			sawTimeout = true
			if ev.Status != contracts.EventTimedOut {
				t.Fatalf("timeout event status = %s, want TIMEOUT", ev.Status)
			}
		case contracts.EventRetry:
			sawRetry = true
			if ev.Attempt != 2 {
				t.Fatalf("retry event attempt = %d, want 2", ev.Attempt)
			}
		}
	}
	if !sawTimeout {
		t.Fatalf("expected a TIMEOUT event")
	}
	if !sawRetry {
		t.Fatalf("expected a RETRY event")
	}
	if len(traceIDs) != 1 {
		t.Fatalf("trace_id was not stable across attempts: %v", traceIDs)
	}
	if len(logicalIDs) != 1 {
		t.Fatalf("logical_operation_id was not stable across attempts: %v", logicalIDs)
	}
	var sawAttempt1, sawAttempt2 bool
	for _, a := range attempts {
		if a == 1 {
			sawAttempt1 = true
		}
		if a == 2 {
			sawAttempt2 = true
		}
		if a < 1 || a > 2 {
			t.Fatalf("unexpected attempt value %d", a)
		}
	}
	if !sawAttempt1 || !sawAttempt2 {
		t.Fatalf("expected both attempt 1 and attempt 2 events, attempts seen: %v", attempts)
	}
}

func TestService_ResetClearsAttemptsObserved(t *testing.T) {
	svc, _, _, _ := newTestService(t, 5*time.Millisecond, 40)
	ctx := context.Background()
	if _, err := svc.Process(ctx, Request{
		ExecutionID: "exec-1", TraceID: "trace-1", LogicalOperationID: "checkout-1", CheckoutID: "8271",
	}); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if svc.AttemptsObserved() == 0 {
		t.Fatalf("expected at least one observed attempt before reset")
	}
	svc.Reset()
	if got := svc.AttemptsObserved(); got != 0 {
		t.Fatalf("Reset did not clear attempts observed, got %d", got)
	}
	if got, _ := svc.config(); got != DefaultTimeoutMs {
		t.Fatalf("Reset did not restore default timeout, got %d", got)
	}
}
