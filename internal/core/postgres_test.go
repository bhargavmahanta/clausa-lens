package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/causalens/causalens/internal/contracts"
	"github.com/jackc/pgx/v5/pgconn"
)

func repositoryMock(t *testing.T) (*PostgresRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewPostgresRepository(db), mock
}

func eventFixture() contracts.ExecutionEvent {
	return contracts.ExecutionEvent{SchemaVersion: "1.0", EventID: "evt-1", ExecutionID: "exec-1", TraceID: "trace-1", Component: contracts.ComponentRef{Name: "checkout", Instance: "one"}, Operation: contracts.OperationRef{Name: "run", Kind: contracts.OperationInternal}, EventType: contracts.EventStart, Attempt: 1, LogicalOperationID: "op-1", OccurredAt: "2026-08-29T10:32:01Z", Sequence: 0, Status: contracts.EventRunning, Attributes: map[string]any{}}
}

func runFixture(status contracts.ReplayRunStatus) contracts.ReplayRun {
	run := contracts.ReplayRun{SchemaVersion: contracts.ContractVersion, RunID: "run-1", ExecutionID: "replay-exec", CapsuleID: "cap-1", CapsuleHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", RunType: contracts.RunTypeBaseline, TrialNumber: 1, Status: status, ObservedEventIDs: []string{}}
	if status == contracts.ReplayRunRunning {
		run.StartedAt = "2026-08-29T10:00:00Z"
	}
	return run
}

func capsuleFixture() contracts.ReplayCapsule {
	return contracts.ReplayCapsule{SchemaVersion: contracts.ContractVersion, CapsuleID: "cap-1", CreatedAt: "2026-08-29T10:33:00Z", Source: contracts.CapsuleSource{IncidentID: "inc-1", TraceID: "trace-1", ExecutionID: "exec-1", CaptureEnvironment: contracts.CaptureDemo, CapturedAt: "2026-08-29T10:32:01Z"}, SystemPack: contracts.SystemPackRef{ID: "checkout_duplicate_effect", Version: "1.0.0", InterfaceVersion: contracts.ContractVersion}, Trigger: contracts.Trigger{RequestOrMessage: map[string]any{"method": "POST"}, SanitizedHeaders: map[string]string{}}, EventIDs: []string{"evt-1"}, GraphID: "graph-1", StateFixtures: []contracts.StateFixture{{FixtureID: "state-1", Kind: contracts.StateFixturePostgresRowset, ContentRef: "fixture://golden/state-1", ContentDigest: strings.Repeat("b", 64), SanitizationStatus: contracts.SanitizationPass, ResetStrategy: contracts.FixtureTruncateAndLoad}}, DependencyFixtures: []contracts.DependencyFixture{{FixtureID: "dependency-1", Dependency: contracts.DependencyPaymentSimulator, RequestMatch: map[string]any{}, Response: map[string]any{}, LatencyMS: 350, FailureMode: contracts.FailureModeNone, InvocationLimit: 2}}, TimingPolicy: contracts.TimingPolicy{ClockToleranceMS: 5, TimeoutMS: 200}, ReplayPlan: contracts.ReplayPlan{Entrypoint: "gateway.checkout", RequiredComponents: []string{"gateway", "checkout", "payment", "ledger"}, FixtureLoadOrder: []string{"state-1", "dependency-1"}, ResetStrategy: contracts.ReplayResetGoldenV1}, FailureOracle: contracts.FailureOracleSpec{ID: "duplicate_ledger_effect", Version: "1.0.0", ExpectedMatch: true, ExpectedEffectSummary: contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2}}, AllowedInterventions: []contracts.InterventionSpec{{Type: contracts.InterventionPaymentLatency, ValueType: contracts.InterventionValueInteger, Unit: contracts.InterventionUnitMilliseconds, Minimum: 0, Maximum: 5000}}, Safety: contracts.SafetyPolicy{PolicyVersion: contracts.ContractVersion, SanitizationStatus: contracts.SanitizationPass, BlockedDestinations: []string{"production-databases"}, AllowedDestinations: []string{"payment-simulator", "replay-postgres"}, CredentialProfile: contracts.CredentialReplayOnly}, Integrity: contracts.Integrity{Algorithm: contracts.IntegritySHA256, Digest: strings.Repeat("a", 64)}}
}

func diffFixture() contracts.ReplayDiff {
	baseline := contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2}
	comparison := contracts.EffectSummary{PaymentAttemptCount: 1, LedgerCommitCount: 1}
	oracle := func(matched bool, summary contracts.EffectSummary) contracts.OracleResult {
		return contracts.OracleResult{Oracle: contracts.FailureOracleRef{ID: "duplicate_ledger_effect", Version: "1.0.0"}, Matched: matched, EffectSummary: summary, RequiredEvidenceEventIDs: []string{"evt-1"}, Explanation: "evidence"}
	}
	return contracts.ReplayDiff{SchemaVersion: contracts.ContractVersion, DiffID: "diff-1", BaselineRunID: "base", ComparisonRunID: "compare", AlignmentVersion: contracts.ContractVersion, Intervention: contracts.Intervention{Type: contracts.InterventionPaymentLatency, From: 350, To: 50, Unit: contracts.InterventionUnitMilliseconds}, BaselineOracleResult: oracle(true, baseline), ComparisonOracleResult: oracle(false, comparison), MatchedEvents: []contracts.EventAlignment{}, AddedEventIDs: []string{}, RemovedEventIDs: []string{"evt-1"}, ChangedEvents: []contracts.EventChange{}, FirstMeaningfulDivergence: &contracts.FirstDivergence{BaselineEventID: "evt-1", ComparisonEventID: "evt-2", Rule: "PAYMENT_COMPLETES_BEFORE_TIMEOUT", BaselineValue: "TIMEOUT", ComparisonValue: "SUCCESS", BaselineTimelineIndex: 1, ComparisonTimelineIndex: 1}, BaselineEffectSummary: baseline, ComparisonEffectSummary: comparison, EffectDelta: contracts.EffectDelta{PaymentAttemptCount: -1, LedgerCommitCount: -1}, EvidenceSummary: "duplicate effect disappeared", Limitations: []string{}}
}

func TestIngestEventCommitsCanonicalPayload(t *testing.T) {
	r, mock := repositoryMock(t)
	e := eventFixture()
	payload, _ := json.Marshal(e)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO execution_events").WithArgs(e.EventID, e.ExecutionID, e.TraceID, nil, payload, e.OccurredAt, e.Sequence).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := r.IngestEvent(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestIngestEventMapsDuplicateWithoutDriverLeak(t *testing.T) {
	r, mock := repositoryMock(t)
	e := eventFixture()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO execution_events").WillReturnError(&pgconn.PgError{Code: "23505", Message: "driver detail"})
	mock.ExpectRollback()
	err := r.IngestEvent(context.Background(), e)
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("got %v", err)
	}
	if regexp.MustCompile("driver detail|23505").MatchString(err.Error()) {
		t.Fatalf("driver detail leaked: %v", err)
	}
}

func TestPutIncidentRollsBackWhenGraphInsertFails(t *testing.T) {
	r, mock := repositoryMock(t)
	i := integrationIncident("inc-1", "graph-1")
	g := integrationGraph("graph-1", "inc-1", "evt-1")
	mock.ExpectBegin()
	eventPayload, _ := json.Marshal(eventFixture())
	mock.ExpectQuery("SELECT payload FROM execution_events").WithArgs("evt-1").WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(eventPayload))
	mock.ExpectExec("INSERT INTO incidents").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO execution_graphs").WillReturnError(errors.New("broken graph"))
	mock.ExpectRollback()
	if err := r.PutIncident(context.Background(), i, g); !errors.Is(err, ErrInternal) {
		t.Fatalf("got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPutIncidentRejectsHardEdgeThatContradictsTimeline(t *testing.T) {
	r, mock := repositoryMock(t)
	i := integrationIncident("inc-1", "graph-1")
	g := integrationGraph("graph-1", "inc-1", "evt-1", "evt-2")
	g.Edges = []contracts.GraphEdge{{EdgeID: "edge-1", FromEventID: "evt-2", ToEventID: "evt-1", Type: contracts.GraphEdgeDependency}}
	e1, e2 := eventFixture(), eventFixture()
	e2.EventID = "evt-2"
	p1, _ := json.Marshal(e1)
	p2, _ := json.Marshal(e2)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT payload FROM execution_events").WithArgs("evt-1").WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(p1))
	mock.ExpectQuery("SELECT payload FROM execution_events").WithArgs("evt-2").WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(p2))
	mock.ExpectRollback()
	if err := r.PutIncident(context.Background(), i, g); !errors.Is(err, ErrInternal) {
		t.Fatalf("got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPutIncidentRejectsUnresolvedEvidenceEvents(t *testing.T) {
	r, mock := repositoryMock(t)
	i := integrationIncident("inc-1", "graph-1")
	i.EvidenceEventIDs = []string{"evt-1", "missing"}
	g := integrationGraph("graph-1", "inc-1", "evt-1")
	mock.ExpectBegin()
	eventPayload, _ := json.Marshal(eventFixture())
	mock.ExpectQuery("SELECT payload FROM execution_events").WithArgs("evt-1").WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(eventPayload))
	mock.ExpectRollback()
	if err := r.PutIncident(context.Background(), i, g); !errors.Is(err, ErrInternal) {
		t.Fatalf("got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPutRunRejectsIncompleteContract(t *testing.T) {
	r := NewPostgresRepository(nil)
	err := r.PutRun(context.Background(), contracts.ReplayRun{SchemaVersion: contracts.ContractVersion, RunID: "run-1", CapsuleID: "cap-1", Status: contracts.ReplayRunCreated})
	if err == nil {
		t.Fatal("incomplete run was persisted")
	}
}

func TestPutRunAuthorizesWhatIfAgainstStoredBaseline(t *testing.T) {
	r, mock := repositoryMock(t)
	baseline := completedBaselineFixture("base")
	whatIf := runFixture(contracts.ReplayRunCreated)
	whatIf.RunID, whatIf.RunType, whatIf.BaselineRunID = "what-if", contracts.RunTypeWhatIf, baseline.RunID
	whatIf.Intervention = &contracts.Intervention{Type: contracts.InterventionPaymentLatency, From: 350, To: 50, Unit: contracts.InterventionUnitMilliseconds}
	raw, _ := json.Marshal(baseline)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT payload FROM replay_runs.*FOR UPDATE").WithArgs(baseline.RunID).WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(raw))
	mock.ExpectExec("INSERT INTO replay_runs").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := r.PutRun(context.Background(), whatIf); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPutRunRejectsUnsafeWhatIfBaseline(t *testing.T) {
	r, mock := repositoryMock(t)
	baseline := completedBaselineFixture("base")
	baseline.IsolationEvidence.Verdict = contracts.VerdictFail
	whatIf := runFixture(contracts.ReplayRunCreated)
	whatIf.RunType, whatIf.BaselineRunID = contracts.RunTypeWhatIf, baseline.RunID
	whatIf.Intervention = &contracts.Intervention{Type: contracts.InterventionPaymentLatency, From: 350, To: 50, Unit: contracts.InterventionUnitMilliseconds}
	raw, _ := json.Marshal(baseline)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT payload FROM replay_runs.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(raw))
	mock.ExpectRollback()
	if err := r.PutRun(context.Background(), whatIf); !errors.Is(err, ErrInvalidLifecycle) {
		t.Fatalf("got %v", err)
	}
}

func TestPutCapsuleAndDiffRejectInvalidContractsBeforeSQL(t *testing.T) {
	r, mock := repositoryMock(t)
	if err := r.PutCapsule(context.Background(), contracts.ReplayCapsule{}); err == nil || errors.Is(err, ErrInternal) {
		t.Fatalf("invalid capsule was not rejected by contract validation: %v", err)
	}
	if err := r.PutDiff(context.Background(), contracts.ReplayDiff{}); err == nil || errors.Is(err, ErrInternal) {
		t.Fatalf("invalid diff was not rejected by contract validation: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPutDiffRejectsComparisonForDifferentBaseline(t *testing.T) {
	r, mock := repositoryMock(t)
	d := diffFixture()
	baseline := completedBaselineFixture(d.BaselineRunID)
	comparison := completedWhatIfFixture(d.ComparisonRunID, baseline, d.Intervention)
	comparison.BaselineRunID = "other-baseline"
	bp, _ := json.Marshal(baseline)
	cp, _ := json.Marshal(comparison)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT payload FROM replay_runs.*FOR UPDATE").WithArgs(d.BaselineRunID).WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(bp))
	mock.ExpectQuery("SELECT payload FROM replay_runs.*FOR UPDATE").WithArgs(d.ComparisonRunID).WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(cp))
	mock.ExpectRollback()
	if err := r.PutDiff(context.Background(), d); !errors.Is(err, ErrInvalidLifecycle) {
		t.Fatalf("got %v", err)
	}
}

func TestGetEventDecodesAndClassifiesReads(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		r, mock := repositoryMock(t)
		want := eventFixture()
		payload, _ := json.Marshal(want)
		mock.ExpectQuery("SELECT payload FROM execution_events").WithArgs(want.EventID).WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(payload))
		got, err := r.GetEvent(context.Background(), want.EventID)
		if err != nil || got.EventID != want.EventID {
			t.Fatalf("got %#v, %v", got, err)
		}
	})
	t.Run("not found", func(t *testing.T) {
		r, mock := repositoryMock(t)
		mock.ExpectQuery("SELECT payload FROM execution_events").WillReturnError(sql.ErrNoRows)
		_, err := r.GetEvent(context.Background(), "missing")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("internal", func(t *testing.T) {
		r, mock := repositoryMock(t)
		mock.ExpectQuery("SELECT payload FROM execution_events").WillReturnError(errors.New("wire secret"))
		_, err := r.GetEvent(context.Background(), "e")
		if !errors.Is(err, ErrInternal) || regexp.MustCompile("wire secret").MatchString(err.Error()) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestResourcePayloadRoundTrips(t *testing.T) {
	tests := []struct {
		name, table, id string
		put             func(*PostgresRepository) error
		get             func(*PostgresRepository) (string, error)
		payload         any
	}{
		{"capsule", "replay_capsules", "cap-1", func(r *PostgresRepository) error {
			return r.PutCapsule(context.Background(), capsuleFixture())
		}, func(r *PostgresRepository) (string, error) {
			v, e := r.GetCapsule(context.Background(), "cap-1")
			return v.CapsuleID, e
		}, capsuleFixture()},
		{"run", "replay_runs", "run-1", func(r *PostgresRepository) error {
			return r.PutRun(context.Background(), runFixture(contracts.ReplayRunCreated))
		}, func(r *PostgresRepository) (string, error) {
			v, e := r.GetRun(context.Background(), "run-1")
			return v.RunID, e
		}, runFixture(contracts.ReplayRunCreated)},
		{"diff", "replay_diffs", "diff-1", func(r *PostgresRepository) error {
			return r.PutDiff(context.Background(), diffFixture())
		}, func(r *PostgresRepository) (string, error) {
			v, e := r.GetDiff(context.Background(), "diff-1")
			return v.DiffID, e
		}, diffFixture()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, mock := repositoryMock(t)
			payload, _ := json.Marshal(tt.payload)
			if tt.name == "diff" {
				d := tt.payload.(contracts.ReplayDiff)
				baseline := completedBaselineFixture(d.BaselineRunID)
				comparison := completedWhatIfFixture(d.ComparisonRunID, baseline, d.Intervention)
				baselinePayload, _ := json.Marshal(baseline)
				comparisonPayload, _ := json.Marshal(comparison)
				mock.ExpectBegin()
				mock.ExpectQuery("SELECT payload FROM replay_runs.*FOR UPDATE").WithArgs(d.BaselineRunID).WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(baselinePayload))
				mock.ExpectQuery("SELECT payload FROM replay_runs.*FOR UPDATE").WithArgs(d.ComparisonRunID).WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(comparisonPayload))
				mock.ExpectExec("INSERT INTO replay_diffs").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			} else {
				mock.ExpectExec("INSERT INTO " + tt.table).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			if err := tt.put(r); err != nil {
				t.Fatal(err)
			}
			mock.ExpectQuery("SELECT payload FROM " + tt.table).WithArgs(tt.id).WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(payload))
			id, err := tt.get(r)
			if err != nil || id != tt.id {
				t.Fatalf("got %q, %v", id, err)
			}
		})
	}
}

func TestTransitionRunConditionalLifecycle(t *testing.T) {
	t.Run("legal commit", func(t *testing.T) {
		r, mock := repositoryMock(t)
		run := runFixture(contracts.ReplayRunValidating)
		mock.ExpectBegin()
		stored, _ := json.Marshal(runFixture(contracts.ReplayRunCreated))
		mock.ExpectQuery("SELECT payload FROM replay_runs.*FOR UPDATE").WithArgs(run.RunID).WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(stored))
		mock.ExpectExec("UPDATE replay_runs").WithArgs(run.RunID, contracts.ReplayRunCreated, run.Status, nil, sqlmock.AnyArg(), run.CapsuleID).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		if err := r.TransitionRun(context.Background(), contracts.ReplayRunCreated, run); err != nil {
			t.Fatal(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("illegal rejected before sql", func(t *testing.T) {
		r, mock := repositoryMock(t)
		run := contracts.ReplayRun{RunID: "run-1", Status: contracts.ReplayRunCompleted}
		if err := r.TransitionRun(context.Background(), contracts.ReplayRunCreated, run); !errors.Is(err, ErrInvalidLifecycle) {
			t.Fatalf("got %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("stale state rolls back", func(t *testing.T) {
		r, mock := repositoryMock(t)
		run := runFixture(contracts.ReplayRunRunning)
		mock.ExpectBegin()
		stored, _ := json.Marshal(runFixture(contracts.ReplayRunValidating))
		mock.ExpectQuery("SELECT payload FROM replay_runs.*FOR UPDATE").WithArgs(run.RunID).WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(stored))
		mock.ExpectExec("UPDATE replay_runs").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT EXISTS").WithArgs(run.RunID).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectRollback()
		if err := r.TransitionRun(context.Background(), contracts.ReplayRunValidating, run); !errors.Is(err, ErrInvalidLifecycle) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("completed requires outcome", func(t *testing.T) {
		r, mock := repositoryMock(t)
		run := contracts.ReplayRun{RunID: "run-1", Status: contracts.ReplayRunCompleted}
		if err := r.TransitionRun(context.Background(), contracts.ReplayRunRunning, run); !errors.Is(err, ErrInvalidLifecycle) {
			t.Fatalf("got %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestTransitionRunRejectsImmutablePayloadChange(t *testing.T) {
	r, mock := repositoryMock(t)
	stored := runFixture(contracts.ReplayRunCreated)
	next := stored
	next.Status, next.ExecutionID = contracts.ReplayRunValidating, "changed"
	raw, _ := json.Marshal(stored)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT payload FROM replay_runs.*FOR UPDATE").WithArgs(stored.RunID).WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(raw))
	mock.ExpectRollback()
	if err := r.TransitionRun(context.Background(), contracts.ReplayRunCreated, next); !errors.Is(err, ErrInvalidLifecycle) {
		t.Fatalf("got %v", err)
	}
}

func completedBaselineFixture(id string) contracts.ReplayRun {
	run := runFixture(contracts.ReplayRunCompleted)
	run.RunID, run.Outcome = id, contracts.ReplayOutcomeReproduced
	run.StartedAt, run.CompletedAt = "2026-08-29T10:00:00Z", "2026-08-29T10:00:01Z"
	run.EffectSummary = &contracts.EffectSummary{}
	run.FailureOracleResult = &contracts.OracleResult{Oracle: contracts.FailureOracleRef{ID: "oracle", Version: "1.0.0"}, RequiredEvidenceEventIDs: []string{}, Explanation: "evidence"}
	run.IsolationEvidence = &contracts.IsolationEvidence{PolicyVersion: contracts.ContractVersion, Verdict: contracts.VerdictPass, RuntimeNamespace: "ns", NetworkPolicy: contracts.VerdictPass, CredentialProfile: contracts.CredentialReplayOnly, DatastoreDestinations: []string{}, SimulatorInteractions: []contracts.DependencyInteraction{}, DeniedInteractions: []contracts.DependencyInteraction{}, TeardownResult: contracts.VerdictPass}
	return run
}

func completedWhatIfFixture(id string, baseline contracts.ReplayRun, intervention contracts.Intervention) contracts.ReplayRun {
	run := completedBaselineFixture(id)
	run.RunType, run.BaselineRunID, run.Intervention = contracts.RunTypeWhatIf, baseline.RunID, &intervention
	run.CapsuleID, run.CapsuleHash = baseline.CapsuleID, baseline.CapsuleHash
	run.Outcome = contracts.ReplayOutcomeMitigated
	return run
}

func TestListIncidentsUsesStableFilterCursorAndLimit(t *testing.T) {
	r, mock := repositoryMock(t)
	limit := 1
	first := contracts.Incident{SchemaVersion: "1.0", IncidentID: "inc-2", Status: contracts.IncidentReady, DetectedAt: "2026-08-29T11:00:00Z"}
	second := contracts.Incident{SchemaVersion: "1.0", IncidentID: "inc-1", Status: contracts.IncidentReady, DetectedAt: "2026-08-29T10:00:00Z"}
	p1, _ := json.Marshal(first)
	p2, _ := json.Marshal(second)
	firstTime, _ := time.Parse(time.RFC3339Nano, first.DetectedAt)
	secondTime, _ := time.Parse(time.RFC3339Nano, second.DetectedAt)
	mock.ExpectQuery(`SELECT payload,detected_at,incident_id FROM incidents WHERE status=\$1 ORDER BY detected_at DESC,incident_id DESC LIMIT \$2`).WithArgs(contracts.IncidentReady, 2).WillReturnRows(sqlmock.NewRows([]string{"payload", "detected_at", "incident_id"}).AddRow(p1, firstTime, first.IncidentID).AddRow(p2, secondTime, second.IncidentID)).RowsWillBeClosed()
	got, err := r.ListIncidents(context.Background(), contracts.IncidentListQuery{Status: contracts.IncidentReady, Limit: &limit})
	if err != nil || len(got.Items) != 1 || got.Items[0].IncidentID != "inc-2" || got.NextCursor == "" {
		t.Fatalf("got %#v, %v", got, err)
	}
	mock.ExpectQuery(`WHERE \(detected_at,incident_id\)<\(\$1,\$2\).*LIMIT \$3`).WithArgs(firstTime, first.IncidentID, 2).WillReturnRows(sqlmock.NewRows([]string{"payload", "detected_at", "incident_id"}).AddRow(p2, secondTime, second.IncidentID)).RowsWillBeClosed()
	got, err = r.ListIncidents(context.Background(), contracts.IncidentListQuery{Cursor: got.NextCursor, Limit: &limit})
	if err != nil || len(got.Items) != 1 || got.Items[0].IncidentID != "inc-1" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestListIncidentsReturnsEmptyArray(t *testing.T) {
	r, mock := repositoryMock(t)
	mock.ExpectQuery("SELECT payload,detected_at,incident_id FROM incidents").WithArgs(21).WillReturnRows(sqlmock.NewRows([]string{"payload", "detected_at", "incident_id"})).RowsWillBeClosed()
	got, err := r.ListIncidents(context.Background(), contracts.IncidentListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Items == nil {
		t.Fatal("items must be an empty array, not null")
	}
}

func TestGetIncidentDetailReadsGraphAndTimelineEventsInReadTransaction(t *testing.T) {
	r, mock := repositoryMock(t)
	i := contracts.Incident{SchemaVersion: "1.0", IncidentID: "inc-1", GraphID: "graph-1"}
	g := contracts.ExecutionGraph{SchemaVersion: "1.0", GraphID: "graph-1", IncidentID: "inc-1", Nodes: []contracts.GraphNode{{EventID: "evt-1", TimelineIndex: 0}}}
	e := eventFixture()
	ip, _ := json.Marshal(i)
	gp, _ := json.Marshal(g)
	ep, _ := json.Marshal(e)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT i.payload,g.payload").WithArgs(i.IncidentID).WillReturnRows(sqlmock.NewRows([]string{"incident", "graph"}).AddRow(ip, gp))
	mock.ExpectQuery("SELECT e.payload FROM jsonb_array_elements").WithArgs(gp).WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(ep)).RowsWillBeClosed()
	mock.ExpectCommit()
	got, err := r.GetIncidentDetail(context.Background(), i.IncidentID)
	if err != nil || got.Graph.GraphID != "graph-1" || len(got.Events) != 1 || got.Events[0].EventID != "evt-1" {
		t.Fatalf("got %#v, %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetIncidentDetailRejectsMissingGraphEvents(t *testing.T) {
	r, mock := repositoryMock(t)
	i := contracts.Incident{SchemaVersion: "1.0", IncidentID: "inc-1", GraphID: "graph-1"}
	g := contracts.ExecutionGraph{SchemaVersion: "1.0", GraphID: "graph-1", IncidentID: "inc-1", OrderingPolicyVersion: "1.0", Nodes: []contracts.GraphNode{{EventID: "missing", TimelineIndex: 0}}, Edges: []contracts.GraphEdge{}}
	ip, _ := json.Marshal(i)
	gp, _ := json.Marshal(g)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT i.payload,g.payload").WithArgs(i.IncidentID).WillReturnRows(sqlmock.NewRows([]string{"incident", "graph"}).AddRow(ip, gp))
	mock.ExpectQuery("SELECT e.payload FROM jsonb_array_elements").WithArgs(gp).WillReturnRows(sqlmock.NewRows([]string{"payload"})).RowsWillBeClosed()
	mock.ExpectRollback()

	if _, err := r.GetIncidentDetail(context.Background(), i.IncidentID); !errors.Is(err, ErrInternal) {
		t.Fatalf("got %v, want internal consistency error", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
