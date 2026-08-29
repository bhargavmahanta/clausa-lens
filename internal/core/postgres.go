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
