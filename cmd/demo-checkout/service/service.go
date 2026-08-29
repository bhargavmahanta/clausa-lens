// Package service implements the demo Checkout: it owns the 200 ms
// checkout timeout and the maximum-two-attempts retry policy from
// docs/DEMO_SCENARIO.md. When payment latency exceeds the timeout, Checkout
// starts a retry but does not cancel the in-flight first attempt -- both
// attempts run to completion and both commit a ledger effect. That
// uncancelled first attempt is the golden scenario's duplicate-effect bug.
package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	ledgersvc "github.com/causalens/causalens/cmd/demo-ledger/service"
	paymentsvc "github.com/causalens/causalens/cmd/demo-payment/service"
	"github.com/causalens/causalens/internal/capture"
	"github.com/causalens/causalens/internal/contracts"
)

// Default fault configuration, matching the P0 fixed values: checkout
// timeout 200 ms, maximum 2 attempts.
const (
	DefaultTimeoutMs   = 200
	DefaultMaxAttempts = 2
)

// PaymentClient is the payment dependency Checkout calls. The demo binary
// wires this to an HTTP client (cmd/demo-payment/service.Client); tests may
// substitute a stub.
type PaymentClient interface {
	Authorize(ctx context.Context, req paymentsvc.AuthorizeRequest) (paymentsvc.AuthorizeResult, error)
}

// LedgerClient is the ledger dependency Checkout calls after a payment
// attempt approves. The demo binary wires this to an HTTP client
// (cmd/demo-ledger/service.Client); tests may substitute a stub.
type LedgerClient interface {
	Commit(ctx context.Context, req ledgersvc.CommitRequest) (ledgersvc.CommitResult, error)
}

// Request starts one checkout for a logical operation already allocated by
// the Gateway.
type Request struct {
	ExecutionID        string
	TraceID            string
	LogicalOperationID string
	ParentEventID      string
	CheckoutID         string
}

// Result reports how many payment attempts this checkout launched.
type Result struct {
	Attempts int
}

// Service is the demo Checkout. It is safe for concurrent use.
type Service struct {
	mu          sync.Mutex
	timeoutMs   int
	maxAttempts int

	payment  PaymentClient
	ledger   LedgerClient
	recorder *capture.Recorder

	attemptsObserved int64
}

// New returns a Checkout with the P0 default fault configuration: 200 ms
// timeout, maximum 2 attempts.
func New(payment PaymentClient, ledger LedgerClient, recorder *capture.Recorder) *Service {
	s := &Service{payment: payment, ledger: ledger, recorder: recorder}
	s.Reset()
	return s
}

// Reset restores the default timeout and max-attempts policy and clears the
// observed-attempt counter, matching the reset contract in CONTRACTS.md.
func (s *Service) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timeoutMs = DefaultTimeoutMs
	s.maxAttempts = DefaultMaxAttempts
	atomic.StoreInt64(&s.attemptsObserved, 0)
}

// AttemptsObserved returns the number of payment attempts launched since the
// last Reset.
func (s *Service) AttemptsObserved() int {
	return int(atomic.LoadInt64(&s.attemptsObserved))
}

func (s *Service) config() (timeoutMs, maxAttempts int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.timeoutMs, s.maxAttempts
}

type attemptOutcome struct {
	attempt int
	err     error
}

// launchAttempt calls payment then ledger for one attempt and reports the
// outcome on done. It never cancels early: once launched, an attempt always
// runs to completion, which is what allows a timed-out first attempt to
// still land a duplicate ledger effect.
func (s *Service) launchAttempt(ctx context.Context, req Request, parentEventID string, attempt int, done chan<- attemptOutcome) {
	atomic.AddInt64(&s.attemptsObserved, 1)
	go func() {
		_, err := s.payment.Authorize(ctx, paymentsvc.AuthorizeRequest{
			ExecutionID:        req.ExecutionID,
			TraceID:            req.TraceID,
			LogicalOperationID: req.LogicalOperationID,
			ParentEventID:      parentEventID,
			Attempt:            attempt,
		})
		if err != nil {
			done <- attemptOutcome{attempt: attempt, err: fmt.Errorf("checkout: payment attempt %d: %w", attempt, err)}
			return
		}
		_, err = s.ledger.Commit(ctx, ledgersvc.CommitRequest{
			ExecutionID:        req.ExecutionID,
			TraceID:            req.TraceID,
			LogicalOperationID: req.LogicalOperationID,
			ParentEventID:      parentEventID,
			CheckoutID:         req.CheckoutID,
			Attempt:            attempt,
		})
		if err != nil {
			err = fmt.Errorf("checkout: ledger commit attempt %d: %w", attempt, err)
		}
		done <- attemptOutcome{attempt: attempt, err: err}
	}()
}

// Process runs the golden checkout scenario: attempt 1, a 200 ms timeout
// race, an uncancelled retry on timeout, and a wait for every launched
// attempt to finish before responding.
func (s *Service) Process(ctx context.Context, req Request) (Result, error) {
	timeoutMs, maxAttempts := s.config()

	start, err := s.recorder.Record(ctx, capture.RecordInput{
		ExecutionID:        req.ExecutionID,
		TraceID:            req.TraceID,
		LogicalOperationID: req.LogicalOperationID,
		ParentEventID:      req.ParentEventID,
		Attempt:            1,
		Operation:          contracts.OperationRef{Name: "checkout.process", Kind: contracts.OperationInternal},
		EventType:          contracts.EventStart,
		Status:             contracts.EventRunning,
		Attributes:         map[string]any{"checkout_timeout_ms": timeoutMs},
	})
	if err != nil {
		return Result{}, fmt.Errorf("checkout: record start event: %w", err)
	}

	done := make(chan attemptOutcome, maxAttempts)
	s.launchAttempt(ctx, req, start.EventID, 1, done)
	attemptsLaunched := 1

	timer := time.NewTimer(time.Duration(timeoutMs) * time.Millisecond)
	defer timer.Stop()

	var outcomes []attemptOutcome

	select {
	case out := <-done:
		// Attempt 1 completed before the timeout: no retry needed.
		outcomes = append(outcomes, out)
	case <-timer.C:
		if _, err := s.recorder.Record(ctx, capture.RecordInput{
			ExecutionID:        req.ExecutionID,
			TraceID:            req.TraceID,
			LogicalOperationID: req.LogicalOperationID,
			ParentEventID:      start.EventID,
			Attempt:            1,
			Operation:          contracts.OperationRef{Name: "checkout.process", Kind: contracts.OperationControl},
			EventType:          contracts.EventTimeout,
			Status:             contracts.EventTimedOut,
			Attributes:         map[string]any{"checkout_timeout_ms": timeoutMs},
		}); err != nil {
			return Result{}, fmt.Errorf("checkout: record timeout event: %w", err)
		}

		if maxAttempts > 1 {
			if _, err := s.recorder.Record(ctx, capture.RecordInput{
				ExecutionID:        req.ExecutionID,
				TraceID:            req.TraceID,
				LogicalOperationID: req.LogicalOperationID,
				ParentEventID:      start.EventID,
				Attempt:            2,
				Operation:          contracts.OperationRef{Name: "checkout.process", Kind: contracts.OperationControl},
				EventType:          contracts.EventRetry,
				Status:             contracts.EventRunning,
			}); err != nil {
				return Result{}, fmt.Errorf("checkout: record retry event: %w", err)
			}
			// The retry does not cancel attempt 1's goroutine above; it keeps
			// running and will still call payment and ledger.
			s.launchAttempt(ctx, req, start.EventID, 2, done)
			attemptsLaunched = 2
		}
	}

	for len(outcomes) < attemptsLaunched {
		outcomes = append(outcomes, <-done)
	}
	for _, out := range outcomes {
		if out.err != nil {
			return Result{}, out.err
		}
	}

	if _, err := s.recorder.Record(ctx, capture.RecordInput{
		ExecutionID:        req.ExecutionID,
		TraceID:            req.TraceID,
		LogicalOperationID: req.LogicalOperationID,
		ParentEventID:      start.EventID,
		Attempt:            attemptsLaunched,
		Operation:          contracts.OperationRef{Name: "checkout.process", Kind: contracts.OperationInternal},
		EventType:          contracts.EventComplete,
		Status:             contracts.EventSuccess,
	}); err != nil {
		return Result{}, fmt.Errorf("checkout: record complete event: %w", err)
	}

	return Result{Attempts: attemptsLaunched}, nil
}
