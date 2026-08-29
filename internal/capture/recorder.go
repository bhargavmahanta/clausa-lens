package capture

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/causalens/causalens/internal/contracts"
)

// clock is overridable in tests that need deterministic timestamps.
// Production code always uses time.Now.
var clock = time.Now

// RecordInput carries everything a caller needs to supply for one
// ExecutionEvent; Recorder fills in schema_version, event_id, sequence, and
// occurred_at.
type RecordInput struct {
	ExecutionID        string
	TraceID            string
	ParentEventID      string
	ReplayRunID        string
	Operation          contracts.OperationRef
	EventType          contracts.EventType
	Attempt            int
	LogicalOperationID string
	DurationMs         *int
	Status             contracts.EventStatus
	Attributes         map[string]any
}

// Recorder emits canonical ExecutionEvent records for one component
// instance. Sequence numbers are monotonic per (execution_id,
// component.instance) pair, matching CONTRACTS.md: "sequence is monotonic
// within one component instance and execution, not globally."
type Recorder struct {
	Component contracts.ComponentRef
	IDs       *IDGenerator
	Sink      Sink

	mu  sync.Mutex
	seq map[string]int
}

// NewRecorder returns a Recorder for one component instance. ids and sink
// are typically shared across every service in the process so event_id
// stays globally unique and evidence lands in one place.
func NewRecorder(component contracts.ComponentRef, ids *IDGenerator, sink Sink) *Recorder {
	return &Recorder{Component: component, IDs: ids, Sink: sink, seq: make(map[string]int)}
}

func (r *Recorder) nextSequence(executionID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := executionID + "|" + r.Component.Instance
	n := r.seq[key]
	r.seq[key] = n + 1
	return n
}

// Record builds, validates (against the frozen contracts.ExecutionEvent
// rules), and emits one ExecutionEvent. It returns the built event so
// callers can chain parent_event_id for subsequent events.
func (r *Recorder) Record(ctx context.Context, in RecordInput) (contracts.ExecutionEvent, error) {
	if in.Attempt < 1 {
		return contracts.ExecutionEvent{}, fmt.Errorf("capture: attempt must be >= 1, got %d", in.Attempt)
	}
	attrs := in.Attributes
	if attrs == nil {
		attrs = map[string]any{}
	}
	event := contracts.ExecutionEvent{
		SchemaVersion:      contracts.ContractVersion,
		EventID:            r.IDs.NextEventID(r.Component.Name),
		ExecutionID:        in.ExecutionID,
		TraceID:            in.TraceID,
		ParentEventID:      in.ParentEventID,
		ReplayRunID:        in.ReplayRunID,
		Component:          r.Component,
		Operation:          in.Operation,
		EventType:          in.EventType,
		Attempt:            in.Attempt,
		LogicalOperationID: in.LogicalOperationID,
		OccurredAt:         clock().UTC().Format(time.RFC3339Nano),
		Sequence:           r.nextSequence(in.ExecutionID),
		DurationMS:         in.DurationMs,
		Status:             in.Status,
		Attributes:         attrs,
	}
	if err := event.Validate(); err != nil {
		return contracts.ExecutionEvent{}, fmt.Errorf("capture: invalid event: %w", err)
	}
	if r.Sink != nil {
		if err := r.Sink.Emit(ctx, event); err != nil {
			return contracts.ExecutionEvent{}, fmt.Errorf("capture: emit event: %w", err)
		}
	}
	return event, nil
}
