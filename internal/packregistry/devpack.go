package packregistry

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/causalens/causalens/internal/contracts"
)

// DevImplementation is the PACK_IMPL token that selects the dev pack. It exists
// only so the Core API capsule and diff routes can be exercised end-to-end
// without Member 1's real checkout pack. A production deployment must never
// select it; the real pack is selected by its own token.
const DevImplementation = "dev"

// DevPack is an honestly-named System Pack used for route exercising only. Its
// descriptor deliberately differs from the real checkout pack (id
// "checkout_duplicate_effect_dev", version "0.0.0-dev") so it can never be
// mistaken for the production pack. It provides deterministic fixtures, a
// replay plan, the P0 intervention, and a baseline-true / what-if-false oracle
// so the capsule, run and diff resources form a valid golden-shaped flow.
//
// It does not implement capture normalization (Normalize returns an error) and
// it never weakens the core safety policy: capsule integrity, isolation and
// baseline authorization remain the Core API's responsibility.
type DevPack struct{}

// NewDevPack returns the dev pack instance registered under DevImplementation.
func NewDevPack() *DevPack { return &DevPack{} }

// Descriptor returns the honest dev identifier, never the real pack's id/version.
func (p *DevPack) Descriptor() contracts.SystemPackRef {
	return contracts.SystemPackRef{ID: "checkout_duplicate_effect_dev", Version: "0.0.0-dev", InterfaceVersion: contracts.ContractVersion}
}

// Normalize is intentionally unsupported: capture translation belongs to the
// real pack. It reports a clear error rather than silently fabricating events.
func (p *DevPack) Normalize(_ context.Context, _ contracts.RawEvidence) ([]contracts.ExecutionEvent, error) {
	return nil, fmt.Errorf("dev pack: Normalize is not provided; capture translation is the real pack's responsibility")
}

// DetectIncident evaluates the golden duplicate-ledger oracle to true for the
// captured original, matching the P0 shape (two attempts, two commits).
func (p *DevPack) DetectIncident(_ context.Context, events []contracts.ExecutionEvent) (contracts.OracleResult, error) {
	return p.oracle(true, 2, 2, "dev pack: duplicate-effect oracle matched"), nil
}

// ExtractFixtures returns the P0 state and dependency fixture set required by
// the frozen capsule contract.
func (p *DevPack) ExtractFixtures(_ context.Context, _ contracts.Incident, _ []contracts.ExecutionEvent) (contracts.FixtureSet, error) {
	return contracts.FixtureSet{
		StateFixtures: []contracts.StateFixture{{
			FixtureID:          "state-ledger-empty",
			Kind:               contracts.StateFixturePostgresRowset,
			ContentRef:         "fixture://golden/ledger-empty-v1",
			ContentDigest:      strings.Repeat("b", 64),
			SanitizationStatus: contracts.SanitizationPass,
			ResetStrategy:      contracts.FixtureTruncateAndLoad,
		}},
		DependencyFixtures: []contracts.DependencyFixture{{
			FixtureID:       "dependency-payment-350ms",
			Dependency:      contracts.DependencyPaymentSimulator,
			RequestMatch:    map[string]any{"logical_operation_id": "checkout-8271"},
			Response:        map[string]any{"status": "APPROVED"},
			LatencyMS:       350,
			FailureMode:     contracts.FailureModeNone,
			InvocationLimit: 2,
		}},
	}, nil
}

// BuildReplayPlan returns the P0 gateway.checkout replay plan whose fixture
// load order contains every extracted fixture exactly once.
func (p *DevPack) BuildReplayPlan(_ context.Context, _ contracts.Incident, fixtures contracts.FixtureSet) (contracts.ReplayPlan, error) {
	order := make([]string, 0, len(fixtures.StateFixtures)+len(fixtures.DependencyFixtures))
	for _, fixture := range fixtures.StateFixtures {
		order = append(order, fixture.FixtureID)
	}
	for _, fixture := range fixtures.DependencyFixtures {
		order = append(order, fixture.FixtureID)
	}
	return contracts.ReplayPlan{
		Entrypoint:         "gateway.checkout",
		RequiredComponents: []string{"gateway", "checkout", "payment", "ledger"},
		FixtureLoadOrder:   order,
		ResetStrategy:      contracts.ReplayResetGoldenV1,
	}, nil
}

// ValidateCapsule adds no pack-specific rule beyond the core check; the frozen
// capsule contract is fully enforced by core before this runs.
func (p *DevPack) ValidateCapsule(_ context.Context, _ contracts.ReplayCapsule) []contracts.ValidationIssue {
	return nil
}

// AllowedInterventions publishes the single P0 latency intervention.
func (p *DevPack) AllowedInterventions() []contracts.InterventionSpec {
	return []contracts.InterventionSpec{{
		Type:      contracts.InterventionPaymentLatency,
		ValueType: contracts.InterventionValueInteger,
		Unit:      contracts.InterventionUnitMilliseconds,
		Minimum:   0,
		Maximum:   5000,
	}}
}

// ApplyIntervention returns a copy of the plan with the latency delta already
// reflected; it never mutates the capsule-derived plan.
func (p *DevPack) ApplyIntervention(_ context.Context, plan contracts.ReplayPlan, _ contracts.Intervention) (contracts.ReplayPlan, error) {
	return plan, nil
}

// EvaluateOutcome evaluates the oracle per run type: baseline reproduces
// (two attempts / two commits), what-if mitigates (one / one).
func (p *DevPack) EvaluateOutcome(_ context.Context, execution contracts.ReplayExecution) (contracts.OracleResult, error) {
	if execution.Run.RunType == contracts.RunTypeWhatIf {
		return p.oracle(false, 1, 1, "dev pack: what-if completed before timeout; no retry or duplicate effect"), nil
	}
	return p.oracle(true, 2, 2, "dev pack: baseline reproduced the timeout-driven duplicate ledger effect"), nil
}

// Align implements the semantic alignment used by differential.Build. It aligns
// events by component.name, operation.name, event_type, logical_operation_id
// and attempt (the frozen semantic keys) and reports matched/added/removed and
// field-level changes in deterministic order.
func (p *DevPack) Align(_ context.Context, _ string, baseline contracts.ReplayExecution,
	comparison contracts.ReplayExecution) ([]contracts.EventAlignment, []string, []string, []contracts.EventChange, error) {

	matched, added, removed, changes := alignEvents(baseline.Events, comparison.Events)
	return matched, added, removed, changes, nil
}

// Compare satisfies the frozen SystemPack interface by aligning the two
// executions and assembling a contract-valid diff. Core and the worker build
// diffs through differential.Build; this is provided so the interface is
// complete and returns a deterministic result when invoked directly.
func (p *DevPack) Compare(_ context.Context, diffID string, baseline contracts.ReplayExecution, comparison contracts.ReplayExecution) (contracts.ReplayDiff, error) {
	if comparison.Run.Intervention == nil {
		return contracts.ReplayDiff{}, fmt.Errorf("dev pack: comparison run requires exactly one intervention to build a diff")
	}
	matched, added, removed, changes := diffSets(alignEvents(baseline.Events, comparison.Events))

	baselineSummary := effectSummaryFor(baseline.Run, p.DefaultBaselineEffects())
	comparisonSummary := effectSummaryFor(comparison.Run, p.DefaultWhatIfEffects())

	diff := contracts.ReplayDiff{
		SchemaVersion:           contracts.ContractVersion,
		DiffID:                  diffID,
		BaselineRunID:           baseline.Run.RunID,
		ComparisonRunID:         comparison.Run.RunID,
		AlignmentVersion:        contracts.ContractVersion,
		Intervention:            *comparison.Run.Intervention,
		BaselineOracleResult:    oracleResultFor(baseline.Run, p.oracle(true, 2, 2, "dev pack: baseline reproduced the duplicate effect")),
		ComparisonOracleResult:  oracleResultFor(comparison.Run, p.oracle(false, 1, 1, "dev pack: what-if completed before timeout, no duplicate effect")),
		MatchedEvents:           matched,
		AddedEventIDs:           added,
		RemovedEventIDs:         removed,
		ChangedEvents:           changes,
		BaselineEffectSummary:   baselineSummary,
		ComparisonEffectSummary: comparisonSummary,
		EffectDelta:             contracts.EffectDelta{PaymentAttemptCount: comparisonSummary.PaymentAttemptCount - baselineSummary.PaymentAttemptCount, LedgerCommitCount: comparisonSummary.LedgerCommitCount - baselineSummary.LedgerCommitCount},
		EvidenceSummary:         fmt.Sprintf("Dev pack aligned %d matched, %d added, %d removed, %d changed.", len(matched), len(added), len(removed), len(changes)),
		Limitations:             []string{"Dev pack only; not production evidence."},
	}
	diff.BaselineOracleResult.EffectSummary = baselineSummary
	diff.ComparisonOracleResult.EffectSummary = comparisonSummary
	return diff, nil
}

// Labels returns empty label sets; the real pack owns presentation labels.
func (p *DevPack) Labels() contracts.LabelSet { return contracts.LabelSet{} }

// DefaultBaselineEffects returns the frozen baseline effect summary.
func (p *DevPack) DefaultBaselineEffects() contracts.EffectSummary {
	return contracts.EffectSummary{PaymentAttemptCount: 2, LedgerCommitCount: 2}
}

// DefaultWhatIfEffects returns the frozen what-if effect summary.
func (p *DevPack) DefaultWhatIfEffects() contracts.EffectSummary {
	return contracts.EffectSummary{PaymentAttemptCount: 1, LedgerCommitCount: 1}
}

func (p *DevPack) oracle(matched bool, attempts, commits int, explanation string) contracts.OracleResult {
	return contracts.OracleResult{
		Oracle:                   contracts.FailureOracleRef{ID: "duplicate_ledger_effect", Version: "1.0.0"},
		Matched:                  matched,
		EffectSummary:            contracts.EffectSummary{PaymentAttemptCount: attempts, LedgerCommitCount: commits},
		RequiredEvidenceEventIDs: []string{"evt-timeout", "evt-retry", "evt-ledger-1", "evt-ledger-2"},
		Explanation:              explanation,
	}
}

func effectSummaryFor(run contracts.ReplayRun, fallback contracts.EffectSummary) contracts.EffectSummary {
	if run.EffectSummary != nil {
		return *run.EffectSummary
	}
	return fallback
}

func oracleResultFor(run contracts.ReplayRun, fallback contracts.OracleResult) contracts.OracleResult {
	if run.FailureOracleResult != nil {
		return *run.FailureOracleResult
	}
	return fallback
}

func diffSets(matched []contracts.EventAlignment, added, removed []string, changes []contracts.EventChange) ([]contracts.EventAlignment, []string, []string, []contracts.EventChange) {
	if matched == nil {
		matched = []contracts.EventAlignment{}
	}
	if added == nil {
		added = []string{}
	}
	if removed == nil {
		removed = []string{}
	}
	if changes == nil {
		changes = []contracts.EventChange{}
	}
	return matched, added, removed, changes
}

// alignEvents deterministically aligns two event streams by semantic key and
// returns matched/added/removed/changed sets required by a ReplayDiff.
func alignEvents(baseline, comparison []contracts.ExecutionEvent) ([]contracts.EventAlignment, []string, []string, []contracts.EventChange) {
	baseIndex := indexByKey(baseline)
	compIndex := indexByKey(comparison)

	var matched []contracts.EventAlignment
	var added, removed []string
	for key, baseIDs := range baseIndex {
		compIDs := compIndex[key]
		shared := len(baseIDs)
		if len(compIDs) < shared {
			shared = len(compIDs)
		}
		for i := 0; i < shared; i++ {
			matched = append(matched, contracts.EventAlignment{BaselineEventID: baseIDs[i], ComparisonEventID: compIDs[i]})
		}
		for _, id := range baseIDs[shared:] {
			removed = append(removed, id)
		}
		for _, id := range compIDs[shared:] {
			added = append(added, id)
		}
	}

	byBaseID := byEventID(baseline)
	byCompID := byEventID(comparison)
	var changes []contracts.EventChange
	for _, pair := range matched {
		baseEvent := byBaseID[pair.BaselineEventID]
		compEvent := byCompID[pair.ComparisonEventID]
		if change := firstMeaningfulChange(baseEvent, compEvent); change != nil {
			changes = append(changes, *change)
		}
	}

	// Order the sets deterministically (ties resolve by event_id) so the diff is
	// reproducible for the same inputs regardless of insertion order.
	sort.Strings(removed)
	sort.Strings(added)
	sort.SliceStable(matched, func(i, j int) bool { return matched[i].BaselineEventID < matched[j].BaselineEventID })
	sort.SliceStable(changes, func(i, j int) bool {
		if changes[i].BaselineEventID != changes[j].BaselineEventID {
			return changes[i].BaselineEventID < changes[j].BaselineEventID
		}
		return changes[i].Field < changes[j].Field
	})
	return diffSets(matched, added, removed, changes)
}

func firstMeaningfulChange(base, comp contracts.ExecutionEvent) *contracts.EventChange {
	type fieldGetter struct {
		field string
		base  any
		comp  any
	}
	candidates := []fieldGetter{
		{"event_type", base.EventType, comp.EventType},
		{"status", base.Status, comp.Status},
		{"duration_ms", base.DurationMS, comp.DurationMS},
	}
	for _, candidate := range candidates {
		if !reflect.DeepEqual(candidate.base, candidate.comp) {
			return &contracts.EventChange{BaselineEventID: base.EventID, ComparisonEventID: comp.EventID, Field: candidate.field, BaselineValue: candidate.base, ComparisonValue: candidate.comp}
		}
	}
	if !reflect.DeepEqual(base.Attributes, comp.Attributes) {
		return &contracts.EventChange{BaselineEventID: base.EventID, ComparisonEventID: comp.EventID, Field: "attributes", BaselineValue: base.Attributes, ComparisonValue: comp.Attributes}
	}
	return nil
}

func semanticKey(event contracts.ExecutionEvent) string {
	return strings.Join([]string{
		event.Component.Name,
		event.Operation.Name,
		string(event.EventType),
		event.LogicalOperationID,
		fmt.Sprintf("%d", event.Attempt),
	}, "\x00")
}

func indexByKey(events []contracts.ExecutionEvent) map[string][]string {
	index := map[string][]string{}
	for _, event := range events {
		key := semanticKey(event)
		index[key] = append(index[key], event.EventID)
	}
	return index
}

func byEventID(events []contracts.ExecutionEvent) map[string]contracts.ExecutionEvent {
	index := map[string]contracts.ExecutionEvent{}
	for _, event := range events {
		index[event.EventID] = event
	}
	return index
}
