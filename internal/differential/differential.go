package differential

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/causalens/causalens/internal/contracts"
	"github.com/causalens/causalens/internal/graph"
)

var (
	// ErrDiffPrerequisites reports that the frozen ReplayDiff prerequisites were
	// not satisfied by the supplied baseline and comparison runs.
	ErrDiffPrerequisites = errors.New("replay diff prerequisites not met")
	// ErrInvalidDiff reports that the assembled ReplayDiff violates the frozen
	// contract after the prerequitites were met.
	ErrInvalidDiff = errors.New("invalid replay diff")
)

// PrerequisitesError describes which frozen prerequisite was not met. It wraps
// ErrDiffPrerequisites so callers can test with errors.Is while still
// inspecting the specific Reason.
type PrerequisitesError struct {
	Reason string
}

func (e *PrerequisitesError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDiffPrerequisites, e.Reason)
}

func (e *PrerequisitesError) Unwrap() error { return ErrDiffPrerequisites }

// Store is the minimal read seam the analyzer needs to assemble a diff. It is
// satisfied by internal/core PostgresRepository (GetRun) plus a run-event
// loader for events and per-run execution graphs.
type Store interface {
	GetRun(ctx context.Context, runID string) (contracts.ReplayRun, error)
	EventsForRun(ctx context.Context, run contracts.ReplayRun) ([]contracts.ExecutionEvent, error)
	GraphsForRun(ctx context.Context, run contracts.ReplayRun) (contracts.ExecutionGraph, error)
}

// Alignment abstracts the pack's semantic event matching so the analyzer is
// pack-agnostic. It returns matched/added/removed/changed event sets for the
// two ordered runs. Implemented by the System Pack at wiring time.
type Alignment interface {
	Align(ctx context.Context, diffID string, baseline contracts.ReplayExecution,
		comparison contracts.ReplayExecution) ([]contracts.EventAlignment,
		[]string, []string, []contracts.EventChange, error)
}

// DiffBuilder is an optional, richer capability a pack may provide. When the
// pack also implements the frozen SystemPack.Compare, differential.Build uses
// its canonical FirstMeaningfulDivergence when the generic field-change
// derivation finds none (a structural divergence, e.g. a removed TIMEOUT event,
// is not a field change on a matched pair). Packs that do not implement it keep
// the generic behavior; core stays pack-agnostic.
type DiffBuilder interface {
	Compare(ctx context.Context, diffID string, baseline contracts.ReplayExecution,
		comparison contracts.ReplayExecution) (contracts.ReplayDiff, error)
}

// Build assembles a contract-valid ReplayDiff for a baseline and comparison run.
// It validates the frozen prerequisites (both COMPLETED, baseline REPRODUCED,
// comparison what-if with a matching baseline_run_id, same capsule hash, exactly
// one intervention) and rejects otherwise with a typed PrerequisitesError.
func Build(ctx context.Context, store Store, diffID, baselineRunID, comparisonRunID string, pack Alignment) (contracts.ReplayDiff, error) {
	if diffID == "" || baselineRunID == "" || comparisonRunID == "" || baselineRunID == comparisonRunID {
		return contracts.ReplayDiff{}, prereq("diff ids must be distinct and non-empty")
	}

	baselineRun, err := store.GetRun(ctx, baselineRunID)
	if err != nil {
		return contracts.ReplayDiff{}, err
	}
	comparisonRun, err := store.GetRun(ctx, comparisonRunID)
	if err != nil {
		return contracts.ReplayDiff{}, err
	}

	if baselineRun.RunType != contracts.RunTypeBaseline {
		return contracts.ReplayDiff{}, prereq("baseline must be a BASELINE run")
	}
	if err := verifyPrerequisites(baselineRun, comparisonRun); err != nil {
		return contracts.ReplayDiff{}, err
	}

	baselineEvents, err := loadOrderedEvents(ctx, store, baselineRun)
	if err != nil {
		return contracts.ReplayDiff{}, err
	}
	comparisonEvents, err := loadOrderedEvents(ctx, store, comparisonRun)
	if err != nil {
		return contracts.ReplayDiff{}, err
	}

	baselineExec := contracts.ReplayExecution{Run: baselineRun, Events: baselineEvents.events, Graph: baselineEvents.graph}
	comparisonExec := contracts.ReplayExecution{Run: comparisonRun, Events: comparisonEvents.events, Graph: comparisonEvents.graph}

	matched, added, removed, changes, err := pack.Align(ctx, diffID, baselineExec, comparisonExec)
	if err != nil {
		return contracts.ReplayDiff{}, err
	}

	baselineSummary := contracts.EffectSummary{}
	comparisonSummary := contracts.EffectSummary{}
	if baselineRun.EffectSummary != nil {
		baselineSummary = *baselineRun.EffectSummary
	}
	if comparisonRun.EffectSummary != nil {
		comparisonSummary = *comparisonRun.EffectSummary
	}

	diff := contracts.ReplayDiff{
		SchemaVersion:           contracts.ContractVersion,
		DiffID:                  diffID,
		BaselineRunID:           baselineRun.RunID,
		ComparisonRunID:         comparisonRun.RunID,
		AlignmentVersion:        contracts.ContractVersion,
		Intervention:            *comparisonRun.Intervention,
		BaselineOracleResult:    *baselineRun.FailureOracleResult,
		ComparisonOracleResult:  *comparisonRun.FailureOracleResult,
		MatchedEvents:           matched,
		AddedEventIDs:           added,
		RemovedEventIDs:         removed,
		ChangedEvents:           changes,
		BaselineEffectSummary:   baselineSummary,
		ComparisonEffectSummary: comparisonSummary,
		EffectDelta: contracts.EffectDelta{
			PaymentAttemptCount: comparisonSummary.PaymentAttemptCount - baselineSummary.PaymentAttemptCount,
			LedgerCommitCount:   comparisonSummary.LedgerCommitCount - baselineSummary.LedgerCommitCount,
		},
		EvidenceSummary: evidenceSummary(baselineSummary, comparisonSummary, matched, added, removed, changes),
		Limitations:     Limitations(""),
	}

	if comparisonRun.Outcome != contracts.ReplayOutcomeInconclusive {
		divergence := FirstDivergenceFrom(baselineEvents.events, comparisonEvents.events, changes, added, removed)
		if divergence == nil {
			// Structural divergence: the pack may express a scenario-specific
			// rule (e.g. a removed TIMEOUT event) that the generic
			// field-change derivation cannot name. Delegate to its Compare.
			if builder, ok := pack.(DiffBuilder); ok {
				full, err := builder.Compare(ctx, diffID, baselineExec, comparisonExec)
				if err == nil {
					divergence = full.FirstMeaningfulDivergence
				}
			}
		}
		diff.FirstMeaningfulDivergence = divergence
	}

	if err := diff.Validate(); err != nil {
		return contracts.ReplayDiff{}, fmt.Errorf("%w: %v", ErrInvalidDiff, err)
	}
	return diff, nil
}

// FirstDivergenceFrom deterministically derives a FirstDivergence from the
// alignment change set. It uses only field-level changes whose paired event
// IDs resolve to known timeline positions; if no such change exists it returns
// nil rather than fabricating values. Callers must clear the result when the
// comparison outcome is INCONCLUSIVE or the runs are structurally identical.
func FirstDivergenceFrom(baseline, comparison []contracts.ExecutionEvent,
	changes []contracts.EventChange, added, removed []string) *contracts.FirstDivergence {
	if len(changes) == 0 {
		return nil
	}
	baseIdx := indexByEventID(baseline)
	compIdx := indexByEventID(comparison)

	type candidate struct {
		change  contracts.EventChange
		baseIdx int
		compIdx int
	}
	var candidates []candidate
	for _, ch := range changes {
		bi, baseOK := baseIdx[ch.BaselineEventID]
		ci, compOK := compIdx[ch.ComparisonEventID]
		if !baseOK || !compOK {
			continue
		}
		candidates = append(candidates, candidate{change: ch, baseIdx: bi, compIdx: ci})
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].baseIdx != candidates[j].baseIdx {
			return candidates[i].baseIdx < candidates[j].baseIdx
		}
		if candidates[i].compIdx != candidates[j].compIdx {
			return candidates[i].compIdx < candidates[j].compIdx
		}
		return candidates[i].change.Field < candidates[j].change.Field
	})

	first := candidates[0].change
	return &contracts.FirstDivergence{
		BaselineEventID:         first.BaselineEventID,
		ComparisonEventID:       first.ComparisonEventID,
		Rule:                    first.Field,
		BaselineValue:           first.BaselineValue,
		ComparisonValue:         first.ComparisonValue,
		BaselineTimelineIndex:   candidates[0].baseIdx,
		ComparisonTimelineIndex: candidates[0].compIdx,
	}
}

// Limitations returns a frozen-valid, non-empty limitation string slice for
// the given pack version (or a generic statement when version is empty).
func Limitations(packVersion string) []string {
	packVersion = strings.TrimSpace(packVersion)
	if packVersion == "" {
		return []string{"Applies to differential analyzer fixtures."}
	}
	return []string{fmt.Sprintf("Applies to %s fixtures.", packVersion)}
}

func verifyPrerequisites(baseline, comparison contracts.ReplayRun) error {
	if baseline.Status != contracts.ReplayRunCompleted {
		return prereq("baseline must be COMPLETED")
	}
	if baseline.Outcome != contracts.ReplayOutcomeReproduced {
		return prereq("baseline outcome must be REPRODUCED")
	}
	if comparison.Status != contracts.ReplayRunCompleted {
		return prereq("comparison must be COMPLETED")
	}
	if comparison.RunType != contracts.RunTypeWhatIf {
		return prereq("comparison must be a WHAT_IF run")
	}
	if comparison.BaselineRunID != baseline.RunID {
		return prereq("comparison baseline_run_id must match the baseline run")
	}
	if comparison.CapsuleHash != baseline.CapsuleHash {
		return prereq("compared runs must share the same capsule hash")
	}
	if comparison.Intervention == nil {
		return prereq("comparison must record exactly one intervention")
	}
	return nil
}

func prereq(reason string) error {
	return &PrerequisitesError{Reason: reason}
}

type orderedEvents struct {
	events []contracts.ExecutionEvent
	graph  contracts.ExecutionGraph
}

func loadOrderedEvents(ctx context.Context, store Store, run contracts.ReplayRun) (orderedEvents, error) {
	raw, err := store.EventsForRun(ctx, run)
	if err != nil {
		return orderedEvents{}, err
	}
	graphForRun, err := store.GraphsForRun(ctx, run)
	if err != nil {
		return orderedEvents{}, err
	}
	ordered, err := graph.Order(raw, graphForRun)
	if err != nil {
		return orderedEvents{}, err
	}
	return orderedEvents{events: ordered, graph: graphForRun}, nil
}

func indexByEventID(events []contracts.ExecutionEvent) map[string]int {
	index := make(map[string]int, len(events))
	for i, event := range events {
		index[event.EventID] = i
	}
	return index
}

func evidenceSummary(baseline, comparison contracts.EffectSummary, matched []contracts.EventAlignment, added, removed []string, changes []contracts.EventChange) string {
	return fmt.Sprintf(
		"What-if reconstruction changed the event stream: %d matched, %d added, %d removed, %d changed. Effect delta %+d payment attempts, %+d ledger commits.",
		len(matched), len(added), len(removed), len(changes),
		comparison.PaymentAttemptCount-baseline.PaymentAttemptCount,
		comparison.LedgerCommitCount-baseline.LedgerCommitCount,
	)
}
