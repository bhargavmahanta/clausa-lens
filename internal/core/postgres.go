package core

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/causalens/causalens/internal/contracts"
	internalgraph "github.com/causalens/causalens/internal/graph"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type RepositoryErrorKind string

const (
	RepositoryNotFound         RepositoryErrorKind = "not_found"
	RepositoryConflict         RepositoryErrorKind = "conflict"
	RepositoryInvalidLifecycle RepositoryErrorKind = "invalid_lifecycle"
	RepositoryInternal         RepositoryErrorKind = "internal"
)

type RepositoryError struct{ Kind RepositoryErrorKind }

func (e *RepositoryError) Error() string { return "repository: " + string(e.Kind) }
func (e *RepositoryError) Is(target error) bool {
	t, ok := target.(*RepositoryError)
	return ok && e.Kind == t.Kind
}

var (
	ErrNotFound         = &RepositoryError{Kind: RepositoryNotFound}
	ErrConflict         = &RepositoryError{Kind: RepositoryConflict}
	ErrDuplicate        = ErrConflict
	ErrInvalidLifecycle = &RepositoryError{Kind: RepositoryInvalidLifecycle}
	ErrInternal         = &RepositoryError{Kind: RepositoryInternal}
)

type PostgresRepository struct{ db *sql.DB }

func OpenPostgres(databaseURL string) (*sql.DB, error)     { return sql.Open("pgx", databaseURL) }
func NewPostgresRepository(db *sql.DB) *PostgresRepository { return &PostgresRepository{db: db} }

func repositoryError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23505" || pgErr.Code == "23503") {
		return ErrConflict
	}
	return ErrInternal
}

func payload(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, ErrInternal
	}
	return b, nil
}

func decodePayload[T any](b []byte) (T, error) {
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return v, ErrInternal
	}
	return v, nil
}

func (r *PostgresRepository) IngestEvent(ctx context.Context, e contracts.ExecutionEvent) error {
	if err := e.Validate(); err != nil {
		return err
	}
	p, err := payload(e)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return repositoryError(err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO execution_events(event_id,execution_id,trace_id,replay_run_id,payload,occurred_at,sequence) VALUES($1,$2,$3,$4,$5,$6,$7)`, e.EventID, e.ExecutionID, e.TraceID, nullableString(e.ReplayRunID), p, e.OccurredAt, e.Sequence)
	if err != nil {
		return repositoryError(err)
	}
	return repositoryError(tx.Commit())
}

func (r *PostgresRepository) GetEvent(ctx context.Context, id string) (contracts.ExecutionEvent, error) {
	return getJSON[contracts.ExecutionEvent](ctx, r.db, `SELECT payload FROM execution_events WHERE event_id=$1`, id)
}

// EventsForExecution returns every persisted event for one execution in the
// deterministic chronological order (occurred_at, then sequence, then
// event_id). This is the query the post-ingestion detector uses to evaluate an
// accumulated execution against the failure oracle.
func (r *PostgresRepository) EventsForExecution(ctx context.Context, executionID string) ([]contracts.ExecutionEvent, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT payload FROM execution_events WHERE execution_id=$1 ORDER BY occurred_at, sequence, event_id`, executionID)
	if err != nil {
		return nil, repositoryError(err)
	}
	defer rows.Close()
	return scanEvents(ctx, rows)
}

func (r *PostgresRepository) PutIncident(ctx context.Context, i contracts.Incident, g contracts.ExecutionGraph) error {
	if i.Validate() != nil || g.Validate() != nil || g.IncidentID != i.IncidentID || i.GraphID != g.GraphID {
		return ErrInternal
	}
	ip, err := payload(i)
	if err != nil {
		return err
	}
	gp, err := payload(g)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return repositoryError(err)
	}
	defer tx.Rollback()
	events := make([]contracts.ExecutionEvent, 0, len(g.Nodes))
	nodeEvents := make(map[string]struct{}, len(g.Nodes))
	for _, node := range g.Nodes {
		var raw []byte
		if err := tx.QueryRowContext(ctx, `SELECT payload FROM execution_events WHERE event_id=$1`, node.EventID).Scan(&raw); err != nil {
			return ErrInternal
		}
		event, err := decodePayload[contracts.ExecutionEvent](raw)
		if err != nil || event.ExecutionID != i.ExecutionID || event.TraceID != i.TraceID {
			return ErrInternal
		}
		nodeEvents[node.EventID] = struct{}{}
		events = append(events, event)
	}
	for _, evidenceID := range i.EvidenceEventIDs {
		if _, exists := nodeEvents[evidenceID]; !exists {
			return ErrInternal
		}
	}
	if _, err := internalgraph.Order(events, g); err != nil {
		return ErrInternal
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO incidents(incident_id,status,detected_at,payload) VALUES($1,$2,$3,$4)`, i.IncidentID, i.Status, i.DetectedAt, ip); err != nil {
		return repositoryError(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO execution_graphs(graph_id,incident_id,payload) VALUES($1,$2,$3)`, g.GraphID, i.IncidentID, gp); err != nil {
		return repositoryError(err)
	}
	return repositoryError(tx.Commit())
}

func (r *PostgresRepository) PutCapsule(ctx context.Context, c contracts.ReplayCapsule) error {
	if err := c.Validate(); err != nil {
		return err
	}
	p, err := payload(c)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO replay_capsules(capsule_id,incident_id,created_at,payload) VALUES($1,$2,$3,$4)`, c.CapsuleID, c.Source.IncidentID, c.CreatedAt, p)
	return repositoryError(err)
}
func (r *PostgresRepository) GetCapsule(ctx context.Context, id string) (contracts.ReplayCapsule, error) {
	return getJSON[contracts.ReplayCapsule](ctx, r.db, `SELECT payload FROM replay_capsules WHERE capsule_id=$1`, id)
}

func (r *PostgresRepository) PutRun(ctx context.Context, run contracts.ReplayRun) error {
	if err := run.Validate(); err != nil {
		return err
	}
	p, err := payload(run)
	if err != nil {
		return err
	}
	if run.RunType == contracts.RunTypeBaseline {
		_, err = r.db.ExecContext(ctx, `INSERT INTO replay_runs(run_id,capsule_id,status,outcome,payload) VALUES($1,$2,$3,$4,$5)`, run.RunID, run.CapsuleID, run.Status, nullableOutcome(run.Outcome), p)
		return repositoryError(err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return repositoryError(err)
	}
	defer tx.Rollback()
	baseline, err := getJSON[contracts.ReplayRun](ctx, tx, `SELECT payload FROM replay_runs WHERE run_id=$1 FOR UPDATE`, run.BaselineRunID)
	if err != nil {
		return err
	}
	if baseline.RunID != run.BaselineRunID || baseline.RunType != contracts.RunTypeBaseline || baseline.Status != contracts.ReplayRunCompleted || baseline.Outcome != contracts.ReplayOutcomeReproduced || baseline.IsolationEvidence == nil || baseline.IsolationEvidence.Verdict != contracts.VerdictPass || baseline.CapsuleID != run.CapsuleID || baseline.CapsuleHash != run.CapsuleHash || baseline.Intervention != nil {
		return ErrInvalidLifecycle
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO replay_runs(run_id,capsule_id,status,outcome,payload) VALUES($1,$2,$3,$4,$5)`, run.RunID, run.CapsuleID, run.Status, nullableOutcome(run.Outcome), p); err != nil {
		return repositoryError(err)
	}
	return repositoryError(tx.Commit())
}
func (r *PostgresRepository) GetRun(ctx context.Context, id string) (contracts.ReplayRun, error) {
	return getJSON[contracts.ReplayRun](ctx, r.db, `SELECT payload FROM replay_runs WHERE run_id=$1`, id)
}

// ListRuns returns runs in the given status, in deterministic run_id order.
func (r *PostgresRepository) ListRuns(ctx context.Context, status contracts.ReplayRunStatus) ([]contracts.ReplayRun, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT payload FROM replay_runs WHERE status=$1 ORDER BY run_id`, status)
	if err != nil {
		return nil, repositoryError(err)
	}
	defer rows.Close()
	return scanRuns(ctx, rows)
}

func scanRuns(ctx context.Context, rows *sql.Rows) ([]contracts.ReplayRun, error) {
	runs := []contracts.ReplayRun{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, ErrInternal
		}
		run, err := decodePayload[contracts.ReplayRun](raw)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, ErrInternal
	}
	return runs, nil
}

func (r *PostgresRepository) PutDiff(ctx context.Context, d contracts.ReplayDiff) error {
	if err := d.Validate(); err != nil {
		return err
	}
	p, err := payload(d)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return repositoryError(err)
	}
	defer tx.Rollback()
	baseline, err := getJSON[contracts.ReplayRun](ctx, tx, `SELECT payload FROM replay_runs WHERE run_id=$1 FOR UPDATE`, d.BaselineRunID)
	if err != nil {
		return err
	}
	comparison, err := getJSON[contracts.ReplayRun](ctx, tx, `SELECT payload FROM replay_runs WHERE run_id=$1 FOR UPDATE`, d.ComparisonRunID)
	if err != nil {
		return err
	}
	if baseline.Status != contracts.ReplayRunCompleted || baseline.RunType != contracts.RunTypeBaseline || baseline.Outcome != contracts.ReplayOutcomeReproduced || baseline.Intervention != nil || comparison.Status != contracts.ReplayRunCompleted || comparison.RunType != contracts.RunTypeWhatIf || comparison.BaselineRunID != baseline.RunID || comparison.CapsuleHash != baseline.CapsuleHash || comparison.Intervention == nil || !reflect.DeepEqual(*comparison.Intervention, d.Intervention) {
		return ErrInvalidLifecycle
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO replay_diffs(diff_id,baseline_run_id,comparison_run_id,payload) VALUES($1,$2,$3,$4)`, d.DiffID, d.BaselineRunID, d.ComparisonRunID, p); err != nil {
		return repositoryError(err)
	}
	return repositoryError(tx.Commit())
}
func (r *PostgresRepository) GetDiff(ctx context.Context, id string) (contracts.ReplayDiff, error) {
	return getJSON[contracts.ReplayDiff](ctx, r.db, `SELECT payload FROM replay_diffs WHERE diff_id=$1`, id)
}

// ResetCounts reports how many incident and run rows a Reset cleared.
type ResetCounts struct {
	Incidents int
	Runs      int
}

// Reset clears replay and capture state in foreign-key order and returns the
// counts of cleared incidents and runs. It is used by the /demo/reset route.
func (r *PostgresRepository) Reset(ctx context.Context) (ResetCounts, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return ResetCounts{}, repositoryError(err)
	}
	defer tx.Rollback()
	incidents, err := countRows(ctx, tx, `incidents`)
	if err != nil {
		return ResetCounts{}, err
	}
	runs, err := countRows(ctx, tx, `replay_runs`)
	if err != nil {
		return ResetCounts{}, err
	}
	for _, table := range []string{"replay_diffs", "replay_runs", "replay_capsules", "execution_graphs", "incidents", "execution_events"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table); err != nil {
			return ResetCounts{}, repositoryError(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return ResetCounts{}, repositoryError(err)
	}
	return ResetCounts{Incidents: incidents, Runs: runs}, nil
}

func countRows(ctx context.Context, q rowQueryer, table string) (int, error) {
	var n int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		return 0, repositoryError(err)
	}
	return n, nil
}

// EventsForRun loads a run's replayed events (matching replay_run_id) in a
// stable chronological order. It never falls back to the incident's original
// events or the capsule's event_ids, so a run with no recorded replay events
// returns an empty slice and the worker can fail it rather than re-score source
// evidence.
func (r *PostgresRepository) EventsForRun(ctx context.Context, run contracts.ReplayRun) ([]contracts.ExecutionEvent, error) {
	events, err := loadEventsForRun(ctx, r.db, run)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, nil
	}
	return events, nil
}

func (r *PostgresRepository) GraphsForRun(ctx context.Context, run contracts.ReplayRun) (contracts.ExecutionGraph, error) {
	events, err := r.EventsForRun(ctx, run)
	if err != nil {
		return contracts.ExecutionGraph{}, err
	}
	return BuildRunGraph(run.RunID, events)
}

type eventQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func loadEventsForRun(ctx context.Context, q eventQueryer, run contracts.ReplayRun) ([]contracts.ExecutionEvent, error) {
	rows, err := q.QueryContext(ctx, `SELECT payload FROM execution_events WHERE replay_run_id=$1 ORDER BY occurred_at, sequence, event_id`, run.RunID)
	if err != nil {
		return nil, repositoryError(err)
	}
	defer rows.Close()
	return scanEvents(ctx, rows)
}

func scanEvents(ctx context.Context, rows *sql.Rows) ([]contracts.ExecutionEvent, error) {
	events := []contracts.ExecutionEvent{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, ErrInternal
		}
		event, err := decodePayload[contracts.ExecutionEvent](raw)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, ErrInternal
	}
	return events, nil
}

type rowQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func getJSON[T any](ctx context.Context, q rowQueryer, query string, args ...any) (T, error) {
	var raw []byte
	if err := q.QueryRowContext(ctx, query, args...).Scan(&raw); err != nil {
		var zero T
		return zero, repositoryError(err)
	}
	return decodePayload[T](raw)
}

func nullableOutcome(outcome contracts.ReplayOutcome) any {
	if outcome == "" {
		return nil
	}
	return outcome
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func legalTransition(from, to contracts.ReplayRunStatus) bool {
	switch from {
	case contracts.ReplayRunCreated:
		return to == contracts.ReplayRunValidating
	case contracts.ReplayRunValidating:
		return to == contracts.ReplayRunRunning || to == contracts.ReplayRunBlocked || to == contracts.ReplayRunFailed
	case contracts.ReplayRunRunning:
		return to == contracts.ReplayRunCompleted || to == contracts.ReplayRunBlocked || to == contracts.ReplayRunFailed
	default:
		return false
	}
}

func validRunState(run contracts.ReplayRun) bool {
	terminal := run.Status == contracts.ReplayRunCompleted || run.Status == contracts.ReplayRunFailed || run.Status == contracts.ReplayRunBlocked
	if terminal != (run.CompletedAt != "") {
		return false
	}
	if run.Status == contracts.ReplayRunCompleted {
		if run.Outcome == "" || run.EffectSummary == nil || run.FailureOracleResult == nil || run.IsolationEvidence == nil || run.IsolationEvidence.Verdict != contracts.VerdictPass || run.Error != nil {
			return false
		}
		if run.RunType == contracts.RunTypeBaseline {
			return run.Outcome == contracts.ReplayOutcomeReproduced || run.Outcome == contracts.ReplayOutcomeNotReproduced || run.Outcome == contracts.ReplayOutcomeInconclusive
		}
		if run.RunType == contracts.RunTypeWhatIf {
			return run.Outcome == contracts.ReplayOutcomeMitigated || run.Outcome == contracts.ReplayOutcomeUnchanged || run.Outcome == contracts.ReplayOutcomeInconclusive
		}
		return false
	}
	if run.Outcome != "" {
		return false
	}
	if run.Status == contracts.ReplayRunFailed || run.Status == contracts.ReplayRunBlocked {
		return run.Error != nil
	}
	return run.Error == nil
}

func (r *PostgresRepository) TransitionRun(ctx context.Context, from contracts.ReplayRunStatus, run contracts.ReplayRun) error {
	if !legalTransition(from, run.Status) || run.Validate() != nil {
		return ErrInvalidLifecycle
	}
	p, err := payload(run)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return repositoryError(err)
	}
	defer tx.Rollback()
	stored, err := getJSON[contracts.ReplayRun](ctx, tx, `SELECT payload FROM replay_runs WHERE run_id=$1 FOR UPDATE`, run.RunID)
	if err != nil {
		return err
	}
	if stored.Status != from || stored.ExecutionID != run.ExecutionID || stored.CapsuleID != run.CapsuleID || stored.CapsuleHash != run.CapsuleHash || stored.RunType != run.RunType || stored.BaselineRunID != run.BaselineRunID || !reflect.DeepEqual(stored.Intervention, run.Intervention) || stored.TrialNumber != run.TrialNumber {
		return ErrInvalidLifecycle
	}
	result, err := tx.ExecContext(ctx, `UPDATE replay_runs SET status=$3,outcome=$4,payload=$5 WHERE run_id=$1 AND status=$2 AND capsule_id=$6`, run.RunID, from, run.Status, nullableOutcome(run.Outcome), p, run.CapsuleID)
	if err != nil {
		return repositoryError(err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return ErrInternal
	}
	if n == 0 {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM replay_runs WHERE run_id=$1)`, run.RunID).Scan(&exists); err != nil {
			return repositoryError(err)
		}
		if exists {
			return ErrInvalidLifecycle
		}
		return ErrNotFound
	}
	return repositoryError(tx.Commit())
}

type incidentCursor struct {
	DetectedAt time.Time `json:"detected_at"`
	IncidentID string    `json:"incident_id"`
}

func encodeCursor(c incidentCursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}
func decodeCursor(s string) (incidentCursor, error) {
	var c incidentCursor
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return c, ErrConflict
	}
	if err := json.Unmarshal(b, &c); err != nil || c.DetectedAt.IsZero() || c.IncidentID == "" {
		return c, ErrConflict
	}
	return c, nil
}

func (r *PostgresRepository) ListIncidents(ctx context.Context, q contracts.IncidentListQuery) (contracts.IncidentListResponse, error) {
	limit := 20
	if q.Limit != nil {
		limit = *q.Limit
	}
	if limit < 1 || limit > 100 {
		return contracts.IncidentListResponse{}, ErrConflict
	}
	args := []any{}
	where := []string{}
	if q.Status != "" {
		args = append(args, q.Status)
		where = append(where, fmt.Sprintf("status=$%d", len(args)))
	}
	if q.Cursor != "" {
		c, err := decodeCursor(q.Cursor)
		if err != nil {
			return contracts.IncidentListResponse{}, err
		}
		args = append(args, c.DetectedAt, c.IncidentID)
		where = append(where, fmt.Sprintf("(detected_at,incident_id)<($%d,$%d)", len(args)-1, len(args)))
	}
	query := `SELECT payload,detected_at,incident_id FROM incidents`
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, " AND ")
	}
	args = append(args, limit+1)
	query += fmt.Sprintf(` ORDER BY detected_at DESC,incident_id DESC LIMIT $%d`, len(args))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return contracts.IncidentListResponse{}, repositoryError(err)
	}
	defer rows.Close()
	response := contracts.IncidentListResponse{Items: []contracts.Incident{}}
	var last incidentCursor
	for rows.Next() {
		var raw []byte
		var detected time.Time
		var id string
		if err := rows.Scan(&raw, &detected, &id); err != nil {
			return contracts.IncidentListResponse{}, ErrInternal
		}
		incident, err := decodePayload[contracts.Incident](raw)
		if err != nil {
			return contracts.IncidentListResponse{}, err
		}
		if len(response.Items) == limit {
			response.NextCursor = encodeCursor(last)
			break
		}
		response.Items = append(response.Items, incident)
		last = incidentCursor{detected, id}
	}
	if err := rows.Err(); err != nil {
		return contracts.IncidentListResponse{}, ErrInternal
	}
	return response, nil
}

func (r *PostgresRepository) GetIncidentDetail(ctx context.Context, id string) (contracts.IncidentDetailResponse, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return contracts.IncidentDetailResponse{}, repositoryError(err)
	}
	defer tx.Rollback()
	var iraw, graw []byte
	err = tx.QueryRowContext(ctx, `SELECT i.payload,g.payload FROM incidents i JOIN execution_graphs g ON g.incident_id=i.incident_id WHERE i.incident_id=$1`, id).Scan(&iraw, &graw)
	if err != nil {
		return contracts.IncidentDetailResponse{}, repositoryError(err)
	}
	i, err := decodePayload[contracts.Incident](iraw)
	if err != nil {
		return contracts.IncidentDetailResponse{}, err
	}
	g, err := decodePayload[contracts.ExecutionGraph](graw)
	if err != nil {
		return contracts.IncidentDetailResponse{}, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT e.payload FROM jsonb_array_elements($1::jsonb->'nodes') n JOIN execution_events e ON e.event_id=n->>'event_id' ORDER BY (n->>'timeline_index')::bigint`, graw)
	if err != nil {
		return contracts.IncidentDetailResponse{}, repositoryError(err)
	}
	defer rows.Close()
	d := contracts.IncidentDetailResponse{Incident: i, Graph: g, Events: []contracts.ExecutionEvent{}}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return contracts.IncidentDetailResponse{}, ErrInternal
		}
		e, err := decodePayload[contracts.ExecutionEvent](raw)
		if err != nil {
			return contracts.IncidentDetailResponse{}, err
		}
		d.Events = append(d.Events, e)
	}
	if err := rows.Err(); err != nil {
		return contracts.IncidentDetailResponse{}, ErrInternal
	}
	if len(d.Events) != len(g.Nodes) {
		return contracts.IncidentDetailResponse{}, ErrInternal
	}
	nodes := append([]contracts.GraphNode(nil), g.Nodes...)
	sort.Slice(nodes, func(a, b int) bool { return nodes[a].TimelineIndex < nodes[b].TimelineIndex })
	for index, event := range d.Events {
		if nodes[index].TimelineIndex != index || nodes[index].EventID != event.EventID {
			return contracts.IncidentDetailResponse{}, ErrInternal
		}
	}
	if err := tx.Commit(); err != nil {
		return contracts.IncidentDetailResponse{}, repositoryError(err)
	}
	return d, nil
}
