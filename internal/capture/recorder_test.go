package capture

import (
	"context"
	"testing"

	"github.com/causalens/causalens/internal/contracts"
)

func newTestRecorder(component, instance string, sink Sink) *Recorder {
	return NewRecorder(contracts.ComponentRef{Name: component, Instance: instance}, NewIDGenerator(1), sink)
}

func TestRecorder_EmitsValidContractEvent(t *testing.T) {
	sink := NewInMemorySink()
	r := newTestRecorder("payment", "payment-1", sink)

	ev, err := r.Record(context.Background(), RecordInput{
		ExecutionID: "exec-1", TraceID: "trace-1", LogicalOperationID: "checkout-1",
		Attempt: 1, Operation: contracts.OperationRef{Name: "authorize", Kind: contracts.OperationDependency},
		EventType: contracts.EventStart, Status: contracts.EventRunning,
		Attributes: map[string]any{"configured_latency_ms": 350},
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := ev.Validate(); err != nil {
		t.Fatalf("emitted event fails contracts.ExecutionEvent.Validate(): %v", err)
	}
	if len(sink.Events()) != 1 {
		t.Fatalf("expected 1 event in sink, got %d", len(sink.Events()))
	}
}

func TestRecorder_GloballyUniqueEventIDAcrossComponents(t *testing.T) {
	sink := NewInMemorySink()
	ids := NewIDGenerator(1)
	gateway := NewRecorder(contracts.ComponentRef{Name: "gateway", Instance: "gateway-1"}, ids, sink)
	checkout := NewRecorder(contracts.ComponentRef{Name: "checkout", Instance: "checkout-1"}, ids, sink)

	seen := map[string]bool{}
	for i := 0; i < 5; i++ {
		for _, r := range []*Recorder{gateway, checkout} {
			ev, err := r.Record(context.Background(), RecordInput{
				ExecutionID: "exec-1", TraceID: "trace-1", LogicalOperationID: "checkout-1",
				Attempt: 1, Operation: contracts.OperationRef{Name: "op", Kind: contracts.OperationInternal},
				EventType: contracts.EventStart, Status: contracts.EventRunning,
			})
			if err != nil {
				t.Fatalf("Record: %v", err)
			}
			if seen[ev.EventID] {
				t.Fatalf("duplicate event_id %s", ev.EventID)
			}
			seen[ev.EventID] = true
		}
	}
}

func TestRecorder_AttemptBelowOneRejected(t *testing.T) {
	sink := NewInMemorySink()
	r := newTestRecorder("checkout", "checkout-1", sink)
	_, err := r.Record(context.Background(), RecordInput{
		ExecutionID: "exec-1", TraceID: "trace-1", LogicalOperationID: "checkout-1",
		Attempt: 0, Operation: contracts.OperationRef{Name: "checkout.process", Kind: contracts.OperationInternal},
		EventType: contracts.EventStart, Status: contracts.EventRunning,
	})
	if err == nil {
		t.Fatalf("expected attempt=0 to be rejected")
	}
}

func TestRecorder_SequenceMonotonicPerComponentInstanceAndExecution(t *testing.T) {
	sink := NewInMemorySink()
	r := newTestRecorder("checkout", "checkout-1", sink)
	ctx := context.Background()

	var last = -1
	for i := 0; i < 3; i++ {
		ev, err := r.Record(ctx, RecordInput{
			ExecutionID: "exec-1", TraceID: "trace-1", LogicalOperationID: "checkout-1",
			Attempt: 1, Operation: contracts.OperationRef{Name: "checkout.process", Kind: contracts.OperationInternal},
			EventType: contracts.EventStart, Status: contracts.EventRunning,
		})
		if err != nil {
			t.Fatalf("Record: %v", err)
		}
		if ev.Sequence <= last {
			t.Fatalf("sequence not monotonic: %d after %d", ev.Sequence, last)
		}
		last = ev.Sequence
	}

	other, err := r.Record(ctx, RecordInput{
		ExecutionID: "exec-2", TraceID: "trace-2", LogicalOperationID: "checkout-2",
		Attempt: 1, Operation: contracts.OperationRef{Name: "checkout.process", Kind: contracts.OperationInternal},
		EventType: contracts.EventStart, Status: contracts.EventRunning,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if other.Sequence != 0 {
		t.Fatalf("expected sequence 0 for a new execution, got %d", other.Sequence)
	}
}

func TestRecorder_AttributeAllowListRejectsUnlistedKeys(t *testing.T) {
	sink := NewInMemorySink()
	r := newTestRecorder("payment", "payment-1", sink)
	_, err := r.Record(context.Background(), RecordInput{
		ExecutionID: "exec-1", TraceID: "trace-1", LogicalOperationID: "checkout-1",
		Attempt: 1, Operation: contracts.OperationRef{Name: "authorize", Kind: contracts.OperationDependency},
		EventType: contracts.EventStart, Status: contracts.EventRunning,
		Attributes: map[string]any{"card_number": "4111111111111111"},
	})
	if err == nil {
		t.Fatalf("expected non-allow-listed attribute to be rejected")
	}
}
