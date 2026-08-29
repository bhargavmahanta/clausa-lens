package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/core"
	graphpkg "github.com/causalens/causalens/internal/graph"
)

// detectAndPersistIncident evaluates the accumulated events of one execution
// against the configured System Pack's failure oracle and, on a match, builds
// the execution graph and atomically persists a stable, idempotent incident.
//
// It is deterministic per execution: the incident and graph identifiers are
// derived from the execution id, so re-evaluating the same execution never
// creates a duplicate (a PutIncident conflict is treated as an already-present
// incident and returns nil). It returns nil with no incident when the oracle
// does not match or when the accumulated execution is not yet complete; the
// caller decides whether to surface detection problems (the events route keeps
// the accepted 202 and logs detection errors instead of failing the ingest).
func detectAndPersistIncident(ctx context.Context, repository APIRepository, pack contracts.SystemPack, executionID string) error {
	if pack == nil {
		return nil
	}
	events, err := repository.EventsForExecution(ctx, executionID)
	if err != nil {
		return fmt.Errorf("detect: load execution %q: %w", executionID, err)
	}
	if len(events) == 0 {
		return nil
	}
	oracle, err := pack.DetectIncident(ctx, events)
	if err != nil {
		return fmt.Errorf("detect: oracle for %q: %w", executionID, err)
	}
	if !oracle.Matched {
		return nil
	}

	incidentID := "inc-" + executionID
	graphID := "graph-" + executionID
	edges := parentChildEdges(events)
	nodes, err := graphpkg.BuildNodes(events, edges)
	if err != nil {
		return fmt.Errorf("detect: build graph for %q: %w", executionID, err)
	}
	incident := contracts.Incident{
		SchemaVersion:      contracts.ContractVersion,
		IncidentID:         incidentID,
		Status:             contracts.IncidentReady,
		FailureOracle:      oracle.Oracle,
		SystemPack:         pack.Descriptor(),
		TraceID:            events[0].TraceID,
		ExecutionID:        executionID,
		DetectedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		Summary:            oracle.Explanation,
		EvidenceEventIDs:   oracle.RequiredEvidenceEventIDs,
		GraphID:            graphID,
		SanitizationStatus: contracts.SanitizationPass,
	}
	graph := contracts.ExecutionGraph{
		SchemaVersion:         contracts.ContractVersion,
		GraphID:               graphID,
		IncidentID:            incidentID,
		OrderingPolicyVersion: contracts.ContractVersion,
		Nodes:                 nodes,
		Edges:                 edges,
	}
	if err := repository.PutIncident(ctx, incident, graph); err != nil {
		// A duplicate incident/graph id means this execution was already detected:
		// idempotent, not an error.
		if errors.Is(err, core.ErrConflict) {
			return nil
		}
		return fmt.Errorf("detect: persist incident for %q: %w", executionID, err)
	}
	return nil
}

// parentChildEdges derives PARENT_CHILD ordering edges from each event's
// parent_event_id. Nodes that reference a parent not yet in the execution are
// left dangling (these produce a BuildNodes graph-validation error so detection
// defers until the referenced event has arrived).
func parentChildEdges(events []contracts.ExecutionEvent) []contracts.GraphEdge {
	edges := make([]contracts.GraphEdge, 0, len(events))
	for i, event := range events {
		if event.ParentEventID == "" {
			continue
		}
		edges = append(edges, contracts.GraphEdge{
			EdgeID:      fmt.Sprintf("edge-%d", i),
			FromEventID: event.ParentEventID,
			ToEventID:   event.EventID,
			Type:        contracts.GraphEdgeParentChild,
		})
	}
	return edges
}
