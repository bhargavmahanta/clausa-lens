package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/replay"
)

func runnerPlan() contracts.ReplayPlan {
	return contracts.ReplayPlan{
		Entrypoint:         "gateway.checkout",
		RequiredComponents: []string{"gateway", "checkout", "payment", "ledger"},
		FixtureLoadOrder:   []string{"state-ledger-empty", "dependency-payment-350ms"},
		ResetStrategy:      contracts.ReplayResetGoldenV1,
	}
}

func runnerFixtures() contracts.FixtureSet {
	return contracts.FixtureSet{
		StateFixtures: []contracts.StateFixture{{
			FixtureID: "state-ledger-empty", Kind: contracts.StateFixturePostgresRowset,
			ContentRef: "fixture://golden/ledger-empty-v1", ContentDigest: strings.Repeat("b", 64),
			SanitizationStatus: contracts.SanitizationPass, ResetStrategy: contracts.FixtureTruncateAndLoad,
		}},
		DependencyFixtures: []contracts.DependencyFixture{{
			FixtureID: "dependency-payment-350ms", Dependency: contracts.DependencyPaymentSimulator,
			RequestMatch: map[string]any{"logical_operation_id": "checkout-8271"},
			Response:     map[string]any{"status": "APPROVED"}, LatencyMS: 350,
			FailureMode: contracts.FailureModeNone, InvocationLimit: 2,
		}},
	}
}

func runnerConfig() replay.RunnerConfig {
	return replay.RunnerConfig{RunID: "run-base", Namespace: "replay-run-run-base", Plan: runnerPlan(), Fixtures: runnerFixtures(), LatencyMS: 350}
}

func TestRunnerNamespacesEventIDsByRunID(t *testing.T) {
	result, err := newDemoRunner().Run(context.Background(), runnerConfig())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	foundEvent := false
	foundParent := false
	for _, event := range result.Events {
		if event.EventID == "evt-run-base-gateway-1" {
			foundEvent = true
		}
		if event.ParentEventID == "evt-run-base-gateway-1" {
			foundParent = true
		}
	}
	if !foundEvent {
		t.Fatal("missing namespaced event ID evt-run-base-gateway-1")
	}
	if !foundParent {
		t.Fatal("missing namespaced parent event ID evt-run-base-gateway-1")
	}
}

func TestFreshRunnersProduceDisjointEventIDsForDifferentRuns(t *testing.T) {
	firstCfg := runnerConfig()
	firstCfg.RunID = "run-first"
	firstCfg.Namespace = "replay-run-run-first"
	first, err := newDemoRunner().Run(context.Background(), firstCfg)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}

	secondCfg := runnerConfig()
	secondCfg.RunID = "run-second"
	secondCfg.Namespace = "replay-run-run-second"
	second, err := newDemoRunner().Run(context.Background(), secondCfg)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}

	firstIDs := make(map[string]bool, len(first.Events))
	for _, event := range first.Events {
		firstIDs[event.EventID] = true
	}
	for _, event := range second.Events {
		if firstIDs[event.EventID] {
			t.Errorf("fresh runners produced overlapping event ID %q", event.EventID)
		}
	}
}

// TestRunnerRejectsWrongEntrypoint verifies the runner consumes (and refuses to
// ignore) the capsule replay plan's entrypoint.
func TestRunnerRejectsWrongEntrypoint(t *testing.T) {
	cfg := runnerConfig()
	cfg.Plan.Entrypoint = "not-gateway.checkout"
	_, err := newDemoRunner().Run(context.Background(), cfg)
	if !errors.Is(err, errUnexpectedPlan) {
		t.Fatalf("expected errUnexpectedPlan, got %v", err)
	}
}

// TestRunnerRejectsUnknownFixtureInLoadOrder verifies fixtures are loaded in the
// declared order and an unknown fixture id is rejected.
func TestRunnerRejectsUnknownFixtureInLoadOrder(t *testing.T) {
	cfg := runnerConfig()
	cfg.Plan.FixtureLoadOrder = []string{"state-ledger-empty", "does-not-exist"}
	_, err := newDemoRunner().Run(context.Background(), cfg)
	if !errors.Is(err, errUnexpectedPlan) {
		t.Fatalf("expected errUnexpectedPlan for unknown fixture, got %v", err)
	}
}

// TestRunnerUsesEffectiveLatency verifies the runner applies the run's effective
// latency (what-if target) to the payment simulator, overriding the fixture
// default, by inspecting the captured payment START event.
func TestRunnerUsesEffectiveLatency(t *testing.T) {
	cfg := runnerConfig()
	cfg.LatencyMS = 50 // what-if PAYMENT_LATENCY 350 -> 50
	result, err := newDemoRunner().Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result.Events) == 0 {
		t.Fatal("no events captured")
	}
	latencySeen := -1
	for _, e := range result.Events {
		if e.Component.Name == "payment" && e.EventType == contracts.EventStart {
			if v, ok := e.Attributes["configured_latency_ms"]; ok {
				if ms, ok := v.(int); ok {
					latencySeen = ms
				}
			}
		}
	}
	if latencySeen != 50 {
		t.Fatalf("effective latency not applied; payment configured_latency_ms = %d, want 50", latencySeen)
	}
	for _, e := range result.Events {
		if e.ReplayRunID != "run-base" {
			t.Fatalf("event %s missing replay_run_id", e.EventID)
		}
	}
}

// TestRunnerIsolationHasNoFabricatedDestinations verifies isolation evidence
// truthfully reports in-process execution: no fabricated payment-simulator host
// and no replay datastore URL, and no external datastore/simulator interactions.
func TestRunnerIsolationHasNoFabricatedDestinations(t *testing.T) {
	cfg := runnerConfig()
	cfg.LatencyMS = 350
	result, err := newDemoRunner().Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	iso := result.Isolation
	if len(iso.DatastoreDestinations) != 0 || len(iso.SimulatorInteractions) != 0 {
		t.Fatalf("in-process replay must report no external destinations/interactions: %+v", iso)
	}
	if iso.NetworkPolicy != contracts.VerdictPass || iso.CredentialProfile != contracts.CredentialReplayOnly || iso.RuntimeNamespace != "replay-run-run-base" {
		t.Fatalf("isolation fields wrong: %+v", iso)
	}
	// The evidence must not mention a fabricated network destination.
	raw, _ := json.Marshal(iso)
	for _, forbidden := range []string{"http://payment-simulator", "postgres://replay/"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("isolation fabricated destination %q: %s", forbidden, raw)
		}
	}
	if err := replay.ValidateIsolation(iso); err != nil {
		t.Fatalf("isolation should validate: %v", err)
	}
}
