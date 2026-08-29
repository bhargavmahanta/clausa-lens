package main

import (
	"context"
	"sync"

	checkoutsvc "github.com/causalens/causalens/cmd/demo-checkout/service"
	gatewaysvc "github.com/causalens/causalens/cmd/demo-gateway/service"
	ledgersvc "github.com/causalens/causalens/cmd/demo-ledger/service"
	paymentsvc "github.com/causalens/causalens/cmd/demo-payment/service"
	"github.com/causalens/causalens/internal/capture"
	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/replay"
)

// replaySink is a replay-only capture sink that stamps replay_run_id on every
// event emitted by the demo services and collects them for one run. It is
// concurrency-safe because the demo checkout launches payment attempts in
// goroutines. It records teardown failure if any emit fails.
type replaySink struct {
	mu       sync.Mutex
	runID    string
	events   []contracts.ExecutionEvent
	emitErr  error
	teardown bool
}

func (s *replaySink) Emit(_ context.Context, event contracts.ExecutionEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	event.ReplayRunID = s.runID
	if err := event.Validate(); err != nil {
		s.emitErr = err
		return err
	}
	s.events = append(s.events, event)
	return nil
}

func (s *replaySink) snapshot() []contracts.ExecutionEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]contracts.ExecutionEvent(nil), s.events...)
}

// demoRunner executes a live replay by driving the real demo service chain
// (Gateway -> Checkout -> Payment -> Ledger) in-process against replay-only
// in-memory state. It does not contact any production or uncontrolled service,
// and it captures the run's events with replay_run_id through a replay sink. It
// imports Trinabha's service packages read-only; no demo code is modified here.
//
// The single recorder IDGenerator is shared across every service and every run
// so event_ids stay globally unique for the worker process (CONTRACTS.md). Each
// run gets a fresh checkout identifier generator seeded at DefaultCheckoutSeed
// so identifiers are deterministic per run, matching the reset contract.
type demoRunner struct {
	ids *capture.IDGenerator
}

func newDemoRunner() *demoRunner { return &demoRunner{ids: capture.NewIDGenerator(1)} }

// Run drives one replay and returns its observed events plus real isolation
// evidence derived from what it actually touched.
func (r *demoRunner) Run(ctx context.Context, cfg replay.RunnerConfig) (replay.RunResult, error) {
	sink := &replaySink{runID: cfg.RunID}

	ledgerRec := capture.NewRecorder(contracts.ComponentRef{Name: "ledger", Instance: "ledger-1"}, r.ids, sink)
	paymentRec := capture.NewRecorder(contracts.ComponentRef{Name: "payment", Instance: "payment-1"}, r.ids, sink)
	checkoutRec := capture.NewRecorder(contracts.ComponentRef{Name: "checkout", Instance: "checkout-1"}, r.ids, sink)
	gatewayRec := capture.NewRecorder(contracts.ComponentRef{Name: "gateway", Instance: "gateway-1"}, r.ids, sink)

	ledger := ledgersvc.New(ledgerRec)
	payment := paymentsvc.New(paymentRec)
	payment.SetLatencyMs(cfg.LatencyMS)
	checkout := checkoutsvc.New(payment, ledger, checkoutRec)
	checkout.Reset()
	checkoutIDs := capture.NewIDGenerator(capture.DefaultCheckoutSeed)
	gateway := gatewaysvc.New(checkout, checkoutIDs, gatewayRec)

	// Reset and load replay-only fixtures in the declared order: reset the ledger
	// to empty state (TRUNCATE_AND_LOAD) and set the configured latency. The
	// dependency fixture bearing the effective latency drives the simulator.
	ledger.Reset()
	payment.SetLatencyMs(cfg.LatencyMS)

	_, err := gateway.Checkout(ctx, gatewaysvc.Request{})
	if err != nil {
		_ = r.teardown(ledger)
		return replay.RunResult{Teardown: true}, err
	}

	if sink.emitErr != nil {
		_ = r.teardown(ledger)
		return replay.RunResult{Teardown: true}, sink.emitErr
	}

	events := sink.snapshot()
	teardownOK := r.teardown(ledger)
	return replay.RunResult{
		Events:    events,
		Isolation: isolationFor(events, cfg.Namespace, teardownOK),
		Teardown:  !teardownOK,
	}, nil
}

// teardown resets replay-only state after a run and reports whether it
// succeeded. In-process there is nothing to close; resetting the ledger is the
// teardown action, and it mirrors the GOLDEN_RESET_V1 reset strategy.
func (r *demoRunner) teardown(ledger *ledgersvc.Service) bool {
	ledger.Reset()
	return true
}

// isolationFor derives real isolation evidence from the events a run actually
// produced: it touched the payment simulator when a payment event exists and the
// replay-only ledger datastore when a ledger event exists.
func isolationFor(events []contracts.ExecutionEvent, namespace string, teardownOK bool) contracts.IsolationEvidence {
	paymentTouched := false
	ledgerTouched := false
	for _, event := range events {
		switch event.Component.Name {
		case "payment":
			if event.Operation.Kind == contracts.OperationDependency {
				paymentTouched = true
			}
		case "ledger":
			ledgerTouched = true
		}
	}
	return replay.IsolationFor("", namespace, paymentTouched, ledgerTouched, teardownOK)
}
