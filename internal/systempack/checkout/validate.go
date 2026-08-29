package checkout

import (
	"context"
	"strconv"

	"github.com/causalens/causalens/internal/contracts"
)

// ValidateCapsule implements contracts.SystemPack: scenario-specific
// capsule checks on top of the core contracts.ReplayCapsule.Validate()
// checks Bhargav's compiler already runs. It only adds checks; it never
// weakens safety, and it never authorizes a destination or credential
// profile the core policy has not already allowed.
func (p *Pack) ValidateCapsule(_ context.Context, capsule contracts.ReplayCapsule) []contracts.ValidationIssue {
	var issues []contracts.ValidationIssue

	hasLedgerState := false
	for _, f := range capsule.StateFixtures {
		if f.FixtureID == "state-ledger-empty" {
			hasLedgerState = true
		}
	}
	if !hasLedgerState {
		issues = append(issues, contracts.ValidationIssue{
			Code: contracts.FixtureMissing, Path: "/state_fixtures",
			Message: "checkout_duplicate_effect requires the state-ledger-empty fixture",
		})
	}

	if len(capsule.DependencyFixtures) == 0 {
		issues = append(issues, contracts.ValidationIssue{
			Code: contracts.FixtureMissing, Path: "/dependency_fixtures",
			Message: "checkout_duplicate_effect requires a payment_simulator dependency fixture",
		})
	}
	for i, f := range capsule.DependencyFixtures {
		if f.Dependency != contracts.DependencyPaymentSimulator {
			continue
		}
		// A compiled capsule always represents the golden baseline (350ms
		// latency, 2 attempts); a what-if intervention changes the replay
		// plan for one run without recompiling the capsule
		// (docs/CONTRACTS.md: "A compiled capsule is immutable").
		if f.LatencyMS != paymentsvcDefaultLatencyMs {
			issues = append(issues, contracts.ValidationIssue{
				Code: contracts.SchemaInvalid, Path: pathIndex("/dependency_fixtures", i, "/latency_ms"),
				Message: "baseline payment dependency fixture must encode the golden 350ms latency",
			})
		}
		if f.InvocationLimit != 2 {
			issues = append(issues, contracts.ValidationIssue{
				Code: contracts.SchemaInvalid, Path: pathIndex("/dependency_fixtures", i, "/invocation_limit"),
				Message: "baseline payment dependency fixture must allow exactly 2 invocations (max attempts)",
			})
		}
	}

	return issues
}

// paymentsvcDefaultLatencyMs mirrors cmd/demo-payment/service.DefaultLatencyMs
// (350) without importing the demo binary's service package from the
// System Pack, keeping the pack's dependency graph acyclic and independent
// of any one demo service's internal wiring.
const paymentsvcDefaultLatencyMs = 350

func pathIndex(base string, index int, suffix string) string {
	return base + "/" + strconv.Itoa(index) + suffix
}
