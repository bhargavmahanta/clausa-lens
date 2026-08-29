package checkout

import (
	"context"
	"fmt"
	"sort"

	"github.com/causalens/causalens/internal/contracts"
)

// OracleID and OracleVersion match the frozen FailureOracleRef in
// docs/CONTRACTS.md's P0 examples.
const (
	OracleID      = "duplicate_ledger_effect"
	OracleVersion = "1.0.0"
)

func oracleRef() contracts.FailureOracleRef {
	return contracts.FailureOracleRef{ID: OracleID, Version: OracleVersion}
}

// DetectIncident implements contracts.SystemPack: it evaluates the
// duplicate-effect failure oracle over captured evidence.
func (p *Pack) DetectIncident(_ context.Context, events []contracts.ExecutionEvent) (contracts.OracleResult, error) {
	return evaluate(events)
}

// EvaluateOutcome implements contracts.SystemPack: it evaluates the same
// failure oracle over a captured or replayed execution's events. It does
// not depend on execution.Graph, since replay.Evaluate calls it with only
// Run and Events populated.
func (p *Pack) EvaluateOutcome(_ context.Context, execution contracts.ReplayExecution) (contracts.OracleResult, error) {
	return evaluate(execution.Events)
}

// evaluate is the oracle's evidence-based matching logic, shared by
// DetectIncident and EvaluateOutcome. The oracle matches only when one
// logical_operation_id's evidence contains a checkout TIMEOUT event, a
// RETRY event, an event with attempt >= 2, and at least two committed
// ledger EFFECT events -- exactly the rule in CONTRACTS.md. Every field is
// derived from the supplied evidence; nothing is hardcoded, so the same
// input always produces the same output.
func evaluate(events []contracts.ExecutionEvent) (contracts.OracleResult, error) {
	if len(events) == 0 {
		return contracts.OracleResult{}, fmt.Errorf("checkout: no evidence to evaluate")
	}

	logicalOperationID := selectLogicalOperationID(events)
	group := eventsForLogicalOperation(events, logicalOperationID)

	var (
		hasTimeout     bool
		hasRetry       bool
		maxAttempt     int
		timeoutEventID string
		retryEventID   string
	)
	attemptsSeen := map[int]bool{}
	var committedEffects []contracts.ExecutionEvent

	for _, e := range group {
		if e.Attempt > maxAttempt {
			maxAttempt = e.Attempt
		}
		switch e.EventType {
		case contracts.EventTimeout:
			hasTimeout = true
			if timeoutEventID == "" {
				timeoutEventID = e.EventID
			}
		case contracts.EventRetry:
			hasRetry = true
			if retryEventID == "" {
				retryEventID = e.EventID
			}
		case contracts.EventStart:
			if e.Operation.Kind == contracts.OperationDependency {
				attemptsSeen[e.Attempt] = true
			}
		case contracts.EventEffect:
			if committed, ok := e.Attributes["effect_committed"].(bool); ok && committed {
				committedEffects = append(committedEffects, e)
			}
		}
	}
	sortEvents(committedEffects)

	matched := hasTimeout && hasRetry && maxAttempt >= 2 && len(committedEffects) >= 2

	result := contracts.OracleResult{
		Oracle:  oracleRef(),
		Matched: matched,
		EffectSummary: contracts.EffectSummary{
			PaymentAttemptCount: len(attemptsSeen),
			LedgerCommitCount:   len(committedEffects),
		},
	}

	if matched {
		ids := []string{timeoutEventID, retryEventID}
		for _, e := range committedEffects {
			ids = append(ids, e.EventID)
		}
		result.RequiredEvidenceEventIDs = ids
		result.Explanation = fmt.Sprintf(
			"One logical checkout (%s) produced %d committed ledger effects after a timeout-driven retry.",
			logicalOperationID, len(committedEffects))
		return result, nil
	}

	// Non-match: required_evidence_event_ids must still be non-empty
	// (docs/CONTRACTS.md what-if example), so report every observed event
	// for this logical operation.
	sortEvents(group)
	ids := make([]string, 0, len(group))
	for _, e := range group {
		ids = append(ids, e.EventID)
	}
	result.RequiredEvidenceEventIDs = ids
	result.Explanation = fmt.Sprintf("%s: %s", logicalOperationID, explainMismatch(hasTimeout, hasRetry, maxAttempt, len(committedEffects)))
	return result, nil
}

func explainMismatch(hasTimeout, hasRetry bool, maxAttempt, committedCount int) string {
	switch {
	case !hasTimeout:
		return "no checkout timeout event was observed"
	case !hasRetry:
		return "no retry event was observed"
	case maxAttempt < 2:
		return "no attempt >= 2 was observed"
	case committedCount < 2:
		return "fewer than two committed ledger effects were observed"
	default:
		return "oracle evidence incomplete"
	}
}

// selectLogicalOperationID picks the logical operation to evaluate.
// Deterministic: if every event shares one logical_operation_id (the P0
// golden scenario, one execution per evaluation), that value is used.
// Otherwise the lexicographically smallest logical_operation_id whose
// evidence matches the oracle is preferred; if none match, the
// lexicographically smallest logical_operation_id is used.
func selectLogicalOperationID(events []contracts.ExecutionEvent) string {
	seen := map[string]bool{}
	var ids []string
	for _, e := range events {
		if !seen[e.LogicalOperationID] {
			seen[e.LogicalOperationID] = true
			ids = append(ids, e.LogicalOperationID)
		}
	}
	if len(ids) == 1 {
		return ids[0]
	}
	sort.Strings(ids)
	for _, id := range ids {
		group := eventsForLogicalOperation(events, id)
		if quickMatches(group) {
			return id
		}
	}
	return ids[0]
}

func quickMatches(events []contracts.ExecutionEvent) bool {
	r, _ := evaluate(events)
	return r.Matched
}

func eventsForLogicalOperation(events []contracts.ExecutionEvent, id string) []contracts.ExecutionEvent {
	out := make([]contracts.ExecutionEvent, 0, len(events))
	for _, e := range events {
		if e.LogicalOperationID == id {
			out = append(out, e)
		}
	}
	return out
}

// sortEvents orders events deterministically by component-local sequence
// then event_id, matching the tie-breaking rule CONTRACTS.md uses for
// timeline ordering.
func sortEvents(events []contracts.ExecutionEvent) {
	sort.Slice(events, func(i, j int) bool {
		if events[i].Sequence != events[j].Sequence {
			return events[i].Sequence < events[j].Sequence
		}
		return events[i].EventID < events[j].EventID
	})
}
