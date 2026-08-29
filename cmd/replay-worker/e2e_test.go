package main

import (
	"context"
	"testing"
	"time"

	"github.com/causalens/causalens/internal/capsule"
	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/core"
	"github.com/causalens/causalens/internal/differential"
	graphpkg "github.com/causalens/causalens/internal/graph"
	"github.com/causalens/causalens/internal/replay"
	checkout "github.com/causalens/causalens/internal/systempack/checkout"
)

func srcEvent(id string, comp string, op string, kind contracts.OperationKind, eventType contracts.EventType, attempt int, seq int, occurred string, status contracts.EventStatus, attrs map[string]any) contracts.ExecutionEvent {
	return contracts.ExecutionEvent{
		SchemaVersion: contracts.ContractVersion, EventID: id, ExecutionID: "exec-8271", TraceID: "trace-8271",
		Component: contracts.ComponentRef{Name: comp, Instance: comp + "-1"}, Operation: contracts.OperationRef{Name: op, Kind: kind},
		EventType: eventType, Attempt: attempt, LogicalOperationID: "checkout-8271", OccurredAt: occurred, Sequence: seq,
		Status: status, Attributes: attrs,
	}
}

func goldenIncidentEvents() []contracts.ExecutionEvent {
	return []contracts.ExecutionEvent{
		srcEvent("evt-p1", "payment", "authorize", contracts.OperationDependency, contracts.EventStart, 1, 0, "2026-08-29T10:32:01.015Z", contracts.EventRunning, map[string]any{"configured_latency_ms": 350}),
		srcEvent("evt-p2", "payment", "authorize", contracts.OperationDependency, contracts.EventStart, 2, 1, "2026-08-29T10:32:01.210Z", contracts.EventRunning, map[string]any{"configured_latency_ms": 350}),
		srcEvent("evt-timeout", "checkout", "checkout", contracts.OperationControl, contracts.EventTimeout, 1, 0, "2026-08-29T10:32:01.204Z", contracts.EventTimedOut, map[string]any{}),
		srcEvent("evt-retry", "checkout", "checkout", contracts.OperationControl, contracts.EventRetry, 1, 1, "2026-08-29T10:32:01.205Z", contracts.EventRunning, map[string]any{}),
		srcEvent("evt-l1", "ledger", "ledger.commit", contracts.OperationSideEffect, contracts.EventEffect, 1, 0, "2026-08-29T10:32:01.365Z", contracts.EventSuccess, map[string]any{"effect_id": "eff-1", "effect_committed": true}),
		srcEvent("evt-l2", "ledger", "ledger.commit", contracts.OperationSideEffect, contracts.EventEffect, 2, 1, "2026-08-29T10:32:01.560Z", contracts.EventSuccess, map[string]any{"effect_id": "eff-2", "effect_committed": true}),
	}
}

func compileCapsule(t *testing.T) contracts.ReplayCapsule {
	t.Helper()
	incident := contracts.Incident{
		SchemaVersion: contracts.ContractVersion, IncidentID: "inc-1", Status: contracts.IncidentReady,
		FailureOracle: contracts.FailureOracleRef{ID: checkout.OracleID, Version: checkout.OracleVersion},
		SystemPack:    contracts.SystemPackRef{ID: checkout.PackID, Version: checkout.PackVersion, InterfaceVersion: contracts.ContractVersion},
		TraceID:       "trace-8271", ExecutionID: "exec-8271", DetectedAt: "2026-08-29T10:32:01.561Z",
		Summary:          "Timeout-driven retry committed two ledger effects for checkout-8271.",
		EvidenceEventIDs: []string{"evt-timeout", "evt-retry", "evt-l1", "evt-l2"}, GraphID: "graph-1",
		SanitizationStatus: contracts.SanitizationPass,
	}
	events := goldenIncidentEvents()
	pack := checkout.New()
	cap, err := capsule.Compile(pack, incident.SystemPack, incident, events, contracts.CapsuleSource{
		IncidentID: "inc-1", TraceID: "trace-8271", ExecutionID: "exec-8271", CaptureEnvironment: contracts.CaptureDemo, CapturedAt: incident.DetectedAt,
	}, contracts.Trigger{RequestOrMessage: map[string]any{"method": "POST", "path": "/checkout"}, SanitizedHeaders: map[string]string{"content-type": "application/json"}},
		"cap-1", "graph-1", time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return cap
}

func buildGoldenIncident(t *testing.T, store *core.Store) (contracts.Incident, contracts.ReplayCapsule) {
	t.Helper()
	ctx := context.Background()
	events := goldenIncidentEvents()
	for _, e := range events {
		if err := store.IngestEvent(ctx, e); err != nil {
			t.Fatalf("ingest source event %s: %v", e.EventID, err)
		}
	}
	incident := contracts.Incident{
		SchemaVersion: contracts.ContractVersion, IncidentID: "inc-1", Status: contracts.IncidentReady,
		FailureOracle: contracts.FailureOracleRef{ID: checkout.OracleID, Version: checkout.OracleVersion},
		SystemPack:    contracts.SystemPackRef{ID: checkout.PackID, Version: checkout.PackVersion, InterfaceVersion: contracts.ContractVersion},
		TraceID:       "trace-8271", ExecutionID: "exec-8271", DetectedAt: "2026-08-29T10:32:01.561Z",
		Summary:          "Timeout-driven retry committed two ledger effects for checkout-8271.",
		EvidenceEventIDs: []string{"evt-timeout", "evt-retry", "evt-l1", "evt-l2"}, GraphID: "graph-1",
		SanitizationStatus: contracts.SanitizationPass,
	}
	nodes, err := graphpkg.BuildNodes(events, nil)
	if err != nil {
		t.Fatalf("build graph nodes: %v", err)
	}
	graph := contracts.ExecutionGraph{SchemaVersion: contracts.ContractVersion, GraphID: "graph-1", IncidentID: "inc-1", OrderingPolicyVersion: contracts.ContractVersion, Nodes: nodes, Edges: []contracts.GraphEdge{}}
	if err := store.PutIncident(ctx, incident, graph); err != nil {
		t.Fatalf("put incident: %v", err)
	}

	cap := compileCapsule(t)
	if err := store.PutCapsule(ctx, cap); err != nil {
		t.Fatalf("put capsule: %v", err)
	}
	return incident, cap
}

func newRun(t *testing.T, store *core.Store, runID string, runType contracts.RunType, baselineRunID string, intervention *contracts.Intervention, hash string) contracts.ReplayRun {
	t.Helper()
	run, err := replay.NewRun(runID, "exec-replay-"+runID, "cap-1", hash, runType, baselineRunID, intervention, 1)
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	if err := store.PutRun(context.Background(), run); err != nil {
		t.Fatalf("put run: %v", err)
	}
	return run
}

// TestReplayTracerBullet is the E2E/E3 tracer bullet: from a READY incident it
// compiles a capsule, executes a baseline replay through the real demo service
// chain (real events with replay_run_id), executes a what-if with exactly the
// PAYMENT_LATENCY 350->50 intervention, builds a real Replay Diff, and verifies
// the first meaningful divergence.
func TestReplayTracerBullet(t *testing.T) {
	ctx := context.Background()
	store := core.NewStore()
	_, cap := buildGoldenIncident(t, store)
	pack := checkout.New()
	runner := newDemoRunner()

	// Baseline run.
	newRun(t, store, "run-base", contracts.RunTypeBaseline, "", nil, cap.Integrity.Digest)
	baseRun, err := processRun(ctx, store, pack, runner, "run-base", 5*time.Minute)
	if err != nil {
		t.Fatalf("baseline processRun: %v", err)
	}
	if baseRun.Status != contracts.ReplayRunCompleted || baseRun.Outcome != contracts.ReplayOutcomeReproduced {
		t.Fatalf("baseline: status=%s outcome=%s", baseRun.Status, baseRun.Outcome)
	}
	if baseRun.EffectSummary == nil || baseRun.EffectSummary.PaymentAttemptCount != 2 || baseRun.EffectSummary.LedgerCommitCount != 2 {
		t.Fatalf("baseline effect summary: %+v", baseRun.EffectSummary)
	}
	if baseRun.IsolationEvidence == nil || baseRun.IsolationEvidence.Verdict != contracts.VerdictPass {
		t.Fatalf("baseline isolation: %+v", baseRun.IsolationEvidence)
	}
	baseEvents := mustEventsForRun(t, store, "run-base")
	if len(baseEvents) == 0 {
		t.Fatal("baseline captured no replay events")
	}
	for _, e := range baseEvents {
		if e.ReplayRunID != "run-base" {
			t.Fatalf("baseline event %s missing replay_run_id (got %q)", e.EventID, e.ReplayRunID)
		}
	}

	// What-if run.
	intervention := &contracts.Intervention{Type: contracts.InterventionPaymentLatency, From: 350, To: 50, Unit: contracts.InterventionUnitMilliseconds}
	newRun(t, store, "run-whatif", contracts.RunTypeWhatIf, "run-base", intervention, cap.Integrity.Digest)
	compRun, err := processRun(ctx, store, pack, runner, "run-whatif", 5*time.Minute)
	if err != nil {
		t.Fatalf("what-if processRun: %v", err)
	}
	if compRun.Status != contracts.ReplayRunCompleted || compRun.Outcome != contracts.ReplayOutcomeMitigated {
		t.Fatalf("what-if: status=%s outcome=%s", compRun.Status, compRun.Outcome)
	}
	if compRun.EffectSummary == nil || compRun.EffectSummary.PaymentAttemptCount != 1 || compRun.EffectSummary.LedgerCommitCount != 1 {
		t.Fatalf("what-if effect summary: %+v", compRun.EffectSummary)
	}
	compEvents := mustEventsForRun(t, store, "run-whatif")
	if len(compEvents) == 0 {
		t.Fatal("what-if captured no replay events")
	}
	for _, e := range compEvents {
		if e.ReplayRunID != "run-whatif" {
			t.Fatalf("what-if event %s missing replay_run_id (got %q)", e.EventID, e.ReplayRunID)
		}
	}

	// Baseline and what-if must reference different observed event IDs.
	if sameEventIDs(baseEvents, compEvents) {
		t.Fatal("baseline and what-if observed the same replay events")
	}

	// Real Replay Diff over the completed runs.
	diff, err := differential.Build(ctx, store, "diff-1", "run-base", "run-whatif", pack)
	if err != nil {
		t.Fatalf("build diff: %v", err)
	}
	if err := diff.Validate(); err != nil {
		t.Fatalf("diff fails Validate: %v", err)
	}
	if diff.FirstMeaningfulDivergence == nil {
		t.Fatalf("expected a first meaningful divergence: %+v", diff)
	}
	if diff.BaselineEffectSummary.LedgerCommitCount != 2 || diff.ComparisonEffectSummary.LedgerCommitCount != 1 {
		t.Fatalf("diff effect summaries: baseline=%+v comparison=%+v", diff.BaselineEffectSummary, diff.ComparisonEffectSummary)
	}
}

func mustEventsForRun(t *testing.T, store *core.Store, runID string) []contracts.ExecutionEvent {
	t.Helper()
	run, err := store.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("get run %s: %v", runID, err)
	}
	events, err := store.EventsForRun(context.Background(), run)
	if err != nil {
		t.Fatalf("events for run %s: %v", runID, err)
	}
	return events
}

func sameEventIDs(a, b []contracts.ExecutionEvent) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[string]bool{}
	for _, e := range a {
		set[e.EventID] = true
	}
	for _, e := range b {
		if !set[e.EventID] {
			return false
		}
	}
	return true
}
