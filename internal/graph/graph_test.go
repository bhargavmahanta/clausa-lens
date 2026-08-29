package graph

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/causalens/causalens/internal/contracts"
)

func TestOrderUsesTimelineIndex(t *testing.T) {
	events := []contracts.ExecutionEvent{
		event("b", "2026-08-29T10:00:00.000Z", "worker", "2", 2),
		event("a", "2026-08-29T09:00:00.000Z", "worker", "1", 1),
	}
	graph := validGraph(
		[]contracts.GraphNode{{EventID: "a", TimelineIndex: 1}, {EventID: "b", TimelineIndex: 0}},
		nil,
	)

	got, err := Order(events, graph)
	if err != nil {
		t.Fatalf("Order() error = %v", err)
	}
	if ids := eventIDs(got); !reflect.DeepEqual(ids, []string{"b", "a"}) {
		t.Fatalf("Order() IDs = %v, want [b a]", ids)
	}
	if ids := eventIDs(events); !reflect.DeepEqual(ids, []string{"b", "a"}) {
		t.Fatalf("Order() mutated input: %v", ids)
	}
}

func TestOrderRejectsInvalidGraphAndEventMismatch(t *testing.T) {
	tests := []struct {
		name   string
		events []contracts.ExecutionEvent
		graph  contracts.ExecutionGraph
	}{
		{"missing node event", []contracts.ExecutionEvent{event("a", "2026-08-29T10:00:00Z", "c", "i", 0)}, validGraph([]contracts.GraphNode{{EventID: "missing", TimelineIndex: 0}}, nil)},
		{"event omitted from graph", []contracts.ExecutionEvent{event("a", "2026-08-29T10:00:00Z", "c", "i", 0), event("b", "2026-08-29T10:00:01Z", "c", "i", 1)}, validGraph([]contracts.GraphNode{{EventID: "a", TimelineIndex: 0}}, nil)},
		{"duplicate event", []contracts.ExecutionEvent{event("a", "2026-08-29T10:00:00Z", "c", "i", 0), event("a", "2026-08-29T10:00:00Z", "c", "i", 0)}, validGraph([]contracts.GraphNode{{EventID: "a", TimelineIndex: 0}}, nil)},
		{"duplicate timeline index", []contracts.ExecutionEvent{event("a", "2026-08-29T10:00:00Z", "c", "i", 0), event("b", "2026-08-29T10:00:01Z", "c", "i", 1)}, validGraph([]contracts.GraphNode{{EventID: "a", TimelineIndex: 0}, {EventID: "b", TimelineIndex: 0}}, nil)},
		{"dangling edge", []contracts.ExecutionEvent{event("a", "2026-08-29T10:00:00Z", "c", "i", 0)}, validGraph([]contracts.GraphNode{{EventID: "a", TimelineIndex: 0}}, []contracts.GraphEdge{edge("e", "a", "missing", contracts.GraphEdgeDependency)})},
		{"self edge", []contracts.ExecutionEvent{event("a", "2026-08-29T10:00:00Z", "c", "i", 0)}, validGraph([]contracts.GraphNode{{EventID: "a", TimelineIndex: 0}}, []contracts.GraphEdge{edge("e", "a", "a", contracts.GraphEdgeDependency)})},
		{"hard edge contradicts timeline", []contracts.ExecutionEvent{event("a", "2026-08-29T10:00:00Z", "c", "i", 0), event("b", "2026-08-29T10:00:01Z", "c", "i", 1)}, validGraph([]contracts.GraphNode{{EventID: "a", TimelineIndex: 1}, {EventID: "b", TimelineIndex: 0}}, []contracts.GraphEdge{edge("e", "a", "b", contracts.GraphEdgeDependency)})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Order(tt.events, tt.graph); err == nil {
				t.Fatal("Order() error = nil")
			}
		})
	}
	_, err := Order(tests[5].events, tests[5].graph)
	if errors.Is(err, ErrGraphCycle) {
		t.Fatalf("self edge error = %v, must remain an invalid edge", err)
	}
}

func TestOrderIdentifiesHardEdgeCycle(t *testing.T) {
	events := []contracts.ExecutionEvent{event("a", "2026-08-29T10:00:00Z", "c", "i", 0), event("b", "2026-08-29T10:00:01Z", "c", "i", 1)}
	graph := validGraph(
		[]contracts.GraphNode{{EventID: "a", TimelineIndex: 0}, {EventID: "b", TimelineIndex: 1}},
		[]contracts.GraphEdge{edge("ab", "a", "b", contracts.GraphEdgeDependency), edge("ba", "b", "a", contracts.GraphEdgeRetry)},
	)

	_, err := Order(events, graph)
	if !errors.Is(err, ErrGraphCycle) || !strings.Contains(err.Error(), string(contracts.GraphCycle)) {
		t.Fatalf("Order() error = %v, want identifiable %s", err, contracts.GraphCycle)
	}
}

func TestBuildNodesIsDeterministicAndHonorsHardEdges(t *testing.T) {
	events := []contracts.ExecutionEvent{
		event("late-parent", "2026-08-29T10:00:01.000Z", "z", "1", 0),
		event("early-child", "2026-08-29T10:00:00.000Z", "a", "1", 0),
		event("free", "2026-08-29T09:59:59.000Z", "m", "1", 0),
	}
	edges := []contracts.GraphEdge{edge("hard", "late-parent", "early-child", contracts.GraphEdgeParentChild)}

	want := []contracts.GraphNode{{EventID: "free", TimelineIndex: 0}, {EventID: "late-parent", TimelineIndex: 1}, {EventID: "early-child", TimelineIndex: 2}}
	for _, input := range [][]contracts.ExecutionEvent{events, {events[2], events[0], events[1]}, {events[1], events[2], events[0]}} {
		got, err := BuildNodes(input, edges)
		if err != nil {
			t.Fatalf("BuildNodes() error = %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("BuildNodes() = %#v, want %#v", got, want)
		}
	}
}

func TestBuildNodesUsesFiveMillisecondTieBreakersAndIgnoresTemporalEdges(t *testing.T) {
	events := []contracts.ExecutionEvent{
		event("sequence", "2026-08-29T10:00:00.000Z", "a", "1", 2),
		event("instance", "2026-08-29T10:00:00.004Z", "a", "0", 9),
		event("component", "2026-08-29T10:00:00.005Z", "0", "9", 9),
		event("event-a", "2026-08-29T10:00:00.003Z", "a", "1", 1),
		event("event-b", "2026-08-29T10:00:00.002Z", "a", "1", 1),
	}
	edges := []contracts.GraphEdge{edge("presentation", "sequence", "component", contracts.GraphEdgeTemporal)}

	got, err := BuildNodes(events, edges)
	if err != nil {
		t.Fatalf("BuildNodes() error = %v", err)
	}
	want := []contracts.GraphNode{{EventID: "component", TimelineIndex: 0}, {EventID: "instance", TimelineIndex: 1}, {EventID: "event-a", TimelineIndex: 2}, {EventID: "event-b", TimelineIndex: 3}, {EventID: "sequence", TimelineIndex: 4}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildNodes() = %#v, want %#v", got, want)
	}
}

func TestBuildNodesRejectsInvalidReferencesAndIdentifiesCycle(t *testing.T) {
	events := []contracts.ExecutionEvent{event("a", "2026-08-29T10:00:00Z", "c", "i", 0), event("b", "2026-08-29T10:00:01Z", "c", "i", 1)}
	if _, err := BuildNodes(events, []contracts.GraphEdge{edge("dangling", "a", "missing", contracts.GraphEdgeDependency)}); err == nil {
		t.Fatal("BuildNodes() accepted dangling edge")
	}
	if _, err := BuildNodes(events, []contracts.GraphEdge{edge("self", "a", "a", contracts.GraphEdgeDependency)}); err == nil {
		t.Fatal("BuildNodes() accepted self edge")
	}
	_, err := BuildNodes(events, []contracts.GraphEdge{edge("ab", "a", "b", contracts.GraphEdgeDependency), edge("ba", "b", "a", contracts.GraphEdgeRetry)})
	if !errors.Is(err, ErrGraphCycle) {
		t.Fatalf("BuildNodes() cycle error = %v, want errors.Is(ErrGraphCycle)", err)
	}
}

func event(id, occurredAt, component, instance string, sequence int) contracts.ExecutionEvent {
	return contracts.ExecutionEvent{SchemaVersion: contracts.ContractVersion, EventID: id, ExecutionID: "execution", TraceID: "trace", Component: contracts.ComponentRef{Name: component, Instance: instance}, Operation: contracts.OperationRef{Name: "operation", Kind: contracts.OperationInternal}, EventType: contracts.EventStart, Attempt: 1, LogicalOperationID: "logical", OccurredAt: occurredAt, Sequence: sequence, Status: contracts.EventRunning, Attributes: map[string]any{}}
}

func validGraph(nodes []contracts.GraphNode, edges []contracts.GraphEdge) contracts.ExecutionGraph {
	return contracts.ExecutionGraph{SchemaVersion: contracts.ContractVersion, GraphID: "graph", IncidentID: "incident", OrderingPolicyVersion: contracts.ContractVersion, Nodes: nodes, Edges: edges}
}

func edge(id, from, to string, edgeType contracts.GraphEdgeType) contracts.GraphEdge {
	return contracts.GraphEdge{EdgeID: id, FromEventID: from, ToEventID: to, Type: edgeType}
}

func eventIDs(events []contracts.ExecutionEvent) []string {
	ids := make([]string, len(events))
	for i := range events {
		ids[i] = events[i].EventID
	}
	return ids
}
