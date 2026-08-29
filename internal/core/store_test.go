package core

import (
	"github.com/causalens/causalens/internal/contracts"
	"testing"
)

func TestStoreRejectsDuplicateEventIDs(t *testing.T) {
	s := NewStore()
	e := contracts.ExecutionEvent{SchemaVersion: "1.0", EventID: "e", ExecutionID: "x", TraceID: "t", Component: contracts.ComponentRef{Name: "c", Instance: "i"}, Operation: contracts.OperationRef{Name: "o", Kind: "INTERNAL"}, EventType: "START", Attempt: 1, LogicalOperationID: "l", OccurredAt: "2026-08-29T10:32:01Z", Status: "RUNNING", Attributes: map[string]any{}}
	if err := s.IngestEvent(e); err != nil {
		t.Fatal(err)
	}
	if err := s.IngestEvent(e); err == nil {
		t.Fatal("duplicate event ID should fail")
	}
}
