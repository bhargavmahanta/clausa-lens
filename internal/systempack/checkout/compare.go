package checkout

import (
	"context"
	"fmt"

	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/differential"
)

// alignKey is the semantic match key from docs/SYSTEM_PACKS.md's
// comparison rules: "Align events by component.name, operation.name,
// logical operation identifier, event type, and attempt."
type alignKey struct {
	component string
	operation string
	eventType contracts.EventType
	logicalOp string
	attempt   int
}

func keyOf(e contracts.ExecutionEvent) alignKey {
	return alignKey{component: e.Component.Name, operation: e.Operation.Name, eventType: e.EventType, logicalOp: e.LogicalOperationID, attempt: e.Attempt}
}

// Align implements differential.Alignment (internal/differential.Alignment)
// so the live Core API's POST /v1/diffs route can use this pack directly.
// It is not part of the frozen contracts.SystemPack interface; Compare
// (which is) builds its ReplayDiff on top of Align so both paths share one
// alignment implementation.
func (p *Pack) Align(_ context.Context, _ string, baseline, comparison contracts.ReplayExecution) ([]contracts.EventAlignment, []string, []string, []contracts.EventChange, error) {
	matched := []contracts.EventAlignment{}
	added := []string{}
	removed := []string{}
	changed := []contracts.EventChange{}

	comparisonByKey := map[alignKey][]contracts.ExecutionEvent{}
	for _, e := range comparison.Events {
		k := keyOf(e)
		comparisonByKey[k] = append(comparisonByKey[k], e)
	}
	consumed := map[alignKey]int{}
	usedComparison := map[string]bool{}

	for _, b := range baseline.Events {
		k := keyOf(b)
		candidates := comparisonByKey[k]
		idx := consumed[k]
		if idx >= len(candidates) {
			removed = append(removed, b.EventID)
			continue
		}
		c := candidates[idx]
		consumed[k] = idx + 1
		usedComparison[c.EventID] = true
		matched = append(matched, contracts.EventAlignment{BaselineEventID: b.EventID, ComparisonEventID: c.EventID})
		if b.Status != c.Status {
			changed = append(changed, contracts.EventChange{
				BaselineEventID: b.EventID, ComparisonEventID: c.EventID,
				Field: "status", BaselineValue: string(b.Status), ComparisonValue: string(c.Status),
			})
		}
	}
	for _, c := range comparison.Events {
		if !usedComparison[c.EventID] {
			added = append(added, c.EventID)
		}
	}
	return matched, added, removed, changed, nil
}

// Compare implements contracts.SystemPack: it aligns a baseline and
// comparison execution and assembles a full, contract-valid ReplayDiff.
// diffID is supplied by Core so the pack never invents resource identity.
func (p *Pack) Compare(ctx context.Context, diffID string, baseline, comparison contracts.ReplayExecution) (contracts.ReplayDiff, error) {
	matched, added, removed, changed, err := p.Align(ctx, diffID, baseline, comparison)
	if err != nil {
		return contracts.ReplayDiff{}, err
	}

	baselineOracle, err := p.EvaluateOutcome(ctx, baseline)
	if err != nil {
		return contracts.ReplayDiff{}, fmt.Errorf("checkout: evaluate baseline outcome: %w", err)
	}
	comparisonOracle, err := p.EvaluateOutcome(ctx, comparison)
	if err != nil {
		return contracts.ReplayDiff{}, fmt.Errorf("checkout: evaluate comparison outcome: %w", err)
	}

	var intervention contracts.Intervention
	if comparison.Run.Intervention != nil {
		intervention = *comparison.Run.Intervention
	}

	diff := contracts.ReplayDiff{
		SchemaVersion:           contracts.ContractVersion,
		DiffID:                  diffID,
		BaselineRunID:           baseline.Run.RunID,
		ComparisonRunID:         comparison.Run.RunID,
		AlignmentVersion:        contracts.ContractVersion,
		Intervention:            intervention,
		BaselineOracleResult:    baselineOracle,
		ComparisonOracleResult:  comparisonOracle,
		MatchedEvents:           matched,
		AddedEventIDs:           added,
		RemovedEventIDs:         removed,
		ChangedEvents:           changed,
		BaselineEffectSummary:   baselineOracle.EffectSummary,
		ComparisonEffectSummary: comparisonOracle.EffectSummary,
		EffectDelta: contracts.EffectDelta{
			PaymentAttemptCount: comparisonOracle.EffectSummary.PaymentAttemptCount - baselineOracle.EffectSummary.PaymentAttemptCount,
			LedgerCommitCount:   comparisonOracle.EffectSummary.LedgerCommitCount - baselineOracle.EffectSummary.LedgerCommitCount,
		},
		EvidenceSummary: evidenceSummary(baselineOracle.EffectSummary, comparisonOracle.EffectSummary, matched, added, removed, changed),
		Limitations:     differential.Limitations(fmt.Sprintf("%s pack v%s", PackID, PackVersion)),
	}
	diff.FirstMeaningfulDivergence = firstMeaningfulDivergence(baseline, comparison, changed, added, removed)

	if err := diff.Validate(); err != nil {
		return contracts.ReplayDiff{}, fmt.Errorf("checkout: assembled replay diff is invalid: %w", err)
	}
	return diff, nil
}

func evidenceSummary(baseline, comparison contracts.EffectSummary, matched []contracts.EventAlignment, added, removed []string, changed []contracts.EventChange) string {
	return fmt.Sprintf(
		"checkout_duplicate_effect: %d matched, %d added, %d removed, %d changed. Effect delta %+d payment attempts, %+d ledger commits.",
		len(matched), len(added), len(removed), len(changed),
		comparison.PaymentAttemptCount-baseline.PaymentAttemptCount,
		comparison.LedgerCommitCount-baseline.LedgerCommitCount,
	)
}

// firstMeaningfulDivergence prefers Bhargav's generic field-change-based
// derivation (differential.FirstDivergenceFrom). The golden scenario's
// defining divergence is structural rather than a field change on a matched
// pair -- the checkout TIMEOUT event exists only in the baseline because
// payment completed before the timeout in the comparison -- so when the
// generic derivation finds nothing, this looks for exactly that removed
// TIMEOUT event and reports the documented PAYMENT_COMPLETES_BEFORE_TIMEOUT
// rule (docs/CONTRACTS.md ReplayDiff P0 example).
func firstMeaningfulDivergence(baseline, comparison contracts.ReplayExecution, changed []contracts.EventChange, added, removed []string) *contracts.FirstDivergence {
	if d := differential.FirstDivergenceFrom(baseline.Events, comparison.Events, changed, added, removed); d != nil {
		return d
	}

	removedSet := make(map[string]bool, len(removed))
	for _, id := range removed {
		removedSet[id] = true
	}
	for i, e := range baseline.Events {
		if e.EventType != contracts.EventTimeout || !removedSet[e.EventID] {
			continue
		}
		compIdx, compID := 0, ""
		for j, c := range comparison.Events {
			if c.Component.Name == "payment" && c.EventType == contracts.EventComplete {
				compIdx, compID = j, c.EventID
				break
			}
		}
		return &contracts.FirstDivergence{
			BaselineEventID:         e.EventID,
			ComparisonEventID:       compID,
			Rule:                    "PAYMENT_COMPLETES_BEFORE_TIMEOUT",
			BaselineValue:           "TIMEOUT",
			ComparisonValue:         "SUCCESS",
			BaselineTimelineIndex:   i,
			ComparisonTimelineIndex: compIdx,
		}
	}
	return nil
}
