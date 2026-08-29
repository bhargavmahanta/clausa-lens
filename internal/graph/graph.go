package graph

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/causalens/causalens/internal/contracts"
)

var ErrGraphCycle = errors.New(string(contracts.GraphCycle))

// Order validates the supplied graph against events and returns a sorted copy.
func Order(events []contracts.ExecutionEvent, graph contracts.ExecutionGraph) ([]contracts.ExecutionEvent, error) {
	if err := graph.Validate(); err != nil {
		if hasHardCycle(graph.Nodes, graph.Edges) {
			return nil, fmt.Errorf("%w: hard ordering constraints contain a cycle", ErrGraphCycle)
		}
		return nil, fmt.Errorf("invalid graph: %w", err)
	}

	byID := make(map[string]contracts.ExecutionEvent, len(events))
	for _, event := range events {
		if err := event.Validate(); err != nil {
			return nil, fmt.Errorf("invalid event %q: %w", event.EventID, err)
		}
		if _, exists := byID[event.EventID]; exists {
			return nil, fmt.Errorf("duplicate event_id %q", event.EventID)
		}
		byID[event.EventID] = event
	}
	if len(byID) != len(graph.Nodes) {
		return nil, fmt.Errorf("graph/event inconsistency: graph has %d nodes for %d events", len(graph.Nodes), len(byID))
	}

	indices := make(map[string]int, len(graph.Nodes))
	ordered := make([]contracts.ExecutionEvent, len(graph.Nodes))
	for _, node := range graph.Nodes {
		event, exists := byID[node.EventID]
		if !exists {
			return nil, fmt.Errorf("graph node %q has no supplied event", node.EventID)
		}
		if node.TimelineIndex >= len(ordered) {
			return nil, fmt.Errorf("graph timeline indices must be contiguous from zero")
		}
		indices[node.EventID] = node.TimelineIndex
		ordered[node.TimelineIndex] = event
	}
	for _, edge := range graph.Edges {
		if edge.Type != contracts.GraphEdgeTemporal && indices[edge.FromEventID] >= indices[edge.ToEventID] {
			return nil, fmt.Errorf("hard edge %q contradicts timeline indices", edge.EdgeID)
		}
	}

	sort.Slice(ordered, func(i, j int) bool {
		return indices[ordered[i].EventID] < indices[ordered[j].EventID]
	})
	return ordered, nil
}

// BuildNodes derives timeline indices from events and graph constraints.
func BuildNodes(events []contracts.ExecutionEvent, edges []contracts.GraphEdge) ([]contracts.GraphNode, error) {
	byID := make(map[string]contracts.ExecutionEvent, len(events))
	nodes := make([]contracts.GraphNode, len(events))
	for i, event := range events {
		if err := event.Validate(); err != nil {
			return nil, fmt.Errorf("invalid event %q: %w", event.EventID, err)
		}
		if _, exists := byID[event.EventID]; exists {
			return nil, fmt.Errorf("duplicate event_id %q", event.EventID)
		}
		byID[event.EventID] = event
		nodes[i] = contracts.GraphNode{EventID: event.EventID, TimelineIndex: i}
	}

	validationGraph := contracts.ExecutionGraph{
		SchemaVersion: contracts.ContractVersion, GraphID: "validation", IncidentID: "validation",
		OrderingPolicyVersion: contracts.ContractVersion, Nodes: nodes, Edges: edges,
	}
	if err := validationGraph.Validate(); err != nil {
		if hasHardCycle(nodes, edges) {
			return nil, fmt.Errorf("%w: hard ordering constraints contain a cycle", ErrGraphCycle)
		}
		return nil, fmt.Errorf("invalid graph constraints: %w", err)
	}

	adjacency := make(map[string][]string, len(events))
	indegree := make(map[string]int, len(events))
	for id := range byID {
		indegree[id] = 0
	}
	for _, edge := range edges {
		if edge.Type == contracts.GraphEdgeTemporal {
			continue
		}
		adjacency[edge.FromEventID] = append(adjacency[edge.FromEventID], edge.ToEventID)
		indegree[edge.ToEventID]++
	}

	result := make([]contracts.GraphNode, 0, len(events))
	for len(result) < len(events) {
		eligible := make([]contracts.ExecutionEvent, 0)
		var earliest time.Time
		for id, degree := range indegree {
			if degree != 0 {
				continue
			}
			event := byID[id]
			occurredAt, _ := time.Parse(time.RFC3339Nano, event.OccurredAt)
			if earliest.IsZero() || occurredAt.Before(earliest) {
				earliest = occurredAt
			}
			eligible = append(eligible, event)
		}
		if len(eligible) == 0 {
			return nil, fmt.Errorf("%w: hard ordering constraints contain a cycle", ErrGraphCycle)
		}

		cutoff := earliest.Add(5 * time.Millisecond)
		candidates := eligible[:0]
		for _, event := range eligible {
			occurredAt, _ := time.Parse(time.RFC3339Nano, event.OccurredAt)
			if !occurredAt.After(cutoff) {
				candidates = append(candidates, event)
			}
		}
		sort.Slice(candidates, func(i, j int) bool { return tieLess(candidates[i], candidates[j]) })
		selected := candidates[0]
		result = append(result, contracts.GraphNode{EventID: selected.EventID, TimelineIndex: len(result)})
		delete(indegree, selected.EventID)
		for _, next := range adjacency[selected.EventID] {
			indegree[next]--
		}
	}
	return result, nil
}

func tieLess(a, b contracts.ExecutionEvent) bool {
	if a.Component.Name != b.Component.Name {
		return a.Component.Name < b.Component.Name
	}
	if a.Component.Instance != b.Component.Instance {
		return a.Component.Instance < b.Component.Instance
	}
	if a.Sequence != b.Sequence {
		return a.Sequence < b.Sequence
	}
	return a.EventID < b.EventID
}

func hasHardCycle(nodes []contracts.GraphNode, edges []contracts.GraphEdge) bool {
	adjacency := make(map[string][]string, len(nodes))
	for _, edge := range edges {
		if edge.Type != contracts.GraphEdgeTemporal && edge.FromEventID != edge.ToEventID {
			adjacency[edge.FromEventID] = append(adjacency[edge.FromEventID], edge.ToEventID)
		}
	}
	state := make(map[string]uint8, len(nodes))
	var visit func(string) bool
	visit = func(id string) bool {
		if state[id] == 1 {
			return true
		}
		if state[id] == 2 {
			return false
		}
		state[id] = 1
		for _, next := range adjacency[id] {
			if visit(next) {
				return true
			}
		}
		state[id] = 2
		return false
	}
	for _, node := range nodes {
		if visit(node.EventID) {
			return true
		}
	}
	return false
}
