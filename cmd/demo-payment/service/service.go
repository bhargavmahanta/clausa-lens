// Package service implements the demo Payment dependency simulator: it
// applies a configurable, controlled latency per attempt and always
// approves. Real payment providers are out of P0 scope.
package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/causalens/causalens/internal/capture"
	"github.com/causalens/causalens/internal/contracts"
)

// DefaultLatencyMs matches the frozen P0 default fault configuration.
const DefaultLatencyMs = 350

// AuthorizeRequest carries the correlation identifiers and attempt number
// for one payment attempt.
type AuthorizeRequest struct {
	ExecutionID        string
	TraceID            string
	LogicalOperationID string
	ParentEventID      string
	Attempt            int
}

// AuthorizeResult is the simulator's response. Status is always APPROVED in
// P0; the simulator does not model declines.
type AuthorizeResult struct {
	Status string
}

// Service is the demo Payment simulator. It is safe for concurrent use.
type Service struct {
	mu        sync.Mutex
	latencyMs int
	attempts  int

	recorder *capture.Recorder
	// sleep is overridable in tests; production code always uses the real
	// clock via time.Sleep.
	sleep func(time.Duration)
}

// New returns a Payment simulator with the P0 default fault configuration
// (350 ms configured latency).
func New(recorder *capture.Recorder) *Service {
	s := &Service{recorder: recorder, sleep: time.Sleep}
	s.Reset()
	return s
}

// Reset restores the default configured latency and clears the attempt
// counter, matching the reset contract in CONTRACTS.md.
func (s *Service) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latencyMs = DefaultLatencyMs
	s.attempts = 0
}

// SetLatencyMs sets the configured per-attempt latency. Used by the checkout
// System Pack's PAYMENT_LATENCY intervention.
func (s *Service) SetLatencyMs(ms int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latencyMs = ms
}

// LatencyMs returns the currently configured latency.
func (s *Service) LatencyMs() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latencyMs
}

// AttemptCount returns the number of Authorize calls observed since the last
// Reset.
func (s *Service) AttemptCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attempts
}

// Authorize applies the configured, controlled latency and reports
// approval. The delay is a fixed configured duration, never a random
// jitter, so replay stays deterministic.
func (s *Service) Authorize(ctx context.Context, req AuthorizeRequest) (AuthorizeResult, error) {
	if req.Attempt < 1 {
		return AuthorizeResult{}, fmt.Errorf("payment: attempt must be >= 1, got %d", req.Attempt)
	}

	s.mu.Lock()
	latency := s.latencyMs
	s.attempts++
	s.mu.Unlock()

	if s.recorder != nil {
		if _, err := s.recorder.Record(ctx, capture.RecordInput{
			ExecutionID:        req.ExecutionID,
			TraceID:            req.TraceID,
			LogicalOperationID: req.LogicalOperationID,
			ParentEventID:      req.ParentEventID,
			Attempt:            req.Attempt,
			Operation:          contracts.OperationRef{Name: "authorize", Kind: contracts.OperationDependency},
			EventType:          contracts.EventStart,
			Status:             contracts.EventRunning,
			Attributes:         map[string]any{"configured_latency_ms": latency},
		}); err != nil {
			return AuthorizeResult{}, fmt.Errorf("payment: record start event: %w", err)
		}
	}

	s.sleep(time.Duration(latency) * time.Millisecond)

	if s.recorder != nil {
		duration := latency
		if _, err := s.recorder.Record(ctx, capture.RecordInput{
			ExecutionID:        req.ExecutionID,
			TraceID:            req.TraceID,
			LogicalOperationID: req.LogicalOperationID,
			ParentEventID:      req.ParentEventID,
			Attempt:            req.Attempt,
			Operation:          contracts.OperationRef{Name: "authorize", Kind: contracts.OperationDependency},
			EventType:          contracts.EventComplete,
			Status:             contracts.EventSuccess,
			DurationMs:         &duration,
		}); err != nil {
			return AuthorizeResult{}, fmt.Errorf("payment: record complete event: %w", err)
		}
	}

	return AuthorizeResult{Status: "APPROVED"}, nil
}
