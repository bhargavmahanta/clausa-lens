// Command replay-worker drives a single ReplayRun through its lifecycle to a
// terminal state, evaluating the outcome with the deployment's selected System
// Pack and persisting the result. It is the replay-runtime seam: the bounds of
// "one isolated execution at a time" plus the pack's outcome evaluation live
// here, while internal/replay owns the lifecycle and safety rules.
//
// It processes exactly one run specified by -run-id. A deployment (or the demo
// harness) invokes it per run so a baseline run is driven to COMPLETED before a
// what-if run is authorized. It selects the pack from PACK_IMPL via the same
// pack-agnostic registry the Core API uses, so no scenario logic lives here.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/core"
	"github.com/causalens/causalens/internal/packregistry"
	"github.com/causalens/causalens/internal/replay"
)

// processRun advances one run from its current status to a terminal state and
// persists it. It refuses to run without a pack and skips already-terminal runs.
// It is factored out of main so the lifecycle driver can be unit-tested against
// a fake store without a database.
func processRun(ctx context.Context, store replay.RunStore, events replay.EventLoader,
	pack contracts.SystemPack, runID string) (contracts.ReplayRun, error) {

	run, err := store.GetRun(ctx, runID)
	if err != nil {
		return contracts.ReplayRun{}, fmt.Errorf("replay worker: get run: %w", err)
	}
	if pack == nil {
		return run, fmt.Errorf("replay worker: no System Pack wired (set PACK_IMPL)")
	}
	if isTerminal(run.Status) {
		return run, nil
	}

	if run.Status == contracts.ReplayRunCreated {
		run, err = replay.AdvanceRun(ctx, store, run, contracts.ReplayRunValidating)
		if err != nil {
			return run, err
		}
	}
	if run.Status == contracts.ReplayRunValidating {
		run, err = replay.AdvanceRun(ctx, store, run, contracts.ReplayRunRunning)
		if err != nil {
			return run, err
		}
	}
	if run.Status != contracts.ReplayRunRunning {
		return run, fmt.Errorf("replay worker: run %q is in unexpected status %s", run.RunID, run.Status)
	}

	observed, err := events.EventsForRun(ctx, run)
	if err != nil {
		return run, fmt.Errorf("replay worker: load events: %w", err)
	}
	evaluated, err := replay.Evaluate(ctx, pack, run, observed)
	if err != nil {
		return run, err
	}
	if evaluated.Status == contracts.ReplayRunRunning {
		return run, fmt.Errorf("replay worker: Evaluate did not reach a terminal status")
	}
	if err := store.TransitionRun(ctx, contracts.ReplayRunRunning, evaluated); err != nil {
		return evaluated, fmt.Errorf("replay worker: persist %s: %w", evaluated.Status, err)
	}
	return evaluated, nil
}

func isTerminal(status contracts.ReplayRunStatus) bool {
	return status == contracts.ReplayRunCompleted || status == contracts.ReplayRunFailed || status == contracts.ReplayRunBlocked
}

func main() {
	runID := flag.String("run-id", "", "id of the run to drive to a terminal state")
	flag.Parse()
	if *runID == "" {
		log.Fatal("-run-id is required")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	db, err := core.OpenPostgres(databaseURL)
	if err != nil {
		log.Fatal("failed to initialize database")
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatal("database is unavailable")
	}
	store := core.NewPostgresRepository(db)
	pack := packregistry.Resolve(os.Getenv("PACK_IMPL"))
	result, err := processRun(ctx, store, store, pack, *runID)
	if err != nil {
		log.Fatalf("replay worker: %v", err)
	}
	log.Printf("replay worker: run %s -> %s (outcome %s)", result.RunID, result.Status, result.Outcome)
}
