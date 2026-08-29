package core

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/causalens/causalens/internal/contracts"
)

func TestPostgresRepositoryIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	db, err := OpenPostgres(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `TRUNCATE replay_diffs, replay_runs, replay_capsules, execution_graphs, incidents, execution_events CASCADE`); err != nil {
		t.Fatal(err)
	}
	repository := NewPostgresRepository(db)

	e1 := eventFixture()
	e2 := eventFixture()
	e2.EventID = "evt-2"
	if err := repository.IngestEvent(ctx, e1); err != nil {
		t.Fatal(err)
	}
	if err := repository.IngestEvent(ctx, e2); err != nil {
		t.Fatal(err)
	}
	if err := repository.IngestEvent(ctx, e1); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate event: %v", err)
	}

	incident := integrationIncident("inc-1", "graph-1")
	graph := integrationGraph("graph-1", "inc-1", e2.EventID, e1.EventID)
	if err := repository.PutIncident(ctx, incident, graph); err != nil {
		t.Fatal(err)
	}
	detail, err := repository.GetIncidentDetail(ctx, incident.IncidentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Events) != 2 || detail.Events[0].EventID != e2.EventID || detail.Events[1].EventID != e1.EventID {
		t.Fatalf("timeline order: %#v", detail.Events)
	}

	rolledBack := integrationIncident("inc-rollback", graph.GraphID)
	rolledBackGraph := integrationGraph(graph.GraphID, rolledBack.IncidentID, e1.EventID)
	if err := repository.PutIncident(ctx, rolledBack, rolledBackGraph); !errors.Is(err, ErrConflict) {
		t.Fatalf("graph conflict: %v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM incidents WHERE incident_id=$1`, rolledBack.IncidentID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("incident rollback count=%d err=%v", count, err)
	}

	capsule := capsuleFixture()
	capsule.Source.IncidentID = incident.IncidentID
	if err := repository.PutCapsule(ctx, capsule); err != nil {
		t.Fatal(err)
	}
	run := contracts.ReplayRun{SchemaVersion: contracts.ContractVersion, RunID: "run-1", ExecutionID: "replay-exec", CapsuleID: capsule.CapsuleID, CapsuleHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", RunType: contracts.RunTypeBaseline, TrialNumber: 1, Status: contracts.ReplayRunCreated, ObservedEventIDs: []string{}}
	if err := repository.PutRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	run.Status = contracts.ReplayRunValidating
	if err := repository.TransitionRun(ctx, contracts.ReplayRunCreated, run); err != nil {
		t.Fatal(err)
	}
	if err := repository.TransitionRun(ctx, contracts.ReplayRunCreated, run); !errors.Is(err, ErrInvalidLifecycle) {
		t.Fatalf("stale lifecycle transition: %v", err)
	}
	persisted, err := repository.GetRun(ctx, run.RunID)
	if err != nil || persisted.Status != contracts.ReplayRunValidating {
		t.Fatalf("persisted run=%#v err=%v", persisted, err)
	}
	changed := run
	changed.Status = contracts.ReplayRunRunning
	changed.StartedAt = "2026-08-29T10:34:00Z"
	changed.ExecutionID = "changed-execution"
	if err := repository.TransitionRun(ctx, contracts.ReplayRunValidating, changed); !errors.Is(err, ErrInvalidLifecycle) {
		t.Fatalf("immutable transition: %v", err)
	}
	persisted, err = repository.GetRun(ctx, run.RunID)
	if err != nil || persisted.ExecutionID != run.ExecutionID || persisted.Status != contracts.ReplayRunValidating {
		t.Fatalf("immutable payload changed: %#v err=%v", persisted, err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO execution_events(event_id,execution_id,trace_id,payload,occurred_at,sequence) VALUES('bad-id','exec','trace','{"event_id":"other"}',now(),0)`); err == nil {
		t.Fatal("payload identity constraint accepted mismatched event_id")
	}
}

func integrationIncident(id, graphID string) contracts.Incident {
	return contracts.Incident{SchemaVersion: contracts.ContractVersion, IncidentID: id, Status: contracts.IncidentReady, FailureOracle: contracts.FailureOracleRef{ID: "duplicate_ledger_effect", Version: "1.0.0"}, SystemPack: contracts.SystemPackRef{ID: "checkout_duplicate_effect", Version: "1.0.0", InterfaceVersion: contracts.ContractVersion}, TraceID: "trace-1", ExecutionID: "exec-1", DetectedAt: "2026-08-29T10:32:02Z", Summary: "duplicate effect", EvidenceEventIDs: []string{"evt-1"}, GraphID: graphID, SanitizationStatus: contracts.SanitizationPass}
}

func integrationGraph(id, incidentID string, eventIDs ...string) contracts.ExecutionGraph {
	nodes := make([]contracts.GraphNode, len(eventIDs))
	for index, eventID := range eventIDs {
		nodes[index] = contracts.GraphNode{EventID: eventID, TimelineIndex: index}
	}
	return contracts.ExecutionGraph{SchemaVersion: contracts.ContractVersion, GraphID: id, IncidentID: incidentID, OrderingPolicyVersion: contracts.ContractVersion, Nodes: nodes, Edges: []contracts.GraphEdge{}}
}
