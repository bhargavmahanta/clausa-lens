package core

import (
	"context"
	"time"

	"github.com/causalens/causalens/internal/contracts"
)

// RunLeaseStore is the claim/lease seam the replay worker uses to recover runs
// a crashed worker left mid-lifecycle without re-executing them. A successful
// claim moves a CREATED run to VALIDATING and grants a lease; an expired lease
// marks the run reclaimable (to be FAILED, never re-run).
type RunLeaseStore interface {
	ClaimRun(ctx context.Context, runID string, lease time.Duration) (contracts.ReplayRun, error)
	RenewLease(ctx context.Context, runID string, lease time.Duration) error
	ReclaimExpiredRuns(ctx context.Context, lease time.Duration) ([]contracts.ReplayRun, error)
}

// ClaimRun atomically claims a run at CREATED (advancing it to VALIDATING) and
// grants a lease, so a single worker owns it. A second worker's claim fails.
func (s *Store) ClaimRun(ctx context.Context, runID string, lease time.Duration) (contracts.ReplayRun, error) {
	if err := ctx.Err(); err != nil {
		return contracts.ReplayRun{}, ErrInternal
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return contracts.ReplayRun{}, ErrNotFound
	}
	if run.Status != contracts.ReplayRunCreated {
		return contracts.ReplayRun{}, ErrInvalidLifecycle
	}
	run.Status = contracts.ReplayRunValidating
	s.runs[runID] = run
	s.leaseUntil[runID] = time.Now().Add(lease)
	return run, nil
}

// RenewLease extends a run's lease (used when the runtime reaches RUNNING).
func (s *Store) RenewLease(ctx context.Context, runID string, lease time.Duration) error {
	if err := ctx.Err(); err != nil {
		return ErrInternal
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[runID]; !ok {
		return ErrNotFound
	}
	s.leaseUntil[runID] = time.Now().Add(lease)
	return nil
}

// ReclaimExpiredRuns returns runs stranded in VALIDATING/RUNNING whose lease has
// expired (or that never had one), so the worker can fail them. It does not
// re-run or re-claim a run; recovery is FAIL-only, preventing duplicate replay.
func (s *Store) ReclaimExpiredRuns(ctx context.Context, _ time.Duration) ([]contracts.ReplayRun, error) {
	if err := ctx.Err(); err != nil {
		return nil, ErrInternal
	}
	now := time.Now()
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []contracts.ReplayRun
	for id, run := range s.runs {
		if run.Status != contracts.ReplayRunValidating && run.Status != contracts.ReplayRunRunning {
			continue
		}
		until, ok := s.leaseUntil[id]
		if !ok || until.Before(now) {
			out = append(out, run)
		}
	}
	return out, nil
}

// ClaimRun atomically claims a run at CREATED and grants a lease (Postgres).
func (r *PostgresRepository) ClaimRun(ctx context.Context, runID string, lease time.Duration) (contracts.ReplayRun, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return contracts.ReplayRun{}, repositoryError(err)
	}
	defer tx.Rollback()
	stored, err := getJSON[contracts.ReplayRun](ctx, tx, `SELECT payload FROM replay_runs WHERE run_id=$1 FOR UPDATE`, runID)
	if err != nil {
		return contracts.ReplayRun{}, err
	}
	if stored.Status != contracts.ReplayRunCreated {
		return contracts.ReplayRun{}, ErrInvalidLifecycle
	}
	stored.Status = contracts.ReplayRunValidating
	p, err := payload(stored)
	if err != nil {
		return contracts.ReplayRun{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE replay_runs SET status=$2, lease_until=$3, payload=$4 WHERE run_id=$1`, runID, stored.Status, time.Now().Add(lease), p); err != nil {
		return contracts.ReplayRun{}, repositoryError(err)
	}
	if err := tx.Commit(); err != nil {
		return contracts.ReplayRun{}, repositoryError(err)
	}
	return stored, nil
}

// RenewLease extends a run's lease (Postgres).
func (r *PostgresRepository) RenewLease(ctx context.Context, runID string, lease time.Duration) error {
	res, err := r.db.ExecContext(ctx, `UPDATE replay_runs SET lease_until=$2 WHERE run_id=$1`, runID, time.Now().Add(lease))
	if err != nil {
		return repositoryError(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return ErrInternal
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ReclaimExpiredRuns returns runs stranded in VALIDATING/RUNNING with an expired
// (or missing) lease (Postgres).
func (r *PostgresRepository) ReclaimExpiredRuns(ctx context.Context, _ time.Duration) ([]contracts.ReplayRun, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT payload FROM replay_runs WHERE status IN ('VALIDATING','RUNNING') AND (lease_until IS NULL OR lease_until < now()) ORDER BY run_id`)
	if err != nil {
		return nil, repositoryError(err)
	}
	defer rows.Close()
	return scanRuns(ctx, rows)
}
