// Package service implements the demo Ledger: it records committed effects
// for the golden checkout scenario using demo-scoped in-memory state, per
// docs/DEMO_SCENARIO.md. The ledger effect key is
// checkout_id + payment_attempt.
package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/causalens/causalens/internal/capture"
	"github.com/causalens/causalens/internal/contracts"
)

// DefaultDeduplicationEnabled matches the frozen P0 default fault
// configuration: deduplication disabled.
const DefaultDeduplicationEnabled = false

// CommitRequest is the ledger's commit request shape. CheckoutID is the
// business identifier the effect key is derived from; LogicalOperationID is
// the stable CausaLens identifier used for event correlation.
type CommitRequest struct {
	ExecutionID        string
	TraceID            string
	LogicalOperationID string
	ParentEventID      string
	CheckoutID         string
	Attempt            int
}

// CommitResult reports whether a distinct ledger effect was committed for
// this attempt.
type CommitResult struct {
	EffectID  string
	Committed bool
}

// Service is the demo Ledger. It is safe for concurrent use.
type Service struct {
	mu sync.Mutex

	dedupEnabled bool
	// effects holds every committed effect_id, in commit order.
	effects []string
	// firstEffectByCheckout tracks the first committed effect per
	// checkout_id; used only when deduplication is enabled.
	firstEffectByCheckout map[string]string

	recorder *capture.Recorder
}

// New returns a Ledger with the P0 default fault configuration
// (deduplication disabled) and empty state.
func New(recorder *capture.Recorder) *Service {
	s := &Service{recorder: recorder}
	s.Reset()
	return s
}

// Reset restores deterministic empty state and the default deduplication
// policy, matching the reset contract in CONTRACTS.md.
func (s *Service) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dedupEnabled = DefaultDeduplicationEnabled
	s.effects = nil
	s.firstEffectByCheckout = make(map[string]string)
}

// SetDeduplicationEnabled toggles the P1 deduplication intervention. The P0
// golden scenario always runs with deduplication disabled.
func (s *Service) SetDeduplicationEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dedupEnabled = enabled
}

// DeduplicationEnabled reports the current deduplication policy.
func (s *Service) DeduplicationEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dedupEnabled
}

// EffectKey derives the ledger effect key: checkout_id + payment_attempt.
func EffectKey(checkoutID string, attempt int) string {
	return fmt.Sprintf("%s-%d", checkoutID, attempt)
}

// Commit records one ledger effect for the given checkout attempt. When
// deduplication is enabled and the checkout_id already has a committed
// effect, the attempt is observed but not committed, and Committed is false.
func (s *Service) Commit(ctx context.Context, req CommitRequest) (CommitResult, error) {
	if req.Attempt < 1 {
		return CommitResult{}, fmt.Errorf("ledger: attempt must be >= 1, got %d", req.Attempt)
	}
	effectID := EffectKey(req.CheckoutID, req.Attempt)

	s.mu.Lock()
	committed := true
	if s.dedupEnabled {
		if _, exists := s.firstEffectByCheckout[req.CheckoutID]; exists {
			committed = false
		} else {
			s.firstEffectByCheckout[req.CheckoutID] = effectID
		}
	}
	if committed {
		s.effects = append(s.effects, effectID)
	}
	s.mu.Unlock()

	status := contracts.EventSuccess
	if !committed {
		status = contracts.EventBlocked
	}

	if s.recorder != nil {
		if _, err := s.recorder.Record(ctx, capture.RecordInput{
			ExecutionID:        req.ExecutionID,
			TraceID:            req.TraceID,
			LogicalOperationID: req.LogicalOperationID,
			ParentEventID:      req.ParentEventID,
			Attempt:            req.Attempt,
			Operation:          contracts.OperationRef{Name: "ledger.commit", Kind: contracts.OperationSideEffect},
			EventType:          contracts.EventEffect,
			Status:             status,
			Attributes: map[string]any{
				"effect_id":        effectID,
				"effect_committed": committed,
			},
		}); err != nil {
			return CommitResult{}, fmt.Errorf("ledger: record effect event: %w", err)
		}
	}

	return CommitResult{EffectID: effectID, Committed: committed}, nil
}

// CommittedEffectCount returns the number of ledger effects committed since
// the last Reset.
func (s *Service) CommittedEffectCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.effects)
}

// CommittedEffects returns a snapshot of every committed effect_id, in
// commit order.
func (s *Service) CommittedEffects() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.effects))
	copy(out, s.effects)
	return out
}
