package main

import (
	"context"
	"errors"
	"fmt"
	"sync"

	checkoutsvc "github.com/causalens/causalens/cmd/demo-checkout/service"
	gatewaysvc "github.com/causalens/causalens/cmd/demo-gateway/service"
	ledgersvc "github.com/causalens/causalens/cmd/demo-ledger/service"
	paymentsvc "github.com/causalens/causalens/cmd/demo-payment/service"
	"github.com/causalens/causalens/internal/capture"
	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/replay"
)

// errUnexpectedPlan is returned when the capsule's replay plan declares an
// entrypoint or required component this runtime cannot execute.
var errUnexpectedPlan = errors.New("replay worker: unsupported replay plan entrypoint or component")

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
// in-memory state. It consumes the capsule's plan and fixtures: it validates the
// gateway.checkout entrypoint and required components, loads fixtures in the
// declared fixture-load order (resetting the empty-ledger state fixture and
// configuring the payment dependency fixture), and applies the effective
// payment latency for the run. It never contacts a production or public
// destination and captures the run's events with replay_run_id through a
// replay sink. It imports Member 1's service packages read-only; no demo code
// is modified here.
type demoRunner struct {
	ids *capture.IDGenerator
}

func newDemoRunner() *demoRunner { return &demoRunner{ids: capture.NewIDGenerator(1)} }

// Run drives one replay and returns its observed events plus truthful in-process
// isolation evidence.
func (r *demoRunner) Run(ctx context.Context, cfg replay.RunnerConfig) (replay.RunResult, error) {
	if err := validatePlan(cfg.Plan, cfg.Fixtures); err != nil {
		return replay.RunResult{}, err
	}

	sink := &replaySink{runID: cfg.RunID}

	ledgerRec := capture.NewRecorder(contracts.ComponentRef{Name: "ledger", Instance: "ledger-1"}, r.ids, sink)
	paymentRec := capture.NewRecorder(contracts.ComponentRef{Name: "payment", Instance: "payment-1"}, r.ids, sink)
	checkoutRec := capture.NewRecorder(contracts.ComponentRef{Name: "checkout", Instance: "checkout-1"}, r.ids, sink)
	gatewayRec := capture.NewRecorder(contracts.ComponentRef{Name: "gateway", Instance: "gateway-1"}, r.ids, sink)

	ledger := ledgersvc.New(ledgerRec)
	payment := paymentsvc.New(paymentRec)
	checkout := checkoutsvc.New(payment, ledger, checkoutRec)
	checkoutIDs := capture.NewIDGenerator(capture.DefaultCheckoutSeed)
	gateway := gatewaysvc.New(checkout, checkoutIDs, gatewayRec)

	// Load fixtures in the capsule-declared order. A state fixture is reset to
	// empty (TRUNCATE_AND_LOAD); a dependency fixture configures the payment
	// simulator's latency. The run's effective latency is applied last.
	if err := loadFixtures(ctx, ledger, payment, cfg.Plan, cfg.Fixtures, cfg.LatencyMS); err != nil {
		return replay.RunResult{}, err
	}

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
		Isolation: replay.IsolationFor(cfg.Namespace, teardownOK),
		Teardown:  !teardownOK,
	}, nil
}

// validatePlan checks the capsule replay plan and fixture set against what the
// in-process runtime supports: the gateway.checkout entrypoint, the required
// components, one empty-ledger state fixture, and exactly one payment
// dependency fixture.
func validatePlan(plan contracts.ReplayPlan, fixtures contracts.FixtureSet) error {
	if plan.Entrypoint != "gateway.checkout" {
		return fmt.Errorf("%w: entrypoint %q", errUnexpectedPlan, plan.Entrypoint)
	}
	required := map[string]bool{"gateway": true, "checkout": true, "payment": true, "ledger": true}
	for _, component := range plan.RequiredComponents {
		if !required[component] {
			return fmt.Errorf("%w: component %q", errUnexpectedPlan, component)
		}
	}
	if len(fixtures.StateFixtures) == 0 || len(fixtures.DependencyFixtures) == 0 {
		return fmt.Errorf("%w: expected state and dependency fixtures", errUnexpectedPlan)
	}
	for _, fixture := range fixtures.DependencyFixtures {
		if fixture.Dependency != contracts.DependencyPaymentSimulator {
			return fmt.Errorf("%w: dependency %q", errUnexpectedPlan, fixture.Dependency)
		}
	}
	return nil
}

// loadFixtures resets and loads fixtures in the capsule-declared fixture-load
// order. It resets the empty-ledger state fixture (TRUNCATE_AND_LOAD), configures
// the payment dependency simulator's latency from each dependency fixture, then
// applies the run's effective latency last.
func loadFixtures(ctx context.Context, ledger *ledgersvc.Service, payment *paymentsvc.Service, plan contracts.ReplayPlan, fixtures contracts.FixtureSet, effectiveLatencyMS int) error {
	if strErr := ctx.Err(); strErr != nil {
		return strErr
	}
	if effectiveLatencyMS < 0 {
		return fmt.Errorf("%w: negative effective latency %d", errUnexpectedPlan, effectiveLatencyMS)
	}
	stateByID := map[string]contracts.StateFixture{}
	for _, fixture := range fixtures.StateFixtures {
		stateByID[fixture.FixtureID] = fixture
	}
	depByID := map[string]contracts.DependencyFixture{}
	for _, fixture := range fixtures.DependencyFixtures {
		depByID[fixture.FixtureID] = fixture
	}
	ledger.Reset()
	for _, id := range plan.FixtureLoadOrder {
		if state, ok := stateByID[id]; ok {
			if state.Kind != contracts.StateFixturePostgresRowset || state.ResetStrategy != contracts.FixtureTruncateAndLoad {
				return fmt.Errorf("%w: state fixture %q", errUnexpectedPlan, id)
			}
			ledger.Reset() // load empty state
			continue
		}
		if dep, ok := depByID[id]; ok {
			if dep.Dependency != contracts.DependencyPaymentSimulator {
				return fmt.Errorf("%w: dependency fixture %q", errUnexpectedPlan, id)
			}
			payment.SetLatencyMs(dep.LatencyMS)
			continue
		}
		return fmt.Errorf("%w: unknown fixture %q in load order", errUnexpectedPlan, id)
	}
	payment.SetLatencyMs(effectiveLatencyMS)
	return nil
}

// teardown resets replay-only state after a run and reports whether it
// succeeded. In-process there is nothing to close; resetting the ledger is the
// teardown action, and it mirrors the GOLDEN_RESET_V1 reset strategy.
func (r *demoRunner) teardown(ledger *ledgersvc.Service) bool {
	ledger.Reset()
	return true
}
