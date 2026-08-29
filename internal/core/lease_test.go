package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/causalens/causalens/internal/contracts"
)

func leaseRun(runID string) contracts.ReplayRun {
	return contracts.ReplayRun{SchemaVersion: contracts.ContractVersion, RunID: runID, ExecutionID: "e", CapsuleID: "cap-1", CapsuleHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", RunType: contracts.RunTypeBaseline, TrialNumber: 1, Status: contracts.ReplayRunCreated}
}

// TestClaimRunIsSingleOwner verifies only one worker can claim a CREATED run.
func TestClaimRunIsSingleOwner(t *testing.T) {
	store := NewStore()
	ctx := context.Background()
	if err := store.PutRun(ctx, leaseRun("run-1")); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimRun(ctx, "run-1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Status != contracts.ReplayRunValidating {
		t.Fatalf("claim should advance to VALIDATING: %s", claimed.Status)
	}
	if _, err := store.ClaimRun(ctx, "run-1", time.Minute); !errors.Is(err, ErrInvalidLifecycle) {
		t.Fatalf("second claim must fail: %v", err)
	}
}

// TestReclaimExpiredRunsReportsOnlyExpired verifies expired/missing leases are
// reported and an active lease is left alone.
func TestReclaimExpiredRunsReportsOnlyExpired(t *testing.T) {
	store := NewStore()
	ctx := context.Background()
	_ = store.PutRun(ctx, leaseRun("expired"))
	_ = store.PutRun(ctx, leaseRun("active"))
	_ = store.PutRun(ctx, leaseRun("fresh-created"))

	expired, err := store.ClaimRun(ctx, "expired", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	store.leaseUntil["expired"] = time.Now().Add(-time.Hour) // force expiry

	active, err := store.ClaimRun(ctx, "active", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	store.leaseUntil["active"] = time.Now().Add(time.Hour) // keep active

	_ = expired
	_ = active
	// CREATE_DONE stays CREATED, not reclaimable.
	reclaimable, err := store.ReclaimExpiredRuns(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, run := range reclaimable {
		ids = append(ids, run.RunID)
	}
	if len(ids) != 1 || ids[0] != "expired" {
		t.Fatalf("expected only the expired run, got %v", ids)
	}
}
