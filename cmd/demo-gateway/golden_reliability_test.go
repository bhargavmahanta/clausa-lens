package main

import (
	"context"
	"testing"
	"time"

	gatewaysvc "github.com/causalens/causalens/cmd/demo-gateway/service"
	checkoutpack "github.com/causalens/causalens/internal/systempack/checkout"
)

// TestGoldenScenario_TenRunsFromReset runs the captured scenario repeatedly
// from a clean reset and confirms every run reproduces the same structural
// shape -- timeout, retry, both attempts, two committed ledger effects, and
// a true duplicate-effect oracle match -- allowing only timing to vary. It
// reports a 10/10 structural success count and the observed timing range,
// and every event is proven to pass Bhargav's real Core API validation
// (via the harness's HTTP sink) on every run.
func TestGoldenScenario_TenRunsFromReset(t *testing.T) {
	const runs = 10
	h := newGoldenHarness(t)
	pack := checkoutpack.New()

	var min, max time.Duration
	successes := 0

	for i := 0; i < runs; i++ {
		h.reset(context.Background())

		started := time.Now()
		result, err := h.gateway.Checkout(context.Background(), gatewaysvc.Request{CheckoutID: "8271"})
		elapsed := time.Since(started)
		if err != nil {
			t.Fatalf("run %d: Checkout: %v", i+1, err)
		}

		if result.Attempts != 2 {
			t.Fatalf("run %d: Attempts = %d, want 2", i+1, result.Attempts)
		}
		if got := h.payment.AttemptCount(); got != 2 {
			t.Fatalf("run %d: payment attempt count = %d, want 2", i+1, got)
		}
		if got := h.ledger.CommittedEffectCount(); got != 2 {
			t.Fatalf("run %d: ledger commit count = %d, want 2", i+1, got)
		}

		oracle, err := pack.DetectIncident(context.Background(), h.local.Events())
		if err != nil {
			t.Fatalf("run %d: DetectIncident: %v", i+1, err)
		}
		if !oracle.Matched {
			t.Fatalf("run %d: oracle did not match: %s", i+1, oracle.Explanation)
		}
		if oracle.EffectSummary.PaymentAttemptCount != 2 || oracle.EffectSummary.LedgerCommitCount != 2 {
			t.Fatalf("run %d: oracle EffectSummary = %+v, want {2 2}", i+1, oracle.EffectSummary)
		}

		if i == 0 || elapsed < min {
			min = elapsed
		}
		if i == 0 || elapsed > max {
			max = elapsed
		}
		successes++
		t.Logf("run %d/%d: PASS (attempts=%d commits=%d oracle=%v elapsed=%s)",
			i+1, runs, result.Attempts, h.ledger.CommittedEffectCount(), oracle.Matched, elapsed)
	}

	t.Logf("golden run success count: %d/%d, timing range: %s - %s", successes, runs, min, max)
	if successes != runs {
		t.Fatalf("structural success count = %d/%d, want %d/%d", successes, runs, runs, runs)
	}
}
