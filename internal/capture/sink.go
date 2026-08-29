// Package capture emits canonical CausaLens ExecutionEvent records
// (docs/CONTRACTS.md #ExecutionEvent v1.0) for the demo Gateway-Checkout-
// Payment-Ledger system, using the frozen contracts.ExecutionEvent type
// directly rather than a parallel definition. Emission targets the live
// Core API's POST /v1/events route (docs/CONTRACTS.md API resource table).
package capture

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/causalens/causalens/internal/contracts"
)

// Sink accepts a validated ExecutionEvent for downstream storage. Emit must
// not mutate event.
type Sink interface {
	Emit(ctx context.Context, event contracts.ExecutionEvent) error
}

// InMemorySink records events in arrival order. Used by tests and by
// anything that needs to inspect emitted evidence directly.
type InMemorySink struct {
	mu     sync.Mutex
	events []contracts.ExecutionEvent
}

// NewInMemorySink returns an empty sink.
func NewInMemorySink() *InMemorySink { return &InMemorySink{} }

// Emit implements Sink.
func (s *InMemorySink) Emit(_ context.Context, event contracts.ExecutionEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

// Events returns a snapshot of every event recorded so far, in arrival order.
func (s *InMemorySink) Events() []contracts.ExecutionEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contracts.ExecutionEvent, len(s.events))
	copy(out, s.events)
	return out
}

// Reset discards every recorded event.
func (s *InMemorySink) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = nil
}

// MultiSink fans a single Emit out to every configured sink, returning the
// first error but still attempting every sink.
type MultiSink struct {
	Sinks []Sink
}

// Emit implements Sink.
func (m MultiSink) Emit(ctx context.Context, event contracts.ExecutionEvent) error {
	var firstErr error
	for _, s := range m.Sinks {
		if err := s.Emit(ctx, event); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// HTTPSink posts each event as JSON to the Core API's POST /v1/events
// route, matching AcceptedEventResponse on success.
type HTTPSink struct {
	URL    string
	Client *http.Client
}

// NewHTTPSink returns a sink that POSTs events as JSON to url
// (e.g. "http://core-api:8080/v1/events").
func NewHTTPSink(url string) *HTTPSink { return &HTTPSink{URL: url, Client: http.DefaultClient} }

// Emit implements Sink.
func (h *HTTPSink) Emit(ctx context.Context, event contracts.ExecutionEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("capture: marshal event: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("capture: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := h.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("capture: post event: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("capture: unexpected status %d posting event %s", resp.StatusCode, event.EventID)
	}
	return nil
}
