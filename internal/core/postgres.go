package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/causalens/causalens/internal/contracts"
)

type PostgresRepository struct{ db *sql.DB }

func NewPostgresRepository(db *sql.DB) *PostgresRepository { return &PostgresRepository{db: db} }
func (r *PostgresRepository) IngestEvent(ctx context.Context, e contracts.ExecutionEvent) error {
	if err := e.Validate(); err != nil {
		return err
	}
	p, err := json.Marshal(e)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO execution_events(event_id,execution_id,trace_id,replay_run_id,payload,occurred_at,sequence) VALUES($1,$2,$3,$4,$5,$6,$7)`, e.EventID, e.ExecutionID, e.TraceID, e.ReplayRunID, p, e.OccurredAt, e.Sequence); err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return tx.Commit()
}

func (r *PostgresRepository) PutIncident(ctx context.Context, i contracts.Incident, g contracts.ExecutionGraph) error {
	if i.SchemaVersion != "1.0" || i.IncidentID == "" {
		return fmt.Errorf("invalid incident")
	}
	ip, err := json.Marshal(i)
	if err != nil {
		return err
	}
	gp, err := json.Marshal(g)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO incidents(incident_id,status,payload) VALUES($1,$2,$3)`, i.IncidentID, i.Status, ip); err != nil {
		return fmt.Errorf("insert incident: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO execution_graphs(graph_id,incident_id,payload) VALUES($1,$2,$3)`, g.GraphID, i.IncidentID, gp); err != nil {
		return fmt.Errorf("insert graph: %w", err)
	}
	return tx.Commit()
}
