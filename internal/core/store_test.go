package core

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/causalens/causalens/internal/contracts"
)

func event(id string) contracts.ExecutionEvent {
	return contracts.ExecutionEvent{SchemaVersion: "1.0", EventID: id, ExecutionID: "exec", TraceID: "trace", Component: contracts.ComponentRef{Name: "c", Instance: "i"}, Operation: contracts.OperationRef{Name: "o", Kind: contracts.OperationInternal}, EventType: contracts.EventStart, Attempt: 1, LogicalOperationID: "logical", OccurredAt: "2026-08-29T10:32:01Z", Status: contracts.EventRunning, Attributes: map[string]any{}}
}

func incident(id, graphID, detected string, status contracts.IncidentStatus) contracts.Incident {
	return contracts.Incident{SchemaVersion: "1.0", IncidentID: id, Status: status, FailureOracle: contracts.FailureOracleRef{ID: "oracle", Version: "1.0.0"}, SystemPack: contracts.SystemPackRef{ID: "pack", Version: "1.0.0", InterfaceVersion: "1.0"}, TraceID: "trace", ExecutionID: "exec", DetectedAt: detected, Summary: "summary", EvidenceEventIDs: []string{"e1"}, GraphID: graphID, SanitizationStatus: contracts.SanitizationPass}
}

func graph(id, incidentID string, nodes ...contracts.GraphNode) contracts.ExecutionGraph {
	return contracts.ExecutionGraph{SchemaVersion: "1.0", GraphID: id, IncidentID: incidentID, OrderingPolicyVersion: "1.0", Nodes: nodes, Edges: []contracts.GraphEdge{}}
}

func TestStoreRepositorySeamAndDuplicateEvent(t *testing.T) {
	var repository Repository = NewStore()
	ctx := context.Background()
	e := event("e1")
	if err := repository.IngestEvent(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err := repository.IngestEvent(ctx, e); !errors.Is(err, ErrConflict) {
		t.Fatalf("got %v", err)
	}
}

func TestStorePutIncidentIsAtomicAndRejectsDuplicateIDs(t *testing.T) {
	ctx := context.Background()
	s := NewStore()
	for _, id := range []string{"e1", "e2"} {
		if err := s.IngestEvent(ctx, event(id)); err != nil {
			t.Fatal(err)
		}
	}
	i1 := incident("inc-1", "g-1", "2026-08-29T10:32:01Z", contracts.IncidentReady)
	g1 := graph("g-1", "inc-1", contracts.GraphNode{EventID: "e1", TimelineIndex: 0})
	if err := s.PutIncident(ctx, i1, g1); err != nil {
		t.Fatal(err)
	}
	if err := s.PutIncident(ctx, incident("inc-2", "g-1", "2026-08-29T10:32:02Z", contracts.IncidentReady), graph("g-1", "inc-2", contracts.GraphNode{EventID: "e2", TimelineIndex: 0})); !errors.Is(err, ErrConflict) {
		t.Fatalf("got %v", err)
	}
	if _, err := s.GetIncidentDetail(ctx, "inc-2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("partial incident mutation: %v", err)
	}
	if err := s.PutIncident(ctx, incident("inc-bad", "g-bad", "2026-08-29T10:32:02Z", contracts.IncidentReady), graph("g-bad", "inc-bad", contracts.GraphNode{EventID: "missing", TimelineIndex: 0})); !errors.Is(err, ErrInternal) {
		t.Fatalf("got %v", err)
	}
	if _, err := s.GetIncidentDetail(ctx, "inc-bad"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("partial invalid mutation: %v", err)
	}
}

func TestStoreRejectsUnresolvedEvidenceEvents(t *testing.T) {
	ctx := context.Background()
	s := NewStore()
	if err := s.IngestEvent(ctx, event("e1")); err != nil {
		t.Fatal(err)
	}
	unresolved := incident("inc-1", "g-1", "2026-08-29T10:32:01Z", contracts.IncidentReady)
	unresolved.EvidenceEventIDs = []string{"e1", "missing"}
	if err := s.PutIncident(ctx, unresolved, graph("g-1", "inc-1", contracts.GraphNode{EventID: "e1", TimelineIndex: 0})); !errors.Is(err, ErrInternal) {
		t.Fatalf("got %v", err)
	}
	if _, err := s.GetIncidentDetail(ctx, "inc-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("partial invalid mutation: %v", err)
	}
}

func TestStoreListIncidentsOrderingFilteringAndPagination(t *testing.T) {
	ctx := context.Background()
	s := NewStore()
	if err := s.IngestEvent(ctx, event("e1")); err != nil {
		t.Fatal(err)
	}
	inputs := []contracts.Incident{
		incident("inc-a", "g-a", "2026-08-29T10:32:01Z", contracts.IncidentReady),
		incident("inc-c", "g-c", "2026-08-29T10:32:02Z", contracts.IncidentReady),
		incident("inc-b", "g-b", "2026-08-29T10:32:02Z", contracts.IncidentDetected),
	}
	for _, i := range inputs {
		if err := s.PutIncident(ctx, i, graph(i.GraphID, i.IncidentID, contracts.GraphNode{EventID: "e1", TimelineIndex: 0})); err != nil {
			t.Fatal(err)
		}
	}
	limit := 1
	first, err := s.ListIncidents(ctx, contracts.IncidentListQuery{Status: contracts.IncidentReady, Limit: &limit})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || first.Items[0].IncidentID != "inc-c" || first.NextCursor == "" {
		t.Fatalf("first page: %#v", first)
	}
	second, err := s.ListIncidents(ctx, contracts.IncidentListQuery{Status: contracts.IncidentReady, Limit: &limit, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{second.Items[0].IncidentID, second.NextCursor}; !reflect.DeepEqual(got, []string{"inc-a", ""}) {
		t.Fatalf("second page: %#v", second)
	}
}

func TestStoreDetailReturnsOnlyGraphEventsInTimelineOrder(t *testing.T) {
	ctx := context.Background()
	s := NewStore()
	for _, id := range []string{"outside", "e1", "e2"} {
		if err := s.IngestEvent(ctx, event(id)); err != nil {
			t.Fatal(err)
		}
	}
	i := incident("inc", "g", "2026-08-29T10:32:01Z", contracts.IncidentReady)
	g := graph("g", "inc", contracts.GraphNode{EventID: "e2", TimelineIndex: 0}, contracts.GraphNode{EventID: "e1", TimelineIndex: 1})
	if err := s.PutIncident(ctx, i, g); err != nil {
		t.Fatal(err)
	}
	detail, err := s.GetIncidentDetail(ctx, "inc")
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{detail.Events[0].EventID, detail.Events[1].EventID}; !reflect.DeepEqual(got, []string{"e2", "e1"}) {
		t.Fatalf("got %v", got)
	}
}
