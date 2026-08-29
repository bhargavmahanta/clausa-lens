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

func TestStorePersistsIncident(t *testing.T) {
	s := NewStore()
	i := contracts.Incident{SchemaVersion: "1.0", IncidentID: "inc-1", Status: "READY", GraphID: "g-1"}
	g := contracts.ExecutionGraph{GraphID: "g-1", SchemaVersion: "1.0"}
	if err := s.PutIncident(i, g); err != nil {
		t.Fatal(err)
	}
	got, graph, ok := s.Incident("inc-1")
	if !ok || got.IncidentID != "inc-1" || graph.GraphID != "g-1" {
		t.Fatal("incident detail not returned")
	}
}
