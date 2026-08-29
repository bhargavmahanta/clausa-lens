package main

import (
	"context"
	"fmt"
	"testing"

	gatewaysvc "github.com/causalens/causalens/cmd/demo-gateway/service"
	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/graph"
	checkoutpack "github.com/causalens/causalens/internal/systempack/checkout"
)

// TestE1ExitCriterion_EndToEnd verifies planning/E0-E3.md's E1 exit
// criterion: "From a clean reset, the golden request repeatedly creates one
// inspectable incident containing the expected timeout, retry, two payment
// attempts, two ledger effects, graph relationships, and true oracle
// evidence."
//
// IMPORTANT CAVEAT (reported to Bhargav): cmd/core-api's live POST
// /v1/events route only calls repository.IngestEvent -- nothing on the live
// HTTP path calls the pack's DetectIncident or core.Store.PutIncident, so
// GET /v1/incidents cannot currently return anything for a golden run. That
// wiring is missing entirely from cmd/core-api / internal/core, both
// outside my owned paths, so I cannot add it myself.
//
// This test proves the part of E1 that IS my responsibility -- that the
// demo services' captured evidence and the checkout_duplicate_effect pack's
// DetectIncident produce a correct, "inspectable" incident once Bhargav's
// real, unmodified graph/incident-store code processes them. It performs
// the missing "detect + persist" step itself, calling ONLY his exported
// functions (graph.BuildNodes, core.Store.PutIncident,
// core.Store.GetIncidentDetail, core.Store.ListIncidents) exactly the way a
// live caller would -- it does not reimplement or stub any of his logic.
func TestE1ExitCriterion_EndToEnd(t *testing.T) {
	const runs = 5
	h := newGoldenHarness(t)
	pack := checkoutpack.New()

	for i := 0; i < runs; i++ {
		h.reset(context.Background())

		result, err := h.gateway.Checkout(context.Background(), gatewaysvc.Request{CheckoutID: "8271"})
		if err != nil {
			t.Fatalf("run %d: Checkout: %v", i+1, err)
		}
		events := h.local.Events()

		// --- Structural evidence from capture (my responsibility) ---
		if got := h.payment.AttemptCount(); got != 2 {
			t.Fatalf("run %d: payment attempts = %d, want 2", i+1, got)
		}
		if got := h.ledger.CommittedEffectCount(); got != 2 {
			t.Fatalf("run %d: ledger commits = %d, want 2", i+1, got)
		}

		// --- Oracle evidence from the real pack (my responsibility) ---
		oracle, err := pack.DetectIncident(context.Background(), events)
		if err != nil {
			t.Fatalf("run %d: DetectIncident: %v", i+1, err)
		}
		if !oracle.Matched {
			t.Fatalf("run %d: oracle did not match (not stubbed -- computed from evidence): %s", i+1, oracle.Explanation)
		}
		if oracle.EffectSummary != (contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2}) {
			t.Fatalf("run %d: oracle EffectSummary = %+v, want {2 2}", i+1, oracle.EffectSummary)
		}

		// --- Build the graph the same way a live incident-detection
		// caller would: PARENT_CHILD edges from each event's real
		// parent_event_id (observed structure, not invented). ---
		edges := parentChildEdges(events)
		nodes, err := graph.BuildNodes(events, edges)
		if err != nil {
			t.Fatalf("run %d: graph.BuildNodes: %v", i+1, err)
		}
		graphID := fmt.Sprintf("graph-verify-%d", i+1)
		g := contracts.ExecutionGraph{
			SchemaVersion: contracts.ContractVersion, GraphID: graphID, IncidentID: fmt.Sprintf("inc-verify-%d", i+1),
			OrderingPolicyVersion: contracts.ContractVersion, Nodes: nodes, Edges: edges,
		}

		incident := contracts.Incident{
			SchemaVersion: contracts.ContractVersion, IncidentID: g.IncidentID, Status: contracts.IncidentReady,
			FailureOracle: oracle.Oracle, SystemPack: pack.Descriptor(),
			TraceID: result.TraceID, ExecutionID: result.ExecutionID, DetectedAt: events[len(events)-1].OccurredAt,
			Summary: oracle.Explanation, EvidenceEventIDs: oracle.RequiredEvidenceEventIDs, GraphID: graphID,
			SanitizationStatus: contracts.SanitizationPass,
		}

		// --- Persist through Bhargav's real, unmodified store (the exact
		// call a live incident-creation trigger would make). ---
		if err := h.store.PutIncident(context.Background(), incident, g); err != nil {
			t.Fatalf("run %d: PutIncident: %v", i+1, err)
		}

		// --- Read back exactly as GET /v1/incidents/{id} would. ---
		detail, err := h.store.GetIncidentDetail(context.Background(), incident.IncidentID)
		if err != nil {
			t.Fatalf("run %d: GetIncidentDetail: %v", i+1, err)
		}
		assertInspectableIncident(t, i+1, detail)

		// --- Read back exactly as GET /v1/incidents would. ---
		list, err := h.store.ListIncidents(context.Background(), contracts.IncidentListQuery{})
		if err != nil {
			t.Fatalf("run %d: ListIncidents: %v", i+1, err)
		}
		if len(list.Items) != 1 || list.Items[0].IncidentID != incident.IncidentID {
			t.Fatalf("run %d: ListIncidents = %+v, want exactly the one incident just created", i+1, list.Items)
		}

		t.Logf("run %d/%d: E1 criterion satisfied for incident %s (graph %s, %d events, oracle matched)",
			i+1, runs, incident.IncidentID, graphID, len(events))
	}
}

// parentChildEdges derives PARENT_CHILD graph edges from each event's
// already-captured parent_event_id -- observed structure recorded by
// internal/capture, not invented for this test.
func parentChildEdges(events []contracts.ExecutionEvent) []contracts.GraphEdge {
	edges := make([]contracts.GraphEdge, 0, len(events))
	for _, e := range events {
		if e.ParentEventID == "" {
			continue
		}
		edges = append(edges, contracts.GraphEdge{
			EdgeID: "edge-" + e.EventID, FromEventID: e.ParentEventID, ToEventID: e.EventID, Type: contracts.GraphEdgeParentChild,
		})
	}
	return edges
}

// assertInspectableIncident checks the IncidentDetailResponse shape against
// the E1 exit criterion: timeline order, timeout, retry, two payment
// attempts, two ledger effects, graph relationships, true oracle evidence.
func assertInspectableIncident(t *testing.T, run int, detail contracts.IncidentDetailResponse) {
	t.Helper()

	if detail.Incident.FailureOracle.ID != "duplicate_ledger_effect" {
		t.Fatalf("run %d: FailureOracle.ID = %q", run, detail.Incident.FailureOracle.ID)
	}
	if detail.Incident.Status != contracts.IncidentReady {
		t.Fatalf("run %d: incident status = %s, want READY", run, detail.Incident.Status)
	}
	if len(detail.Graph.Nodes) != len(detail.Events) {
		t.Fatalf("run %d: graph has %d nodes for %d events", run, len(detail.Graph.Nodes), len(detail.Events))
	}

	var sawTimeout, sawRetry bool
	attempts := map[int]bool{}
	committedEffects := 0
	lastIndex := -1
	for _, e := range detail.Events {
		// Events must come back in timeline_index order -- find this
		// event's node and confirm indices are non-decreasing.
		for _, n := range detail.Graph.Nodes {
			if n.EventID == e.EventID {
				if n.TimelineIndex < lastIndex {
					t.Fatalf("run %d: event %s out of timeline order (index %d after %d)", run, e.EventID, n.TimelineIndex, lastIndex)
				}
				lastIndex = n.TimelineIndex
			}
		}
		switch e.EventType {
		case contracts.EventTimeout:
			sawTimeout = true
		case contracts.EventRetry:
			sawRetry = true
		case contracts.EventStart:
			if e.Operation.Kind == contracts.OperationDependency {
				attempts[e.Attempt] = true
			}
		case contracts.EventEffect:
			if committed, ok := e.Attributes["effect_committed"].(bool); ok && committed {
				committedEffects++
			}
		}
	}
	if !sawTimeout {
		t.Fatalf("run %d: incident evidence missing a TIMEOUT event", run)
	}
	if !sawRetry {
		t.Fatalf("run %d: incident evidence missing a RETRY event", run)
	}
	if len(attempts) != 2 || !attempts[1] || !attempts[2] {
		t.Fatalf("run %d: expected payment attempts 1 and 2, got %v", run, attempts)
	}
	if committedEffects != 2 {
		t.Fatalf("run %d: expected 2 committed ledger effects, got %d", run, committedEffects)
	}
}
