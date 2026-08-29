package checkout

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/causalens/causalens/internal/contracts"
)

// ledgerEmptyStateContent is the canonical, sanitized content of the
// replay-only empty ledger fixture (docs/SYSTEM_PACKS.md: "Empty,
// resettable replay ledger state"). Its digest is computed deterministically
// so the same fixture always produces the same content_digest.
var ledgerEmptyStateContent = []byte(`{"table":"ledger_effects","rows":[]}`)

// ExtractFixtures implements contracts.SystemPack: it selects the sanitized
// state and dependency fixtures required to replay the golden checkout
// scenario. The payment latency and invocation limit are derived from the
// incident's own captured evidence rather than hardcoded, so a differently
// configured incident still yields a faithful fixture.
func (p *Pack) ExtractFixtures(_ context.Context, _ contracts.Incident, events []contracts.ExecutionEvent) (contracts.FixtureSet, error) {
	latencyMs := observedConfiguredLatencyMs(events)
	invocationLimit := observedPaymentAttemptCount(events)
	if invocationLimit < 1 {
		invocationLimit = 1
	}

	digest := sha256.Sum256(ledgerEmptyStateContent)

	stateFixture := contracts.StateFixture{
		FixtureID:          "state-ledger-empty",
		Kind:               contracts.StateFixturePostgresRowset,
		ContentRef:         "fixture://golden/ledger-empty-v1",
		ContentDigest:      hex.EncodeToString(digest[:]),
		SanitizationStatus: contracts.SanitizationPass,
		ResetStrategy:      contracts.FixtureTruncateAndLoad,
	}

	dependencyFixture := contracts.DependencyFixture{
		FixtureID:       fmt.Sprintf("dependency-payment-%dms", latencyMs),
		Dependency:      contracts.DependencyPaymentSimulator,
		RequestMatch:    map[string]any{"logical_operation_id": selectLogicalOperationID(events)},
		Response:        map[string]any{"status": "APPROVED"},
		LatencyMS:       latencyMs,
		FailureMode:     contracts.FailureModeNone,
		InvocationLimit: invocationLimit,
	}

	return contracts.FixtureSet{
		StateFixtures:      []contracts.StateFixture{stateFixture},
		DependencyFixtures: []contracts.DependencyFixture{dependencyFixture},
	}, nil
}

// BuildReplayPlan implements contracts.SystemPack: it declares the golden
// scenario's fixed entrypoint, required service sequence, and reset
// strategy, and orders fixture loading state-before-dependency (matching
// the frozen P0 example in docs/CONTRACTS.md).
func (p *Pack) BuildReplayPlan(_ context.Context, _ contracts.Incident, fixtures contracts.FixtureSet) (contracts.ReplayPlan, error) {
	order := make([]string, 0, len(fixtures.StateFixtures)+len(fixtures.DependencyFixtures))
	for _, f := range fixtures.StateFixtures {
		order = append(order, f.FixtureID)
	}
	for _, f := range fixtures.DependencyFixtures {
		order = append(order, f.FixtureID)
	}

	return contracts.ReplayPlan{
		Entrypoint:         "gateway.checkout",
		RequiredComponents: []string{"gateway", "checkout", "payment", "ledger"},
		FixtureLoadOrder:   order,
		ResetStrategy:      contracts.ReplayResetGoldenV1,
	}, nil
}

// observedConfiguredLatencyMs reads the configured_latency_ms attribute
// from the first payment authorize START event in evidence, falling back
// to the P0 default when evidence carries none.
func observedConfiguredLatencyMs(events []contracts.ExecutionEvent) int {
	for _, e := range events {
		if e.EventType != contracts.EventStart || e.Operation.Kind != contracts.OperationDependency {
			continue
		}
		if v, ok := e.Attributes["configured_latency_ms"]; ok {
			if ms, ok := asInt(v); ok {
				return ms
			}
		}
	}
	return 350
}

// observedPaymentAttemptCount reports how many distinct payment attempts
// evidence shows, falling back to the P0 default max attempts when
// evidence carries none.
func observedPaymentAttemptCount(events []contracts.ExecutionEvent) int {
	result, err := evaluate(events)
	if err == nil && result.EffectSummary.PaymentAttemptCount > 0 {
		return result.EffectSummary.PaymentAttemptCount
	}
	return 2
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		if n == float64(int(n)) {
			return int(n), true
		}
	case json.Number:
		i, err := n.Int64()
		if err == nil {
			return int(i), true
		}
	}
	return 0, false
}
